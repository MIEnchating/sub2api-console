package routingwrite_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

func TestAutomaticWriteRechecksAuthorizationAfterReadingRemoteAccount(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		monitoring bool
		cleanup    bool
	}{
		{name: "monitoring selected during account read", monitoring: true},
		{name: "concurrency write disabled during account read"},
		{name: "monitoring selected before destructive cleanup", monitoring: true, cleanup: true},
		{name: "policy changed before destructive cleanup", cleanup: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			_, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(20), 4, 4, true)
			var once sync.Once
			var mutations atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodGet {
					mutations.Add(1)
				}
				var updateErr error
				if request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/41" {
					once.Do(func() {
						if scenario.monitoring {
							_, updateErr = fixture.store.SetMode(request.Context(), runtimepolicy.Monitoring)
						} else {
							_, updateErr = fixture.store.UpdatePolicy(request.Context(), map[string]any{"auto_apply": map[string]any{"concurrency": false}}, "operator")
						}
					})
				}
				if updateErr != nil {
					t.Errorf("operator configuration update failed: %v", updateErr)
					http.Error(w, "update failed", http.StatusInternalServerError)
					return
				}
				fixture.ServeHTTP(w, request)
			}))
			t.Cleanup(server.Close)
			concurrency := int64(6)
			target := business.AccountRoutingTarget{AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: &concurrency}
			if scenario.cleanup {
				cleanup := "delete"
				target.CleanupAction = &cleanup
			}
			targets := map[string]business.AccountRoutingTarget{"41": target}
			if !scenario.cleanup {
				second := target
				second.AccountID = "42"
				targets["42"] = second
			}
			result, err := routingwrite.New(routingTarget{url: server.URL}, fixture.store).Apply(t.Context(), targets, "scheduler")
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if mutations.Load() != 0 || result.RemoteWrite || result.Changed != 0 || fixture.states["41"]["concurrency"] != 4 || fixture.states["41"]["schedulable"] != true {
				t.Fatalf("stale authorization still sent a remote mutation: state=%v result=%+v err=%v", fixture.states["41"], result, err)
			}
			if fixture.states["42"]["concurrency"] != 4 || fixture.states["42"]["schedulable"] != true {
				t.Fatalf("the other pending account bypassed the batch authorization check: state=%v", fixture.states["42"])
			}
			if err != nil || result.Failed != 0 || len(result.Results) != len(targets) {
				t.Fatalf("authorization changes must skip pending targets without failing: %+v err=%v", result, err)
			}
			for _, item := range result.Results {
				if !item.Skipped || item.Error != nil || item.Reason == nil {
					t.Fatalf("missing explicit authorization deferral: %+v", item)
				}
			}
			audit, err := fixture.store.AuditEvents(t.Context(), nil, false)
			if err != nil || len(audit) != len(targets) {
				t.Fatalf("missing authorization deferral audits: %+v err=%v", audit, err)
			}
			for _, record := range audit {
				if record.State != "skipped" || record.Writeback || record.Error != nil || record.ReadbackConfirmed == nil || *record.ReadbackConfirmed || record.RemoteConfirmed == nil || *record.RemoteConfirmed {
					t.Fatalf("authorization deferral must not claim failure or confirmation: %+v", record)
				}
			}
			if _, err := fixture.store.EvaluateAlertIncidents(t.Context()); err != nil {
				t.Fatal(err)
			}
			queue, err := fixture.store.NotificationQueueDetails(t.Context(), "test-channel", true)
			if err != nil || len(queue.ProducerFiring) != 0 || len(queue.ConsumerItems) != 0 {
				t.Fatalf("authorization deferral generated notifications: %+v err=%v", queue, err)
			}
		})
	}
}
