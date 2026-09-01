package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"os"
	"strings"
	"syscall"
	"time"
)

const (
	inspectionConfigKey       = "auto-inspection"
	inspectionHeartbeatKey    = "auto-inspection-heartbeat"
	inspectionHistoryKey      = "auto-inspection-heartbeat-history"
	inspectionLeaseName       = "auto-inspection"
	inspectionHistoryLimit    = 50
	inspectionInterruptedText = "服务进程中断，上一轮自动巡检未完成"
	routingCalculationKey     = "routing-calculation-at"
)

type AutoInspectionConfig struct {
	Enabled         bool `json:"enabled"`
	IntervalSeconds int  `json:"interval_seconds"`
}

type OperationTiming struct {
	Operation       string  `json:"operation"`
	DurationSeconds float64 `json:"duration_seconds"`
	StartedAt       *string `json:"started_at,omitempty"`
}

type InspectionRoundSummary struct {
	Channels  int `json:"channels"`
	Probed    int `json:"probed"`
	Samples   int `json:"samples"`
	Fused     int `json:"fused"`
	Recovered int `json:"recovered"`
	Applied   int `json:"applied"`
	CleanedUp int `json:"cleaned_up"`
	Alerts    int `json:"alerts"`
}

type InspectionHeartbeat struct {
	CheckedAt         string                  `json:"checked_at"`
	CompletedAt       *string                 `json:"completed_at"`
	Status            string                  `json:"status"`
	Operations        []string                `json:"operations"`
	OperationTiming   []OperationTiming       `json:"operation_timings"`
	TaskID            *string                 `json:"task_id"`
	Error             *string                 `json:"error"`
	Skipped           bool                    `json:"skipped"`
	Summary           *InspectionRoundSummary `json:"summary,omitempty"`
	MonitoringEnabled *bool                   `json:"monitoring_enabled,omitempty"`
}

type inspectionLease struct {
	OwnerID   string
	OwnerPID  int
	OwnerHost string
	CheckedAt string
	ExpiresAt string
}

func DefaultAutoInspectionConfig() AutoInspectionConfig {
	return AutoInspectionConfig{Enabled: false, IntervalSeconds: 15}
}

func ValidateAutoInspectionConfig(value AutoInspectionConfig) error {
	if value.IntervalSeconds < 15 || value.IntervalSeconds > 86400 {
		return errors.New("interval_seconds 必须在 15 到 86400 之间")
	}
	return nil
}

func (s *Store) AutoInspectionConfig(ctx context.Context) (AutoInspectionConfig, error) {
	document, err := s.readPolicyDocument(ctx, s.db, inspectionConfigKey)
	if err != nil {
		return AutoInspectionConfig{}, err
	}
	if document == nil {
		return DefaultAutoInspectionConfig(), nil
	}
	if len(document) != 2 {
		return AutoInspectionConfig{}, errors.New("自动巡检配置包含未知或缺失字段")
	}
	enabled := strictAnyBool(document["enabled"])
	if enabled == nil {
		return AutoInspectionConfig{}, errors.New("enabled 必须是布尔值")
	}
	interval, err := strictInteger(document["interval_seconds"])
	if err != nil {
		return AutoInspectionConfig{}, errors.New("interval_seconds 必须是整数")
	}
	result := AutoInspectionConfig{Enabled: *enabled, IntervalSeconds: interval}
	if err := ValidateAutoInspectionConfig(result); err != nil {
		return AutoInspectionConfig{}, err
	}
	return result, nil
}

