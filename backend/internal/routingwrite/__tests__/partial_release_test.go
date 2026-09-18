package routingwrite_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
)

func TestPartialAutomaticReleaseRetainsBaselineForNextRound(t *testing.T) {
	_, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(20), 4, 4, true)
	var failScheduling atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if failScheduling.Load() && request.Method == http.MethodPost && request.URL.Path == "/api/v1/admin/accounts/41/schedulable" {
			http.Error(w, "scheduling update rejected", http.StatusConflict)
			return
		}
		fixture.ServeHTTP(w, request)
	}))
	t.Cleanup(server.Close)
	service := routingwrite.New(routingTarget{url: server.URL}, fixture.store)
	initial, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: writeCapacityPointer(6), Schedulable: writeEnabledPointer(false), DesiredHealth: "fused"},
	}, "scheduler")
	if err != nil || initial.Failed != 0 || initial.Changed != 1 {
		t.Fatalf("initial ownership failed: %+v err=%v", initial, err)
	}
	targets := map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, ReleaseControl: true, DesiredHealth: "excluded"},
	}
	failScheduling.Store(true)
	restored, err := service.Apply(t.Context(), targets, "scheduler")
	if err != nil || restored.Failed != 1 {
		t.Fatalf("expected partial restoration failure: %+v err=%v", restored, err)
	}
	baselines, err := fixture.store.RoutingBaselines(t.Context())
	if err != nil || len(baselines) != 1 {
		t.Fatalf("partial release must retain the original baseline: %+v err=%v", baselines, err)
	}
	failScheduling.Store(false)
	restored, err = service.Apply(t.Context(), targets, "scheduler")
	if err != nil || restored.Failed != 0 || len(restored.Results) != 1 || !restored.Results[0].Restored {
		t.Fatalf("the next round must finish restoration: %+v err=%v", restored, err)
	}
	baselines, err = fixture.store.RoutingBaselines(t.Context())
	if err != nil || len(baselines) != 0 {
		t.Fatalf("completed restoration must remove its baseline: %+v err=%v", baselines, err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.states["41"]["concurrency"] != 4 || fixture.states["41"]["schedulable"] != true {
		t.Fatalf("original routing configuration was not restored: %v", fixture.states["41"])
	}
}
