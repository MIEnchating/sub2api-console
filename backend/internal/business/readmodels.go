package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

type AccountRecentResult struct {
	Result                *string        `json:"result"`
	EventType             *string        `json:"event_type"`
	Score                 *float64       `json:"score"`
	ObservedAt            *string        `json:"observed_at"`
	LatencyMS             *float64       `json:"latency_ms"`
	FailureReason         *string        `json:"failure_reason"`
	Source                string         `json:"source"`
	ClassificationLatency *string        `json:"-"`
	ClassificationPayload map[string]any `json:"-"`
}

type AccountStatus struct {
	ID                  string                `json:"id"`
	Name                string                `json:"name"`
	Groups              []string              `json:"groups"`
	UpstreamHost        *string               `json:"upstream_host"`
	UpstreamType        *string               `json:"upstream_type"`
	Platform            *string               `json:"platform"`
	AccountType         *string               `json:"account_type"`
	Schedulable         *bool                 `json:"schedulable"`
	Priority            *int64                `json:"priority"`
	LoadFactor          *string               `json:"load_factor"`
	Concurrency         *int64                `json:"concurrency"`
	Multiplier          *string               `json:"multiplier"`
	Balance             *string               `json:"balance"`
	Paused              *bool                 `json:"paused"`
	PausedReason        *string               `json:"paused_reason"`
	RoutingState        *string               `json:"routing_state"`
	HealthStatus        *string               `json:"health_status"`
	Health              string                `json:"health"`
	DesiredHealth       *string               `json:"desired_health"`
	ApplyPending        bool                  `json:"apply_pending"`
	ApplyError          *string               `json:"apply_error"`
	DecisionState       *string               `json:"decision_state"`
	DecisionReason      *string               `json:"decision_reason"`
	LastError           *string               `json:"last_error"`
	UpstreamBlock       *string               `json:"upstream_block"`
	UpstreamBlockReason *string               `json:"upstream_block_reason"`
	FailureStreak       *int64                `json:"failure_streak"`
	RecoveryPassStreak  *int64                `json:"recovery_pass_streak"`
	TargetPriority      *int64                `json:"target_priority"`
	TargetLoadFactor    *string               `json:"target_load_factor"`
	TargetSchedulable   *bool                 `json:"target_schedulable"`
	TargetConcurrency   *int64                `json:"target_concurrency"`
	HealthScore         *float64              `json:"health_score"`
	ShortScore          *float64              `json:"short_score"`
	LongScore           *float64              `json:"long_score"`
	SampleCount         int64                 `json:"sample_count"`
	RecentResults       []AccountRecentResult `json:"recent_results"`
	TTFBP50MS           *float64              `json:"ttfb_p50_ms"`
	TTFBP95MS           *float64              `json:"ttfb_p95_ms"`
	Weight              *float64              `json:"weight"`
}

type AccountBinding struct {
	ID               int64   `json:"id"`
	LocalAccountID   string  `json:"local_account_id"`
	UpstreamHost     string  `json:"upstream_host"`
	UpstreamKeyID    string  `json:"upstream_key_id"`
	UpstreamKeyName  string  `json:"upstream_key_name"`
	UpstreamGroup    *string `json:"upstream_group"`
	UpstreamGroupID  *string `json:"upstream_group_id"`
	LocalGroup       string  `json:"local_group"`
	LocalRate        *string `json:"local_rate"`
	UpstreamRate     *string `json:"upstream_rate"`
	SourceAuthHost   *string `json:"source_auth_host,omitempty"`
	BindingHostAlias *string `json:"binding_host_alias,omitempty"`
	Description      *string `json:"description"`
	Status           *string `json:"status"`
	UpdatedAt        string  `json:"updated_at"`
}

type AccountDetail struct {
	AccountStatus
	Metadata   map[string]any     `json:"metadata"`
	GroupRates map[string]*string `json:"group_rates"`
	GroupIDs   map[string]*string `json:"group_ids"`
	Bindings   []AccountBinding   `json:"bindings"`
	TestModel  *string            `json:"test_model"`
}

