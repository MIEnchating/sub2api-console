package routing_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

type placementRepository struct {
	policy   map[string]any
	accounts []business.RoutingAccount
	samples  []business.RoutingSample
	previous []business.PreviousRoutingDecision
}

func (r *placementRepository) ControlPolicy(context.Context) (map[string]any, error) {
	return r.policy, nil
}

func (r *placementRepository) RoutingAccounts(context.Context, *string, *string) ([]business.RoutingAccount, error) {
	return r.accounts, nil
}

func (r *placementRepository) RoutingSamples(context.Context, *string, *string, string, int) ([]business.RoutingSample, error) {
	return r.samples, nil
}

func (r *placementRepository) PreviousRoutingDecisions(context.Context, *string, *string) ([]business.PreviousRoutingDecision, error) {
	return r.previous, nil
}

func (r *placementRepository) CleanupStates(context.Context, *string) (map[string]time.Time, error) {
	return nil, nil
}

func (r *placementRepository) PersistRoutingRound(context.Context, *string, *string, []business.RoutingEvaluationWrite, []business.RoutingDecisionWrite, []business.AccountRoutingTarget, []business.CleanupStateWrite, []business.RuntimeEventWrite, bool, time.Time) error {
	return nil
}

func placementFixture(accounts ...business.RoutingAccount) *placementRepository {
	return &placementRepository{
		accounts: accounts,
		policy: map[string]any{
			"selection": map[string]any{"strategy": "price_first"},
			"weights":   map[string]any{"scheduling_missing_rate_fallback": "fail_open", "performance_min_samples": 5, "change_threshold": "0.1", "cooldown_seconds": 60},
			"traffic":   map[string]any{"enabled": true}, "breaker": map[string]any{},
			"degrade": map[string]any{}, "recovery": map[string]any{}, "scaling": map[string]any{},
			"cleanup": map[string]any{"action": "none"}, "scope": map[string]any{},
		},
	}
}

func placementAccount(id, rate string, priority int64) business.RoutingAccount {
	enabled, group := true, "7"
	return business.RoutingAccount{
		ID: id, Name: id, GroupName: "codex", GroupID: &group, Schedulable: &enabled,
		Priority: &priority, BaselinePriority: &priority, HasRoutingBaseline: true,
		Multiplier: &rate, EffectiveState: "healthy", Metadata: map[string]any{},
	}
}

func (r *placementRepository) addSuccesses(id string, count int, latency int, age time.Duration) {
	now := time.Now().UTC().Add(-age)
	for index := range count {
		r.samples = append(r.samples, business.RoutingSample{
			AccountID: id, GroupName: "codex", Source: "traffic", Result: "通过",
			ObservedAt: now.Add(-time.Duration(index) * time.Second).Format(time.RFC3339Nano),
			Payload:    map[string]any{"request_id": fmt.Sprintf("%s-%d-%d", id, age, index), "status_code": 200, "first_token_ms": latency, "model": "test-model", "input_tokens": 1, "output_tokens": 1},
		})
	}
}

func TestStablePlacementPromotesClearlyBetterAccountPastIntermediateNoise(t *testing.T) {
	repository := placementFixture(placementAccount("41", "1", 20), placementAccount("42", "0.90", 21), placementAccount("43", "0.79", 22))
	for _, account := range repository.accounts {
		repository.addSuccesses(account.ID, 5, 1000, time.Second)
	}
	result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if decision := result.AccountDecisions["43"]; decision.Rank == nil || *decision.Rank != 1 {
		t.Fatalf("intermediate account blocked a clearly better challenger: %+v", result.AccountDecisions)
	}
}

func TestStablePlacementSmallPriceNoisePreservesAppliedOrderForEveryStrategy(t *testing.T) {
	for _, strategy := range []string{"balanced", "price_first", "speed_first", "reliability"} {
		t.Run(strategy, func(t *testing.T) {
			repository := placementFixture(placementAccount("41", "1", 20), placementAccount("42", "0.95", 21))
			repository.policy["selection"] = map[string]any{"strategy": strategy}
			for _, account := range repository.accounts {
				repository.addSuccesses(account.ID, 5, 1000, time.Second)
			}
			result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			for id, priority := range map[string]int64{"41": 20, "42": 21} {
				if actual := result.AccountTargets[id].Priority; actual == nil || *actual != priority {
					t.Fatalf("small price noise displaced a confirmed priority: %+v", result.AccountTargets)
				}
			}
		})
	}
}

