package accountworkbench

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

var transferableAccountFields = []string{"notes", "proxy_id", "concurrency", "priority", "rate_multiplier", "load_factor", "group_ids", "expires_at", "auto_pause_on_expired", "upstream_billing_probe_enabled", "confirm_mixed_channel_risk"}
var transferableCredentialFields = []string{"model_mapping", "compact_model_mapping", "intercept_warmup_requests", "user_agent"}
var transferableExtraFields = []string{"openai_long_context_billing_enabled", "openai_oauth_responses_websockets_v2_enabled", "openai_oauth_responses_websockets_v2_mode", "openai_responses_flatten_namespaces", "openai_ws_allow_store_recovery", "openai_ws_enabled", "openai_ws_force_http", "model_rate_limits"}

// ExtractTemplate copies account configuration, never source identity or OAuth
// credentials. It is only valid for an OpenAI OAuth source account.
func ExtractTemplate(account map[string]any) (TemplateConfig, error) {
	if inputText(account["platform"]) != "openai" || inputText(account["type"]) != "oauth" {
		return nil, errors.New("配置模板来源必须是 OpenAI OAuth 账号")
	}
	config := TemplateConfig{}
	for _, key := range transferableAccountFields {
		if value, exists := account[key]; exists {
			if key == "rate_multiplier" || key == "load_factor" {
				if _, lossy := value.(float64); lossy {
					return nil, errors.New("来源倍率必须保留原始十进制精度")
				}
			}
			raw, err := json.Marshal(value)
			if err != nil {
				return nil, fmt.Errorf("来源配置字段 %s 无效", key)
			}
			config[key] = raw
		}
	}
	if _, exists := config["group_ids"]; !exists {
		ids := []any{}
		appendGroupIDs := func(raw any, key string) {
			switch groups := raw.(type) {
			case []any:
				for _, group := range groups {
					if id := inputObject(group)[key]; id != nil {
						ids = append(ids, id)
					}
				}
			case []map[string]any:
				for _, group := range groups {
					if id := group[key]; id != nil {
						ids = append(ids, id)
					}
				}
			}
		}
		appendGroupIDs(account["groups"], "id")
		if len(ids) == 0 {
			appendGroupIDs(account["account_groups"], "group_id")
		}
		config["group_ids"], _ = json.Marshal(ids)
	}
	for _, field := range []struct {
		source, destination string
		keys                []string
	}{
		{"credentials", "credential_extras", transferableCredentialFields}, {"extra", "extra", transferableExtraFields},
	} {
		fields := inputObject(account[field.source])
		filtered := map[string]any{}
		for _, key := range field.keys {
			if value, exists := fields[key]; exists {
				filtered[key] = cloneInputValue(value)
			}
		}
		if len(filtered) != 0 {
			raw, err := json.Marshal(filtered)
			if err != nil {
				return nil, errors.New("来源附加配置无法读取")
			}
			config[field.destination] = raw
		}
	}
	return configstore.NormalizeWorkbenchTemplateConfig(config)
}

// MatchTemplate applies an explicit selection or a deterministic plan/domain
// match. Conditional templates win over fallback templates; ties use stable ID.
func MatchTemplate(item InputItem, templates []configstore.WorkbenchTemplate, templateID string) (*configstore.WorkbenchTemplate, error) {
	templateID = strings.TrimSpace(templateID)
	if templateID != "" {
		for _, template := range templates {
			if template.ID != templateID {
				continue
			}
			// An explicit selection is an operator override. Plan/domain rules
			// drive automatic matching only; selecting a template intentionally
			// permits accounts whose metadata is incomplete or differs.
			copy := cloneWorkbenchTemplate(template)
			return &copy, nil
		}
		return nil, errors.New("所选账号模板不存在或不属于当前管理目标，请刷新模板列表")
	}
	matches := []configstore.WorkbenchTemplate{}
	fallbacks := []configstore.WorkbenchTemplate{}
	for _, template := range templates {
		if !templateMatches(item, template.Match) {
			continue
		}
		if strings.TrimSpace(template.Match.PlanType) == "" && strings.TrimSpace(template.Match.EmailDomain) == "" {
			fallbacks = append(fallbacks, template)
		} else {
			matches = append(matches, template)
		}
	}
	if len(matches) == 0 {
		matches = fallbacks
	}
	if len(matches) == 0 {
		return nil, nil
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Priority != matches[j].Priority {
			return matches[i].Priority > matches[j].Priority
		}
		return matches[i].ID < matches[j].ID
	})
	copy := cloneWorkbenchTemplate(matches[0])
	return &copy, nil
}

func templateMatches(item InputItem, match configstore.WorkbenchTemplateMatch) bool {
	if plan := strings.TrimSpace(match.PlanType); plan != "" && !strings.EqualFold(plan, item.PlanType) {
		return false
	}
	if domain := strings.TrimSpace(match.EmailDomain); domain != "" {
		_, actual, exists := strings.Cut(item.Email, "@")
		if !exists || !strings.EqualFold(domain, actual) {
			return false
		}
	}
	return true
}

func cloneWorkbenchTemplate(template configstore.WorkbenchTemplate) configstore.WorkbenchTemplate {
	copy := template
	copy.Config = make(TemplateConfig, len(template.Config))
	for key, raw := range template.Config {
		copy.Config[key] = append(json.RawMessage(nil), raw...)
	}
	return copy
}

// ApplyTemplate produces a private admin payload. It does not activate the
// account: the service must stage, verify, and read back the remote mutation.
func ApplyTemplate(item InputItem, template *configstore.WorkbenchTemplate) (map[string]any, error) {
	if inputText(item.Credentials["access_token"]) == "" && inputText(item.Credentials["refresh_token"]) == "" {
		return nil, errors.New("账号缺少可用 OAuth 凭据")
	}
	config := TemplateConfig{}
	if template != nil {
		config = template.Config
	}
	normalized, err := configstore.NormalizeWorkbenchTemplateConfig(config)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(item.Name)
	if name == "" {
		name = strings.TrimSpace(item.Email)
	}
	if name == "" {
		name = fmt.Sprintf("OpenAI OAuth %d", item.Index+1)
	}
	payload := map[string]any{"name": name, "platform": "openai", "type": "oauth", "credentials": cloneInputMap(item.Credentials), "concurrency": json.Number("1"), "priority": json.Number("50"), "rate_multiplier": json.Number("1"), "group_ids": []any{}, "auto_pause_on_expired": true}
	if template != nil {
		payload["concurrency"] = json.Number("10")
		payload["priority"] = json.Number("0")
	}
	for key, raw := range normalized {
		value, err := decodeInputJSON(string(raw))
		if err != nil {
			return nil, err
		}
		if key == "credential_extras" {
			credentials := payload["credentials"].(map[string]any)
			for field, extra := range inputObject(value) {
				credentials[field] = extra
			}
			continue
		}
		payload[key] = value
	}
	return payload, nil
}
