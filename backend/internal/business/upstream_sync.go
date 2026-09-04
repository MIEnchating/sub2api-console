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
	DisplayBalance    *string
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
	AuthMethod       string
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
	DisplayBalance       *string `json:"display_balance"`
	BalanceUnit          *string `json:"balance_unit"`
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
	if _, err := s.upstreamIdentityID(ctx, host); err != nil {
		return UpstreamSyncWriteResult{}, err
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
	rechargeRaw, rechargeSourceHost, rateErr := stableRechargeRate(ctx, tx, host)
	if rateErr != nil {
		if errors.Is(rateErr, sql.ErrNoRows) {
			rechargeRaw = "1"
		} else {
			return UpstreamSyncWriteResult{}, rateErr
		}
	}
	if rechargeSourceHost != host {
		note := "stable-upstream-inherited"
		if rechargeSourceHost == "" {
			note = "console-sync-default"
		}
		if _, insertErr := tx.ExecContext(ctx, `INSERT INTO recharge_rates(host,recharge_rate,note,updated_at)
			VALUES(?,?,?,?) ON CONFLICT(host) DO NOTHING`, host, rechargeRaw, note, now); insertErr != nil {
			return UpstreamSyncWriteResult{}, insertErr
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
	result.DisplayBalance = normalizeDecimal(stringValue(metadata["display_balance"]))
	if result.DisplayBalance != nil && *result.DisplayBalance == "" {
		result.DisplayBalance = nil
	}
	if unit := strings.TrimSpace(stringValue(metadata["balance_unit"])); unit != "" {
		result.BalanceUnit = &unit
	}
	if value.NameOnly {
		setOptionalMetadata(metadata, "site_name", value.Balance.SiteName)
		metadata["name_status"], metadata["name_error"], metadata["name_checked_at"] = "已读取", nil, now
		balance = nil
	}

	if value.Catalog != nil {
		hasCatalogBaseline := strings.TrimSpace(stringValue(metadata["catalog_checked_at"])) != ""
		result.AccountTotal, result.AccountRateSucceeded, result.AccountRateFailed, err = catalogAccountRateCounts(
			ctx, tx, host, keys, partialCatalog,
		)
		if err != nil {
			return UpstreamSyncWriteResult{}, err
		}
		if err := persistCatalogTx(ctx, tx, host, groups, keys, recharge, now, partialCatalog, hasCatalogBaseline); err != nil {
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
		if !partialCatalog {
			metadata["rate_sync_status"] = "succeeded"
			metadata["rate_sync_error"] = nil
			metadata["rate_sync_at"] = now
		}
	}
	if balance != nil {
		result.RawBalance = balance.RawBalance
		result.Balance = divideDecimalPointers(balance.RawBalance, recharge)
		result.DisplayBalance = balance.DisplayBalance
		result.BalanceUnit = balance.BalanceUnit
		result.BalanceStatus = balance.Status
		metadata["balance_status"] = balance.Status
		metadata["balance_error"] = nil
		metadata["balance_checked_at"] = now
		setOptionalMetadata(metadata, "site_name", balance.SiteName)
		setOptionalMetadata(metadata, "quota_per_unit", balance.QuotaPerUnit)
		setOptionalMetadata(metadata, "display_balance", balance.DisplayBalance)
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
		if method := strings.TrimSpace(value.AuthMethod); method != "" {
			metadata["last_auth_success_method"] = method
			metadata["last_auth_success_at"] = now
		}
		if value.AuthRecovered {
			metadata["auth_recovered_at"] = now
			metadata["last_auth_recovery_method"] = "refresh_token"
		}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return UpstreamSyncWriteResult{}, err
	}
	authStatus := existingAuthStatus
	if value.AuthenticationOK {
		authStatus = UpstreamAuthStatusAuthenticated
		if value.AuthRecovered {
			authStatus = UpstreamAuthStatusRecovered
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
	upstreamID, _, err := upstreamIdentityHostsForQueryer(ctx, tx, host)
	if err != nil {
		return 0, 0, 0, err
	}
	if err := ensureBindingIdentitiesTx(ctx, tx, upstreamID); err != nil {
		return 0, 0, 0, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT bi.upstream_key_id FROM binding_identities bi
		JOIN bindings b ON b.id=bi.binding_id
		LEFT JOIN manual_priority_accounts m ON m.account_id=b.local_account_id
		WHERE bi.upstream_id=? AND m.account_id IS NULL`, upstreamID)
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
	case "key":
		// A single-Key refresh is not evidence about the Host-wide catalog.
	case "key_balance":
		metadata["balance_status"], metadata["balance_error"], metadata["balance_checked_at"] = "读取失败", reason, now
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
	authStatus := existingAuthStatus
	if authenticationFailure {
		authStatus = UpstreamAuthStatusInvalid
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
	if result.DisplayBalance != nil {
		result.DisplayBalance = normalizeDecimal(*result.DisplayBalance)
		if result.DisplayBalance == nil || *result.DisplayBalance == "" {
			return nil, errors.New("上游显示余额必须是有限数值")
		}
	}
	if result.BalanceUnit != nil {
		unit := strings.ToLower(strings.TrimSpace(*result.BalanceUnit))
		if unit == "" {
			result.BalanceUnit = nil
		} else {
			result.BalanceUnit = &unit
		}
	}
	return &result, nil
}

func persistCatalogTx(ctx context.Context, tx *sql.Tx, host string, groups []UpstreamCatalogGroup, keys []UpstreamCatalogKey, recharge *string, now string, partial, hasCatalogBaseline bool) error {
	upstreamID, identityHosts, err := upstreamIdentityHostsForQueryer(ctx, tx, host)
	if err != nil {
		return err
	}
	if err := ensureBindingIdentitiesTx(ctx, tx, upstreamID); err != nil {
		return err
	}
	if err := ensureCatalogEntitiesFromRowsTx(ctx, tx, upstreamID); err != nil {
		return err
	}
	if err := repairBindingCatalogReferences(ctx, tx, identityHosts, groups, keys, now); err != nil {
		return err
	}
	if err := ensureBindingIdentitiesTx(ctx, tx, upstreamID); err != nil {
		return err
	}
	if err := ensureCatalogEntitiesFromBindingsTx(ctx, tx, upstreamID, now); err != nil {
		return err
	}
	groupIDs := map[string]struct{}{}
	for _, item := range groups {
		groupIDs[item.GroupID] = struct{}{}
		effective := divideMultiplierPointers(item.RawRate, recharge)
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
	if err := upsertLiveCatalogEntitiesTx(ctx, tx, upstreamID, groups, keys, now, hasCatalogBaseline); err != nil {
		return err
	}
	if partial {
		return nil
	}
	if err := reconcileCatalogEntitiesTx(ctx, tx, upstreamID, "group", groupIDs, now); err != nil {
		return err
	}
	return reconcileCatalogEntitiesTx(ctx, tx, upstreamID, "key", keyIDs, now)
}

func repairBindingCatalogReferences(ctx context.Context, tx *sql.Tx, hosts []string, groups []UpstreamCatalogGroup, keys []UpstreamCatalogKey, now string) error {
	byID := make(map[string]UpstreamCatalogGroup, len(groups))
	byName := make(map[string]UpstreamCatalogGroup, len(groups))
	ambiguousNames := map[string]struct{}{}
	for _, group := range groups {
		byID[group.GroupID] = group
		if previous, exists := byName[group.Name]; exists && previous.GroupID != group.GroupID {
			delete(byName, group.Name)
			ambiguousNames[group.Name] = struct{}{}
			continue
		}
		if _, ambiguous := ambiguousNames[group.Name]; !ambiguous {
			byName[group.Name] = group
		}
	}
	for _, key := range keys {
		if key.UpstreamGroup == nil {
			continue
		}
		reference := strings.TrimSpace(*key.UpstreamGroup)
		if reference == "" {
			continue
		}
		group, found := byID[reference]
		if !found {
			group, found = byName[reference]
		}
		if !found {
			continue
		}
		placeholders, hostArguments := sqlStringArguments(hosts)
		arguments := []any{key.Name, group.Name, group.GroupID, now}
		arguments = append(arguments, hostArguments...)
		arguments = append(arguments, key.KeyID, key.Name, group.Name, group.GroupID)
		if _, err := tx.ExecContext(ctx, `UPDATE bindings SET upstream_key_name=?,upstream_group=?,upstream_group_id=?,updated_at=?
			WHERE upstream_host IN (`+placeholders+`) AND upstream_key_id=? AND (
				upstream_key_name<>? OR COALESCE(upstream_group,'')<>? OR COALESCE(upstream_group_id,'')<>?
			) AND NOT EXISTS(SELECT 1 FROM manual_priority_accounts m WHERE m.account_id=bindings.local_account_id)`, arguments...); err != nil {
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