type accountProjection struct {
	AccountStatus
	metadataRaw  string
	groupIDs     map[string]*string
	groupRates   map[string]*string
	latestEvents map[string]string
}

type decisionProjection struct {
	state     string
	reason    *string
	updatedAt *string
	weight    *float64
}

type evaluationProjection struct {
	healthScore *float64
	shortScore  *float64
	longScore   *float64
	sampleCount int64
	p50         *float64
	p95         *float64
}

type routingApplyView struct {
	fields    map[string]bool
	automatic bool
}

func (s *Store) Accounts(ctx context.Context) ([]AccountStatus, error) {
	projections, err := s.accountProjections(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]AccountStatus, len(projections))
	for index := range projections {
		result[index] = projections[index].AccountStatus
	}
	return result, nil
}

func (s *Store) Account(ctx context.Context, accountID string) (*AccountDetail, error) {
	normalized := strings.TrimSpace(accountID)
	if !positiveNumericID(normalized) {
		return nil, errors.New("账号必须使用有效的稳定 ID")
	}
	projections, err := s.accountProjections(ctx)
	if err != nil {
		return nil, err
	}
	var selected *accountProjection
	for index := range projections {
		if projections[index].ID == normalized {
			selected = &projections[index]
			break
		}
	}
	if selected == nil {
		return nil, sql.ErrNoRows
	}
	metadata, err := decodeObject(selected.metadataRaw)
	if err != nil {
		metadata = map[string]any{"_invalid_configuration": []string{"account.metadata_json"}}
	}
	bindings, err := s.accountBindings(ctx, normalized)
	if err != nil {
		return nil, err
	}
	var testModel *string
	if policy, policyErr := s.readPolicyDocument(ctx, s.db, "control-plane"); policyErr == nil && policy != nil {
		if models, ok := policy["account_test_models"].(map[string]any); ok {
			if value, ok := models[normalized].(string); ok && strings.TrimSpace(value) != "" {
				normalizedModel := strings.TrimSpace(value)
				testModel = &normalizedModel
			}
		}
	}
	return &AccountDetail{
		AccountStatus: selected.AccountStatus,
		Metadata:      metadata,
		GroupRates:    selected.groupRates,
		GroupIDs:      selected.groupIDs,
		Bindings:      bindings,
		TestModel:     testModel,
	}, nil
}

