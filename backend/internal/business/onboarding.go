package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type OnboardingCandidate struct {
	Number             int                    `json:"number"`
	UpstreamID         string                 `json:"upstream_id"`
	Host               string                 `json:"host"`
	UpstreamName       string                 `json:"upstream_name"`
	GroupID            *string                `json:"group_id"`
	GroupName          string                 `json:"group_name"`
	Description        *string                `json:"description"`
	Platform           *string                `json:"platform"`
	Status             *string                `json:"status"`
	Multiplier         *string                `json:"multiplier"`
	RecommendedBinding string                 `json:"recommended_binding"`
	Bindable           bool                   `json:"bindable"`
	CanCreateKey       bool                   `json:"can_create_key"`
	CanBindExistingKey bool                   `json:"can_bind_existing_key"`
	Bound              bool                   `json:"bound"`
	BoundAccounts      []UpstreamBoundAccount `json:"bound_accounts"`
	KeyPresent         bool                   `json:"key_present"`
	UpstreamKeyID      *string                `json:"upstream_key_id"`
	UpstreamKeyName    *string                `json:"upstream_key_name"`
	RechargeRate       *string                `json:"recharge_rate"`
	UnavailableReason  *string                `json:"unavailable_reason"`
}

type LocalOnboardingGroup struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Platform *string `json:"platform"`
}

type PendingOnboarding struct {
	OperationID          string
	UpstreamID           string
	UpstreamHost         string
	UpstreamType         string
	UpstreamKeyID        string
	UpstreamKeyName      *string
	UpstreamAccountID    string
	UpstreamGroupID      string
	UpstreamGroupName    string
	LocalGroupID         string
	LocalGroupName       string
	LocalGroupIDs        []string
	Multiplier           string
	IntentHash           string
	Reason               string
	KeyCommitUnknown     bool
	AccountCommitUnknown bool
	CreatedAt            string
	UpdatedAt            string
}

type OnboardingProjection struct {
	OperationID       string
	AccountID         string
	AccountName       string
	Platform          string
	UpstreamHost      string
	UpstreamType      string
	BaseURL           string
	UpstreamKeyID     string
	UpstreamKeyName   string
	UpstreamGroupID   string
	UpstreamGroupName string
	LocalGroupID      string
	LocalGroupName    string
	LocalGroups       []LocalOnboardingGroup
	Multiplier        string
	Schedulable       bool
	Priority          *int64
	Concurrency       *int64
	Models            []string
	Notes             string
	Actor             string
	ReadbackConfirmed bool
}

func (s *Store) OnboardingCandidates(ctx context.Context, host string) ([]OnboardingCandidate, error) {
	host = canonicalHost(host)
	upstreamName := ""
	if summary, summaryErr := s.Upstreams(ctx); summaryErr == nil {
		for _, upstream := range summary.Hosts {
			if upstream.Host == host {
				upstreamName = upstream.Name
				break
			}
		}
	}
	groups, err := s.UpstreamGroups(ctx, host, true)
	if err != nil {
		return nil, err
	}
	keys, err := s.onboardingKeys(ctx, host)
	if err != nil {
		return nil, err
	}
	result := make([]OnboardingCandidate, 0, len(groups))
	for index, group := range groups {
		candidate := OnboardingCandidate{
			Number: index + 1, UpstreamID: group.UpstreamID, Host: host, UpstreamName: upstreamName, GroupID: group.GroupID, GroupName: group.Name,
			Description: group.Description, Platform: group.Platform, Status: group.Status,
			Multiplier: group.EffectiveRate, RecommendedBinding: recommendedBinding(group.Name, group.Description),
			Bound: group.Bound, BoundAccounts: group.BoundAccounts,
			KeyPresent: group.KeyPresent, RechargeRate: group.RechargeRate,
			UnavailableReason: group.UnavailableReason,
		}
		if group.GroupID != nil {
			if key, found := keys[*group.GroupID]; found {
				candidate.UpstreamKeyID, candidate.UpstreamKeyName = &key.id, &key.name
			}
		}
		active := activeStatus(pointerValue(group.Status))
		candidate.CanCreateKey = active && group.GroupID != nil && strings.TrimSpace(group.Name) != "" &&
			group.EffectiveRate != nil && strings.TrimSpace(*group.EffectiveRate) != "" && group.UnavailableReason == nil
		candidate.CanBindExistingKey = group.Bindable && candidate.UpstreamKeyID != nil
		candidate.Bindable = candidate.CanBindExistingKey
		if candidate.UnavailableReason == nil && !candidate.CanCreateKey {
			reason := onboardingGroupReason(group)
			candidate.UnavailableReason = &reason
		}
		result = append(result, candidate)
	}
	return result, nil
}

