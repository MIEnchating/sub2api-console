package routingwrite_test

import (
	"encoding/json"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestRecoveryConfirmsConcurrencyBeforeEnablingTraffic(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		count  int
		ignore bool
	}{
		{name: "single ignored concurrency stays paused", count: 1, ignore: true},
		{name: "batch ignored concurrency stays paused", count: 2, ignore: true},
		{name: "single confirmed concurrency enables traffic", count: 1},
		{name: "batch confirmed concurrency enables traffic", count: 2},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(40), 20, 0, false)
			fixture.states["42"]["schedulable"] = false
			fixture.states["42"]["concurrency"] = 20
			if _, err := fixture.store.SyncManagementSnapshot(t.Context(), fixture.accounts(), []map[string]any{{"id": json.Number("7"), "name": "capacity-group"}}, "test"); err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{
				"advanced_policy": map[string]any{"writeback": map[string]any{"verification": false}},
			}, "test"); err != nil {
				t.Fatal(err)
			}
			fixture.ignoreParameters = scenario.ignore
			fixture.confirmedParameters = map[string]bool{}
			targets := map[string]business.AccountRoutingTarget{}
			for _, id := range []string{"41", "42"}[:scenario.count] {
				enabled, concurrency := true, int64(5)
				targets[id] = business.AccountRoutingTarget{AccountID: id, GroupNames: []string{"capacity-group"}, Schedulable: &enabled, Concurrency: &concurrency}
			}
			result, err := service.Apply(t.Context(), targets, "test")
			if err != nil {
				t.Fatal(err)
			}
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			for id := range targets {
				if actual := fixture.states[id]["schedulable"]; actual != !scenario.ignore {
					t.Fatalf("account %s enabled with unconfirmed concurrency: state=%v result=%+v", id, fixture.states[id], result)
				}
			}
			if !scenario.ignore && fixture.enableBeforeConfirm {
				t.Fatal("traffic was enabled before reading back the written concurrency")
			}
			if scenario.ignore && result.Failed != scenario.count {
				t.Fatalf("ignored concurrency write must report failure: %+v", result)
			}
		})
	}
}
