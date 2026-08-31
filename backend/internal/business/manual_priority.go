package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

const (
	defaultManualPriorityMax         int64 = 10
	defaultManualPriorityLoadFactor        = "100"
	defaultManualPriorityConcurrency int64 = 100
)

type ManualPriorityConfig struct {
	ReservedMax        int64  `json:"reserved_max"`
	DefaultLoadFactor  string `json:"default_load_factor"`
	DefaultConcurrency int64  `json:"default_concurrency"`
}

type ManualPriorityAssignment struct {
	AccountID             string `json:"account_id"`
	Priority              int64  `json:"priority"`
	LoadFactor            string `json:"load_factor"`
	Concurrency           int64  `json:"concurrency"`
	SyncBalanceMultiplier bool   `json:"sync_balance_multiplier"`
}

type ManualPriorityControl struct {
	AccountID             string
	SyncBalanceMultiplier bool
}

type ManualPriorityRelease struct {
	AccountID        string
	AccountName      string
	AssignedPriority int64
	Priority         int64
	LoadFactor       *string
	Concurrency      int64
	Schedulable      *bool
}

func (s *Store) ManualPriorityConfig(ctx context.Context) (ManualPriorityConfig, error) {
	document, err := s.readPolicyDocument(ctx, s.db, "control-plane")
	if err != nil {
		return ManualPriorityConfig{}, err
	}
	return manualPriorityConfig(document)
}

func manualPriorityConfig(document map[string]any) (ManualPriorityConfig, error) {
	result := ManualPriorityConfig{
		ReservedMax: defaultManualPriorityMax, DefaultLoadFactor: defaultManualPriorityLoadFactor,
		DefaultConcurrency: defaultManualPriorityConcurrency,
	}
	if document == nil {
		return result, nil
	}
	raw, present := document["manual_priority"]
	if !present {
		return result, nil
	}
	section, ok := raw.(map[string]any)
	if !ok {
		return ManualPriorityConfig{}, errors.New("策略字段 manual_priority 必须是对象")
	}
	if value, present := section["reserved_max"]; present {
		parsed, err := strictInteger(value)
		if err != nil || parsed < 1 || parsed > 1000 {
			return ManualPriorityConfig{}, errors.New("策略字段 manual_priority.reserved_max 必须是 1 到 1000 之间的整数")
		}
		result.ReservedMax = int64(parsed)
	}
	return result, nil
}

