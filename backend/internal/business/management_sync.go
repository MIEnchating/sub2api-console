package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
)

type ManagementSyncResult struct {
	Accounts    int   `json:"accounts"`
	GroupLinks  int   `json:"group_links"`
	Groups      int   `json:"groups"`
	EventID     int64 `json:"event_id"`
	RemoteWrite bool  `json:"remote_write"`
	ReadOnly    bool  `json:"read_only"`
}

type AccountBaseURLObservation struct {
	AccountID string
	BaseURL   *string
	Source    string
}

func (s *Store) CommitAccountBaseURLObservations(ctx context.Context, values []AccountBaseURLObservation) error {
	if len(values) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	seen := map[string]struct{}{}
	for _, value := range values {
		accountID := strings.TrimSpace(value.AccountID)
		if accountID == "" {
			return errors.New("Base URL 观测缺少账号 ID")
		}
		if _, duplicate := seen[accountID]; duplicate {
			return fmt.Errorf("Base URL 观测包含重复账号 ID：%s", accountID)
		}
		seen[accountID] = struct{}{}
		var metadataRaw string
		if err := tx.QueryRowContext(ctx, `SELECT metadata_json FROM accounts WHERE id=?`, accountID).Scan(&metadataRaw); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("Base URL 观测账号 %s 不存在", accountID)
			}
			return err
		}
		metadata, err := decodeObject(metadataRaw)
		if err != nil {
			return fmt.Errorf("账号 %s 元数据记录损坏", accountID)
		}
		if value.BaseURL == nil || strings.TrimSpace(*value.BaseURL) == "" {
			delete(metadata, "base_url")
			delete(metadata, "base_url_source")
		} else {
			metadata["base_url"] = strings.TrimSpace(*value.BaseURL)
			source := strings.TrimSpace(value.Source)
			if source == "" {
				source = "explicit"
			}
			metadata["base_url_source"] = source
		}
		metadata["base_url_checked_at"] = now
		encoded, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE accounts SET metadata_json=?,updated_at=? WHERE id=?`, string(encoded), now, accountID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type managementAccount struct {
	ID            string
	Name          string
	UpstreamHost  *string
	UpstreamType  *string
	Schedulable   *bool
	Priority      *int64
	LoadFactor    *string
	Concurrency   *int64
	Multiplier    *string
	Balance       *string
	RoutingState  *string
	HealthStatus  *string
	Metadata      map[string]any
	GroupsPresent bool
	Groups        []managementMembership
}

type managementMembership struct {
	Name string
	ID   *string
}

func (s *Store) SyncManagementSnapshot(
	ctx context.Context,
	accountRows []map[string]any,
	groupRows []map[string]any,
	actor string,
) (ManagementSyncResult, error) {
	groupsByID, groupsByName, err := managementGroupCatalog(groupRows)
	if err != nil {
		return ManagementSyncResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ManagementSyncResult{}, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	globalStrategy, err := managementGlobalStrategy(ctx, s, tx)
	if err != nil {
		return ManagementSyncResult{}, err
	}
	for _, row := range groupRows {
		groupID, _ := managementStableID(row["id"])
		if groupID == "" {
			groupID, _ = managementStableID(row["group_id"])
		}
		name := groupsByID[groupID]
		rate, present, err := managementOptionalDecimal(row, "rate_multiplier", "multiplier", "rate")
		if err != nil {
			return ManagementSyncResult{}, fmt.Errorf("分组 %s 的倍率无效：%w", name, err)
		}
		var existingRate, existingPlatform sql.NullString
		err = tx.QueryRowContext(ctx, `SELECT rate_multiplier,platform FROM local_groups WHERE name=?`, name).Scan(&existingRate, &existingPlatform)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return ManagementSyncResult{}, err
		}
		if !present && existingRate.Valid {
			rate = stringPointer(existingRate.String)
		}
		platform, err := managementOptionalText(row, existingPlatform, "platform")
		if err != nil {
			return ManagementSyncResult{}, fmt.Errorf("分组 %s 的平台无效：%w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO local_groups(
			name,remote_id,strategy,strategy_source,platform,rate_multiplier,updated_at
		) VALUES(?,?,?,?,?,?,?) ON CONFLICT(name) DO UPDATE SET
			remote_id=excluded.remote_id,platform=excluded.platform,rate_multiplier=excluded.rate_multiplier,updated_at=excluded.updated_at`,
			name, groupID, globalStrategy, "global_default", managementNullableString(platform), managementNullableString(rate), now,
		); err != nil {
			return ManagementSyncResult{}, err
		}
	}
	seenAccounts := map[string]struct{}{}
	allGroupNames := map[string]struct{}{}
	for name := range groupsByName {
		allGroupNames[name] = struct{}{}
	}
	accountCount, groupLinks := 0, 0
	for _, row := range accountRows {
		account, err := managementAccountProjection(ctx, tx, row, groupsByID, groupsByName)
		if err != nil {
			return ManagementSyncResult{}, err
		}
		if _, duplicate := seenAccounts[account.ID]; duplicate {
			return ManagementSyncResult{}, fmt.Errorf("管理快照返回重复账号 ID：%s", account.ID)
		}
		seenAccounts[account.ID] = struct{}{}
		metadata, err := json.Marshal(account.Metadata)
		if err != nil {
			return ManagementSyncResult{}, fmt.Errorf("账号 %s 元数据无法序列化：%w", account.ID, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO accounts(
			id,name,upstream_host,upstream_type,schedulable,priority,load_factor,concurrency,multiplier,balance,
			paused,paused_reason,routing_state,routing_tier,health_status,failure_streak,recovery_pass_streak,metadata_json,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,0,NULL,?,NULL,?,NULL,NULL,?,?) ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,upstream_host=excluded.upstream_host,upstream_type=excluded.upstream_type,
			schedulable=excluded.schedulable,priority=excluded.priority,load_factor=excluded.load_factor,
			concurrency=excluded.concurrency,multiplier=excluded.multiplier,balance=excluded.balance,
			routing_state=excluded.routing_state,health_status=excluded.health_status,
			metadata_json=excluded.metadata_json,updated_at=excluded.updated_at`,
			account.ID, account.Name, managementNullableString(account.UpstreamHost), managementNullableString(account.UpstreamType),
			managementNullableBool(account.Schedulable), managementNullableInt(account.Priority), managementNullableString(account.LoadFactor),
			managementNullableInt(account.Concurrency), managementNullableString(account.Multiplier), managementNullableString(account.Balance),
			managementNullableString(account.RoutingState), managementNullableString(account.HealthStatus), string(metadata), now,
		); err != nil {
			return ManagementSyncResult{}, err
		}
		if account.GroupsPresent {
			if _, err := tx.ExecContext(ctx, `DELETE FROM account_groups WHERE account_id=?`, account.ID); err != nil {
				return ManagementSyncResult{}, err
			}
			for _, membership := range account.Groups {
				if _, err := tx.ExecContext(ctx, `INSERT INTO account_groups(account_id,group_name,group_id,group_rate)
					VALUES(?,?,?,?)`, account.ID, membership.Name, managementNullableString(membership.ID), managementNullableString(account.Multiplier)); err != nil {
					return ManagementSyncResult{}, err
				}
				if _, found := groupsByName[membership.Name]; !found {
					if _, err := tx.ExecContext(ctx, `INSERT INTO local_groups(name,remote_id,strategy,strategy_source,updated_at)
						VALUES(?,?,?,?,?) ON CONFLICT(name) DO UPDATE SET
						remote_id=COALESCE(local_groups.remote_id,excluded.remote_id),updated_at=excluded.updated_at`,
						membership.Name, managementNullableString(membership.ID), globalStrategy, "global_default", now); err != nil {
						return ManagementSyncResult{}, err
					}
				}
				allGroupNames[membership.Name] = struct{}{}
				groupLinks++
			}
		}
		accountCount++
	}
	if _, err := tx.ExecContext(ctx, `UPDATE local_groups SET account_count=(
		SELECT COUNT(*) FROM account_groups WHERE group_name=local_groups.name
	),updated_at=?`, now); err != nil {
		return ManagementSyncResult{}, err
	}
	payload := map[string]any{
		"actor": strings.TrimSpace(actor), "accounts": accountCount, "groups": len(allGroupNames),
		"group_links": groupLinks, "remote_write": false,
	}
	if err := insertRuntimeEventWithStatus(ctx, tx, "management.snapshot.synced", "succeeded",
		fmt.Sprintf("管理快照同步完成：账号 %d，分组 %d", accountCount, len(allGroupNames)), payload, now); err != nil {
		return ManagementSyncResult{}, err
	}
	var eventID int64
	if err := tx.QueryRowContext(ctx, `SELECT MIN(source_id) FROM runtime_events WHERE source_id < 0`).Scan(&eventID); err != nil {
		return ManagementSyncResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ManagementSyncResult{}, err
	}
	return ManagementSyncResult{
		Accounts: accountCount, GroupLinks: groupLinks, Groups: len(allGroupNames), EventID: eventID,
		RemoteWrite: false, ReadOnly: true,
	}, nil
}

func managementGroupCatalog(rows []map[string]any) (map[string]string, map[string]string, error) {
	byID, byName := map[string]string{}, map[string]string{}
	for _, row := range rows {
		rawID, present := managementPresent(row, "id", "group_id")
		if !present {
			return nil, nil, errors.New("管理快照中的分组缺少稳定 ID")
		}
		id, err := managementStableID(rawID)
		if err != nil {
			return nil, nil, errors.New("管理快照中的分组缺少稳定 ID")
		}
		rawName, present := managementPresent(row, "name", "group")
		name, valid := rawName.(string)
		name = strings.TrimSpace(name)
		if !present || !valid || name == "" {
			return nil, nil, fmt.Errorf("管理快照中的分组 %s 缺少名称", id)
		}
		if previous, found := byID[id]; found && previous != name {
			return nil, nil, fmt.Errorf("管理快照返回重复分组 ID：%s", id)
		}
		if previous, found := byName[name]; found && previous != id {
			return nil, nil, fmt.Errorf("管理快照中的分组名称 %s 对应多个稳定 ID", name)
		}
		byID[id], byName[name] = name, id
	}
	return byID, byName, nil
}

func managementAccountProjection(
	ctx context.Context,
	tx *sql.Tx,
	row map[string]any,
	groupsByID map[string]string,
	groupsByName map[string]string,
) (managementAccount, error) {
	rawID, present := managementPresent(row, "id", "account_id")
	if !present {
		return managementAccount{}, errors.New("管理快照中的账号缺少稳定 ID")
	}
	id, err := managementStableID(rawID)
	if err != nil {
		return managementAccount{}, errors.New("管理快照中的账号缺少稳定 ID")
	}
	var existing struct {
		name, metadata                                                                          string
		upstreamHost, upstreamType, loadFactor, multiplier, balance, routingState, healthStatus sql.NullString
		schedulable                                                                             sql.NullBool
		priority, concurrency                                                                   sql.NullInt64
	}
	err = tx.QueryRowContext(ctx, `SELECT name,upstream_host,upstream_type,schedulable,priority,load_factor,
		concurrency,multiplier,balance,routing_state,health_status,metadata_json FROM accounts WHERE id=?`, id).Scan(
		&existing.name, &existing.upstreamHost, &existing.upstreamType, &existing.schedulable, &existing.priority,
		&existing.loadFactor, &existing.concurrency, &existing.multiplier, &existing.balance,
		&existing.routingState, &existing.healthStatus, &existing.metadata,
	)
	exists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return managementAccount{}, err
	}
	rawName, namePresent := managementPresent(row, "name", "username")
	name := existing.name
	if namePresent {
		var valid bool
		name, valid = rawName.(string)
		name = strings.TrimSpace(name)
		if !valid || name == "" {
			return managementAccount{}, fmt.Errorf("管理快照中的账号 %s 名称为空", id)
		}
	} else if !exists {
		return managementAccount{}, fmt.Errorf("管理快照中的账号 %s 缺少名称", id)
	}
	metadata := map[string]any{}
	if exists {
		metadata, err = decodeJSONObject(existing.metadata)
		if err != nil {
			return managementAccount{}, fmt.Errorf("账号 %s 元数据损坏", id)
		}
	}
	delete(metadata, "error_message")
	for _, key := range []string{
		"platform", "type", "status", "credentials_status", "error_message", "group_ids", "quota_limit", "quota_used",
		"quota_daily_limit", "quota_daily_used", "quota_weekly_limit", "quota_weekly_used", "expires_at",
		"auto_pause_on_expired", "rate_limited_at", "rate_limit_reset_at", "temp_unschedulable_until",
		"temp_unschedulable_reason", "overload_until", "last_used_at",
	} {
		if value, found := row[key]; found {
			metadata[key] = value
		}
	}
	baseURL, baseURLPresent, err := managementAccountBaseURL(row)
	if err != nil {
		return managementAccount{}, fmt.Errorf("账号 %s 的 base_url 无效：%w", id, err)
	}
	if baseURLPresent {
		if baseURL == nil {
			delete(metadata, "base_url")
			delete(metadata, "base_url_source")
		} else {
			metadata["base_url"] = *baseURL
			metadata["base_url_source"] = "explicit"
		}
	}
	groups, groupsPresent, err := managementMemberships(row, groupsByID, groupsByName)
	if err != nil {
		return managementAccount{}, fmt.Errorf("账号 %s：%w", id, err)
	}
	upstreamHost, err := managementOptionalText(row, existing.upstreamHost, "upstream_host")
	if err != nil {
		return managementAccount{}, fmt.Errorf("账号 %s 的 upstream_host 无效", id)
	}
	upstreamType, err := managementOptionalText(row, existing.upstreamType, "upstream_type", "type")
	if err != nil {
		return managementAccount{}, fmt.Errorf("账号 %s 的 upstream_type 无效", id)
	}
	schedulable, err := managementOptionalBool(row, existing.schedulable, "schedulable")
	if err != nil {
		return managementAccount{}, fmt.Errorf("账号 %s 的 schedulable 无效", id)
	}
	priority, err := managementOptionalInteger(row, existing.priority, "priority")
	if err != nil {
		return managementAccount{}, fmt.Errorf("账号 %s 的 priority 无效", id)
	}
	loadFactor, err := managementOptionalDecimalExisting(row, existing.loadFactor, "load_factor")
	if err != nil {
		return managementAccount{}, fmt.Errorf("账号 %s 的 load_factor 无效", id)
	}
	concurrency, err := managementOptionalInteger(row, existing.concurrency, "concurrency")
	if err != nil {
		return managementAccount{}, fmt.Errorf("账号 %s 的 concurrency 无效", id)
	}
	multiplier, err := managementOptionalDecimalExisting(row, existing.multiplier, "rate_multiplier", "multiplier")
	if err != nil {
		return managementAccount{}, fmt.Errorf("账号 %s 的 multiplier 无效", id)
	}
	if exists {
		var bound bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM bindings
			WHERE local_account_id=? AND COALESCE(status,'')<>'missing')`, id).Scan(&bound); err != nil {
			return managementAccount{}, err
		}
		if bound {
			// A bound account's rate-derived name and converted cost are owned by
			// upstream rate sync, not by a potentially stale management snapshot.
			name = existing.name
			multiplier = nullString(existing.multiplier)
		}
	}
	balance, err := managementOptionalDecimalExisting(row, existing.balance, "balance")
	if err != nil {
		return managementAccount{}, fmt.Errorf("账号 %s 的 balance 无效", id)
	}
	routingState, err := managementOptionalText(row, existing.routingState, "routing_state")
	if err != nil {
		return managementAccount{}, fmt.Errorf("账号 %s 的 routing_state 无效", id)
	}
	healthStatus, err := managementOptionalText(row, existing.healthStatus, "test_result")
	if err != nil {
		return managementAccount{}, fmt.Errorf("账号 %s 的 test_result 无效", id)
	}
	return managementAccount{
		ID: id, Name: name,
		UpstreamHost: upstreamHost, UpstreamType: upstreamType, Schedulable: schedulable, Priority: priority,
		LoadFactor: loadFactor, Concurrency: concurrency, Multiplier: multiplier, Balance: balance,
		RoutingState: routingState, HealthStatus: healthStatus,
		Metadata: metadata, GroupsPresent: groupsPresent, Groups: groups,
	}, nil
}

