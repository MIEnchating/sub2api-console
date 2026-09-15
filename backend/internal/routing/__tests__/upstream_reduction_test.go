package routing_test

import (
	"context"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

type upstreamReductionRepository struct {
	*upstreamCapacityRepository
	policy map[string]any
}

func (r *upstreamReductionRepository) ControlPolicy(context.Context) (map[string]any, error) {
	return r.policy, nil
}

func reductionFixture(t *testing.T, accounts ...business.RoutingAccount) *upstreamReductionRepository {
	t.Helper()
	base := upstreamCapacityFixture(t, accounts...)
	policy, err := base.ControlPolicy(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	policy["upstream_concurrency"] = map[string]any{"enabled": true}
	policy["auto_apply"] = map[string]any{"concurrency": false, "schedulable": false, "priority": false, "load_factor": false}
	scaling := policy["scaling"].(map[string]any)
	scaling["enabled"], scaling["global_max_concurrency"] = false, 1
	scaling["step_down"], scaling["cooldown_seconds"] = 1, 3600
	for index := range base.accounts {
		base.accounts[index].UpstreamConcurrencyStatus = business.UpstreamConcurrencyKnown
	}
	return &upstreamReductionRepository{upstreamCapacityRepository: base, policy: policy}
}

func TestIndependentUpstreamReductionLowersOverageWithScalingAndFieldSwitchesDisabled(t *testing.T) {
	repository := reductionFixture(t, upstreamCapacityAccount("41", 80, 10), upstreamCapacityAccount("42", 80, 10))
	for _, id := range []string{"41", "42"} {
		repository.previous = append(repository.previous, business.PreviousRoutingDecision{AccountID: id, GroupName: "codex", State: "healthy", LastApplyAt: time.Now()})
	}
	result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"41", "42"} {
		target := result.AccountTargets[id]
		if target.Concurrency == nil || *target.Concurrency != 5 || target.ConfigurationError != nil {
			t.Fatalf("independent overage correction must bypass scaling steps, cooldown and write switches: %+v", target)
		}
	}
}

func TestIndependentUpstreamReductionDoesNotTouchCapacityWithoutConfirmedFiniteOverage(t *testing.T) {
	for _, name := range []string{"at limit with fused reservation", "unlimited", "unknown", "other upstream type", "disabled"} {
		t.Run(name, func(t *testing.T) {
			first, second := upstreamCapacityAccount("41", 10, 10), upstreamCapacityAccount("42", 10, 10)
			repository := reductionFixture(t, first, second)
			switch name {
			case "at limit with fused reservation":
				inactive := false
				repository.accounts[1].Schedulable = &inactive
				repository.accounts[1].EffectiveState = "fused"
			case "unlimited":
				zero := int64(0)
				for index := range repository.accounts {
					repository.accounts[index].UpstreamConcurrencyLimit = &zero
				}
			case "unknown":
				for index := range repository.accounts {
					repository.accounts[index].UpstreamConcurrencyStatus = "unknown"
					repository.accounts[index].UpstreamConcurrencyLimit = nil
				}
			case "other upstream type":
				kind := "newapi"
				for index := range repository.accounts {
					repository.accounts[index].UpstreamType = &kind
				}
			case "disabled":
				repository.policy["upstream_concurrency"] = map[string]any{"enabled": false}
			}
			result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			if target := result.AccountTargets["41"]; target.Concurrency != nil || target.DesiredHealth == "concurrency_limited" {
				t.Fatalf("capacity without confirmed finite overage must remain unchanged: %+v", target)
			}
		})
	}
}

func TestIndependentUpstreamReductionReservesProtectedAccountsAndPausesByStablePriority(t *testing.T) {
	first, second, manual := upstreamCapacityAccount("10", 5, 3), upstreamCapacityAccount("2", 5, 3), upstreamCapacityAccount("41", 2, 3)
	priority := int64(1)
	manual.ManualPriority = &priority
	repository := reductionFixture(t, first, second, manual)
	result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["2"]; target.Concurrency == nil || *target.Concurrency != 1 || target.Schedulable == nil || !*target.Schedulable {
		t.Fatalf("one unreserved slot must remain with the best stable account: %+v", target)
	}
	if target := result.AccountTargets["10"]; target.DesiredHealth != "concurrency_limited" || target.Schedulable == nil || *target.Schedulable || target.Concurrency != nil && *target.Concurrency == 0 {
		t.Fatalf("low priority account must wait without writing unlimited zero: %+v", target)
	}
	if _, exists := result.AccountTargets["41"]; exists {
		t.Fatal("manual account must remain protected")
	}
}

func TestIndependentUpstreamReductionUsesExistingPriceWeightsWithoutGrowingSmallAccounts(t *testing.T) {
	first, second, small := upstreamCapacityAccount("41", 100, 17), upstreamCapacityAccount("42", 100, 17), upstreamCapacityAccount("43", 1, 17)
	expensive := "4"
	second.Multiplier = &expensive
	repository := reductionFixture(t, first, second, small)
	repository.policy["selection"].(map[string]any)["strategy"] = "price_first"
	for _, id := range []string{"41", "42", "43"} {
		repository.samples = append(repository.samples, business.RoutingSample{AccountID: id, GroupName: "codex", Source: "traffic", Result: "通过", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Payload: map[string]any{"input_tokens": 10, "output_tokens": 5}})
	}
	result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	cheap, costly := result.AccountTargets["41"].Concurrency, result.AccountTargets["42"].Concurrency
	if cheap == nil || costly == nil || *cheap <= *costly || *cheap+*costly != 16 || result.AccountTargets["43"].Concurrency != nil {
		t.Fatalf("price weights must apportion exactly the excess while small accounts never grow: %+v", result.AccountTargets)
	}
}

func TestIndependentUpstreamReductionPreservesCapacityPauseAfterLimitIncreases(t *testing.T) {
	first, waiting := upstreamCapacityAccount("41", 1, 10), upstreamCapacityAccount("42", 1, 10)
	inactive := false
	waiting.Schedulable, waiting.EffectiveState = &inactive, "concurrency_limited"
	repository := reductionFixture(t, first, waiting)
	result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["42"]; target.Schedulable == nil || *target.Schedulable || target.Concurrency != nil || target.DesiredHealth != "concurrency_limited" {
		t.Fatalf("reduction-only mode cannot expand or resume a capacity pause: %+v", target)
	}
}

func TestIndependentUpstreamReductionStillLowersOverageWhileHealthEvidenceIsPending(t *testing.T) {
	repository := reductionFixture(t, upstreamCapacityAccount("41", 80, 10), upstreamCapacityAccount("42", 80, 10))
	for _, id := range []string{"41", "42"} {
		repository.samples = append(repository.samples, business.RoutingSample{AccountID: id, GroupName: "codex", Source: "traffic", Result: "通过", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Payload: map[string]any{"input_tokens": 0, "output_tokens": 0}})
	}
	result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"41", "42"} {
		if !result.AccountDecisions[id].EvidencePending {
			t.Fatal("fixture must exercise genuinely pending health evidence")
		}
		if target := result.AccountTargets[id]; target.Concurrency == nil || *target.Concurrency != 5 {
			t.Fatalf("pending health may lower weight but cannot block a capacity-only reduction: %+v", target)
		}
	}
}

func TestIndependentUpstreamReductionDoesNotReallocateTheSamePoolThroughRegularScaling(t *testing.T) {
	repository := reductionFixture(t, upstreamCapacityAccount("41", 100, 10), upstreamCapacityAccount("42", 1, 10))
	repository.policy["scaling"].(map[string]any)["enabled"] = true
	repository.policy["auto_apply"] = map[string]any{"concurrency": true, "schedulable": true}
	result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"]; target.Concurrency == nil || *target.Concurrency != 9 {
		t.Fatalf("large account should use only the remaining nine slots: %+v", target)
	}
	if target := result.AccountTargets["42"]; target.Concurrency != nil || target.Schedulable == nil || !*target.Schedulable {
		t.Fatalf("regular scaling must not reallocate an already corrected upstream pool: %+v", target)
	}
}