func (s *Store) UpdateAutoInspectionConfig(ctx context.Context, value AutoInspectionConfig) (AutoInspectionConfig, error) {
	if err := ValidateAutoInspectionConfig(value); err != nil {
		return AutoInspectionConfig{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AutoInspectionConfig{}, err
	}
	defer tx.Rollback()
	document := map[string]any{"enabled": value.Enabled, "interval_seconds": int64(value.IntervalSeconds)}
	if err := s.writePolicyDocument(ctx, tx, inspectionConfigKey, document, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return AutoInspectionConfig{}, err
	}
	if err := tx.Commit(); err != nil {
		return AutoInspectionConfig{}, err
	}
	return value, nil
}

func (s *Store) InspectionHeartbeats(ctx context.Context, limit int) ([]InspectionHeartbeat, error) {
	if limit < 1 || limit > inspectionHistoryLimit {
		return nil, errors.New("limit 必须在 1 到 50 之间")
	}
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value_json FROM app_state WHERE key=?`, inspectionHistoryKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return []InspectionHeartbeat{}, nil
	}
	if err != nil {
		return nil, err
	}
	var records []InspectionHeartbeat
	if err := json.Unmarshal([]byte(raw), &records); err != nil {
		return nil, errors.New("心跳历史损坏，无法读取")
	}
	if len(records) > limit {
		records = records[:limit]
	}
	if records == nil {
		records = []InspectionHeartbeat{}
	}
	for index := range records {
		records[index] = normalizeInspectionHeartbeatArrays(records[index])
	}
	return records, nil
}

func (s *Store) ClearInspectionHeartbeats(ctx context.Context) (int64, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value_json FROM app_state WHERE key=?`, inspectionHistoryKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	records := []InspectionHeartbeat{}
	if err := json.Unmarshal([]byte(raw), &records); err != nil {
		return 0, errors.New("心跳历史损坏，无法安全清空")
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM app_state WHERE key=?`, inspectionHistoryKey); err != nil {
		return 0, err
	}
	return int64(len(records)), nil
}

func (s *Store) RecordInspectionHeartbeat(ctx context.Context, heartbeat InspectionHeartbeat) error {
	normalized, err := normalizeInspectionHeartbeat(heartbeat)
	if err != nil {
		return err
	}
	connection, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer connection.Close()
	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = connection.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	var raw string
	err = connection.QueryRowContext(ctx, `SELECT value_json FROM app_state WHERE key=?`, inspectionHistoryKey).Scan(&raw)
	records := []InspectionHeartbeat{}
	if err == nil {
		if err := json.Unmarshal([]byte(raw), &records); err != nil {
			return errors.New("心跳历史损坏，无法安全追加")
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	filtered := make([]InspectionHeartbeat, 0, len(records)+1)
	filtered = append(filtered, normalized)
	for _, record := range records {
		if record.CheckedAt != normalized.CheckedAt {
			filtered = append(filtered, record)
		}
		if len(filtered) == inspectionHistoryLimit {
			break
		}
	}
	encoded, err := json.Marshal(filtered)
	if err != nil {
		return err
	}
	updatedAt := normalized.CheckedAt
	if normalized.CompletedAt != nil {
		updatedAt = *normalized.CompletedAt
	}
	if _, err := connection.ExecContext(ctx, `INSERT INTO app_state(key,value_json,updated_at) VALUES(?,?,?)
		ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json,updated_at=excluded.updated_at`,
		inspectionHistoryKey, string(encoded), updatedAt); err != nil {
		return err
	}
	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}

func (s *Store) AcquireInspectionLease(
	ctx context.Context,
	ownerID string,
	ownerPID int,
	ownerHost string,
	checkedAt time.Time,
	now time.Time,
	ttl time.Duration,
) (bool, error) {
	if strings.TrimSpace(ownerID) == "" || ownerPID <= 0 || strings.TrimSpace(ownerHost) == "" || ttl < time.Second {
		return false, errors.New("巡检租约参数无效")
	}
	connection, err := s.db.Conn(ctx)
	if err != nil {
		return false, err
	}
	defer connection.Close()
	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return false, err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = connection.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	lease, err := readInspectionLease(ctx, connection)
	if err != nil {
		return false, err
	}
	current := now.UTC()
	if lease != nil && inspectionLeaseActive(*lease, current) && lease.OwnerID != ownerID {
		return false, nil
	}
	if _, err := connection.ExecContext(ctx, `DELETE FROM scheduler_leases WHERE lease_name=?`, inspectionLeaseName); err != nil {
		return false, err
	}
	formattedNow := current.Format(time.RFC3339Nano)
	if _, err := connection.ExecContext(ctx, `INSERT INTO scheduler_leases(
		lease_name,owner_id,owner_pid,owner_host,checked_at,acquired_at,renewed_at,expires_at
	) VALUES(?,?,?,?,?,?,?,?)`, inspectionLeaseName, ownerID, ownerPID, ownerHost,
		checkedAt.UTC().Format(time.RFC3339Nano), formattedNow, formattedNow, current.Add(ttl).Format(time.RFC3339Nano)); err != nil {
		return false, err
	}
	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return false, err
	}
	committed = true
	return true, nil
}

func (s *Store) RenewInspectionLease(ctx context.Context, ownerID string, now time.Time, ttl time.Duration) (bool, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE scheduler_leases SET renewed_at=?,expires_at=?
		WHERE lease_name=? AND owner_id=?`, now.UTC().Format(time.RFC3339Nano),
		now.UTC().Add(ttl).Format(time.RFC3339Nano), inspectionLeaseName, ownerID)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (s *Store) ReleaseInspectionLease(ctx context.Context, ownerID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM scheduler_leases WHERE lease_name=? AND owner_id=?`, inspectionLeaseName, ownerID)
	return err
}

func (s *Store) ActiveInspectionCheckedAt(ctx context.Context, now time.Time) (*string, error) {
	lease, err := readInspectionLease(ctx, s.db)
	if err != nil || lease == nil {
		return nil, err
	}
	if !inspectionLeaseActive(*lease, now.UTC()) {
		return nil, nil
	}
	return stringPointer(lease.CheckedAt), nil
}

func (s *Store) ReconcileInterruptedInspections(ctx context.Context, now time.Time) (int, error) {
	needed, err := s.inspectionReconciliationNeeded(ctx, now.UTC())
	if err != nil || !needed {
		return 0, err
	}
	connection, err := s.db.Conn(ctx)
	if err != nil {
		return 0, err
	}
	defer connection.Close()
	if _, err := connection.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return 0, err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = connection.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	lease, err := readInspectionLease(ctx, connection)
	if err != nil {
		return 0, err
	}
	var activeCheckedAt string
	if lease != nil && inspectionLeaseActive(*lease, now.UTC()) {
		activeCheckedAt = lease.CheckedAt
	} else if lease != nil {
		if _, err := connection.ExecContext(ctx, `DELETE FROM scheduler_leases WHERE lease_name=?`, inspectionLeaseName); err != nil {
			return 0, err
		}
	}
	var raw string
	err = connection.QueryRowContext(ctx, `SELECT value_json FROM app_state WHERE key=?`, inspectionHistoryKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
			return 0, err
		}
		committed = true
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	records := []InspectionHeartbeat{}
	if err := json.Unmarshal([]byte(raw), &records); err != nil {
		records = []InspectionHeartbeat{}
	}
	completedAt := now.UTC().Format(time.RFC3339Nano)
	interrupted := 0
	for index := range records {
		if records[index].Status == "running" && records[index].CheckedAt != activeCheckedAt {
			records[index].Status = "failed"
			records[index].CompletedAt = &completedAt
			records[index].Error = stringPointer(inspectionInterruptedText)
			interrupted++
		}
	}
	if interrupted > 0 {
		encoded, err := json.Marshal(records)
		if err != nil {
			return 0, err
		}
		if _, err := connection.ExecContext(ctx, `UPDATE app_state SET value_json=?,updated_at=? WHERE key=?`,
			string(encoded), completedAt, inspectionHistoryKey); err != nil {
			return 0, err
		}
	}
	if _, err := connection.ExecContext(ctx, "COMMIT"); err != nil {
		return 0, err
	}
	committed = true
	return interrupted, nil
}

func (s *Store) inspectionReconciliationNeeded(ctx context.Context, now time.Time) (bool, error) {
	lease, err := readInspectionLease(ctx, s.db)
	if err != nil {
		return false, err
	}
	activeCheckedAt := ""
	if lease != nil {
		if !inspectionLeaseActive(*lease, now.UTC()) {
			return true, nil
		}
		activeCheckedAt = lease.CheckedAt
	}
	var raw string
	err = s.db.QueryRowContext(ctx, `SELECT value_json FROM app_state WHERE key=?`, inspectionHistoryKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	records := []InspectionHeartbeat{}
	if json.Unmarshal([]byte(raw), &records) != nil {
		return false, nil
	}
	for _, record := range records {
		if record.Status == "running" && record.CheckedAt != activeCheckedAt {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) InspectionTaskDue(ctx context.Context, taskName string, intervalSeconds int, now time.Time) (bool, error) {
	if intervalSeconds < 1 || intervalSeconds > 86400 {
		return false, errors.New("interval_seconds 必须在 1 到 86400 之间")
	}
	state, err := s.inspectionTaskState(ctx)
	if err != nil {
		return false, err
	}
	raw, found := state[taskName]
	if !found {
		return true, nil
	}
	last, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return true, nil
	}
	return now.UTC().Sub(last.UTC()) >= time.Duration(intervalSeconds)*time.Second, nil
}

func (s *Store) MarkInspectionTask(ctx context.Context, taskName string, now time.Time) error {
	state, err := s.inspectionTaskState(ctx)
	if err != nil {
		return err
	}
	state[taskName] = now.UTC().Format(time.RFC3339Nano)
	encoded, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO app_state(key,value_json,updated_at) VALUES(?,?,?)
		ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json,updated_at=excluded.updated_at`,
		inspectionHeartbeatKey, string(encoded), now.UTC().Format(time.RFC3339Nano))
	return err
}

// RoutingWritebackPending reports whether routing must run again because the
// policy changed, or a current decision lacks a successful writeback attempt.
// It covers policy-only changes, explicit failures, and a process interruption
// between decision persistence and apply.
const routingWritebackPendingSQL = `WITH current_decisions AS (
	SELECT rd.account_id,MAX(rd.updated_at) AS decided_at
	FROM routing_decisions rd
	WHERE rd.updated_at>=COALESCE(
		(SELECT updated_at FROM app_state WHERE key='routing-decision-epoch'),rd.updated_at)
	GROUP BY rd.account_id
), latest_attempt AS (
	SELECT cd.account_id,cd.decided_at,
		(SELECT oa.state FROM operation_audit oa
		 WHERE oa.operation_type='routing.writeback' AND oa.object_id=cd.account_id
		   AND oa.created_at>=cd.decided_at
		 ORDER BY oa.created_at DESC,oa.source_id ASC LIMIT 1) AS state
	FROM current_decisions cd
)
	SELECT CASE WHEN
		COALESCE((SELECT MAX(updated_at) FROM policy_nodes WHERE policy_key='control-plane'),'') >
		COALESCE((SELECT updated_at FROM app_state WHERE key=?),'')
	THEN 1 ELSE EXISTS(SELECT 1 FROM latest_attempt WHERE state IS NULL OR state<>'succeeded') END`

func (s *Store) RoutingWritebackPending(ctx context.Context) (bool, error) {
	var pending int
	err := s.db.QueryRowContext(ctx, routingWritebackPendingSQL,
		routingCalculationKey).Scan(&pending)
	return pending == 1, err
}

func (s *Store) inspectionTaskState(ctx context.Context) (map[string]string, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value_json FROM app_state WHERE key=?`, inspectionHeartbeatKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return map[string]string{}, nil
	}
	return result, nil
}

type inspectionQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readInspectionLease(ctx context.Context, queryer inspectionQueryer) (*inspectionLease, error) {
	var lease inspectionLease
	err := queryer.QueryRowContext(ctx, `SELECT owner_id,owner_pid,owner_host,checked_at,expires_at
		FROM scheduler_leases WHERE lease_name=?`, inspectionLeaseName).Scan(
		&lease.OwnerID, &lease.OwnerPID, &lease.OwnerHost, &lease.CheckedAt, &lease.ExpiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &lease, err
}

func inspectionLeaseActive(lease inspectionLease, now time.Time) bool {
	expiresAt, err := time.Parse(time.RFC3339Nano, lease.ExpiresAt)
	if err != nil || !expiresAt.After(now.UTC()) {
		return false
	}
	localHost, _ := os.Hostname()
	if lease.OwnerHost == localHost {
		err := syscall.Kill(lease.OwnerPID, 0)
		return err == nil || errors.Is(err, syscall.EPERM)
	}
	return true
}

func normalizeInspectionHeartbeat(value InspectionHeartbeat) (InspectionHeartbeat, error) {
	checkedAt, err := time.Parse(time.RFC3339Nano, value.CheckedAt)
	if err != nil {
		return InspectionHeartbeat{}, errors.New("checked_at 必须是有效时间")
	}
	value.CheckedAt = checkedAt.UTC().Format(time.RFC3339Nano)
	if value.CompletedAt != nil {
		completedAt, err := time.Parse(time.RFC3339Nano, *value.CompletedAt)
		if err != nil {
			return InspectionHeartbeat{}, errors.New("completed_at 必须是有效时间")
		}
		value.CompletedAt = stringPointer(completedAt.UTC().Format(time.RFC3339Nano))
	}
	if value.Status != "running" && value.Status != "succeeded" && value.Status != "failed" && value.Status != "cancelled" {
		value.Status = "failed"
	}
	operations := make([]string, 0, len(value.Operations))
	for _, operation := range value.Operations {
		if normalized := strings.TrimSpace(operation); normalized != "" {
			operations = append(operations, normalized)
		}
	}
	value.Operations = operations
	timings := make([]OperationTiming, 0, len(value.OperationTiming))
	for _, timing := range value.OperationTiming {
		if strings.TrimSpace(timing.Operation) == "" || timing.DurationSeconds < 0 || math.IsNaN(timing.DurationSeconds) || math.IsInf(timing.DurationSeconds, 0) {
			continue
		}
		if timing.StartedAt != nil {
			startedAt, parseErr := time.Parse(time.RFC3339Nano, *timing.StartedAt)
			if parseErr != nil {
				continue
			}
			timing.StartedAt = stringPointer(startedAt.UTC().Format(time.RFC3339Nano))
		}
		timings = append(timings, timing)
	}
	value.OperationTiming = timings
	return normalizeInspectionHeartbeatArrays(value), nil
}

func normalizeInspectionHeartbeatArrays(value InspectionHeartbeat) InspectionHeartbeat {
	if value.Operations == nil {
		value.Operations = []string{}
	}
	if value.OperationTiming == nil {
		value.OperationTiming = []OperationTiming{}
	}
	return value
}

func InspectionInterruptedText() string {
	return inspectionInterruptedText
}
