package routingwrite_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"testing"
)

func TestOutsideAllocationScopeRecoveryIgnoresUpstreamQuotaButKeepsAccountConcurrency(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(1), 31, 100, true)
	paused := business.AccountRoutingTarget{AccountID: "41", GroupNames: []string{"capacity-group"}, Schedulable: writeEnabledPointer(false), DesiredHealth: business.AccountStateConcurrencyLimited}
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": paused}, "test")
	if err != nil || result.Changed != 1 {
		t.Fatalf("confirmed pause: %+v %v", result, err)
	}
	_, err = fixture.store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"scaling": map[string]any{"enabled": false}, "upstream_concurrency": map[string]any{"enabled": true, "account_mode": "upstreams", "upstream_ids": []any{"other-upstream"}}}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	paused.Schedulable, paused.DesiredHealth = writeEnabledPointer(true), "healthy"
	result, err = service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": paused}, "test")
	if err != nil || result.Changed != 1 || result.Failed != 0 || fixture.states["41"]["schedulable"] != true || fixture.states["41"]["concurrency"] != 31 {
		t.Fatalf("scope exit recovery: %+v %v state=%+v", result, err, fixture.states["41"])
	}
}

func TestConfirmedAutomaticFuseFreesCapacityForSharedRecovery(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 1, 10, true)
	for id, state := range map[string]string{"41": business.AccountStateConcurrencyLimited, "42": "fused"} {
		result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{id: {AccountID: id, GroupNames: []string{"capacity-group"}, Schedulable: writeEnabledPointer(false), DesiredHealth: state}}, "test")
		if err != nil || result.Changed != 1 {
			t.Fatalf("confirm stop: %+v %v", result, err)
		}
	}
	setReductionWritePolicy(t, fixture, true)
	target := reductionWriteTarget(t, fixture, "41", 1)
	target.UpstreamAllocation, target.DesiredHealth, target.Schedulable = true, "healthy", writeEnabledPointer(true)
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
	if err != nil || result.Failed != 0 || fixture.states["41"]["schedulable"] != true {
		t.Fatalf("confirmed fuse must release shared capacity: %+v %v", result, err)
	}
	if fixture.states["42"]["schedulable"] != false {
		t.Fatal("capacity reuse must not recover fused account")
	}
}

func TestConfirmedAutomaticFuseFreesCapacityWithOnlyScalingEnabled(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 1, 10, true)
	for id, state := range map[string]string{"41": business.AccountStateConcurrencyLimited, "42": "fused"} {
		result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{id: {AccountID: id, GroupNames: []string{"capacity-group"}, Schedulable: writeEnabledPointer(false), DesiredHealth: state}}, "test")
		if err != nil || result.Changed != 1 {
			t.Fatalf("confirm stop: %+v %v", result, err)
		}
	}
	if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"scaling": map[string]any{"enabled": true}, "upstream_concurrency": map[string]any{"enabled": false}}}, "test"); err != nil {
		t.Fatal(err)
	}
	target := business.AccountRoutingTarget{AccountID: "41", GroupNames: []string{"capacity-group"}, DesiredHealth: "healthy", Schedulable: writeEnabledPointer(true)}
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
	if err != nil || result.Failed != 0 || fixture.states["41"]["schedulable"] != true {
		t.Fatalf("confirmed fuse must release shared capacity: %+v %v", result, err)
	}
	if fixture.states["42"]["schedulable"] != false {
		t.Fatal("capacity reuse must not recover fused account")
	}
}

func TestGroupScalingOffAndSharedOffRecoversWithoutOtherGroupsGlobalBudget(t *testing.T) {
	service, fixture := newGlobalCapacityFixture(t, 31, 100, true)
	target := business.AccountRoutingTarget{AccountID: "41", GroupNames: []string{"capacity-group"}, Schedulable: writeEnabledPointer(false), DesiredHealth: business.AccountStateConcurrencyLimited}
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
	if err != nil || result.Changed != 1 {
		t.Fatalf("pause: %+v %v", result, err)
	}
	if _, err := fixture.store.UpdateGroupPolicy(t.Context(), "7", map[string]any{"enabled": true, "strategy": "balanced", "min_pool_size": 1, "weight_budget": 400, "balanced_price_ratio": 0.5, "breaker_enabled": true, "recovery_enabled": true, "weights_enabled": true, "scaling_enabled": false, "probe_enabled": true, "probe_interval_seconds": 300, "probe_model": "gpt-5.2"}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"upstream_concurrency": map[string]any{"enabled": false}}}, "test"); err != nil {
		t.Fatal(err)
	}
	target.Schedulable, target.DesiredHealth = writeEnabledPointer(true), "healthy"
	result, err = service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
	if err != nil || result.Failed != 0 || fixture.states["41"]["schedulable"] != true || fixture.states["41"]["concurrency"] != 31 {
		t.Fatalf("disabled group inherited global gate: %+v %v", result, err)
	}
}

func TestIndependentAllocationTargetCannotBypassEnabledScalingSwitches(t *testing.T) {
	service, fixture := newUpstreamCapacityWriteFixture(t, writeCapacityPointer(10), 4, 4, true)
	setReductionWritePolicy(t, fixture, true)
	if _, err := fixture.store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"scaling": map[string]any{"enabled": true}}}, "test"); err != nil {
		t.Fatal(err)
	}
	target := reductionWriteTarget(t, fixture, "41", 2)
	target.UpstreamAllocation, target.DesiredHealth = true, "healthy"
	result, err := service.Apply(t.Context(), map[string]business.AccountRoutingTarget{"41": target}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed != 0 || fixture.states["41"]["concurrency"] != 4 {
		t.Fatalf("independent path bypassed enabled scaling switches: %+v", result)
	}
}
