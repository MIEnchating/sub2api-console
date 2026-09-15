package routingwrite

import (
	"errors"
	"fmt"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func hasUpstreamReduction(targets map[string]business.AccountRoutingTarget) bool {
	for _, target := range targets {
		if target.UpstreamReductionID != "" {
			return true
		}
	}
	return false
}

func independentReduction(target business.AccountRoutingTarget, policy writePolicy) bool {
	return policy.upstreamReductionEnabled && target.UpstreamReductionID != ""
}

func independentPause(target business.AccountRoutingTarget, policy writePolicy) bool {
	return independentReduction(target, policy) && target.DesiredHealth == "concurrency_limited" && target.Schedulable != nil && !*target.Schedulable
}

func (guard *upstreamCapacityGuard) checkReduction(target business.AccountRoutingTarget, current values) (bool, error) {
	if guard == nil || guard.members[target.AccountID] != target.UpstreamReductionID || target.UpstreamReductionLimit == nil || *target.UpstreamReductionLimit <= 0 {
		return false, errors.New("上游并发下调来源未确认，请重新同步并计算")
	}
	pool := guard.pools[target.UpstreamReductionID]
	if pool == nil || pool.unknown || pool.limit == nil || *pool.limit <= 0 || *pool.limit != *target.UpstreamReductionLimit {
		return false, errors.New("上游并发上限已变化或不可确认，请重新计算下调额度")
	}
	if current.schedulable == nil || current.concurrency == nil || *current.concurrency < 0 {
		return false, errors.New("账号当前并发状态不可确认，不能执行自动下调")
	}
	if !*current.schedulable {
		return false, nil
	}
	if !guard.reductionAllowed[target.AccountID] {
		return false, errors.New("账号已不在允许自动下调的托管范围，请重新计算")
	}
	if target.Schedulable != nil && !*target.Schedulable && target.DesiredHealth != "concurrency_limited" {
		return false, errors.New("独立并发下调只允许因等待并发额度而暂停账号")
	}
	guard.once.Do(guard.loadRemoteCapacity)
	guard.mu.Lock()
	defer guard.mu.Unlock()
	if pool.reductionErr != nil {
		return false, fmt.Errorf("无法复核上游当前并发，已停止自动下调：%w", pool.reductionErr)
	}
	latest, found := guard.remote[target.AccountID]
	if !found || latest.schedulable == nil || latest.concurrency == nil {
		return false, errors.New("账号并发快照缺失，不能执行自动下调")
	}
	if !*latest.schedulable || pool.activeUsed <= *pool.limit {
		return false, nil
	}
	if desired := target.Concurrency; desired != nil {
		if *desired <= 0 || *current.concurrency > 0 && *desired > *current.concurrency || *latest.concurrency > 0 && *desired > *latest.concurrency {
			return false, errors.New("独立上游并发下调不能写零或提高当前并发，请重新计算")
		}
	}
	return true, nil
}
