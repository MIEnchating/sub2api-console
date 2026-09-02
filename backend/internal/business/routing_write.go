package business

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type RoutingBaseline struct {
	AccountID          string
	TargetFingerprint  string
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
	rows, err := s.db.QueryContext(ctx, `SELECT account_id,target_fingerprint,schedulable,priority,load_factor,concurrency,status,captured_at,
		ownership_version,managed_schedulable,managed_priority,managed_load_factor,managed_concurrency,managed_status
		FROM routing_baselines WHERE ownership_version<>2
		ORDER BY CASE WHEN account_id GLOB '[0-9]*' THEN CAST(account_id AS INTEGER) ELSE 0 END,account_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RoutingBaseline{}
	for rows.Next() {
		var item RoutingBaseline
		var schedulable, priority, concurrency, managedSchedulable, managedPriority, managedConcurrency sql.NullInt64
		var loadFactor, status, managedLoadFactor, managedStatus sql.NullString
		if err := rows.Scan(&item.AccountID, &item.TargetFingerprint, &schedulable, &priority, &loadFactor, &concurrency, &status, &item.CapturedAt,
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
	if !validRoutingTargetFingerprint(baseline.TargetFingerprint) {
		return errors.New("调度基线缺少有效的管理目标指纹")
	}
	capturedAt := strings.TrimSpace(baseline.CapturedAt)
	if capturedAt == "" {
		capturedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO routing_baselines(
		account_id,target_fingerprint,schedulable,priority,load_factor,concurrency,status,captured_at,ownership_version
	) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(account_id) DO UPDATE SET
		target_fingerprint=excluded.target_fingerprint,
		schedulable=CASE WHEN routing_baselines.ownership_version=2 OR routing_baselines.target_fingerprint<>excluded.target_fingerprint THEN excluded.schedulable ELSE routing_baselines.schedulable END,
		priority=CASE WHEN routing_baselines.ownership_version=2 OR routing_baselines.target_fingerprint<>excluded.target_fingerprint THEN excluded.priority ELSE routing_baselines.priority END,
		load_factor=CASE WHEN routing_baselines.ownership_version=2 OR routing_baselines.target_fingerprint<>excluded.target_fingerprint THEN excluded.load_factor ELSE routing_baselines.load_factor END,
		concurrency=CASE WHEN routing_baselines.ownership_version=2 OR routing_baselines.target_fingerprint<>excluded.target_fingerprint THEN excluded.concurrency ELSE routing_baselines.concurrency END,
		status=CASE WHEN routing_baselines.ownership_version=2 OR routing_baselines.target_fingerprint<>excluded.target_fingerprint THEN excluded.status ELSE routing_baselines.status END,
		captured_at=CASE WHEN routing_baselines.ownership_version=2 OR routing_baselines.target_fingerprint<>excluded.target_fingerprint THEN excluded.captured_at ELSE routing_baselines.captured_at END,
		ownership_version=CASE WHEN routing_baselines.ownership_version=2 OR routing_baselines.target_fingerprint<>excluded.target_fingerprint THEN 1 ELSE routing_baselines.ownership_version END,
		managed_schedulable=CASE WHEN routing_baselines.ownership_version=2 OR routing_baselines.target_fingerprint<>excluded.target_fingerprint THEN NULL ELSE routing_baselines.managed_schedulable END,
		managed_priority=CASE WHEN routing_baselines.ownership_version=2 OR routing_baselines.target_fingerprint<>excluded.target_fingerprint THEN NULL ELSE routing_baselines.managed_priority END,
		managed_load_factor=CASE WHEN routing_baselines.ownership_version=2 OR routing_baselines.target_fingerprint<>excluded.target_fingerprint THEN NULL ELSE routing_baselines.managed_load_factor END,
		managed_concurrency=CASE WHEN routing_baselines.ownership_version=2 OR routing_baselines.target_fingerprint<>excluded.target_fingerprint THEN NULL ELSE routing_baselines.managed_concurrency END,
		managed_status=CASE WHEN routing_baselines.ownership_version=2 OR routing_baselines.target_fingerprint<>excluded.target_fingerprint THEN NULL ELSE routing_baselines.managed_status END`,
		baseline.AccountID, baseline.TargetFingerprint, boolDatabase(baseline.Schedulable), baseline.Priority,
		baseline.LoadFactor, baseline.Concurrency, baseline.Status, capturedAt, baseline.OwnershipVersion)
	if err != nil {
		return err
	}
	if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
		return fmt.Errorf("账号 %s 的调度基线属于其他管理目标", baseline.AccountID)
	}
	return nil
}

func (s *Store) UpdateRoutingManagedIntent(ctx context.Context, accountID, targetFingerprint string, intent RoutingManagedIntent) error {
	if !positiveNumericID(accountID) {
		return errors.New("账号必须使用有效的稳定 ID")
	}
	if !validRoutingTargetFingerprint(targetFingerprint) {
		return errors.New("调度写入所有权缺少有效的管理目标指纹")
	}
	sets, arguments := routingManagedIntentUpdate(intent)
	arguments = append(arguments, accountID, targetFingerprint)
	result, err := s.db.ExecContext(ctx, `UPDATE routing_baselines SET `+strings.Join(sets, ",")+` WHERE account_id=? AND target_fingerprint=?`, arguments...)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return fmt.Errorf("保存账号 %s 的调度写入所有权失败", accountID)
	}
	return nil
}

