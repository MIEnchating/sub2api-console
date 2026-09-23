package routingwrite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

type upstreamCapacityReader interface {
	RoutingCapacityAccounts(context.Context) ([]business.RoutingAccount, error)
}

type upstreamCapacityWait struct{ reason string }

func (wait *upstreamCapacityWait) Error() string { return wait.reason }

func (s *Service) skipCapacityWait(ctx context.Context, result AccountResult, target business.AccountRoutingTarget, current values, desired map[string]any, operationID, actor string, wait *upstreamCapacityWait) AccountResult {
	reason := wait.Error()
	op := operation(operationID, operationType(target.ReleaseControl), target, actor, current.asMap(), map[string]any{
		"desired": desired, "reason": reason,
	}, false, false, nil)
	op.State, op.FieldName = "skipped", nil
	if err := s.repository.RecordAccountOperation(ctx, op); err != nil {
		return failedResult(result, fmt.Errorf("保存并发等待记录失败：%w", err))
	}
	result.Skipped, result.Reason, result.Effective = true, &reason, current.asMap()
	return result
}

type upstreamCapacityPlan struct {
	reader      upstreamCapacityReader
	members     map[string]string
	pools       map[string]bool
	reductions  map[string]business.AccountRoutingTarget
	repository  Repository
	globalLimit *int64
	constrained map[string]bool
}

func prepareUpstreamCapacity(ctx context.Context, repository Repository, targets map[string]business.AccountRoutingTarget, policy writePolicy, document map[string]any) (*upstreamCapacityPlan, []string, error) {
	reader, supported := repository.(upstreamCapacityReader)
	if !supported && hasUpstreamReduction(targets) {
		return nil, nil, errors.New("缺少共享并发库存，不能执行上游并发下调")
	}
	potential := map[string]bool{}
	ordinary := false
	for id, target := range targets {
		if target.AbandonControl || target.CleanupAction != nil {
			continue
		}
		if target.ReleaseControl || target.RestoreConcurrency || target.UpstreamReductionID != "" || (policy.autoApply["concurrency"] && target.Concurrency != nil) ||
			(policy.autoApply["schedulable"] && target.Schedulable != nil && *target.Schedulable) {
			potential[id] = true
			ordinary = ordinary || target.UpstreamReductionID == "" || target.UpstreamAllocation
		}
	}
	if len(potential) == 0 {
		return nil, nil, nil
	}
	var globalLimit *int64
	if ordinary {
		var err error
		globalLimit, err = globalCapacityLimit(document)
		if err != nil {
			return nil, nil, err
		}
	}
	if !supported {
		if globalLimit != nil {
			return nil, nil, errors.New("缺少全局并发库存，不能执行自动扩容或恢复")
		}
		return nil, nil, nil
	}
	inventory, err := reader.RoutingCapacityAccounts(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("共享并发库存读取失败，请同步账号后重试：%w", err)
	}
	plan := &upstreamCapacityPlan{reader: reader, members: map[string]string{}, pools: map[string]bool{}, reductions: map[string]business.AccountRoutingTarget{}, repository: repository}
	plan.globalLimit = globalLimit
	for id, target := range targets {
		if target.UpstreamReductionID != "" {
			plan.reductions[id] = target
		}
	}
	constrained, err := upstreamConstrainedAccounts(ctx, repository, document, inventory)
	if err != nil {
		return nil, nil, err
	}
	hasConstrainedTarget := false
	for id := range potential {
		hasConstrainedTarget = hasConstrainedTarget || constrained[id]
	}
	if !hasConstrainedTarget {
		globalLimit = nil
		plan.globalLimit = nil
	}
	plan.constrained = constrained
	for _, account := range inventory {
		if constrained[account.ID] && potential[account.ID] && account.UpstreamType != nil && strings.EqualFold(*account.UpstreamType, "sub2api") && account.UpstreamID != "" {
			plan.pools[account.UpstreamID] = true
		}
	}
	resources := []string{}
	for _, account := range inventory {
		if globalLimit == nil && !plan.pools[account.UpstreamID] {
			continue
		}
		if !stableRoutingAccountID(account.ID) {
			return nil, nil, errors.New("共享并发库存包含无效账号 ID，请同步账号后重试")
		}
		plan.members[account.ID] = account.UpstreamID
		resources = append(resources, mutationguard.Account(account.ID))
	}
	for id := range plan.pools {
		resources = append(resources, "upstream-identity/"+id)
	}
	if globalLimit != nil || len(plan.pools) > 0 {
		resources = append(resources, "routing-global-concurrency")
	}
	return plan, resources, nil
}

