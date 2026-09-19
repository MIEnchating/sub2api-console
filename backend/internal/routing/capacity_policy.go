package routing

import "github.com/MIEnchating/sub2api-console/backend/internal/business"

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

// UpstreamCostPauseReleasesCapacity permits reuse of an in-scope cost stop that
// Console previously confirmed. This is not authorization to recover that account.
func UpstreamCostPauseReleasesCapacity(document map[string]any, account business.RoutingAccount) (bool, error) {
	config, err := parseEngineConfig(document)
	if err != nil {
		return false, err
	}
	config, enabled, err := config.forGroup(account.GroupID)
	if err != nil || !enabled {
		return false, err
	}
	return confirmedManagedCostPause(account, config), nil
}

func confirmedManagedCostPause(account business.RoutingAccount, config engineConfig) bool {
	if !config.upstreamAllocationEnabledFor(account) || sub2APIUpstreamID(account) == "" || account.EffectiveState != "cost_blocked" ||
		account.Schedulable == nil || *account.Schedulable || !account.HasRoutingBaseline ||
		account.ManagedSchedulable == nil || *account.ManagedSchedulable || account.ExternalControl || account.Paused || account.ManualPriority != nil {
		return false
	}
	allowed, _ := eligibleScope(account, config)
	_, paused := config.pausedAccounts[account.ID]
	_, fused := config.manualFusedAccounts[account.ID]
	return allowed && !paused && !fused && accountMetadataManaged(account, config) && !accountExternallyModified(account)
}

func releaseConfirmedCostReservations(inventory []business.RoutingAccount, primary map[string]*candidate, configs map[string]engineConfig, budget *scalingBudget, pools upstreamScalingPools) {
	released := map[string]bool{}
	for id, item := range primary {
		if confirmedManagedCostPause(item.account, configs[item.account.GroupName]) {
			released[id] = true
		}
	}
	// Both inventory reads must confirm the same stopped state. A stale or
	// active observation must retain its reservation until a later round.
	for _, account := range inventory {
		if account.EffectiveState != "cost_blocked" || account.Schedulable == nil || *account.Schedulable {
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
