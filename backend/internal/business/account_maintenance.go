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
	AccountID         string `json:"account_id"`
	AccountName       string `json:"account_name"`
	ExpectedName      string `json:"expected_name"`
	UpstreamHost      string `json:"upstream_host"`
	UpstreamType      string `json:"upstream_type"`
	UpstreamKeyID     string `json:"upstream_key_id"`
	UpstreamGroupID   string `json:"upstream_group_id"`
	RechargeRate      string `json:"recharge_rate"`
	CurrentMultiplier string `json:"current_multiplier"`
	NamingSiteName    string `json:"-"`
	NamingBaseURL     string `json:"-"`
}

func (account BoundAccountMaintenance) NameForMultiplier(multiplier string) string {
	return naming.AccountName(account.NamingSiteName, account.NamingBaseURL, multiplier)
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

type MissingBindingCleanupResult struct {
	Cleaned int      `json:"cleaned"`
	IDs     []string `json:"ids"`
	EventID int64    `json:"event_id"`
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
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT a.id,a.name,COALESCE(a.multiplier,''),u.host,u.upstream_type,
		b.upstream_key_id,COALESCE(b.upstream_group_id,''),COALESCE(r.recharge_rate,'1'),u.base_url,u.metadata_json
		FROM accounts a JOIN bindings b ON b.local_account_id=a.id
		JOIN upstreams u ON u.host=b.upstream_host
		LEFT JOIN recharge_rates r ON r.host=u.host
		ORDER BY CASE WHEN a.id GLOB '[0-9]*' THEN CAST(a.id AS INTEGER) ELSE 0 END,a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]BoundAccountMaintenance, 0)
	for rows.Next() {
		var item BoundAccountMaintenance
		var multiplier, baseURL, metadataRaw string
		if err := rows.Scan(&item.AccountID, &item.AccountName, &multiplier, &item.UpstreamHost, &item.UpstreamType,
			&item.UpstreamKeyID, &item.UpstreamGroupID, &item.RechargeRate, &baseURL, &metadataRaw); err != nil {
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
		if _, err := tx.ExecContext(ctx, `UPDATE account_groups SET group_rate=? WHERE account_id=?`, value.Rate, value.AccountID); err != nil {
			return err
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
