package routing

import (
	"sort"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func costWallReached(tier string) bool {
	return tier == "equal" || tier == "above"
}

func (config engineConfig) accountCostWallEnabled(accountID string) bool {
	_, ignored := config.ignoreCostWallAccounts[accountID]
	return config.costWallEnabled && !ignored
}

func applyCostWallFallbacks(groups map[string][]*candidate, byAccount map[string][]*candidate, inventory []business.RoutingAccount, config engineConfig, now time.Time) {
	// Fuse minimum-pool protection cannot keep a costly account open while
	// another usable account exists. Apply the cost boundary after fuse decisions.
	for _, memberships := range byAccount {
		primary := primaryMembership(memberships)
		applyAccountCostWall(primary, memberships, config, now)
		for _, item := range memberships {
			item.state, item.schedulable, item.reason = primary.state, primary.schedulable, primary.reason
		}
	}
	if !config.costWallEnabled || !config.costWallFallbackEnabled {
		return
	}
	available := map[string]bool{}
	for group, items := range groups {
		for _, item := range items {
			if item.schedulable && costWallRecoveryAllowed(item.account, now) {
				available[group] = true
			}
		}
	}
	// Scoped calculations and manual ownership must still see confirmed peers.
	for _, account := range inventory {
		if _, managed := byAccount[account.ID]; managed || !remoteSchedulable(account) || account.Paused || accountDisabled(account) || catalogBindingInvalid(account.CatalogBindingState) || accountUpstreamBlock(account, now) != "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(account.EffectiveState)) {
		case "fused", "hard_open", "soft_open", "cost_blocked", "paused", "disabled", "binding_invalid":
			continue
		}
		available[account.GroupName] = true
	}
	groupNames := make([]string, 0, len(groups))
	for group := range groups {
		groupNames = append(groupNames, group)
	}
	sort.Strings(groupNames)
	for _, group := range groupNames {
		if available[group] {
			continue
		}
		var chosen *candidate
		for _, item := range groups[group] {
			if item.state != "cost_blocked" || item.health.Fatal || item.fuseKind != "" || !costWallRecoveryAllowed(item.account, now) {
				continue
			}
			if chosen == nil || costFallbackLess(item, chosen) {
				chosen = item
			}
		}
		if chosen == nil {
			continue
		}
		for _, item := range byAccount[chosen.account.ID] {
			item.state, item.schedulable = "survivor", true
			item.reason = "成本墙保底：分组 " + group + " 没有可用账号，临时启用；正常账号恢复后关闭"
			available[item.account.GroupName] = true
		}
	}
}

func costFallbackLess(left, right *candidate) bool {
	if compared := left.rate.Cmp(right.rate); compared != 0 {
		return compared < 0
	}
	if left.health.HealthScore != right.health.HealthScore {
		return left.health.HealthScore > right.health.HealthScore
	}
	return stableIDLess(left.account.ID, right.account.ID)
}

// CostWallProbeBlocks evaluates current prices, including profit controls and
// managed memberships, before a scheduling decision has necessarily been saved.
func CostWallProbeBlocks(accounts []business.RoutingAccount, policy map[string]any) (map[string]bool, error) {
	config, err := parseEngineConfig(policy)
	if err != nil {
		return nil, err
	}
	if !config.costWallEnabled || !config.costWallStopAutoProbe {
		return map[string]bool{}, nil
	}
	byAccount := map[string][]*candidate{}
	for _, account := range accounts {
		if !config.accountCostWallEnabled(account.ID) {
			continue
		}
		groupConfig, enabled, err := config.forGroup(account.GroupID)
		if err != nil {
			return nil, err
		}
		if !enabled || groupConfig.groupExcluded(account.GroupID) || !groupConfig.groupManaged(account.GroupID) || !accountMetadataManaged(account, groupConfig) {
			continue
		}
		wall, _, err := effectiveCostWall(account)
		if err != nil {
			return nil, err
		}
		rate, _, _, _ := resolveRate(account, groupConfig, wall)
		byAccount[account.ID] = append(byAccount[account.ID], &candidate{account: account, rate: rate, costWall: wall})
	}
	blocked := map[string]bool{}
	for id, memberships := range byAccount {
		primary := primaryMembership(memberships)
		if config.costWallFallbackEnabled && remoteSchedulable(primary.account) && strings.EqualFold(primary.account.EffectiveState, "survivor") {
			continue
		}
		blocked[id] = true
		for _, item := range memberships {
			tier, _ := costTier(primary.rate, item.costWall)
			if !costWallReached(tier) {
				blocked[id] = false
				break
			}
		}
	}
	return blocked, nil
}