type upstreamCapacityPool struct {
	limit        *int64
	unknown      bool
	stale        bool
	used         int64
	reserved     map[string]int64
	err          error
	activeUsed   int64
	reductionErr error
}

type upstreamCapacityGuard struct {
	ctx                 context.Context
	admin               Admin
	members             map[string]string
	pools               map[string]*upstreamCapacityPool
	remote              map[string]values
	once                sync.Once
	mu                  sync.Mutex
	reductionAllowed    map[string]bool
	global              *globalCapacityBudget
	released            map[string]bool
	managedStopReleased map[string]bool
	constrained         map[string]bool
}

func (plan *upstreamCapacityPlan) recheck(ctx context.Context, admin Admin, document map[string]any) (*upstreamCapacityGuard, error) {
	if plan == nil || len(plan.pools) == 0 && plan.globalLimit == nil {
		if plan != nil && len(plan.reductions) > 0 {
			return nil, errors.New("上游身份未确认，不能执行并发下调")
		}
		return nil, nil
	}
	inventory, err := plan.reader.RoutingCapacityAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("取得变更租约后共享并发库存读取失败，请重试：%w", err)
	}
	constrained, err := upstreamConstrainedAccounts(ctx, plan.repository, document, inventory)
	if err != nil {
		return nil, err
	}
	for id, enabled := range plan.constrained {
		if constrained[id] != enabled {
			return nil, errors.New("等待写回期间账号共享并发范围已变化，请重新计算调度")
		}
	}
	guard := &upstreamCapacityGuard{managedStopReleased: map[string]bool{}, ctx: ctx, admin: admin, members: map[string]string{}, pools: map[string]*upstreamCapacityPool{}, remote: map[string]values{}, reductionAllowed: map[string]bool{}, released: map[string]bool{}}
	guard.constrained = constrained
	if plan.globalLimit != nil {
		guard.global = newGlobalCapacityBudget(*plan.globalLimit, inventory)
	}
	for _, account := range inventory {
		if plan.globalLimit == nil && !plan.pools[account.UpstreamID] {
			continue
		}
		if upstreamID, exists := plan.members[account.ID]; !exists || upstreamID != account.UpstreamID {
			return nil, errors.New("等待写回期间上游关联账号已变化，请重新计算调度")
		}
		guard.members[account.ID] = account.UpstreamID
		if guard.global != nil && !constrained[account.ID] {
			guard.global.released[account.ID] = false
		}
		guard.released[account.ID] = constrained[account.ID] && strings.EqualFold(strings.TrimSpace(account.EffectiveState), business.AccountStateConcurrencyLimited) && account.Schedulable != nil && !*account.Schedulable
		if !plan.pools[account.UpstreamID] {
			continue
		}
		if account.UpstreamType == nil || !strings.EqualFold(*account.UpstreamType, "sub2api") {
			return nil, errors.New("等待写回期间上游关联账号已变化，请重新计算调度")
		}
		pool := guard.pools[account.UpstreamID]
		if pool == nil {
			pool = &upstreamCapacityPool{reserved: map[string]int64{}}
			guard.pools[account.UpstreamID] = pool
		}
		if account.UpstreamConcurrencyLimit == nil || *account.UpstreamConcurrencyLimit < 0 {
			pool.unknown = true
		} else if pool.limit == nil || *account.UpstreamConcurrencyLimit < *pool.limit {
			value := *account.UpstreamConcurrencyLimit
			pool.limit = &value
		}
		pool.stale = pool.stale || account.UpstreamConcurrencyStatus == business.UpstreamConcurrencyStale
		if target, correcting := plan.reductions[account.ID]; correcting && account.UpstreamConcurrencyStatus != business.UpstreamConcurrencyKnown && account.UpstreamConcurrencyStatus != business.UpstreamConcurrencyStale && !(target.UpstreamAllocation && account.UpstreamConcurrencyStatus == business.UpstreamConcurrencyUnlimited && account.UpstreamConcurrencyLimit != nil && *account.UpstreamConcurrencyLimit == 0) {
			pool.unknown = true
		}
	}
	if len(guard.members) != len(plan.members) {
		return nil, errors.New("等待写回期间上游关联账号已变化，请重新计算调度")
	}
	if len(guard.members) > 0 {
		reader, ok := plan.repository.(interface {
			RoutingAccounts(context.Context, *string, *string) ([]business.RoutingAccount, error)
		})
		if !ok {
			if len(plan.reductions) > 0 {
				return nil, errors.New("账号托管范围不可复核，不能执行上游并发下调")
			}
			return guard, nil
		}
		accounts, err := reader.RoutingAccounts(ctx, nil, nil)
		if err != nil {
			return nil, err
		}
		stops, err := routing.UpstreamManagedStopScope(document, accounts)
		if err != nil {
			return nil, err
		}
		for _, account := range accounts {
			if _, member := guard.members[account.ID]; member && constrained[account.ID] {
				released := stops[account.ID]
				if released {
					guard.managedStopReleased[account.ID] = true
					if guard.global != nil {
						guard.global.released[account.ID] = true
					}
				}
			}
			target, correcting := plan.reductions[account.ID]
			if !correcting {
				continue
			}
			allowed, err := routing.UpstreamReductionAllowed(document, account)
			if target.UpstreamAllocation {
				allowed, err = routing.UpstreamAllocationAllowed(document, account)
			}
			if err != nil {
				return nil, err
			}
			if allowed && account.UpstreamID == target.UpstreamReductionID {
				for _, group := range target.GroupNames {
					if group == account.GroupName {
						guard.reductionAllowed[account.ID] = true
					}
				}
			}
		}
	}
	return guard, nil
}

