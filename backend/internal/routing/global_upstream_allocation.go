package routing

import "math"

// Plan the global shares before processing individual upstreams. Current
// reservations still gate actual growth until reductions have been confirmed.
func planGlobalUpstreamShares(primary map[string]*candidate, configs map[string]engineConfig, pools upstreamScalingPools, budget *scalingBudget) {
	selected := map[string]bool{}
	members := map[string][]*candidate{}
	globalLimit := int64(math.MaxInt64)
	for _, item := range primary {
		config := configs[item.account.GroupName]
		pool := pools[sub2APIUpstreamID(item.account)]
		if !config.globalScalingEnabled || !config.upstreamCapacityEnabledFor(item.account) || pool == nil || !pools.manages(item.account) || pool.unknown || pool.stale || pool.limit == nil || !item.schedulable || !placementLoadFactorEligible(item) || item.account.ManualPriority != nil || item.account.Paused || item.account.Concurrency == nil {
			continue
		}
		if config.scalingEnabled && item.evidencePending {
			continue
		}
		if !config.scalingEnabled && !remoteSchedulable(item.account) && (!confirmedConcurrencyPause(item.account) || item.evidencePending) {
			continue
		}
		selected[item.account.ID] = true
		members[pool.id] = append(members[pool.id], item)
		globalLimit = min(globalLimit, config.scalingGlobalMax)
	}
	if len(members) < 2 {
		return
	}
	available := max(int64(0), globalLimit-budget.reservedOutside(selected))
	items := []*candidate{}
	ceilings := map[string]int64{}
	for id, group := range members {
		pool := pools[id]
		reserved := int64(0)
		for accountID, current := range pool.current {
			if !selected[accountID] {
				reserved = saturatingCapacityAdd(reserved, current)
			}
		}
		ceiling := int64(math.MaxInt64)
		if *pool.limit > 0 {
			ceiling = max(int64(0), *pool.limit-reserved)
		}
		ceilings[id] = ceiling
		items = append(items, group...)
	}
	sortCapacityWithHysteresis(items, configs)
	shares := map[string]int64{}
	for id := range members {
		// Zero is a planned share too; it must not fall back to allocating
		// capacity already assigned to a healthier account in another pool.
		shares[id] = 0
	}
	accountShares := map[string]int64{}
	minimums := map[string]int64{}
	minimumTotal := int64(0)
	for id, group := range members {
		total := int64(0)
		for _, item := range group {
			value := int64(1)
			if config := configs[item.account.GroupName]; config.scalingEnabled {
				value = config.scalingMin
			}
			minimums[item.account.ID] = value
			total = saturatingCapacityAdd(total, value)
		}
		if total > ceilings[id] {
			total = int64(len(group))
			for _, item := range group {
				minimums[item.account.ID] = 1
			}
		}
		minimumTotal = saturatingCapacityAdd(minimumTotal, total)
	}
	minimumsFit := minimumTotal <= available
	// Honor configured minima when feasible; otherwise protect one per account.
	for _, item := range items {
		id := sub2APIUpstreamID(item.account)
		minimum := minimums[item.account.ID]
		if !minimumsFit {
			minimum = 1
		}
		if available >= minimum && ceilings[id]-shares[id] >= minimum {
			shares[id] += minimum
			accountShares[item.account.ID] = minimum
			available -= minimum
		}
	}
	for available > 0 {
		active := []*candidate{}
		for _, item := range items {
			config := configs[item.account.GroupName]
			maximum := config.scalingMax
			if !config.scalingEnabled {
				maximum = ceilings[sub2APIUpstreamID(item.account)]
			}
			if accountShares[item.account.ID] > 0 && accountShares[item.account.ID] < maximum && shares[sub2APIUpstreamID(item.account)] < ceilings[sub2APIUpstreamID(item.account)] {
				active = append(active, item)
			}
		}
		if len(active) == 0 {
			break
		}
		extra := boundedConcurrencyQuotas(active, available+int64(len(active)), func(item *candidate) (int64, int64) {
			config := configs[item.account.GroupName]
			id := sub2APIUpstreamID(item.account)
			maximum := config.scalingMax
			if !config.scalingEnabled {
				maximum = ceilings[id]
			}
			return 1, 1 + min(maximum-accountShares[item.account.ID], ceilings[id]-shares[id])
		})
		used := int64(0)
		for _, item := range active {
			id := sub2APIUpstreamID(item.account)
			increase := min(extra[item.account.ID]-1, ceilings[id]-shares[id], available)
			shares[id] += increase
			accountShares[item.account.ID] += increase
			available -= increase
			used += increase
		}
		if used == 0 {
			break
		}
	}
	budget.upstreamShares = shares
}
