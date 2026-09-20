package routing

import (
	"math"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

// GlobalScalingEnabled keeps the global scaling budget opt-in, including group
// overrides. Independent upstream allocation does not enable that budget.
func GlobalScalingEnabled(document map[string]any) bool {
	if scaling, ok := document["scaling"].(map[string]any); ok && scaling["enabled"] == true {
		return true
	}
	bindings, _ := document["group_policy_bindings"].(map[string]any)
	for _, raw := range bindings {
		binding, ok := raw.(map[string]any)
		if ok && binding["enabled"] != false && binding["scaling_enabled"] == true {
			return true
		}
	}
	return false
}

func confirmedManagedStop(account business.RoutingAccount, config engineConfig) bool {
	if !config.upstreamCapacityEnabledFor(account) || sub2APIUpstreamID(account) == "" || !releasableAutomaticStop(account.EffectiveState) ||
		account.Schedulable == nil || *account.Schedulable || !account.HasRoutingBaseline ||
		account.ManagedSchedulable == nil || *account.ManagedSchedulable || account.ExternalControl || account.Paused || account.ManualPriority != nil {
		return false
	}
	allowed, _ := eligibleScope(account, config)
	_, paused := config.pausedAccounts[account.ID]
	_, fused := config.manualFusedAccounts[account.ID]
	return allowed && !paused && !fused && accountMetadataManaged(account, config) && !accountExternallyModified(account)
}

func releaseConfirmedStopReservations(inventory []business.RoutingAccount, primary map[string]*candidate, configs map[string]engineConfig, budget *scalingBudget, pools upstreamScalingPools) {
	released := map[string]bool{}
	for id, item := range primary {
		if !item.schedulable && confirmedManagedStop(item.account, configs[item.account.GroupName]) {
			released[id] = true
		}
	}
	// Both inventory reads must confirm the same stopped state. A stale or
	// active observation must retain its reservation until a later round.
	for _, account := range inventory {
		if !releasableAutomaticStop(account.EffectiveState) || account.Schedulable == nil || *account.Schedulable {
			delete(released, account.ID)
		}
	}
	for id := range released {
		budget.reservations[id] = 0
		if pool := pools[sub2APIUpstreamID(primary[id].account)]; pool != nil {
			pool.current[id] = 0
		}
	}
	budget.allocated = 0
	for _, capacity := range budget.reservations {
		budget.allocated = saturatingCapacityAdd(budget.allocated, capacity)
	}
	budget.initialAllocated = budget.allocated
}

func (config engineConfig) upstreamCapacityEnabledFor(account business.RoutingAccount) bool {
	return config.scalingEnabled || config.upstreamAllocationEnabledFor(account)
}

// A former capacity pause outside allocation scope can recover through normal
// scheduling. Reserve its configured capacity before allocating selected peers.
func reserveOutsideAllocationScope(primary map[string]*candidate, configs map[string]engineConfig, budget *scalingBudget, pools upstreamScalingPools) {
	for id, item := range primary {
		if configs[item.account.GroupName].upstreamCapacityEnabledFor(item.account) || !confirmedConcurrencyPause(item.account) {
			continue
		}
		capacity := int64(math.MaxInt64)
		if item.account.Concurrency != nil && *item.account.Concurrency > 0 {
			capacity = *item.account.Concurrency
		}
		budget.reservations[id] = capacity
		if pool := pools[sub2APIUpstreamID(item.account)]; pool != nil {
			pool.current[id] = capacity
		}
	}
	budget.allocated = 0
	for _, capacity := range budget.reservations {
		budget.allocated = saturatingCapacityAdd(budget.allocated, capacity)
	}
	budget.initialAllocated = budget.allocated
}

// Capacity scope and managed-stop release use the same stable primary group as
// account-level calculation, rather than OR-ing conflicting group switches.
func UpstreamCapacityScope(document map[string]any, accounts []business.RoutingAccount) (map[string]bool, error) {
	return capacityPolicyMap(document, accounts, false)
}

func UpstreamManagedStopScope(document map[string]any, accounts []business.RoutingAccount) (map[string]bool, error) {
	return capacityPolicyMap(document, accounts, true)
}

func capacityPolicyMap(document map[string]any, accounts []business.RoutingAccount, release bool) (map[string]bool, error) {
	base, err := parseEngineConfig(document)
	if err != nil {
		return nil, err
	}
	result := map[string]bool{}
	primary := map[string]*candidate{}
	for _, account := range accounts {
		config, enabled, err := base.forGroup(account.GroupID)
		if err != nil {
			return nil, err
		}
		if !enabled {
			continue
		}
		item := &candidate{account: account}
		if current := primary[account.ID]; current != nil && !membershipLess(item, current) {
			continue
		}
		primary[account.ID] = item
		result[account.ID] = config.upstreamCapacityEnabledFor(account)
		if release {
			result[account.ID] = confirmedManagedStop(account, config)
		}
	}
	return result, nil
}

func NewAccountUpstreamCapacityEnabled(document map[string]any, upstreamID, groupID string, override *bool) (bool, error) {
	config, err := parseEngineConfig(document)
	if err != nil {
		return false, err
	}
	config, enabled, err := config.forGroup(&groupID)
	if err != nil || !enabled {
		return false, err
	}
	selected, _ := config.upstreamAllocationScope.Selected("", upstreamID)
	if override != nil {
		selected = *override
	}
	return config.scalingEnabled || config.upstreamReductionEnabled && selected, nil
}

// Only confirmed automatic stops release reservations; manual controls still
// retain their capacity and health recovery remains a separate decision.
func releasableAutomaticStop(state string) bool {
	return state == "cost_blocked" || state == "fused"
}

// NewAccountGlobalCapacityLimit applies global scaling only to accounts that
// participate through their own group or shared Sub2API allocation scope.
func NewAccountGlobalCapacityLimit(document map[string]any, upstreamID, groupID string, override *bool, sub2API bool) (*int64, error) {
	config, err := parseEngineConfig(document)
	if err != nil {
		return nil, err
	}
	config, enabled, err := config.forGroup(&groupID)
	if err != nil || !enabled {
		return nil, err
	}
	selected, _ := config.upstreamAllocationScope.Selected("", upstreamID)
	if override != nil {
		selected = *override
	}
	if !config.globalScalingEnabled || !(config.scalingEnabled || sub2API && config.upstreamReductionEnabled && selected) {
		return nil, nil
	}
	limit := config.scalingGlobalMax
	return &limit, nil
}
