package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

type GroupAllocationChannel struct {
	AccountID           string   `json:"account_id"`
	AccountName         string   `json:"account_name"`
	Health              string   `json:"health"`
	HealthScore         *float64 `json:"health_score"`
	SampleCount         int64    `json:"sample_count"`
	TTFBP95MS           *float64 `json:"ttfb_p95_ms"`
	Rate                *string  `json:"rate"`
	Priority            *int64   `json:"priority"`
	Weight              *float64 `json:"weight"`
	AssignedConcurrency *int64   `json:"assigned_concurrency"`
	Schedulable         *bool    `json:"schedulable"`
	Rank                *int64   `json:"rank"`
	Reason              *string  `json:"reason"`
	UpdatedAt           *string  `json:"updated_at"`
}

type GroupAllocation struct {
	GroupID              string                   `json:"group_id"`
	GroupName            string                   `json:"group_name"`
	Platform             *string                  `json:"platform"`
	RateMultiplier       *string                  `json:"rate_multiplier"`
	Status               string                   `json:"status"`
	ProbeIntervalSeconds int64                    `json:"probe_interval_seconds"`
	WeightBudget         int64                    `json:"weight_budget"`
	TotalWeight          float64                  `json:"total_weight"`
	HasAllocation        bool                     `json:"has_allocation"`
	Strategy             string                   `json:"strategy"`
	AccountCount         int64                    `json:"account_count"`
	HealthyAccounts      int64                    `json:"healthy_accounts"`
	AvailableAccounts    int64                    `json:"available_accounts"`
	FusedAccounts        int64                    `json:"fused_accounts"`
	PausedAccounts       int64                    `json:"paused_accounts"`
	UnavailableAccounts  int64                    `json:"unavailable_accounts"`
	RateLimitedAccounts  int64                    `json:"rate_limited_accounts"`
	PendingAccounts      int64                    `json:"pending_accounts"`
	HighestHealthScore   *float64                 `json:"highest_health_score"`
	AverageHealthScore   *float64                 `json:"average_health_score"`
	AssignedConcurrency  int64                    `json:"assigned_concurrency"`
	Channels             []GroupAllocationChannel `json:"channels"`
}

