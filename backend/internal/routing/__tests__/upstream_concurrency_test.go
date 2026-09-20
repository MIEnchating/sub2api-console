package routing_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

type upstreamCapacityRepository struct {
	*business.Store
	accounts []business.RoutingAccount
	previous []business.PreviousRoutingDecision
	samples  []business.RoutingSample
}

func (r *upstreamCapacityRepository) RoutingCapacityAccounts(context.Context) ([]business.RoutingAccount, error) {
	return r.accounts, nil
}

func (r *upstreamCapacityRepository) RoutingAccounts(_ context.Context, accountID, groupName *string) ([]business.RoutingAccount, error) {
	var selected []business.RoutingAccount
	for _, account := range r.accounts {
		if accountID != nil && account.ID != *accountID || groupName != nil && account.GroupName != *groupName {
			continue
		}
		selected = append(selected, account)
	}
	return selected, nil
}

func (r *upstreamCapacityRepository) PreviousRoutingDecisions(context.Context, *string, *string) ([]business.PreviousRoutingDecision, error) {
	return r.previous, nil
}

func (r *upstreamCapacityRepository) RoutingSamples(context.Context, *string, *string, string, int) ([]business.RoutingSample, error) {
	return r.samples, nil
}

func (r *upstreamCapacityRepository) PersistRoutingRound(context.Context, *string, *string, []business.RoutingEvaluationWrite, []business.RoutingDecisionWrite, []business.AccountRoutingTarget, []business.CleanupStateWrite, []business.RuntimeEventWrite, bool, time.Time) error {
	return nil
}

