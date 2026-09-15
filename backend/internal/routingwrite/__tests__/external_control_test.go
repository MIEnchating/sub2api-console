package routingwrite_test

import (
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestWriteRechecksExternalControlAgainstFreshRemoteValues(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		manageAll bool
		field     string
		value     any
		ownField  bool
		cleanup   bool
		released  bool
	}{
		{name: "owned concurrency changed outside Console", field: "concurrency", value: 3, released: true},
		{name: "owned scheduling changed outside Console", field: "schedulable", value: false, ownField: true, released: true},
		{name: "owned priority changed outside Console", field: "priority", value: 5, ownField: true, released: true},
		{name: "owned load factor changed outside Console", field: "load_factor", value: 3, ownField: true, released: true},
		{name: "owned field changed before destructive cleanup", field: "concurrency", value: 3, cleanup: true, released: true},
		{name: "unowned priority changed outside Console", field: "priority", value: 5},
		{name: "owned concurrency unchanged", field: "concurrency", value: 6},
		{name: "manage all permits restoring owned concurrency", manageAll: true, field: "concurrency", value: 3},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(20), 4, 4, true)
			if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{
				"advanced_policy": map[string]any{"scope": map[string]any{"manage_all_accounts": scenario.manageAll}},
			}, "test"); err != nil {
				t.Fatal(err)
			}
			concurrency := int64(6)
			target := business.AccountRoutingTarget{AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: &concurrency}
			if scenario.ownField {
				switch scenario.field {
				case "schedulable":
					target.Schedulable = writeEnabledPointer(true)
				case "priority":
					target.Priority = writeCapacityPointer(1000)
				case "load_factor":
					load := "10"
					target.LoadFactor = &load
				}
			}
			targets := map[string]business.AccountRoutingTarget{
				"41": target,
			}
			initial, err := service.Apply(t.Context(), targets, "test")
			if err != nil || initial.Failed != 0 || initial.Changed != 1 {
				t.Fatalf("initial ownership failed: result=%+v err=%v", initial, err)
			}
			fixture.mu.Lock()
			fixture.states["41"][scenario.field] = scenario.value
			fixture.mu.Unlock()
			concurrency = 5
			if scenario.cleanup {
				cleanup := "delete"
				target.CleanupAction = &cleanup
				targets["41"] = target
			}
			result, err := service.Apply(t.Context(), targets, "test")
			if err != nil || result.Failed != 0 || len(result.Results) != 1 {
				t.Fatalf("external-control recheck failed: result=%+v err=%v", result, err)
			}
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if scenario.released {
				if !result.Results[0].Released || result.RemoteWrite || fixture.states["41"][scenario.field] != scenario.value {
					t.Fatalf("fresh external value was overwritten: state=%v result=%+v", fixture.states["41"], result)
				}
			} else if result.Results[0].Released || fixture.states["41"]["concurrency"] != 5 {
				t.Fatalf("valid automatic change was blocked: state=%v result=%+v", fixture.states["41"], result)
			}
		})
	}
}

func TestReleasedExternalControlCannotBeReacquiredUnlessManageAllIsEnabled(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		manageAll bool
		release   bool
	}{
		{name: "released ownership is retained"},
		{name: "manage all explicitly permits reacquiring ownership", manageAll: true},
		{name: "old automatic release cannot discard the external control marker", release: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(20), 4, 4, true)
			setManageAll := func(enabled bool) {
				t.Helper()
				if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{
					"advanced_policy": map[string]any{"scope": map[string]any{"manage_all_accounts": enabled}},
				}, "test"); err != nil {
					t.Fatal(err)
				}
			}
			setManageAll(false)
			concurrency := int64(6)
			targets := map[string]business.AccountRoutingTarget{
				"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: &concurrency},
			}
			initial, err := service.Apply(t.Context(), targets, "test")
			if err != nil || initial.Failed != 0 || initial.Changed != 1 {
				t.Fatalf("initial ownership failed: result=%+v err=%v", initial, err)
			}
			fixture.mu.Lock()
			fixture.states["41"]["concurrency"] = 3
			fixture.mu.Unlock()
			released, err := service.Apply(t.Context(), targets, "test")
			if err != nil || released.Released != 1 {
				t.Fatalf("external change was not released: result=%+v err=%v", released, err)
			}
			if scenario.manageAll {
				setManageAll(true)
			}
			if scenario.release {
				target := targets["41"]
				target.ReleaseControl = true
				targets["41"] = target
			}
			repeated, err := service.Apply(t.Context(), targets, "test")
			if err != nil || repeated.Failed != 0 {
				t.Fatalf("repeated old target failed: result=%+v err=%v", repeated, err)
			}
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if scenario.manageAll {
				if fixture.states["41"]["concurrency"] != 6 || repeated.Changed != 1 {
					t.Fatalf("explicit manage-all did not regain control: state=%v result=%+v", fixture.states["41"], repeated)
				}
			} else if fixture.states["41"]["concurrency"] != 3 || repeated.RemoteWrite || len(repeated.Results) != 1 || !repeated.Results[0].Skipped {
				t.Fatalf("released account was reacquired by an old target: state=%v result=%+v", fixture.states["41"], repeated)
			}
		})
	}
}
