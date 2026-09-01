package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/naming"
)

type BoundAccountMaintenance struct {
	AccountID             string `json:"account_id"`
	AccountName           string `json:"account_name"`
	ExpectedName          string `json:"expected_name"`
	UpstreamHost          string `json:"upstream_host"`
	SourceAuthHost        string `json:"source_auth_host"`
	UpstreamType          string `json:"upstream_type"`
	UpstreamKeyID         string `json:"upstream_key_id"`
	UpstreamGroupID       string `json:"upstream_group_id"`
	RechargeRate          string `json:"recharge_rate"`
	CurrentMultiplier     string `json:"current_multiplier"`
	KnownRawRate          string `json:"known_raw_rate"`
	KnownRawRateSource    string `json:"known_raw_rate_source"`
	NamingSiteName        string `json:"-"`
	NamingBaseURL         string `json:"-"`
	ConsoleOnboarded      bool   `json:"-"`
	ManualPriority        bool   `json:"-"`
	SyncBalanceMultiplier bool   `json:"-"`
}

func (account BoundAccountMaintenance) NameForMultiplier(multiplier string) string {
	return naming.AccountName(account.NamingSiteName, account.NamingBaseURL, multiplier)
}

func (account BoundAccountMaintenance) RateSourceHost() string {
	if host := strings.TrimSpace(account.SourceAuthHost); host != "" {
		return host
	}
	return strings.TrimSpace(account.UpstreamHost)
}

type BindingVerification struct {
	AccountID string
	Exists    bool
}

type AccountNameRepairCommit struct {
	AccountID string
	Name      string
}

type AccountRateObservation struct {
	AccountID string
	Rate      string
}

type AccountDefaultsRepairCommit struct {
	AccountID         string
	Priority          *int64
	Concurrency       *int64
	LoadFactorPresent bool
	LoadFactor        *string
	RemoteRepaired    bool
}

type MissingBindingCleanupResult struct {
	Cleaned int      `json:"cleaned"`
	IDs     []string `json:"ids"`
	EventID int64    `json:"event_id"`
}

type AccountUpstreamHostRepairItem struct {
	AccountID string  `json:"account_id"`
	Name      string  `json:"account_name"`
	Before    *string `json:"before"`
	After     *string `json:"after"`
	Status    string  `json:"status"`
	Reason    *string `json:"reason,omitempty"`
}

type AccountUpstreamHostRepairResult struct {
	Requested int                             `json:"requested"`
	Repaired  int                             `json:"repaired"`
	Unchanged int                             `json:"unchanged"`
	Skipped   int                             `json:"skipped"`
	Items     []AccountUpstreamHostRepairItem `json:"items"`
	EventID   int64                           `json:"event_id"`
}

