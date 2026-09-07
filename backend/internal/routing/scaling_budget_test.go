package routing

import (
	"context"
	"errors"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestGlobalConcurrencyBudgetSharedAcrossPrimaryGroups(t *testing.T) {
	policy := routingPolicy()
	policy["scaling"] = map[string]any{"enabled": true, "global_max_concurrency": 100, "min_per_account": 1, "max_per_account": 100, "scale_up_ratio": 0.4, "step_up": 10}
	config, err := parseEngineConfig(policy)
	if err != nil {
		t.Fatal(err)
	}
	first, second := healthyTestCandidate("41", "codex", 100), healthyTestCandidate("42", "claude", 100)
	firstCurrent, secondCurrent := int64(45), int64(45)
	first.account.Concurrency, second.account.Concurrency = &firstCurrent, &secondCurrent
	groups := map[string][]*candidate{"codex": {first}, "claude": {second}}
	assignAccountPlacements(groups, map[string]engineConfig{"codex": config, "claude": config}, map[string][]*candidate{"41": {first}, "42": {second}})
	total := int64(0)
	for _, item := range []*candidate{first, second} {
		if item.desiredConcurrency != nil {
			total += *item.desiredConcurrency
		} else {
			total += *item.account.Concurrency
		}
	}
	if total > config.scalingGlobalMax {
		t.Fatalf("target total=%d exceeds global cap=%d (initial total=90)", total, config.scalingGlobalMax)
	}
}

func TestMinimumConcurrencyRespectsRemainingGlobalBudget(t *testing.T) {
	first, second := healthyTestCandidate("41", "codex", 100), healthyTestCandidate("42", "codex", 100)
	rank, current := 1, int64(1)
	first.rank, second.rank = &rank, &rank
	first.account.Concurrency, second.account.Concurrency = &current, &current
	applyScaling([]*candidate{first, second}, engineConfig{scalingEnabled: true, scalingGlobalMax: 5, scalingMin: 3, scalingMax: 10, scalingUpRatio: .8, scalingStepUp: 1})
	total := *first.desiredConcurrency + *second.desiredConcurrency
	if total > 5 {
		t.Fatalf("clamping to minimum exceeded budget: desired total=%d cap=5", total)
	}
}

type capacityInventoryRepository struct {
	routingRepositoryStub
	inventoryError    error
	scopedConcurrency *int64
}

func (r *capacityInventoryRepository) RoutingAccounts(_ context.Context, accountID, groupName *string) ([]business.RoutingAccount, error) {
	if accountID == nil && groupName == nil {
		return r.accounts, r.inventoryError
	}
	selected := []business.RoutingAccount{}
	for _, account := range r.accounts {
		if accountID != nil && account.ID != *accountID {
			continue
		}
		if groupName != nil && account.GroupName != *groupName {
			continue
		}
		if r.scopedConcurrency != nil {
			account.Concurrency = r.scopedConcurrency
		}
		selected = append(selected, account)
	}
	return selected, nil
}

func scopedCapacityRepository() *capacityInventoryRepository {
	policy := routingPolicy()
	policy["scaling"] = map[string]any{"enabled": true, "global_max_concurrency": 100, "min_per_account": 1, "max_per_account": 100, "scale_up_ratio": .4, "step_up": 10}
	enabled, multiplier := true, "1"
	current, manual, paused, manualPriority := int64(45), int64(30), int64(20), int64(1)
	return &capacityInventoryRepository{routingRepositoryStub: routingRepositoryStub{
		policy: policy,
		accounts: []business.RoutingAccount{
			{ID: "41", GroupName: "codex", Concurrency: &current, Schedulable: &enabled, Multiplier: &multiplier, Metadata: map[string]any{}},
			{ID: "42", GroupName: "manual", Concurrency: &manual, ManualPriority: &manualPriority, Schedulable: &enabled, Multiplier: &multiplier, Metadata: map[string]any{}},
			{ID: "43", GroupName: "paused", Concurrency: &paused, Paused: true, Schedulable: &enabled, Multiplier: &multiplier, Metadata: map[string]any{}},
			{ID: "43", GroupName: "paused-copy", Concurrency: &paused, Paused: true, Schedulable: &enabled, Multiplier: &multiplier, Metadata: map[string]any{}},
		},
	}}
}

func TestScopedScalingReservesOtherAccountsWithoutWritingOutsideScope(t *testing.T) {
	accountID, groupName := "41", "codex"
	for name, scope := range map[string]Scope{"account": {AccountID: &accountID}, "group": {GroupName: &groupName}} {
		t.Run(name, func(t *testing.T) {
			repository := scopedCapacityRepository()
			result, err := NewService(repository).Calculate(context.Background(), scope, true)
			if err != nil {
				t.Fatal(err)
			}
			target := result.AccountTargets["41"]
			if target.Concurrency == nil || *target.Concurrency != 50 {
				t.Fatalf("45 configured plus 50 protected capacity should leave only 5 additional slots, got %#v", target)
			}
			if len(repository.targets) != 1 || repository.targets[0].AccountID != "41" {
				t.Fatalf("global inventory expanded scoped writes: %#v", repository.targets)
			}
		})
	}
}

func TestScopedScalingRejectsUnreadableGlobalCapacityBeforePersisting(t *testing.T) {
	repository := scopedCapacityRepository()
	repository.inventoryError = errors.New("inventory unavailable")
	accountID := "41"
	_, err := NewService(repository).Calculate(context.Background(), Scope{AccountID: &accountID}, true)
	if !errors.Is(err, repository.inventoryError) {
		t.Fatalf("unverified global capacity was accepted: %v", err)
	}
	if len(repository.targets) != 0 {
		t.Fatalf("persisted unsafe targets: %#v", repository.targets)
	}
}

func TestScalingInventoryCannotLowerTheSelectedAccountBaseline(t *testing.T) {
	repository := scopedCapacityRepository()
	current, inventoryCurrent := int64(45), int64(40)
	repository.scopedConcurrency = &current
	repository.accounts[0].Concurrency = &inventoryCurrent
	accountID := "41"
	result, err := NewService(repository).Calculate(context.Background(), Scope{AccountID: &accountID}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"].Concurrency; target == nil || *target != 50 {
		t.Fatalf("inventory reduced the selected baseline and created false headroom: %v", target)
	}
}

func TestMinimumBudgetConflictIsReportedWithoutExceedingCapacity(t *testing.T) {
	repository := scopedCapacityRepository()
	policy := repository.policy["scaling"].(map[string]any)
	policy["global_max_concurrency"], policy["min_per_account"], policy["scale_up_ratio"] = 5, 3, .8
	current := int64(1)
	first := repository.accounts[0]
	first.Concurrency = &current
	second := first
	second.ID = "42"
	repository.accounts = []business.RoutingAccount{first, second}
	result, err := NewService(repository).Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ConfigurationErrors) != 1 {
		t.Fatalf("minimum conflict was hidden: %#v", result)
	}
	total := int64(0)
	for _, target := range result.AccountTargets {
		if target.Concurrency == nil {
			total += current
		} else {
			total += *target.Concurrency
		}
	}
	if total > 5 {
		t.Fatalf("target total=%d exceeds cap=5", total)
	}
}

