package business

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

const catalogMissingConfirmationCount = 2

type upstreamCatalogEntity struct {
	kind                string
	id                  string
	parentID            *string
	name                string
	observedStatus      *string
	lifecycleState      string
	missingObservations int64
}

type accountCatalogBindingState struct {
	Status string
	Reason string
}

func (s *Store) ensureStableUpstreamRelations(ctx context.Context) error {
	if err := s.ensureMissingUpstreamIdentities(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := ensureBindingIdentitiesTx(ctx, tx, ""); err != nil {
		return err
	}
	if err := ensureCatalogEntitiesFromRowsTx(ctx, tx); err != nil {
		return err
	}
	if err := ensureCatalogEntitiesFromBindingsTx(ctx, tx, "", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

func ensureBindingIdentitiesTx(ctx context.Context, tx *sql.Tx, upstreamID string) error {
	query := `INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,upstream_group_id,updated_at)
		SELECT b.id,h.upstream_id,b.upstream_key_id,NULLIF(TRIM(b.upstream_group_id),''),b.updated_at
		FROM bindings b JOIN upstream_identity_hosts h ON h.host=b.upstream_host`
	arguments := []any{}
	if strings.TrimSpace(upstreamID) != "" {
		query += ` WHERE h.upstream_id=?`
		arguments = append(arguments, strings.TrimSpace(upstreamID))
	} else {
		query += ` WHERE 1=1`
	}
	query += ` ON CONFLICT(binding_id) DO UPDATE SET upstream_id=excluded.upstream_id,
		upstream_key_id=excluded.upstream_key_id,upstream_group_id=excluded.upstream_group_id,updated_at=excluded.updated_at`
	if _, err := tx.ExecContext(ctx, query, arguments...); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM binding_identities WHERE NOT EXISTS(
		SELECT 1 FROM bindings b WHERE b.id=binding_identities.binding_id
	)`)
	return err
}

func ensureCatalogEntitiesFromRowsTx(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `UPDATE upstream_catalog_entities SET
		lifecycle_state='suspected',missing_observations=1,confirmed_missing_at=NULL
		WHERE lifecycle_state='missing' AND missing_observations=?
		AND observed_status IN ('missing','deleted')
		AND last_seen_at=missing_since AND missing_since=confirmed_missing_at AND confirmed_missing_at=updated_at`,
		catalogMissingConfirmationCount); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT h.upstream_id,'group',g.group_id,NULL,g.name,g.status,g.updated_at,h.is_primary
		FROM upstream_groups g JOIN upstream_identity_hosts h ON h.host=g.host
		UNION ALL
		SELECT h.upstream_id,'key',k.key_id,NULLIF(TRIM(k.upstream_group),''),k.name,k.status,k.updated_at,h.is_primary
		FROM upstream_keys k JOIN upstream_identity_hosts h ON h.host=k.host
		ORDER BY 1,2,3,8 DESC`)
	if err != nil {
		return err
	}
	defer rows.Close()
	type catalogRow struct {
		upstreamID, kind, id, name, updatedAt string
		parentID, status                      sql.NullString
		primary                               int64
	}
	items := []catalogRow{}
	for rows.Next() {
		var item catalogRow
		if err := rows.Scan(&item.upstreamID, &item.kind, &item.id, &item.parentID, &item.name, &item.status, &item.updatedAt, &item.primary); err != nil {
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	groupIDs := map[string]string{}
	ambiguousGroups := map[string]struct{}{}
	for _, item := range items {
		if item.kind != "group" {
			continue
		}
		groupIDs[catalogGroupReferenceKey(item.upstreamID, item.id)] = item.id
		nameKey := catalogGroupReferenceKey(item.upstreamID, item.name)
		if prior, found := groupIDs[nameKey]; found && prior != item.id {
			delete(groupIDs, nameKey)
			ambiguousGroups[nameKey] = struct{}{}
		} else if _, ambiguous := ambiguousGroups[nameKey]; !ambiguous {
			groupIDs[nameKey] = item.id
		}
	}
	for _, item := range items {
		if item.kind == "key" && item.parentID.Valid {
			if groupID, found := groupIDs[catalogGroupReferenceKey(item.upstreamID, item.parentID.String)]; found {
				item.parentID.String = groupID
			}
		}
		lifecycle, observations := initialCatalogLifecycle(item.status.String)
		var missingSince, confirmedMissingAt any
		if lifecycle == "suspected" || lifecycle == "missing" {
			missingSince = item.updatedAt
		}
		if lifecycle == "missing" {
			confirmedMissingAt = item.updatedAt
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO upstream_catalog_entities(
			upstream_id,entity_kind,entity_id,parent_entity_id,name,observed_status,lifecycle_state,missing_observations,
			last_seen_at,missing_since,confirmed_missing_at,updated_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(upstream_id,entity_kind,entity_id) DO NOTHING`,
			item.upstreamID, item.kind, item.id, nullString(item.parentID), item.name, nullString(item.status), lifecycle,
			observations, item.updatedAt, missingSince, confirmedMissingAt, item.updatedAt); err != nil {
			return err
		}
	}
	return nil
}

func catalogGroupReferenceKey(upstreamID, reference string) string {
	return strings.TrimSpace(upstreamID) + "\x00" + strings.TrimSpace(reference)
}

func initialCatalogLifecycle(status string) (string, int64) {
	switch normalizedCatalogStatus(stringPointer(strings.TrimSpace(status))) {
	case "missing", "deleted":
		return "suspected", 1
	case "suspected":
		return "suspected", 1
	default:
		return "active", 0
	}
}

func ensureCatalogEntitiesFromBindingsTx(ctx context.Context, tx *sql.Tx, upstreamID, now string) error {
	filter, arguments := "", []any{}
	if strings.TrimSpace(upstreamID) != "" {
		filter = " AND bi.upstream_id=?"
		arguments = append(arguments, strings.TrimSpace(upstreamID))
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO upstream_catalog_entities(
		upstream_id,entity_kind,entity_id,parent_entity_id,name,lifecycle_state,missing_observations,updated_at
	) SELECT DISTINCT bi.upstream_id,'key',bi.upstream_key_id,bi.upstream_group_id,
		COALESCE(NULLIF(TRIM(b.upstream_key_name),''),bi.upstream_key_id),'active',0,?
		FROM binding_identities bi JOIN bindings b ON b.id=bi.binding_id
		WHERE TRIM(bi.upstream_key_id)<>''`+filter+`
		ON CONFLICT(upstream_id,entity_kind,entity_id) DO NOTHING`, append([]any{now}, arguments...)...); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO upstream_catalog_entities(
		upstream_id,entity_kind,entity_id,name,lifecycle_state,missing_observations,updated_at
	) SELECT DISTINCT bi.upstream_id,'group',bi.upstream_group_id,
		COALESCE(NULLIF(TRIM(b.upstream_group),''),bi.upstream_group_id),'active',0,?
		FROM binding_identities bi JOIN bindings b ON b.id=bi.binding_id
		WHERE TRIM(COALESCE(bi.upstream_group_id,''))<>''`+filter+`
		ON CONFLICT(upstream_id,entity_kind,entity_id) DO NOTHING`, append([]any{now}, arguments...)...); err != nil {
		return err
	}
	return nil
}

func upsertLiveCatalogEntitiesTx(
	ctx context.Context,
	tx *sql.Tx,
	upstreamID string,
	groups []UpstreamCatalogGroup,
	keys []UpstreamCatalogKey,
	now string,
) error {
	groupIDs := make(map[string]string, len(groups)*2)
	ambiguousNames := map[string]struct{}{}
	for _, group := range groups {
		groupID := strings.TrimSpace(group.GroupID)
		name := strings.TrimSpace(group.Name)
		if groupID == "" {
			continue
		}
		groupIDs[groupID] = groupID
		if name == "" {
			continue
		}
		if prior, found := groupIDs[name]; found && prior != groupID {
			delete(groupIDs, name)
			ambiguousNames[name] = struct{}{}
		} else if _, ambiguous := ambiguousNames[name]; !ambiguous {
			groupIDs[name] = groupID
		}
	}
	for _, group := range groups {
		if _, err := tx.ExecContext(ctx, `INSERT INTO upstream_catalog_entities(
			upstream_id,entity_kind,entity_id,name,observed_status,lifecycle_state,missing_observations,last_seen_at,updated_at
		) VALUES(?,'group',?,?,?,'active',0,?,?) ON CONFLICT(upstream_id,entity_kind,entity_id) DO UPDATE SET
			name=excluded.name,observed_status=excluded.observed_status,lifecycle_state='active',missing_observations=0,
			last_seen_at=excluded.last_seen_at,missing_since=NULL,confirmed_missing_at=NULL,updated_at=excluded.updated_at`,
			upstreamID, group.GroupID, group.Name, group.Status, now, now); err != nil {
			return err
		}
	}
	for _, key := range keys {
		parentID := key.UpstreamGroup
		if reference := strings.TrimSpace(pointerValue(key.UpstreamGroup)); reference != "" {
			if stableID, found := groupIDs[reference]; found {
				parentID = stringPointer(stableID)
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO upstream_catalog_entities(
			upstream_id,entity_kind,entity_id,parent_entity_id,name,observed_status,lifecycle_state,missing_observations,last_seen_at,updated_at
		) VALUES(?,'key',?,?,?,?, 'active',0,?,?) ON CONFLICT(upstream_id,entity_kind,entity_id) DO UPDATE SET
			parent_entity_id=excluded.parent_entity_id,name=excluded.name,observed_status=excluded.observed_status,
			lifecycle_state='active',missing_observations=0,last_seen_at=excluded.last_seen_at,
			missing_since=NULL,confirmed_missing_at=NULL,updated_at=excluded.updated_at`,
			upstreamID, key.KeyID, parentID, key.Name, key.Status, now, now); err != nil {
			return err
		}
	}
	return nil
}

func reconcileCatalogEntitiesTx(ctx context.Context, tx *sql.Tx, upstreamID, kind string, live map[string]struct{}, now string) error {
	rows, err := tx.QueryContext(ctx, `SELECT entity_id,missing_observations FROM upstream_catalog_entities
		WHERE upstream_id=? AND entity_kind=?`, upstreamID, kind)
	if err != nil {
		return err
	}
	type existingEntity struct {
		id           string
		observations int64
	}
	items := []existingEntity{}
	for rows.Next() {
		var item existingEntity
		if err := rows.Scan(&item.id, &item.observations); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range items {
		if _, present := live[item.id]; present {
			continue
		}
		observations := item.observations + 1
		state := "suspected"
		var confirmed any
		if observations >= catalogMissingConfirmationCount {
			state, confirmed = "missing", now
		}
		if _, err := tx.ExecContext(ctx, `UPDATE upstream_catalog_entities SET lifecycle_state=?,missing_observations=?,
			missing_since=COALESCE(missing_since,?),confirmed_missing_at=COALESCE(confirmed_missing_at,?),updated_at=?
			WHERE upstream_id=? AND entity_kind=? AND entity_id=?`, state, observations, now, confirmed, now, upstreamID, kind, item.id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) accountCatalogBindingStates(ctx context.Context) (map[string]accountCatalogBindingState, error) {
	if err := s.ensureStableUpstreamRelations(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT b.local_account_id,bi.upstream_key_id,bi.upstream_group_id,
		k.parent_entity_id,k.observed_status,k.lifecycle_state,k.missing_observations,
		g.entity_id,g.observed_status,g.lifecycle_state,g.missing_observations
		FROM binding_identities bi JOIN bindings b ON b.id=bi.binding_id
		LEFT JOIN upstream_catalog_entities k ON k.upstream_id=bi.upstream_id AND k.entity_kind='key' AND k.entity_id=bi.upstream_key_id
		LEFT JOIN upstream_catalog_entities g ON g.upstream_id=bi.upstream_id AND g.entity_kind='group'
			AND g.entity_id=COALESCE(NULLIF(TRIM(k.parent_entity_id),''),bi.upstream_group_id)
		ORDER BY b.local_account_id,bi.binding_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]accountCatalogBindingState{}
	for rows.Next() {
		var accountID, keyID string
		var bindingGroupID, keyGroupID, keyObserved, keyLifecycle sql.NullString
		var groupID, groupObserved, groupLifecycle sql.NullString
		var keyMissing, groupMissing sql.NullInt64
		if err := rows.Scan(&accountID, &keyID, &bindingGroupID, &keyGroupID, &keyObserved, &keyLifecycle, &keyMissing,
			&groupID, &groupObserved, &groupLifecycle, &groupMissing); err != nil {
			return nil, err
		}
		state := catalogBindingState(
			keyID, firstNonblankNullString(keyGroupID, bindingGroupID),
			nullString(keyObserved), nullString(keyLifecycle), keyMissing,
			nullString(groupID), nullString(groupObserved), nullString(groupLifecycle), groupMissing,
		)
		previous, found := result[accountID]
		if !found || catalogBindingSeverity(state.Status) > catalogBindingSeverity(previous.Status) {
			result[accountID] = state
		} else if catalogBindingSeverity(state.Status) == catalogBindingSeverity(previous.Status) && state.Reason != previous.Reason {
			previous.Reason = strings.Join(uniqueSortedStrings([]string{previous.Reason, state.Reason}), "；")
			result[accountID] = previous
		}
	}
	return result, rows.Err()
}

func catalogBindingState(
	keyID string,
	groupReference *string,
	keyObserved, keyLifecycle *string,
	keyMissing sql.NullInt64,
	groupID, groupObserved, groupLifecycle *string,
	groupMissing sql.NullInt64,
) accountCatalogBindingState {
	group := pointerValue(groupReference)
	keyLife, groupLife := normalizedCatalogStatus(keyLifecycle), normalizedCatalogStatus(groupLifecycle)
	keyMiss, groupMiss := int64(0), int64(0)
	if keyMissing.Valid {
		keyMiss = keyMissing.Int64
	}
	if groupMissing.Valid {
		groupMiss = groupMissing.Int64
	}
	if keyLife == "missing" && group != "" && groupLife == "missing" {
		return accountCatalogBindingState{Status: "key_and_group_missing", Reason: fmt.Sprintf(
			"上游 Key %s 和所属分组 %s 已确认删除（连续 %d 次完整同步未返回）", keyID, group, max(keyMiss, groupMiss))}
	}
	if keyLife == "missing" {
		return accountCatalogBindingState{Status: "key_missing", Reason: fmt.Sprintf(
			"绑定的上游 Key %s 已确认删除（连续 %d 次完整同步未返回）", keyID, keyMiss)}
	}
	if group != "" && groupLife == "missing" {
		return accountCatalogBindingState{Status: "group_missing", Reason: fmt.Sprintf(
			"上游 Key %s 所属分组 %s 已确认删除（连续 %d 次完整同步未返回）", keyID, group, groupMiss)}
	}
	if keyLife == "suspected" || group != "" && groupLife == "suspected" {
		return accountCatalogBindingState{Status: "suspected", Reason: "本轮完整同步未返回绑定的上游 Key 或分组，等待下一轮复核；当前不停止调度"}
	}
	if keyLife == "" {
		return accountCatalogBindingState{Status: "unknown", Reason: fmt.Sprintf("上游 Key %s 尚未完成稳定身份确认", keyID)}
	}
	if status := normalizedCatalogStatus(keyObserved); status != "" && !activeStatus(status) {
		return accountCatalogBindingState{Status: status, Reason: fmt.Sprintf("上游 Key %s 状态为 %s", keyID, status)}
	}
	if group != "" && groupID == nil {
		return accountCatalogBindingState{Status: "unknown", Reason: fmt.Sprintf("上游 Key %s 的所属分组 %s 尚未完成稳定身份确认", keyID, group)}
	}
	if status := normalizedCatalogStatus(groupObserved); status != "" && !activeStatus(status) {
		return accountCatalogBindingState{Status: "group_inactive", Reason: fmt.Sprintf("上游 Key %s 所属分组 %s 状态为 %s", keyID, group, status)}
	}
	return accountCatalogBindingState{Status: "active", Reason: fmt.Sprintf("上游 Key %s 使用稳定 ID 绑定，所属分组仍存在", keyID)}
}

func catalogBindingSeverity(status string) int {
	switch normalizedCatalogStatus(&status) {
	case "key_and_group_missing":
		return 5
	case "key_missing", "group_missing":
		return 4
	case "suspected":
		return 3
	case "group_inactive", "inactive", "disabled", "expired":
		return 2
	case "unknown", "unbound":
		return 1
	default:
		return 0
	}
}

func firstNonblankNullString(values ...sql.NullString) *string {
	for _, value := range values {
		if value.Valid && strings.TrimSpace(value.String) != "" {
			return stringPointer(strings.TrimSpace(value.String))
		}
	}
	return nil
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
