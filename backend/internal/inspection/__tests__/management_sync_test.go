package inspection_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
	"github.com/MIEnchating/sub2api-console/backend/internal/inspection"
	"github.com/MIEnchating/sub2api-console/backend/internal/management"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type managementTarget struct{ collectionTarget }

func (managementTarget) AccountDefaults(context.Context) (configstore.AccountDefaultsSettings, error) {
	return configstore.AccountDefaultsSettings{}, nil
}

func TestHeartbeatSynchronizesManagementEvenWhenOtherTasksAreNotDue(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "catalog changes are visible on each heartbeat", true: "failed catalog read stops the heartbeat"}[failure], func(t *testing.T) {
			store, err := business.Open(filepath.Join(t.TempDir(), "business.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			if err := store.Bootstrap(t.Context()); err != nil {
				t.Fatal(err)
			}
			if _, err := store.SetMode(t.Context(), runtimepolicy.Monitoring); err != nil {
				t.Fatal(err)
			}
			if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"probe": map[string]any{"enabled": false}, "recovery": map[string]any{"enabled": false}}}, "test"); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"upstream-sync", "traffic"} {
				if err := store.MarkInspectionTask(t.Context(), name, time.Now().UTC()); err != nil {
					t.Fatal(err)
				}
			}
			tasks, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = tasks.Close() })
			accountID := "41"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.Method != http.MethodGet {
					t.Errorf("unexpected write: %s", req.Method)
					w.WriteHeader(405)
					return
				}
				if failure {
					http.Error(w, "catalog unavailable", 503)
					return
				}
				var items []map[string]any
				switch req.URL.Path {
				case "/api/v1/admin/groups":
					items = []map[string]any{{"id": "7", "name": "codex"}}
				case "/api/v1/admin/accounts":
					items = []map[string]any{{"id": accountID, "name": "same-name", "group_ids": []any{"7"}, "schedulable": true, "rate_multiplier": "1"}}
				default:
					t.Errorf("unexpected call when no task is due: %s", req.URL.Path)
					w.WriteHeader(404)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": items, "total": len(items)}})
			}))
			t.Cleanup(server.Close)
			target := managementTarget{collectionTarget{server.URL}}
			runner := inspection.NewRunner(store, target, evidence.New(store, nil), routing.NewService(store), nil, nil, nil, tasks, management.New(target, store, tasks))
			for _, id := range []string{"41", "42"} {
				accountID = id
				result, err := runner.Execute(t.Context(), business.DefaultAutoInspectionConfig())
				if err != nil {
					t.Fatal(err)
				}
				if result.TaskID == nil || result.Skipped {
					t.Fatalf("heartbeat skipped management synchronization: %+v", result)
				}
				task, err := tasks.Get(t.Context(), *result.TaskID)
				if err != nil {
					t.Fatal(err)
				}
				if failure {
					if result.Status != "failed" || task.Result["error"] == nil || task.Result["routing"] != nil {
						t.Fatalf("failed sync continued: %+v", task)
					}
					continue
				}
				ids, err := store.ManagementAccountIDs(t.Context())
				if err != nil || !reflect.DeepEqual(ids, []string{id}) {
					t.Fatalf("catalog=%v err=%v", ids, err)
				}
				if result.Status != "succeeded" || !reflect.DeepEqual(result.Operations, []string{"management_sync"}) || task.Result["management_sync"] == nil {
					t.Fatalf("missing synchronization result: %+v %+v", result, task)
				}
			}
			preview, err := runner.Preview(t.Context(), time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			if len(preview.Operations) == 0 || preview.Operations[0].Operation != "management_sync" || !preview.Operations[0].Due {
				t.Fatalf("preview omitted heartbeat synchronization: %+v", preview)
			}
		})
	}
}
