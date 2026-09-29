package management_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/management"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskcontext"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type recoveryRunner struct{ runs []func(context.Context) }

func (r *recoveryRunner) Go(f func(context.Context)) error               { r.runs = append(r.runs, f); return nil }
func (r *recoveryRunner) GoTask(_ string, f func(context.Context)) error { return r.Go(f) }
func (*recoveryRunner) CancelTask(string) bool                           { return false }

func TestRateSyncRestartReadsCurrentValuesAndDoesNotRepeatMatchingWrite(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "resume", true: "changed target rejected"}[changed], func(t *testing.T) {
			store, _ := rateCollectionStore(t)
			tasks, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer tasks.Close()
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet && !strings.Contains(r.URL.Path, "upstream-billing-probe") {
					t.Errorf("unexpected write: %s %s", r.Method, r.URL.Path)
				}
				switch r.URL.Path {
				case "/api/v1/admin/accounts/upstream-billing-probe/batch":
					if calls.Add(1) == 1 {
						cancel(taskcontext.ErrInterrupted)
						return
					}
					w.Write([]byte(`{"data":{"results":[{"account_id":11,"snapshot":{"status":"ok","data":{"resolved_rate_multiplier":1}}}]}}`))
				case "/api/v1/admin/accounts":
					w.Write([]byte(`{"data":{"items":[{"id":11,"name":"Relay-1","rate_multiplier":1}],"total":1}}`))
				default:
					t.Errorf("unexpected endpoint: %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			target := &rateCollectionTarget{settings: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "restart-private-key", TimeoutSeconds: 5}}
			service := management.New(target, store, tasks, accountops.New(target, store, tasks))
			runner := &recoveryRunner{}
			service.UseTaskRunner(runner)
			task, err := service.EnqueueAccountRateSync(context.Background(), []string{"11"}, "test")
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(task.Recovery)
			if strings.Contains(string(raw), target.settings.AdminKey) {
				t.Fatal("recovery persisted admin key")
			}
			runner.runs[0](ctx)
			pending, err := tasks.PendingRecovery(context.Background())
			if err != nil || len(pending) != 1 {
				t.Fatalf("not recoverable: %v %v", pending, err)
			}
			if changed {
				target.changeKey()
			}
			restarted := management.New(target, store, tasks, accountops.New(target, store, tasks))
			next := &recoveryRunner{}
			restarted.UseTaskRunner(next)
			err = restarted.ResumeTask(context.Background(), pending[0])
			if changed {
				if !errors.Is(err, targetguard.ErrChanged) {
					t.Fatalf("changed target: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			next.runs[0](context.Background())
			final, err := tasks.Get(context.Background(), task.ID)
			if err != nil {
				t.Fatal(err)
			}
			if final.Status != "succeeded" || final.Result["unchanged"] != json.Number("1") || calls.Load() != 2 {
				t.Fatalf("recovery failed: %#v calls=%d", final, calls.Load())
			}
		})
	}
}