func (s *Store) LocalOnboardingGroup(ctx context.Context, groupID string) (LocalOnboardingGroup, error) {
	groupID = strings.TrimSpace(groupID)
	if !positiveNumericID(groupID) {
		return LocalOnboardingGroup{}, errors.New("本地分组必须使用有效稳定 ID")
	}
	var result LocalOnboardingGroup
	err := s.db.QueryRowContext(ctx, `SELECT remote_id,name,platform FROM local_groups WHERE remote_id=?`, groupID).
		Scan(&result.ID, &result.Name, &result.Platform)
	if errors.Is(err, sql.ErrNoRows) {
		return LocalOnboardingGroup{}, errors.New("目标本地分组不存在")
	}
	return result, err
}

func (s *Store) PendingOnboarding(ctx context.Context, host, upstreamGroupID string, localGroupIDs []string) (*PendingOnboarding, error) {
	_, selectionJSON, err := canonicalPendingLocalGroupIDs(localGroupIDs)
	if err != nil {
		return nil, err
	}
	upstreamID, _, err := upstreamIdentityHostsForQueryer(ctx, s.db, host)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT p.operation_id,p.upstream_id,p.upstream_host,p.upstream_type,p.upstream_key_id,p.upstream_key_name,p.upstream_account_id,
		p.upstream_group_id,p.upstream_group_name,p.local_group_id,p.local_group_name,p.local_group_ids_json,p.multiplier,p.intent_hash,p.reason,
		p.key_commit_unknown,p.account_commit_unknown,p.created_at,p.updated_at
		FROM onboarding_pending p LEFT JOIN upstream_identity_hosts h ON h.host=p.upstream_host
		WHERE (p.upstream_id=? OR (p.upstream_id='' AND h.upstream_id=?)) AND p.upstream_group_id=?
		ORDER BY p.updated_at DESC,p.operation_id`, upstreamID, upstreamID, strings.TrimSpace(upstreamGroupID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matched *PendingOnboarding
	for rows.Next() {
		var value PendingOnboarding
		var keyName sql.NullString
		var storedSelection string
		if err := rows.Scan(
			&value.OperationID, &value.UpstreamID, &value.UpstreamHost, &value.UpstreamType, &value.UpstreamKeyID, &keyName, &value.UpstreamAccountID,
			&value.UpstreamGroupID, &value.UpstreamGroupName, &value.LocalGroupID, &value.LocalGroupName, &storedSelection,
			&value.Multiplier, &value.IntentHash, &value.Reason, &value.KeyCommitUnknown, &value.AccountCommitUnknown, &value.CreatedAt, &value.UpdatedAt,
		); err != nil {
			return nil, err
		}
		storedSelection = strings.TrimSpace(storedSelection)
		if storedSelection == "" {
			if strings.TrimSpace(value.IntentHash) == "" {
				return nil, errors.New("待续开户记录缺少首次冻结意图，已拒绝远端写入")
			}
			return nil, errors.New("待续开户记录缺少规范化本地分组集合，已拒绝远端写入")
		} else {
			var storedIDs []string
			if err := json.Unmarshal([]byte(storedSelection), &storedIDs); err != nil {
				return nil, errors.New("待续开户记录的本地分组集合损坏")
			}
			storedIDs, canonicalStoredSelection, err := canonicalPendingLocalGroupIDs(storedIDs)
			if err != nil {
				return nil, errors.New("待续开户记录的本地分组集合损坏")
			}
			if canonicalStoredSelection != selectionJSON {
				continue
			}
			value.LocalGroupIDs = storedIDs
		}
		value.UpstreamKeyName = nullString(keyName)
		if matched != nil {
			return nil, errors.New("同一开户身份存在多条待续记录，已拒绝远端写入")
		}
		matched = &value
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if matched == nil {
		return nil, nil
	}
	if strings.TrimSpace(matched.IntentHash) == "" {
		return nil, errors.New("旧版待续开户记录缺少首次冻结意图，已拒绝远端写入")
	}
	return matched, nil
}

func (s *Store) SavePendingOnboarding(ctx context.Context, value PendingOnboarding) error {
	localGroupIDs, selectionJSON, err := canonicalPendingLocalGroupIDs(value.LocalGroupIDs)
	if err != nil {
		return err
	}
	primaryPresent := false
	for _, id := range localGroupIDs {
		if id == strings.TrimSpace(value.LocalGroupID) {
			primaryPresent = true
			break
		}
	}
	if strings.TrimSpace(value.OperationID) == "" || canonicalHost(value.UpstreamHost) == "" ||
		strings.TrimSpace(value.UpstreamGroupID) == "" ||
		!positiveNumericID(value.LocalGroupID) || !primaryPresent || normalizePositiveDecimal(value.Multiplier) == nil ||
		strings.TrimSpace(value.IntentHash) == "" {
		return errors.New("待续开户记录字段不完整")
	}
	if strings.TrimSpace(value.UpstreamKeyID) == "" && (value.UpstreamKeyName == nil || strings.TrimSpace(*value.UpstreamKeyName) == "") {
		return errors.New("待续开户记录缺少 Key ID 与 marker")
	}
	upstreamID, _, err := upstreamIdentityHostsForQueryer(ctx, s.db, value.UpstreamHost)
	if err != nil {
		return fmt.Errorf("待续开户记录无法解析稳定上游身份: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if value.CreatedAt == "" {
		value.CreatedAt = now
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO onboarding_pending(operation_id,upstream_id,upstream_host,upstream_type,
		upstream_key_id,upstream_key_name,upstream_account_id,upstream_group_id,upstream_group_name,local_group_id,local_group_name,
		local_group_ids_json,multiplier,intent_hash,reason,key_commit_unknown,account_commit_unknown,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(operation_id) DO UPDATE SET upstream_key_id=excluded.upstream_key_id,
		upstream_key_name=excluded.upstream_key_name,upstream_account_id=excluded.upstream_account_id,reason=excluded.reason,
		local_group_ids_json=CASE WHEN onboarding_pending.local_group_ids_json='' THEN excluded.local_group_ids_json ELSE onboarding_pending.local_group_ids_json END,
		key_commit_unknown=excluded.key_commit_unknown,account_commit_unknown=excluded.account_commit_unknown,
		updated_at=excluded.updated_at`,
		value.OperationID, upstreamID, canonicalHost(value.UpstreamHost), strings.TrimSpace(value.UpstreamType), value.UpstreamKeyID,
		managementNullableString(value.UpstreamKeyName), value.UpstreamAccountID, value.UpstreamGroupID, value.UpstreamGroupName, value.LocalGroupID,
		value.LocalGroupName, selectionJSON, value.Multiplier, value.IntentHash, safeOnboardingReason(value.Reason), value.KeyCommitUnknown,
		value.AccountCommitUnknown, value.CreatedAt, now)
	return err
}