func (s *Store) RepairAccountUpstreamHosts(ctx context.Context, accountIDs []string, actor string) (AccountUpstreamHostRepairResult, error) {
	result := AccountUpstreamHostRepairResult{Requested: len(accountIDs), Items: make([]AccountUpstreamHostRepairItem, 0, len(accountIDs))}
	if len(accountIDs) == 0 {
		return result, nil
	}
	if err := s.ensureStableUpstreamRelations(ctx); err != nil {
		return result, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	seen := map[string]struct{}{}
	for _, rawID := range accountIDs {
		accountID := strings.TrimSpace(rawID)
		if !positiveNumericID(accountID) {
			return result, errors.New("归属 Host 修复包含无效账号 ID")
		}
		if _, duplicate := seen[accountID]; duplicate {
			return result, fmt.Errorf("归属 Host 修复包含重复账号 ID：%s", accountID)
		}
		seen[accountID] = struct{}{}
		var name string
		var current sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT name,upstream_host FROM accounts WHERE id=?`, accountID).Scan(&name, &current); err != nil {
			return result, err
		}
		hostRows, err := tx.QueryContext(ctx, `SELECT DISTINCT bi.upstream_id,primary_host.host
			FROM bindings b JOIN binding_identities bi ON bi.binding_id=b.id
			JOIN upstream_identity_hosts primary_host ON primary_host.upstream_id=bi.upstream_id AND primary_host.is_primary=1
			WHERE b.local_account_id=? ORDER BY primary_host.host`, accountID)
		if err != nil {
			return result, err
		}
		hosts := make([]string, 0, 2)
		for hostRows.Next() {
			var upstreamID, host string
			if err := hostRows.Scan(&upstreamID, &host); err != nil {
				hostRows.Close()
				return result, err
			}
			hosts = append(hosts, host)
		}
		if err := hostRows.Err(); err != nil {
			hostRows.Close()
			return result, err
		}
		if err := hostRows.Close(); err != nil {
			return result, err
		}
		before := normalizedHost(nullString(current))
		item := AccountUpstreamHostRepairItem{AccountID: accountID, Name: name, Before: before}
		if len(hosts) == 0 {
			reason := "账号没有可用于确认归属的绑定记录"
			item.Status, item.Reason = "无法修复", &reason
			result.Skipped++
		} else if len(hosts) > 1 {
			reason := "账号绑定到了多个上游 Host，需要人工确认"
			item.Status, item.Reason = "无法自动修复", &reason
			result.Skipped++
		} else {
			after := hosts[0]
			item.After = &after
			if before != nil && strings.EqualFold(*before, after) {
				item.Status = "无需修复"
				result.Unchanged++
			} else {
				if _, err := tx.ExecContext(ctx, `UPDATE accounts SET upstream_host=?,updated_at=? WHERE id=?`, after, now, accountID); err != nil {
					return result, err
				}
				item.Status = "已修复"
				result.Repaired++
			}
		}
		result.Items = append(result.Items, item)
	}
	if result.Repaired > 0 {
		payload, err := json.Marshal(map[string]any{"actor": strings.TrimSpace(actor), "items": result.Items})
		if err != nil {
			return result, err
		}
		var minimum sql.NullInt64
		if err := tx.QueryRowContext(ctx, `SELECT MIN(source_id) FROM runtime_events WHERE source_id < 0`).Scan(&minimum); err != nil {
			return result, err
		}
		result.EventID = -1
		if minimum.Valid && minimum.Int64 <= -1 {
			result.EventID = minimum.Int64 - 1
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO runtime_events(source_id,event_type,created_at,status,summary,payload_json)
			VALUES(?,?,?,?,?,?)`, result.EventID, "account.upstream_host.repaired", now, "succeeded",
			fmt.Sprintf("已修复 %d 个账号的归属 Host", result.Repaired), string(payload)); err != nil {
			return result, err
		}
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Store) AccountNamesForMaintenance(ctx context.Context, requestedIDs []string) (map[string]string, error) {
	requested := make(map[string]struct{}, len(requestedIDs))
	for _, id := range requestedIDs {
		requested[strings.TrimSpace(id)] = struct{}{}
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,name FROM accounts ORDER BY CASE WHEN id GLOB '[0-9]*' THEN CAST(id AS INTEGER) ELSE 0 END,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]string, len(requested))
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		if len(requested) > 0 {
			if _, found := requested[id]; !found {
				continue
			}
		}
		result[id] = name
	}
	return result, rows.Err()
}

