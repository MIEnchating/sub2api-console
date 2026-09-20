package routing

import (
	"math"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func (config engineConfig) upstreamAllocationEnabledFor(account business.RoutingAccount) bool {
	if account.UpstreamType == nil || !strings.EqualFold(strings.TrimSpace(*account.UpstreamType), "sub2api") {
		return false
	}
	selected, _ := config.upstreamAllocationScope.Selected(account.ID, sub2APIUpstreamID(account))
	return config.upstreamReductionEnabled && selected
}

// UpstreamAllocationAllowed retains scope protection while permitting recovery
// only for accounts whose capacity pause was read back and saved by Console.
func UpstreamAllocationAllowed(document map[string]any, account business.RoutingAccount) (bool, error) {
	if account.Schedulable == nil || !*account.Schedulable && !confirmedConcurrencyPause(account) {
		return false, nil
	}
	active := true
	account.Schedulable = &active
	return UpstreamReductionAllowed(document, account)
}

func (pool *upstreamScalingPool) allocateShared(primary map[string]*candidate, configs map[string]engineConfig, global *scalingBudget) bool {
	if pool.unknown || pool.stale || pool.limit == nil || *pool.limit < 0 {
		return false
	}
	for _, account := range pool.accounts {
		if account.UpstreamConcurrencyStatus != business.UpstreamConcurrencyKnown && !(account.UpstreamConcurrencyStatus == business.UpstreamConcurrencyUnlimited && *pool.limit == 0) {
			return false
		}
	}
	hasSharedAllocation := false
	for id := range pool.accounts {
		if item := primary[id]; item != nil && configs[item.account.GroupName].upstreamAllocationEnabledFor(item.account) {
			hasSharedAllocation = true
			break
		}
	}
	if !hasSharedAllocation {
		return false
	}
	items := []*candidate{}
	allocationConfigs := make(map[string]engineConfig, len(configs))
	for name, config := range configs {
		allocationConfigs[name] = config
	}
	for id := range pool.accounts {
		item := primary[id]
		if item == nil {
			continue
		}
		config := configs[item.account.GroupName]
		if !config.upstreamAllocationEnabledFor(item.account) && !config.scalingEnabled || sub2APIUpstreamID(item.account) != pool.id || !item.schedulable || !placementLoadFactorEligible(item) || item.account.Paused || item.account.ManualPriority != nil || item.account.Schedulable == nil {
			continue
		}
		if !remoteSchedulable(item.account) && (!confirmedConcurrencyPause(item.account) || item.evidencePending) {
			if previouslyConcurrencyLimited(item.account) {
				limitConcurrency(item, "等待健康条件确认后再恢复共享并发")
			}
			continue
		}
		// Unlimited upstreams have no finite capacity to distribute. Only recover
		// previously capacity-paused accounts using their confirmed positive limit.
		if *pool.limit == 0 && !confirmedConcurrencyPause(item.account) {
			continue
		}
		if config.scalingEnabled && item.evidencePending {
			continue
		}
		if item.account.Concurrency == nil {
			continue
		}
		if !config.scalingEnabled {
			config.scalingMin = 1
			config.scalingMax = *pool.limit
			if *pool.limit == 0 {
				config.scalingMax = max(int64(1), *item.account.Concurrency)
			}
			config.scalingStepUp, config.scalingStepDown = math.MaxInt64, math.MaxInt64
			config.applyConcurrency, config.applySchedulable = true, true
			markUpstreamReduction(item, pool)
			item.upstreamAllocation = true
		}
		allocationConfigs[item.account.GroupName] = config
		items = append(items, item)
	}
	if len(items) == 0 {
		return false
	}
	sortCapacityWithHysteresis(items, allocationConfigs)
	pool.apply(items, allocationConfigs, global)
	return true
}

// Sum protected reservations directly. Subtracting a selected unlimited account
// from a saturated total would incorrectly discard other accounts' capacity.
func (budget *scalingBudget) reservedOutside(selected map[string]bool) int64 {
	reserved := max(int64(0), budget.allocated-budget.initialAllocated)
	for id, capacity := range budget.reservations {
		if !selected[id] {
			reserved = saturatingCapacityAdd(reserved, capacity)
		}
	}
	return reserved
}
