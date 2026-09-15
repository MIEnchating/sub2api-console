package probe_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type protectionTarget struct{ endpoint string }

func (s protectionTarget) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return configstore.TargetSettings{BaseURL: s.endpoint, AdminKey: "test-key", TimeoutSeconds: 1}, nil
}

type protectionTasks struct{ last taskstore.Task }

func (s *protectionTasks) Save(_ context.Context, task taskstore.Task) error {
	s.last = task
	return nil
}

type protectionRunner struct{ run func(context.Context) }

func (r *protectionRunner) Go(run func(context.Context)) error {
	r.run = run
	return nil
}

func (r *protectionRunner) GoTask(_ string, run func(context.Context)) error { return r.Go(run) }
func (*protectionRunner) CancelTask(string) bool                             { return false }

func TestQueuedProbeSkipsNewlyProtectedAccountsWithoutHealthEvidence(t *testing.T) {
	for _, protection := range []string{"manual-priority", "manual-fuse"} {
		t.Run(protection, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "probe.sqlite3")
			repository, err := business.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = repository.Close() })
			ctx := context.Background()
			if err := repository.Bootstrap(ctx); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.Exec(`INSERT INTO accounts(id,name,schedulable,priority,load_factor,concurrency,metadata_json,updated_at)
				VALUES('41','protected',1,20,'1',3,'{"known_models":["test-model"]}','now'),
				('42','available',1,20,'1',3,'{"known_models":["test-model"]}','now');
				INSERT INTO account_groups(account_id,group_name) VALUES('41','codex'),('42','codex')`)
			_ = db.Close()
			if err != nil {
				t.Fatal(err)
			}
			var protectedCalls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/accounts/41") || r.Header.Get("Authorization") == "Bearer probe-account-41" {
					protectedCalls.Add(1)
				}
				if r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts/42" {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
						"id": "42", "type": "apikey", "platform": "openai",
						"credentials": map[string]any{"base_url": "http://" + r.Host, "api_key": "probe-account-42"},
					}})
					return
				}
				if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "Bearer probe-account-42" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\n"))
			}))
			t.Cleanup(server.Close)
			runner, tasks := &protectionRunner{}, &protectionTasks{}
			service := probe.New(repository, protectionTarget{endpoint: server.URL}, tasks)
			service.UseTaskRunner(runner)
			if _, err := service.Enqueue(ctx, probe.Request{SelectedAccountIDs: []string{"41", "42"}}, "test"); err != nil {
				t.Fatal(err)
			}
			if protection == "manual-priority" {
				_, err = repository.AssignManualPriority(ctx, "41", 3, "100", 100, false, "test")
			} else {
				err = repository.CommitAccountControlReadback(ctx, "41", "fuse", "test", false, business.AccountOperation{
					OperationID: "test-fuse", OperationType: "account.control", State: "succeeded", Phase: "readback",
					Actor: "test", RemoteConfirmed: true, ReadbackConfirmed: true, Writeback: true,
				})
			}
			if err != nil {
				t.Fatal(err)
			}
			runner.run(ctx)
			if protectedCalls.Load() != 0 || tasks.last.Result["skipped"] != 1 || tasks.last.Result["passed"] != 1 {
				t.Fatalf("queued probe ignored new protection: calls=%d task=%+v", protectedCalls.Load(), tasks.last)
			}
			accountID := "41"
			samples, err := repository.RoutingSamples(ctx, &accountID, nil, "active-probe", 10)
			if err != nil || len(samples) != 0 {
				t.Fatalf("protected account received new health evidence: samples=%+v err=%v", samples, err)
			}
		})
	}
}