func TestStablePlacementRequiresFreshConfidenceBeforeDisplacingIncumbent(t *testing.T) {
	for _, oldCount := range []int{0, 8} {
		t.Run(fmt.Sprintf("historical_successes_%d", oldCount), func(t *testing.T) {
			repository := placementFixture(placementAccount("41", "1", 20), placementAccount("42", "0.1", 21))
			repository.addSuccesses("41", 8, 1000, time.Second)
			repository.addSuccesses("42", 1, 1000, time.Second)
			repository.addSuccesses("42", oldCount, 1000, 3*time.Hour)
			result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			if decision := result.AccountDecisions["41"]; decision.Rank == nil || *decision.Rank != 1 {
				t.Fatalf("insufficient fresh evidence displaced the incumbent: %+v", result.AccountDecisions)
			}
		})
	}
}

func TestStablePlacementPreservesPrioritySlotsWhenMembershipChanges(t *testing.T) {
	for _, addNew := range []bool{false, true} {
		t.Run(fmt.Sprintf("new_account_%t", addNew), func(t *testing.T) {
			repository := placementFixture(placementAccount("41", "1", 20), placementAccount("43", "1", 22))
			if addNew {
				repository.accounts = append(repository.accounts, placementAccount("44", "1", 1))
			}
			for _, account := range repository.accounts {
				repository.addSuccesses(account.ID, 5, 1000, time.Second)
			}
			result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			for id, priority := range map[string]int64{"41": 20, "43": 22} {
				if actual := result.AccountTargets[id].Priority; actual == nil || *actual != priority {
					t.Fatalf("membership change moved account %s from its stable slot: %+v", id, result.AccountTargets)
				}
			}
		})
	}
}

func TestStablePlacementCoordinatesCooldownBeforeSwappingPriorities(t *testing.T) {
	repository := placementFixture(placementAccount("41", "1", 20), placementAccount("42", "0.1", 21))
	for _, account := range repository.accounts {
		repository.addSuccesses(account.ID, 5, 1000, time.Second)
	}
	repository.previous = []business.PreviousRoutingDecision{{AccountID: "41", GroupName: "codex", State: "healthy", LastApplyAt: time.Now().UTC()}}
	result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	for id, priority := range map[string]int64{"41": 20, "42": 21} {
		if actual := result.AccountTargets[id].Priority; actual == nil || *actual != priority {
			t.Fatalf("partial cooldown changed one side of a priority swap: %+v", result.AccountTargets)
		}
	}
}

func TestStablePlacementPromotesAfterCooldownAndKeepsConfirmedNewPosition(t *testing.T) {
	repository := placementFixture(placementAccount("41", "1", 20), placementAccount("42", "0.1", 21))
	for _, account := range repository.accounts {
		repository.addSuccesses(account.ID, 5, 1000, time.Second)
	}
	repository.previous = []business.PreviousRoutingDecision{{AccountID: "41", GroupName: "codex", State: "healthy", LastApplyAt: time.Now().UTC().Add(-2 * time.Minute)}}
	for range 3 {
		result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
		if err != nil {
			t.Fatal(err)
		}
		if target := result.AccountTargets["42"]; target.Priority == nil || *target.Priority != 20 {
			t.Fatalf("confirmed improvement did not retain the leading slot: %+v", result.AccountTargets)
		}
		for index := range repository.accounts {
			repository.accounts[index].Priority = result.AccountTargets[repository.accounts[index].ID].Priority
		}
	}
}

func TestStablePlacementIncomparableModelsDoNotCreateSpeedPromotion(t *testing.T) {
	repository := placementFixture(placementAccount("41", "1", 20), placementAccount("42", "1", 21))
	repository.policy["selection"] = map[string]any{"strategy": "speed_first"}
	repository.addSuccesses("41", 5, 4000, time.Second)
	repository.addSuccesses("42", 5, 100, time.Second)
	for index := range repository.samples {
		if repository.samples[index].AccountID == "42" {
			repository.samples[index].Payload["model"] = "different-model"
		}
	}
	result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"]; target.Priority == nil || *target.Priority != 20 {
		t.Fatalf("incomparable models created a speed advantage: %+v", result.AccountTargets)
	}
}

