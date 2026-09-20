package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type UpstreamAllocationSetting struct {
	Revision      string `json:"revision"`
	TargetID      string `json:"target_id"`
	UpstreamID    string `json:"upstream_id"`
	Override      *bool  `json:"override"`
	Selected      bool   `json:"selected"`
	Effective     bool   `json:"effective"`
	GlobalEnabled bool   `json:"global_enabled"`
	Source        string `json:"source"`
}

type UpstreamAllocationUpdate struct {
	Override           *bool  `json:"override"`
	ExpectedRevision   string `json:"expected_revision"`
	ExpectedUpstreamID string `json:"expected_upstream_id"`
}

func allocationTarget(ctx context.Context, q policyQueryer, kind, id string) (string, error) {
	if id == "" || strings.TrimSpace(id) != id {
		return "", errors.New("必须使用有效的稳定 ID")
	}
	if kind == "accounts" {
		if !positiveNumericID(id) {
			return "", errors.New("账号必须使用有效的稳定 ID")
		}
		items, err := routingCapacityInventoryForAccount(ctx, q, &id)
		if err != nil {
			return "", err
		}
		item, ok := items[id]
		if !ok {
			return "", sql.ErrNoRows
		}
		if item.UpstreamType == nil || !strings.EqualFold(*item.UpstreamType, "sub2api") || item.UpstreamID == "" {
			return "", errors.New("仅支持具有稳定上游绑定的 Sub2API 账号，请先同步账号")
		}
		return item.UpstreamID, nil
	}
	if kind != "upstreams" {
		return "", errors.New("共享并发设置目标类型无效")
	}
	var platform string
	if err := q.QueryRowContext(ctx, `SELECT u.upstream_type FROM upstream_identity_hosts h JOIN upstreams u ON u.host=h.host WHERE h.upstream_id=? AND h.is_primary=1`, id).Scan(&platform); err != nil {
		return "", err
	}
	if !strings.EqualFold(platform, "sub2api") {
		return "", errors.New("仅 Sub2API 上游支持共享并发分配")
	}
	return id, nil
}

func allocationSetting(document map[string]any, kind, id, upstreamID string) (UpstreamAllocationSetting, error) {
	raw, exists := document["upstream_concurrency"]
	section, ok := raw.(map[string]any)
	if exists && !ok {
		return UpstreamAllocationSetting{}, errors.New("共享并发策略必须是对象")
	}
	scope, err := ParseUpstreamAllocationScope(section)
	if err != nil {
		return UpstreamAllocationSetting{}, err
	}
	revision, err := policyRevision(document)
	if err != nil {
		return UpstreamAllocationSetting{}, err
	}
	accountID := ""
	overrides := scope.UpstreamOverrides
	if kind == "accounts" {
		accountID, overrides = id, scope.AccountOverrides
	}
	selected, source := scope.Selected(accountID, upstreamID)
	result := UpstreamAllocationSetting{Revision: revision, TargetID: id, UpstreamID: upstreamID, Selected: selected, Effective: scope.Enabled && selected, GlobalEnabled: scope.Enabled, Source: source}
	if value, exists := overrides[id]; exists {
		result.Override = &value
	}
	return result, nil
}

func (s *Store) UpstreamAllocationSetting(ctx context.Context, kind, id string) (UpstreamAllocationSetting, error) {
	upstreamID, err := allocationTarget(ctx, s.db, kind, id)
	if err != nil {
		return UpstreamAllocationSetting{}, err
	}
	document, err := s.readPolicyDocument(ctx, s.db, "control-plane")
	if err != nil {
		return UpstreamAllocationSetting{}, err
	}
	if document == nil {
		return UpstreamAllocationSetting{}, errors.New("控制面策略记录不存在")
	}
	return allocationSetting(document, kind, id, upstreamID)
}

// SetUpstreamAllocationSetting patches one stable target inside the same policy
// document consumed by the scheduler. Version and binding checks are atomic.
func (s *Store) SetUpstreamAllocationSetting(ctx context.Context, kind, id string, update UpstreamAllocationUpdate, actor string) (UpstreamAllocationSetting, error) {
	if strings.TrimSpace(update.ExpectedRevision) == "" {
		return UpstreamAllocationSetting{}, errors.New("请刷新共享并发设置后重试")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UpstreamAllocationSetting{}, err
	}
	defer tx.Rollback()
	upstreamID, err := allocationTarget(ctx, tx, kind, id)
	if err != nil {
		return UpstreamAllocationSetting{}, err
	}
	if kind == "accounts" && update.ExpectedUpstreamID != upstreamID {
		return UpstreamAllocationSetting{}, errors.New("账号上游绑定已变化，请刷新后重试")
	}
	document, err := s.readPolicyDocument(ctx, tx, "control-plane")
	if err != nil {
		return UpstreamAllocationSetting{}, err
	}
	if document == nil {
		return UpstreamAllocationSetting{}, errors.New("控制面策略记录不存在")
	}
	current, err := allocationSetting(document, kind, id, upstreamID)
	if err != nil {
		return UpstreamAllocationSetting{}, err
	}
	if current.Revision != update.ExpectedRevision {
		return UpstreamAllocationSetting{}, ErrPolicyRevisionConflict
	}
	section, _ := document["upstream_concurrency"].(map[string]any)
	section = copyObject(section)
	key := "upstream_overrides"
	if kind == "accounts" {
		key = "account_overrides"
	}
	values, _ := section[key].(map[string]any)
	values = copyObject(values)
	if update.Override == nil {
		delete(values, id)
	} else {
		values[id] = *update.Override
	}
	section[key], document["upstream_concurrency"] = values, section
	result, err := allocationSetting(document, kind, id, upstreamID)
	if err != nil {
		return UpstreamAllocationSetting{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.writePolicyDocument(ctx, tx, "control-plane", document, now); err != nil {
		return UpstreamAllocationSetting{}, err
	}
	payload, err := json.Marshal(map[string]any{"actor": actor, "target_kind": kind, "target_id": id, "upstream_id": upstreamID, "override": update.Override})
	if err != nil {
		return UpstreamAllocationSetting{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO runtime_events(source_id,event_type,created_at,status,summary,payload_json) VALUES((SELECT COALESCE(MIN(source_id),0)-1 FROM runtime_events WHERE source_id<0),?,?,?,?,?)`, "policy.upstream_allocation", now, "succeeded", "上游共享并发分配设置已更新", string(payload)); err != nil {
		return UpstreamAllocationSetting{}, err
	}
	if err := tx.Commit(); err != nil {
		return UpstreamAllocationSetting{}, err
	}
	return result, nil
}
