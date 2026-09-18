package routing

import (
	"fmt"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

// UpstreamReductionAllowed rechecks the same managed scope used to produce
// a reduction target. The writer calls it with current policy and account rows
// after acquiring the account and upstream leases.
func UpstreamReductionAllowed(document map[string]any, account business.RoutingAccount) (bool, error) {
	config, err := parseEngineConfig(document)
	if err != nil {
		return false, err
	}
	config, enabled, err := config.forGroup(account.GroupID)
	if err != nil {
		return false, err
	}
	allowed, _ := eligibleScope(account, config)
	_, policyPaused := config.pausedAccounts[account.ID]
	_, manualFused := config.manualFusedAccounts[account.ID]
	return enabled && config.upstreamReductionEnabled && allowed && accountMetadataManaged(account, config) &&
		!account.Paused && !policyPaused && !manualFused && account.ManualPriority == nil && (config.manageAllAccounts || !account.ExternalControl && !accountExternallyModified(account)) &&
		account.Schedulable != nil && *account.Schedulable, nil
}

func (pool *upstreamScalingPool) reduceOverage(primary map[string]*candidate, configs map[string]engineConfig) bool {
	if pool.unknown || pool.limit == nil || *pool.limit <= 0 {
		return false
	}
	// A paused/fused account retains its reservation for allocation, but its
	// stored setting is not active usage and must not trigger a new correction.
	active := int64(0)
	for _, account := range pool.accounts {
		if account.UpstreamConcurrencyStatus != business.UpstreamConcurrencyKnown && account.UpstreamConcurrencyStatus != business.UpstreamConcurrencyStale {
			return false
		}
		if account.Schedulable != nil && *account.Schedulable && account.Concurrency != nil {
			active = saturatingCapacityAdd(active, pool.current[account.ID])
		}
	}
	if active <= *pool.limit {
		return false
	}
	items := []*candidate{}
	selected := map[string]bool{}
	for id := range pool.accounts {
		item := primary[id]
		if item == nil || !configs[item.account.GroupName].upstreamReductionEnabled || sub2APIUpstreamID(item.account) != pool.id ||
			!item.schedulable || !placementLoadFactorEligible(item) || item.account.Paused ||
			item.account.ManualPriority != nil || item.account.Schedulable == nil || !*item.account.Schedulable || item.account.Concurrency == nil {
			continue
		}
		items, selected[id] = append(items, item), true
	}
	if len(items) == 0 {
		return false
	}
	sortCapacityWithHysteresis(items, configs)
	reserved := int64(0)
	for id, capacity := range pool.current {
		if !selected[id] {
			reserved = saturatingCapacityAdd(reserved, capacity)
		}
	}
	available := max(int64(0), *pool.limit-reserved)
	eligible := items[:min(int64(len(items)), available)]
	for _, item := range items[len(eligible):] {
		markUpstreamReduction(item, pool)
		limitConcurrency(item, "上游共享并发超额，按调度权重暂停；仅自动下调已开启，恢复需启用智能扩容或人工核对")
	}
	quotas := boundedConcurrencyQuotas(eligible, available, func(item *candidate) (int64, int64) {
		return 1, min(pool.current[item.account.ID], *pool.limit)
	})
	for _, item := range eligible {
		quota := quotas[item.account.ID]
		if quota >= pool.current[item.account.ID] {
			continue
		}
		markUpstreamReduction(item, pool)
		item.desiredConcurrency = &quota
		appendConcurrencyReason(item, fmt.Sprintf("上游共享并发超额，按调度权重将并发下调为 %d；不执行扩容", quota))
	}
	return true
}

func markUpstreamReduction(item *candidate, pool *upstreamScalingPool) {
	item.upstreamReductionID, item.upstreamReductionLimit = pool.id, cloneInt64(pool.limit)
	item.scalingCooldown = false
	item.concurrencyConfigurationError = nil
}
