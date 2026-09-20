package routing_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"testing"
)

func TestSelectedSharedAllocationReservesUnselectedAndNewAccounts(t *testing.T) {
	r := allocationFixture(t, upstreamCapacityAccount("41", 1, 10), upstreamCapacityAccount("42", 4, 10), upstreamCapacityAccount("43", 1, 10))
	r.policy["upstream_concurrency"] = map[string]any{"enabled": true, "account_mode": "selected", "account_ids": []any{"41"}}
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"]; target.Concurrency == nil || *target.Concurrency != 5 || !target.UpstreamAllocation {
		t.Fatalf("selected account must receive only unreserved capacity: %+v", target)
	}
	for _, id := range []string{"42", "43"} {
		if target := result.AccountTargets[id]; target.Concurrency != nil || target.UpstreamAllocation || target.Schedulable == nil || !*target.Schedulable {
			t.Fatalf("unselected/new account %s changed: %+v", id, target)
		}
	}
}

func TestSelectedSharedAllocationWithEmptySelectionDoesNotAllocate(t *testing.T) {
	r := allocationFixture(t, upstreamCapacityAccount("41", 1, 10))
	r.policy["upstream_concurrency"] = map[string]any{"enabled": true, "account_mode": "selected", "account_ids": []any{}}
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"]; target.Concurrency != nil || target.UpstreamAllocation {
		t.Fatalf("empty selection must not mean all: %+v", target)
	}
}

func TestSelectedSharedAllocationRestoresOrdinarySchedulingForUnselectedWaitingAccount(t *testing.T) {
	first, waiting := upstreamCapacityAccount("41", 1, 10), upstreamCapacityAccount("42", 4, 10)
	stopped := false
	waiting.Schedulable = &stopped
	waiting.EffectiveState = "concurrency_limited"
	r := allocationFixture(t, first, waiting)
	r.policy["upstream_concurrency"] = map[string]any{"enabled": true, "account_mode": "selected", "account_ids": []any{"41"}}
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["42"]; target.Schedulable == nil || !*target.Schedulable || target.UpstreamAllocation || target.Concurrency != nil || target.DesiredHealth == "concurrency_limited" {
		t.Fatalf("removed account must return to ordinary scheduling: %+v", target)
	}
	if target := result.AccountTargets["41"]; target.Concurrency == nil || *target.Concurrency != 6 {
		t.Fatalf("selected peer must reserve recovering account's four slots: %+v", target)
	}
	allowed, err := routing.UpstreamAllocationAllowed(r.policy, waiting)
	if err != nil || allowed {
		t.Fatalf("writer scope must reject unselected recovery: %t %v", allowed, err)
	}
}

func TestSelectedSharedAllocationStaleReductionPreservesUnselectedCapacity(t *testing.T) {
	r := allocationFixture(t, upstreamCapacityAccount("41", 8, 10), upstreamCapacityAccount("42", 4, 10))
	for i := range r.accounts {
		r.accounts[i].UpstreamConcurrencyStatus = "stale"
	}
	r.policy["upstream_concurrency"] = map[string]any{"enabled": true, "account_mode": "selected", "account_ids": []any{"41"}}
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"]; target.Concurrency == nil || *target.Concurrency != 6 {
		t.Fatalf("selected reduction must reserve sibling's four slots: %+v", target)
	}
	if target := result.AccountTargets["42"]; target.Concurrency != nil || target.UpstreamReductionID != "" {
		t.Fatalf("unselected reduction: %+v", target)
	}
}

func TestSelectedSharedAllocationKeepsConfiguredScalingForOtherAccounts(t *testing.T) {
	r := allocationFixture(t, upstreamCapacityAccount("41", 1, 10), upstreamCapacityAccount("42", 1, 10))
	r.policy["upstream_concurrency"] = map[string]any{"enabled": true, "account_mode": "selected", "account_ids": []any{"41"}}
	scaling := r.policy["scaling"].(map[string]any)
	scaling["enabled"] = true
	scaling["step_up"] = 1
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"41", "42"} {
		if target := result.AccountTargets[id]; target.Concurrency == nil || *target.Concurrency != 2 || target.UpstreamAllocation {
			t.Fatalf("account %s must follow enabled scaling step, without independent-write override: %+v", id, target)
		}
	}
}