func (s *Store) AssignManualPriority(ctx context.Context, accountID string, priority int64, loadFactor string, concurrency int64, syncBalanceMultiplier bool, actor string) (ManualPriorityAssignment, error) {
	if !positiveNumericID(accountID) {
		return ManualPriorityAssignment{}, errors.New("账号必须使用有效的稳定 ID")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ManualPriorityAssignment{}, err
	}
	defer tx.Rollback()
	document, err := s.readPolicyDocument(ctx, tx, "control-plane")
	if err != nil {
		return ManualPriorityAssignment{}, err
	}
	config, err := manualPriorityConfig(document)
	if err != nil {
		return ManualPriorityAssignment{}, err
	}
	if priority < 1 || priority > config.ReservedMax {
		return ManualPriorityAssignment{}, fmt.Errorf("人工优先位必须在 1 到 %d 之间", config.ReservedMax)
	}
	loadFactor = strings.TrimSpace(loadFactor)
	parsedLoadFactor, ok := new(big.Rat).SetString(loadFactor)
	if !ok || parsedLoadFactor.Cmp(big.NewRat(1, 1)) < 0 {
		return ManualPriorityAssignment{}, errors.New("负载因子必须大于或等于 1")
	}
	loadFactor = decimalRatText(parsedLoadFactor)
	if concurrency < 1 || concurrency > 10_000_000 {
		return ManualPriorityAssignment{}, errors.New("并发上限必须是 1 到 10000000 之间的整数")
	}
	var name string
	var previousPriority, previousConcurrency sql.NullInt64
	var previousLoadFactor sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT name,priority,load_factor,concurrency FROM accounts WHERE id=?`, accountID).
		Scan(&name, &previousPriority, &previousLoadFactor, &previousConcurrency); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ManualPriorityAssignment{}, errors.New("账号不存在")
		}
		return ManualPriorityAssignment{}, err
	}
	var occupiedBy, occupiedGroup string
	err = tx.QueryRowContext(ctx, `SELECT DISTINCT m.account_id,shared.group_name
		FROM manual_priority_accounts m
		JOIN account_groups shared ON shared.account_id=m.account_id
		JOIN account_groups current ON current.account_id=? AND current.group_name=shared.group_name
		WHERE m.priority=? AND m.account_id<>?
		ORDER BY shared.group_name,m.account_id LIMIT 1`, accountID, priority, accountID).Scan(&occupiedBy, &occupiedGroup)
	if err == nil {
		return ManualPriorityAssignment{}, fmt.Errorf("分组 %s 的人工优先位 %d 已被账号 %s 占用", occupiedGroup, priority, occupiedBy)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ManualPriorityAssignment{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO manual_priority_accounts(
		account_id,priority,previous_priority,previous_load_factor,previous_concurrency,sync_balance_multiplier,created_at,updated_at
	) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(account_id) DO UPDATE SET
		priority=excluded.priority,sync_balance_multiplier=excluded.sync_balance_multiplier,updated_at=excluded.updated_at`,
		accountID, priority, nullableInt64(previousPriority), nullString(previousLoadFactor), nullableInt64(previousConcurrency),
		boolDatabaseValue(syncBalanceMultiplier), now, now); err != nil {
		return ManualPriorityAssignment{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM routing_decisions WHERE account_id=?`, accountID); err != nil {
		return ManualPriorityAssignment{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM cleanup_states WHERE account_id=?`, accountID); err != nil {
		return ManualPriorityAssignment{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE accounts SET target_priority=NULL,target_load_factor=NULL,
		target_schedulable=NULL,target_concurrency=NULL,routing_state='manual_priority',updated_at=? WHERE id=?`, now, accountID); err != nil {
		return ManualPriorityAssignment{}, err
	}
	if err := recordManualPriorityEvent(ctx, tx, accountID, name, &priority, &syncBalanceMultiplier, actor, now); err != nil {
		return ManualPriorityAssignment{}, err
	}
	if err := tx.Commit(); err != nil {
		return ManualPriorityAssignment{}, err
	}
	return ManualPriorityAssignment{
		AccountID: accountID, Priority: priority, LoadFactor: loadFactor, Concurrency: concurrency,
		SyncBalanceMultiplier: syncBalanceMultiplier,
	}, nil
}

// RevertManualPriorityReservation only undoes a local slot reservation when no
// remote write was accepted. User-facing cancellation must go through
// CommitManualPriorityRelease after a confirmed management-platform readback.
func (s *Store) RevertManualPriorityReservation(ctx context.Context, accountID, actor string) error {
	if !positiveNumericID(accountID) {
		return errors.New("账号必须使用有效的稳定 ID")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var name string
	var schedulable, previousPriority, previousConcurrency sql.NullInt64
	var previousLoadFactor sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT a.name,a.schedulable,m.previous_priority,m.previous_load_factor,m.previous_concurrency
		FROM manual_priority_accounts m JOIN accounts a ON a.id=m.account_id WHERE m.account_id=?`, accountID).
		Scan(&name, &schedulable, &previousPriority, &previousLoadFactor, &previousConcurrency); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("账号当前不在人工优先位")
		}
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO routing_baselines(
		account_id,schedulable,priority,load_factor,concurrency,captured_at,ownership_version
	) VALUES(?,?,?,?,?,?,1)`, accountID, nullableInt64(schedulable), nullableInt64(previousPriority),
		nullString(previousLoadFactor), nullableInt64(previousConcurrency), now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM manual_priority_accounts WHERE account_id=?`, accountID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE accounts SET routing_state=NULL,target_priority=NULL,target_load_factor=NULL,
		target_schedulable=NULL,target_concurrency=NULL,updated_at=? WHERE id=?`, now, accountID); err != nil {
		return err
	}
	if result, err := tx.ExecContext(ctx, `UPDATE policy_nodes SET updated_at=? WHERE policy_key='control-plane'`, now); err != nil {
		return err
	} else if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected < 1 {
		return errors.New("控制面策略记录不存在")
	}
	if err := recordManualPriorityEvent(ctx, tx, accountID, name, nil, nil, actor, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ManualPriorityRelease(ctx context.Context, accountID string) (ManualPriorityRelease, error) {
	if !positiveNumericID(accountID) {
		return ManualPriorityRelease{}, errors.New("账号必须使用有效的稳定 ID")
	}
	var result ManualPriorityRelease
	var previousPriority, previousConcurrency, schedulable sql.NullInt64
	var previousLoadFactor sql.NullString
	result.AccountID = accountID
	if err := s.db.QueryRowContext(ctx, `SELECT a.name,m.priority,m.previous_priority,m.previous_load_factor,
		m.previous_concurrency,a.schedulable FROM manual_priority_accounts m
		JOIN accounts a ON a.id=m.account_id WHERE m.account_id=?`, accountID).Scan(
		&result.AccountName, &result.AssignedPriority, &previousPriority, &previousLoadFactor,
		&previousConcurrency, &schedulable,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ManualPriorityRelease{}, errors.New("账号当前不在人工优先位")
		}
		return ManualPriorityRelease{}, err
	}
	if !previousPriority.Valid || previousPriority.Int64 < 1 {
		return ManualPriorityRelease{}, errors.New("缺少设置人工优先位前的优先级，无法安全恢复；请先重新同步管理平台账号")
	}
	if !previousConcurrency.Valid || previousConcurrency.Int64 < 1 {
		return ManualPriorityRelease{}, errors.New("缺少设置人工优先位前的并发上限，无法安全恢复；请先重新同步管理平台账号")
	}
	result.Priority = previousPriority.Int64
	result.LoadFactor = nullString(previousLoadFactor)
	result.Concurrency = previousConcurrency.Int64
	result.Schedulable = strictNullBool(schedulable)
	return result, nil
}

func (s *Store) CommitManualPriorityRelease(
	ctx context.Context,
	release ManualPriorityRelease,
	actor string,
	operation AccountOperation,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var currentPriority int64
	if err := tx.QueryRowContext(ctx, `SELECT priority FROM manual_priority_accounts WHERE account_id=?`, release.AccountID).Scan(&currentPriority); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("人工优先位在远端确认期间已被取消")
		}
		return err
	}
	if currentPriority != release.AssignedPriority {
		return errors.New("人工优先位在远端确认期间已发生变化，请重新操作")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO routing_baselines(
		account_id,schedulable,priority,load_factor,concurrency,captured_at,ownership_version
	) VALUES(?,?,?,?,?,?,1)`, release.AccountID, boolDatabase(release.Schedulable), release.Priority,
		release.LoadFactor, release.Concurrency, now); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM manual_priority_accounts WHERE account_id=? AND priority=?`,
		release.AccountID, release.AssignedPriority)
	if err != nil {
		return err
	}
	if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected != 1 {
		return errors.New("人工优先位在远端确认期间已发生变化，请重新操作")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE accounts SET priority=?,load_factor=?,concurrency=?,routing_state=NULL,
		target_priority=NULL,target_load_factor=NULL,target_schedulable=NULL,target_concurrency=NULL,updated_at=? WHERE id=?`,
		release.Priority, release.LoadFactor, release.Concurrency, now, release.AccountID); err != nil {
		return err
	}
	if result, err := tx.ExecContext(ctx, `UPDATE policy_nodes SET updated_at=? WHERE policy_key='control-plane'`, now); err != nil {
		return err
	} else if affected, rowsErr := result.RowsAffected(); rowsErr != nil || affected < 1 {
		return errors.New("控制面策略记录不存在")
	}
	if err := recordManualPriorityEvent(ctx, tx, release.AccountID, release.AccountName, nil, nil, actor, now); err != nil {
		return err
	}
	operation.ObjectID = release.AccountID
	operation.ObjectName = &release.AccountName
	if err := insertAccountOperation(ctx, tx, operation); err != nil {
		return err
	}
	return tx.Commit()
}

