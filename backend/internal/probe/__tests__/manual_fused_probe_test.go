package probe_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
)

func TestManualProbeDiagnosesFusedAccountWithoutResumingScheduling(t *testing.T) {
	for _, scope := range []string{"account", "batch", "platform", "upstream-failure", "automatic", "recovery"} {
		t.Run(scope, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "manual-fused-probe.db")
			store, err := business.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			if err := store.Bootstrap(t.Context()); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			if _, err := db.Exec(`INSERT INTO accounts(id,name,schedulable,metadata_json,updated_at)
				VALUES('41','diagnosis',1,'{"platform":"openai","known_models":["test-model"]}','now');
				INSERT INTO account_groups(account_id,group_name) VALUES('41','default')`); err != nil {
				t.Fatal(err)
			}
			if err := store.CommitAccountControlReadback(t.Context(), "41", "fuse", "test", false, business.AccountOperation{
				OperationID: "test-fuse", OperationType: "account.control", State: "succeeded", Phase: "readback",
				Actor: "test", RemoteConfirmed: true, ReadbackConfirmed: true, Writeback: true,
			}); err != nil {
				t.Fatal(err)
			}
			var generated atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts/41" {
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
						"id": "41", "type": "apikey", "platform": "openai", "schedulable": false,
						"credentials": map[string]any{"base_url": "http://" + r.Host, "api_key": "isolated-test"},
					}})
					return
				}
				if r.Method == http.MethodPost && r.URL.Path == "/v1/responses" {
					generated.Add(1)
					if scope == "upstream-failure" {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusBadRequest)
						_, _ = w.Write([]byte(`{"error":{"message":"test model unavailable"}}`))
						return
					}
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\n"))
					return
				}
				t.Errorf("unexpected request, including any scheduling write: %s %s", r.Method, r.URL.Path)
				http.NotFound(w, r)
			}))
			t.Cleanup(server.Close)
			runner, tasks := &protectionRunner{}, &protectionTasks{}
			service := probe.New(store, protectionTarget{endpoint: server.URL}, tasks)
			service.UseTaskRunner(runner)
			id, platform := "41", "openai"
			request := probe.Request{AccountID: &id}
			switch scope {
			case "batch":
				request = probe.Request{SelectedAccountIDs: []string{id}}
			case "platform":
				request = probe.Request{Platform: &platform, ProbeModel: "test-model"}
			case "automatic":
				request.Automatic = true
			case "recovery":
				request = probe.Request{AccountIDs: []string{id}}
			}
			_, err = service.Enqueue(t.Context(), request, "test")
			if scope == "automatic" || scope == "recovery" {
				if err == nil || runner.run != nil {
					t.Fatal("automatic probes must not bypass a manual fuse")
				}
				return
			}
			if err != nil {
				t.Fatalf("explicit manual diagnosis rejected: %v", err)
			}
			runner.run(t.Context())
			outcome, result := "passed", "通过"
			if scope == "upstream-failure" {
				outcome, result = "failed", "失败"
			}
			if generated.Load() != 1 || tasks.last.Result[outcome] != 1 {
				t.Fatalf("manual diagnosis did not run: generated=%d task=%+v", generated.Load(), tasks.last)
			}
			protection, err := store.AccountMutationProtection(t.Context(), id)
			if err != nil || !protection.ManualFused {
				t.Fatalf("manual fuse lost: protection=%+v err=%v", protection, err)
			}
			var schedulable bool
			if err := db.QueryRow(`SELECT schedulable FROM accounts WHERE id='41'`).Scan(&schedulable); err != nil {
				t.Fatal(err)
			}
			if schedulable {
				t.Fatal("diagnosis must not resume scheduling")
			}
			samples, err := store.RoutingSamples(t.Context(), &id, nil, "active_probe", 10)
			if err != nil || len(samples) != 1 || samples[0].Result != result {
				t.Fatalf("diagnosis evidence missing: samples=%+v err=%v", samples, err)
			}
		})
	}
}
