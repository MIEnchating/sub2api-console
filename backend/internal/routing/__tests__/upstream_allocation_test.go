package routing_test

import (
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func allocationFixture(t *testing.T, accounts ...business.RoutingAccount) *upstreamReductionRepository {
	r := reductionFixture(t, accounts...)
	r.policy["scaling"].(map[string]any)["global_max_concurrency"] = 100
	return r
}

func TestSharedAllocationWithScalingDisabledGivesEveryHealthyAccountCapacity(t *testing.T) {
	r := allocationFixture(t, upstreamCapacityAccount("41", 1, 10), upstreamCapacityAccount("42", 1, 10), upstreamCapacityAccount("43", 1, 10))
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]int64{"41": 4, "42": 3, "43": 3} {
		target := result.AccountTargets[id]
		if target.Concurrency == nil || *target.Concurrency != want || target.Schedulable == nil || !*target.Schedulable {
			t.Fatalf("account %s must receive %d slots: %+v", id, want, target)
		}
	}
}

func TestSharedAllocationReclaimsOversizedAccountBeforeRecoveringWaitingSibling(t *testing.T) {
	first, waiting := upstreamCapacityAccount("41", 10, 10), upstreamCapacityAccount("42", 1, 10)
	inactive := false
	waiting.Schedulable, waiting.EffectiveState = &inactive, "concurrency_limited"
	r := allocationFixture(t, first, waiting)
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"]; target.Concurrency == nil || *target.Concurrency != 5 {
		t.Fatalf("must reclaim five slots: %+v", target)
	}
	if target := result.AccountTargets["42"]; target.Schedulable == nil || *target.Schedulable {
		t.Fatalf("must wait for readback: %+v", target)
	}
	confirmed := int64(5)
	r.accounts[0].Concurrency = &confirmed
	result, err = routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["42"]; target.Schedulable == nil || !*target.Schedulable || target.DesiredHealth == "concurrency_limited" {
		t.Fatalf("confirmed free capacity must restore waiting account without generic switches: %+v", target)
	}
}

func TestSharedAllocationRespectsGlobalCapacityWithScalingEnabled(t *testing.T) {
	r := allocationFixture(t, upstreamCapacityAccount("41", 1, 10), upstreamCapacityAccount("42", 1, 10))
	r.policy["scaling"].(map[string]any)["enabled"] = true
	r.policy["scaling"].(map[string]any)["global_max_concurrency"] = 3
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"]; target.Concurrency == nil || *target.Concurrency != 2 {
		t.Fatalf("first account must get two global slots: %+v", target)
	}
	if target := result.AccountTargets["42"]; target.Concurrency != nil || target.Schedulable == nil || !*target.Schedulable {
		t.Fatalf("second account must retain its one slot: %+v", target)
	}
}

func TestSharedAllocationInsufficientQuotaKeepsOneSlotPerSelectedAccount(t *testing.T) {
	r := allocationFixture(t, upstreamCapacityAccount("41", 8, 2), upstreamCapacityAccount("42", 8, 2), upstreamCapacityAccount("43", 8, 2))
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"41", "42"} {
		target := result.AccountTargets[id]
		if target.Concurrency == nil || *target.Concurrency != 1 || target.Schedulable == nil || !*target.Schedulable {
			t.Fatalf("selected account needs one slot: %+v", target)
		}
	}
	if target := result.AccountTargets["43"]; target.Schedulable == nil || *target.Schedulable || target.DesiredHealth != "concurrency_limited" {
		t.Fatalf("only excess account should wait: %+v", target)
	}
}

func TestSharedAllocationStaleCapacityNeverFundsRecoveryOrGrowth(t *testing.T) {
	first, waiting := upstreamCapacityAccount("41", 1, 10), upstreamCapacityAccount("42", 1, 10)
	inactive := false
	waiting.Schedulable, waiting.EffectiveState = &inactive, "concurrency_limited"
	r := allocationFixture(t, first, waiting)
	for i := range r.accounts {
		r.accounts[i].UpstreamConcurrencyStatus = business.UpstreamConcurrencyStale
	}
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"]; target.Concurrency != nil {
		t.Fatalf("cache cannot fund growth: %+v", target)
	}
	if target := result.AccountTargets["42"]; target.Schedulable == nil || *target.Schedulable {
		t.Fatalf("cache cannot fund recovery: %+v", target)
	}
}

