package routing

import (
	"fmt"
	"sort"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

// ManualCostWallChange only owns the scheduling switch. Manual priorities,
// concurrency, weights and their release baselines remain under manual control.
type ManualCostWallChange struct {
	Account     business.RoutingAccount
	GroupIDs    []string
	GroupNames  []string
	Schedulable bool
	State       string
}

func PlanManualCostWall(policy map[string]any, accounts []business.RoutingAccount, now time.Time) ([]ManualCostWallChange, error) {
	config, err := parseEngineConfig(policy)
	if err != nil {
		return nil, err
	}
	byID := map[string][]business.RoutingAccount{}
	for _, a := range accounts {
		if a.ManualPriority != nil {
			byID[a.ID] = append(byID[a.ID], a)
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	changes := []ManualCostWallChange{}
	for _, id := range ids {
		members := byID[id]
		a := members[0]
		if a.Schedulable == nil || a.Paused || accountDisabled(a) {
			continue
		}
		if _, paused := config.pausedAccounts[id]; paused {
			continue
		}
		if _, fused := config.manualFusedAccounts[id]; fused {
			continue
		}
		if fusedRoutingState(a.EffectiveState) {
			continue
		}
		blocked := config.accountCostWallEnabled(id)
		known := true
		groups, names := []string{}, []string{}
		rate, _ := nonnegativeDecimal(a.Multiplier)
		for _, member := range members {
			if member.GroupID == nil {
				known = false
				continue
			}
			groups = append(groups, *member.GroupID)
			names = append(names, member.GroupName)
			if !blocked {
				continue
			}
			wall, _, err := effectiveCostWall(member)
			if err != nil {
				return nil, fmt.Errorf("账号 %s 成本墙计算失败：%w", id, err)
			}
			if wall == nil || rate == nil {
				known = false
				continue
			}
			if rate.Cmp(wall) < 0 {
				blocked = false
			}
		}
		if !known {
			continue
		}
		desired := !blocked
		if desired && (a.EffectiveState != "cost_blocked" || !costWallRecoveryAllowed(a, now) || catalogBindingInvalid(a.CatalogBindingState)) {
			continue
		}
		if *a.Schedulable == desired {
			continue
		}
		state := "cost_blocked"
		if desired {
			state = ""
		}
		sort.Strings(groups)
		changes = append(changes, ManualCostWallChange{Account: a, GroupIDs: groups, GroupNames: names, Schedulable: desired, State: state})
	}
	return changes, nil
}
