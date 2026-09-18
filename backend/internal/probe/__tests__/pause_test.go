package probe_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
)

func TestPauseRechecksQueuedAutomaticProbesAndRetriesButAllowsManualProbes(t *testing.T) {
	for _, mode := range []string{"automatic", "recovery", "manual", "retry"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "pause.sqlite3")
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
				VALUES('41','scheduled',1,'{"known_models":["test-model"]}','now');
				INSERT INTO account_groups(account_id,group_name) VALUES('41','codex')`); err != nil {
				t.Fatal(err)
			}
			_, err = store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"probe": map[string]any{
				"retry_enabled": true, "retry_count": 2,
			}}}, "test")
			if err != nil {
				t.Fatal(err)
			}
			pause := func() error {
				now := time.Now().UTC()
				_, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"probe": map[string]any{
					"pause_window": map[string]any{"enabled": true, "start": now.Add(-time.Hour).Format("15:04"), "end": now.Add(time.Hour).Format("15:04"), "timezone": "UTC"},
				}}}, "test")
				return err
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
					if mode == "retry" {
						if err := pause(); err != nil {
							t.Error(err)
						}
						http.Error(w, "retryable failure", http.StatusServiceUnavailable)
						return
					}
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
			request := probe.Request{AccountID: &id, Automatic: mode != "manual"}
			if mode == "recovery" {
				request = probe.Request{AccountIDs: []string{id}}
			}
			if _, err := service.Enqueue(t.Context(), request, "test"); err != nil {
				t.Fatal(err)
			}
			if mode != "retry" {
				if err := pause(); err != nil {
					t.Fatal(err)
				}
			}
			runner.run(t.Context())
			var samples int
			if err := db.QueryRow(`SELECT COUNT(*) FROM health_samples`).Scan(&samples); err != nil {
				t.Fatal(err)
			}
			if mode == "automatic" || mode == "recovery" {
				if reads.Load() != 0 || generated.Load() != 0 || samples != 0 || tasks.last.Result["skipped"] != 1 {
					t.Fatalf("pause must skip credentials, generation and health evidence: reads=%d generated=%d samples=%d task=%+v", reads.Load(), generated.Load(), samples, tasks.last)
				}
				results := tasks.last.Result["results"].([]probe.Result)
				if results[0].FailureReason == nil || !strings.Contains(*results[0].FailureReason, "暂停时段") {
					t.Fatalf("missing pause reason: %+v", results)
				}
			} else if generated.Load() != 1 || samples != 1 {
				t.Fatalf("manual request or initial retry attempt must run exactly once: generated=%d samples=%d task=%+v", generated.Load(), samples, tasks.last)
			}
		})
	}
}