func TestSharedAllocationAdjustsPausedUnlimitedAccountBeforeRecovery(t *testing.T) {
	first, waiting := upstreamCapacityAccount("41", 1, 10), upstreamCapacityAccount("42", 0, 10)
	inactive := false
	waiting.Schedulable, waiting.EffectiveState = &inactive, "concurrency_limited"
	r := allocationFixture(t, first, waiting)
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	target := result.AccountTargets["42"]
	if target.Concurrency == nil || *target.Concurrency != 5 || target.Schedulable == nil || *target.Schedulable {
		t.Fatalf("unlimited paused setting must become finite before recovery: %+v", target)
	}
}

func TestSharedAllocationDeduplicatesMultiGroupAccount(t *testing.T) {
	first, second := upstreamCapacityAccount("41", 1, 10), upstreamCapacityAccount("42", 1, 10)
	duplicate := first
	duplicate.GroupName = "second-group"
	r := allocationFixture(t, first, second, duplicate)
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	total := int64(0)
	if len(result.AccountTargets) != 2 {
		t.Fatalf("must emit exactly one target per stable ID: %+v", result.AccountTargets)
	}
	for _, id := range []string{"41", "42"} {
		target := result.AccountTargets[id]
		if target.Concurrency == nil || *target.Concurrency < 1 {
			t.Fatalf("each unique account needs capacity: %+v", target)
		}
		total += *target.Concurrency
	}
	if total != 10 {
		t.Fatalf("two unique accounts must share exactly ten slots, got %d", total)
	}
}

func TestNewUnlimitedAccountDoesNotEraseOtherUpstreamGlobalReservation(t *testing.T) {
	existing, added, outside := upstreamCapacityAccount("41", 5, 10), upstreamCapacityAccount("42", 0, 10), upstreamCapacityAccount("43", 8, 10)
	outside.UpstreamID = "other-upstream"
	priority := int64(1)
	outside.ManualPriority = &priority
	r := allocationFixture(t, existing, added, outside)
	r.policy["scaling"].(map[string]any)["step_down"] = 100
	r.policy["scaling"].(map[string]any)["enabled"] = true
	r.policy["scaling"].(map[string]any)["global_max_concurrency"] = 10
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"41", "42"} {
		target := result.AccountTargets[id]
		if target.Concurrency == nil || *target.Concurrency != 1 {
			t.Fatalf("new unlimited setting must preserve eight reserved slots and allocate one per account: %+v", target)
		}
	}
}

func TestSharedAllocationIgnoresDisabledGlobalLimitAndOtherUpstreamUsage(t *testing.T) {
	for _, scenario := range []string{"finite Sub2API", "unlimited Sub2API", "unlimited NewAPI"} {
		t.Run(scenario, func(t *testing.T) {
			first, waiting, outside := upstreamCapacityAccount("41", 1, 10), upstreamCapacityAccount("42", 1, 10), upstreamCapacityAccount("43", 1000, 2000)
			inactive := false
			waiting.Schedulable, waiting.EffectiveState = &inactive, "concurrency_limited"
			outside.UpstreamID = "other-upstream"
			if strings.HasPrefix(scenario, "unlimited") {
				*outside.Concurrency = 0
			}
			priority := int64(1)
			outside.ManualPriority = &priority
			if scenario == "unlimited NewAPI" {
				kind := "newapi"
				outside.UpstreamType, outside.ManualPriority = &kind, nil
			}
			r := allocationFixture(t, first, waiting, outside)
			r.policy["scaling"].(map[string]any)["global_max_concurrency"] = 2
			result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			if target := result.AccountTargets["41"]; target.Concurrency == nil || *target.Concurrency != 5 {
				t.Fatalf("disabled global scaling blocked upstream-local growth: %+v", target)
			}
			if scenario == "unlimited NewAPI" {
				target := result.AccountTargets["43"]
				if target.Concurrency != nil || target.DesiredHealth == "concurrency_limited" {
					t.Fatalf("New API must retain unrestricted concurrency: %+v", target)
				}
			}
			if target := result.AccountTargets["42"]; target.Schedulable == nil || !*target.Schedulable || target.DesiredHealth == "concurrency_limited" {
				t.Fatalf("disabled global scaling must not block upstream-local recovery: %+v", target)
			}
		})
	}
}

