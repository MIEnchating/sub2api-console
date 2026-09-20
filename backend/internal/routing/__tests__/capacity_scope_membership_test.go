package routing_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"testing"
)

func TestCapacityWriterUsesSamePrimaryGroupScalingAsCalculation(t *testing.T) {
	for _, on := range []bool{false, true} {
		t.Run(map[bool]string{false: "primary off secondary on", true: "primary on secondary off"}[on], func(t *testing.T) {
			account := upstreamCapacityAccount("41", 1, 10)
			second := account
			other := "8"
			second.GroupID, second.GroupName = &other, "second"
			r := allocationFixture(t, account, second)
			r.policy["upstream_concurrency"] = map[string]any{"enabled": false}
			r.policy["group_policy_bindings"] = map[string]any{"7": map[string]any{"scaling_enabled": on}, "8": map[string]any{"scaling_enabled": !on}}
			scope, err := routing.UpstreamCapacityScope(r.policy, []business.RoutingAccount{second, account})
			if err != nil {
				t.Fatal(err)
			}
			if scope["41"] != on {
				t.Fatalf("writer must follow primary group, got %v want %v", scope["41"], on)
			}
		})
	}
}

func TestSharedAllocationDoesNotConstrainNewAPIWhenItsGroupScalingIsOff(t *testing.T) {
	account := upstreamCapacityAccount("41", 31, 100)
	kind := "newapi"
	account.UpstreamType = &kind
	r := allocationFixture(t, account)
	r.policy["group_policy_bindings"] = map[string]any{"7": map[string]any{"scaling_enabled": false}, "8": map[string]any{"scaling_enabled": true}}
	scope, err := routing.UpstreamCapacityScope(r.policy, []business.RoutingAccount{account})
	if err != nil {
		t.Fatal(err)
	}
	if scope["41"] {
		t.Fatal("Sub2API shared allocation must not opt New API into global capacity controls")
	}
}

func TestStaleSharedQuotaStillHonorsEnabledScalingReductionStep(t *testing.T) {
	account := upstreamCapacityAccount("41", 40, 10)
	r := allocationFixture(t, account)
	r.accounts[0].UpstreamConcurrencyStatus = "stale"
	r.policy["scaling"].(map[string]any)["enabled"] = true
	r.policy["scaling"].(map[string]any)["step_down"] = 1
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	target := result.AccountTargets["41"]
	if target.Concurrency == nil || *target.Concurrency != 39 || target.UpstreamReductionID != "" {
		t.Fatalf("stale fallback bypassed scaling step: %+v", target)
	}
}