func TestStablePlacementMissingRateDoesNotCreatePricePromotion(t *testing.T) {
	first, second := placementAccount("41", "10", 20), placementAccount("42", "1", 21)
	second.Multiplier = nil
	repository := placementFixture(first, second)
	for _, account := range repository.accounts {
		repository.addSuccesses(account.ID, 5, 1000, time.Second)
	}
	result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"]; target.Priority == nil || *target.Priority != 20 {
		t.Fatalf("fallback rate was treated as a confirmed price advantage: %+v", result.AccountTargets)
	}
}

func TestStablePlacementUnchangedPriceSpeedHealthTradeoffsDoNotCycle(t *testing.T) {
	repository := placementFixture(placementAccount("41", "0.1", 20), placementAccount("42", "0.2", 21), placementAccount("43", "1.6", 22))
	repository.policy["selection"] = map[string]any{"strategy": "balanced"}
	repository.policy["scoring"] = map[string]any{"short_ratio": 0.000001, "event_scores": map[string]any{"gateway_error": 50, "slow_ttfb": 90}}
	repository.addSuccesses("41", 5, 200, time.Second)
	repository.addSuccesses("42", 5, 400, time.Second)
	repository.addSuccesses("43", 5, 100, time.Second)
	repository.addSuccesses("41", 5, 1000, 3*time.Hour)
	repository.addSuccesses("43", 5, 6000, 3*time.Hour)
	for index := range repository.samples {
		row := &repository.samples[index]
		observed, _ := time.Parse(time.RFC3339Nano, row.ObservedAt)
		if row.AccountID == "41" && observed.Before(time.Now().UTC().Add(-time.Hour)) {
			row.Result, row.Payload["status_code"] = "失败", 503
		}
	}
	confirmed := map[string]int64{}
	for round := range 4 {
		result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
		if err != nil {
			t.Fatal(err)
		}
		for index := range repository.accounts {
			account := &repository.accounts[index]
			priority := result.AccountTargets[account.ID].Priority
			if priority == nil {
				t.Fatalf("account %s lost its priority", account.ID)
			}
			if round > 0 && *priority != confirmed[account.ID] {
				t.Fatalf("unchanged three-dimensional evidence cycled positions in round %d: account=%s previous=%d current=%d", round, account.ID, confirmed[account.ID], *priority)
			}
			confirmed[account.ID], account.Priority = *priority, priority
		}
	}
}

func TestStablePlacementMissingBaselineDoesNotRepeatDegradationPenalty(t *testing.T) {
	account := placementAccount("41", "1", 20)
	account.BaselinePriority = nil
	repository := placementFixture(account)
	repository.addSuccesses("41", 5, 6000, time.Second)
	var confirmed int64
	for round := range 3 {
		result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
		if err != nil {
			t.Fatal(err)
		}
		priority := result.AccountTargets["41"].Priority
		if priority == nil {
			t.Fatal("degraded account lost its priority")
		}
		if round > 0 && *priority != confirmed {
			t.Fatalf("missing captured baseline repeatedly added the degradation penalty: before=%d after=%d", confirmed, *priority)
		}
		confirmed = *priority
		repository.accounts[0].Priority, repository.accounts[0].EffectiveState = priority, "degraded"
	}
}

