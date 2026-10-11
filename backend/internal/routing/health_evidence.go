package routing

import (
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

type routingHealthEvidence struct {
	health             Health
	rows               []business.RoutingSample
	performanceP50MS   *float64
	performanceP95MS   *float64
	performanceSamples int
	performanceModel   string
}

func evaluateRoutingHealth(
	account business.RoutingAccount,
	samples []business.RoutingSample,
	previous business.PreviousRoutingDecision,
	config engineConfig,
	now time.Time,
) routingHealthEvidence {
	source := "active_probe"
	if config.trafficEnabled {
		source = "traffic"
	}
	rows := filterAndLimitSamples(samples, source, config.sampleWindow()*2)
	healthRows, performanceRows := selectRoutingEvidence(
		rows, now, config.trafficMaxAge, config.probeMaxAge, config.sampleWindow(),
	)
	placementPerformanceRows := performanceRows[:min(max(config.longWindow, config.performanceMinSamples), len(performanceRows))]
	performanceRows = performanceRows[:min(config.longWindow, len(performanceRows))]
	if fusedRoutingState(account.EffectiveState) {
		healthRows = withRecoveryProbeEvidence(healthRows, rows, now, config.probeMaxAge, previousStateSince(previous))
	}
	healthRows = withCriticalProbeEvidence(healthRows, rows, now, config.probeMaxAge, config.sampleClassification)
	historyRows := selectScoringHistory(rows, healthRows, now, config.historyMaxAge, config.longWindow)
	if fusedRoutingState(account.EffectiveState) {
		// Recovery must use fresh post-fuse evidence instead of pre-fuse history.
		historyRows = healthRows
	}
	health := scoreRoutingHealth(healthRows, historyRows, config.sampleClassification)
	health.P50MS, health.P95MS, _, _ = performanceLatencySummary(performanceRows)
	p50, p95, count, model := performanceLatencySummary(placementPerformanceRows)
	return routingHealthEvidence{
		health: health, rows: healthRows, performanceP50MS: p50, performanceP95MS: p95,
		performanceSamples: count, performanceModel: model,
	}
}
