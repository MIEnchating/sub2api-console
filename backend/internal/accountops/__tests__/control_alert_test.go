package accountops_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
	"github.com/MIEnchating/sub2api-console/backend/internal/alerting"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
)

func TestConfirmedManualFuseQueuesAlertWithoutWaitingForInspection(t *testing.T) {
	for _, scenario := range []struct {
		name, action           string
		writeFails, queueFails bool
	}{
		{name: "confirmed fuse queues alert", action: "fuse"},
		{name: "confirmed recovery queues alert", action: "recover"},
		{name: "failed write does not queue alert", action: "fuse", writeFails: true},
		{name: "queue failure keeps confirmed control successful", action: "fuse", queueFails: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "control.db")
			store, err := business.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			if err := store.Bootstrap(t.Context()); err != nil {
				t.Fatal(err)
			}
			if _, err := store.UpdatePolicy(t.Context(), map[string]any{"mode": runtimepolicy.Full}, "test"); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			if _, err := db.Exec(`INSERT INTO accounts(id,name,schedulable,updated_at) VALUES('998','manual-account',1,'now')`); err != nil {
				t.Fatal(err)
			}
			schedulable := true
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					if scenario.writeFails {
						http.Error(w, "write rejected", http.StatusForbidden)
						return
					}
					schedulable = scenario.action == "recover"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 998, "name": "manual-account", "schedulable": schedulable}})
			}))
			t.Cleanup(server.Close)
			alertTasks := &settingsTasks{}
			var runner taskrunner.Runner = &settingsRunner{}
			if scenario.queueFails {
				runner = rejectedControlAlertRunner{}
			}
			alerts := alerting.NewTaskService(nil, alertTasks)
			alerts.UseTaskRunner(runner)
			service := accountops.New(settingsTarget{endpoint: server.URL}, store, &settingsTasks{})
			service.UseControlAlerts(alerts.Enqueue)
			result, err := service.Control(t.Context(), "998", scenario.action, "test")
			if scenario.writeFails {
				if err == nil || alertTasks.last.ID != "" {
					t.Fatalf("unconfirmed fuse queued an alert: task=%+v err=%v", alertTasks.last, err)
				}
				return
			}
			if scenario.queueFails {
				if err != nil || result["readback_confirmed"] != true || len(result["cleanup_warnings"].([]string)) != 1 {
					t.Fatalf("notification failure obscured a confirmed control: result=%+v err=%v", result, err)
				}
				return
			}
			if err != nil || alertTasks.last.Status != "queued" || alertTasks.last.Skill != "sub2api-alert-evaluation" {
				t.Fatalf("confirmed fuse did not queue an independent alert task: task=%+v err=%v", alertTasks.last, err)
			}
		})
	}
}

type rejectedControlAlertRunner struct{}

func (rejectedControlAlertRunner) Go(func(context.Context)) error { return errors.New("queue full") }
