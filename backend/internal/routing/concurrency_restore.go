package routing

import "fmt"

// Turning off capacity control must also undo a confirmed scheduler reduction.
// A small configured value alone is not evidence of a reduction: only restore
// a known positive baseline while the current value still matches our write.
func restoreUnconstrainedConcurrency(primary map[string]*candidate, configs map[string]engineConfig, budget *scalingBudget, pools upstreamScalingPools) {
	for id, item := range primary {
		account := item.account
		if configs[account.GroupName].upstreamCapacityEnabledFor(account) || !item.schedulable ||
			!remoteSchedulable(account) || !placementLoadFactorEligible(item) || account.Paused ||
			account.ManualPriority != nil || account.ExternalControl || !account.HasRoutingBaseline ||
			account.BaselineConcurrency == nil || account.ManagedConcurrency == nil || account.Concurrency == nil ||
			*account.Concurrency <= 0 || *account.Concurrency != *account.ManagedConcurrency ||
			*account.BaselineConcurrency <= *account.Concurrency {
			continue
		}
		item.desiredConcurrency = cloneInt64(account.BaselineConcurrency)
		item.restoreConcurrency = true
		appendConcurrencyReason(item, fmt.Sprintf("并发控制已关闭，恢复接管前并发 %d；负载仍按调权策略分配", *item.desiredConcurrency))
		// Reserve the restoration before allocating selected peers, including
		// other groups sharing the global budget. Never spend this old reduction.
		budget.reservations[id] = max(budget.reservations[id], *item.desiredConcurrency)
		if pool := pools[sub2APIUpstreamID(account)]; pool != nil {
			pool.current[id] = max(pool.current[id], *item.desiredConcurrency)
		}
	}
	budget.allocated = 0
	for _, capacity := range budget.reservations {
		budget.allocated = saturatingCapacityAdd(budget.allocated, capacity)
	}
	budget.initialAllocated = budget.allocated
}
