package routingwrite

import (
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

// Allocation is a distinct capability from legacy reduction targets. Both use
// the existing policy/identity leases and forced readback; reserve still checks
// the initial remote snapshot against upstream AND global budgets before growth.
func (guard *upstreamCapacityGuard) checkAllocation(target business.AccountRoutingTarget, current values) (bool, error) {
	if guard == nil || guard.members[target.AccountID] != target.UpstreamReductionID || target.UpstreamReductionLimit == nil || *target.UpstreamReductionLimit < 0 {
		return false, errors.New("共享并发分配来源未确认，请重新同步并计算")
	}
	pool := guard.pools[target.UpstreamReductionID]
	if pool == nil || pool.unknown || pool.stale || pool.limit == nil || *pool.limit != *target.UpstreamReductionLimit {
		return false, errors.New("上游额度已变化或尚未确认，请同步后重新分配")
	}
	if !guard.reductionAllowed[target.AccountID] {
		return false, errors.New("账号不在共享并发自动分配范围，不能更改并发或恢复")
	}
	if current.schedulable == nil || current.concurrency == nil || *current.concurrency < 0 {
		return false, errors.New("账号当前并发状态不可确认，请同步账号")
	}
	if target.Concurrency != nil && *target.Concurrency <= 0 {
		return false, errors.New("共享并发分配必须保留正数并发")
	}
	if !*current.schedulable && !guard.released[target.AccountID] {
		return false, errors.New("仅允许自动恢复已确认的等待并发额度账号")
	}
	if target.Schedulable != nil {
		if !*target.Schedulable && target.DesiredHealth != business.AccountStateConcurrencyLimited {
			return false, errors.New("共享并发分配只允许因等待额度暂停账号")
		}
		if *target.Schedulable {
			switch target.DesiredHealth {
			case "healthy", "unknown", "degraded", "survivor":
			default:
				return false, errors.New("账号尚未满足健康与调度条件，不能自动恢复")
			}
		}
	}
	guard.once.Do(guard.loadRemoteCapacity)
	guard.mu.Lock()
	defer guard.mu.Unlock()
	if pool.reductionErr != nil {
		return false, pool.reductionErr
	}
	latest, found := guard.remote[target.AccountID]
	if !found || latest.schedulable == nil || latest.concurrency == nil || *latest.schedulable != *current.schedulable || *latest.concurrency != *current.concurrency {
		return false, errors.New("账号并发快照已变化，请同步后重新分配")
	}
	return true, nil
}
