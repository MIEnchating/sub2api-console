package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ManagementSyncResult struct {
	Accounts      int   `json:"accounts"`
	GroupLinks    int   `json:"group_links"`
	Groups        int   `json:"groups"`
	DeletedGroups int   `json:"deleted_groups"`
	EventID       int64 `json:"event_id"`
	RemoteWrite   bool  `json:"remote_write"`
	ReadOnly      bool  `json:"read_only"`
}

func (s *Store) ManagementAccountIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM accounts ORDER BY
		CASE WHEN id GLOB '[0-9]*' THEN CAST(id AS INTEGER) ELSE 0 END,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var accountID string
		if err := rows.Scan(&accountID); err != nil {
			return nil, err
		}
		accountID = strings.TrimSpace(accountID)
		if accountID == "" {
			return nil, errors.New("本地账号记录缺少稳定 ID")
		}
		result = append(result, accountID)
	}
	return result, rows.Err()
}

func ManagementSnapshotAccountIDs(rows []map[string]any) ([]string, error) {
	seen := make(map[string]struct{}, len(rows))
	result := make([]string, 0, len(rows))
	for _, row := range rows {
		rawID, present := managementPresent(row, "id", "account_id")
		if !present {
			return nil, errors.New("管理快照中的账号缺少稳定 ID")
		}
		id, err := managementStableID(rawID)
		if err != nil {
			return nil, errors.New("管理快照中的账号缺少稳定 ID")
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, fmt.Errorf("管理快照返回重复账号 ID：%s", id)
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Strings(result)
	return result, nil
}

type ManagementSnapshotGroupIdentity struct {
	ID   string
	Name string
}

func ManagementSnapshotGroupIdentities(rows []map[string]any) ([]ManagementSnapshotGroupIdentity, error) {
	groupsByID, _, err := managementGroupCatalog(rows)
	if err != nil {
		return nil, err
	}
	groupIDs := make([]string, 0, len(groupsByID))
	for groupID := range groupsByID {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Strings(groupIDs)
	result := make([]ManagementSnapshotGroupIdentity, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		result = append(result, ManagementSnapshotGroupIdentity{ID: groupID, Name: groupsByID[groupID]})
	}
	return result, nil
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
	separatedDeletedIDs := map[string]struct{}{}
	if err := stageManagementGroupIdentityConflicts(ctx, tx, groupsByID, groupsByName, separatedDeletedIDs, now); err != nil {
		return ManagementSyncResult{}, fmt.Errorf("分组名称冲突预处理失败：%w", err)
	}
	groupIDs := make([]string, 0, len(groupsByID))
	for groupID := range groupsByID {
		groupIDs = append(groupIDs, groupID)
	}
	sort.Strings(groupIDs)
	for _, groupID := range groupIDs {
		name := groupsByID[groupID]
		if err := reconcileManagementGroupIdentity(ctx, tx, groupID, name, now); err != nil {
			return ManagementSyncResult{}, fmt.Errorf("分组 %s 改名同步失败：%w", groupID, err)
		}
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
		if platform != nil {
			normalizedPlatform := strings.ToLower(strings.TrimSpace(*platform))
			platform = stringPointer(normalizedPlatform)
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
			changed, err := managementAccountGroupsChanged(ctx, tx, account.ID, account.Groups)
			if err != nil {
				return ManagementSyncResult{}, err
			}
			if changed {
				for _, statement := range []string{
					`DELETE FROM routing_decisions WHERE account_id=?`,
					`DELETE FROM account_health_evaluations WHERE account_id=?`,
				} {
					if _, err := tx.ExecContext(ctx, statement, account.ID); err != nil {
						return ManagementSyncResult{}, err
					}
				}
			}
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
	deletedGroups, err := pruneManagementDeletedGroups(ctx, s, tx, groupsByID, separatedDeletedIDs, now)
	if err != nil {
		return ManagementSyncResult{}, fmt.Errorf("已删除分组清理失败：%w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE local_groups SET account_count=(
		SELECT COUNT(*) FROM account_groups WHERE group_name=local_groups.name
	),updated_at=?`, now); err != nil {
		return ManagementSyncResult{}, err
	}
	payload := map[string]any{
		"actor": strings.TrimSpace(actor), "accounts": accountCount, "groups": len(allGroupNames),
		"group_links": groupLinks, "deleted_groups": deletedGroups, "remote_write": false,
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
		Accounts: accountCount, GroupLinks: groupLinks, Groups: len(allGroupNames), DeletedGroups: deletedGroups, EventID: eventID,
		RemoteWrite: false, ReadOnly: true,
	}, nil
}

func managementAccountGroupsChanged(
	ctx context.Context,
	tx *sql.Tx,
	accountID string,
	desired []managementMembership,
) (bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT group_name,COALESCE(group_id,'') FROM account_groups
		WHERE account_id=? ORDER BY group_name,COALESCE(group_id,'')`, accountID)
	if err != nil {
		return false, err
	}
	current := make([]string, 0)
	for rows.Next() {
		var name, id string
		if err := rows.Scan(&name, &id); err != nil {
			_ = rows.Close()
			return false, err
		}
		current = append(current, strings.TrimSpace(name)+"\x00"+strings.TrimSpace(id))
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return false, err
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	wanted := make([]string, 0, len(desired))
	for _, membership := range desired {
		id := ""
		if membership.ID != nil {
			id = strings.TrimSpace(*membership.ID)
		}
		wanted = append(wanted, strings.TrimSpace(membership.Name)+"\x00"+id)
	}
	sort.Strings(wanted)
	if len(current) != len(wanted) {
		return true, nil
	}
	for index := range current {
		if current[index] != wanted[index] {
			return true, nil
		}
	}
	return false, nil
}

type managementDeletedGroup struct {
	name string
	id   string
}

func pruneManagementDeletedGroups(
	ctx context.Context,
	store *Store,
	tx *sql.Tx,
	remoteGroups map[string]string,
	separatedDeletedIDs map[string]struct{},
	now string,
) (int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT lg.name,lg.remote_id FROM local_groups lg
		WHERE lg.remote_id IS NOT NULL AND TRIM(lg.remote_id)<>'' AND NOT EXISTS(
			SELECT 1 FROM account_groups ag WHERE ag.group_id=lg.remote_id OR EXISTS(
				SELECT 1 FROM local_groups sibling
				WHERE sibling.remote_id=lg.remote_id AND sibling.name=ag.group_name
			)
		) ORDER BY lg.remote_id,lg.name`)
	if err != nil {
		return 0, err
	}
	deleted := make([]managementDeletedGroup, 0)
	for rows.Next() {
		var value managementDeletedGroup
		if err := rows.Scan(&value.name, &value.id); err != nil {
			_ = rows.Close()
			return 0, err
		}
		value.name = strings.TrimSpace(value.name)
		value.id = strings.TrimSpace(value.id)
		if _, exists := remoteGroups[value.id]; !exists {
			deleted = append(deleted, value)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if len(deleted) == 0 {
		return 0, nil
	}
	deletedIDs := make(map[string]struct{}, len(deleted))
	for _, group := range deleted {
		projectionName := group.name
		if _, separated := separatedDeletedIDs[group.id]; !separated {
			// Historical and binding rows are name-keyed. Detach them before the
			// deleted name becomes available to a future stable identity.
			projectionName, err = archiveManagementDeletedGroupReferences(ctx, tx, group)
			if err != nil {
				return 0, err
			}
		}
		for _, statement := range []string{
			`DELETE FROM routing_decisions WHERE group_name=?`,
			`DELETE FROM account_health_evaluations WHERE group_name=?`,
		} {
			if _, err := tx.ExecContext(ctx, statement, projectionName); err != nil {
				return 0, err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM local_groups WHERE name=? AND remote_id=?`, group.name, group.id); err != nil {
			return 0, err
		}
		deletedIDs[group.id] = struct{}{}
	}
	control, err := store.readPolicyDocument(ctx, tx, "control-plane")
	if err != nil {
		return 0, err
	}
	if control != nil && removeDeletedGroupPolicyReferences(control, deletedIDs) {
		if err := store.writePolicyDocument(ctx, tx, "control-plane", control, now); err != nil {
			return 0, err
		}
	}
	return len(deletedIDs), nil
}

func archiveManagementDeletedGroupReferences(
	ctx context.Context,
	tx *sql.Tx,
	group managementDeletedGroup,
) (string, error) {
	base := managementDeletedGroupArchiveBase(group.name, group.id)
	archivedName := base
	for suffix := 2; ; suffix++ {
		occupied, err := managementGroupReferenceNameOccupied(ctx, tx, archivedName)
		if err != nil {
			return "", err
		}
		if !occupied {
			break
		}
		archivedName = fmt.Sprintf("%s (%d)", base, suffix)
	}
	if err := migrateManagementGroupReferences(ctx, tx, group.id, archivedName, []string{group.name}); err != nil {
		return "", fmt.Errorf("分组 %s 历史引用归档失败：%w", group.id, err)
	}
	return archivedName, nil
}

func managementGroupReferenceNameOccupied(ctx context.Context, tx *sql.Tx, name string) (bool, error) {
	var occupied bool
	err := tx.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM local_groups WHERE name=?) OR
		EXISTS(SELECT 1 FROM account_groups WHERE group_name=?) OR
		EXISTS(SELECT 1 FROM routing_decisions WHERE group_name=?) OR
		EXISTS(SELECT 1 FROM account_health_evaluations WHERE group_name=?) OR
		EXISTS(SELECT 1 FROM health_samples WHERE group_name=?) OR
		EXISTS(SELECT 1 FROM bindings WHERE local_group=?) OR
		EXISTS(SELECT 1 FROM onboarding_pending WHERE local_group_name=?) OR
		EXISTS(SELECT 1 FROM usage_records WHERE group_name=?)`,
		name, name, name, name, name, name, name, name,
	).Scan(&occupied)
	return occupied, err
}

func managementDeletedGroupArchiveBase(name, groupID string) string {
	return fmt.Sprintf("%s（已删除 #%s）", name, groupID)
}

func removeDeletedGroupPolicyReferences(control map[string]any, deletedIDs map[string]struct{}) bool {
	changed := false
	if bindings, ok := control["group_policy_bindings"].(map[string]any); ok {
		for groupID := range deletedIDs {
			if _, found := bindings[groupID]; found {
				delete(bindings, groupID)
				changed = true
			}
		}
	}
	if scope, ok := control["scope"].(map[string]any); ok {
		for _, field := range []string{"managed_group_ids", "excluded_group_ids"} {
			if filtered, removed := removeDeletedGroupIDs(scope[field], deletedIDs); removed {
				scope[field] = filtered
				changed = true
			}
		}
	}
	if pricing, ok := control["price_management"].(map[string]any); ok {
		if filtered, removed := removeDeletedGroupIDs(pricing["managed_group_ids"], deletedIDs); removed {
			pricing["managed_group_ids"] = filtered
			changed = true
		}
		if rawSets, ok := pricing["exchange_group_sets"].([]any); ok {
			sets := make([]any, 0, len(rawSets))
			rawNames, hasNames := pricing["exchange_group_set_names"].([]any)
			names := make([]any, 0, len(rawNames))
			removed := false
			for setIndex, rawSet := range rawSets {
				filtered, setRemoved := removeDeletedGroupIDs(rawSet, deletedIDs)
				removed = removed || setRemoved
				values, valid := filtered.([]any)
				if !valid || len(values) >= 2 {
					sets = append(sets, filtered)
					if hasNames && setIndex < len(rawNames) {
						names = append(names, rawNames[setIndex])
					}
				} else {
					removed = true
				}
			}
			if removed {
				pricing["exchange_group_sets"] = sets
				if hasNames {
					pricing["exchange_group_set_names"] = names
				}
				changed = true
			}
		}
	}
	return changed
}

func removeDeletedGroupIDs(value any, deletedIDs map[string]struct{}) (any, bool) {
	values, ok := value.([]any)
	if !ok {
		return value, false
	}
	filtered := make([]any, 0, len(values))
	removed := false
	for _, raw := range values {
		groupID, text := raw.(string)
		if text {
			_, deleted := deletedIDs[strings.TrimSpace(groupID)]
			if deleted {
				removed = true
				continue
			}
		}
		filtered = append(filtered, raw)
	}
	return filtered, removed
}

type localGroupIdentityRow struct {
	name               string
	remoteID           sql.NullString
	strategy           string
	strategySource     string
	platform           sql.NullString
	rateMultiplier     sql.NullString
	profitEnabled      sql.NullInt64
	profitMinMargin    sql.NullString
	profitSafetyBuffer sql.NullString
	accountCount       int64
}

type managementLocalGroupIdentity struct {
	name     string
	remoteID string
}

// Move conflicting identities out of the final namespace before applying renames.
// This makes swaps atomic and keeps historical references from a deleted ID distinct
// when a new ID reuses its former name.
func stageManagementGroupIdentityConflicts(
	ctx context.Context,
	tx *sql.Tx,
	remoteGroupsByID map[string]string,
	remoteGroupsByName map[string]string,
	separatedDeletedIDs map[string]struct{},
	now string,
) error {
	rows, err := tx.QueryContext(ctx, `SELECT name,remote_id FROM local_groups ORDER BY COALESCE(remote_id,''),name`)
	if err != nil {
		return err
	}
	localGroups := make([]managementLocalGroupIdentity, 0)
	occupiedNames := make(map[string]struct{}, len(remoteGroupsByName))
	for name := range remoteGroupsByName {
		occupiedNames[name] = struct{}{}
	}
	for rows.Next() {
		var group managementLocalGroupIdentity
		var remoteID sql.NullString
		if err := rows.Scan(&group.name, &remoteID); err != nil {
			_ = rows.Close()
			return err
		}
		group.name = strings.TrimSpace(group.name)
		occupiedNames[group.name] = struct{}{}
		if remoteID.Valid {
			group.remoteID = strings.TrimSpace(remoteID.String)
			if group.remoteID != "" {
				localGroups = append(localGroups, group)
			}
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	type stagedIdentity struct {
		archivedName  string
		preferredName string
		transient     bool
	}
	stagedByID := make(map[string]stagedIdentity)
	for _, group := range localGroups {
		desiredName, remains := remoteGroupsByID[group.remoteID]
		if remains {
			if group.name != desiredName {
				stagedByID[group.remoteID] = stagedIdentity{preferredName: desiredName, transient: true}
			}
			continue
		}
		if replacementID, reused := remoteGroupsByName[group.name]; reused && replacementID != group.remoteID {
			if _, found := stagedByID[group.remoteID]; !found {
				stagedByID[group.remoteID] = stagedIdentity{
					archivedName:  managementDeletedGroupArchiveBase(group.name, group.remoteID),
					preferredName: group.name,
				}
			}
		}
	}
	stageIDs := make([]string, 0, len(stagedByID))
	for groupID := range stagedByID {
		stageIDs = append(stageIDs, groupID)
	}
	sort.Strings(stageIDs)
	for _, groupID := range stageIDs {
		staged := stagedByID[groupID]
		base := staged.archivedName
		if staged.transient {
			base = fmt.Sprintf("__sub2api_management_sync_%s__", groupID)
		}
		stagedName := base
		for suffix := 2; ; suffix++ {
			_, occupied := occupiedNames[stagedName]
			if !occupied {
				var err error
				occupied, err = managementGroupReferenceNameOccupied(ctx, tx, stagedName)
				if err != nil {
					return fmt.Errorf("分组 %s 暂存名称检查失败：%w", groupID, err)
				}
				if !occupied {
					break
				}
			}
			stagedName = fmt.Sprintf("%s (%d)", base, suffix)
		}
		occupiedNames[stagedName] = struct{}{}
		if err := reconcileManagementGroupIdentityFrom(ctx, tx, groupID, stagedName, staged.preferredName, now); err != nil {
			return fmt.Errorf("分组 %s 暂存失败：%w", groupID, err)
		}
		if !staged.transient {
			separatedDeletedIDs[groupID] = struct{}{}
		}
	}
	return nil
}

func reconcileManagementGroupIdentity(ctx context.Context, tx *sql.Tx, groupID, currentName, now string) error {
	return reconcileManagementGroupIdentityFrom(ctx, tx, groupID, currentName, currentName, now)
}

func reconcileManagementGroupIdentityFrom(
	ctx context.Context,
	tx *sql.Tx,
	groupID string,
	currentName string,
	preferredName string,
	now string,
) error {
	rows, err := tx.QueryContext(ctx, `SELECT name,remote_id,strategy,strategy_source,platform,rate_multiplier,
		profit_control_enabled,profit_min_margin,profit_safety_buffer,account_count
		FROM local_groups WHERE remote_id=? OR name=? ORDER BY
		CASE WHEN name=? THEN 0 WHEN name=? THEN 1 ELSE 2 END,name`,
		groupID, currentName, preferredName, currentName)
	if err != nil {
		return err
	}
	defer rows.Close()
	values := make([]localGroupIdentityRow, 0, 2)
	for rows.Next() {
		var value localGroupIdentityRow
		if err := rows.Scan(
			&value.name, &value.remoteID, &value.strategy, &value.strategySource, &value.platform,
			&value.rateMultiplier, &value.profitEnabled, &value.profitMinMargin,
			&value.profitSafetyBuffer, &value.accountCount,
		); err != nil {
			return err
		}
		if value.remoteID.Valid && strings.TrimSpace(value.remoteID.String) != groupID {
			return fmt.Errorf("名称 %q 已属于分组 ID %s", currentName, value.remoteID.String)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(values) == 0 {
		return nil
	}
	canonical := values[0]
	canonical.name = currentName
	canonical.remoteID = sql.NullString{String: groupID, Valid: true}
	for _, value := range values[1:] {
		if canonical.strategySource != "group_override" && value.strategySource == "group_override" {
			canonical.strategy, canonical.strategySource = value.strategy, value.strategySource
		}
		mergeNullString(&canonical.platform, value.platform)
		mergeNullString(&canonical.rateMultiplier, value.rateMultiplier)
		mergeNullInt64(&canonical.profitEnabled, value.profitEnabled)
		mergeNullString(&canonical.profitMinMargin, value.profitMinMargin)
		mergeNullString(&canonical.profitSafetyBuffer, value.profitSafetyBuffer)
		if value.accountCount > canonical.accountCount {
			canonical.accountCount = value.accountCount
		}
	}
	staleNames := make([]string, 0, len(values))
	for _, value := range values {
		if value.name != currentName {
			staleNames = append(staleNames, value.name)
		}
	}
	if len(staleNames) == 0 && len(values) == 1 && values[0].remoteID.Valid {
		return nil
	}
	if err := migrateManagementGroupReferences(ctx, tx, groupID, currentName, staleNames); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO local_groups(
		name,remote_id,strategy,strategy_source,platform,rate_multiplier,profit_control_enabled,
		profit_min_margin,profit_safety_buffer,account_count,updated_at
	) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(name) DO UPDATE SET
		remote_id=excluded.remote_id,strategy=excluded.strategy,strategy_source=excluded.strategy_source,
		platform=excluded.platform,rate_multiplier=excluded.rate_multiplier,
		profit_control_enabled=excluded.profit_control_enabled,profit_min_margin=excluded.profit_min_margin,
		profit_safety_buffer=excluded.profit_safety_buffer,account_count=excluded.account_count,updated_at=excluded.updated_at`,
		canonical.name, groupID, canonical.strategy, canonical.strategySource,
		managementSQLNullString(canonical.platform), managementSQLNullString(canonical.rateMultiplier), managementSQLNullInt64(canonical.profitEnabled),
		managementSQLNullString(canonical.profitMinMargin), managementSQLNullString(canonical.profitSafetyBuffer), canonical.accountCount, now,
	); err != nil {
		return err
	}
	for _, staleName := range staleNames {
		if _, err := tx.ExecContext(ctx, `DELETE FROM local_groups WHERE name=? AND remote_id=?`, staleName, groupID); err != nil {
			return err
		}
	}
	return nil
}

func migrateManagementGroupReferences(ctx context.Context, tx *sql.Tx, groupID, currentName string, staleNames []string) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO account_groups(account_id,group_name,group_id,group_rate)
		SELECT account_id,?,?,group_rate FROM account_groups WHERE group_id=?
		ON CONFLICT(account_id,group_name) DO UPDATE SET group_id=excluded.group_id,group_rate=excluded.group_rate`,
		currentName, groupID, groupID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM account_groups WHERE group_id=? AND group_name<>?`, groupID, currentName); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE account_groups SET group_id=? WHERE group_name=? AND group_id IS NULL`, groupID, currentName); err != nil {
		return err
	}
	for _, staleName := range staleNames {
		if _, err := tx.ExecContext(ctx, `INSERT INTO account_groups(account_id,group_name,group_id,group_rate)
			SELECT account_id,?,?,group_rate FROM account_groups WHERE group_name=? AND group_id IS NULL
			ON CONFLICT(account_id,group_name) DO UPDATE SET group_id=excluded.group_id,group_rate=excluded.group_rate`,
			currentName, groupID, staleName); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM account_groups WHERE group_name=? AND group_id IS NULL`, staleName); err != nil {
			return err
		}
		for _, statement := range []string{
			`UPDATE routing_decisions SET group_name=? WHERE group_name=?`,
			`UPDATE account_health_evaluations SET group_name=? WHERE group_name=?`,
			`UPDATE bindings SET local_group=? WHERE local_group=?`,
			`UPDATE onboarding_pending SET local_group_name=? WHERE local_group_name=?`,
		} {
			if _, err := tx.ExecContext(ctx, statement, currentName, staleName); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM health_samples
			WHERE group_name=? AND evidence_key IS NOT NULL AND EXISTS(
				SELECT 1 FROM health_samples AS stale
				WHERE stale.group_name=? AND stale.account_id=health_samples.account_id
				AND stale.source=health_samples.source AND stale.evidence_key=health_samples.evidence_key
				AND (COALESCE(stale.observed_at,'')>COALESCE(health_samples.observed_at,'') OR
					(COALESCE(stale.observed_at,'')=COALESCE(health_samples.observed_at,'') AND stale.id>health_samples.id))
			)`, currentName, staleName); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE OR IGNORE health_samples SET group_name=? WHERE group_name=?`, currentName, staleName); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM health_samples WHERE group_name=?`, staleName); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM usage_records
			WHERE group_name=? AND EXISTS(
				SELECT 1 FROM usage_records AS stale
				WHERE stale.group_name=? AND stale.request_id=usage_records.request_id
				AND stale.account_id=usage_records.account_id
				AND stale.observed_at=usage_records.observed_at AND stale.id>usage_records.id
			)`, currentName, staleName); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE OR IGNORE usage_records SET group_name=? WHERE group_name=?`, currentName, staleName); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM usage_records WHERE group_name=?`, staleName); err != nil {
			return err
		}
	}
	return nil
}

func mergeNullString(target *sql.NullString, candidate sql.NullString) {
	if !target.Valid && candidate.Valid {
		*target = candidate
	}
}

func mergeNullInt64(target *sql.NullInt64, candidate sql.NullInt64) {
	if !target.Valid && candidate.Valid {
		*target = candidate
	}
}

func managementSQLNullString(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

func managementSQLNullInt64(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
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
			if id != nil {
				catalogName, found := byID[*id]
				if !found {
					return nil, true, fmt.Errorf("分组列表引用目录外稳定 ID：%s", *id)
				}
				if name != "" && name != catalogName {
					return nil, true, fmt.Errorf("分组列表中的稳定 ID 与名称不一致：%s 应为 %s", *id, catalogName)
				}
				name = catalogName
			} else if name != "" {
				if catalogID, found := byName[name]; found {
					id = stringPointer(catalogID)
				}
			}
		case string, json.Number, int, int64:
			text := strings.TrimSpace(fmt.Sprint(item))
			if parsed, err := managementStableID(item); err == nil {
				catalogName, found := byID[parsed]
				if !found {
					return nil, true, fmt.Errorf("分组列表引用目录外稳定 ID：%s", parsed)
				}
				id = stringPointer(parsed)
				name = catalogName
			} else {
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
