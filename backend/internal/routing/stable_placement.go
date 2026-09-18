package routing

import (
	"math"
	"math/big"
	"sort"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func preparePlacementCooldowns(byAccount map[string][]*candidate, configs map[string]engineConfig, previous map[string]business.PreviousRoutingDecision, now time.Time) {
	for _, memberships := range byAccount {
		item := primaryMembership(memberships)
		if item == nil {
			continue
		}
		prior, found := previousDecision(previous, item.account.ID, item.account.GroupName)
		item.placementCooldown = found && !prior.LastApplyAt.IsZero() && now.Sub(prior.LastApplyAt) < configs[item.account.GroupName].cooldown
	}
}

func sortOwnedWithHysteresis(items []*candidate, config engineConfig) {
	sortPlacementCandidates(items, func(*candidate) engineConfig { return config }, false)
}

func sortCapacityWithHysteresis(items []*candidate, configs map[string]engineConfig) {
	sortPlacementCandidates(items, func(item *candidate) engineConfig { return configs[item.account.GroupName] }, true)
}

func sortPlacementCandidates(items []*candidate, configuration func(*candidate) engineConfig, capacity bool) {
	sort.SliceStable(items, func(left, right int) bool {
		if capacity && remoteSchedulable(items[left].account) != remoteSchedulable(items[right].account) {
			return remoteSchedulable(items[left].account)
		}
		leftPriority, leftKnown := automaticPlacementPriority(items[left], configuration(items[left]))
		rightPriority, rightKnown := automaticPlacementPriority(items[right], configuration(items[right]))
		if leftKnown != rightKnown {
			return leftKnown
		}
		if leftKnown && leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		return stableIDLess(items[left].account.ID, items[right].account.ID)
	})
	blockers := map[*candidate]int{}
	for index, challenger := range items {
		for _, incumbent := range items[:index] {
			config := placementPairConfig(configuration(challenger), configuration(incumbent))
			if !placementCanCross(challenger, incumbent, config) {
				blockers[challenger]++
			}
		}
	}
	// A promotion may cross small differences, but never a better account.
	// Removing the selected slot updates crossing blockers in O(n), keeping
	// the whole round quadratic and unchanged tradeoffs from cycling.
	for position := range items {
		incumbent := items[position]
		best := position
		cooling := incumbent.placementCooldown
		for index := position + 1; index < len(items); index++ {
			cooling = cooling || items[index].placementCooldown
			config := placementPairConfig(configuration(items[index]), configuration(incumbent))
			if blockers[items[index]] > 0 {
				continue
			}
			if !canPromotePlacement(items[index], incumbent, config) {
				continue
			}
			safety := placementSafetyImproves(items[index], incumbent)
			if !safety && cooling {
				continue
			}
			if best == position || placementSafetyImproves(items[index], items[best]) || placementScoreBetter(items[index], items[best], config) {
				best = index
			}
		}
		chosen := items[best]
		for _, later := range items[best+1:] {
			config := placementPairConfig(configuration(later), configuration(chosen))
			if !placementCanCross(later, chosen, config) {
				blockers[later]--
			}
		}
		if best != position {
			copy(items[position+1:best+1], items[position:best])
			items[position] = chosen
		}
	}
}

func placementPairConfig(challenger, incumbent engineConfig) engineConfig {
	incumbent.performanceMinSamples = max(incumbent.performanceMinSamples, challenger.performanceMinSamples)
	if challenger.changeThreshold != nil && (incumbent.changeThreshold == nil || challenger.changeThreshold.Cmp(incumbent.changeThreshold) > 0) {
		incumbent.changeThreshold = challenger.changeThreshold
	}
	return incumbent
}

func canPromotePlacement(challenger, incumbent *candidate, config engineConfig) bool {
	if placementSafetyImproves(challenger, incumbent) {
		return true
	}
	if !challenger.schedulable || challenger.state != "healthy" || challenger.evidencePending ||
		placementSafetyTier(challenger) > placementSafetyTier(incumbent) || !placementEvidenceReady(challenger, config) {
		return false
	}
	challengerQuality, incumbentQuality := placementPairQualities(challenger, incumbent, config)
	return weightAdvantageExceeds(challengerQuality, incumbentQuality, config.changeThreshold)
}

func placementSafetyTier(item *candidate) int {
	if !item.schedulable || !placementLoadFactorEligible(item) {
		return 3
	}
	if item.state == "survivor" {
		return 2
	}
	if item.state == "degraded" && !item.evidencePending {
		return 1
	}
	return 0
}

func placementSafetyImproves(challenger, incumbent *candidate) bool {
	if placementSafetyTier(challenger) >= placementSafetyTier(incumbent) {
		return false
	}
	return challenger.schedulable && !challenger.evidencePending && challenger.state == "healthy" && challenger.health.SampleCount > 0
}

func placementCanCross(challenger, incumbent *candidate, config engineConfig) bool {
	if placementSafetyTier(challenger) != placementSafetyTier(incumbent) {
		return placementSafetyTier(challenger) < placementSafetyTier(incumbent)
	}
	left, right := placementPairQualities(challenger, incumbent, config)
	return left >= right
}

func placementScoreBetter(challenger, incumbent *candidate, config engineConfig) bool {
	if placementSafetyTier(challenger) != placementSafetyTier(incumbent) {
		return placementSafetyTier(challenger) < placementSafetyTier(incumbent)
	}
	left, right := placementPairQualities(challenger, incumbent, config)
	return left > right
}

func placementPairQualities(challenger, incumbent *candidate, config engineConfig) (float64, float64) {
	left, right := *challenger, *incumbent
	left.strategy, right.strategy = config.strategy, config.strategy
	if !left.rateKnown || !right.rateKnown {
		left.rate, right.rate = big.NewRat(1, 1), big.NewRat(1, 1)
	}
	left.rankingLatencyMS, right.rankingLatencyMS = 1000, 1000
	if leftLatency, rightLatency, comparable := comparablePlacementLatencies(challenger, incumbent, config); comparable {
		left.rankingLatencyMS, right.rankingLatencyMS = leftLatency, rightLatency
	}
	benchmark := strategyScoreBenchmark([]*candidate{&left, &right})
	if len(left.placementMemberships) == 0 && len(right.placementMemberships) == 0 && left.placementConfig == right.placementConfig {
		return strategyQuality(&left, config, benchmark), strategyQuality(&right, config, benchmark)
	}
	return placementAccountQuality(&left, config, benchmark), placementAccountQuality(&right, config, benchmark)
}

func placementAccountQuality(item *candidate, fallback engineConfig, benchmark strategyInputs) float64 {
	if len(item.placementMemberships) == 0 {
		return placementMembershipQuality(item, item, fallback, benchmark)
	}
	quality := 0.0
	for _, membership := range item.placementMemberships {
		quality += placementMembershipQuality(item, membership, fallback, benchmark)
	}
	return quality / float64(len(item.placementMemberships))
}

func placementMembershipQuality(evidence, membership *candidate, fallback engineConfig, benchmark strategyInputs) float64 {
	config := fallback
	if membership.placementConfig != nil {
		config = *membership.placementConfig
	}
	comparison := *membership
	comparison.rate, comparison.rankingLatencyMS = evidence.rate, evidence.rankingLatencyMS
	comparison.strategy = config.strategy
	return strategyQuality(&comparison, config, benchmark) * membership.placementQualityScale
}

func automaticPlacementPriority(item *candidate, config engineConfig) (int64, bool) {
	priority, found := positivePriority(item.account.Priority)
	return priority, found && priority > config.manualPriorityMax
}

func assignStablePriorities(items []*candidate, config engineConfig) {
	slots := make([]int64, 0, len(items))
	used := make(map[int64]struct{}, len(items))
	last := config.manualPriorityMax
	for _, item := range items {
		priority, found := automaticPlacementPriority(item, config)
		if _, duplicate := used[priority]; !found || duplicate {
			continue
		}
		used[priority] = struct{}{}
		slots = append(slots, priority)
		last = max(last, priority)
	}
	for len(slots) < len(items) {
		if last == math.MaxInt64 {
			last = config.manualPriorityMax
		}
		last++
		if _, occupied := used[last]; occupied {
			continue
		}
		used[last] = struct{}{}
		slots = append(slots, last)
	}
	sort.Slice(slots, func(left, right int) bool { return slots[left] < slots[right] })
	last = config.manualPriorityMax
	for index, item := range items {
		priority := max(slots[index], min(last, math.MaxInt64-1)+1)
		penalty := int64(0)
		if item.state == "degraded" && !item.evidencePending {
			penalty = config.degradePriorityStep * int64(max(1, item.health.FailureStreak))
		} else if item.state == "survivor" {
			penalty = config.degradePriorityStep
		}
		if penalty > 0 {
			baseline := slots[index]
			if item.account.BaselinePriority != nil {
				baseline = max(config.manualPriorityMax+1, *item.account.BaselinePriority)
			} else if strings.EqualFold(item.account.EffectiveState, "degraded") || strings.EqualFold(item.account.EffectiveState, "survivor") {
				penalty = 0
			}
			priority = max(priority, min(baseline, math.MaxInt64-penalty)+penalty)
		}
		rank := index + 1
		item.rank, item.desiredPriority, item.placementPlanned = &rank, &priority, true
		last = priority
	}
}