func (s *Store) accountBindings(ctx context.Context, accountID string) ([]AccountBinding, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,local_account_id,upstream_host,upstream_key_id,
		upstream_key_name,upstream_group,upstream_group_id,local_group,local_rate,upstream_rate,
		source_auth_host,binding_host_alias,description,status,updated_at
		FROM bindings WHERE local_account_id=? ORDER BY id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AccountBinding, 0)
	for rows.Next() {
		var item AccountBinding
		var upstreamGroup, upstreamGroupID, localRate, upstreamRate sql.NullString
		var sourceAuthHost, bindingHostAlias, description, status sql.NullString
		if err := rows.Scan(
			&item.ID, &item.LocalAccountID, &item.UpstreamHost, &item.UpstreamKeyID, &item.UpstreamKeyName,
			&upstreamGroup, &upstreamGroupID, &item.LocalGroup, &localRate, &upstreamRate,
			&sourceAuthHost, &bindingHostAlias, &description, &status, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.UpstreamGroup = nullString(upstreamGroup)
		item.UpstreamGroupID = nullString(upstreamGroupID)
		item.LocalRate = nullString(localRate)
		item.UpstreamRate = nullString(upstreamRate)
		item.SourceAuthHost = nullString(sourceAuthHost)
		item.BindingHostAlias = nullString(bindingHostAlias)
		item.Description = nullString(description)
		item.Status = nullString(status)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) accountProjections(ctx context.Context) ([]accountProjection, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,upstream_host,upstream_type,schedulable,priority,
		load_factor,concurrency,multiplier,balance,paused,paused_reason,routing_state,health_status,
		failure_streak,recovery_pass_streak,target_priority,target_load_factor,target_schedulable,
		target_concurrency,metadata_json FROM accounts
		ORDER BY CASE WHEN id GLOB '[0-9]*' THEN CAST(id AS INTEGER) ELSE 0 END,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	projections := make([]accountProjection, 0)
	for rows.Next() {
		var item accountProjection
		var upstreamHost, upstreamType, loadFactor, multiplier, balance sql.NullString
		var pausedReason, routingState, healthStatus, targetLoadFactor sql.NullString
		var schedulable, paused, targetSchedulable sql.NullInt64
		var priority, concurrency, failureStreak, recoveryPassStreak sql.NullInt64
		var targetPriority, targetConcurrency sql.NullInt64
		if err := rows.Scan(
			&item.ID, &item.Name, &upstreamHost, &upstreamType, &schedulable, &priority,
			&loadFactor, &concurrency, &multiplier, &balance, &paused, &pausedReason, &routingState,
			&healthStatus, &failureStreak, &recoveryPassStreak, &targetPriority, &targetLoadFactor,
			&targetSchedulable, &targetConcurrency, &item.metadataRaw,
		); err != nil {
			return nil, err
		}
		item.Groups = []string{}
		item.RecentResults = []AccountRecentResult{}
		item.groupIDs = map[string]*string{}
		item.groupRates = map[string]*string{}
		item.latestEvents = map[string]string{}
		item.UpstreamHost = nullString(upstreamHost)
		item.UpstreamType = nullString(upstreamType)
		item.Platform = accountMetadataText(item.metadataRaw, "platform")
		item.AccountType = accountMetadataText(item.metadataRaw, "account_type", "type")
		if item.AccountType == nil {
			item.AccountType = item.UpstreamType
		}
		item.Schedulable = strictBool(schedulable)
		item.LastError = accountMetadataText(item.metadataRaw, "error_message", "last_error")
		if metadata, decodeErr := decodeObject(item.metadataRaw); decodeErr == nil {
			block, reason := AccountUpstreamBlockDetails(metadata, item.Schedulable, time.Now())
			if block != "" {
				item.UpstreamBlock = stringPointer(block)
				item.UpstreamBlockReason = stringPointer(reason)
			}
		}
		item.Priority = nullInt(priority)
		item.LoadFactor = nullString(loadFactor)
		item.Concurrency = nullInt(concurrency)
		item.Multiplier = nullString(multiplier)
		item.Balance = nullString(balance)
		item.Paused = strictBool(paused)
		item.PausedReason = nullString(pausedReason)
		item.RoutingState = nullString(routingState)
		item.HealthStatus = nullString(healthStatus)
		item.FailureStreak = nullInt(failureStreak)
		item.RecoveryPassStreak = nullInt(recoveryPassStreak)
		item.TargetPriority = nullInt(targetPriority)
		item.TargetLoadFactor = nullString(targetLoadFactor)
		item.TargetSchedulable = strictBool(targetSchedulable)
		item.TargetConcurrency = nullInt(targetConcurrency)
		projections = append(projections, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	byID := make(map[string]*accountProjection, len(projections))
	for index := range projections {
		byID[projections[index].ID] = &projections[index]
	}
	if err := s.loadAccountGroups(ctx, byID); err != nil {
		return nil, err
	}
	decisions, err := s.loadAccountDecisions(ctx, byID)
	if err != nil {
		return nil, err
	}
	evaluations, err := s.loadAccountEvaluations(ctx, byID)
	if err != nil {
		return nil, err
	}
	if err := s.loadRecentEvidence(ctx, byID); err != nil {
		return nil, err
	}
	for index := range projections {
		item := &projections[index]
		if item.LastError != nil {
			continue
		}
		for _, result := range item.RecentResults {
			if result.FailureReason != nil {
				item.LastError = result.FailureReason
				break
			}
		}
	}
	applyErrors, err := s.loadApplyErrors(ctx)
	if err != nil {
		return nil, err
	}
	mode, err := s.Mode(ctx)
	if err != nil {
		return nil, err
	}
	applyView, err := s.routingApplyView(ctx, mode)
	if err != nil {
		return nil, err
	}
	excludedIDs, degradeThreshold, err := s.monitorPolicy(ctx, mode)
	if err != nil {
		return nil, err
	}
	for index := range projections {
		item := &projections[index]
		applyAccountCalculations(item, decisions[item.ID], evaluations[item.ID], applyErrors[item.ID], applyView)
		if mode == runtimepolicy.Monitoring {
			applyMonitoringHealth(item, excludedIDs, degradeThreshold)
		}
	}
	return projections, nil
}

func (s *Store) routingApplyView(ctx context.Context, mode string) (routingApplyView, error) {
	result := routingApplyView{fields: map[string]bool{}, automatic: mode == runtimepolicy.Full}
	if mode == runtimepolicy.Monitoring {
		return result, nil
	}
	policy, err := s.readPolicyDocument(ctx, s.db, "control-plane")
	if err != nil {
		return routingApplyView{}, err
	}
	rawAutoApply, present := policy["auto_apply"]
	if !present {
		for _, field := range []string{"schedulable", "priority", "load_factor", "concurrency"} {
			result.fields[field] = true
		}
		return result, nil
	}
	autoApply, ok := rawAutoApply.(map[string]any)
	if !ok {
		return routingApplyView{}, errors.New("调度策略 auto_apply 配置无效")
	}
	for _, field := range []string{"schedulable", "priority", "load_factor", "concurrency"} {
		raw, present := autoApply[field]
		if !present {
			continue
		}
		value, ok := raw.(bool)
		if !ok {
			return routingApplyView{}, fmt.Errorf("调度策略 auto_apply.%s 配置无效", field)
		}
		result.fields[field] = value
	}
	return result, nil
}

func (s *Store) loadAccountGroups(ctx context.Context, accounts map[string]*accountProjection) error {
	rows, err := s.db.QueryContext(ctx, `SELECT account_id,group_name,group_id,group_rate
		FROM account_groups ORDER BY account_id,group_name`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var accountID, groupName string
		var groupID, groupRate sql.NullString
		if err := rows.Scan(&accountID, &groupName, &groupID, &groupRate); err != nil {
			return err
		}
		if item := accounts[accountID]; item != nil {
			item.Groups = append(item.Groups, groupName)
			item.groupIDs[groupName] = nullString(groupID)
			item.groupRates[groupName] = nullString(groupRate)
		}
	}
	return rows.Err()
}

func (s *Store) loadAccountDecisions(ctx context.Context, accounts map[string]*accountProjection) (map[string][]decisionProjection, error) {
	query := `SELECT account_id,group_name,routing_state,role,reason,updated_at,payload_json
		FROM routing_decisions`
	args := []any{}
	var epoch string
	err := s.db.QueryRowContext(ctx, `SELECT updated_at FROM app_state WHERE key='routing-decision-epoch'`).Scan(&epoch)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		query += ` WHERE julianday(updated_at)>=julianday(?)`
		args = append(args, epoch)
	}
	query += ` ORDER BY updated_at DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string][]decisionProjection)
	for rows.Next() {
		var accountID, groupName, payloadRaw string
		var routingState, role, reason, updatedAt sql.NullString
		if err := rows.Scan(&accountID, &groupName, &routingState, &role, &reason, &updatedAt, &payloadRaw); err != nil {
			return nil, err
		}
		account := accounts[accountID]
		if account == nil || !containsString(account.Groups, groupName) {
			continue
		}
		state := ""
		if routingState.Valid && routingState.String != "" {
			state = routingState.String
		} else if role.Valid {
			state = role.String
		}
		var weight *float64
		if payload, decodeErr := decodeObject(payloadRaw); decodeErr == nil {
			weight = finiteFloat(payload["weight"])
		}
		result[accountID] = append(result[accountID], decisionProjection{
			state: state, reason: nullString(reason), updatedAt: nullString(updatedAt), weight: weight,
		})
	}
	return result, rows.Err()
}

func (s *Store) loadAccountEvaluations(ctx context.Context, accounts map[string]*accountProjection) (map[string][]evaluationProjection, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT account_id,group_name,health_score,short_score,long_score,
		sample_count,ttfb_p50_ms,ttfb_p95_ms,latest_event FROM account_health_evaluations
		ORDER BY evaluated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string][]evaluationProjection)
	for rows.Next() {
		var accountID, groupName string
		var healthScore, shortScore, longScore, p50, p95 sql.NullFloat64
		var sampleCount int64
		var latestEvent sql.NullString
		if err := rows.Scan(&accountID, &groupName, &healthScore, &shortScore, &longScore, &sampleCount, &p50, &p95, &latestEvent); err != nil {
			return nil, err
		}
		account := accounts[accountID]
		if account == nil || !containsString(account.Groups, groupName) {
			continue
		}
		if latest := nullString(latestEvent); latest != nil {
			account.latestEvents[groupName] = *latest
		}
		result[accountID] = append(result[accountID], evaluationProjection{
			healthScore: nullFiniteFloat(healthScore), shortScore: nullFiniteFloat(shortScore),
			longScore: nullFiniteFloat(longScore), sampleCount: sampleCount,
			p50: nullFiniteFloat(p50), p95: nullFiniteFloat(p95),
		})
	}
	return result, rows.Err()
}

func (s *Store) loadRecentEvidence(ctx context.Context, accounts map[string]*accountProjection) error {
	rows, err := s.db.QueryContext(ctx, `WITH deduplicated AS (
		SELECT id,account_id,result,latency_p50,latency_p95,failure_reason,observed_at,source,evidence_key,payload_json,
		ROW_NUMBER() OVER (PARTITION BY account_id,source,COALESCE(NULLIF(evidence_key,''),'row:' || id)
		ORDER BY COALESCE(observed_at,'') DESC,id DESC) AS duplicate_rank FROM health_samples
		WHERE LOWER(REPLACE(source,'_','-'))<>'account-state'
	), ranked AS (
		SELECT *,ROW_NUMBER() OVER (PARTITION BY account_id ORDER BY COALESCE(observed_at,'') DESC,id DESC) AS account_rank
		FROM deduplicated WHERE duplicate_rank=1
	) SELECT account_id,result,latency_p50,latency_p95,failure_reason,observed_at,source,payload_json
	FROM ranked WHERE account_rank<=10 ORDER BY account_id,account_rank`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var accountID, source string
		var result, p50Raw, p95Raw, failureReason, observedAt, payloadRaw sql.NullString
		if err := rows.Scan(&accountID, &result, &p50Raw, &p95Raw, &failureReason, &observedAt, &source, &payloadRaw); err != nil {
			return err
		}
		if account := accounts[accountID]; account != nil {
			payload, decodeErr := decodeObject(payloadRaw.String)
			if decodeErr != nil {
				payload = map[string]any{}
			}
			latency := recentFirstTokenLatency(source, p50Raw, p95Raw, payloadRaw)
			classificationLatency := nullString(p95Raw)
			if classificationLatency == nil {
				classificationLatency = nullString(p50Raw)
			}
			account.RecentResults = append(account.RecentResults, AccountRecentResult{
				Result: nullString(result), ObservedAt: nullString(observedAt), LatencyMS: latency,
				FailureReason: nullString(failureReason), Source: source,
				ClassificationLatency: classificationLatency, ClassificationPayload: payload,
			})
		}
	}
	return rows.Err()
}

func recentFirstTokenLatency(source string, p50Raw, p95Raw, payloadRaw sql.NullString) *float64 {
	normalizedSource := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(source)), "_", "-")
	trustedProbe := normalizedSource == "active-probe" || normalizedSource == "probe"
	payload, err := decodeObject(payloadRaw.String)
	metric, _ := payload["latency_metric"].(string)
	metric = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(metric)), "-", "_")
	if !trustedProbe && (err != nil || (metric != "first_token" && metric != "ttfb")) {
		return nil
	}
	latency := finiteFloatFromNullString(p95Raw)
	if latency == nil {
		latency = finiteFloatFromNullString(p50Raw)
	}
	return latency
}

func (s *Store) loadApplyErrors(ctx context.Context) (map[string]struct {
	message string
	at      *string
}, error) {
	rows, err := s.db.QueryContext(ctx, `WITH ranked AS (
		SELECT object_id,error,state,created_at,ROW_NUMBER() OVER(PARTITION BY object_id ORDER BY created_at DESC,
		CASE WHEN source_id < 0 THEN 0 ELSE 1 END,CASE WHEN source_id < 0 THEN source_id END ASC,
		CASE WHEN source_id >= 0 THEN source_id END DESC) AS position
		FROM operation_audit WHERE operation_type IN ('routing.writeback','cleanup.delete') AND object_id IS NOT NULL
		AND (state='failed' OR readback_confirmed=1)
	) SELECT object_id,error,created_at FROM ranked WHERE position=1 AND state='failed' ORDER BY object_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]struct {
		message string
		at      *string
	})
	for rows.Next() {
		var objectID, message, createdAt sql.NullString
		if err := rows.Scan(&objectID, &message, &createdAt); err != nil {
			return nil, err
		}
		if !objectID.Valid || strings.TrimSpace(objectID.String) == "" {
			continue
		}
		accountID := strings.TrimSpace(objectID.String)
		if _, exists := result[accountID]; exists {
			continue
		}
		text := "自动执行失败"
		if message.Valid && message.String != "" {
			text = message.String
		}
		result[accountID] = struct {
			message string
			at      *string
		}{text, nullString(createdAt)}
	}
	return result, rows.Err()
}