func upstreamCapacityFixture(t *testing.T, accounts ...business.RoutingAccount) *upstreamCapacityRepository {
	t.Helper()
	store, err := business.Open(filepath.Join(t.TempDir(), "upstream-capacity.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err = store.UpdatePolicy(context.Background(), map[string]any{"advanced_policy": map[string]any{
		"scaling": map[string]any{"enabled": true, "global_max_concurrency": 100, "min_per_account": 1, "max_per_account": 100, "step_up": 100, "step_down": 100},
	}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	return &upstreamCapacityRepository{Store: store, accounts: accounts}
}

func upstreamCapacityAccount(id string, current, limit int64) business.RoutingAccount {
	upstreamID, upstreamType, group, multiplier, enabled := "upstream-1", "sub2api", "7", "1", true
	return business.RoutingAccount{ID: id, Name: id, GroupName: "codex", GroupID: &group, Schedulable: &enabled,
		UpstreamID: upstreamID, UpstreamType: &upstreamType, UpstreamConcurrencyLimit: &limit, Concurrency: &current,
		Multiplier: &multiplier, Metadata: map[string]any{}}
}

func TestUpstreamConcurrencyDistributesFiniteCapacityByStableAccountID(t *testing.T) {
	first, second, third := upstreamCapacityAccount("41", 1, 10), upstreamCapacityAccount("42", 1, 10), upstreamCapacityAccount("43", 1, 10)
	repository := upstreamCapacityFixture(t, third, second, first)
	result, err := routing.NewService(repository).Calculate(context.Background(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	for id, expected := range map[string]int64{"41": 4, "42": 3, "43": 3} {
		if target := result.AccountTargets[id].Concurrency; target == nil || *target != expected {
			t.Fatalf("account %s should receive %d slots from a shared limit of 10: %+v", id, expected, result.AccountTargets)
		}
	}
}

func TestUpstreamConcurrencyReservesManualPausedAndOutOfScopeAccountsOnce(t *testing.T) {
	first, manual, paused, outside := upstreamCapacityAccount("41", 1, 10), upstreamCapacityAccount("42", 2, 10), upstreamCapacityAccount("43", 2, 10), upstreamCapacityAccount("44", 2, 10)
	priority := int64(1)
	manual.ManualPriority = &priority
	paused.Paused = true
	copy := outside
	copy.GroupName = "claude"
	repository := upstreamCapacityFixture(t, first, manual, paused, outside, copy)
	id := "41"
	result, err := routing.NewService(repository).Calculate(context.Background(), routing.Scope{AccountID: &id}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets[id].Concurrency; target == nil || *target != 4 || len(result.AccountTargets) != 1 {
		t.Fatalf("protected accounts must reserve six slots without expanding write scope: %+v", result.AccountTargets)
	}
}

func TestUpstreamConcurrencyDoesNotSpendReductionBeforeReadback(t *testing.T) {
	first, second := upstreamCapacityAccount("41", 8, 10), upstreamCapacityAccount("42", 2, 10)
	repository := upstreamCapacityFixture(t, first, second)
	result, err := routing.NewService(repository).Calculate(context.Background(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"].Concurrency; target == nil || *target != 5 {
		t.Fatalf("oversized account should release three slots: %+v", result.AccountTargets)
	}
	if target := result.AccountTargets["42"].Concurrency; target != nil {
		t.Fatalf("unconfirmed reduction funded an increase: %v", *target)
	}
	repository.accounts[0].Concurrency = result.AccountTargets["41"].Concurrency
	result, err = routing.NewService(repository).Calculate(context.Background(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["42"].Concurrency; target == nil || *target != 5 {
		t.Fatalf("confirmed free slots were not assigned next round: %+v", result.AccountTargets)
	}
}

func TestUpstreamConcurrencyUnknownCapacityPreservesCurrentSetting(t *testing.T) {
	account := upstreamCapacityAccount("41", 90, 100)
	account.UpstreamConcurrencyLimit = nil
	repository := upstreamCapacityFixture(t, account)
	result, err := routing.NewService(repository).Calculate(context.Background(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"].Concurrency; target != nil {
		t.Fatalf("unknown upstream capacity must not produce a concurrency write: %v", *target)
	}
	if !strings.Contains(result.AccountDecisions["41"].Reason, "上游并发") {
		t.Fatalf("missing capacity was not explained: %+v", result.AccountDecisions["41"])
	}
}

func TestUpstreamConcurrencyInsufficientCapacityNeverWritesUnlimitedZero(t *testing.T) {
	repository := upstreamCapacityFixture(t, upstreamCapacityAccount("41", 2, 1), upstreamCapacityAccount("42", 2, 1))
	result, err := routing.NewService(repository).Calculate(context.Background(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if decision := result.AccountDecisions["42"]; decision.Schedulable || decision.RoutingState != "concurrency_limited" {
		t.Fatalf("the lower priority account must pause when the limit is below the number of accounts: %+v", decision)
	}
	for _, target := range result.AccountTargets {
		if target.Concurrency != nil && *target.Concurrency < 1 {
			t.Fatalf("zero disables the remote limiter and must never be assigned: %+v", target)
		}
	}
}

func TestUpstreamConcurrencyUsesSchedulingStrategyToGiveCheaperAccountsMoreCapacity(t *testing.T) {
	first, second := upstreamCapacityAccount("41", 1, 16), upstreamCapacityAccount("42", 1, 16)
	expensive := "4"
	second.Multiplier = &expensive
	repository := upstreamCapacityFixture(t, first, second)
	if _, err := repository.UpdatePolicy(context.Background(), map[string]any{"global_strategy": "price_first"}, "test"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"41", "42"} {
		repository.samples = append(repository.samples, business.RoutingSample{AccountID: id, GroupName: "codex", Source: "traffic", Result: "通过", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Payload: map[string]any{"input_tokens": 10, "output_tokens": 5}})
	}
	result, err := routing.NewService(repository).Calculate(context.Background(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	firstTarget, secondTarget := result.AccountTargets["41"].Concurrency, result.AccountTargets["42"].Concurrency
	if firstTarget == nil || secondTarget == nil || *firstTarget != 11 || *secondTarget != 5 {
		t.Fatalf("price-first weights must influence the 16 available slots: targets=%+v decisions=%+v", result.AccountTargets, result.AccountDecisions)
	}
}

func TestUpstreamConcurrencyConfirmedPauseWaitsForSafeCapacityThenResumesWithoutOscillation(t *testing.T) {
	first, second := upstreamCapacityAccount("41", 2, 1), upstreamCapacityAccount("42", 2, 1)
	repository := upstreamCapacityFixture(t, first, second)
	if _, err := repository.UpdatePolicy(context.Background(), map[string]any{"auto_apply": map[string]any{"schedulable": true, "concurrency": true}}, "test"); err != nil {
		t.Fatal(err)
	}
	service := routing.NewService(repository)
	for round := 0; round < 3; round++ {
		result, err := service.Calculate(context.Background(), routing.Scope{}, true)
		if err != nil {
			t.Fatal(err)
		}
		if decision := result.AccountDecisions["42"]; decision.Schedulable || decision.RoutingState != "concurrency_limited" {
			t.Fatalf("round %d: insufficient capacity must keep the lower priority account paused: %+v", round, decision)
		}
		applyUpstreamCapacityReadback(repository, result)
	}
	limit := int64(2)
	for i := range repository.accounts {
		repository.accounts[i].UpstreamConcurrencyLimit = &limit
	}
	result, err := service.Calculate(context.Background(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["42"]; target.Schedulable == nil || *target.Schedulable || target.Concurrency == nil || *target.Concurrency != 1 {
		t.Fatalf("the retained limit of 2 must first be reduced and read back while paused: %+v", target)
	}
	applyUpstreamCapacityReadback(repository, result)
	for round := 0; round < 3; round++ {
		result, err = service.Calculate(context.Background(), routing.Scope{}, true)
		if err != nil {
			t.Fatal(err)
		}
		if target := result.AccountTargets["42"]; target.Schedulable == nil || !*target.Schedulable || target.DesiredHealth == "concurrency_limited" {
			t.Fatalf("round %d: confirmed capacity must resume and keep both accounts available: %+v", round, target)
		}
		applyUpstreamCapacityReadback(repository, result)
	}
}

func applyUpstreamCapacityReadback(repository *upstreamCapacityRepository, result routing.Result) {
	for i := range repository.accounts {
		target, found := result.AccountTargets[repository.accounts[i].ID]
		if !found {
			continue
		}
		if target.Concurrency != nil {
			repository.accounts[i].Concurrency = target.Concurrency
		}
		if target.Schedulable != nil {
			repository.accounts[i].Schedulable = target.Schedulable
		}
		repository.accounts[i].EffectiveState = target.DesiredHealth
	}
}

func TestUpstreamConcurrencyRecoveryRequiresBothAutomaticWriteSwitches(t *testing.T) {
	for _, flags := range []struct {
		name        string
		schedulable bool
		concurrency bool
	}{
		{name: "both disabled"},
		{name: "concurrency disabled", schedulable: true},
		{name: "schedulable disabled", concurrency: true},
	} {
		t.Run(flags.name, func(t *testing.T) {
			account := upstreamCapacityAccount("41", 1, 10)
			disabled := false
			account.Schedulable, account.EffectiveState = &disabled, "concurrency_limited"
			repository := upstreamCapacityFixture(t, account)
			if _, err := repository.UpdatePolicy(context.Background(), map[string]any{"auto_apply": map[string]any{"schedulable": flags.schedulable, "concurrency": flags.concurrency}}, "test"); err != nil {
				t.Fatal(err)
			}
			result, err := routing.NewService(repository).Calculate(context.Background(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			if target := result.AccountTargets["41"]; target.Schedulable == nil || *target.Schedulable || target.DesiredHealth != "concurrency_limited" {
				t.Fatalf("unconfirmed automatic write combination must not restore traffic: %+v", target)
			}
		})
	}
}

func TestUpstreamConcurrencyBudgetRemainsBoundedByGlobalCapacity(t *testing.T) {
	first, unrelated := upstreamCapacityAccount("41", 1, 20), upstreamCapacityAccount("42", 90, 100)
	unrelated.UpstreamID = "upstream-2"
	unrelated.GroupName, unrelated.GroupID = "", nil
	repository := upstreamCapacityFixture(t, first, unrelated)
	id := "41"
	result, err := routing.NewService(repository).Calculate(context.Background(), routing.Scope{AccountID: &id}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets[id].Concurrency; target == nil || *target != 10 {
		t.Fatalf("the ungrouped account must reserve 90 global slots: %+v", result.AccountTargets)
	}
}

func TestUpstreamConcurrencyCooldownCannotFundAnotherAccountsExpansion(t *testing.T) {
	repository := upstreamCapacityFixture(t, upstreamCapacityAccount("41", 8, 10), upstreamCapacityAccount("42", 2, 10))
	repository.previous = []business.PreviousRoutingDecision{{AccountID: "41", GroupName: "codex", State: "unknown", LastApplyAt: time.Now().UTC()}}
	result, err := routing.NewService(repository).Calculate(context.Background(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.AccountTargets["41"].Concurrency != nil || result.AccountTargets["42"].Concurrency != nil {
		t.Fatalf("a cooldown-suppressed reduction cannot be spent: %+v", result.AccountTargets)
	}
}

func TestUpstreamConcurrencySharedAccountAcrossGroupsReceivesOneQuota(t *testing.T) {
	first, second := upstreamCapacityAccount("41", 1, 10), upstreamCapacityAccount("42", 1, 10)
	copy := first
	group := "8"
	copy.GroupName, copy.GroupID = "claude", &group
	repository := upstreamCapacityFixture(t, first, second, copy)
	result, err := routing.NewService(repository).Calculate(context.Background(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.AccountTargets) != 2 || len(result.AccountTargets["41"].GroupNames) != 2 {
		t.Fatalf("group memberships must share one stable account target: %+v", result.AccountTargets)
	}
	firstTarget, secondTarget := result.AccountTargets["41"].Concurrency, result.AccountTargets["42"].Concurrency
	if firstTarget == nil || secondTarget == nil || *firstTarget+*secondTarget != 10 {
		t.Fatalf("duplicate memberships consumed capacity twice: %+v", result.AccountTargets)
	}
}

func TestUpstreamConcurrencyUnknownProtectedAccountPreventsExpansion(t *testing.T) {
	first, other := upstreamCapacityAccount("41", 1, 10), upstreamCapacityAccount("42", 1, 10)
	other.Concurrency = nil
	repository := upstreamCapacityFixture(t, first, other)
	id := "41"
	result, err := routing.NewService(repository).Calculate(context.Background(), routing.Scope{AccountID: &id}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets[id]; target.Concurrency != nil || target.Schedulable == nil || *target.Schedulable {
		t.Fatalf("an unknown protected allocation must not count as free capacity: %+v", target)
	}
}

func TestUpstreamConcurrencyUnlimitedUserLimitRetainsGlobalCap(t *testing.T) {
	repository := upstreamCapacityFixture(t, upstreamCapacityAccount("41", 90, 0))
	result, err := routing.NewService(repository).Calculate(context.Background(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"]; target.Schedulable == nil || !*target.Schedulable || target.Concurrency == nil || *target.Concurrency != 100 {
		t.Fatalf("a zero user limit means unlimited upstream capacity, while the global cap still applies: %+v", target)
	}
}

func TestUpstreamConcurrencyRecoveryKeepsManualAndCostWallBlocks(t *testing.T) {
	for _, scenario := range []string{"manual", "cost-wall"} {
		t.Run(scenario, func(t *testing.T) {
			account := upstreamCapacityAccount("41", 1, 10)
			disabled := false
			account.Schedulable, account.EffectiveState = &disabled, "concurrency_limited"
			if scenario == "manual" {
				account.Paused = true
			} else {
				wall := "0.5"
				account.GroupCostWall = &wall
			}
			accounts := []business.RoutingAccount{account}
			if scenario == "cost-wall" {
				peer := upstreamCapacityAccount("42", 1, 10)
				affordable := "0.1"
				peer.Multiplier = &affordable
				accounts = append(accounts, peer)
			}
			repository := upstreamCapacityFixture(t, accounts...)
			if _, err := repository.UpdatePolicy(context.Background(), map[string]any{"auto_apply": map[string]any{"schedulable": true, "concurrency": true}}, "test"); err != nil {
				t.Fatal(err)
			}
			result, err := routing.NewService(repository).Calculate(context.Background(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			if target := result.AccountTargets["41"]; target.Schedulable == nil || *target.Schedulable || target.DesiredHealth == "concurrency_limited" {
				t.Fatalf("capacity recovery must preserve the higher priority manual or cost-wall state: %+v", target)
			}
		})
	}
}

func TestUpstreamConcurrencyDisabledCapacityControlsRestoreOrdinaryScheduling(t *testing.T) {
	account := upstreamCapacityAccount("41", 1, 10)
	disabled := false
	account.Schedulable, account.EffectiveState = &disabled, "concurrency_limited"
	repository := upstreamCapacityFixture(t, account)
	if _, err := repository.UpdatePolicy(context.Background(), map[string]any{"advanced_policy": map[string]any{"scaling": map[string]any{"enabled": false}}}, "test"); err != nil {
		t.Fatal(err)
	}
	result, err := routing.NewService(repository).Calculate(context.Background(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"]; target.Concurrency != nil || target.Schedulable == nil || !*target.Schedulable || target.DesiredHealth == "concurrency_limited" {
		t.Fatalf("disabled capacity controls must permit ordinary recovery: %+v", target)
	}
}

func TestUpstreamConcurrencyNewlyEnabledAccountAlsoRequiresBothWriteSwitches(t *testing.T) {
	account := upstreamCapacityAccount("41", 1, 10)
	disabled := false
	account.Schedulable = &disabled
	repository := upstreamCapacityFixture(t, account)
	result, err := routing.NewService(repository).Calculate(context.Background(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"]; target.Schedulable == nil || *target.Schedulable {
		t.Fatalf("a fresh enable target cannot bypass the concurrency auto-write switch: %+v", target)
	}
}

func TestUpstreamConcurrencyStaleObservationCannotExpandOrResume(t *testing.T) {
	for _, paused := range []bool{false, true} {
		name := "active account"
		if paused {
			name = "capacity paused account"
		}
		t.Run(name, func(t *testing.T) {
			account := upstreamCapacityAccount("41", 1, 10)
			account.UpstreamConcurrencyStatus = "stale"
			if paused {
				disabled := false
				account.Schedulable, account.EffectiveState = &disabled, "concurrency_limited"
			}
			repository := upstreamCapacityFixture(t, account)
			if _, err := repository.UpdatePolicy(context.Background(), map[string]any{"auto_apply": map[string]any{"schedulable": true, "concurrency": true}}, "test"); err != nil {
				t.Fatal(err)
			}
			result, err := routing.NewService(repository).Calculate(context.Background(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			target := result.AccountTargets["41"]
			if target.Concurrency != nil || target.Schedulable == nil || *target.Schedulable == paused {
				t.Fatalf("stale upstream data must not authorize new capacity: %+v", target)
			}
		})
	}
}

func TestUpstreamConcurrencyPartialAutomaticWriteFlagsKeepPreviewButBlockExecution(t *testing.T) {
	for _, flags := range []struct {
		name        string
		schedulable bool
		concurrency bool
		blocked     bool
	}{
		{name: "only concurrency enabled", concurrency: true, blocked: true},
		{name: "only schedulable enabled", schedulable: true, blocked: true},
		{name: "both disabled", blocked: true},
		{name: "both enabled", schedulable: true, concurrency: true},
	} {
		t.Run(flags.name, func(t *testing.T) {
			finite, unlimited := upstreamCapacityAccount("41", 1, 10), upstreamCapacityAccount("42", 1, 0)
			unlimited.UpstreamID = "upstream-2"
			repository := upstreamCapacityFixture(t, finite, unlimited)
			if _, err := repository.UpdatePolicy(context.Background(), map[string]any{"auto_apply": map[string]any{"schedulable": flags.schedulable, "concurrency": flags.concurrency}}, "test"); err != nil {
				t.Fatal(err)
			}
			result, err := routing.NewService(repository).Calculate(context.Background(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			target := result.AccountTargets["41"]
			if target.Concurrency == nil || *target.Concurrency != 10 {
				t.Fatalf("incomplete write switches must retain the computed capacity preview: %+v", target)
			}
			if (target.ConfigurationError != nil) != flags.blocked {
				t.Fatalf("finite capacity allocation requires both write switches: %+v", target)
			}
			if (len(result.ConfigurationErrors) > 0) != flags.blocked {
				t.Fatalf("round result must explain incomplete write configuration: %+v", result.ConfigurationErrors)
			}
			if flags.blocked && (!strings.Contains(*target.ConfigurationError, "并发") || !strings.Contains(*target.ConfigurationError, "可调度")) {
				t.Fatalf("the correction must explain both required switches: %s", *target.ConfigurationError)
			}
			if result.AccountTargets["42"].ConfigurationError != nil {
				t.Fatal("unlimited upstreams must retain their existing independent write controls")
			}
		})
	}
}

func TestUpstreamConcurrencyEqualWeightUsesNumericStableIDsForSurvivalAndRemainders(t *testing.T) {
	for _, limit := range []int64{1, 5} {
		name := "survival selection"
		if limit == 5 {
			name = "integer remainder"
		}
		t.Run(name, func(t *testing.T) {
			repository := upstreamCapacityFixture(t, upstreamCapacityAccount("10", 1, limit), upstreamCapacityAccount("2", 1, limit))
			result, err := routing.NewService(repository).Calculate(context.Background(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			if limit == 1 {
				if !result.AccountDecisions["2"].Schedulable || result.AccountDecisions["10"].Schedulable {
					t.Fatalf("numeric ID 2 must remain ahead of ID 10 when weights tie: %+v", result.AccountDecisions)
				}
				return
			}
			if target := result.AccountTargets["2"].Concurrency; target == nil || *target != 3 {
				t.Fatalf("numeric ID 2 must receive the tied integer remainder: %+v", result.AccountTargets)
			}
		})
	}
}
