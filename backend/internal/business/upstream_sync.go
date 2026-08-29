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

type UpstreamCatalogGroup struct {
	GroupID     string
	Name        string
	Description *string
	Platform    *string
	Status      *string
	RawRate     *string
}

type UpstreamCatalogKey struct {
	KeyID         string
	Name          string
	UpstreamGroup *string
	RateAmbiguous bool
	Status        *string
	Rate          *string
}

type UpstreamCatalogSnapshot struct {
	Groups []UpstreamCatalogGroup
	Keys   []UpstreamCatalogKey
}

type UpstreamBalanceObservation struct {
	RawBalance        *string
	Status            string
	HardClosed        *bool
	HardClosedPresent bool
	SiteName          *string
	QuotaPerUnit      *string
	BalanceUnit       *string
}

type UpstreamSyncWrite struct {
	Host             string
	Catalog          *UpstreamCatalogSnapshot
	Balance          *UpstreamBalanceObservation
	NameOnly         bool
	KeyID            *string
	AuthRecovered    bool
	AuthenticationOK bool
}

type UpstreamSyncWriteResult struct {
	Host                 string  `json:"host"`
	GroupCount           int     `json:"group_count"`
	KeyCount             int     `json:"key_count"`
	AccountTotal         int     `json:"account_total"`
	AccountRateSucceeded int     `json:"account_rate_succeeded"`
	AccountRateFailed    int     `json:"account_rate_failed"`
	RawBalance           *string `json:"raw_balance"`
	Balance              *string `json:"balance"`
	BalanceStatus        string  `json:"balance_status"`
	CheckedAt            string  `json:"checked_at"`
}