func (s *Store) UpgradePendingOnboardingIntent(ctx context.Context, operationID, previousHash, nextHash string) (bool, error) {
	operationID = strings.TrimSpace(operationID)
	previousHash = strings.TrimSpace(previousHash)
	nextHash = strings.TrimSpace(nextHash)
	if operationID == "" || previousHash == "" || nextHash == "" || previousHash == nextHash {
		return false, errors.New("待续开户意图升级参数无效")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE onboarding_pending SET intent_hash=?,updated_at=?
		WHERE operation_id=? AND intent_hash=? AND upstream_key_id<>'' AND upstream_account_id=''
		AND key_commit_unknown=0 AND account_commit_unknown=0`,
		nextHash, time.Now().UTC().Format(time.RFC3339Nano), operationID, previousHash)
	if err != nil {
		return false, err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return updated == 1, nil
}

func canonicalPendingLocalGroupIDs(values []string) ([]string, string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !positiveNumericID(value) {
			return nil, "", errors.New("待续开户记录的本地分组集合无效")
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil, "", errors.New("待续开户记录缺少本地分组集合")
	}
	sort.Slice(result, func(left, right int) bool {
		leftID, _ := strconv.ParseUint(result[left], 10, 64)
		rightID, _ := strconv.ParseUint(result[right], 10, 64)
		if leftID == rightID {
			return result[left] < result[right]
		}
		return leftID < rightID
	})
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, "", err
	}
	return result, string(encoded), nil
}

func (s *Store) CommitOnboardingProjection(ctx context.Context, value OnboardingProjection) error {
	if !positiveNumericID(value.AccountID) || !positiveNumericID(value.LocalGroupID) ||
		strings.TrimSpace(value.OperationID) == "" || strings.TrimSpace(value.AccountName) == "" ||
		canonicalHost(value.UpstreamHost) == "" || strings.TrimSpace(value.UpstreamKeyID) == "" ||
		normalizePositiveDecimal(value.Multiplier) == nil {
		return errors.New("账号添加字段不完整")
	}
	metadataValues := map[string]any{
		"onboarding": true, "onboarding_operation_id": value.OperationID,
		"onboarding_upstream_group_id": value.UpstreamGroupID, "onboarding_upstream_group": value.UpstreamGroupName,
		"onboarding_local_group_id": value.LocalGroupID, "onboarding_actor": actorOrDefault(value.Actor), "notes": value.Notes,
	}
	if platform := strings.ToLower(strings.TrimSpace(value.Platform)); platform != "" {
		metadataValues["platform"] = platform
	}
	if baseURL := strings.TrimSpace(value.BaseURL); baseURL != "" {
		metadataValues["base_url"] = baseURL
	}
	if len(value.Models) > 0 {
		metadataValues["known_models"] = append([]string{}, value.Models...)
	}
	metadata, err := json.Marshal(metadataValues)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO accounts(id,name,upstream_host,upstream_type,schedulable,priority,concurrency,
		multiplier,paused,metadata_json,updated_at) VALUES(?,?,?,?,?,?,?,?,0,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name,upstream_host=excluded.upstream_host,
		upstream_type=excluded.upstream_type,schedulable=excluded.schedulable,priority=excluded.priority,concurrency=excluded.concurrency,
		multiplier=excluded.multiplier,metadata_json=excluded.metadata_json,updated_at=excluded.updated_at`,
		value.AccountID, value.AccountName, canonicalHost(value.UpstreamHost), value.UpstreamType, value.Schedulable,
		value.Priority, value.Concurrency, value.Multiplier, string(metadata), now); err != nil {
		return err
	}
	localGroups := value.LocalGroups
	if len(localGroups) == 0 {
		localGroups = []LocalOnboardingGroup{{ID: value.LocalGroupID, Name: value.LocalGroupName}}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM account_groups WHERE account_id=?`, value.AccountID); err != nil {
		return err
	}
	for _, group := range localGroups {
		if _, err := tx.ExecContext(ctx, `INSERT INTO account_groups(account_id,group_name,group_id,group_rate) VALUES(?,?,?,?)`,
			value.AccountID, group.Name, group.ID, value.Multiplier); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,
		upstream_key_name,upstream_group,upstream_group_id,local_group,local_rate,description,status,metadata_json,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,'active',?,?) ON CONFLICT(local_account_id,upstream_host,upstream_key_id) DO UPDATE SET
		upstream_key_name=excluded.upstream_key_name,upstream_group=excluded.upstream_group,
		upstream_group_id=excluded.upstream_group_id,local_group=excluded.local_group,local_rate=excluded.local_rate,
		status=excluded.status,metadata_json=excluded.metadata_json,updated_at=excluded.updated_at`,
		value.AccountID, canonicalHost(value.UpstreamHost), value.UpstreamKeyID, value.UpstreamKeyName,
		value.UpstreamGroupName, value.UpstreamGroupID, value.LocalGroupName, value.Multiplier,
		onboardingBindingDescription(value.ReadbackConfirmed), fmt.Sprintf(`{"operation_id":%q,"local_group_id":%q}`, value.OperationID, value.LocalGroupID), now); err != nil {
		return err
	}
	upstreamID, _, err := upstreamIdentityHostsForQueryer(ctx, tx, value.UpstreamHost)
	if err != nil {
		return err
	}
	if err := ensureBindingIdentitiesTx(ctx, tx, upstreamID); err != nil {
		return err
	}
	if err := ensureCatalogEntitiesFromBindingsTx(ctx, tx, upstreamID, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE local_groups SET account_count=(SELECT COUNT(*) FROM account_groups
		WHERE group_name=local_groups.name),updated_at=?`, now); err != nil {
		return err
	}
	field := "created"
	name := value.AccountName
	phase := "remote-write"
	if value.ReadbackConfirmed {
		phase = "readback"
	}
	operation := AccountOperation{
		OperationID: value.OperationID, OperationType: "account.onboarding", State: "succeeded", Phase: phase,
		Actor: actorOrDefault(value.Actor), RemoteConfirmed: true, ReadbackConfirmed: value.ReadbackConfirmed, ObjectID: value.AccountID,
		ObjectName: &name, GroupNames: onboardingLocalGroupNames(localGroups), FieldName: &field,
		After: map[string]any{"name": value.AccountName, "group_ids": onboardingLocalGroupIDs(localGroups), "schedulable": value.Schedulable,
			"rate_multiplier": value.Multiplier, "concurrency": value.Concurrency, "priority": value.Priority}, Writeback: true,
	}
	if err := insertAccountOperation(ctx, tx, operation); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM onboarding_pending WHERE operation_id=?`, value.OperationID); err != nil {
		return err
	}
	return tx.Commit()
}

