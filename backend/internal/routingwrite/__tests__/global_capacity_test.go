package routingwrite_test

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
)

func newGlobalCapacityFixture(t *testing.T, current, other int, active bool) (*routingwrite.Service, *upstreamCapacityFixture) {
	t.Helper()
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(100), current, other, active)
	if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{
		"scaling": map[string]any{"enabled": true, "global_max_concurrency": 10},
	}}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{
		Host: "other-capacity.example", BaseURL: "https://other-capacity.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{
		Host: "other-capacity.example", Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: writeCapacityPointer(100), ProfileUserID: "18"},
	}); err != nil {
		t.Fatal(err)
	}
	fixture.states["42"]["upstream_host"] = "other-capacity.example"
	if _, err := fixture.store.SyncManagementSnapshot(t.Context(), fixture.accounts(), []map[string]any{{"id": json.Number("7"), "name": "capacity-group"}}, "test"); err != nil {
		t.Fatal(err)
	}
	return service, fixture
}

func TestGlobalCapacityGrowthWaitsWhenAnotherUpstreamConsumesCalculatedHeadroom(t *testing.T) {
	service, fixture := newGlobalCapacityFixture(t, 4, 4, true)
	fixture.snapshotConcurrency = map[string]int{"42": 6}
	desired := int64(6)
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: &desired},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed != 0 || result.Failed != 0 || len(result.Results) != 1 || !result.Results[0].Skipped || result.Results[0].Reason == nil || !strings.Contains(*result.Results[0].Reason, "全局") {
		t.Fatalf("cross-upstream capacity shortage must wait without issuing a write or failure: %+v", result)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.states["41"]["concurrency"] != 4 {
		t.Fatal("stale growth exceeded the global budget after another upstream consumed its headroom")
	}
}

func TestGlobalCapacityBatchAndSeparateTasksCannotReuseCrossUpstreamHeadroom(t *testing.T) {
	for _, separateTasks := range []bool{false, true} {
		name := "one batch"
		if separateTasks {
			name = "separate tasks"
		}
		t.Run(name, func(t *testing.T) {
			service, fixture := newGlobalCapacityFixture(t, 4, 4, true)
			desired := int64(6)
			targets := map[string]business.AccountRoutingTarget{
				"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: &desired},
				"42": {AccountID: "42", GroupNames: []string{"capacity-group"}, Concurrency: &desired},
			}
			if separateTasks {
				start := make(chan struct{})
				var workers sync.WaitGroup
				for id, target := range targets {
					workers.Go(func() {
						<-start
						if _, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{id: target}, "test"); err != nil {
							t.Error(err)
						}
					})
				}
				close(start)
				workers.Wait()
			} else if _, err := service.Apply(t.Context(), targets, "test"); err != nil {
				t.Fatal(err)
			}
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			total := fixture.states["41"]["concurrency"].(int) + fixture.states["42"]["concurrency"].(int)
			if total != 10 {
				t.Fatalf("cross-upstream writers must share the two remaining global slots: %v", fixture.states)
			}
		})
	}
}

func TestGlobalCapacityGrowthRejectsUnconfirmedOutsideInventory(t *testing.T) {
	for _, scenario := range []string{"missing account", "new remote account", "missing concurrency", "unlimited concurrency", "snapshot failure"} {
		t.Run(scenario, func(t *testing.T) {
			service, fixture := newGlobalCapacityFixture(t, 4, 4, true)
			switch scenario {
			case "missing account":
				fixture.omitSibling = true
			case "new remote account":
				fixture.states["43"] = map[string]any{"id": "43", "schedulable": true, "concurrency": 3}
			case "missing concurrency":
				delete(fixture.states["42"], "concurrency")
			case "unlimited concurrency":
				fixture.states["42"]["concurrency"] = 0
			case "snapshot failure":
				fixture.failSnapshot = true
			}
			desired := int64(6)
			result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
				"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: &desired},
			}, "test")
			if err != nil {
				t.Fatal(err)
			}
			if result.Changed != 0 || result.Failed != 1 {
				t.Fatalf("an unconfirmed global snapshot must prevent growth and preserve a real read failure: %+v", result)
			}
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if fixture.states["41"]["concurrency"] != 4 {
				t.Fatal("growth ignored unconfirmed outside capacity")
			}
		})
	}
}

