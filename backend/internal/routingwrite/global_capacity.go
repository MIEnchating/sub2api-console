package routingwrite

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func globalCapacityLimit(document map[string]any) (*int64, error) {
	scaling, ok := document["scaling"].(map[string]any)
	if !ok {
		if document["scaling"] != nil {
			return nil, errors.New("策略字段 scaling 必须是对象")
		}
		scaling = map[string]any{}
	}
	if !routing.GlobalScalingEnabled(document) {
		return nil, nil
	}
	limit := int64(900)
	if raw, present := scaling["global_max_concurrency"]; present {
		var err error
		limit, err = integer(raw)
		if err != nil || limit < 1 || limit > 10_000_000 {
			return nil, errors.New("策略字段 scaling.global_max_concurrency 必须在 1 到 10000000 之间")
		}
	}
	return &limit, nil
}

type globalCapacityBudget struct {
	limit    int64
	used     int64
	reserved map[string]int64
	released map[string]bool
	err      error
}

func newGlobalCapacityBudget(limit int64, inventory []business.RoutingAccount) *globalCapacityBudget {
	budget := &globalCapacityBudget{limit: limit, reserved: map[string]int64{}, released: map[string]bool{}}
	for _, account := range inventory {
		budget.released[account.ID] = strings.EqualFold(strings.TrimSpace(account.EffectiveState), business.AccountStateConcurrencyLimited) && account.Schedulable != nil && !*account.Schedulable
	}
	return budget
}

func (budget *globalCapacityBudget) capacity(accountID string, current values) (int64, bool) {
	return capacityReservation(current, budget.released[accountID])
}

func capacityReservation(current values, released bool) (int64, bool) {
	if released && current.schedulable != nil && !*current.schedulable {
		return 0, true
	}
	if current.schedulable == nil || current.concurrency == nil || *current.concurrency <= 0 {
		return 0, false
	}
	return *current.concurrency, true
}

func (budget *globalCapacityBudget) observe(accountID string, current values, readErr error) {
	capacity, known := budget.capacity(accountID, current)
	if readErr != nil {
		budget.err = readErr
	} else if !known {
		budget.err = fmt.Errorf("远端账号 %s 的全局并发保留量无法确认", accountID)
	} else if increase := max(int64(0), capacity-budget.reserved[accountID]); increase > math.MaxInt64-budget.used {
		budget.err = errors.New("全局并发合计超出可核对范围")
	} else {
		budget.used += increase
		budget.reserved[accountID] = max(budget.reserved[accountID], capacity)
	}
}

func (budget *globalCapacityBudget) check(accountID string, current values, afterCapacity int64) (int64, error) {
	if budget == nil {
		return 0, nil
	}
	budget.observe(accountID, current, nil)
	if budget.err != nil {
		return 0, fmt.Errorf("全局并发复核失败，已阻止扩容和恢复；请同步账号后重试：%w", budget.err)
	}
	reserved := budget.reserved[accountID]
	available := max(int64(0), budget.limit-(budget.used-reserved))
	additional := max(int64(0), afterCapacity-reserved)
	if afterCapacity > available {
		before, _ := activeCapacity(current)
		return 0, &upstreamCapacityWait{reason: fmt.Sprintf("等待并发额度，本轮扩容或恢复未执行：全局上限 %d，已分配及预留 %d，剩余 %d，本次需新增 %d（账号当前调度容量 %d → 目标 %d）；后续调度将自动复核，容量释放并确认后再执行", budget.limit, budget.used, max(int64(0), budget.limit-budget.used), additional, before, afterCapacity)}
	}
	return additional, nil
}

func (budget *globalCapacityBudget) commit(accountID string, additional, afterCapacity int64) {
	if budget == nil {
		return
	}
	budget.used += additional
	budget.reserved[accountID] = max(budget.reserved[accountID], afterCapacity)
}
