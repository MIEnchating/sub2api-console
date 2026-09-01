package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type RoutingAccount struct {
	ID                   string
	Name                 string
	GroupName            string
	GroupID              *string
	GroupCostWall        *string
	ProfitEnabled        *bool
	ProfitMinMargin      *string
	ProfitSafetyBuffer   *string
	UpstreamHost         *string
	UpstreamType         *string
	UpstreamAuthStatus   *string
	Schedulable          *bool
	Priority             *int64
	ManualPriority       *int64
	BaselinePriority     *int64
	ManagedSchedulable   *bool
	ManagedPriority      *int64
	ManagedLoadFactor    *string
	ManagedConcurrency   *int64
	ExternalControl      bool
	LoadFactor           *string
	Concurrency          *int64
	Multiplier           *string
	Paused               bool
	PausedReason         *string
	EffectiveState       string
	CatalogBindingState  string
	CatalogBindingReason *string
	Metadata             map[string]any
}

type RoutingSample struct {
	AccountID     string
	GroupName     string
	Result        string
	LatencyP95    *string
	FailureReason string
	Source        string
	ObservedAt    string
	Payload       map[string]any
}

type PreviousRoutingDecision struct {
	AccountID          string
	GroupName          string
	Priority           *int64
	Schedulable        *bool
	State              string
	UpdatedAt          string
	LastApplyAt        time.Time
	LastWeightWriteAt  time.Time
	LastScalingWriteAt time.Time
	Payload            map[string]any
}

type RoutingEvaluationWrite struct {
	AccountID   string
	GroupName   string
	HealthScore *float64
	ShortScore  *float64
	LongScore   *float64
	SampleCount int
	TTFBP50MS   *float64
	TTFBP95MS   *float64
	LatestEvent string
}

type RoutingDecisionWrite struct {
	AccountID   string
	GroupName   string
	Priority    *int64
	Schedulable *bool
	Role        string
	State       string
	Rank        *int
	Reason      string
	Payload     map[string]any
}

type CleanupStateWrite struct {
	AccountID     string
	EligibleSince *time.Time
}

type RuntimeEventWrite struct {
	EventType string
	Status    string
	Summary   string
	Payload   map[string]any
}

type AccountRoutingTarget struct {
	AccountID          string   `json:"account_id"`
	Priority           *int64   `json:"target_priority"`
	LoadFactor         *string  `json:"target_load_factor"`
	Schedulable        *bool    `json:"target_schedulable"`
	Concurrency        *int64   `json:"target_concurrency"`
	GroupNames         []string `json:"group_names"`
	DesiredHealth      string   `json:"desired_health"`
	WriteCooldown      bool     `json:"write_cooldown_active"`
	ScalingCooldown    bool     `json:"scaling_cooldown_active"`
	ReleaseControl     bool     `json:"release_control,omitempty"`
	AbandonControl     bool     `json:"abandon_control,omitempty"`
	CleanupAction      *string  `json:"cleanup_action,omitempty"`
	ConfigurationError *string  `json:"configuration_error,omitempty"`
}

