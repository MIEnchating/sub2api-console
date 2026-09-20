package routingwrite_test

import (
	"encoding/json"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestSharedAllocationWritesGrowthWithGenericSwitchesDisabled(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 1, 1, true)
	setReductionWritePolicy(t, fixture, true)
	target := reductionWriteTarget(t, fixture, "41", 5)
	target.UpstreamAllocation = true
	target.DesiredHealth = "healthy"
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed != 1 || result.Failed != 0 || fixture.states["41"]["concurrency"] != 5 {
		t.Fatalf("shared allocator must write within confirmed quota: %+v", result)
	}
}

func TestSharedAllocationRecoversOnlyConfirmedCapacityPause(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 1, 1, true)
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Schedulable: writeEnabledPointer(false), DesiredHealth: business.AccountStateConcurrencyLimited}}, "test")
	if err != nil || result.Changed != 1 {
		t.Fatalf("pause fixture: %+v %v", result, err)
	}
	setReductionWritePolicy(t, fixture, true)
	target := reductionWriteTarget(t, fixture, "41", 1)
	target.UpstreamAllocation = true
	target.DesiredHealth = "healthy"
	target.Schedulable = writeEnabledPointer(true)
	result, err = service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed != 1 || result.Failed != 0 || fixture.states["41"]["schedulable"] != true {
		t.Fatalf("confirmed pause must recover without generic switches: %+v", result)
	}
}

func TestSharedAllocationRejectsUnsafeOrStaleWrites(t *testing.T) {
	for _, scenario := range []string{"manual pause", "global full", "upstream full", "stale quota", "changed limit", "scope excluded", "disabled", "snapshot failure", "zero", "unconfirmed sibling reduction"} {
		t.Run(scenario, func(t *testing.T) {
			service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 1, 1, scenario != "manual pause")
			setReductionWritePolicy(t, fixture, true)
			target := reductionWriteTarget(t, fixture, "41", 5)
			target.UpstreamAllocation, target.DesiredHealth, target.Schedulable = true, "healthy", writeEnabledPointer(true)
			targets := map[string]business.AccountRoutingTarget{"41": target}
			switch scenario {
			case "global full":
				if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"scaling": map[string]any{"enabled": true, "global_max_concurrency": 2}}}, "test"); err != nil {
					t.Fatal(err)
				}
				disableCapacityFixtureGroupScaling(t, fixture)
			case "upstream full":
				fixture.states["42"]["concurrency"] = 9
			case "unconfirmed sibling reduction":
				fixture.states["42"]["concurrency"] = 9
				sibling := reductionWriteTarget(t, fixture, "42", 5)
				sibling.UpstreamAllocation, sibling.DesiredHealth = true, "healthy"
				targets["42"] = sibling
			case "stale quota":
				reductionSyncLimit(t, fixture, nil, "17")
			case "changed limit":
				reductionSyncLimit(t, fixture, writeCapacityPointer(8), "17")
			case "scope excluded":
				if _, err := fixture.store.SetAccountScopeControl(t.Context(), "41", "exclude", "test"); err != nil {
					t.Fatal(err)
				}
			case "disabled":
				setReductionWritePolicy(t, fixture, false)
			case "snapshot failure":
				fixture.failSnapshot = true
			case "zero":
				target.Concurrency = writeCapacityPointer(0)
				targets["41"] = target
			}
			result, err := service.Apply(t.Context(), targets, "test")
			if fixture.states["41"]["concurrency"] != 1 || fixture.states["41"]["schedulable"] != (scenario != "manual pause") {
				t.Fatalf("unsafe allocation changed account: %+v err=%v", result, err)
			}
			if scenario == "global full" || scenario == "upstream full" || scenario == "unconfirmed sibling reduction" {
				if err != nil || result.Failed != 0 {
					t.Fatalf("capacity wait must not become execution failure: %+v %v", result, err)
				}
			}
		})
	}
}