func (s *Store) monitorPolicy(ctx context.Context, mode string) (map[string]struct{}, float64, error) {
	if mode != runtimepolicy.Monitoring {
		return map[string]struct{}{}, 75, nil
	}
	control, err := s.readPolicyDocument(ctx, s.db, "control-plane")
	if err != nil {
		return nil, 0, err
	}
	excluded := map[string]struct{}{}
	threshold := 75.0
	if scope, ok := control["scope"].(map[string]any); ok {
		if values, ok := scope["excluded_account_ids"].([]any); ok {
			for _, value := range values {
				if normalized := strings.TrimSpace(fmt.Sprint(value)); normalized != "" {
					excluded[normalized] = struct{}{}
				}
			}
		}
	}
	if degrade, ok := control["degrade"].(map[string]any); ok {
		if value := finiteFloat(degrade["score_threshold"]); value != nil && *value != 0 {
			threshold = *value
		}
	}
	return excluded, threshold, nil
}

func applyAccountCalculations(
	item *accountProjection,
	decisions []decisionProjection,
	evaluations []evaluationProjection,
	applyError struct {
		message string
		at      *string
	},
	applyView routingApplyView,
) {
	var selected *decisionProjection
	selectedState := ""
	for index := range decisions {
		state := NormalizeAccountState(decisions[index].state)
		if selected == nil || accountStatePriority(state) > accountStatePriority(selectedState) {
			selected = &decisions[index]
			selectedState = state
		}
	}
	if selected != nil {
		item.DecisionState = stringPointer(selectedState)
		item.DecisionReason = selected.reason
		item.DesiredHealth = stringPointer(selectedState)
	}
	currentHealth := AccountStateUnknown
	if item.RoutingState != nil {
		currentHealth = NormalizeAccountState(*item.RoutingState)
	}
	if currentHealth == AccountStateUnknown && item.HealthStatus != nil {
		currentHealth = NormalizeAccountState(*item.HealthStatus)
	}
	if metadataState := accountMetadataState(item.metadataRaw); metadataState == AccountStateDisabled {
		currentHealth = metadataState
	}
	effectiveHealth := currentHealth
	if item.Paused != nil && *item.Paused {
		effectiveHealth = "paused"
	} else if currentHealth == "disabled" {
		effectiveHealth = "disabled"
	} else if selectedState == "excluded" {
		effectiveHealth = "excluded"
	}
	item.Health = effectiveHealth
	statePending := selectedState != "" && selectedState != effectiveHealth && applyView.fields["schedulable"] &&
		effectiveHealth != "paused" && effectiveHealth != "disabled" && effectiveHealth != "excluded"
	item.ApplyPending = selectedState != "" && selectedState != "excluded" &&
		(statePending || routingTargetMismatch(item, applyView.fields))
	if item.ApplyPending {
		if applyView.automatic {
			item.ApplyError = stringPointer("尚未应用到 Sub2API")
		} else {
			item.ApplyError = stringPointer("当前运行模式只保存调度目标，不会自动执行")
		}
		if applyError.message != "" && (selected == nil || selected.updatedAt == nil || applyError.at == nil || *applyError.at >= *selected.updatedAt) {
			item.ApplyError = stringPointer(applyError.message)
		}
	}
	scored := make([]evaluationProjection, 0, len(evaluations))
	for _, evaluation := range evaluations {
		if evaluation.sampleCount > 0 {
			scored = append(scored, evaluation)
		}
	}
	item.HealthScore = averageEvaluation(scored, func(value evaluationProjection) *float64 { return value.healthScore })
	item.ShortScore = averageEvaluation(scored, func(value evaluationProjection) *float64 { return value.shortScore })
	item.LongScore = averageEvaluation(scored, func(value evaluationProjection) *float64 { return value.longScore })
	item.TTFBP50MS = averageEvaluation(scored, func(value evaluationProjection) *float64 { return value.p50 })
	item.TTFBP95MS = averageEvaluation(scored, func(value evaluationProjection) *float64 { return value.p95 })
	weights := make([]float64, 0, len(decisions))
	for _, decision := range decisions {
		if decision.weight != nil {
			weights = append(weights, *decision.weight)
		}
	}
	item.Weight = roundedAverage(weights)
	for _, evaluation := range scored {
		if evaluation.sampleCount > item.SampleCount {
			item.SampleCount = evaluation.sampleCount
		}
	}
}

