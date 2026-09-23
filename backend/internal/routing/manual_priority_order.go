package routing

import (
	"fmt"
	"sort"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

// ManualPriorityChange changes only the effective priority. The user's reserved
// position and pre-control baseline remain intact, including after partial writes.
type ManualPriorityChange struct {
	AccountID        string
	ReservedPriority int64
	BeforePriority   int64
	Priority         int64
	Groups           []string
	P95MS            float64
	Samples          int
}

type manualOrderCandidate struct {
	account  business.RoutingAccount
	groups   []string
	priority int64
	latency  float64
	samples  int
	model    string
	eligible bool
}

// PlanManualPriorityOrder compares fresh successful requests of the same model.
// It never enables accounts, creates health decisions, or uses generation duration.
func PlanManualPriorityOrder(policy map[string]any, accounts []business.RoutingAccount, samples []business.RoutingSample, now time.Time) ([]ManualPriorityChange, error) {
	config, err := parseEngineConfig(policy)
	if err != nil {
		return nil, err
	}
	section, _ := policy["manual_priority"].(map[string]any)
	enabled, _ := section["latency_priority_enabled"].(bool)
	if !enabled || !config.trafficEnabled {
		return nil, nil
	}
	byID := map[string]*manualOrderCandidate{}
	evidence := map[string][]business.RoutingSample{}
	for _, sample := range samples {
		if sample.Source != "traffic" {
			continue
		}
		evidence[sample.AccountID] = append(evidence[sample.AccountID], sample)
	}
	for _, account := range accounts {
		if account.ManualPriority == nil {
			continue
		}
		item := byID[account.ID]
		if item == nil {
			item = &manualOrderCandidate{account: account, priority: *account.ManualPriority, eligible: true}
			if account.Priority != nil {
				item.priority = *account.Priority
			}
			byID[account.ID] = item
			summary, err := evaluateRoutingHealth(account, evidence[account.ID], business.PreviousRoutingDecision{}, config, policy, now)
			if err != nil {
				return nil, err
			}
			item.samples, item.model = summary.performanceSamples, summary.performanceModel
			if summary.performanceP95MS != nil {
				item.latency = *summary.performanceP95MS
			}
		}
		item.groups = append(item.groups, account.GroupName)
		groupConfig, active, err := config.forGroup(account.GroupID)
		if err != nil {
			return nil, err
		}
		_, excluded := config.excludedAccounts[account.ID]
		_, paused := config.pausedAccounts[account.ID]
		_, fused := config.manualFusedAccounts[account.ID]
		if !active || groupConfig.groupExcluded(account.GroupID) || !groupConfig.groupManaged(account.GroupID) ||
			!accountMetadataManaged(account, groupConfig) || account.Paused || excluded || paused || fused ||
			account.Schedulable == nil || !*account.Schedulable || account.Priority == nil || account.ExternalControl ||
			business.AccountUpstreamBlock(account.Metadata, account.Schedulable, now) != "" ||
			item.priority < 1 || item.priority > config.manualPriorityMax || item.latency <= 0 ||
			item.samples < config.performanceMinSamples || item.model == "" {
			item.eligible = false
		}
	}
	items := make([]*manualOrderCandidate, 0, len(byID))
	for _, item := range byID {
		item.groups = uniqueSorted(item.groups)
		// Unranked or paused accounts keep their actual position, never a fabricated latency.
		if !item.eligible && item.account.Priority != nil {
			item.priority = *item.account.Priority
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].latency != items[j].latency {
			return items[i].latency < items[j].latency
		}
		if items[i].priority != items[j].priority {
			return items[i].priority < items[j].priority
		}
		return items[i].account.ID < items[j].account.ID
	})
	for i, fast := range items {
		if !fast.eligible {
			continue
		}
		best := fast
		for _, slow := range items[i+1:] {
			if !slow.eligible || fast.model != slow.model || fast.latency >= slow.latency || slow.priority >= best.priority ||
				!sharedManualGroup(fast, slow) || !manualSwapSafe(items, fast, slow) {
				continue
			}
			best = slow
		}
		fast.priority, best.priority = best.priority, fast.priority
	}
	// A frozen account can retain an effective slot assigned by a previous round.
	// Reject an ambiguous plan instead of overwriting that account's position.
	for i, first := range items {
		for _, second := range items[i+1:] {
			if first.priority == second.priority && sharedManualGroup(first, second) {
				return nil, fmt.Errorf("手动控制位置存在冲突（账号 %s、%s），请同步账号并调整保留位置后重试", first.account.ID, second.account.ID)
			}
		}
	}
	result := []ManualPriorityChange{}
	for _, item := range items {
		if !item.eligible || item.priority == *item.account.Priority {
			continue
		}
		result = append(result, ManualPriorityChange{AccountID: item.account.ID, ReservedPriority: *item.account.ManualPriority,
			BeforePriority: *item.account.Priority, Priority: item.priority, Groups: item.groups, P95MS: item.latency, Samples: item.samples})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].AccountID < result[j].AccountID })
	return result, nil
}

func sharedManualGroup(a, b *manualOrderCandidate) bool {
	for _, left := range a.groups {
		for _, right := range b.groups {
			if left == right {
				return true
			}
		}
	}
	return false
}
func manualSwapSafe(items []*manualOrderCandidate, a, b *manualOrderCandidate) bool {
	for _, item := range items {
		if item == a || item == b {
			continue
		}
		if item.priority == b.priority && sharedManualGroup(item, a) || item.priority == a.priority && sharedManualGroup(item, b) {
			return false
		}
	}
	return true
}