func TestNewWaitingAccountReclaimsAndSharesExistingAccountCapacity(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 1, 10, false)
	for _, state := range fixture.states {
		state["rate_multiplier"] = "0.1"
	}
	if _, err := fixture.store.SyncManagementSnapshot(t.Context(), fixture.accounts(), []map[string]any{{"id": json.Number("7"), "name": "capacity-group", "rate_multiplier": "1"}}, "test"); err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.CommitOnboardingProjection(t.Context(), business.OnboardingProjection{
		OperationID: "new-account", AccountID: "41", AccountName: "capacity-41", Platform: "gemini",
		UpstreamHost: "capacity.example", UpstreamType: "sub2api", UpstreamKeyID: "81", UpstreamGroupID: "8",
		LocalGroupID: "7", LocalGroupName: "capacity-group", Multiplier: "0.1", Concurrency: writeCapacityPointer(1),
		Schedulable: false, ReadbackConfirmed: true, WaitingForCapacity: true,
	}); err != nil {
		t.Fatal(err)
	}
	inventory, err := fixture.store.RoutingCapacityAccounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	foundWaiting := false
	for _, account := range inventory {
		if account.ID != "41" {
			continue
		}
		foundWaiting = true
		if account.EffectiveState != business.AccountStateConcurrencyLimited {
			t.Fatalf("new account must enter allocation as confirmed capacity wait: %+v", account)
		}
	}
	if !foundWaiting {
		t.Fatalf("new waiting account missing from capacity inventory: %+v", inventory)
	}
	setReductionWritePolicy(t, fixture, true)
	if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"scaling": map[string]any{"enabled": false, "global_max_concurrency": 10}}}, "test"); err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 3; round++ {
		plan, err := routing.NewService(fixture.store).Calculate(t.Context(), routing.Scope{}, true)
		if err != nil {
			t.Fatal(err)
		}

		applied, err := service.Apply(t.Context(), plan.AccountTargets, "test")

		if err != nil || applied.Failed != 0 {
			t.Fatalf("round %d failed: %+v %v", round, applied, err)
		}
		if round == 0 && fixture.states["41"]["schedulable"] != false {
			t.Fatal("new account recovered before the existing account reduction was confirmed")
		}
	}
	for _, id := range []string{"41", "42"} {
		if fixture.states[id]["schedulable"] != true || fixture.states[id]["concurrency"] != 5 {
			t.Fatalf("three confirmed rounds must converge to 5+5: %v", fixture.states)
		}
	}
}

func TestSharedAllocationUnlimitedUpstreamStillUsesGlobalBudgetForRecovery(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 1, 1, true)
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": {AccountID: "41", GroupNames: []string{"capacity-group"}, Schedulable: writeEnabledPointer(false), DesiredHealth: business.AccountStateConcurrencyLimited}}, "test")
	if err != nil || result.Changed != 1 {
		t.Fatalf("initial pause: %+v %v", result, err)
	}
	setReductionWritePolicy(t, fixture, true)
	reductionSyncLimit(t, fixture, writeCapacityPointer(0), "17")
	target := reductionWriteTarget(t, fixture, "41", 1)
	target.UpstreamAllocation, target.DesiredHealth, target.Schedulable = true, "healthy", writeEnabledPointer(true)
	result, err = service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
	if err != nil || result.Changed != 1 || result.Failed != 0 || fixture.states["41"]["schedulable"] != true {
		t.Fatalf("unlimited upstream should recover within confirmed global budget: %+v %v", result, err)
	}
}

func TestSharedAllocationWriteIgnoresDisabledGlobalLimit(t *testing.T) {
	service, fixture := newGlobalCapacityFixture(t, 1, 1000, true)
	setReductionWritePolicy(t, fixture, true)
	if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"scaling": map[string]any{"enabled": false, "global_max_concurrency": 2}}}, "test"); err != nil {
		t.Fatal(err)
	}
	target := reductionWriteTarget(t, fixture, "41", 10)
	target.UpstreamAllocation, target.DesiredHealth = true, "healthy"
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
	if err != nil || result.Failed != 0 || fixture.states["41"]["concurrency"] != 10 {
		t.Fatalf("independent upstream allocation must not enable global scaling limit: %+v %v", result, err)
	}
}