func TestUpstreamSharedAllocationIncludesNewAccountsOnlyOnSelectedStableID(t *testing.T) {
	first, added, other := upstreamCapacityAccount("41", 1, 10), upstreamCapacityAccount("42", 1, 10), upstreamCapacityAccount("43", 1, 10)
	first.UpstreamID, added.UpstreamID, other.UpstreamID = "Upstream-A", "Upstream-A", "upstream-a"
	r := allocationFixture(t, first, added, other)
	r.policy["upstream_concurrency"] = map[string]any{"enabled": true, "account_mode": "upstreams", "upstream_ids": []any{"Upstream-A", "Upstream-A"}}
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"41", "42"} {
		target := result.AccountTargets[id]
		if target.Concurrency == nil || *target.Concurrency != 5 || !target.UpstreamAllocation {
			t.Fatalf("selected upstream account %s must share quota: %+v", id, target)
		}
	}
	if target := result.AccountTargets["43"]; target.Concurrency != nil || target.UpstreamAllocation {
		t.Fatalf("other stable upstream must stay unchanged: %+v", target)
	}
}

func TestUpstreamSharedAllocationEmptySelectionDoesNotAllocate(t *testing.T) {
	r := allocationFixture(t, upstreamCapacityAccount("41", 1, 10))
	r.policy["upstream_concurrency"] = map[string]any{"enabled": true, "account_mode": "upstreams", "upstream_ids": []any{}}
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	for id, target := range result.AccountTargets {
		if target.Concurrency != nil || target.UpstreamAllocation {
			t.Fatalf("empty upstream scope changed %s: %+v", id, target)
		}
	}
}

func TestSharedAllocationAccountOverrideIncludesAccountOutsideSelectedUpstreams(t *testing.T) {
	r := allocationFixture(t, upstreamCapacityAccount("147", 1, 10), upstreamCapacityAccount("148", 4, 10))
	r.policy["upstream_concurrency"] = map[string]any{"enabled": true, "account_mode": "upstreams", "upstream_ids": []any{"other"}, "account_overrides": map[string]any{"147": true}}
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	target := result.AccountTargets["147"]
	if target.Concurrency == nil || *target.Concurrency != 6 || !target.UpstreamAllocation {
		t.Fatalf("single account opt-in must preserve sibling reservation: %+v", target)
	}
}

func TestSharedAllocationAccountOverrideWinsOverUpstreamAndGlobalScope(t *testing.T) {
	r := allocationFixture(t, upstreamCapacityAccount("41", 1, 10), upstreamCapacityAccount("42", 1, 10))
	r.policy["upstream_concurrency"] = map[string]any{"enabled": true, "account_mode": "selected", "account_ids": []any{}, "upstream_overrides": map[string]any{"upstream-1": true}, "account_overrides": map[string]any{"41": false}}
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"]; target.UpstreamAllocation || target.Concurrency != nil {
		t.Fatalf("explicitly disabled account changed: %+v", target)
	}
	if target := result.AccountTargets["42"]; target.Concurrency == nil || *target.Concurrency != 9 {
		t.Fatalf("new sibling must inherit upstream opt-in: %+v", target)
	}
}

func TestSharedAllocationOverridesRespectMasterOffAndWriterRechecks(t *testing.T) {
	for _, master := range []bool{false, true} {
		r := allocationFixture(t, upstreamCapacityAccount("41", 1, 10))
		r.policy["upstream_concurrency"] = map[string]any{"enabled": master, "account_mode": "all", "upstream_overrides": map[string]any{"upstream-1": false}, "account_overrides": map[string]any{"41": true}}
		result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
		if err != nil {
			t.Fatal(err)
		}
		if result.AccountTargets["41"].UpstreamAllocation != master {
			t.Fatalf("master %t target %+v", master, result.AccountTargets["41"])
		}
		allowed, err := routing.UpstreamAllocationAllowed(r.policy, r.accounts[0])
		if err != nil || allowed != master {
			t.Fatalf("master %t writer %t %v", master, allowed, err)
		}
		r.policy["upstream_concurrency"].(map[string]any)["account_overrides"] = map[string]any{}
		allowed, err = routing.UpstreamAllocationAllowed(r.policy, r.accounts[0])
		if err != nil || allowed {
			t.Fatalf("reset must respect upstream off: %t %v", allowed, err)
		}
	}
}

func TestSharedAllocationRejectsMalformedOverridesBeforeCalculating(t *testing.T) {
	r := allocationFixture(t, upstreamCapacityAccount("41", 1, 10))
	r.policy["upstream_concurrency"] = map[string]any{"enabled": true, "account_overrides": map[string]any{"41": "true"}}
	if _, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true); err == nil {
		t.Fatal("malformed override silently ignored")
	}
}
