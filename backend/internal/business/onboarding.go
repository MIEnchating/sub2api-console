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
	ID   string `json:"id"`
	Name string `json:"name"`
}

type PendingOnboarding struct {
	OperationID       string
	UpstreamHost      string
	UpstreamType      string
	BaseURL           string
	UpstreamKeyID     string
	UpstreamKeyName   *string
	UpstreamGroupID   string
	UpstreamGroupName string
	LocalGroupID      string
	LocalGroupName    string
	Multiplier        string
	Reason            string
	CreatedAt         string
	UpdatedAt         string
}

type OnboardingProjection struct {
	OperationID       string
	AccountID         string
	AccountName       string
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
	err := s.db.QueryRowContext(ctx, `SELECT remote_id,name FROM local_groups WHERE remote_id=?`, groupID).Scan(&result.ID, &result.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return LocalOnboardingGroup{}, errors.New("目标本地分组不存在")
	}
	return result, err
}

func (s *Store) PendingOnboarding(ctx context.Context, host, upstreamGroupID, localGroupID, multiplier string) (*PendingOnboarding, error) {
	var value PendingOnboarding
	var keyName sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT operation_id,upstream_host,upstream_type,upstream_key_id,upstream_key_name,
		upstream_group_id,upstream_group_name,local_group_id,local_group_name,multiplier,reason,created_at,updated_at
		FROM onboarding_pending WHERE upstream_host=? AND upstream_group_id=? AND local_group_id=? AND multiplier=?
		ORDER BY updated_at DESC LIMIT 1`, canonicalHost(host), strings.TrimSpace(upstreamGroupID), strings.TrimSpace(localGroupID), strings.TrimSpace(multiplier)).Scan(
		&value.OperationID, &value.UpstreamHost, &value.UpstreamType, &value.UpstreamKeyID, &keyName,
		&value.UpstreamGroupID, &value.UpstreamGroupName, &value.LocalGroupID, &value.LocalGroupName,
		&value.Multiplier, &value.Reason, &value.CreatedAt, &value.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	value.UpstreamKeyName = nullString(keyName)
	return &value, nil
}

func (s *Store) SavePendingOnboarding(ctx context.Context, value PendingOnboarding) error {
	if strings.TrimSpace(value.OperationID) == "" || canonicalHost(value.UpstreamHost) == "" ||
		strings.TrimSpace(value.UpstreamKeyID) == "" || strings.TrimSpace(value.UpstreamGroupID) == "" ||
		!positiveNumericID(value.LocalGroupID) || normalizePositiveDecimal(value.Multiplier) == nil {
		return errors.New("待续开户记录字段不完整")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if value.CreatedAt == "" {
		value.CreatedAt = now
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO onboarding_pending(operation_id,upstream_host,upstream_type,
		upstream_key_id,upstream_key_name,upstream_group_id,upstream_group_name,local_group_id,local_group_name,
		multiplier,reason,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(operation_id) DO UPDATE SET reason=excluded.reason,updated_at=excluded.updated_at`,
		value.OperationID, canonicalHost(value.UpstreamHost), strings.TrimSpace(value.UpstreamType), value.UpstreamKeyID,
		managementNullableString(value.UpstreamKeyName), value.UpstreamGroupID, value.UpstreamGroupName, value.LocalGroupID,
		value.LocalGroupName, value.Multiplier, safeOnboardingReason(value.Reason), value.CreatedAt, now)
	return err
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
	if baseURL := strings.TrimSpace(value.BaseURL); baseURL != "" {
		metadataValues["base_url"] = baseURL
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

func onboardingBindingDescription(readbackConfirmed bool) string {
	if readbackConfirmed {
		return "账号添加后远程读回确认"
	}
	return "账号添加响应已接受，未启用写后确认"
}

type onboardingKey struct{ id, name string }

func (s *Store) onboardingKeys(ctx context.Context, host string) (map[string]onboardingKey, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key_id,name,upstream_group,status FROM upstream_keys WHERE host=? ORDER BY key_id`, host)
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