func onboardingBindingDescription(readbackConfirmed bool) string {
	if readbackConfirmed {
		return "账号添加后远程读回确认"
	}
	return "账号添加响应已接受，未启用写后确认"
}

func onboardingLocalGroupIDs(groups []LocalOnboardingGroup) []string {
	result := make([]string, 0, len(groups))
	for _, group := range groups {
		result = append(result, group.ID)
	}
	return result
}

func onboardingLocalGroupNames(groups []LocalOnboardingGroup) []string {
	result := make([]string, 0, len(groups))
	for _, group := range groups {
		result = append(result, group.Name)
	}
	return result
}

type onboardingKey struct{ id, name string }

func (s *Store) onboardingKeys(ctx context.Context, host string) (map[string]onboardingKey, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key_id,name,upstream_group,status FROM upstream_keys
		WHERE host=? AND lower(name) NOT LIKE 'console-probe-%' ORDER BY key_id`, host)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]onboardingKey{}
	for rows.Next() {
		var id, name string
		var group, status sql.NullString
		if err := rows.Scan(&id, &name, &group, &status); err != nil {
			return nil, err
		}
		if group.Valid && activeStatus(status.String) {
			if _, exists := result[group.String]; !exists {
				result[group.String] = onboardingKey{id: id, name: name}
			}
		}
	}
	return result, rows.Err()
}

func (s *Store) ProtectedUpstreamKeyIDs(ctx context.Context, host string) ([]string, error) {
	if err := s.ensureUpstreamIdentities(ctx); err != nil {
		return nil, err
	}
	upstreamID, hosts, err := upstreamIdentityHostsForQueryer(ctx, s.db, host)
	if err != nil {
		return nil, err
	}
	placeholders, hostArguments := sqlStringArguments(hosts)
	arguments := []any{upstreamID}
	arguments = append(arguments, hostArguments...)
	arguments = append(arguments, hostArguments...)
	query := `SELECT upstream_key_id FROM binding_identities WHERE upstream_id=?
		UNION SELECT upstream_key_id FROM bindings WHERE upstream_host IN (` + placeholders + `)
		UNION SELECT upstream_key_id FROM onboarding_pending WHERE upstream_host IN (` + placeholders + `)
		ORDER BY upstream_key_id`
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []string{}
	for rows.Next() {
		var keyID string
		if err := rows.Scan(&keyID); err != nil {
			return nil, err
		}
		if keyID = strings.TrimSpace(keyID); keyID != "" {
			result = append(result, keyID)
		}
	}
	return result, rows.Err()
}

func (s *Store) UpstreamKeyProtected(ctx context.Context, host, keyID string) (bool, error) {
	host, keyID = canonicalHost(host), strings.TrimSpace(keyID)
	if host == "" || keyID == "" {
		return false, errors.New("上游 Host 和 Key ID 不能为空")
	}
	return upstreamKeyProtectedForQueryer(ctx, s.db, host, keyID)
}

func upstreamKeyProtectedForQueryer(
	ctx context.Context,
	queryer accountDeleteScopeQueryer,
	host, keyID string,
) (bool, error) {
	var protected int
	err := queryer.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM upstream_identity_hosts selected
		JOIN binding_identities bi ON bi.upstream_id=selected.upstream_id
		WHERE selected.host=? AND bi.upstream_key_id=?
		UNION ALL
		SELECT 1 FROM upstream_identity_hosts selected
		JOIN upstream_identity_hosts aliases ON aliases.upstream_id=selected.upstream_id
		JOIN bindings b ON b.upstream_host=aliases.host
		WHERE selected.host=? AND b.upstream_key_id=?
		UNION ALL
		SELECT 1 FROM upstream_identity_hosts selected
		JOIN upstream_identity_hosts aliases ON aliases.upstream_id=selected.upstream_id
		JOIN onboarding_pending pending ON pending.upstream_host=aliases.host
		WHERE selected.host=? AND pending.upstream_key_id=?
	)`, host, keyID, host, keyID, host, keyID).Scan(&protected)
	return protected == 1, err
}

func onboardingGroupReason(group UpstreamGroup) string {
	switch {
	case group.Status == nil:
		return "上游分组状态不可读"
	case !activeStatus(*group.Status):
		return "上游分组当前未启用"
	case group.GroupID == nil:
		return "上游分组缺少稳定 ID"
	case strings.TrimSpace(group.Name) == "":
		return "上游分组名称为空"
	case group.EffectiveRate == nil || strings.TrimSpace(*group.EffectiveRate) == "":
		return "上游分组倍率不可读"
	default:
		return "上游分组当前不可创建 Key"
	}
}

func recommendedBinding(name string, description *string) string {
	text := strings.ToLower(name + " " + pointerValue(description))
	for _, item := range []struct{ marker, value string }{
		{"codex", "codex"}, {"gpt pro", "pro"}, {"pro", "pro"}, {"gemini", "Gemini"},
		{"image", "生图"}, {"claude", "A-CCMAX"},
	} {
		if strings.Contains(text, item.marker) {
			return item.value
		}
	}
	return "待确认"
}

func safeOnboardingReason(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 500 {
		value = value[:500]
	}
	return value
}

func actorOrDefault(value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return "console"
}