func managementAccountBaseURL(row map[string]any) (*string, bool, error) {
	raw, present := row["base_url"]
	if !present {
		credentialsRaw, credentialsPresent := row["credentials"]
		if !credentialsPresent {
			return nil, false, nil
		}
		if credentialsRaw == nil {
			return nil, false, nil
		}
		credentials, ok := credentialsRaw.(map[string]any)
		if !ok {
			return nil, false, errors.New("credentials 必须是对象")
		}
		raw, present = credentials["base_url"]
		if !present {
			return nil, false, nil
		}
	}
	if raw == nil {
		return nil, true, nil
	}
	value, ok := raw.(string)
	if !ok {
		return nil, false, errors.New("必须是字符串")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, true, nil
	}
	return &value, true, nil
}

func managementMemberships(row map[string]any, byID, byName map[string]string) ([]managementMembership, bool, error) {
	raw, present := managementPresent(row, "groups", "account_groups", "group_ids")
	if !present {
		return nil, false, nil
	}
	if raw == nil {
		return nil, true, errors.New("分组列表不能是 null")
	}
	values, ok := raw.([]any)
	if !ok {
		values = []any{raw}
	}
	result := make([]managementMembership, 0, len(values))
	seen := map[string]*string{}
	for _, value := range values {
		var id *string
		var name string
		switch item := value.(type) {
		case map[string]any:
			rawID, idPresent := managementPresent(item, "id", "group_id")
			rawName, namePresent := managementPresent(item, "name", "group")
			if idPresent && rawID != nil {
				parsed, err := managementStableID(rawID)
				if err != nil {
					return nil, true, errors.New("分组列表包含无效稳定 ID")
				}
				id = stringPointer(parsed)
			}
			if namePresent && rawName != nil {
				parsed, valid := rawName.(string)
				if !valid {
					return nil, true, errors.New("分组列表包含无效名称")
				}
				name = strings.TrimSpace(parsed)
			}
			if name == "" && id != nil {
				name = byID[*id]
			}
		case string, json.Number, int, int64:
			text := strings.TrimSpace(fmt.Sprint(item))
			if parsed, err := managementStableID(item); err == nil {
				id = stringPointer(parsed)
				name = byID[parsed]
			}
			if name == "" {
				name = text
				if catalogID, found := byName[name]; found {
					id = stringPointer(catalogID)
				}
			}
		default:
			return nil, true, errors.New("分组列表包含无效项目")
		}
		if name == "" {
			return nil, true, errors.New("分组列表包含无法识别的项目")
		}
		if previous, found := seen[name]; found {
			if !managementEqualStrings(previous, id) {
				return nil, true, fmt.Errorf("分组 %s 返回冲突稳定 ID", name)
			}
			continue
		}
		seen[name] = id
		result = append(result, managementMembership{Name: name, ID: id})
	}
	return result, true, nil
}