func accountMetadataState(raw string) string {
	metadata, err := decodeObject(raw)
	if err != nil {
		return AccountStateUnknown
	}
	status, _ := metadata["status"].(string)
	return NormalizeAccountState(status)
}

func accountMetadataText(raw string, keys ...string) *string {
	metadata, err := decodeObject(raw)
	if err != nil {
		return nil
	}
	for _, key := range keys {
		value, ok := metadata[key].(string)
		value = strings.TrimSpace(value)
		if ok && value != "" {
			return stringPointer(value)
		}
	}
	return nil
}

func routingTargetMismatch(item *accountProjection, fields map[string]bool) bool {
	if fields["schedulable"] && item.TargetSchedulable != nil && !boolPointersEqual(item.Schedulable, item.TargetSchedulable) {
		return true
	}
	if fields["priority"] && item.TargetPriority != nil && !intPointersEqual(item.Priority, item.TargetPriority) {
		return true
	}
	if fields["load_factor"] && item.TargetLoadFactor != nil && !decimalPointersEqual(item.LoadFactor, item.TargetLoadFactor) {
		return true
	}
	return fields["concurrency"] && item.TargetConcurrency != nil && !intPointersEqual(item.Concurrency, item.TargetConcurrency)
}

func intPointersEqual(left, right *int64) bool {
	return left != nil && right != nil && *left == *right
}