func TestGlobalCapacityKeepsNonCapacityPausesReservedAndRechecksRecovery(t *testing.T) {
	for _, recovery := range []bool{false, true} {
		name := "outside manual pause keeps its reservation"
		if recovery {
			name = "own reservation cannot restore traffic above global limit"
		}
		t.Run(name, func(t *testing.T) {
			service, fixture := newGlobalCapacityFixture(t, 4, 8, !recovery)
			target := business.AccountRoutingTarget{AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: writeCapacityPointer(6)}
			if recovery {
				target.Concurrency, target.Schedulable = nil, writeEnabledPointer(true)
			} else {
				fixture.states["42"]["schedulable"] = false
			}
			result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
			if err != nil {
				t.Fatal(err)
			}
			if result.Changed != 0 || result.Failed != 0 || len(result.Results) != 1 || !result.Results[0].Skipped {
				t.Fatalf("reserved non-capacity pauses must still constrain global growth and recovery: %+v", result)
			}
		})
	}
}

func TestGlobalCapacityDoesNotSpendUnconfirmedCrossUpstreamReduction(t *testing.T) {
	service, fixture := newGlobalCapacityFixture(t, 6, 4, true)
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: writeCapacityPointer(4)},
		"42": {AccountID: "42", GroupNames: []string{"capacity-group"}, Concurrency: writeCapacityPointer(6)},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if result.Changed != 1 || fixture.states["41"]["concurrency"] != 4 || fixture.states["42"]["concurrency"] != 4 {
		t.Fatalf("an unconfirmed reduction must not finance another upstream's growth: states=%v result=%+v", fixture.states, result)
	}
}

func TestGlobalCapacityEnabledThroughGroupOverrideStillGuardsSharedBudget(t *testing.T) {
	service, fixture := newGlobalCapacityFixture(t, 4, 6, true)
	if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{
		"scaling": map[string]any{"enabled": false, "global_max_concurrency": 10},
	}}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.UpdateGroupPolicy(t.Context(), "7", map[string]any{
		"enabled": true, "strategy": "balanced", "min_pool_size": 1, "weight_budget": 400,
		"balanced_price_ratio": 0.5, "breaker_enabled": true, "recovery_enabled": true,
		"weights_enabled": true, "scaling_enabled": true, "probe_enabled": true,
		"probe_interval_seconds": 300, "probe_model": "gpt-5.2",
	}, "test"); err != nil {
		t.Fatal(err)
	}
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: writeCapacityPointer(6)},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed != 0 || result.Failed != 0 || len(result.Results) != 1 || !result.Results[0].Skipped {
		t.Fatalf("group-enabled scaling must honor the global budget even when the global scaling toggle is off: %+v", result)
	}
}

type globalCapacityPolicyStore struct {
	*business.Store
	policy map[string]any
}

func (store *globalCapacityPolicyStore) ControlPolicy(context.Context) (map[string]any, error) {
	return store.policy, nil
}