func (s *Store) RoutingAccounts(ctx context.Context, accountID, groupName *string) ([]RoutingAccount, error) {
	if err := s.ensureStableUpstreamRelations(ctx); err != nil {
		return nil, err
	}
	clauses := []string{}
	arguments := []any{}
	if accountID != nil {
		clauses = append(clauses, "a.id=?")
		arguments = append(arguments, strings.TrimSpace(*accountID))
	}
	if groupName != nil {
		clauses = append(clauses, "a.id IN (SELECT account_id FROM account_groups WHERE group_name=?)")
		arguments = append(arguments, strings.TrimSpace(*groupName))
	}
	query := `SELECT a.id,a.name,ag.group_name,ag.group_id,
		lg.rate_multiplier,lg.profit_control_enabled,lg.profit_min_margin,lg.profit_safety_buffer,
		a.upstream_host,a.upstream_type,u.auth_status,a.schedulable,a.priority,m.priority,rb.priority,
		rb.managed_schedulable,rb.managed_priority,rb.managed_load_factor,rb.managed_concurrency,
		CASE WHEN rb.ownership_version=2 THEN 1 ELSE 0 END,a.load_factor,
		a.concurrency,a.multiplier,a.paused,a.paused_reason,COALESCE(a.routing_state,''),a.metadata_json
		FROM accounts a JOIN account_groups ag ON ag.account_id=a.id
		LEFT JOIN local_groups lg ON lg.name=ag.group_name
		LEFT JOIN upstreams u ON u.host=a.upstream_host
		LEFT JOIN manual_priority_accounts m ON m.account_id=a.id
		LEFT JOIN routing_baselines rb ON rb.account_id=a.id`
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += ` ORDER BY ag.group_name,CASE WHEN a.id GLOB '[0-9]*' THEN CAST(a.id AS INTEGER) ELSE 0 END,a.id`
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	result := []RoutingAccount{}
	for rows.Next() {
		var item RoutingAccount
		var groupID, costWall, profitMargin, profitBuffer sql.NullString
		var upstreamHost, upstreamType, authStatus, loadFactor, multiplier, pausedReason sql.NullString
		var profitEnabled, schedulable, paused sql.NullInt64
		var priority, manualPriority, baselinePriority, managedPriority, managedConcurrency, concurrency sql.NullInt64
		var managedSchedulable, externalControl sql.NullInt64
		var managedLoadFactor sql.NullString
		var metadataRaw string
		if err := rows.Scan(
			&item.ID, &item.Name, &item.GroupName, &groupID,
			&costWall, &profitEnabled, &profitMargin, &profitBuffer,
			&upstreamHost, &upstreamType, &authStatus, &schedulable, &priority, &manualPriority, &baselinePriority,
			&managedSchedulable, &managedPriority, &managedLoadFactor, &managedConcurrency, &externalControl, &loadFactor,
			&concurrency, &multiplier, &paused, &pausedReason, &item.EffectiveState, &metadataRaw,
		); err != nil {
			return nil, err
		}
		item.GroupID, item.GroupCostWall = nullString(groupID), nullString(costWall)
		item.ProfitEnabled = strictNullBool(profitEnabled)
		item.ProfitMinMargin, item.ProfitSafetyBuffer = nullString(profitMargin), nullString(profitBuffer)
		item.UpstreamHost, item.UpstreamType, item.UpstreamAuthStatus = nullString(upstreamHost), nullString(upstreamType), nullString(authStatus)
		item.Schedulable, item.Priority, item.ManualPriority, item.BaselinePriority, item.LoadFactor = strictNullBool(schedulable), nullInt(priority), nullInt(manualPriority), nullInt(baselinePriority), nullString(loadFactor)
		item.ManagedSchedulable, item.ManagedPriority = strictNullBool(managedSchedulable), nullInt(managedPriority)
		item.ManagedLoadFactor, item.ManagedConcurrency = nullString(managedLoadFactor), nullInt(managedConcurrency)
		item.ExternalControl = externalControl.Valid && externalControl.Int64 == 1
		item.Concurrency, item.Multiplier = nullInt(concurrency), nullString(multiplier)
		item.Paused, item.PausedReason = paused.Valid && paused.Int64 == 1, nullString(pausedReason)
		if err := json.Unmarshal([]byte(metadataRaw), &item.Metadata); err != nil || item.Metadata == nil {
			return nil, fmt.Errorf("账号 %s metadata 配置无效", item.ID)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	states, err := s.accountCatalogBindingStates(ctx)
	if err != nil {
		return nil, err
	}
	for index := range result {
		if state, found := states[result[index].ID]; found {
			result[index].CatalogBindingState = state.Status
			result[index].CatalogBindingReason = stringPointer(state.Reason)
		}
	}
	return result, nil
}

func (s *Store) RoutingSamples(
	ctx context.Context,
	accountID, groupName *string,
	source string,
	limit int,
) ([]RoutingSample, error) {
	if limit < 1 {
		return nil, errors.New("路由样本上限必须是正整数")
	}
	clauses := []string{"LOWER(REPLACE(source,'_','-')) IN ('traffic','active-probe')"}
	arguments := []any{}
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "traffic":
		// Real-traffic mode uses the unified account history. Active probes fill
		// accounts without fresh traffic, matching Guardian's sample window.
	case "active_probe", "active-probe":
		clauses = append(clauses, "LOWER(REPLACE(source,'_','-'))='active-probe'")
	default:
		return nil, errors.New("路由样本来源模式无效")
	}
	if accountID != nil {
		clauses = append(clauses, "account_id=?")
		arguments = append(arguments, strings.TrimSpace(*accountID))
	}
	if groupName != nil {
		// Health evidence belongs to the account, not to one membership. A
		// multi-group account must use the same sample window when a secondary
		// group is calculated in isolation.
		clauses = append(clauses, "account_id IN (SELECT account_id FROM account_groups WHERE group_name=?)")
		arguments = append(arguments, strings.TrimSpace(*groupName))
	}
	selections, err := s.selectHealthSampleWindow(ctx, clauses, arguments, limit, true)
	if err != nil {
		return nil, err
	}
	samples, err := s.selectedHealthSamples(ctx, selections)
	if err != nil {
		return nil, err
	}
	result := make([]RoutingSample, 0, len(selections))
	for _, selection := range selections {
		sample, present := samples[selection.id]
		if !present {
			continue
		}
		var item RoutingSample
		item.AccountID, item.GroupName, item.Source = sample.accountID, sample.groupName, sample.source
		item.Result, item.FailureReason = pointerValue(nullString(sample.result)), pointerValue(nullString(sample.failureReason))
		item.LatencyP95, item.ObservedAt = nullString(sample.latencyP95), pointerValue(nullString(sample.observedAt))
		if err := json.Unmarshal([]byte(sample.payloadJSON), &item.Payload); err != nil || item.Payload == nil {
			return nil, fmt.Errorf("账号 %s 分组 %s 的健康样本损坏", item.AccountID, item.GroupName)
		}
		result = append(result, item)
	}
	return result, nil
}

func previousRoutingDecisionsQuery(accountID, groupName *string) (string, []any) {
	clauses := []string{`julianday(rd.updated_at)>=COALESCE(
		(SELECT julianday(updated_at) FROM app_state WHERE key='routing-decision-epoch'),julianday(rd.updated_at))`}
	arguments := []any{}
	if accountID != nil {
		clauses = append(clauses, "rd.account_id=?")
		arguments = append(arguments, strings.TrimSpace(*accountID))
	}
	if groupName != nil {
		clauses = append(clauses, "rd.account_id IN (SELECT account_id FROM account_groups WHERE group_name=?)")
		arguments = append(arguments, strings.TrimSpace(*groupName))
	}
	query := `SELECT rd.account_id,rd.group_name,rd.priority,rd.schedulable,rd.routing_state,rd.updated_at,rd.payload_json,
		(SELECT oa.created_at FROM operation_audit oa INDEXED BY ix_operation_audit_routing_lookup
		 WHERE oa.operation_type='routing.writeback' AND oa.state='succeeded'
		 AND oa.remote_confirmed=1 AND oa.readback_confirmed=1 AND oa.object_id=rd.account_id
		 ORDER BY oa.created_at DESC,
		 CASE WHEN oa.source_id < 0 THEN 0 ELSE 1 END,CASE WHEN oa.source_id < 0 THEN oa.source_id END ASC,
		 CASE WHEN oa.source_id >= 0 THEN oa.source_id END DESC LIMIT 1),
		(SELECT oa.created_at FROM operation_audit oa INDEXED BY ix_operation_audit_routing_lookup
		 WHERE oa.operation_type='routing.writeback' AND oa.state='succeeded'
		 AND oa.remote_confirmed=1 AND oa.readback_confirmed=1 AND oa.object_id=rd.account_id
		 AND oa.field_name LIKE '%load_factor%' ORDER BY oa.created_at DESC,
		 CASE WHEN oa.source_id < 0 THEN 0 ELSE 1 END,CASE WHEN oa.source_id < 0 THEN oa.source_id END ASC,
		 CASE WHEN oa.source_id >= 0 THEN oa.source_id END DESC LIMIT 1),
		(SELECT oa.created_at FROM operation_audit oa INDEXED BY ix_operation_audit_routing_lookup
		 WHERE oa.operation_type='routing.writeback' AND oa.state='succeeded'
		 AND oa.remote_confirmed=1 AND oa.readback_confirmed=1 AND oa.object_id=rd.account_id
		 AND oa.field_name LIKE '%concurrency%' ORDER BY oa.created_at DESC,
		 CASE WHEN oa.source_id < 0 THEN 0 ELSE 1 END,CASE WHEN oa.source_id < 0 THEN oa.source_id END ASC,
		 CASE WHEN oa.source_id >= 0 THEN oa.source_id END DESC LIMIT 1)
		FROM routing_decisions rd`
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += ` ORDER BY rd.account_id,rd.group_name`
	return query, arguments
}

func (s *Store) PreviousRoutingDecisions(ctx context.Context, accountID, groupName *string) ([]PreviousRoutingDecision, error) {
	query, arguments := previousRoutingDecisionsQuery(accountID, groupName)
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PreviousRoutingDecision{}
	for rows.Next() {
		var item PreviousRoutingDecision
		var priority, schedulable sql.NullInt64
		var state sql.NullString
		var payloadRaw string
		var applyAt, weightWriteAt, scalingWriteAt sql.NullString
		if err := rows.Scan(&item.AccountID, &item.GroupName, &priority, &schedulable, &state, &item.UpdatedAt, &payloadRaw, &applyAt, &weightWriteAt, &scalingWriteAt); err != nil {
			return nil, err
		}
		item.Priority, item.Schedulable, item.State = nullInt(priority), strictNullBool(schedulable), pointerValue(nullString(state))
		item.LastApplyAt = parsedRoutingTime(applyAt)
		item.LastWeightWriteAt = parsedRoutingTime(weightWriteAt)
		item.LastScalingWriteAt = parsedRoutingTime(scalingWriteAt)
		if err := json.Unmarshal([]byte(payloadRaw), &item.Payload); err != nil || item.Payload == nil {
			return nil, errors.New("历史路由决策损坏")
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) CleanupStates(ctx context.Context, accountID *string) (map[string]time.Time, error) {
	query := `SELECT account_id,eligible_since FROM cleanup_states`
	arguments := []any{}
	if accountID != nil {
		query += ` WHERE account_id=?`
		arguments = append(arguments, strings.TrimSpace(*accountID))
	}
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]time.Time{}
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return nil, fmt.Errorf("账号 %s 的自动处置观察状态损坏", id)
		}
		result[id] = parsed.UTC()
	}
	return result, rows.Err()
}