func decimalPointersEqual(left, right *string) bool {
	if left == nil || right == nil {
		return false
	}
	leftValue, leftOK := new(big.Rat).SetString(strings.TrimSpace(*left))
	rightValue, rightOK := new(big.Rat).SetString(strings.TrimSpace(*right))
	return leftOK && rightOK && leftValue.Cmp(rightValue) == 0
}

func applyMonitoringHealth(item *accountProjection, excluded map[string]struct{}, degradeThreshold float64) {
	current := AccountStateUnknown
	if item.RoutingState != nil {
		current = NormalizeAccountState(*item.RoutingState)
	}
	if current == AccountStateUnknown && item.HealthStatus != nil {
		current = NormalizeAccountState(*item.HealthStatus)
	}
	reason := (*string)(nil)
	if item.Paused != nil && *item.Paused {
		current = "paused"
		reason = item.PausedReason
	} else if _, found := excluded[item.ID]; found {
		current = "excluded"
		reason = stringPointer("账号被排除")
	} else if current == "disabled" {
		reason = stringPointer("账号已停用")
	} else if current == "fused" || current == "cost_blocked" || current == "survivor" {
		reason = item.DecisionReason
	} else if item.SampleCount == 0 || item.HealthScore == nil {
		current = "unknown"
		reason = stringPointer("最近评估窗口没有有效样本")
	} else if *item.HealthScore < degradeThreshold {
		current = "degraded"
		reason = stringPointer("健康分低于降级线 " + strconv.FormatFloat(degradeThreshold, 'g', -1, 64))
	} else {
		current = "healthy"
	}
	item.Health = current
	item.DesiredHealth = nil
	item.DecisionState = stringPointer(current)
	item.DecisionReason = reason
	item.ApplyPending = false
	item.ApplyError = nil
}