func validateManualPriorityCapacity(ctx context.Context, tx *sql.Tx, document map[string]any) error {
	config, err := manualPriorityConfig(document)
	if err != nil {
		return err
	}
	var highest sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(priority) FROM manual_priority_accounts`).Scan(&highest); err != nil {
		return err
	}
	if highest.Valid && highest.Int64 > config.ReservedMax {
		return fmt.Errorf("人工优先位上限不能低于当前已占用的 %d 号位", highest.Int64)
	}
	return nil
}

func (s *Store) ManualPriorityControls(ctx context.Context, requestedIDs []string) (map[string]ManualPriorityControl, error) {
	requested := make(map[string]struct{}, len(requestedIDs))
	for _, accountID := range requestedIDs {
		requested[strings.TrimSpace(accountID)] = struct{}{}
	}
	rows, err := s.db.QueryContext(ctx, `SELECT account_id,sync_balance_multiplier FROM manual_priority_accounts ORDER BY account_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]ManualPriorityControl)
	for rows.Next() {
		var control ManualPriorityControl
		if err := rows.Scan(&control.AccountID, &control.SyncBalanceMultiplier); err != nil {
			return nil, err
		}
		if len(requested) > 0 {
			if _, found := requested[control.AccountID]; !found {
				continue
			}
		}
		result[control.AccountID] = control
	}
	return result, rows.Err()
}

