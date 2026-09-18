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

func TestQueuedCostWallChangeStopsAutomaticProbeButKeepsManualDiagnosis(t *testing.T) {
	for _, test := range []struct {
		name          string
		automatic     bool
		disabledField string
		ignoreBefore  bool
		ignoreAfter   bool
	}{
		{name: "automatic probe rechecks queued cost change", automatic: true},
		{name: "manual diagnosis remains available"},
		{name: "queued master switch change allows automatic probe", automatic: true, disabledField: "enabled"},
		{name: "queued probe switch change allows automatic probe", automatic: true, disabledField: "stop_auto_probe"},
		{name: "queued account exemption allows automatic probe", automatic: true, ignoreAfter: true},
		{name: "removing queued account exemption stops automatic probe", automatic: true, ignoreBefore: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cost-wall.sqlite3")
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
			if _, err := db.Exec(`INSERT INTO local_groups(name,remote_id,rate_multiplier,updated_at) VALUES('codex','7','1','now');
				INSERT INTO accounts(id,name,multiplier,schedulable,metadata_json,updated_at)
				VALUES('41','cost-wall','0.5',1,'{"known_models":["test-model"]}','now');
				INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','codex','7')`); err != nil {
				t.Fatal(err)
			}
			var reads, generated atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/41" {
					reads.Add(1)
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
						"id": "41", "type": "apikey", "platform": "openai",
						"credentials": map[string]any{"base_url": "http://" + request.Host, "api_key": "isolated-test"},
					}})
					return
				}
				if request.Method == http.MethodPost && request.URL.Path == "/v1/responses" {
					generated.Add(1)
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\n"))
					return
				}
				t.Errorf("unexpected request: %s %s", request.Method, request.URL.Path)
				http.NotFound(w, request)
			}))
			t.Cleanup(server.Close)
			runner, tasks := &protectionRunner{}, &protectionTasks{}
			service := probe.New(store, protectionTarget{endpoint: server.URL}, tasks)
			service.UseTaskRunner(runner)
			id := "41"
			if test.ignoreBefore {
				if err := store.SetAccountIgnoreCostWall(t.Context(), id, true, "test"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := service.Enqueue(t.Context(), probe.Request{AccountID: &id, Automatic: test.automatic}, "test"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`UPDATE accounts SET multiplier='1',routing_state='cost_blocked',schedulable=0`); err != nil {
				t.Fatal(err)
			}
			if test.disabledField != "" {
				if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{
					"cost_wall": map[string]any{test.disabledField: false},
				}}, "test"); err != nil {
					t.Fatal(err)
				}
			}
			if test.ignoreBefore || test.ignoreAfter {
				if err := store.SetAccountIgnoreCostWall(t.Context(), id, test.ignoreAfter, "test"); err != nil {
					t.Fatal(err)
				}
			}
			runner.run(t.Context())
			var samples int
			if err := db.QueryRow(`SELECT COUNT(*) FROM health_samples`).Scan(&samples); err != nil {
				t.Fatal(err)
			}
			if test.automatic && test.disabledField == "" && !test.ignoreAfter {
				if reads.Load() != 0 || generated.Load() != 0 || samples != 0 {
					t.Fatalf("queued automatic probe must skip credentials, generation and evidence: reads=%d generated=%d samples=%d", reads.Load(), generated.Load(), samples)
				}
			} else if generated.Load() != 1 || samples != 1 {
				t.Fatalf("allowed probe must remain usable: generated=%d samples=%d task=%+v", generated.Load(), samples, tasks.last)
			}
		})
	}
}