func TestGlobalCapacityGroupOverrideUsesDefaultBudgetWhenScalingSectionIsAbsent(t *testing.T) {
	_, fixture := newGlobalCapacityFixture(t, 400, 500, true)
	policy, err := fixture.store.ControlPolicy(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	delete(policy, "scaling")
	policy["group_policy_bindings"] = map[string]any{"7": map[string]any{"scaling_enabled": true}}
	service := routingwrite.New(routingTarget{url: fixture.serverURL}, &globalCapacityPolicyStore{Store: fixture.store, policy: policy})
	// Remove the unrelated upstream cap so the default global limit is decisive.
	for _, host := range []string{"capacity.example", "other-capacity.example"} {
		if _, err := fixture.store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{
			Host: host, Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: writeCapacityPointer(0), ProfileUserID: "17"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: writeCapacityPointer(401)},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed != 0 || result.Failed != 0 || len(result.Results) != 1 || !result.Results[0].Skipped {
		t.Fatalf("a group-enabled policy without a scaling section must still enforce the default global limit: %+v", result)
	}
}

func TestUpstreamCapacityReservesManualPauseEvenWhenGlobalBudgetHasRoom(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 2, 8, true)
	fixture.states["42"]["schedulable"] = false
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: writeCapacityPointer(6)},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed != 0 || result.Failed != 0 || len(result.Results) != 1 || !result.Results[0].Skipped {
		t.Fatalf("manual pause must keep eight upstream slots reserved even without a restrictive global budget: %+v", result)
	}
}

func TestGlobalCapacityConfirmedCapacityPauseReleasesItsReservation(t *testing.T) {
	service, fixture := newGlobalCapacityFixture(t, 4, 80, true)
	_, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"42": {AccountID: "42", GroupNames: []string{"capacity-group"}, Schedulable: writeEnabledPointer(false), DesiredHealth: business.AccountStateConcurrencyLimited},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: writeCapacityPointer(6)},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed != 1 || result.Failed != 0 {
		t.Fatalf("a confirmed capacity pause must release its global reservation: %+v", result)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if fixture.states["41"]["concurrency"] != 6 || fixture.states["42"]["schedulable"] != false || fixture.states["42"]["concurrency"] != 80 {
		t.Fatalf("capacity release must preserve the paused account's configuration: %v", fixture.states)
	}
}

func TestGlobalCapacityOrdinaryReductionAndPauseDoNotRequireSuccessfulSnapshot(t *testing.T) {
	for _, pause := range []bool{false, true} {
		name := "concurrency reduction"
		if pause {
			name = "scheduling pause"
		}
		t.Run(name, func(t *testing.T) {
			service, fixture := newGlobalCapacityFixture(t, 8, 8, true)
			fixture.failSnapshot = true
			target := business.AccountRoutingTarget{AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: writeCapacityPointer(4)}
			if pause {
				target.Concurrency, target.Schedulable = nil, writeEnabledPointer(false)
			}
			result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
			if err != nil {
				t.Fatal(err)
			}
			if result.Changed != 1 || result.Failed != 0 {
				t.Fatalf("reducing configured or active capacity must remain available without a full snapshot: %+v", result)
			}
		})
	}
}

func TestGlobalCapacityDoesNotConstrainIndependentUpstreamReduction(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 8, 8, true)
	setReductionWritePolicy(t, fixture, true)
	if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{
		"scaling": map[string]any{"enabled": true, "global_max_concurrency": 1},
	}}, "test"); err != nil {
		t.Fatal(err)
	}
	fixture.states["43"] = map[string]any{"id": "43", "schedulable": true, "concurrency": 0}
	target := reductionWriteTarget(t, fixture, "41", 2)
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed != 1 || result.Failed != 0 {
		t.Fatalf("independent reduction must ignore global growth limits and unrelated unknown capacity: %+v", result)
	}
}

func TestGlobalCapacityAutomaticControlReleaseSharesBudgetWithConcurrentGrowth(t *testing.T) {
	service, fixture := newGlobalCapacityFixture(t, 8, 4, true)
	_, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Concurrency: writeCapacityPointer(4)},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, ReleaseControl: true},
		"42": {AccountID: "42", GroupNames: []string{"capacity-group"}, Concurrency: writeCapacityPointer(6)},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	if result.Restored != 0 || result.Changed != 1 || result.Failed != 0 || fixture.states["41"]["concurrency"] != 4 || fixture.states["42"]["concurrency"] != 6 {
		t.Fatalf("automatic baseline restoration must not bypass the shared budget or consume rejected headroom: states=%v result=%+v", fixture.states, result)
	}
}