func (guard *upstreamCapacityGuard) reserve(accountID string, current values, desired map[string]any) error {
	if guard == nil || !guard.constrained[accountID] {
		return nil
	}
	pool := guard.pools[guard.members[accountID]]
	checkUpstream := guard.constrained[accountID] && pool != nil && (pool.unknown || pool.stale || pool.limit == nil || *pool.limit != 0)
	if !checkUpstream && guard.global == nil {
		return nil
	}
	after := valuesWithDesired(current, desired)
	beforeCapacity, beforeKnown := activeCapacity(current)
	afterCapacity, afterKnown := activeCapacity(after)
	if afterKnown && ((beforeKnown && afterCapacity <= beforeCapacity) || afterCapacity == 0) {
		return nil
	}
	// An active account with an explicit unlimited setting can always be
	// reduced to a finite value. That reduction never funds another write.
	if afterKnown && current.schedulable != nil && *current.schedulable && current.concurrency != nil && *current.concurrency == 0 {
		return nil
	}
	if !afterKnown || checkUpstream && (pool.unknown || pool.stale || pool.limit == nil) {
		return errors.New("共享并发上限或账号状态尚未确认，已阻止扩容和恢复；请同步上游与账号后重试")
	}
	// Submit only queues work; the coordinator writes after every account
	// arrives. Loading here therefore precedes all batch mutations, including
	// reductions. The pool-wide leases also block other Console writers.
	guard.once.Do(guard.loadRemoteCapacity)
	guard.mu.Lock()
	defer guard.mu.Unlock()
	if checkUpstream && pool.err != nil {
		return fmt.Errorf("共享并发复核失败，已阻止扩容和恢复；请同步账号后重试：%w", pool.err)
	}
	if !beforeKnown {
		return errors.New("账号当前并发状态不可确认，已阻止扩容和恢复；请同步账号后重试")
	}
	// A batch read can observe a newer manual concurrency setting than the
	// individual read. Enabling without writing concurrency must budget that
	// observed setting as well, including while the account is paused.
	if _, writesConcurrency := desired["concurrency"]; !writesConcurrency {
		latest := guard.remote[accountID]
		if latest.concurrency == nil || *latest.concurrency <= 0 {
			return errors.New("远端账号并发已变化且无法确认，已阻止恢复；请同步账号后重试")
		}
		afterCapacity = max(afterCapacity, *latest.concurrency)
	}
	reserved, additional := int64(0), int64(0)
	if checkUpstream {
		reserved = pool.reserved[accountID]
		if beforeCapacity > reserved {
			if beforeCapacity-reserved > math.MaxInt64-pool.used {
				return errors.New("共享并发合计超出可核对范围，已阻止扩容")
			}
			pool.used += beforeCapacity - reserved
			pool.reserved[accountID] = beforeCapacity
			reserved = beforeCapacity
		}
		additional = max(int64(0), afterCapacity-reserved)
		if afterCapacity > max(int64(0), *pool.limit-(pool.used-reserved)) {
			return &upstreamCapacityWait{reason: fmt.Sprintf("等待并发额度，本轮扩容或恢复未执行：上游用户上限 %d，已分配及预留 %d，剩余 %d，本次需新增 %d（账号当前调度容量 %d → 目标 %d）；后续调度将自动复核，容量释放并确认后再执行", *pool.limit, pool.used, max(int64(0), *pool.limit-pool.used), additional, beforeCapacity, afterCapacity)}
		}
	}
	globalAdditional, err := guard.global.check(accountID, current, afterCapacity)
	if err != nil {
		return err
	}
	// Commit growth only after both budgets accept it under the same mutex.
	if checkUpstream {
		pool.used += additional
		pool.reserved[accountID] = max(reserved, afterCapacity)
	}
	guard.global.commit(accountID, globalAdditional, afterCapacity)
	return nil
}