func TestSharedAllocationWriteUsesConfirmedCostStopButRechecksRemoteState(t *testing.T) {
	for _, scenario := range []string{"confirmed", "remote active", "excluded"} {
		t.Run(scenario, func(t *testing.T) {
			service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 1, 10, true)
			for id, state := range map[string]string{"41": business.AccountStateConcurrencyLimited, "42": "cost_blocked"} {
				result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{
					id: {AccountID: id, GroupNames: []string{"capacity-group"}, Schedulable: writeEnabledPointer(false), DesiredHealth: state},
				}, "test")
				if err != nil || result.Changed != 1 {
					t.Fatalf("confirmed stop: %+v %v", result, err)
				}
			}
			setReductionWritePolicy(t, fixture, true)
			if scenario == "remote active" {
				fixture.states["42"]["schedulable"] = true
			}
			if scenario == "excluded" {
				if _, err := fixture.store.SetAccountScopeControl(t.Context(), "42", "exclude", "test"); err != nil {
					t.Fatal(err)
				}
			}
			target := reductionWriteTarget(t, fixture, "41", 1)
			target.UpstreamAllocation, target.DesiredHealth, target.Schedulable = true, "healthy", writeEnabledPointer(true)
			result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
			if err != nil || result.Failed != 0 || fixture.states["41"]["schedulable"] != (scenario == "confirmed") {
				for _, account := range result.Results {
					if account.Error != nil {
						t.Log(*account.Error)
					}
				}
				t.Fatalf("recovery must use only confirmed in-scope cost-stop capacity: %+v %v", result, err)
			}
		})
	}
}

func TestSharedAllocationWriterRechecksSelectedAccountScope(t *testing.T) {
	for _, selected := range []string{"41", "42"} {
		t.Run(selected, func(t *testing.T) {
			service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 1, 1, true)
			setReductionWritePolicy(t, fixture, true)
			target := reductionWriteTarget(t, fixture, "41", 5)
			target.UpstreamAllocation = true
			target.DesiredHealth = "healthy"
			if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"upstream_concurrency": map[string]any{"enabled": true, "account_mode": "selected", "account_ids": []any{selected}}}}, "test"); err != nil {
				t.Fatal(err)
			}
			result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
			if selected == "41" {
				if err != nil || result.Changed != 1 || fixture.states["41"]["concurrency"] != 5 {
					t.Fatalf("selected allocation was not applied: %+v %v", result, err)
				}
			} else if fixture.states["41"]["concurrency"] != 1 || result.Changed != 0 {
				t.Fatalf("removed account changed during execution: %+v %v", result, err)
			}
		})
	}
}

func TestSharedAllocationWriterRechecksSelectedUpstreamScope(t *testing.T) {
	for _, selected := range []bool{true, false} {
		t.Run(map[bool]string{true: "selected", false: "removed"}[selected], func(t *testing.T) {
			service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 1, 1, true)
			setReductionWritePolicy(t, fixture, true)
			target := reductionWriteTarget(t, fixture, "41", 5)
			target.UpstreamAllocation, target.DesiredHealth = true, "healthy"
			upstreamID := target.UpstreamReductionID
			if !selected {
				upstreamID = "other-upstream"
			}
			if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"upstream_concurrency": map[string]any{"enabled": true, "account_mode": "upstreams", "upstream_ids": []any{upstreamID}}}}, "test"); err != nil {
				t.Fatal(err)
			}
			result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
			if selected {
				if err != nil || result.Changed != 1 || fixture.states["41"]["concurrency"] != 5 {
					t.Fatalf("selected upstream not applied: %+v %v", result, err)
				}
			} else if result.Changed != 0 || fixture.states["41"]["concurrency"] != 1 {
				t.Fatalf("removed upstream changed: %+v %v", result, err)
			}
		})
	}
}
