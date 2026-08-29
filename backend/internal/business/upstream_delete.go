package business

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type UpstreamDeleteAccount struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Groups []string `json:"groups"`
}

type UpstreamDeletePreview struct {
	Host         string                  `json:"host"`
	BaseURL      string                  `json:"base_url"`
	UpstreamType string                  `json:"upstream_type"`
	AccountCount int                     `json:"account_count"`
	GroupCount   int                     `json:"group_count"`
	AccountIDs   []string                `json:"account_ids"`
	Accounts     []UpstreamDeleteAccount `json:"accounts"`
}

type UpstreamDeleteProjection struct {
	Host            string `json:"host"`
	DeletedAccounts int    `json:"deleted_accounts"`
	DeletedGroups   int    `json:"deleted_groups"`
	EventID         int64  `json:"event_id"`
}

type UpstreamDeleteAudit struct {
	Actor                 string
	RemoteDeletedAccounts int
	PrivateAuthDeleted    bool
	ReadbackConfirmed     bool
}

func (s *Store) UpstreamDeletePreview(ctx context.Context, host string) (UpstreamDeletePreview, error) {
	return upstreamDeletePreview(ctx, s.db, canonicalHost(host))
}

func (s *Store) DeleteUpstreamProjection(ctx context.Context, host string, expectedAccountIDs []string, audit UpstreamDeleteAudit) (UpstreamDeleteProjection, error) {
	host = canonicalHost(host)
	expected, err := normalizedStableIDs(expectedAccountIDs)
	if err != nil {
		return UpstreamDeleteProjection{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UpstreamDeleteProjection{}, err
	}
	defer tx.Rollback()
	preview, err := upstreamDeletePreview(ctx, tx, host)
	if err != nil {
		return UpstreamDeleteProjection{}, err
	}
	current := append([]string{}, preview.AccountIDs...)
	sort.Strings(current)
	if strings.Join(current, "\x00") != strings.Join(expected, "\x00") {
		return UpstreamDeleteProjection{}, errors.New("删除预览后的账号范围已变化，请重新确认")
	}
	for _, accountID := range preview.AccountIDs {
		for _, table := range []string{"account_groups", "health_samples", "routing_decisions", "account_health_evaluations", "paused_accounts", "manual_priority_accounts", "routing_baselines", "cleanup_states"} {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE account_id=?", accountID); err != nil {
				return UpstreamDeleteProjection{}, err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM bindings WHERE local_account_id=?`, accountID); err != nil {
			return UpstreamDeleteProjection{}, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM accounts WHERE id=?`, accountID); err != nil {
			return UpstreamDeleteProjection{}, err
		}
	}
	for _, statement := range []string{
		"DELETE FROM bindings WHERE upstream_host=?",
		"DELETE FROM onboarding_pending WHERE upstream_host=?",
		"DELETE FROM upstream_keys WHERE host=?",
		"DELETE FROM upstream_groups WHERE host=?",
		"DELETE FROM recharge_rates WHERE host=?",
		"DELETE FROM upstreams WHERE host=?",
	} {
		if _, err := tx.ExecContext(ctx, statement, host); err != nil {
			return UpstreamDeleteProjection{}, err
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE local_groups SET account_count=(
		SELECT COUNT(*) FROM account_groups WHERE group_name=local_groups.name
	),updated_at=?`, now); err != nil {
		return UpstreamDeleteProjection{}, err
	}
	payload := map[string]any{
		"actor":                strings.TrimSpace(audit.Actor),
		"host":                 host,
		"deleted_accounts":     audit.RemoteDeletedAccounts,
		"deleted_groups":       preview.GroupCount,
		"private_auth_deleted": audit.PrivateAuthDeleted,
		"remote_write":         true,
		"readback_confirmed":   audit.ReadbackConfirmed,
	}
	if err := insertRuntimeEventWithStatus(ctx, tx, "upstream.deleted", "succeeded",
		fmt.Sprintf("上游 %s 已删除：账号 %d，分组 %d", host, audit.RemoteDeletedAccounts, preview.GroupCount), payload, now); err != nil {
		return UpstreamDeleteProjection{}, err
	}
	var eventID int64
	if err := tx.QueryRowContext(ctx, `SELECT MIN(source_id) FROM runtime_events WHERE source_id < 0`).Scan(&eventID); err != nil {
		return UpstreamDeleteProjection{}, err
	}
	if err := tx.Commit(); err != nil {
		return UpstreamDeleteProjection{}, err
	}
	return UpstreamDeleteProjection{Host: host, DeletedAccounts: len(preview.AccountIDs), DeletedGroups: preview.GroupCount, EventID: eventID}, nil
}

type deletePreviewQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type upstreamDeleteAccountRow struct {
	ID, Name string
	Host     sql.NullString
}

func upstreamDeletePreview(ctx context.Context, queryer deletePreviewQueryer, host string) (UpstreamDeletePreview, error) {
	if host == "" {
		return UpstreamDeletePreview{}, errors.New("上游 Host 不能为空")
	}
	result := UpstreamDeletePreview{Host: host, AccountIDs: []string{}, Accounts: []UpstreamDeleteAccount{}}
	if err := queryer.QueryRowContext(ctx, `SELECT base_url,upstream_type FROM upstreams WHERE host=?`, host).Scan(&result.BaseURL, &result.UpstreamType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UpstreamDeletePreview{}, errors.New("上游 Host 不存在")
		}
		return UpstreamDeletePreview{}, err
	}
	accountRows, err := upstreamDeleteAccountRows(ctx, queryer, host)
	if err != nil {
		return UpstreamDeletePreview{}, err
	}
	for _, row := range accountRows {
		account := UpstreamDeleteAccount{ID: row.ID, Name: row.Name}
		if !positiveNumericID(row.ID) {
			return UpstreamDeletePreview{}, errors.New("关联账号不是可用于 Sub2API 删除的稳定数字 ID")
		}
		if row.Host.Valid && canonicalHost(row.Host.String) != "" && canonicalHost(row.Host.String) != host {
			return UpstreamDeletePreview{}, errors.New("关联账号的主 Host 与删除目标不一致，拒绝级联删除")
		}
		var otherHost bool
		if err := queryer.QueryRowContext(ctx, `SELECT EXISTS(
			SELECT 1 FROM bindings WHERE local_account_id=? AND upstream_host<>?
		)`, account.ID, host).Scan(&otherHost); err != nil {
			return UpstreamDeletePreview{}, err
		}
		if otherHost {
			return UpstreamDeletePreview{}, errors.New("关联账号同时绑定其他 Host，拒绝级联删除")
		}
		account.Groups, err = upstreamDeleteAccountGroups(ctx, queryer, account.ID)
		if err != nil {
			return UpstreamDeletePreview{}, err
		}
		result.AccountIDs = append(result.AccountIDs, account.ID)
		result.Accounts = append(result.Accounts, account)
	}
	result.AccountCount = len(result.Accounts)
	if err := queryer.QueryRowContext(ctx, `SELECT COUNT(*) FROM upstream_groups WHERE host=?`, host).Scan(&result.GroupCount); err != nil {
		return UpstreamDeletePreview{}, err
	}
	return result, nil
}

func upstreamDeleteAccountRows(ctx context.Context, queryer deletePreviewQueryer, host string) ([]upstreamDeleteAccountRow, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT DISTINCT a.id,a.name,a.upstream_host
		FROM accounts a LEFT JOIN bindings b ON b.local_account_id=a.id
		WHERE a.upstream_host=? OR b.upstream_host=?
		ORDER BY CASE WHEN a.id GLOB '[0-9]*' THEN CAST(a.id AS INTEGER) ELSE 0 END,a.id`, host, host)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []upstreamDeleteAccountRow{}
	for rows.Next() {
		var account upstreamDeleteAccountRow
		if err := rows.Scan(&account.ID, &account.Name, &account.Host); err != nil {
			return nil, err
		}
		result = append(result, account)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return result, nil
}

func upstreamDeleteAccountGroups(ctx context.Context, queryer deletePreviewQueryer, accountID string) ([]string, error) {
	rows, err := queryer.QueryContext(ctx, `SELECT group_name FROM account_groups WHERE account_id=? ORDER BY group_name`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := []string{}
	for rows.Next() {
		var group string
		if err := rows.Scan(&group); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func normalizedStableIDs(values []string) ([]string, error) {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !positiveNumericID(value) {
			return nil, errors.New("预期账号必须全部使用稳定数字 ID")
		}
		if _, found := seen[value]; found {
			return nil, errors.New("预期账号 ID 不能重复")
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}