func managementGlobalStrategy(ctx context.Context, store *Store, tx *sql.Tx) (string, error) {
	document, err := store.readPolicyDocument(ctx, tx, "control-plane")
	if err != nil {
		return "", err
	}
	value := any("balanced")
	if document != nil {
		if selection, ok := document["selection"].(map[string]any); ok {
			if current, present := selection["strategy"]; present {
				value = current
			}
		} else if current, present := document["strategy"]; present {
			value = current
		}
	}
	return normalizeStrategy(value)
}

func managementPresent(row map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if value, present := row[key]; present {
			return value, true
		}
	}
	return nil, false
}

func managementStableID(value any) (string, error) {
	if value == nil {
		return "", errors.New("stable ID missing")
	}
	var text string
	switch item := value.(type) {
	case json.Number:
		text = item.String()
	case string:
		text = item
	case int:
		text = strconv.Itoa(item)
	case int64:
		text = strconv.FormatInt(item, 10)
	default:
		return "", errors.New("stable ID invalid")
	}
	text = strings.TrimSpace(text)
	parsed, err := strconv.ParseUint(text, 10, 64)
	if err != nil || parsed == 0 || strconv.FormatUint(parsed, 10) != text {
		return "", errors.New("stable ID invalid")
	}
	return text, nil
}

