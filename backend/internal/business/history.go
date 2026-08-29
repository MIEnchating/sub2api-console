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

type RunEvent struct {
	ID        int64          `json:"id"`
	EventType string         `json:"event_type"`
	CreatedAt string         `json:"created_at"`
	Status    string         `json:"status"`
	Summary   string         `json:"summary"`
	Payload   map[string]any `json:"payload"`
}

type HealthSample struct {
	ID            int64          `json:"id"`
	AccountID     string         `json:"account_id"`
	GroupName     string         `json:"group_name"`
	Result        *string        `json:"result"`
	LatencyP50    *string        `json:"latency_p50"`
	LatencyP95    *string        `json:"latency_p95"`
	LatencyP99    *string        `json:"latency_p99"`
	SampleCount   *int64         `json:"sample_count"`
	Attempts      *int64         `json:"attempts"`
	FailureReason *string        `json:"failure_reason"`
	ObservedAt    *string        `json:"observed_at"`
	Source        string         `json:"source"`
	Payload       map[string]any `json:"payload"`
}

type RoutingDecision struct {
	AccountID    string         `json:"account_id"`
	GroupName    string         `json:"group_name"`
	Priority     *int64         `json:"priority"`
	Schedulable  *bool          `json:"schedulable"`
	Role         *string        `json:"role"`
	RoutingState *string        `json:"routing_state"`
	Rank         *int64         `json:"rank"`
	Reason       *string        `json:"reason"`
	UpdatedAt    string         `json:"updated_at"`
	Payload      map[string]any `json:"payload"`
}

type RunRecord struct {
	RunKey          string         `json:"run_key"`
	TaskName        string         `json:"task_name"`
	Status          *string        `json:"status"`
	Stage           *string        `json:"stage"`
	StartedAt       *string        `json:"started_at"`
	EndedAt         *string        `json:"ended_at"`
	DurationSeconds *string        `json:"duration_seconds"`
	Summary         *string        `json:"summary"`
	Payload         map[string]any `json:"payload"`
	UpdatedAt       string         `json:"updated_at"`
}

type OperationalSnapshot struct {
	Namespace  string  `json:"namespace"`
	StateKey   string  `json:"state_key"`
	Value      any     `json:"value"`
	ObservedAt *string `json:"observed_at"`
	UpdatedAt  string  `json:"updated_at"`
}

type UsageRecord struct {
	ID           int64          `json:"id"`
	RequestID    string         `json:"request_id"`
	AccountID    *string        `json:"account_id"`
	AccountName  *string        `json:"account_name"`
	GroupName    *string        `json:"group_name"`
	IsError      *bool          `json:"is_error"`
	ErrorReason  *string        `json:"error_reason"`
	FirstTokenMS *string        `json:"first_token_ms"`
	DurationMS   *string        `json:"duration_ms"`
	Summary      *string        `json:"summary"`
	ObservedAt   *string        `json:"observed_at"`
	Source       string         `json:"source"`
	Payload      map[string]any `json:"payload"`
}

type RequestTrace struct {
	RequestID    string        `json:"request_id"`
	Matched      bool          `json:"matched"`
	AccountID    *string       `json:"account_id"`
	AccountName  *string       `json:"account_name"`
	Records      []UsageRecord `json:"records"`
	RecentErrors []UsageRecord `json:"recent_errors"`
}

