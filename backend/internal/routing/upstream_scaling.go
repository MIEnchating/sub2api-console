package routing

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

type upstreamScalingPool struct {
	id       string
	limit    *int64
	accounts map[string]business.RoutingAccount
	current  map[string]int64
	unknown  bool
	stale    bool
}

type upstreamScalingPools map[string]*upstreamScalingPool

func previouslyConcurrencyLimited(account business.RoutingAccount) bool {
	return strings.EqualFold(strings.TrimSpace(account.EffectiveState), "concurrency_limited")
}

func confirmedConcurrencyPause(account business.RoutingAccount) bool {
	return previouslyConcurrencyLimited(account) && account.Schedulable != nil && !*account.Schedulable
}

func sub2APIUpstreamID(account business.RoutingAccount) string {
	if account.UpstreamType == nil || !strings.EqualFold(strings.TrimSpace(*account.UpstreamType), "sub2api") {
		return ""
	}
	return strings.TrimSpace(account.UpstreamID)
}

func newUpstreamScalingPools(inventory []business.RoutingAccount) upstreamScalingPools {
	pools := upstreamScalingPools{}
	for _, account := range inventory {
		id := sub2APIUpstreamID(account)
		if id == "" {
			continue
		}
		pool := pools[id]
		if pool == nil {
			pool = &upstreamScalingPool{id: id, accounts: map[string]business.RoutingAccount{}, current: map[string]int64{}}
			pools[id] = pool
		}
		pool.accounts[account.ID] = account
		pool.stale = pool.stale || account.UpstreamConcurrencyStatus == "stale"
		if account.UpstreamConcurrencyLimit == nil || *account.UpstreamConcurrencyLimit < 0 {
			pool.unknown = true
		} else if pool.limit == nil || *account.UpstreamConcurrencyLimit > 0 && (*pool.limit == 0 || *account.UpstreamConcurrencyLimit < *pool.limit) {
			pool.limit = cloneInt64(account.UpstreamConcurrencyLimit)
		}
		current := int64(math.MaxInt64)
		if confirmedConcurrencyPause(account) {
			current = 0
		} else if account.Concurrency != nil && *account.Concurrency > 0 {
			current = *account.Concurrency
		}
		pool.current[account.ID] = max(pool.current[account.ID], current)
	}
	return pools
}

func (pools upstreamScalingPools) manages(account business.RoutingAccount) bool {
	pool := pools[sub2APIUpstreamID(account)]
	// Sub2API explicitly treats zero as unlimited. Missing capacity remains
	// unknown and must never be converted to either zero or free headroom.
	return pool != nil && (pool.unknown || pool.stale || pool.limit == nil || *pool.limit != 0 || previouslyConcurrencyLimited(account))
}

func (pools upstreamScalingPools) apply(primary map[string]*candidate, configs map[string]engineConfig, global *scalingBudget) {
	for _, item := range primary {
		if previouslyConcurrencyLimited(item.account) && sub2APIUpstreamID(item.account) == "" && placementLoadFactorEligible(item) {
			limitConcurrency(item, "上游并发来源缺失，保持暂停；请重新核对上游绑定后同步")
		}
	}
	ids := make([]string, 0, len(pools))
	for id := range pools {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		pool := pools[id]
		// Recovery requires confirmed capacity even when scaling is disabled or
		// health evidence is still pending. Do not generate an impossible write.
		for accountID := range pool.accounts {
			item := primary[accountID]
			if item == nil || !item.schedulable || remoteSchedulable(item.account) || !placementLoadFactorEligible(item) {
				continue
			}
			config := configs[item.account.GroupName]
			if previouslyConcurrencyLimited(item.account) && !config.scalingEnabled && !config.upstreamAllocationEnabledFor(item.account) {
				limitConcurrency(item, "账号未启用共享并发分配或智能扩容，保持等待并发额度")
				continue
			}
			if pool.unknown || pool.stale || pool.limit == nil || (!configs[item.account.GroupName].scalingEnabled && !configs[item.account.GroupName].upstreamAllocationEnabledFor(item.account) && (item.account.Concurrency == nil || *item.account.Concurrency <= 0)) {
				item.schedulable = false
				appendConcurrencyReason(item, "恢复前共享并发或账号并发尚未确认，保持暂停；请同步上游与账号")
			}
		}
		if pool.allocateShared(primary, configs, global) {
			continue
		}
		correcting := pool.reduceOverage(primary, configs)
		items := []*candidate{}
		for accountID := range pool.accounts {
			item := primary[accountID]
			if item == nil || sub2APIUpstreamID(item.account) != id || !pools.manages(item.account) {
				continue
			}
			config := configs[item.account.GroupName]
			if item.upstreamReductionID != "" {
				continue
			}
			if correcting {
				if previouslyConcurrencyLimited(item.account) && placementLoadFactorEligible(item) {
					limitConcurrency(item, "上游本轮正在下调超额，等待读回确认后再评估恢复")
				}
				continue
			}
			if !config.scalingEnabled {
				if previouslyConcurrencyLimited(item.account) && placementLoadFactorEligible(item) {
					message := "并发伸缩已关闭，保持上游并发暂停；启用伸缩后重新评估"
					if config.upstreamAllocationEnabledFor(item.account) {
						message = "共享并发尚未确认或账号未满足恢复条件，保持等待；同步成功后自动重新分配"
					}
					limitConcurrency(item, message)
				}
				continue
			}
			if !item.schedulable || item.evidencePending || !placementLoadFactorEligible(item) || item.account.ManualPriority != nil {
				if previouslyConcurrencyLimited(item.account) && placementLoadFactorEligible(item) {
					limitConcurrency(item, "上游并发暂停期间仍需满足健康与调度条件后才能恢复")
				}
				continue
			}
			if remoteSchedulable(item.account) && item.account.Concurrency == nil {
				appendConcurrencyReason(item, "账号当前并发尚未确认，保持当前配置；请同步账号后再分配")
				continue
			}
			if pool.limit != nil && *pool.limit > 0 && (!config.applyConcurrency || !config.applySchedulable) {
				message := "上游有限并发分配需要同时启用并发与可调度状态自动写入；当前仅保留计算预览，请补全开关后重新执行"
				item.concurrencyConfigurationError = &message
				appendConcurrencyReason(item, message)
			}
			items = append(items, item)
		}
		if len(items) == 0 {
			continue
		}
		sortCapacityWithHysteresis(items, configs)
		pool.apply(items, configs, global)
	}
}