func averageEvaluation(values []evaluationProjection, field func(evaluationProjection) *float64) *float64 {
	numbers := make([]float64, 0, len(values))
	for _, value := range values {
		if number := field(value); number != nil {
			numbers = append(numbers, *number)
		}
	}
	return roundedAverage(numbers)
}

func roundedAverage(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	total := 0.0
	for _, value := range values {
		total += value
	}
	result := math.Round(total/float64(len(values))*10000) / 10000
	return &result
}

func decodeObject(raw string) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, errors.New("JSON 对象无效")
	}
	return value, nil
}

func finiteFloat(value any) *float64 {
	if value == nil {
		return nil
	}
	var parsed float64
	var err error
	switch item := value.(type) {
	case bool:
		return nil
	case float64:
		parsed = item
	case json.Number:
		parsed, err = item.Float64()
	case string:
		parsed, err = strconv.ParseFloat(strings.TrimSpace(item), 64)
	default:
		parsed, err = strconv.ParseFloat(fmt.Sprint(item), 64)
	}
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return nil
	}
	return &parsed
}

func finiteFloatFromNullString(value sql.NullString) *float64 {
	if !value.Valid {
		return nil
	}
	return finiteFloat(value.String)
}

func nullFiniteFloat(value sql.NullFloat64) *float64 {
	if !value.Valid || math.IsNaN(value.Float64) || math.IsInf(value.Float64, 0) {
		return nil
	}
	result := value.Float64
	return &result
}

func strictBool(value sql.NullInt64) *bool {
	if !value.Valid || (value.Int64 != 0 && value.Int64 != 1) {
		return nil
	}
	result := value.Int64 == 1
	return &result
}

func nullString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func nullInt(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func stringPointer(value string) *string {
	result := value
	return &result
}

func positiveNumericID(value string) bool {
	number, err := strconv.ParseUint(value, 10, 64)
	return err == nil && number > 0
}

func canonicalHost(value string) string {
	normalized := strings.TrimSpace(strings.TrimRight(value, "/"))
	if separator := strings.Index(normalized, "://"); separator >= 0 {
		normalized = normalized[separator+3:]
		if slash := strings.IndexByte(normalized, '/'); slash >= 0 {
			normalized = normalized[:slash]
		}
	}
	return strings.ToLower(strings.TrimRight(normalized, "/"))
}

func containsString(values []string, expected string) bool {
	index := sort.SearchStrings(values, expected)
	return index < len(values) && values[index] == expected
}

func boolPointersEqual(left, right *bool) bool {
	return left != nil && right != nil && *left == *right
}