func (s *Store) HostBalanceSyncAllowed(ctx context.Context, host string) (bool, error) {
	if err := s.ensureStableUpstreamRelations(ctx); err != nil {
		return false, err
	}
	var boundAccounts, syncableAccounts int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT b.local_account_id),
		COUNT(DISTINCT CASE WHEN m.account_id IS NULL OR m.sync_balance_multiplier=1 THEN b.local_account_id END)
		FROM upstream_identity_hosts h
		JOIN binding_identities bi ON bi.upstream_id=h.upstream_id
		JOIN bindings b ON b.id=bi.binding_id
		LEFT JOIN manual_priority_accounts m ON m.account_id=b.local_account_id
		WHERE h.host=?`, strings.TrimSpace(host)).Scan(&boundAccounts, &syncableAccounts)
	if err != nil {
		return false, err
	}
	return boundAccounts == 0 || syncableAccounts > 0, nil
}

func recordManualPriorityEvent(
	ctx context.Context,
	tx *sql.Tx,
	accountID, name string,
	priority *int64,
	syncBalanceMultiplier *bool,
	actor, now string,
) error {
	action, summary := "remove", fmt.Sprintf("账号 %s（%s）已取消人工优先位", name, accountID)
	if priority != nil {
		action = "assign"
		summary = fmt.Sprintf("账号 %s（%s）已设置到人工优先位 %d", name, accountID, *priority)
	}
	payload, err := json.Marshal(map[string]any{
		"account_id": accountID, "account_name": name, "action": action, "priority": priority,
		"sync_balance_multiplier": syncBalanceMultiplier, "actor": strings.TrimSpace(actor),
	})
	if err != nil {
		return err
	}
	var minimum sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MIN(source_id) FROM runtime_events WHERE source_id < 0`).Scan(&minimum); err != nil {
		return err
	}
	sourceID := int64(-1)
	if minimum.Valid && minimum.Int64 <= -1 {
		sourceID = minimum.Int64 - 1
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO runtime_events(source_id,event_type,created_at,status,summary,payload_json)
		VALUES(?,?,?,?,?,?)`, sourceID, "account.manual_priority", now, "succeeded", summary, string(payload))
	return err
}

func boolDatabaseValue(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableInt64(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}
