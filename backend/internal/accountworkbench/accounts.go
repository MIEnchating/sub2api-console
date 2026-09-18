package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"strings"
)

type Group struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Account struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Email          string            `json:"email"`
	Status         string            `json:"status"`
	Schedulable    bool              `json:"schedulable"`
	Plan           string            `json:"plan"`
	Groups         []Group           `json:"groups"`
	ProxyName      string            `json:"proxy_name"`
	Concurrency    string            `json:"concurrency"`
	LoadFactor     string            `json:"load_factor"`
	RateMultiplier string            `json:"rate_multiplier"`
	ModelMapping   map[string]string `json:"model_mapping"`
	Fingerprint    string            `json:"fingerprint"`
}

func (s *Service) Accounts(ctx context.Context) ([]Account, error) {
	ctx, err := targetguard.Capture(ctx, s.private)
	if err != nil {
		return nil, err
	}
	ctx, err = targetguard.Pin(ctx, s.private)
	if err != nil {
		return nil, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return nil, err
	}
	client, err := s.client(target)
	if err != nil {
		return nil, err
	}
	rows, err := client.Accounts(ctx)
	if err != nil {
		return nil, errors.New("线上账号读取失败，请检查站点连接后重试")
	}
	result := make([]Account, 0, len(rows))
	seen := map[string]bool{}
	for _, row := range rows {
		if text(row["platform"]) != "openai" || text(row["type"]) != "oauth" {
			continue
		}
		account := publicAccount(row)
		if account.ID == "" || seen[account.ID] {
			return nil, errors.New("线上账号缺少稳定 ID 或存在重复，请同步站点后重试")
		}
		seen[account.ID] = true
		result = append(result, account)
	}
	if _, err = targetguard.Pin(ctx, s.private); err != nil {
		return nil, err
	}
	return result, nil
}

func publicAccount(row map[string]any) Account {
	credentials := object(row["credentials"])
	extra := object(row["extra"])
	result := Account{ID: text(row["id"]), Name: text(row["name"]), Email: text(credentials["email"]), Status: text(row["status"]), Schedulable: row["schedulable"] != false, Plan: text(credentials["plan_type"]), Groups: []Group{}, ProxyName: text(object(row["proxy"])["name"]), Concurrency: text(row["concurrency"]), LoadFactor: text(row["load_factor"]), RateMultiplier: text(row["rate_multiplier"]), ModelMapping: map[string]string{}, Fingerprint: text(extra["codex_fingerprint_mode"])}
	if result.Plan == "" {
		result.Plan = text(extra["plan_type"])
	}
	if result.ProxyName == "" && text(row["proxy_id"]) != "" {
		result.ProxyName = "代理 #" + text(row["proxy_id"])
	}
	for key, value := range object(credentials["model_mapping"]) {
		if label, ok := value.(string); ok {
			result.ModelMapping[key] = label
		}
	}
	groups, _ := row["groups"].([]any)
	if len(groups) == 0 {
		if entries, ok := row["account_groups"].([]any); ok {
			for _, entry := range entries {
				group := object(object(entry)["group"])
				if text(group["id"]) == "" {
					group = map[string]any{"id": object(entry)["group_id"]}
				}
				groups = append(groups, group)
			}
		}
	}
	seen := map[string]bool{}
	for _, raw := range groups {
		group := object(raw)
		id := text(group["id"])
		if id != "" && !seen[id] {
			seen[id] = true
			result.Groups = append(result.Groups, Group{ID: id, Name: text(group["name"])})
		}
	}
	if len(result.Groups) == 0 {
		if ids, ok := row["group_ids"].([]any); ok {
			for _, raw := range ids {
				if id := text(raw); id != "" && !seen[id] {
					seen[id] = true
					result.Groups = append(result.Groups, Group{ID: id})
				}
			}
		}
	}
	return result
}

func object(value any) map[string]any { result, _ := value.(map[string]any); return result }
func text(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case json.Number:
		return v.String()
	case int:
		return fmt.Sprint(v)
	case int64:
		return fmt.Sprint(v)
	default:
		return ""
	}
}
