package management_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/management"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type defaultsTaskResults struct{ final chan taskstore.Task }

func (results defaultsTaskResults) Save(_ context.Context, task taskstore.Task) error {
	if task.Status == "failed" || task.Status == "succeeded" {
		results.final <- task
	}
	return nil
}

func TestDefaultsRepairRequiresExplicitNullWhenClearingLoadFactor(t *testing.T) {
	for _, scenario := range []string{"missing", "empty", "unchanged", "cleared"} {
		t.Run(scenario, func(t *testing.T) {
			store, db := rateCollectionStore(t)
			_, err := db.Exec(`UPDATE accounts SET load_factor='-1',priority=20,concurrency=10 WHERE id='11';
				INSERT INTO operation_audit(operation_id,operation_type,state,phase,object_id,created_at)
				VALUES('onboard-test','account.onboarding','succeeded','complete','11','now')`)
			if err != nil {
				t.Fatal(err)
			}
			var written atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if request.URL.Path != "/api/v1/admin/accounts/11" {
					http.NotFound(w, request)
					return
				}
				if request.Method == http.MethodPut {
					written.Store(true)
				}
				row := map[string]any{"id": 11, "name": "Relay-1", "concurrency": 10, "priority": 20, "load_factor": -1}
				if written.Load() {
					switch scenario {
					case "missing":
						delete(row, "load_factor")
					case "empty":
						row["load_factor"] = ""
					case "cleared":
						row["load_factor"] = nil
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": row})
			}))
			t.Cleanup(server.Close)
			results := defaultsTaskResults{final: make(chan taskstore.Task, 1)}
			runner := taskrunner.NewBounded(t.Context(), 1)
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_ = runner.Shutdown(ctx)
			})
			target := &rateCollectionTarget{settings: configstore.TargetSettings{BaseURL: server.URL, AdminKey: "test-key", TimeoutSeconds: 2}}
			service := management.New(target, store, results)
			service.UseTaskRunner(runner)
			if _, err := service.EnqueueAccountDefaultsRepair(t.Context(), []string{"11"}, "test"); err != nil {
				t.Fatal(err)
			}
			var final taskstore.Task
			select {
			case final = <-results.final:
			case <-time.After(5 * time.Second):
				t.Fatal("defaults repair did not finish")
			}
			wantStatus := "failed"
			if scenario == "cleared" {
				wantStatus = "succeeded"
			}
			if final.Status != wantStatus || final.Result["remote_write"] != true {
				t.Fatalf("readback result was misreported: %+v", final)
			}
			var load sql.NullString
			if err := db.QueryRow(`SELECT load_factor FROM accounts WHERE id='11'`).Scan(&load); err != nil {
				t.Fatal(err)
			}
			if scenario == "cleared" {
				if load.Valid {
					t.Fatalf("confirmed clear was not projected: %v", load)
				}
			} else if !load.Valid || load.String != "-1" {
				t.Fatalf("unconfirmed clear replaced the local value: %v", load)
			}
		})
	}
}
