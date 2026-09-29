package inspection_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
	"github.com/MIEnchating/sub2api-console/backend/internal/inspection"
	"github.com/MIEnchating/sub2api-console/backend/internal/management"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskcontext"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type recoveryRunner struct{ run func(context.Context) }

func (r *recoveryRunner) Go(f func(context.Context)) error               { r.run = f; return nil }
func (r *recoveryRunner) GoTask(_ string, f func(context.Context)) error { return r.Go(f) }
func (*recoveryRunner) CancelTask(string) bool                           { return false }

func TestAutomaticInspectionRestartsWithOriginalIDAndCurrentCatalog(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	store, err := business.Open(filepath.Join(t.TempDir(), "business.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetMode(context.Background(), runtimepolicy.Monitoring); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdatePolicy(context.Background(), map[string]any{"advanced_policy": map[string]any{"probe": map[string]any{"enabled": false}, "recovery": map[string]any{"enabled": false}}}, "test"); err != nil {
		t.Fatal(err)
	}
	config := business.DefaultAutoInspectionConfig()
	config.Enabled = true
	if _, err := store.UpdateAutoInspectionConfig(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"upstream-sync", "traffic"} {
		if err := store.MarkInspectionTask(context.Background(), name, time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
	}
	tasks, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer tasks.Close()
	var interrupted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if interrupted.CompareAndSwap(false, true) {
			cancel(taskcontext.ErrInterrupted)
			return
		}
		if r.Method != http.MethodGet {
			t.Errorf("unexpected write: %s", r.Method)
		}
		items := []map[string]any{}
		switch r.URL.Path {
		case "/api/v1/admin/groups":
			items = append(items, map[string]any{"id": "7", "name": "test"})
		case "/api/v1/admin/accounts":
			items = append(items, map[string]any{"id": "41", "name": "current", "group_ids": []any{"7"}, "schedulable": true, "rate_multiplier": "1"})
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": items, "total": len(items)}})
	}))
	defer server.Close()
	target := managementTarget{collectionTarget{server.URL}}
	runner := inspection.NewRunner(store, target, evidence.New(store, nil), routing.NewService(store), nil, nil, nil, tasks, management.New(target, store, tasks))
	result, err := runner.Execute(ctx, config)
	if err != nil || result.TaskID == nil {
		t.Fatalf("initial run: %#v %v", result, err)
	}
	pending, err := tasks.PendingRecovery(context.Background())
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending: %#v %v", pending, err)
	}
	scheduler, err := inspection.NewScheduler(store, runner)
	if err != nil {
		t.Fatal(err)
	}
	service := inspection.NewManualService(scheduler, runner, tasks)
	pool := &recoveryRunner{}
	service.UseTaskRunner(pool)
	if err := service.ResumeTask(context.Background(), pending[0]); err != nil {
		t.Fatal(err)
	}
	pool.run(context.Background())
	final, err := tasks.Get(context.Background(), *result.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != "succeeded" || final.Result["management_sync"] == nil {
		t.Fatalf("not recovered: %#v", final)
	}
	ids, err := store.ManagementAccountIDs(context.Background())
	if err != nil || len(ids) != 1 || ids[0] != "41" {
		t.Fatalf("old catalog used: %v %v", ids, err)
	}
}

func TestRecoveryReleasesOnlyUnfinishedStageMarkerOwnedByInterruptedRun(t *testing.T) {
	for _, scenario := range []struct {
		name                      string
		completed, newer, wantDue bool
	}{
		{name: "unfinished stage", wantDue: true},
		{name: "completed stage", completed: true},
		{name: "newer heartbeat owns marker", newer: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := context.Background()
			store, err := business.Open(filepath.Join(t.TempDir(), "business.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			if err := store.Bootstrap(ctx); err != nil {
				t.Fatal(err)
			}
			tasks, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer tasks.Close()
			target := collectionTarget{"http://127.0.0.1:1"}
			runner := inspection.NewRunner(store, target, nil, nil, nil, nil, nil, tasks)
			task, err := runner.QueueTask(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			started := time.Now().UTC().Add(-time.Second)
			marker := started
			if scenario.newer {
				marker = started.Add(time.Second)
			}
			if err := store.MarkInspectionTask(ctx, "traffic", marker); err != nil {
				t.Fatal(err)
			}
			task.Result = map[string]any{"run_started_at": started.Format(time.RFC3339Nano), "planned_operations": []inspection.QueueOperation{{Operation: "traffic_refresh"}}, "active_operations": []string{"traffic_refresh"}}
			if scenario.completed {
				task.Result["completed_operations"] = []string{"traffic_refresh"}
				task.Result["active_operations"] = []string{}
			}
			scheduler, err := inspection.NewScheduler(store, runner)
			if err != nil {
				t.Fatal(err)
			}
			service := inspection.NewManualService(scheduler, runner, tasks)
			service.UseTaskRunner(&recoveryRunner{})
			if err := service.ResumeTask(ctx, task); err != nil {
				t.Fatal(err)
			}
			due, err := store.InspectionTaskDue(ctx, "traffic", 60, time.Now().UTC())
			if err != nil || due != scenario.wantDue {
				t.Fatalf("due=%v wanted=%v err=%v", due, scenario.wantDue, err)
			}
		})
	}
}