func routingManagedIntentUpdate(intent RoutingManagedIntent) ([]string, []any) {
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
	return sets, arguments
}

func (s *Store) CommitRoutingReadback(
	ctx context.Context,
	accountID string,
	targetFingerprint string,
	readback RoutingReadback,
	intent *RoutingManagedIntent,
	deleteBaseline bool,
	operation AccountOperation,
) error {
	return s.commitAccountMutation(ctx, accountID, operation, func(tx *sql.Tx, now string) error {
		if deleteBaseline || intent != nil {
			if err := requireRoutingBaselineTarget(ctx, tx, accountID, targetFingerprint); err != nil {
				return err
			}
		}
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
			var result sql.Result
			result, err = tx.ExecContext(ctx, `DELETE FROM routing_baselines WHERE account_id=? AND target_fingerprint=?`, accountID, targetFingerprint)
			if err == nil {
				if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
					err = fmt.Errorf("账号 %s 的调度基线在提交前已变化", accountID)
				}
			}
		} else if intent != nil {
			sets, arguments := routingManagedIntentUpdate(*intent)
			arguments = append(arguments, accountID, targetFingerprint)
			var result sql.Result
			result, err = tx.ExecContext(ctx, `UPDATE routing_baselines SET `+strings.Join(sets, ",")+` WHERE account_id=? AND target_fingerprint=?`, arguments...)
			if err == nil {
				if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
					err = fmt.Errorf("保存账号 %s 的调度写入所有权失败", accountID)
				}
			}
		}
		return err
	})
}

func (s *Store) AbandonRoutingControl(
	ctx context.Context,
	accountID string,
	targetFingerprint string,
	readback RoutingReadback,
	operation AccountOperation,
) error {
	return s.commitAccountMutation(ctx, accountID, operation, func(tx *sql.Tx, now string) error {
		if err := requireRoutingBaselineTarget(ctx, tx, accountID, targetFingerprint); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE accounts SET schedulable=?,priority=?,load_factor=?,concurrency=?,
			routing_state='',updated_at=? WHERE id=?`, boolDatabase(readback.Schedulable), readback.Priority,
			readback.LoadFactor, readback.Concurrency, now, accountID)
		if err != nil {
			return err
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			return sql.ErrNoRows
		}
		result, err = tx.ExecContext(ctx, `UPDATE routing_baselines SET ownership_version=2,
			managed_schedulable=NULL,managed_priority=NULL,managed_load_factor=NULL,
			managed_concurrency=NULL,managed_status=NULL WHERE account_id=? AND target_fingerprint=?`, accountID, targetFingerprint)
		if err != nil {
			return err
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			return fmt.Errorf("账号 %s 缺少可释放的调度所有权", accountID)
		}
		return nil
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
	encoded, err := clearedRoutingRuntimeMetadata(raw, accountID)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE accounts SET metadata_json=?,updated_at=? WHERE id=?`,
		encoded, time.Now().UTC().Format(time.RFC3339Nano), accountID)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func clearedRoutingRuntimeMetadata(raw, accountID string) (string, error) {
	metadata := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return "", fmt.Errorf("账号 %s metadata 配置无效", accountID)
	}
	for _, key := range []string{
		"last_error", "error_message", "rate_limited_at", "rate_limit_reset_at",
		"temp_unschedulable_until", "temp_unschedulable_reason", "overload_until",
	} {
		delete(metadata, key)
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func (s *Store) DeleteRoutingBaseline(ctx context.Context, accountID, targetFingerprint string) error {
	if !positiveNumericID(accountID) {
		return errors.New("账号必须使用有效的稳定 ID")
	}
	if !validRoutingTargetFingerprint(targetFingerprint) {
		return errors.New("删除调度基线缺少有效的管理目标指纹")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM routing_baselines WHERE account_id=? AND target_fingerprint=?`, accountID, targetFingerprint)
	if err != nil {
		return err
	}
	if affected, rowsErr := result.RowsAffected(); rowsErr != nil {
		return rowsErr
	} else if affected == 1 {
		return nil
	}
	var foundFingerprint string
	err = s.db.QueryRowContext(ctx, `SELECT target_fingerprint FROM routing_baselines WHERE account_id=?`, accountID).Scan(&foundFingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("账号 %s 的调度基线属于其他管理目标", accountID)
}

func validRoutingTargetFingerprint(value string) bool {
	if len(value) != 64 || strings.TrimSpace(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func requireRoutingBaselineTarget(ctx context.Context, queryer connectionQueryer, accountID, targetFingerprint string) error {
	if !validRoutingTargetFingerprint(targetFingerprint) {
		return errors.New("调度基线操作缺少有效的管理目标指纹")
	}
	var foundFingerprint string
	err := queryer.QueryRowContext(ctx, `SELECT target_fingerprint FROM routing_baselines WHERE account_id=?`, accountID).Scan(&foundFingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("账号 %s 缺少调度基线", accountID)
	}
	if err != nil {
		return err
	}
	if foundFingerprint != targetFingerprint {
		return fmt.Errorf("账号 %s 的调度基线属于其他管理目标", accountID)
	}
	return nil
}