func (pool *upstreamScalingPool) apply(items []*candidate, configs map[string]engineConfig, global *scalingBudget) {
	if pool.unknown || pool.limit == nil || *pool.limit < 0 || pool.stale && *pool.limit == 0 {
		for _, item := range items {
			message := "上游并发上限尚未确认，保持当前并发；请同步上游用户信息"
			if !remoteSchedulable(item.account) {
				limitConcurrency(item, message)
			} else {
				appendConcurrencyReason(item, message)
			}
		}
		return
	}
	limit := *pool.limit
	if limit == 0 {
		// A formerly limited upstream can become unlimited. Global capacity
		// still gates recovery; never emit an account concurrency of zero.
		limit = math.MaxInt64
	}
	selected := map[string]bool{}
	for _, item := range items {
		selected[item.account.ID] = true
	}
	reserved, allocated := int64(0), int64(0)
	for id, current := range pool.current {
		allocated = saturatingCapacityAdd(allocated, current)
		if !selected[id] {
			reserved = saturatingCapacityAdd(reserved, current)
		}
	}
	available := max(int64(0), limit-reserved)
	if configs[items[0].account.GroupName].globalScalingEnabled {
		globalLimit := int64(math.MaxInt64)
		for _, item := range items {
			globalLimit = min(globalLimit, configs[item.account.GroupName].scalingGlobalMax)
		}
		available = min(available, max(int64(0), globalLimit-global.reservedOutside(selected)))
	}
	waitingReason := fmt.Sprintf("上游并发上限 %d，已分配及预留 %d，可分配 %d；按调度权重等待额度释放", limit, reserved, available)
	if configs[items[0].account.GroupName].globalScalingEnabled && available < max(int64(0), limit-reserved) {
		waitingReason = fmt.Sprintf("全局并发上限 %d，其他账号已分配及预留 %d，可分配 %d；等待全局额度释放", configs[items[0].account.GroupName].scalingGlobalMax, global.reservedOutside(selected), available)
	}
	eligible := items[:min(int64(len(items)), available)]
	for _, item := range items[len(eligible):] {
		limitConcurrency(item, waitingReason)
	}
	if len(eligible) == 0 {
		return
	}
	quotas := upstreamConcurrencyQuotas(eligible, configs, available)
	for _, item := range eligible {
		config := configs[item.account.GroupName]
		quota := quotas[item.account.ID]
		if quota < config.scalingMin {
			item.concurrencyIssue = fmt.Sprintf("上游 %s 并发不足以满足单账号下限，已按可用容量分配；请调整单账号下限或上游额度", pool.id)
		}
		current := pool.current[item.account.ID]
		upstreamHeadroom := max(int64(0), limit-allocated)
		globalHeadroom := max(int64(0), config.scalingGlobalMax-global.allocated)
		if item.upstreamAllocation && !config.globalScalingEnabled {
			globalHeadroom = math.MaxInt64
		}
		if pool.stale {
			upstreamHeadroom = 0
			appendConcurrencyReason(item, "上游并发信息待更新，本轮仅允许缩减并发")
		}
		if item.account.Schedulable != nil && !*item.account.Schedulable {
			if pool.stale {
				limitConcurrency(item, "上游并发信息更新成功后再恢复调度")
				continue
			}
			// A non-capacity pause is conservatively reserved in the inventory.
			// Its own reservation can fund reopening, but no sibling reduction can.
			globalRecoveryCapacity := max(int64(0), config.scalingGlobalMax-max(int64(0), global.allocated-current))
			if item.upstreamAllocation && !config.globalScalingEnabled {
				globalRecoveryCapacity = math.MaxInt64
			}
			if globalRecoveryCapacity == 0 {
				limitConcurrency(item, "等待上游及全局并发容量释放并确认后恢复调度")
				continue
			}
			quota = min(quota, globalRecoveryCapacity)
			recoveryHeadroom := min(max(int64(0), limit-max(int64(0), allocated-current)), globalRecoveryCapacity)
			pool.prepareRecovery(item, config, quota, recoveryHeadroom, current, &allocated, global)
			continue
		}
		desired := quota
		if desired > current {
			if item.upstreamAllocation && item.evidencePending {
				desired = current
			} else {
				desired = current + min(desired-current, config.scalingStepUp, upstreamHeadroom, globalHeadroom)
			}
		} else if current != math.MaxInt64 {
			desired = max(desired, current-config.scalingStepDown)
		}
		if desired > current {
			allocated = saturatingCapacityAdd(allocated, desired-current)
			global.allocated = saturatingCapacityAdd(global.allocated, desired-current)
		}
		if item.account.Concurrency == nil || desired != *item.account.Concurrency {
			item.desiredConcurrency = &desired
		}
		appendConcurrencyReason(item, fmt.Sprintf("上游并发按调度权重分配 %d，本轮并发 %d", quota, desired))
	}
}

