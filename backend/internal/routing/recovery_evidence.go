package routing

import (
	"context"
	"fmt"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

type recoveryTrafficReader interface {
	RoutingRecoveryTraffic(context.Context, string, time.Time, time.Time) ([]business.RoutingSample, error)
}

func (s *Service) extendRecoveryHold(ctx context.Context, item *candidate, config engineConfig, previous business.PreviousRoutingDecision, now time.Time) error {
	if !fusedRoutingState(item.account.EffectiveState) && !degradedRoutingState(item.account.EffectiveState, previous.State) {
		return nil
	}
	if item.health.RecoveryPassStreak < config.recoverySuccesses || recoverySpan(item.rows, item.health.RecoveryPassStreak, now) >= config.recoveryHold {
		return nil
	}
	reader, available := s.repository.(recoveryTrafficReader)
	if !available || !config.trafficEnabled {
		return nil
	}
	traffic, err := reader.RoutingRecoveryTraffic(ctx, item.account.ID, now.Add(-config.recoveryHold), now.Add(time.Minute))
	if err != nil {
		return fmt.Errorf("读取账号 %s 的恢复保持证据失败：%w", item.account.ID, err)
	}
	// The later read includes usage corrections for already observed requests.
	// These rows affect only the duration gate, never scores or sample counts.
	rows := make([]business.RoutingSample, 0, len(traffic)+len(item.rows))
	seen := map[string]struct{}{}
	for _, source := range [][]business.RoutingSample{traffic, item.rows} {
		for _, row := range source {
			if requestID := textMetadata(row.Payload, "request_id"); requestID != "" {
				key := row.Source + "\x00" + requestID
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
			}
			rows = append(rows, row)
		}
	}
	current, _ := selectRoutingEvidence(rows, now, config.trafficMaxAge, config.probeMaxAge, len(rows))
	if fusedRoutingState(item.account.EffectiveState) {
		current = withRecoveryProbeEvidence(current, rows, now, config.probeMaxAge, previousStateSince(previous))
	}
	streak := 0
	for _, row := range current {
		classified := classify(Sample{Result: row.Result, FailureReason: row.FailureReason, Source: row.Source, LatencyP95: row.LatencyP95, StatusCode: routingSampleStatus(row), Payload: row.Payload}, config.sampleClassification)
		if classified.Neutral || classified.Failure {
			break
		}
		streak++
	}
	span := recoverySpan(current, streak, now)
	item.recoveryHoldSpan = &span
	return nil
}
