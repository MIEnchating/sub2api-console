package routing

import (
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

// selectScoringHistory retains the source preference established by fresh
// evidence. Expired traffic must not displace the current probe-only baseline.
func selectScoringHistory(rows, current []business.RoutingSample, now time.Time, maxAge time.Duration, limit int) []business.RoutingSample {
	if len(current) == 0 {
		return nil
	}
	for _, row := range current {
		if strings.EqualFold(strings.TrimSpace(row.Source), "traffic") {
			history, _ := selectRoutingEvidence(rows, now, maxAge, maxAge, limit)
			return history
		}
	}
	return freshSourceSamples(rows, "active-probe", now, maxAge, limit)
}

// scoreRoutingHealth separates historical scores from current safety evidence.
// Only fresh evidence can establish fatal status and failure/recovery streaks.
func scoreRoutingHealth(current, history []business.RoutingSample, policy map[string]any) (Health, error) {
	health, err := scoreRoutingRows(current, policy)
	if err != nil || health.SampleCount == 0 {
		return health, err
	}
	historical, err := scoreRoutingRows(history, policy)
	if err != nil {
		return Health{}, err
	}
	currentScore := health.HealthScore
	health.ShortScore, health.LongScore, health.HealthScore = historical.ShortScore, historical.LongScore, historical.HealthScore
	health.SampleCount = historical.SampleCount
	if health.Fatal {
		// A shorter history window must never dilute a fresh credential failure.
		health.HealthScore = currentScore
	}
	return health, nil
}

func scoreRoutingRows(rows []business.RoutingSample, policy map[string]any) (Health, error) {
	samples := make([]Sample, 0, len(rows))
	for _, row := range rows {
		samples = append(samples, Sample{Result: row.Result, FailureReason: row.FailureReason, Source: row.Source, LatencyP95: row.LatencyP95, StatusCode: routingSampleStatus(row), Payload: row.Payload})
	}
	return HealthScore(samples, policy)
}