func (pool *upstreamScalingPool) prepareRecovery(item *candidate, config engineConfig, quota, headroom, reserved int64, allocated *int64, global *scalingBudget) {
	current := item.account.Concurrency
	if current == nil || *current <= 0 || *current > quota {
		item.desiredConcurrency = &quota
		limitConcurrency(item, "先调整暂停账号并发，待读回确认后再恢复调度")
		return
	}
	if !config.applyConcurrency || !config.applySchedulable {
		limitConcurrency(item, fmt.Sprintf("可分配并发 %d；请同时启用并发与可调度状态自动写入后恢复", quota))
		return
	}
	if *current > headroom {
		limitConcurrency(item, "等待上游及全局并发容量释放并确认后恢复调度")
		return
	}
	increase := max(int64(0), *current-reserved)
	*allocated = saturatingCapacityAdd(*allocated, increase)
	global.allocated = saturatingCapacityAdd(global.allocated, increase)
	appendConcurrencyReason(item, fmt.Sprintf("上游及全局容量已确认，恢复调度并保留已确认并发 %d", *current))
}

func upstreamConcurrencyQuotas(items []*candidate, configs map[string]engineConfig, capacity int64) map[string]int64 {
	return boundedConcurrencyQuotas(items, capacity, func(item *candidate) (int64, int64) {
		config := configs[item.account.GroupName]
		return config.scalingMin, config.scalingMax
	})
}

func boundedConcurrencyQuotas(items []*candidate, capacity int64, bounds func(*candidate) (int64, int64)) map[string]int64 {
	result := map[string]int64{}
	minimumTotal := int64(0)
	maximumTotal := int64(0)
	for _, item := range items {
		minimum, maximum := bounds(item)
		minimumTotal = saturatingCapacityAdd(minimumTotal, minimum)
		maximumTotal = saturatingCapacityAdd(maximumTotal, maximum)
	}
	capacity = min(capacity, maximumTotal)
	minimumLimited := minimumTotal > capacity
	for _, item := range items {
		minimum, _ := bounds(item)
		if minimumLimited {
			minimum = 1
		}
		result[item.account.ID] = minimum
		capacity -= minimum
	}
	for capacity > 0 {
		active := []*candidate{}
		totalWeight := 0.0
		for _, item := range items {
			_, maximum := bounds(item)
			if result[item.account.ID] < maximum {
				active = append(active, item)
				totalWeight += item.weight
			}
		}
		if len(active) == 0 {
			break
		}
		remaining := capacity
		remainders := map[string]float64{}
		for _, item := range active {
			share := float64(remaining) / float64(len(active))
			if totalWeight > 0 {
				share = float64(remaining) * item.weight / totalWeight
			}
			_, maximum := bounds(item)
			increment := min(int64(math.Floor(share)), maximum-result[item.account.ID], capacity)
			result[item.account.ID] += increment
			capacity -= increment
			remainders[item.account.ID] = share - math.Floor(share)
		}
		sort.SliceStable(active, func(i, j int) bool { return remainders[active[i].account.ID] > remainders[active[j].account.ID] })
		for _, item := range active {
			if capacity == 0 {
				break
			}
			_, maximum := bounds(item)
			if result[item.account.ID] < maximum {
				result[item.account.ID]++
				capacity--
			}
		}
	}
	return result
}

func saturatingCapacityAdd(left, right int64) int64 {
	if right > math.MaxInt64-left {
		return math.MaxInt64
	}
	return left + right
}

func limitConcurrency(item *candidate, reason string) {
	item.state, item.schedulable = "concurrency_limited", false
	appendConcurrencyReason(item, reason)
}

func appendConcurrencyReason(item *candidate, reason string) {
	if item.reason != "" {
		item.reason += "；"
	}
	item.reason += reason
}