func TestScalingDisabledGroupReservesItsExistingCapacity(t *testing.T) {
	first, second := healthyTestCandidate("41", "codex", 100), healthyTestCandidate("42", "claude", 100)
	current, protected := int64(45), int64(50)
	first.account.Concurrency, second.account.Concurrency = &current, &protected
	config := engineConfig{scalingEnabled: true, scalingGlobalMax: 100, scalingMin: 1, scalingMax: 100, scalingUpRatio: .4, scalingStepUp: 10}
	disabled := config
	disabled.scalingEnabled = false
	assignAccountPlacements(map[string][]*candidate{"codex": {first}, "claude": {second}}, map[string]engineConfig{"codex": config, "claude": disabled}, map[string][]*candidate{"41": {first}, "42": {second}})
	if first.desiredConcurrency == nil || *first.desiredConcurrency != 50 || second.desiredConcurrency != nil {
		t.Fatalf("disabled group capacity was reused or modified: first=%v second=%v", first.desiredConcurrency, second.desiredConcurrency)
	}
}

func TestScalingDoesNotSpendUnconfirmedReductions(t *testing.T) {
	current, rank := int64(50), 1
	first, second := healthyTestCandidate("41", "codex", 100), healthyTestCandidate("42", "codex", 100)
	first.account.Concurrency, second.account.Concurrency = &current, &current
	first.rank, second.rank = &rank, &rank
	first.state = "degraded"
	applyScaling([]*candidate{first, second}, engineConfig{scalingEnabled: true, scalingGlobalMax: 100, scalingMin: 1, scalingMax: 100, scalingUpRatio: .4, scalingStepUp: 10, scalingStepDown: 10})
	if first.desiredConcurrency == nil || *first.desiredConcurrency != 40 || second.desiredConcurrency != nil {
		t.Fatalf("a proposed reduction funded another account before write confirmation: first=%v second=%v", first.desiredConcurrency, second.desiredConcurrency)
	}
}
