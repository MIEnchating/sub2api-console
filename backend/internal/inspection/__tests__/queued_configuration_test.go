package inspection_test

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
	"github.com/MIEnchating/sub2api-console/backend/internal/inspection"
	"github.com/MIEnchating/sub2api-console/backend/internal/management"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestQueuedInspectionRecoveryHonorsCurrentAutomaticSwitch(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		automatic bool
		disabled  bool
		invalid   bool
	}{
		{name: "automatic disabled while queued stops before catalog read", automatic: true, disabled: true},
		{name: "automatic still enabled synchronizes current catalog", automatic: true},
		{name: "manual recovery remains available with automation disabled", disabled: true},
		{name: "unreadable current config stops recovery and preserves prior results", automatic: true, invalid: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			businessPath := filepath.Join(t.TempDir(), "business.db")
			store, err := business.Open(businessPath)
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
			config := business.DefaultAutoInspectionConfig()
			config.Enabled = true
			if _, err := store.UpdateAutoInspectionConfig(t.Context(), config); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"upstream-sync", "traffic"} {
				if err := store.MarkInspectionTask(t.Context(), name, time.Now().UTC()); err != nil {
					t.Fatal(err)
				}
			}
			tasks, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = tasks.Close() })
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != http.MethodGet || (r.URL.Path != "/api/v1/admin/groups" && r.URL.Path != "/api/v1/admin/accounts") {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"items": []any{}, "total": 0}})
			}))
			t.Cleanup(server.Close)
			target := managementTarget{collectionTarget{server.URL}}
			runner := inspection.NewRunner(store, target, evidence.New(store, nil), routing.NewService(store), nil, nil, nil, tasks, management.New(target, store, tasks))
			scheduler, err := inspection.NewScheduler(store, runner)
			if err != nil {
				t.Fatal(err)
			}
			service := inspection.NewManualService(scheduler, runner, tasks)
			pool := &recoveryRunner{}
			service.UseTaskRunner(pool)
			task, err := runner.QueueTask(t.Context(), scenario.automatic)
			if err != nil {
				t.Fatal(err)
			}
			task.Result["completed_note"] = "previous result"
			if err := tasks.Save(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			if err := service.ResumeTask(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			if scenario.disabled {
				config.Enabled = false
				if _, err := scheduler.UpdateConfig(t.Context(), config); err != nil {
					t.Fatal(err)
				}
			}
			if scenario.invalid {
				db, err := sql.Open("sqlite", businessPath)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = db.Close() })
				if _, err := db.Exec(`UPDATE policy_nodes SET node_type='string',scalar_value='invalid' WHERE policy_key='auto-inspection' AND key_name='enabled'`); err != nil {
					t.Fatal(err)
				}
			}
			pool.run(t.Context())
			got, err := tasks.Get(t.Context(), task.ID)
			if err != nil {
				t.Fatal(err)
			}
			if scenario.automatic && (scenario.disabled || scenario.invalid) {
				message := "自动巡检已关闭"
				if scenario.invalid {
					message = "enabled 必须是布尔值"
				}
				if requests.Load() != 0 || got.Status != "failed" || !strings.Contains(got.Message, message) || got.Result["completed_note"] != "previous result" {
					t.Fatalf("rejected recovery: requests=%d task=%+v", requests.Load(), got)
				}
				if _, err := tasks.RecoveryTask(t.Context(), task.ID); !errors.Is(err, taskstore.ErrNotFound) {
					t.Fatalf("disabled recovery remained restartable: %v", err)
				}
				return
			}
			if got.Status != "succeeded" {
				t.Fatalf("allowed recovery did not succeed: %+v", got)
			}
			if scenario.automatic && requests.Load() == 0 {
				t.Fatalf("enabled recovery did not synchronize the current catalog")
			}
		})
	}
}