func activeCapacity(value values) (int64, bool) {
	if value.schedulable != nil && !*value.schedulable {
		return 0, true
	}
	if value.schedulable == nil || value.concurrency == nil || *value.concurrency <= 0 {
		return 0, false
	}
	return *value.concurrency, true
}

func (guard *upstreamCapacityGuard) loadRemoteCapacity() {
	rows := map[string]map[string]any{}
	var readErr error
	lister, batch := guard.admin.(accountLister)
	if limited, ok := guard.admin.(*limitedAdmin); ok {
		_, batch = limited.admin.(accountLister)
	}
	if batch {
		var accounts []map[string]any
		accounts, readErr = lister.Accounts(guard.ctx)
		for _, account := range accounts {
			id := capacityAccountID(account["id"])
			if _, tracked := guard.members[id]; guard.global != nil && !tracked {
				readErr = errors.New("远端账号快照包含尚未同步的账号，无法核对全局并发；请同步账号后重新计算")
			}
			if _, duplicate := rows[id]; duplicate {
				readErr = errors.New("远端账号快照包含重复稳定 ID")
			}
			rows[id] = account
		}
	}
	ids := make([]string, 0, len(guard.members))
	for id := range guard.members {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		pool := guard.pools[guard.members[id]]
		checkUpstream := pool != nil && !pool.unknown && pool.limit != nil && *pool.limit != 0
		if !checkUpstream && guard.global == nil && !guard.reductionAllowed[id] {
			continue
		}
		payload, err := rows[id], readErr
		if !batch {
			payload, err = guard.admin.Account(guard.ctx, id)
		}
		if err == nil && (payload == nil || capacityAccountID(payload["id"]) != id) {
			err = fmt.Errorf("远端账号 %s 未返回匹配的稳定 ID", id)
		}
		var remote values
		if err == nil {
			remote, err = remoteValues(payload)
		}
		if guard.global != nil {
			guard.global.observe(id, remote, err)
		}
		if err == nil {
			guard.remote[id] = remote
		}
		if !checkUpstream {
			continue
		}
		if err == nil {
			guard.remote[id] = remote
			if remote.schedulable == nil || *remote.schedulable && (remote.concurrency == nil || *remote.concurrency < 0) {
				pool.reductionErr = fmt.Errorf("远端账号 %s 未返回可核对的调度状态与并发", id)
			} else if *remote.schedulable {
				capacity := *remote.concurrency
				if capacity == 0 {
					capacity = math.MaxInt64
				}
				if capacity > math.MaxInt64-pool.activeUsed {
					pool.activeUsed = math.MaxInt64
				} else {
					pool.activeUsed += capacity
				}
			}
		} else {
			pool.reductionErr = err
		}
		capacity, known := capacityReservation(remote, guard.released[id] || guard.managedStopReleased[id])
		if err == nil && !known {
			err = fmt.Errorf("远端账号 %s 未返回可核对的调度状态与并发", id)
		}
		if err == nil && capacity > math.MaxInt64-pool.used {
			err = errors.New("远端账号并发合计超出可核对范围")
		}
		if err != nil {
			pool.err = err
			continue
		}
		pool.used += capacity
		pool.reserved[id] = capacity
		guard.remote[id] = remote
	}
}

func capacityAccountID(value any) string {
	var id string
	switch raw := value.(type) {
	case string:
		id = raw
	case json.Number:
		id = raw.String()
	case int64:
		id = strconv.FormatInt(raw, 10)
	case int:
		id = strconv.Itoa(raw)
	}
	if !stableRoutingAccountID(id) {
		return ""
	}
	return id
}

// Read memberships only when group scaling overrides can affect the budget.
// Inventory still includes out-of-scope peers so their capacity is reserved.
func upstreamConstrainedAccounts(ctx context.Context, repository Repository, document map[string]any, inventory []business.RoutingAccount) (map[string]bool, error) {
	accounts := inventory
	if routing.GlobalScalingEnabled(document) {
		if reader, ok := repository.(interface {
			RoutingAccounts(context.Context, *string, *string) ([]business.RoutingAccount, error)
		}); ok {
			members, err := reader.RoutingAccounts(ctx, nil, nil)
			if err != nil {
				return nil, err
			}
			// Group memberships replace the group-less inventory rows.
			grouped := map[string]bool{}
			for _, account := range members {
				grouped[account.ID] = true
			}
			accounts = append([]business.RoutingAccount{}, members...)
			for _, account := range inventory {
				if !grouped[account.ID] {
					accounts = append(accounts, account)
				}
			}
		}
	}
	return routing.UpstreamCapacityScope(document, accounts)
}