func managementOptionalDecimal(row map[string]any, keys ...string) (*string, bool, error) {
	value, present := managementPresent(row, keys...)
	if !present {
		return nil, false, nil
	}
	parsed, err := managementDecimal(value)
	return parsed, true, err
}

func managementDecimal(value any) (*string, error) {
	if value == nil {
		return nil, nil
	}
	if _, boolean := value.(bool); boolean {
		return nil, errors.New("必须是有限数值")
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	parsed, ok := new(big.Rat).SetString(text)
	if !ok {
		return nil, errors.New("必须是有限数值")
	}
	return stringPointer(decimalRatText(parsed)), nil
}

func managementOptionalDecimalExisting(row map[string]any, existing sql.NullString, keys ...string) (*string, error) {
	value, present := managementPresent(row, keys...)
	if !present {
		if existing.Valid {
			return stringPointer(existing.String), nil
		}
		return nil, nil
	}
	return managementDecimal(value)
}

func managementOptionalText(row map[string]any, existing sql.NullString, keys ...string) (*string, error) {
	value, present := managementPresent(row, keys...)
	if !present {
		if existing.Valid {
			return stringPointer(existing.String), nil
		}
		return nil, nil
	}
	if value == nil {
		return nil, nil
	}
	text, ok := value.(string)
	if !ok {
		return nil, errors.New("必须是字符串或 null")
	}
	return stringPointer(strings.TrimSpace(text)), nil
}

func managementOptionalBool(row map[string]any, existing sql.NullBool, keys ...string) (*bool, error) {
	value, present := managementPresent(row, keys...)
	if !present {
		if existing.Valid {
			result := existing.Bool
			return &result, nil
		}
		return nil, nil
	}
	if value == nil {
		return nil, nil
	}
	if parsed := strictAnyBool(value); parsed != nil {
		return parsed, nil
	}
	if number, ok := value.(json.Number); ok && (number.String() == "0" || number.String() == "1") {
		result := number.String() == "1"
		return &result, nil
	}
	return nil, errors.New("必须是布尔值")
}

func managementOptionalInteger(row map[string]any, existing sql.NullInt64, keys ...string) (*int64, error) {
	value, present := managementPresent(row, keys...)
	if !present {
		if existing.Valid {
			result := existing.Int64
			return &result, nil
		}
		return nil, nil
	}
	if value == nil {
		return nil, nil
	}
	var text string
	switch item := value.(type) {
	case json.Number:
		text = item.String()
	case string:
		text = strings.TrimSpace(item)
	case int:
		text = strconv.Itoa(item)
	case int64:
		text = strconv.FormatInt(item, 10)
	default:
		return nil, errors.New("必须是整数")
	}
	parsed, err := strconv.ParseInt(text, 10, 64)
	if err != nil || strconv.FormatInt(parsed, 10) != text {
		return nil, errors.New("必须是整数")
	}
	return &parsed, nil
}

func managementNullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func managementNullableInt(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func managementNullableBool(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}

func managementEqualStrings(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
