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
	AccountID    string `json:"account_id"`
	AccountName  string `json:"account_name"`
	ExpectedName string `json:"expected_name"`
	UpstreamHost string `json:"upstream_host"`
}

type BindingVerification struct {
	AccountID string
	Exists    bool
}

type AccountNameRepairCommit struct {
	AccountID string
	Name      string
}

type MissingBindingCleanupResult struct {
	Cleaned int      `json:"cleaned"`
	IDs     []string `json:"ids"`
	EventID int64    `json:"event_id"`
}

func (s *Store) BoundAccountsForMaintenance(ctx context.Context, requestedIDs []string) ([]BoundAccountMaintenance, error) {
	requested := make(map[string]struct{}, len(requestedIDs))
	for _, id := range requestedIDs {
		requested[strings.TrimSpace(id)] = struct{}{}
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT a.id,a.name,COALESCE(a.multiplier,''),u.host,u.base_url,u.metadata_json
		FROM accounts a JOIN bindings b ON b.local_account_id=a.id
		JOIN upstreams u ON u.host=b.upstream_host
		ORDER BY CASE WHEN a.id GLOB '[0-9]*' THEN CAST(a.id AS INTEGER) ELSE 0 END,a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]BoundAccountMaintenance, 0)
	for rows.Next() {
		var item BoundAccountMaintenance
		var multiplier, baseURL, metadataRaw string
		if err := rows.Scan(&item.AccountID, &item.AccountName, &multiplier, &item.UpstreamHost, &baseURL, &metadataRaw); err != nil {
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
		for _, table := range []string{"account_groups", "health_samples", "routing_decisions", "account_health_evaluations", "paused_accounts", "routing_baselines", "cleanup_states"} {
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