func TestStablePlacementSecondaryGroupStrategyAndBudgetInfluenceAccountRank(t *testing.T) {
	first, second := placementAccount("41", "1", 20), placementAccount("42", "2", 21)
	secondaryFirst, secondarySecond := first, second
	secondaryGroup := "8"
	secondaryFirst.GroupID, secondarySecond.GroupID = &secondaryGroup, &secondaryGroup
	secondaryFirst.GroupName, secondarySecond.GroupName = "secondary", "secondary"
	repository := placementFixture(first, second, secondaryFirst, secondarySecond)
	repository.policy["group_policy_bindings"] = map[string]any{
		"8": map[string]any{"strategy": "speed_first", "weight_budget": 1600},
	}
	repository.addSuccesses("41", 5, 400, time.Second)
	repository.addSuccesses("42", 5, 100, time.Second)
	result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	incumbent, challenger := result.AccountDecisions["41"], result.AccountDecisions["42"]
	if incumbent.Priority == nil || challenger.Priority == nil || *challenger.Priority >= *incumbent.Priority {
		t.Fatalf("secondary speed policy and budget were lost during placement: incumbent=%+v challenger=%+v", incumbent, challenger)
	}
	if challenger.GroupName != "codex" || result.Decisions != 2 {
		t.Fatalf("secondary policy created separate account-level writers: %+v", result)
	}
}

func TestStablePlacementConfirmedDegradationYieldsDespiteCooldownAndCheapPrice(t *testing.T) {
	repository := placementFixture(placementAccount("41", "0.01", 20), placementAccount("42", "1", 21))
	repository.addSuccesses("41", 5, 6000, time.Second)
	repository.addSuccesses("42", 5, 4000, time.Second)
	repository.previous = []business.PreviousRoutingDecision{{AccountID: "41", GroupName: "codex", State: "degraded", LastApplyAt: time.Now().UTC()}}
	result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	healthy, degraded := result.AccountDecisions["42"], result.AccountDecisions["41"]
	if healthy.Rank == nil || *healthy.Rank != 1 || healthy.Priority == nil || degraded.Priority == nil || *healthy.Priority >= *degraded.Priority {
		t.Fatalf("confirmed degradation retained its leading position: %+v", result.AccountDecisions)
	}
}

func TestStablePlacementSharedCapacityKeepsIncumbentForSmallWeightNoise(t *testing.T) {
	for _, reduction := range []bool{false, true} {
		t.Run(fmt.Sprintf("reduction_only_%t", reduction), func(t *testing.T) {
			repository := placementFixture(placementAccount("41", "1", 20), placementAccount("42", "0.95", 21))
			kind, limit, concurrency := "sub2api", int64(1), int64(1)
			for index := range repository.accounts {
				account := &repository.accounts[index]
				account.UpstreamID, account.UpstreamType = "test-upstream", &kind
				account.UpstreamConcurrencyLimit, account.Concurrency = &limit, &concurrency
				account.UpstreamConcurrencyStatus = business.UpstreamConcurrencyKnown
				repository.addSuccesses(account.ID, 5, 1000, time.Second)
			}
			repository.policy["scaling"] = map[string]any{"enabled": !reduction, "min_per_account": 1}
			repository.policy["upstream_concurrency"] = map[string]any{"enabled": reduction}
			repository.policy["auto_apply"] = map[string]any{"concurrency": true, "schedulable": true}
			result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			if target := result.AccountTargets["41"]; target.Schedulable == nil || !*target.Schedulable {
				t.Fatalf("small weight noise evicted the incumbent from shared capacity: %+v", result.AccountTargets)
			}
		})
	}
}

func TestStablePlacementSharedCapacityWaiterDoesNotEvictActiveAccountByOldPriority(t *testing.T) {
	first, second := placementAccount("41", "1", 20), placementAccount("42", "0.95", 11)
	paused := false
	second.Schedulable, second.EffectiveState = &paused, "concurrency_limited"
	repository := placementFixture(first, second)
	kind, limit, concurrency := "sub2api", int64(1), int64(1)
	for index := range repository.accounts {
		account := &repository.accounts[index]
		account.UpstreamID, account.UpstreamType = "test-upstream", &kind
		account.UpstreamConcurrencyLimit, account.Concurrency = &limit, &concurrency
		account.UpstreamConcurrencyStatus = business.UpstreamConcurrencyKnown
		repository.addSuccesses(account.ID, 5, 1000, time.Second)
	}
	repository.policy["scaling"] = map[string]any{"enabled": true, "min_per_account": 1}
	repository.policy["auto_apply"] = map[string]any{"concurrency": true, "schedulable": true}
	result, err := routing.NewService(repository).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"]; target.Schedulable == nil || !*target.Schedulable {
		t.Fatalf("old priority or small price noise evicted the active account: %+v", result.AccountTargets)
	}
}
