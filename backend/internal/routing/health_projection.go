package routing

import (
	"context"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

type HealthProjection struct {
	HealthScore       *float64 `json:"health_score"`
	ShortScore        *float64 `json:"short_score"`
	LongScore         *float64 `json:"long_score"`
	SampleCount       int64    `json:"sample_count"`
	ShortSampleCount  *int64   `json:"short_sample_count"`
	LongSampleCount   int64    `json:"long_sample_count"`
	TTFBP50MS         *float64 `json:"ttfb_p50_ms"`
	TTFBP95MS         *float64 `json:"ttfb_p95_ms"`
	HealthEvaluatedAt string   `json:"health_evaluated_at"`
	HealthEvidenceAt  *string  `json:"health_evidence_at"`
}

// ProjectHealth reads current evidence without advancing routing state or
// saving evaluations. Accounts outside the evaluator's scope have no entry.
func (s *Service) ProjectHealth(ctx context.Context, scope Scope) (map[string]HealthProjection, error) {
	policy, err := s.repository.ControlPolicy(ctx)
	if err != nil {
		return nil, err
	}
	config, err := parseEngineConfig(policy)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	if reader, ok := s.repository.(interface {
		ReadSnapshotTime(context.Context) (time.Time, bool)
	}); ok {
		if snapshotTime, found := reader.ReadSnapshotTime(ctx); found {
			now = snapshotTime.UTC()
		}
	}
	accounts, err := s.repository.RoutingAccounts(ctx, scope.AccountID, scope.GroupName)
	if err != nil {
		return nil, err
	}
	source := "active_probe"
	if config.trafficEnabled {
		source = "traffic"
	}
	samples, err := s.repository.RoutingSamples(ctx, scope.AccountID, scope.GroupName, source, config.sampleWindow())
	if err != nil {
		return nil, err
	}
	previousRows, err := s.previousHealthDecisions(ctx, scope)
	if err != nil {
		return nil, err
	}
	sampleMap := map[string][]business.RoutingSample{}
	for _, sample := range samples {
		sampleMap[sample.AccountID] = append(sampleMap[sample.AccountID], sample)
	}
	previous := map[string]business.PreviousRoutingDecision{}
	for _, item := range previousRows {
		previous[routingKey(item.AccountID, item.GroupName)] = item
		current, found := previous[item.AccountID]
		if !found || item.GroupName < current.GroupName {
			previous[item.AccountID] = item
		}
	}
	byAccount := map[string][]*candidate{}
	configs := map[string]engineConfig{}
	for _, account := range accounts {
		if account.ManualPriority != nil || (!config.manageAllAccounts && (account.ExternalControl || accountExternallyModified(account))) {
			continue
		}
		groupConfig, enabled, err := config.forGroup(account.GroupID)
		if err != nil {
			return nil, err
		}
		if !enabled || groupConfig.groupExcluded(account.GroupID) || !groupConfig.groupManaged(account.GroupID) || !accountMetadataManaged(account, groupConfig) {
			continue
		}
		configs[account.GroupName] = groupConfig
		byAccount[account.ID] = append(byAccount[account.ID], &candidate{account: account})
	}
	result := make(map[string]HealthProjection, len(byAccount))
	for accountID, memberships := range byAccount {
		primary := primaryMembership(memberships)
		groupConfig := configs[primary.account.GroupName]
		prior, _ := previousDecision(previous, accountID, primary.account.GroupName)
		evidence, err := evaluateRoutingHealth(primary.account, sampleMap[accountID], prior, groupConfig, policy, now)
		if err != nil {
			return nil, err
		}
		shortCount := int64(min(groupConfig.shortWindow, evidence.health.SampleCount))
		projection := HealthProjection{
			SampleCount: int64(evidence.health.SampleCount), ShortSampleCount: &shortCount,
			LongSampleCount: int64(evidence.health.SampleCount), HealthEvaluatedAt: now.Format(time.RFC3339Nano),
		}
		if evidence.health.SampleCount > 0 {
			projection.HealthScore = floatPointer(evidence.health.HealthScore)
			projection.ShortScore = floatPointer(evidence.health.ShortScore)
			projection.LongScore = floatPointer(evidence.health.LongScore)
			projection.TTFBP50MS, projection.TTFBP95MS = evidence.health.P50MS, evidence.health.P95MS
			projection.HealthEvidenceAt = latestScoredEvidenceAt(evidence.rows, groupConfig.sampleClassification)
		}
		result[accountID] = projection
	}
	return result, nil
}

func (s *Service) previousHealthDecisions(ctx context.Context, scope Scope) ([]business.PreviousRoutingDecision, error) {
	if reader, ok := s.repository.(interface {
		PreviousHealthDecisions(context.Context, *string, *string) ([]business.PreviousRoutingDecision, error)
	}); ok {
		return reader.PreviousHealthDecisions(ctx, scope.AccountID, scope.GroupName)
	}
	return s.repository.PreviousRoutingDecisions(ctx, scope.AccountID, scope.GroupName)
}

func latestScoredEvidenceAt(rows []business.RoutingSample, config scoringConfig) *string {
	for _, row := range rows {
		classified := classify(Sample{
			Result: row.Result, FailureReason: row.FailureReason, Source: row.Source,
			LatencyP95: row.LatencyP95, StatusCode: routingSampleStatus(row), Payload: row.Payload,
		}, config)
		if classified.Neutral {
			continue
		}
		if latest, err := time.Parse(time.RFC3339Nano, row.ObservedAt); err == nil {
			observedAt := latest.UTC().Format(time.RFC3339Nano)
			return &observedAt
		}
	}
	return nil
}