func (s *Store) ApplyUpstreamSync(ctx context.Context, value UpstreamSyncWrite) (UpstreamSyncWriteResult, error) {
	host := canonicalHost(value.Host)
	if host == "" || (value.Catalog == nil && value.Balance == nil) {
		return UpstreamSyncWriteResult{}, errors.New("上游同步必须包含 Host 以及分组或余额数据")
	}
	if value.NameOnly && (value.Catalog != nil || value.Balance == nil) {
		return UpstreamSyncWriteResult{}, errors.New("上游名称修复只能包含公开站点名称")
	}
	groups, keys, err := normalizeUpstreamCatalog(value.Catalog)
	if err != nil {
		return UpstreamSyncWriteResult{}, err
	}
	partialCatalog := value.KeyID != nil
	if partialCatalog {
		if value.Catalog == nil {
			return UpstreamSyncWriteResult{}, errors.New("按 Key 同步必须包含分组目录")
		}
		keyID := strings.TrimSpace(*value.KeyID)
		if keyID == "" {
			return UpstreamSyncWriteResult{}, errors.New("按 Key 同步必须包含稳定 Key ID")
		}
		selected := make([]UpstreamCatalogKey, 0, 1)
		for _, key := range keys {
			if key.KeyID == keyID {
				selected = append(selected, key)
				break
			}
		}
		if len(selected) != 1 {
			return UpstreamSyncWriteResult{}, errors.New("上游目录未返回指定稳定 Key ID")
		}
		selectedGroups := make([]UpstreamCatalogGroup, 0, 1)
		if selected[0].UpstreamGroup != nil {
			groupReference := strings.TrimSpace(*selected[0].UpstreamGroup)
			for _, group := range groups {
				if group.GroupID == groupReference || group.Name == groupReference {
					selectedGroups = append(selectedGroups, group)
					break
				}
			}
		}
		groups, keys = selectedGroups, selected
	}
	balance, err := normalizeUpstreamBalance(value.Balance)
	if err != nil {
		return UpstreamSyncWriteResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UpstreamSyncWriteResult{}, err
	}
	defer tx.Rollback()
	var metadataRaw string
	var existingRaw, existingMapped sql.NullString
	var existingAuthStatus string
	if err := tx.QueryRowContext(ctx, `SELECT metadata_json,raw_balance,mapped_balance,auth_status FROM upstreams WHERE host=?`, host).
		Scan(&metadataRaw, &existingRaw, &existingMapped, &existingAuthStatus); errors.Is(err, sql.ErrNoRows) {
		return UpstreamSyncWriteResult{}, errors.New("上游 Host 不存在")
	} else if err != nil {
		return UpstreamSyncWriteResult{}, err
	}
	metadata, err := decodeObject(metadataRaw)
	if err != nil {
		return UpstreamSyncWriteResult{}, errors.New("上游 metadata 记录损坏")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var rechargeRaw string
	if err := tx.QueryRowContext(ctx, `SELECT recharge_rate FROM recharge_rates WHERE host=?`, host).Scan(&rechargeRaw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			rechargeRaw = "1"
			if _, insertErr := tx.ExecContext(ctx, `INSERT INTO recharge_rates(host,recharge_rate,note,updated_at)
				VALUES(?,?,?,?)`, host, rechargeRaw, "console-sync-default", now); insertErr != nil {
				return UpstreamSyncWriteResult{}, insertErr
			}
		} else {
			return UpstreamSyncWriteResult{}, err
		}
	}
	recharge := normalizePositiveDecimal(rechargeRaw)
	if recharge == nil {
		return UpstreamSyncWriteResult{}, errors.New("倍率必须是有限正数")
	}
	result := UpstreamSyncWriteResult{Host: host, CheckedAt: now, BalanceStatus: stringValue(metadata["balance_status"])}
	if existingRaw.Valid {
		result.RawBalance = normalizeDecimal(existingRaw.String)
	}
	if existingMapped.Valid {
		result.Balance = normalizeDecimal(existingMapped.String)
	}
	if value.NameOnly {
		setOptionalMetadata(metadata, "site_name", value.Balance.SiteName)
		metadata["name_status"], metadata["name_error"], metadata["name_checked_at"] = "已读取", nil, now
		balance = nil
	}

	if value.Catalog != nil {
		result.AccountTotal, result.AccountRateSucceeded, result.AccountRateFailed, err = catalogAccountRateCounts(
			ctx, tx, host, keys, partialCatalog,
		)
		if err != nil {
			return UpstreamSyncWriteResult{}, err
		}
		if err := persistCatalogTx(ctx, tx, host, groups, keys, recharge, now, partialCatalog); err != nil {
			return UpstreamSyncWriteResult{}, err
		}
		result.GroupCount, result.KeyCount = len(groups), len(keys)
		if !partialCatalog {
			metadata["catalog_status"] = "已同步"
			metadata["catalog_checked_at"] = now
			metadata["catalog_group_count"] = len(groups)
			metadata["catalog_key_count"] = len(keys)
			metadata["catalog_error"] = nil
		}
		metadata["rate_sync_status"] = "succeeded"
		metadata["rate_sync_error"] = nil
		metadata["rate_sync_at"] = now
	}
	if balance != nil {
		result.RawBalance = balance.RawBalance
		result.Balance = divideDecimalPointers(balance.RawBalance, recharge)
		result.BalanceStatus = balance.Status
		metadata["balance_status"] = balance.Status
		metadata["balance_error"] = nil
		metadata["balance_checked_at"] = now
		setOptionalMetadata(metadata, "site_name", balance.SiteName)
		setOptionalMetadata(metadata, "quota_per_unit", balance.QuotaPerUnit)
		setOptionalMetadata(metadata, "balance_unit", balance.BalanceUnit)
		if balance.HardClosedPresent {
			if balance.HardClosed == nil {
				delete(metadata, "balance_hard_closed")
			} else {
				metadata["balance_hard_closed"] = *balance.HardClosed
			}
		} else if balance.RawBalance != nil {
			metadata["balance_hard_closed"] = false
		}
		if _, err := tx.ExecContext(ctx, `UPDATE upstreams SET balance=?,raw_balance=?,mapped_balance=?,checked_at=?,updated_at=? WHERE host=?`,
			balance.RawBalance, balance.RawBalance, result.Balance, now, now, host); err != nil {
			return UpstreamSyncWriteResult{}, err
		}
	}
	if value.AuthenticationOK {
		metadata["auth_checked_at"] = now
		metadata["auth_verified_at"] = now
		metadata["auth_error"] = nil
		if value.AuthRecovered {
			metadata["auth_recovered_at"] = now
		}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return UpstreamSyncWriteResult{}, err
	}
	authStatus := existingAuthStatus
	if value.AuthenticationOK {
		authStatus = "已鉴权"
		if value.AuthRecovered {
			authStatus = "已恢复"
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE upstreams SET auth_status=?,metadata_json=?,updated_at=? WHERE host=?`, authStatus, string(encoded), now, host); err != nil {
		return UpstreamSyncWriteResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return UpstreamSyncWriteResult{}, err
	}
	return result, nil
}

func catalogAccountRateCounts(
	ctx context.Context,
	tx *sql.Tx,
	host string,
	keys []UpstreamCatalogKey,
	partial bool,
) (int, int, int, error) {
	selectedKeys := make(map[string]struct{}, len(keys))
	rateKeys := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		selectedKeys[key.KeyID] = struct{}{}
		if key.Rate != nil {
			rateKeys[key.KeyID] = struct{}{}
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT upstream_key_id FROM bindings WHERE upstream_host=?`, host)
	if err != nil {
		return 0, 0, 0, err
	}
	defer rows.Close()
	total, succeeded := 0, 0
	for rows.Next() {
		var keyID sql.NullString
		if err := rows.Scan(&keyID); err != nil {
			return 0, 0, 0, err
		}
		if partial {
			if _, selected := selectedKeys[keyID.String]; !keyID.Valid || !selected {
				continue
			}
		}
		total++
		if keyID.Valid {
			if _, synced := rateKeys[keyID.String]; synced {
				succeeded++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, 0, err
	}
	return total, succeeded, total - succeeded, nil
}

func (s *Store) RecordUpstreamSyncFailure(ctx context.Context, host, scope, reason string, authenticationFailure bool) error {
	host = canonicalHost(host)
	reason = redactedFailureReason(reason)
	if host == "" || reason == "" {
		return errors.New("上游同步失败记录缺少 Host 或原因")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw, existingAuthStatus string
	if err := tx.QueryRowContext(ctx, `SELECT metadata_json,auth_status FROM upstreams WHERE host=?`, host).Scan(&raw, &existingAuthStatus); errors.Is(err, sql.ErrNoRows) {
		return errors.New("上游 Host 不存在")
	} else if err != nil {
		return err
	}
	metadata, err := decodeObject(raw)
	if err != nil {
		return errors.New("上游 metadata 记录损坏")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	switch scope {
	case "name":
		metadata["name_status"], metadata["name_error"], metadata["name_checked_at"] = "读取失败", reason, now
	case "catalog":
		metadata["catalog_status"], metadata["catalog_error"], metadata["catalog_checked_at"] = "同步失败", reason, now
		metadata["rate_sync_status"], metadata["rate_sync_error"], metadata["rate_sync_at"] = "failed", reason, now
	case "balance":
		metadata["balance_status"], metadata["balance_error"], metadata["balance_checked_at"] = "读取失败", reason, now
	default:
		metadata["catalog_status"], metadata["catalog_error"], metadata["catalog_checked_at"] = "同步失败", reason, now
		metadata["balance_status"], metadata["balance_error"], metadata["balance_checked_at"] = "读取失败", reason, now
		metadata["rate_sync_status"], metadata["rate_sync_error"], metadata["rate_sync_at"] = "failed", reason, now
	}
	authStatus := "未确认"
	if scope == "name" {
		authStatus = existingAuthStatus
	}
	if authenticationFailure {
		authStatus = "鉴权失效"
		metadata["auth_checked_at"], metadata["auth_error"] = now, reason
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE upstreams SET auth_status=?,metadata_json=?,updated_at=? WHERE host=?`, authStatus, string(encoded), now, host); err != nil {
		return err
	}
	return tx.Commit()
}

func normalizeUpstreamCatalog(value *UpstreamCatalogSnapshot) ([]UpstreamCatalogGroup, []UpstreamCatalogKey, error) {
	if value == nil {
		return nil, nil, nil
	}
	groups := make([]UpstreamCatalogGroup, len(value.Groups))
	groupIDs := map[string]struct{}{}
	for index, raw := range value.Groups {
		item := raw
		item.GroupID, item.Name = strings.TrimSpace(item.GroupID), strings.TrimSpace(item.Name)
		if item.GroupID == "" || item.Name == "" {
			return nil, nil, fmt.Errorf("上游分组目录第 %d 项缺少稳定 ID 或名称", index+1)
		}
		if _, exists := groupIDs[item.GroupID]; exists {
			return nil, nil, fmt.Errorf("上游分组目录包含重复 ID：%s", item.GroupID)
		}
		groupIDs[item.GroupID] = struct{}{}
		if item.RawRate != nil {
			item.RawRate = normalizeDecimal(*item.RawRate)
			if item.RawRate == nil || *item.RawRate == "" {
				return nil, nil, fmt.Errorf("上游分组 %s 倍率不是有限数值", item.GroupID)
			}
		}
		groups[index] = item
	}
	keys := make([]UpstreamCatalogKey, len(value.Keys))
	keyIDs := map[string]struct{}{}
	for index, raw := range value.Keys {
		item := raw
		item.KeyID, item.Name = strings.TrimSpace(item.KeyID), strings.TrimSpace(item.Name)
		if item.KeyID == "" {
			return nil, nil, fmt.Errorf("上游 Key 目录第 %d 项缺少稳定 ID", index+1)
		}
		if item.Name == "" {
			item.Name = item.KeyID
		}
		if _, exists := keyIDs[item.KeyID]; exists {
			return nil, nil, fmt.Errorf("上游 Key 目录包含重复 ID：%s", item.KeyID)
		}
		keyIDs[item.KeyID] = struct{}{}
		if item.Rate != nil {
			item.Rate = normalizeDecimal(*item.Rate)
			if item.Rate == nil || *item.Rate == "" {
				return nil, nil, fmt.Errorf("上游 Key %s 倍率不是有限数值", item.KeyID)
			}
		}
		keys[index] = item
	}
	return groups, keys, nil
}

func normalizeUpstreamBalance(value *UpstreamBalanceObservation) (*UpstreamBalanceObservation, error) {
	if value == nil {
		return nil, nil
	}
	result := *value
	if result.Status == "" {
		result.Status = "未返回余额"
	}
	if result.RawBalance != nil {
		result.RawBalance = normalizeDecimal(*result.RawBalance)
		if result.RawBalance == nil || *result.RawBalance == "" {
			return nil, errors.New("上游原始余额必须是有限数值")
		}
	}
	return &result, nil
}

func persistCatalogTx(ctx context.Context, tx *sql.Tx, host string, groups []UpstreamCatalogGroup, keys []UpstreamCatalogKey, recharge *string, now string, partial bool) error {
	boundGroups, err := stringSetFromQueryer(ctx, tx, `SELECT upstream_group_id FROM bindings WHERE upstream_host=? AND upstream_group_id IS NOT NULL`, host)
	if err != nil {
		return err
	}
	boundKeys, err := stringSetFromQueryer(ctx, tx, `SELECT upstream_key_id FROM bindings WHERE upstream_host=? AND upstream_key_id IS NOT NULL`, host)
	if err != nil {
		return err
	}
	groupIDs := map[string]struct{}{}
	for _, item := range groups {
		groupIDs[item.GroupID] = struct{}{}
		effective := divideDecimalPointers(item.RawRate, recharge)
		if _, err := tx.ExecContext(ctx, `INSERT INTO upstream_groups(host,group_id,name,description,platform,status,raw_rate,effective_rate,rate_source,updated_at)
			VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(host,group_id) DO UPDATE SET name=excluded.name,description=excluded.description,
			platform=excluded.platform,status=excluded.status,raw_rate=excluded.raw_rate,effective_rate=excluded.effective_rate,
			rate_source=excluded.rate_source,updated_at=excluded.updated_at`, host, item.GroupID, item.Name, item.Description,
			item.Platform, item.Status, item.RawRate, effective, "live-catalog-mapped", now); err != nil {
			return err
		}
	}
	keyIDs := map[string]struct{}{}
	for _, item := range keys {
		keyIDs[item.KeyID] = struct{}{}
		if _, err := tx.ExecContext(ctx, `INSERT INTO upstream_keys(host,key_id,name,upstream_group,rate,status,metadata_json,updated_at)
			VALUES(?,?,?,?,?,?,'{}',?) ON CONFLICT(host,key_id) DO UPDATE SET name=excluded.name,upstream_group=excluded.upstream_group,
			rate=excluded.rate,status=excluded.status,metadata_json='{}',updated_at=excluded.updated_at`, host, item.KeyID, item.Name,
			item.UpstreamGroup, item.Rate, item.Status, now); err != nil {
			return err
		}
	}
	if partial {
		return nil
	}
	if err := reconcileCatalogRows(ctx, tx, "upstream_groups", "group_id", host, groupIDs, boundGroups, now); err != nil {
		return err
	}
	return reconcileCatalogRows(ctx, tx, "upstream_keys", "key_id", host, keyIDs, boundKeys, now)
}

func stringSetFromQueryer(ctx context.Context, queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, query string, arguments ...any) (map[string]struct{}, error) {
	rows, err := queryer.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]struct{}{}
	for rows.Next() {
		var value sql.NullString
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		if value.Valid && strings.TrimSpace(value.String) != "" {
			result[value.String] = struct{}{}
		}
	}
	return result, rows.Err()
}

func reconcileCatalogRows(ctx context.Context, tx *sql.Tx, table, idColumn, host string, live map[string]struct{}, bound map[string]struct{}, now string) error {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf("SELECT %s FROM %s WHERE host=?", idColumn, table), host)
	if err != nil {
		return err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, id := range ids {
		if _, present := live[id]; present {
			continue
		}
		if _, retained := bound[id]; retained {
			if table == "upstream_groups" {
				_, err = tx.ExecContext(ctx, `UPDATE upstream_groups SET status='missing',rate_source='catalog-missing',updated_at=? WHERE host=? AND group_id=?`, now, host, id)
			} else {
				_, err = tx.ExecContext(ctx, `UPDATE upstream_keys SET status='missing',metadata_json='{}',updated_at=? WHERE host=? AND key_id=?`, now, host, id)
			}
		} else {
			_, err = tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE host=? AND %s=?", table, idColumn), host, id)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func setOptionalMetadata(metadata map[string]any, key string, value *string) {
	if value != nil && strings.TrimSpace(*value) != "" {
		metadata[key] = *value
	}
}

func redactedFailureReason(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 500 {
		value = value[:500]
	}
	return value
}
