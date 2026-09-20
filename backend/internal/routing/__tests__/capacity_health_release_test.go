package routing_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"testing"
	"time"
)

func TestConfirmedAutomaticFuseReleasesSharedCapacityForHealthySibling(t *testing.T) {
	waiting, fused := upstreamCapacityAccount("41", 1, 10), upstreamCapacityAccount("42", 10, 10)
	stopped := false
	waiting.Schedulable, waiting.EffectiveState = &stopped, "concurrency_limited"
	fused.Schedulable, fused.EffectiveState = &stopped, "fused"
	fused.HasRoutingBaseline, fused.ManagedSchedulable = true, &stopped
	r := allocationFixture(t, waiting, fused)
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"]; target.Schedulable == nil || !*target.Schedulable {
		t.Fatalf("confirmed stopped fuse must not starve eligible sibling: %+v", target)
	}
}

func TestNewAPIWaitingAccountWithScalingEnabledDoesNotRequireSub2APIIdentity(t *testing.T) {
	account := upstreamCapacityAccount("41", 1, 10)
	stopped, kind := false, "newapi"
	account.Schedulable, account.EffectiveState = &stopped, "concurrency_limited"
	account.UpstreamType, account.UpstreamID = &kind, ""
	r := upstreamCapacityFixture(t, account)
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"]; target.Schedulable == nil || !*target.Schedulable {
		t.Fatalf("New API cannot be gated by missing Sub2API identity: %+v", target)
	}
}

func TestAutomaticFuseReservationIsRetainedWithoutConfirmedManagedStop(t *testing.T) {
	for _, scenario := range []string{"manual pause", "manual fuse", "outside allocation", "unconfirmed", "remote active", "external control"} {
		t.Run(scenario, func(t *testing.T) {
			waiting, fused := upstreamCapacityAccount("41", 1, 10), upstreamCapacityAccount("42", 10, 10)
			stopped := false
			waiting.Schedulable, waiting.EffectiveState = &stopped, "concurrency_limited"
			fused.Schedulable, fused.EffectiveState = &stopped, "fused"
			fused.HasRoutingBaseline, fused.ManagedSchedulable = true, &stopped
			r := allocationFixture(t, waiting, fused)
			switch scenario {
			case "manual pause":
				r.accounts[1].Paused = true
			case "manual fuse":
				r.policy["scope"].(map[string]any)["manual_fused_account_ids"] = []any{"42"}
			case "outside allocation":
				r.policy["upstream_concurrency"] = map[string]any{"enabled": true, "account_mode": "selected", "account_ids": []any{"41"}}
			case "unconfirmed":
				r.accounts[1].ManagedSchedulable = nil
			case "remote active":
				active := true
				r.accounts[1].Schedulable = &active
			case "external control":
				r.accounts[1].ExternalControl = true
			}
			result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			if target := result.AccountTargets["41"]; target.Schedulable == nil || *target.Schedulable {
				t.Fatalf("protected or unconfirmed reservation was spent: %+v", target)
			}
		})
	}
}

func TestGlobalBudgetRebalancesAcrossUpstreamsWithoutStarvingHealthyAccounts(t *testing.T) {
	first, second := upstreamCapacityAccount("41", 10, 100), upstreamCapacityAccount("42", 10, 100)
	second.UpstreamID = "upstream-2"
	r := allocationFixture(t, first, second)
	r.policy["scaling"].(map[string]any)["enabled"] = true
	r.policy["scaling"].(map[string]any)["global_max_concurrency"] = 10
	r.policy["scaling"].(map[string]any)["step_down"] = 100
	r.policy["auto_apply"].(map[string]any)["concurrency"] = true
	r.policy["auto_apply"].(map[string]any)["schedulable"] = true
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"41", "42"} {
		target := result.AccountTargets[id]
		if target.Schedulable == nil || !*target.Schedulable || target.Concurrency == nil || *target.Concurrency != 5 {
			t.Fatalf("global quota should shrink each upstream to five: %+v", target)
		}
	}
}

func TestGlobalBudgetRedistributesUnusedSmallUpstreamShare(t *testing.T) {
	first, second := upstreamCapacityAccount("41", 1, 2), upstreamCapacityAccount("42", 1, 100)
	second.UpstreamID = "upstream-2"
	r := allocationFixture(t, first, second)
	r.policy["scaling"].(map[string]any)["enabled"] = true
	r.policy["scaling"].(map[string]any)["global_max_concurrency"] = 10
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]int64{"41": 2, "42": 8} {
		if target := result.AccountTargets[id]; target.Concurrency == nil || *target.Concurrency != want {
			t.Fatalf("pool %s want %d: %+v", id, want, target)
		}
	}
}

func TestScarceGlobalBudgetKeepsUnselectedUpstreamWaitingForHealthyPeer(t *testing.T) {
	unknown, healthy := upstreamCapacityAccount("41", 1, 10), upstreamCapacityAccount("42", 1, 10)
	unknown.UpstreamID, healthy.UpstreamID = "upstream-1", "upstream-2"
	stopped := false
	unknown.Schedulable, unknown.EffectiveState = &stopped, "concurrency_limited"
	healthy.Schedulable, healthy.EffectiveState = &stopped, "concurrency_limited"
	r := allocationFixture(t, unknown, healthy)
	r.policy["scaling"].(map[string]any)["enabled"] = true
	r.policy["scaling"].(map[string]any)["global_max_concurrency"] = 1
	r.policy["auto_apply"].(map[string]any)["concurrency"] = true
	r.policy["auto_apply"].(map[string]any)["schedulable"] = true
	r.samples = []business.RoutingSample{{AccountID: "42", GroupName: "codex", Source: "traffic", Result: "通过", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Payload: map[string]any{"input_tokens": 10, "output_tokens": 5}}}
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"]; target.Schedulable == nil || *target.Schedulable {
		t.Fatalf("unselected upstream must retain its zero global share: %+v", target)
	}
	if target := result.AccountTargets["42"]; target.Schedulable == nil || !*target.Schedulable || target.Concurrency != nil && *target.Concurrency != 1 {
		t.Fatalf("healthy peer must receive the one available global slot: %+v decisions=%+v", target, result.AccountDecisions)
	}
}

func TestHealthyWaitingAccountGetsCapacityBeforeAccountWithoutHealthEvidence(t *testing.T) {
	unknown, healthy := upstreamCapacityAccount("41", 1, 1), upstreamCapacityAccount("42", 1, 1)
	stopped := false
	healthy.Schedulable, healthy.EffectiveState = &stopped, "concurrency_limited"
	r := allocationFixture(t, unknown, healthy)
	r.samples = []business.RoutingSample{{AccountID: "42", GroupName: "codex", Source: "traffic", Result: "通过", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Payload: map[string]any{"input_tokens": 10, "output_tokens": 5}}}
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"]; target.Schedulable == nil || *target.Schedulable {
		t.Fatalf("unverified account must yield scarce capacity to healthy account: %+v", target)
	}
	r.accounts[0].Schedulable, r.accounts[0].EffectiveState = &stopped, "concurrency_limited"
	result, err = routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["42"]; target.Schedulable == nil || !*target.Schedulable {
		t.Fatalf("healthy account must recover after capacity release is confirmed: %+v", target)
	}
}
