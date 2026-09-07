package routing

import (
	"math/big"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func pendingHealthCandidate(score float64) *candidate {
	schedulable := true
	return &candidate{
		account: business.RoutingAccount{ID: "41", GroupName: "codex", Schedulable: &schedulable, Metadata: map[string]any{}},
		health:  Health{HealthScore: score, SampleCount: 1, LatestEvent: EventGateway, FailureStreak: 1, Events: []Event{EventGateway}},
		state:   "healthy", rate: big.NewRat(1, 1), strategy: "balanced",
	}
}

func TestPendingFailureRepeatedEvaluationDoesNotCompoundPenalty(t *testing.T) {
	config := engineConfig{httpWindow: 5, httpFailures: 3, transientFailures: 2, degradeEnabled: true, degradeThreshold: 75, gateFloor: 40}
	previous := business.PreviousRoutingDecision{State: "healthy", Payload: map[string]any{"routing_health_score": 100.0}}
	for round := 0; round < 3; round++ {
		item := pendingHealthCandidate(40)
		applyInitialState(item, config, previous, time.Now().UTC())
		if item.state != "degraded" || !item.evidencePending || item.routingHealth != 70 || !item.schedulable {
			t.Fatalf("round %d: pending degradation=%+v", round, item)
		}
		previous = business.PreviousRoutingDecision{State: item.state, Payload: decisionWrite(publicDecision(item, "codex", config, time.Now().UTC())).Payload}
	}
}

func TestPrimaryMembershipSharesPendingPenaltyBeforeGroupWeightCalculation(t *testing.T) {
	primary, secondary := pendingHealthCandidate(40), pendingHealthCandidate(40)
	primary.account.GroupName, secondary.account.GroupName = "group-a", "group-b"
	primaryConfig := engineConfig{httpWindow: 5, httpFailures: 3, transientFailures: 2, degradeEnabled: true, degradeThreshold: 75, gateFloor: 40}
	secondaryConfig := primaryConfig
	secondaryConfig.transientFailures = 1
	previous := business.PreviousRoutingDecision{State: "healthy", Payload: map[string]any{"routing_health_score": 100.0}}
	applyInitialState(primary, primaryConfig, previous, time.Now().UTC())
	applyInitialState(secondary, secondaryConfig, previous, time.Now().UTC())
	alignAccountStateToPrimary(map[string][]*candidate{"41": {primary, secondary}}, map[string]engineConfig{"group-a": primaryConfig, "group-b": secondaryConfig}, nil, time.Now().UTC())
	if primary.routingHealth != 70 || secondary.routingHealth != 70 || !primary.evidencePending || !secondary.evidencePending {
		t.Fatalf("memberships disagree before weights: primary=%v/%v secondary=%v/%v", primary.routingHealth, primary.evidencePending, secondary.routingHealth, secondary.evidencePending)
	}
}

func TestSecondConsecutiveFailureUsesFullHealthPenalty(t *testing.T) {
	item := pendingHealthCandidate(40)
	item.health.FailureStreak, item.health.SampleCount = 2, 2
	item.health.Events = []Event{EventGateway, EventGateway}
	config := engineConfig{httpWindow: 5, httpFailures: 3, transientFailures: 2, degradeEnabled: true, degradeThreshold: 75, gateFloor: 40}
	previous := business.PreviousRoutingDecision{State: "degraded", Payload: map[string]any{"routing_health_score": 70.0, "evidence_pending": true}}
	applyInitialState(item, config, previous, time.Now().UTC())
	if item.state != "degraded" || item.evidencePending || item.routingHealth != 40 {
		t.Fatalf("confirmed failure retained partial penalty: %+v", item)
	}
}

func TestNeutralOnlyEvidenceDoesNotEscalatePendingDegradation(t *testing.T) {
	item := pendingHealthCandidate(0)
	item.health = Health{NeutralCount: 1}
	config := engineConfig{degradeEnabled: true, degradeThreshold: 75}
	previous := business.PreviousRoutingDecision{State: "degraded", Payload: map[string]any{"routing_health_score": 70.0, "evidence_pending": true}}
	applyInitialState(item, config, previous, time.Now().UTC())
	if item.state != "degraded" || !item.evidencePending || item.routingHealth != 70 {
		t.Fatalf("neutral evidence escalated pending degradation: %+v", item)
	}
}

func TestHealthDegradationRespectsThresholdSampleCountAndEnabledSwitch(t *testing.T) {
	for _, tc := range []struct {
		name    string
		score   float64
		count   int
		enabled bool
		state   string
	}{
		{"below threshold", 74, 1, true, "degraded"},
		{"at threshold", 75, 1, true, "healthy"},
		{"disabled", 40, 1, false, "healthy"},
		{"no evidence", 0, 0, true, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := pendingHealthCandidate(tc.score)
			item.health = Health{HealthScore: tc.score, SampleCount: tc.count}
			applyInitialState(item, engineConfig{degradeEnabled: tc.enabled, degradeThreshold: 75}, business.PreviousRoutingDecision{}, time.Now().UTC())
			if item.state != tc.state || !item.schedulable {
				t.Fatalf("state=%s schedulable=%v", item.state, item.schedulable)
			}
		})
	}
}