func TestSharedAllocationReleasesOnlyConfirmedManagedCostPause(t *testing.T) {
	for _, scenario := range []string{"confirmed", "unconfirmed", "manual", "excluded", "outside round", "fused"} {
		t.Run(scenario, func(t *testing.T) {
			waiting, blocked := upstreamCapacityAccount("41", 1, 10), upstreamCapacityAccount("42", 10, 10)
			inactive := false
			waiting.Schedulable, waiting.EffectiveState = &inactive, "concurrency_limited"
			blocked.Schedulable, blocked.EffectiveState = &inactive, "cost_blocked"
			blocked.HasRoutingBaseline, blocked.ManagedSchedulable = true, &inactive
			cost, wall := "20", "10"
			blocked.Multiplier, blocked.GroupCostWall, waiting.GroupCostWall = &cost, &wall, &wall
			r := allocationFixture(t, waiting, blocked)
			scope := routing.Scope{}
			switch scenario {
			case "unconfirmed":
				r.accounts[1].ManagedSchedulable = nil
			case "manual":
				priority := int64(1)
				r.accounts[1].ManualPriority = &priority
			case "excluded":
				r.policy["scope"].(map[string]any)["excluded_account_ids"] = []any{"42"}
			case "outside round":
				id := "41"
				scope.AccountID = &id
			case "fused":
				r.accounts[1].EffectiveState = "fused"
			}
			result, err := routing.NewService(r).Calculate(t.Context(), scope, true)
			if err != nil {
				t.Fatal(err)
			}
			target := result.AccountTargets["41"]
			wantActive := scenario == "confirmed" || scenario == "fused"
			if target.Schedulable == nil || *target.Schedulable != wantActive {
				t.Fatalf("capacity release must require confirmed managed cost stop: %+v", target)
			}
		})
	}
}

func TestSharedAllocationHonorsEnabledGroupScalingGlobalBudget(t *testing.T) {
	r := allocationFixture(t, upstreamCapacityAccount("41", 1, 10), upstreamCapacityAccount("42", 1, 10))
	r.policy["scaling"].(map[string]any)["global_max_concurrency"] = 3
	r.policy["group_policy_bindings"] = map[string]any{"7": map[string]any{"enabled": true, "scaling_enabled": true}}
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"]; target.Concurrency == nil || *target.Concurrency != 2 {
		t.Fatalf("enabled group scaling must enforce global budget: %+v", target)
	}
}

func TestSharedAllocationWithScalingEnabledUsesConfiguredBoundsStepsAndCooldown(t *testing.T) {
	for _, scenario := range []string{"maximum", "step up", "step down", "cooldown"} {
		t.Run(scenario, func(t *testing.T) {
			current, want := int64(1), int64(4)
			if scenario == "step up" {
				want = 2
			}
			if scenario == "step down" {
				current, want = 10, 8
			}
			r := allocationFixture(t, upstreamCapacityAccount("41", current, 100))
			r.policy["auto_apply"].(map[string]any)["concurrency"] = true
			r.policy["auto_apply"].(map[string]any)["schedulable"] = true
			scaling := r.policy["scaling"].(map[string]any)
			scaling["enabled"], scaling["min_per_account"], scaling["max_per_account"], scaling["step_down"] = true, 2, 4, 2
			if scenario == "step up" {
				scaling["step_up"] = 1
			}
			if scenario == "cooldown" {
				r.previous = []business.PreviousRoutingDecision{{AccountID: "41", GroupName: "codex", State: "healthy", LastApplyAt: time.Now()}}
			}
			r.samples = []business.RoutingSample{{AccountID: "41", GroupName: "codex", Source: "traffic", Result: "通过", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Payload: map[string]any{"input_tokens": 10, "output_tokens": 5}}}
			result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			target := result.AccountTargets["41"]
			if target.UpstreamAllocation {
				t.Fatal("enabled scaling must not bypass configured write controls")
			}
			if scenario == "cooldown" {
				if target.Concurrency != nil || !target.ScalingCooldown {
					t.Fatalf("configured cooldown ignored: %+v", target)
				}
			} else if target.Concurrency == nil || *target.Concurrency != want {
				t.Fatalf("configured bounds or step ignored, want %d: %+v", want, target)
			}
		})
	}
}