type SystemLogPage struct {
	Items    []UsageRecord `json:"items"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

type AlertListItem struct {
	AlertIncident
	ObjectName       *string `json:"object_name"`
	DeliveryAttempts int     `json:"delivery_attempts"`
	DeliveredAt      *string `json:"delivered_at"`
}

type AuditEvent struct {
	ID                int64    `json:"id"`
	OperationID       string   `json:"operation_id"`
	OperationType     string   `json:"operation_type"`
	State             string   `json:"state"`
	Phase             string   `json:"phase"`
	RequestID         *string  `json:"request_id"`
	Actor             *string  `json:"actor"`
	Source            *string  `json:"source"`
	Error             *string  `json:"error"`
	RemoteConfirmed   *bool    `json:"remote_confirmed"`
	ReadbackConfirmed *bool    `json:"readback_confirmed"`
	ObjectType        *string  `json:"object_type"`
	ObjectID          *string  `json:"object_id"`
	ObjectName        *string  `json:"object_name"`
	GroupNames        []string `json:"group_names"`
	FieldName         *string  `json:"field_name"`
	Before            any      `json:"before"`
	After             any      `json:"after"`
	Writeback         bool     `json:"writeback"`
	CreatedAt         string   `json:"created_at"`
}

func (s *Store) Events(ctx context.Context, limit *int) ([]RunEvent, error) {
	query := `SELECT source_id,event_type,created_at,status,summary,payload_json FROM runtime_events
		ORDER BY created_at DESC,CASE WHEN source_id < 0 THEN 0 ELSE 1 END,
		CASE WHEN source_id < 0 THEN source_id END ASC,CASE WHEN source_id >= 0 THEN source_id END DESC`
	arguments, err := appendLimit(&query, limit)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RunEvent{}
	for rows.Next() {
		var item RunEvent
		var payload string
		if err := rows.Scan(&item.ID, &item.EventType, &item.CreatedAt, &item.Status, &item.Summary, &payload); err != nil {
			return nil, err
		}
		item.Payload = decodedObjectOrMarker(payload, "runtime_events.payload_json")
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) HealthSamples(ctx context.Context, limit *int, accountID, groupName *string) ([]HealthSample, error) {
	clauses, arguments := filterClauses([]filterValue{{"account_id", accountID}, {"group_name", groupName}})
	query := `SELECT id,account_id,group_name,result,latency_p50,latency_p95,latency_p99,sample_count,attempts,
		failure_reason,observed_at,source,payload_json FROM health_samples` + whereSQL(clauses) + ` ORDER BY COALESCE(observed_at,'') DESC,id DESC`
	limitArguments, err := appendLimit(&query, limit)
	if err != nil {
		return nil, err
	}
	arguments = append(arguments, limitArguments...)
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []HealthSample{}
	for rows.Next() {
		var item HealthSample
		var resultText, p50, p95, p99, failure, observed sql.NullString
		var sampleCount, attempts sql.NullInt64
		var payload string
		if err := rows.Scan(&item.ID, &item.AccountID, &item.GroupName, &resultText, &p50, &p95, &p99,
			&sampleCount, &attempts, &failure, &observed, &item.Source, &payload); err != nil {
			return nil, err
		}
		item.Result, item.LatencyP50, item.LatencyP95, item.LatencyP99 = nullString(resultText), nullString(p50), nullString(p95), nullString(p99)
		item.SampleCount, item.Attempts = nullInt(sampleCount), nullInt(attempts)
		item.FailureReason, item.ObservedAt = nullString(failure), nullString(observed)
		item.Payload = decodedObjectOrMarker(payload, "health_samples.payload_json")
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) RoutingDecisions(ctx context.Context, limit *int, accountID, groupName *string) ([]RoutingDecision, error) {
	clauses, arguments := filterClauses([]filterValue{{"account_id", accountID}, {"group_name", groupName}})
	query := `SELECT account_id,group_name,priority,schedulable,role,routing_state,rank,reason,updated_at,payload_json
		FROM routing_decisions` + whereSQL(clauses) + ` ORDER BY updated_at DESC`
	limitArguments, err := appendLimit(&query, limit)
	if err != nil {
		return nil, err
	}
	arguments = append(arguments, limitArguments...)
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RoutingDecision{}
	for rows.Next() {
		var item RoutingDecision
		var priority, schedulable, rank sql.NullInt64
		var role, state, reason sql.NullString
		var payload string
		if err := rows.Scan(&item.AccountID, &item.GroupName, &priority, &schedulable, &role, &state, &rank, &reason, &item.UpdatedAt, &payload); err != nil {
			return nil, err
		}
		item.Priority, item.Schedulable, item.Rank = nullInt(priority), strictBool(schedulable), nullInt(rank)
		item.Role, item.RoutingState, item.Reason = nullString(role), nullString(state), nullString(reason)
		item.Payload = decodedObjectOrMarker(payload, "routing_decisions.payload_json")
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) RunRecords(ctx context.Context, limit *int) ([]RunRecord, error) {
	query := `SELECT run_key,task_name,status,stage,started_at,ended_at,duration_seconds,summary,payload_json,updated_at
		FROM run_records ORDER BY COALESCE(ended_at,started_at,updated_at) DESC`
	arguments, err := appendLimit(&query, limit)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RunRecord{}
	for rows.Next() {
		var item RunRecord
		var status, stage, started, ended, duration, summary sql.NullString
		var payload string
		if err := rows.Scan(&item.RunKey, &item.TaskName, &status, &stage, &started, &ended, &duration, &summary, &payload, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Status, item.Stage, item.StartedAt, item.EndedAt = nullString(status), nullString(stage), nullString(started), nullString(ended)
		item.DurationSeconds, item.Summary = nullString(duration), nullString(summary)
		item.Payload = decodedObjectOrMarker(payload, "run_records.payload_json")
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) OperationalSnapshots(ctx context.Context, namespace *string, limit *int) ([]OperationalSnapshot, error) {
	query := `SELECT namespace,state_key,value_json,observed_at,updated_at FROM operational_snapshots`
	arguments := []any{}
	if namespace != nil {
		query += ` WHERE namespace=?`
		arguments = append(arguments, *namespace)
	}
	query += ` ORDER BY namespace,state_key`
	limitArguments, err := appendLimit(&query, limit)
	if err != nil {
		return nil, err
	}
	arguments = append(arguments, limitArguments...)
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []OperationalSnapshot{}
	for rows.Next() {
		var item OperationalSnapshot
		var raw string
		var observed sql.NullString
		if err := rows.Scan(&item.Namespace, &item.StateKey, &raw, &observed, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.ObservedAt = nullString(observed)
		if err := json.Unmarshal([]byte(raw), &item.Value); err != nil {
			return nil, fmt.Errorf("运维快照 %s/%s 的 JSON 已损坏：%w", item.Namespace, item.StateKey, err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) UsageRecords(ctx context.Context, limit *int, requestID, accountID *string) ([]UsageRecord, error) {
	clauses, arguments := filterClauses([]filterValue{{"request_id", requestID}, {"account_id", accountID}})
	query := `SELECT id,request_id,account_id,account_name,group_name,is_error,error_reason,first_token_ms,observed_at,source,payload_json
		FROM usage_records` + whereSQL(clauses) + ` ORDER BY COALESCE(observed_at,'') DESC,id DESC`
	limitArguments, err := appendLimit(&query, limit)
	if err != nil {
		return nil, err
	}
	arguments = append(arguments, limitArguments...)
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []UsageRecord{}
	for rows.Next() {
		var item UsageRecord
		var account, name, group, reason, token, observed sql.NullString
		var isError sql.NullInt64
		var payload string
		if err := rows.Scan(&item.ID, &item.RequestID, &account, &name, &group, &isError, &reason, &token, &observed, &item.Source, &payload); err != nil {
			return nil, err
		}
		item.AccountID, item.AccountName, item.GroupName = nullString(account), nullString(name), nullString(group)
		item.IsError, item.ErrorReason, item.FirstTokenMS, item.ObservedAt = strictBool(isError), nullString(reason), nullString(token), nullString(observed)
		item.Payload = decodedObjectOrMarker(payload, "usage_records.payload_json")
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) RequestTrace(ctx context.Context, requestID string) (RequestTrace, error) {
	normalized := strings.TrimSpace(requestID)
	result := RequestTrace{RequestID: normalized, Records: []UsageRecord{}, RecentErrors: []UsageRecord{}}
	if normalized == "" {
		return result, nil
	}
	records, err := s.UsageRecords(ctx, nil, &normalized, nil)
	if err != nil {
		return RequestTrace{}, err
	}
	result.Records, result.Matched = records, len(records) > 0
	if len(records) == 0 {
		return result, nil
	}
	result.AccountID, result.AccountName = records[0].AccountID, records[0].AccountName
	if result.AccountID == nil {
		return result, nil
	}
	accountRecords, err := s.UsageRecords(ctx, nil, nil, result.AccountID)
	if err != nil {
		return RequestTrace{}, err
	}
	for _, record := range accountRecords {
		if record.IsError != nil && *record.IsError {
			result.RecentErrors = append(result.RecentErrors, record)
			if len(result.RecentErrors) == 20 {
				break
			}
		}
	}
	return result, nil
}

func (s *Store) Alerts(ctx context.Context, limit *int) ([]AlertListItem, error) {
	query := `SELECT i.incident_key,i.event_type,i.object_kind,i.object_id,CASE WHEN i.object_kind='account' THEN a.name END,
		i.cause_code,i.status,i.first_seen_at,i.last_seen_at,i.delivery_status,i.last_error,
		COALESCE(d.attempts,0),d.delivered_at
		FROM alert_incidents i LEFT JOIN accounts a ON i.object_kind='account' AND a.id=i.object_id
		LEFT JOIN (SELECT incident_key,SUM(attempts) AS attempts,MAX(delivered_at) AS delivered_at
			FROM alert_deliveries GROUP BY incident_key) d ON d.incident_key=i.incident_key
		ORDER BY i.last_seen_at DESC`
	arguments, err := appendLimit(&query, limit)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []AlertListItem{}
	for rows.Next() {
		var item AlertListItem
		var objectName, delivery, lastError, deliveredAt sql.NullString
		if err := rows.Scan(&item.IncidentKey, &item.EventType, &item.ObjectKind, &item.ObjectID, &objectName,
			&item.CauseCode, &item.Status, &item.FirstSeenAt, &item.LastSeenAt, &delivery, &lastError,
			&item.DeliveryAttempts, &deliveredAt); err != nil {
			return nil, err
		}
		item.ObjectName, item.DeliveryStatus, item.LastError = nullString(objectName), nullString(delivery), nullString(lastError)
		item.DeliveredAt = nullString(deliveredAt)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) ClearAlerts(ctx context.Context) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var count int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM alert_incidents WHERE status<>'firing'`).Scan(&count); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM alert_incidents WHERE status<>'firing'`); err != nil {
		return 0, err
	}
	return count, tx.Commit()
}

type LogCleanupResult struct {
	Runs    int64 `json:"runs"`
	Events  int64 `json:"events"`
	Changes int64 `json:"changes"`
}

func (s *Store) ClearLogRecords(ctx context.Context, before *time.Time) (LogCleanupResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return LogCleanupResult{}, err
	}
	defer tx.Rollback()
	result := LogCleanupResult{}
	deletions := []struct {
		table  string
		column string
		count  *int64
		extra  string
	}{
		{table: "run_records", column: "updated_at", count: &result.Runs, extra: " AND COALESCE(status,'') NOT IN ('queued','running','waiting_input')"},
		{table: "runtime_events", column: "created_at", count: &result.Events},
		{table: "operation_audit", column: "created_at", count: &result.Changes},
	}
	for _, deletion := range deletions {
		query := "DELETE FROM " + deletion.table + " WHERE 1=1" + deletion.extra
		arguments := []any{}
		if before != nil {
			query += " AND " + deletion.column + " < ?"
			arguments = append(arguments, before.UTC().Format(time.RFC3339Nano))
		}
		execution, err := tx.ExecContext(ctx, query, arguments...)
		if err != nil {
			return LogCleanupResult{}, err
		}
		*deletion.count, err = execution.RowsAffected()
		if err != nil {
			return LogCleanupResult{}, err
		}
	}
	return result, tx.Commit()
}

func (s *Store) AuditEvents(ctx context.Context, limit *int, writebackOnly bool) ([]AuditEvent, error) {
	query := `SELECT oa.source_id,oa.operation_id,oa.operation_type,oa.state,oa.phase,oa.request_id,oa.actor,oa.source,oa.error,oa.remote_confirmed,
		oa.readback_confirmed,oa.object_type,oa.object_id,COALESCE(NULLIF(oa.object_name,''),a.name),oa.group_names_json,
		oa.field_name,oa.before_json,oa.after_json,oa.writeback,oa.created_at
		FROM operation_audit oa LEFT JOIN accounts a ON a.id=oa.object_id`
	if writebackOnly {
		query += ` WHERE oa.phase<>'calculation' AND oa.operation_type<>'upstream.rate_sync' AND (
			oa.writeback=1 OR (oa.operation_type IN ('account.scheduling','routing.writeback')
				AND oa.state='succeeded' AND oa.remote_confirmed=0 AND oa.readback_confirmed=1
				AND oa.before_json IS NOT NULL AND oa.after_json IS NOT NULL)
		)`
	}
	query += ` ORDER BY oa.created_at DESC,CASE WHEN oa.source_id < 0 THEN 0 ELSE 1 END,
		CASE WHEN oa.source_id < 0 THEN oa.source_id END ASC,CASE WHEN oa.source_id >= 0 THEN oa.source_id END DESC`
	arguments, err := appendLimit(&query, limit)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []AuditEvent{}
	for rows.Next() {
		var item AuditEvent
		var requestID, actor, source, errorText, objectType, objectID, objectName, fieldName sql.NullString
		var remote, readback, writeback sql.NullInt64
		var groupsRaw string
		var beforeRaw, afterRaw sql.NullString
		if err := rows.Scan(&item.ID, &item.OperationID, &item.OperationType, &item.State, &item.Phase,
			&requestID, &actor, &source, &errorText, &remote, &readback, &objectType, &objectID, &objectName,
			&groupsRaw, &fieldName, &beforeRaw, &afterRaw, &writeback, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.RequestID, item.Actor, item.Source, item.Error = nullString(requestID), nullString(actor), nullString(source), nullString(errorText)
		item.RemoteConfirmed, item.ReadbackConfirmed = strictBool(remote), strictBool(readback)
		item.ObjectType, item.ObjectID, item.ObjectName, item.FieldName = nullString(objectType), nullString(objectID), nullString(objectName), nullString(fieldName)
		item.GroupNames = decodeStringArray(groupsRaw)
		item.Before, item.After = decodeNullableJSON(beforeRaw), decodeNullableJSON(afterRaw)
		item.Writeback = writeback.Valid && writeback.Int64 == 1
		result = append(result, item)
	}
	return result, rows.Err()
}

type filterValue struct {
	column string
	value  *string
}

func filterClauses(filters []filterValue) ([]string, []any) {
	clauses, arguments := []string{}, []any{}
	for _, filter := range filters {
		if filter.value != nil {
			clauses = append(clauses, filter.column+"=?")
			arguments = append(arguments, *filter.value)
		}
	}
	return clauses, arguments
}

func whereSQL(clauses []string) string {
	if len(clauses) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(clauses, " AND ")
}

func appendLimit(query *string, limit *int) ([]any, error) {
	if limit == nil {
		return nil, nil
	}
	if *limit < 0 || *limit > 100000 {
		return nil, errors.New("limit 必须在 0 到 100000 之间")
	}
	*query += ` LIMIT ?`
	return []any{*limit}, nil
}

func decodedObjectOrMarker(raw, field string) map[string]any {
	value, err := decodeObject(raw)
	if err != nil {
		return map[string]any{"_invalid_configuration": []string{field}}
	}
	return value
}

func decodeStringArray(raw string) []string {
	var values []any
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return []string{}
	}
	result := make([]string, len(values))
	for index := range values {
		result[index] = fmt.Sprint(values[index])
	}
	return result
}

func decodeNullableJSON(raw sql.NullString) any {
	if !raw.Valid {
		return nil
	}
	var value any
	if err := json.Unmarshal([]byte(raw.String), &value); err != nil {
		return nil
	}
	return value
}