func (s *Store) BoundAccountsForMaintenance(ctx context.Context, requestedIDs []string) ([]BoundAccountMaintenance, error) {
	requested := make(map[string]struct{}, len(requestedIDs))
	for _, id := range requestedIDs {
		requested[strings.TrimSpace(id)] = struct{}{}
	}
	if err := s.ensureStableUpstreamRelations(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT a.id,a.name,COALESCE(a.multiplier,''),u.host,COALESCE(b.source_auth_host,''),u.upstream_type,
		b.upstream_key_id,COALESCE(b.upstream_group_id,''),COALESCE(source_rate.recharge_rate,primary_rate.recharge_rate,'1'),
		COALESCE(NULLIF(TRIM(b.upstream_rate),''),source_group_catalog.raw_rate,primary_group_catalog.raw_rate,''),
		CASE WHEN NULLIF(TRIM(b.upstream_rate),'') IS NOT NULL THEN 'account_observation'
			WHEN NULLIF(TRIM(source_group_catalog.raw_rate),'') IS NOT NULL
				OR NULLIF(TRIM(primary_group_catalog.raw_rate),'') IS NOT NULL THEN 'group_catalog' ELSE '' END,
		u.base_url,u.metadata_json,
		EXISTS(SELECT 1 FROM operation_audit oa WHERE oa.object_id=a.id AND oa.operation_type='account.onboarding' AND oa.state='succeeded'),
		m.account_id IS NOT NULL,COALESCE(m.sync_balance_multiplier,0)
		FROM accounts a JOIN bindings b ON b.local_account_id=a.id JOIN binding_identities bi ON bi.binding_id=b.id
		JOIN upstream_identity_hosts primary_host ON primary_host.upstream_id=bi.upstream_id AND primary_host.is_primary=1
		JOIN upstreams u ON u.host=primary_host.host
		LEFT JOIN upstream_groups source_group_catalog ON source_group_catalog.host=NULLIF(TRIM(b.source_auth_host),'')
			AND source_group_catalog.group_id=b.upstream_group_id
		LEFT JOIN upstream_groups primary_group_catalog ON primary_group_catalog.host=primary_host.host
			AND primary_group_catalog.group_id=b.upstream_group_id
		LEFT JOIN recharge_rates source_rate ON source_rate.host=NULLIF(TRIM(b.source_auth_host),'')
		LEFT JOIN recharge_rates primary_rate ON primary_rate.host=u.host
		LEFT JOIN manual_priority_accounts m ON m.account_id=a.id
		ORDER BY CASE WHEN a.id GLOB '[0-9]*' THEN CAST(a.id AS INTEGER) ELSE 0 END,a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]BoundAccountMaintenance, 0)
	for rows.Next() {
		var item BoundAccountMaintenance
		var multiplier, baseURL, metadataRaw string
		if err := rows.Scan(&item.AccountID, &item.AccountName, &multiplier, &item.UpstreamHost, &item.SourceAuthHost, &item.UpstreamType,
			&item.UpstreamKeyID, &item.UpstreamGroupID, &item.RechargeRate, &item.KnownRawRate, &item.KnownRawRateSource, &baseURL, &metadataRaw,
			&item.ConsoleOnboarded, &item.ManualPriority, &item.SyncBalanceMultiplier); err != nil {
			return nil, err
		}
		if len(requested) > 0 {
			if _, found := requested[item.AccountID]; !found {
				continue
			}
		}
		metadata := map[string]any{}
		_ = json.Unmarshal([]byte(metadataRaw), &metadata)
		siteName := strings.TrimSpace(stringValue(metadata["site_name"]))
		if siteName == "" {
			siteName = strings.TrimSpace(stringValue(metadata["system_name"]))
		}
		item.CurrentMultiplier = multiplier
		item.NamingSiteName, item.NamingBaseURL = siteName, baseURL
		item.ExpectedName = naming.AccountName(siteName, baseURL, multiplier)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) CommitAccountDefaultsRepairs(ctx context.Context, values []AccountDefaultsRepairCommit, actor string) error {
	if len(values) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	accountIDs := make([]string, 0, len(values))
	for _, value := range values {
		if !positiveNumericID(value.AccountID) {
			return errors.New("账号默认参数修复包含无效账号 ID")
		}
		updates := make([]string, 0, 4)
		arguments := make([]any, 0, 5)
		if value.Priority != nil {
			updates, arguments = append(updates, "priority=?"), append(arguments, *value.Priority)
		}
		if value.Concurrency != nil {
			updates, arguments = append(updates, "concurrency=?"), append(arguments, *value.Concurrency)
		}
		if value.LoadFactorPresent {
			if value.LoadFactor == nil {
				updates = append(updates, "load_factor=NULL")
			} else {
				updates, arguments = append(updates, "load_factor=?"), append(arguments, *value.LoadFactor)
			}
		}
		if len(updates) == 0 {
			continue
		}
		updates = append(updates, "updated_at=?")
		arguments = append(arguments, now, value.AccountID)
		result, updateErr := tx.ExecContext(ctx, `UPDATE accounts SET `+strings.Join(updates, ",")+` WHERE id=?`, arguments...)
		if updateErr != nil {
			return updateErr
		}
		if affected, affectedErr := result.RowsAffected(); affectedErr != nil {
			return affectedErr
		} else if affected != 1 {
			return fmt.Errorf("账号 %s 的本地参数记录不存在", value.AccountID)
		}
		accountIDs = append(accountIDs, value.AccountID)
	}
	remoteRepaired := 0
	for _, value := range values {
		if value.RemoteRepaired {
			remoteRepaired++
		}
	}
	payload := map[string]any{"actor": strings.TrimSpace(actor), "account_ids": accountIDs, "synchronized": len(accountIDs), "repaired": remoteRepaired, "remote_write": remoteRepaired > 0}
	if err := insertRuntimeEventWithStatus(ctx, tx, "account.defaults.reconciled", "succeeded",
		fmt.Sprintf("已核对 %d 个账号的开户默认参数，修复 %d 个", len(accountIDs), remoteRepaired), payload, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CommitBindingVerification(ctx context.Context, values []BindingVerification) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, value := range values {
		status := "verified"
		if !value.Exists {
			status = "missing"
		}
		if _, err := tx.ExecContext(ctx, `UPDATE bindings SET status=?,updated_at=? WHERE local_account_id=?`, status, now, value.AccountID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) CommitAccountNameRepairs(ctx context.Context, values []AccountNameRepairCommit) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, value := range values {
		if _, err := tx.ExecContext(ctx, `UPDATE accounts SET name=?,updated_at=? WHERE id=?`, value.Name, now, value.AccountID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE bindings SET status='verified',updated_at=? WHERE local_account_id=?`, now, value.AccountID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) CommitAccountRateObservations(ctx context.Context, values []AccountRateObservation) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, value := range values {
		if !positiveNumericID(value.AccountID) || normalizePositiveDecimal(value.Rate) == nil {
			return errors.New("账号上游倍率观测包含无效账号或倍率")
		}
		result, err := tx.ExecContext(ctx, `UPDATE bindings SET upstream_rate=?,updated_at=? WHERE local_account_id=?`, value.Rate, now, value.AccountID)
		if err != nil {
			return err
		}
		if affected, err := result.RowsAffected(); err != nil {
			return err
		} else if affected == 0 {
			return fmt.Errorf("账号 %s 没有可更新的上游绑定", value.AccountID)
		}
	}
	return tx.Commit()
}

func (s *Store) CleanupMissingBindings(ctx context.Context, accountIDs []string, actor string) (MissingBindingCleanupResult, error) {
	accountIDs, err := normalizedStableIDs(accountIDs)
	if err != nil {
		return MissingBindingCleanupResult{}, err
	}
	if len(accountIDs) == 0 {
		return MissingBindingCleanupResult{}, errors.New("没有可清理的失效绑定")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MissingBindingCleanupResult{}, err
	}
	defer tx.Rollback()
	names := make([]string, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		var total, missing int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(CASE WHEN status='missing' THEN 1 ELSE 0 END),0)
			FROM bindings WHERE local_account_id=?`, accountID).Scan(&total, &missing); err != nil {
			return MissingBindingCleanupResult{}, err
		}
		if total == 0 || total != missing {
			return MissingBindingCleanupResult{}, fmt.Errorf("账号 %s 的绑定状态已变化，请重新复验", accountID)
		}
		var name string
		if err := tx.QueryRowContext(ctx, `SELECT name FROM accounts WHERE id=?`, accountID).Scan(&name); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return MissingBindingCleanupResult{}, err
		}
		if strings.TrimSpace(name) == "" {
			name = "账号 " + accountID
		}
		names = append(names, name)
		for _, table := range []string{"account_groups", "health_samples", "routing_decisions", "account_health_evaluations", "paused_accounts", "manual_priority_accounts", "routing_baselines", "cleanup_states"} {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE account_id=?", accountID); err != nil {
				return MissingBindingCleanupResult{}, err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM bindings WHERE local_account_id=?`, accountID); err != nil {
			return MissingBindingCleanupResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM accounts WHERE id=?`, accountID); err != nil {
			return MissingBindingCleanupResult{}, err
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE local_groups SET account_count=(
		SELECT COUNT(*) FROM account_groups WHERE group_name=local_groups.name
	),updated_at=?`, now); err != nil {
		return MissingBindingCleanupResult{}, err
	}
	payload := map[string]any{"actor": strings.TrimSpace(actor), "account_ids": accountIDs, "account_names": names,
		"cleaned": len(accountIDs), "remote_write": false}
	if err := insertRuntimeEventWithStatus(ctx, tx, "binding.missing.cleaned", "succeeded",
		fmt.Sprintf("已清理 %d 个管理平台不存在的账号绑定", len(accountIDs)), payload, now); err != nil {
		return MissingBindingCleanupResult{}, err
	}
	var eventID int64
	if err := tx.QueryRowContext(ctx, `SELECT MIN(source_id) FROM runtime_events WHERE source_id < 0`).Scan(&eventID); err != nil {
		return MissingBindingCleanupResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return MissingBindingCleanupResult{}, err
	}
	return MissingBindingCleanupResult{Cleaned: len(accountIDs), IDs: accountIDs, EventID: eventID}, nil
}