func (s *Store) GroupAllocation(ctx context.Context, groupID string) (GroupAllocation, error) {
	if !positiveNumericID(groupID) {
		return GroupAllocation{}, fmt.Errorf("分组必须使用已登记的稳定数字 ID")
	}
	group, err := s.groupByID(ctx, groupID)
	if err != nil {
		return GroupAllocation{}, err
	}
	mode, err := s.Mode(ctx)
	if err != nil {
		return GroupAllocation{}, err
	}
	excludedIDs, degradeThreshold, err := s.monitorPolicy(ctx, mode)
	if err != nil {
		return GroupAllocation{}, err
	}
	result := GroupAllocation{
		GroupID: groupID, GroupName: group.Name, Platform: group.Platform, RateMultiplier: group.RateMultiplier,
		Status: group.Status, ProbeIntervalSeconds: group.ProbeInterval, WeightBudget: group.WeightBudget, Strategy: group.Strategy,
		AccountCount: group.AccountCount, RateLimitedAccounts: group.RateLimitedAccounts,
		AverageHealthScore: group.AverageHealthScore, Channels: []GroupAllocationChannel{},
	}
	rows, err := s.db.QueryContext(ctx, `SELECT a.id,a.name,a.multiplier,rd.priority,rd.schedulable,rd.role,rd.routing_state,
		rd.rank,rd.reason,rd.updated_at,rd.payload_json,he.health_score,he.sample_count,he.ttfb_p95_ms,
		a.schedulable,a.routing_state,a.health_status,a.paused,a.metadata_json
		FROM account_groups ag
		JOIN accounts a ON a.id=ag.account_id
		LEFT JOIN routing_decisions rd ON rd.account_id=a.id AND rd.updated_at>=COALESCE(
			(SELECT updated_at FROM app_state WHERE key='routing-decision-epoch'),rd.updated_at)
		LEFT JOIN account_health_evaluations he ON he.account_id=a.id
		WHERE ag.group_id=? OR (ag.group_id IS NULL AND LOWER(TRIM(ag.group_name))=LOWER(TRIM(?)))
		ORDER BY a.name,a.id`, groupID, group.Name)
	if err != nil {
		return GroupAllocation{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var channel GroupAllocationChannel
		var priority, schedulable, rank, sampleCount, accountSchedulable, paused sql.NullInt64
		var accountMultiplier, role, state, reason, updatedAt, payloadRaw sql.NullString
		var accountRoutingState, accountHealthStatus sql.NullString
		var metadataRaw string
		var healthScore, p95 sql.NullFloat64
		if err := rows.Scan(&channel.AccountID, &channel.AccountName, &accountMultiplier, &priority, &schedulable, &role, &state,
			&rank, &reason, &updatedAt, &payloadRaw, &healthScore, &sampleCount, &p95,
			&accountSchedulable, &accountRoutingState, &accountHealthStatus, &paused, &metadataRaw); err != nil {
			return GroupAllocation{}, err
		}
		channel.Priority, channel.Schedulable, channel.Rank = nullInt(priority), strictBool(schedulable), nullInt(rank)
		channel.Reason, channel.UpdatedAt = nullString(reason), nullString(updatedAt)
		channel.HealthScore, channel.TTFBP95MS = nullFiniteFloat(healthScore), nullFiniteFloat(p95)
		if sampleCount.Valid && sampleCount.Int64 > 0 {
			channel.SampleCount = sampleCount.Int64
		}
		account := accountProjection{AccountStatus: AccountStatus{
			ID: channel.AccountID, Schedulable: strictBool(accountSchedulable), RoutingState: nullString(accountRoutingState),
			HealthStatus: nullString(accountHealthStatus), Paused: strictBool(paused),
		}, metadataRaw: metadataRaw}
		decisions := []decisionProjection{}
		if updatedAt.Valid {
			decisionState := role.String
			if state.Valid && strings.TrimSpace(state.String) != "" {
				decisionState = state.String
			}
			decisions = append(decisions, decisionProjection{state: decisionState, reason: nullString(reason), updatedAt: nullString(updatedAt)})
		}
		evaluations := []evaluationProjection{}
		if sampleCount.Valid || healthScore.Valid || p95.Valid {
			evaluations = append(evaluations, evaluationProjection{
				healthScore: nullFiniteFloat(healthScore), sampleCount: sampleCount.Int64, p95: nullFiniteFloat(p95),
			})
		}
		applyAccountCalculations(&account, decisions, evaluations, struct {
			message string
			at      *string
		}{}, routingApplyView{fields: map[string]bool{}})
		if mode == "monitoring" {
			applyMonitoringHealth(&account, excludedIDs, degradeThreshold)
		}
		channel.Health = account.Health
		if channel.UpdatedAt == nil {
			channel.Schedulable = account.Schedulable
		}
		if payloadRaw.Valid {
			payload, decodeErr := decodeObject(payloadRaw.String)
			if decodeErr == nil {
				channel.Weight = finiteFloat(payload["weight"])
				channel.AssignedConcurrency = allocationInteger(payload["desired_concurrency"])
				channel.Rate = allocationString(payload["rate"])
			}
		}
		if accountMultiplier.Valid && strings.TrimSpace(accountMultiplier.String) != "" {
			channel.Rate = stringPointer(strings.TrimSpace(accountMultiplier.String))
		}
		if channel.UpdatedAt != nil {
			result.HasAllocation = true
		}
		if channel.HealthScore != nil && (result.HighestHealthScore == nil || *channel.HealthScore > *result.HighestHealthScore) {
			value := *channel.HealthScore
			result.HighestHealthScore = &value
		}
		if channel.AssignedConcurrency != nil && *channel.AssignedConcurrency > 0 {
			result.AssignedConcurrency += *channel.AssignedConcurrency
		}
		if channel.Weight != nil {
			result.TotalWeight += *channel.Weight
		}
		classifyAllocationChannel(&result, channel)
		result.Channels = append(result.Channels, channel)
	}
	if err := rows.Err(); err != nil {
		return GroupAllocation{}, err
	}
	sort.SliceStable(result.Channels, func(left, right int) bool {
		leftWeight, rightWeight := result.Channels[left].Weight, result.Channels[right].Weight
		if leftWeight != nil || rightWeight != nil {
			if leftWeight == nil {
				return false
			}
			if rightWeight == nil {
				return true
			}
			if *leftWeight != *rightWeight {
				return *leftWeight > *rightWeight
			}
		}
		return strings.ToLower(result.Channels[left].AccountName) < strings.ToLower(result.Channels[right].AccountName)
	})
	return result, nil
}

func classifyAllocationChannel(allocation *GroupAllocation, channel GroupAllocationChannel) {
	switch channel.Health {
	case AccountStateFused:
		allocation.FusedAccounts++
	case AccountStatePaused:
		allocation.PausedAccounts++
	case AccountStateDisabled, AccountStateExcluded:
		allocation.UnavailableAccounts++
	case AccountStateUnknown:
		if channel.SampleCount == 0 {
			allocation.PendingAccounts++
		} else {
			allocation.UnavailableAccounts++
		}
	case AccountStateHealthy:
		if channel.Schedulable != nil && *channel.Schedulable {
			allocation.HealthyAccounts++
		} else {
			allocation.UnavailableAccounts++
		}
	}
	if channel.Schedulable != nil && *channel.Schedulable &&
		channel.Health != AccountStateFused && channel.Health != AccountStatePaused &&
		channel.Health != AccountStateDisabled && channel.Health != AccountStateExcluded {
		allocation.AvailableAccounts++
	}
}

func allocationInteger(value any) *int64 {
	number := finiteFloat(value)
	if number == nil || *number < 0 || math.Trunc(*number) != *number || *number > math.MaxInt64 {
		return nil
	}
	result := int64(*number)
	return &result
}

func allocationString(value any) *string {
	if value == nil {
		return nil
	}
	if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
		result := strings.TrimSpace(text)
		return &result
	}
	if number, ok := value.(json.Number); ok {
		result := number.String()
		return &result
	}
	return nil
}