func TestConfirmedDegradationDoesNotReturnToPendingOnIsolatedFailure(t *testing.T) {
	item := pendingHealthCandidate(40)
	config := engineConfig{httpWindow: 5, httpFailures: 3, transientFailures: 2, degradeEnabled: true, degradeThreshold: 75, gateFloor: 40}
	previous := business.PreviousRoutingDecision{State: "degraded", Payload: map[string]any{"routing_health_score": 40.0, "evidence_pending": false}}
	applyInitialState(item, config, previous, time.Now().UTC())
	if item.evidencePending || item.routingHealth != 40 || item.state != "degraded" {
		t.Fatalf("confirmed degradation weakened before recovery: %+v", item)
	}
}

func TestLowScoreSlowResponseDegradesWithoutBreakerLatencyThreshold(t *testing.T) {
	item := pendingHealthCandidate(65)
	item.health.LatestEvent, item.health.Events, item.health.FailureStreak = EventSlow, []Event{EventSlow}, 0
	config := engineConfig{degradeEnabled: true, degradeThreshold: 75}
	applyInitialState(item, config, business.PreviousRoutingDecision{}, time.Now().UTC())
	if item.state != "degraded" || item.routingHealth != 65 || item.evidencePending {
		t.Fatalf("low slow-response health must degrade independently of breaker: %+v", item)
	}
}

func TestPendingFailureRetainsPositiveButLowerWeightWithHighGateFloor(t *testing.T) {
	config := engineConfig{httpWindow: 5, httpFailures: 3, transientFailures: 2, degradeEnabled: true, degradeThreshold: 75, gateFloor: 90, weightBudget: 300, balancedPriceRatio: .5}
	item, healthy := pendingHealthCandidate(40), pendingHealthCandidate(100)
	healthy.account.ID, healthy.health.Events, healthy.health.LatestEvent, healthy.health.FailureStreak = "42", []Event{EventHealthy}, EventHealthy, 0
	for _, candidate := range []*candidate{item, healthy} {
		applyInitialState(candidate, config, business.PreviousRoutingDecision{State: "healthy", Payload: map[string]any{"routing_health_score": 100.0}}, time.Now().UTC())
	}
	calculateGroupWeights([]*candidate{item, healthy}, config)
	if item.weight != 100 || healthy.weight != 200 {
		t.Fatalf("pending penalty should halve quality, pending=%v healthy=%v", item.weight, healthy.weight)
	}
}

func TestPendingDegradationAvoidsAdditionalPriorityLoadAndConcurrencyPenalties(t *testing.T) {
	priority, concurrency := int64(20), int64(30)
	item := pendingHealthCandidate(40)
	item.state, item.evidencePending, item.schedulable, item.weight = "degraded", true, true, 100
	item.account.Priority, item.account.Concurrency = &priority, &concurrency
	config := engineConfig{weightsEnabled: true, weightBudget: 200, minLoadFactor: 10, maxLoadFactor: 90, degradePriorityStep: 10, degradeLoadRatio: .5,
		scalingEnabled: true, scalingGlobalMax: 100, scalingMin: 3, scalingMax: 50, scalingStepDown: 5, scalingStepUp: 5, scalingUpRatio: .1}
	assignAccountPlacements(map[string][]*candidate{"codex": {item}}, map[string]engineConfig{"codex": config}, map[string][]*candidate{"41": {item}})
	if item.desiredPriority == nil || *item.desiredPriority != 20 || item.desiredLoad == nil || *item.desiredLoad != "25" || item.desiredConcurrency != nil {
		t.Fatalf("pending degradation stacked penalties: priority=%v load=%v concurrency=%v", item.desiredPriority, item.desiredLoad, item.desiredConcurrency)
	}
}