func parsedRoutingTime(value sql.NullString) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, value.String)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func (s *Store) PersistRoutingRound(
	ctx context.Context,
	accountID, groupName *string,
	evaluations []RoutingEvaluationWrite,
	decisions []RoutingDecisionWrite,
	targets []AccountRoutingTarget,
	cleanupStates []CleanupStateWrite,
	runtimeEvents []RuntimeEventWrite,
	persistDecisions bool,
	now time.Time,
) error {
	if err := validateCanonicalRoutingWrites(evaluations, decisions); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	where, arguments := routingScope(accountID, groupName)
	if _, err := tx.ExecContext(ctx, `DELETE FROM account_health_evaluations`+where, arguments...); err != nil {
		return err
	}
	evaluatedAt := now.UTC().Format(time.RFC3339Nano)
	for _, item := range evaluations {
		if _, err := tx.ExecContext(ctx, `INSERT INTO account_health_evaluations(
			account_id,group_name,health_score,short_score,long_score,sample_count,ttfb_p50_ms,ttfb_p95_ms,latest_event,evaluated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?)`, item.AccountID, item.GroupName, item.HealthScore, item.ShortScore, item.LongScore,
			item.SampleCount, item.TTFBP50MS, item.TTFBP95MS, item.LatestEvent, evaluatedAt); err != nil {
			return err
		}
	}
	if !persistDecisions {
		return tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM routing_decisions`+where, arguments...); err != nil {
		return err
	}
	for _, item := range decisions {
		payload, err := json.Marshal(item.Payload)
		if err != nil {
			return fmt.Errorf("路由决策无法严格 JSON 序列化：%w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO routing_decisions(
			account_id,group_name,priority,schedulable,role,routing_state,rank,reason,updated_at,payload_json
		) VALUES(?,?,?,?,?,?,?,?,?,?)`, item.AccountID, item.GroupName, item.Priority, boolDatabase(item.Schedulable),
			item.Role, item.State, item.Rank, item.Reason, evaluatedAt, string(payload)); err != nil {
			return err
		}
	}
	for _, item := range targets {
		if _, err := tx.ExecContext(ctx, `UPDATE accounts SET target_priority=?,target_load_factor=?,target_schedulable=?,
			target_concurrency=?,updated_at=? WHERE id=?`, item.Priority, item.LoadFactor, boolDatabase(item.Schedulable),
			item.Concurrency, evaluatedAt, item.AccountID); err != nil {
			return err
		}
	}
	for _, item := range cleanupStates {
		if item.EligibleSince == nil {
			if _, err := tx.ExecContext(ctx, `DELETE FROM cleanup_states WHERE account_id=?`, item.AccountID); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO cleanup_states(account_id,eligible_since,updated_at) VALUES(?,?,?)
			ON CONFLICT(account_id) DO UPDATE SET eligible_since=excluded.eligible_since,updated_at=excluded.updated_at`,
			item.AccountID, item.EligibleSince.UTC().Format(time.RFC3339Nano), evaluatedAt); err != nil {
			return err
		}
	}
	for _, event := range runtimeEvents {
		status := strings.TrimSpace(event.Status)
		if status == "" {
			status = "succeeded"
		}
		if err := insertRuntimeEventWithStatus(ctx, tx, event.EventType, status, event.Summary, event.Payload, evaluatedAt); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO app_state(key,value_json,updated_at) VALUES(?,?,?)
		ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json,updated_at=excluded.updated_at`,
		routingCalculationKey, `{}`, evaluatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func routingScope(accountID, groupName *string) (string, []any) {
	clauses := []string{}
	arguments := []any{}
	if accountID != nil {
		clauses = append(clauses, "account_id=?")
		arguments = append(arguments, strings.TrimSpace(*accountID))
	}
	if groupName != nil {
		clauses = append(clauses, "account_id IN (SELECT account_id FROM account_groups WHERE group_name=?)")
		arguments = append(arguments, strings.TrimSpace(*groupName))
	}
	if len(clauses) == 0 {
		return "", arguments
	}
	return " WHERE " + strings.Join(clauses, " AND "), arguments
}

func validateCanonicalRoutingWrites(evaluations []RoutingEvaluationWrite, decisions []RoutingDecisionWrite) error {
	evaluated := make(map[string]struct{}, len(evaluations))
	for _, item := range evaluations {
		if _, found := evaluated[item.AccountID]; found {
			return fmt.Errorf("账号 %s 生成了多份健康评估", item.AccountID)
		}
		evaluated[item.AccountID] = struct{}{}
	}
	decided := make(map[string]struct{}, len(decisions))
	for _, item := range decisions {
		if _, found := decided[item.AccountID]; found {
			return fmt.Errorf("账号 %s 生成了多份最终调度状态", item.AccountID)
		}
		decided[item.AccountID] = struct{}{}
	}
	return nil
}

func strictNullBool(value sql.NullInt64) *bool {
	if !value.Valid || (value.Int64 != 0 && value.Int64 != 1) {
		return nil
	}
	result := value.Int64 == 1
	return &result
}

func boolDatabase(value *bool) any {
	if value == nil {
		return nil
	}
	if *value {
		return int64(1)
	}
	return int64(0)
}
