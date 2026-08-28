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

type RoutingBaseline struct {
	AccountID          string
	Schedulable        *bool
	Priority           *int64
	LoadFactor         *string
	Concurrency        *int64
	Status             *string
	CapturedAt         string
	OwnershipVersion   int
	ManagedSchedulable *bool
	ManagedPriority    *int64
	ManagedLoadFactor  *string
	ManagedConcurrency *int64
	ManagedStatus      *string
}

type RoutingManagedIntent struct {
	Schedulable *bool
	Priority    *int64
	LoadFactor  *string
	Concurrency *int64
	Status      *string
}

type RoutingReadback struct {
	Schedulable  *bool
	Priority     *int64
	LoadFactor   *string
	Concurrency  *int64
	RoutingState *string
}

func (s *Store) RoutingBaselines(ctx context.Context) ([]RoutingBaseline, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT account_id,schedulable,priority,load_factor,concurrency,status,captured_at,
		ownership_version,managed_schedulable,managed_priority,managed_load_factor,managed_concurrency,managed_status
		FROM routing_baselines ORDER BY CASE WHEN account_id GLOB '[0-9]*' THEN CAST(account_id AS INTEGER) ELSE 0 END,account_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RoutingBaseline{}
	for rows.Next() {
		var item RoutingBaseline
		var schedulable, priority, concurrency, managedSchedulable, managedPriority, managedConcurrency sql.NullInt64
		var loadFactor, status, managedLoadFactor, managedStatus sql.NullString
		if err := rows.Scan(&item.AccountID, &schedulable, &priority, &loadFactor, &concurrency, &status, &item.CapturedAt,
			&item.OwnershipVersion, &managedSchedulable, &managedPriority, &managedLoadFactor, &managedConcurrency, &managedStatus); err != nil {
			return nil, err
		}
		item.Schedulable = strictNullBool(schedulable)
		item.Priority, item.Concurrency = nullInt(priority), nullInt(concurrency)
		item.LoadFactor = nullString(loadFactor)
		item.Status = nullString(status)
		item.ManagedSchedulable, item.ManagedPriority = strictNullBool(managedSchedulable), nullInt(managedPriority)
		item.ManagedLoadFactor, item.ManagedConcurrency = nullString(managedLoadFactor), nullInt(managedConcurrency)
		item.ManagedStatus = nullString(managedStatus)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) CaptureRoutingBaseline(ctx context.Context, baseline RoutingBaseline) error {
	if !positiveNumericID(baseline.AccountID) {
		return errors.New("账号必须使用有效的稳定 ID")
	}
	capturedAt := strings.TrimSpace(baseline.CapturedAt)
	if capturedAt == "" {
		capturedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO routing_baselines(
		account_id,schedulable,priority,load_factor,concurrency,status,captured_at,ownership_version
	) VALUES(?,?,?,?,?,?,?,?)`, baseline.AccountID, boolDatabase(baseline.Schedulable), baseline.Priority,
		baseline.LoadFactor, baseline.Concurrency, baseline.Status, capturedAt, baseline.OwnershipVersion)
	return err
}

func (s *Store) UpdateRoutingManagedIntent(ctx context.Context, accountID string, intent RoutingManagedIntent) error {
	if !positiveNumericID(accountID) {
		return errors.New("账号必须使用有效的稳定 ID")
	}
	sets := []string{"ownership_version=1"}
	arguments := []any{}
	for _, item := range []struct {
		column string
		set    bool
		value  any
	}{
		{"managed_schedulable", intent.Schedulable != nil, boolDatabase(intent.Schedulable)},
		{"managed_priority", intent.Priority != nil, intent.Priority},
		{"managed_load_factor", intent.LoadFactor != nil, intent.LoadFactor},
		{"managed_concurrency", intent.Concurrency != nil, intent.Concurrency},
		{"managed_status", intent.Status != nil, intent.Status},
	} {
		if !item.set {
			continue
		}
		sets = append(sets, item.column+"=?")
		arguments = append(arguments, item.value)
	}
	arguments = append(arguments, accountID)
	result, err := s.db.ExecContext(ctx, `UPDATE routing_baselines SET `+strings.Join(sets, ",")+` WHERE account_id=?`, arguments...)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return fmt.Errorf("保存账号 %s 的调度写入所有权失败", accountID)
	}
	return nil
}

func (s *Store) CommitRoutingReadback(
	ctx context.Context,
	accountID string,
	readback RoutingReadback,
	deleteBaseline bool,
	operation AccountOperation,
) error {
	return s.commitAccountMutation(ctx, accountID, operation, func(tx *sql.Tx, now string) error {
		routingState := managementNullableString(readback.RoutingState)
		if deleteBaseline {
			routingState = ""
		}
		result, err := tx.ExecContext(ctx, `UPDATE accounts SET schedulable=?,priority=?,load_factor=?,concurrency=?,
			routing_state=COALESCE(?,routing_state),updated_at=? WHERE id=?`,
			boolDatabase(readback.Schedulable), readback.Priority, readback.LoadFactor, readback.Concurrency,
			routingState, now, accountID)
		if err != nil {
			return err
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			return sql.ErrNoRows
		}
		if deleteBaseline {
			_, err = tx.ExecContext(ctx, `DELETE FROM routing_baselines WHERE account_id=?`, accountID)
		}
		return err
	})
}

func (s *Store) ClearRoutingRuntimeBlocks(ctx context.Context, accountID string) error {
	if !positiveNumericID(accountID) {
		return errors.New("账号必须使用有效的稳定 ID")
	}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT metadata_json FROM accounts WHERE id=?`, accountID).Scan(&raw); err != nil {
		return err
	}
	metadata := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return fmt.Errorf("账号 %s metadata 配置无效", accountID)
	}
	for _, key := range []string{
		"last_error", "error_message", "rate_limited_at", "rate_limit_reset_at",
		"temp_unschedulable_until", "temp_unschedulable_reason", "overload_until",
	} {
		delete(metadata, key)
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE accounts SET metadata_json=?,updated_at=? WHERE id=?`,
		string(encoded), time.Now().UTC().Format(time.RFC3339Nano), accountID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteRoutingBaseline(ctx context.Context, accountID string) error {
	if !positiveNumericID(accountID) {
		return errors.New("账号必须使用有效的稳定 ID")
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM routing_baselines WHERE account_id=?`, accountID)
	return err
}
