package accountworkbench

import (
	"encoding/json"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/decimalutil"
	"strconv"
	"strings"
	"time"
)

// TemplateConfig is a private application payload and a public configuration
// summary. Only explicit configuration fields may enter it, never identity.
type TemplateConfig struct {
	Concurrency                 int64                      `json:"concurrency"`
	Priority                    int64                      `json:"priority"`
	RateMultiplier              string                     `json:"rate_multiplier"`
	LoadFactor                  *string                    `json:"load_factor"`
	ProxyID                     *string                    `json:"proxy_id"`
	GroupIDs                    []string                   `json:"group_ids"`
	AutoPauseOnExpired          bool                       `json:"auto_pause_on_expired"`
	UpstreamBillingProbeEnabled *bool                      `json:"upstream_billing_probe_enabled,omitempty"`
	ConfirmMixedChannelRisk     *bool                      `json:"confirm_mixed_channel_risk,omitempty"`
	ExpiresAt                   *int64                     `json:"expires_at"`
	Notes                       string                     `json:"notes"`
	CredentialExtras            map[string]json.RawMessage `json:"credential_extras"`
	Extra                       map[string]json.RawMessage `json:"extra"`
}

var credentialConfigFields = []string{"model_mapping", "compact_model_mapping", "intercept_warmup_requests", "user_agent", "plan_type"}
var extraConfigFields = []string{"openai_long_context_billing_enabled", "openai_oauth_responses_websockets_v2_enabled", "openai_oauth_responses_websockets_v2_mode", "openai_responses_flatten_namespaces", "openai_ws_allow_store_recovery", "openai_ws_enabled", "openai_ws_force_http", "model_rate_limits", "codex_fingerprint_mode"}

func ExtractConfig(account map[string]any) (TemplateConfig, error) {
	c := TemplateConfig{Concurrency: 1, Priority: 50, RateMultiplier: "1", GroupIDs: []string{}, AutoPauseOnExpired: true, CredentialExtras: map[string]json.RawMessage{}, Extra: map[string]json.RawMessage{}}
	if text(account["platform"]) != "openai" || text(account["type"]) != "oauth" {
		return c, errors.New("仅支持读取 OpenAI OAuth 账号配置")
	}
	for key, destination := range map[string]*int64{"concurrency": &c.Concurrency, "priority": &c.Priority} {
		if raw, exists := account[key]; exists {
			value, err := strconv.ParseInt(text(raw), 10, 64)
			if err != nil || value < 0 {
				return c, errors.New("来源并发或优先级无效")
			}
			*destination = value
		}
	}
	for key, destination := range map[string]**string{"rate_multiplier": nil, "load_factor": &c.LoadFactor} {
		if raw, exists := account[key]; exists && raw != nil {
			value := text(raw)
			if value == "" {
				return c, errors.New("来源倍率或负载因子无效")
			}
			if destination == nil {
				c.RateMultiplier = value
			} else {
				*destination = &value
			}
		}
	}
	if value := text(account["proxy_id"]); value != "" {
		if _, err := strconv.ParseUint(value, 10, 64); err != nil {
			return c, errors.New("来源代理 ID 无效")
		}
		c.ProxyID = &value
	}
	if value, exists := account["auto_pause_on_expired"]; exists {
		v, ok := value.(bool)
		if !ok {
			return c, errors.New("来源到期设置无效")
		}
		c.AutoPauseOnExpired = v
	}
	for key, destination := range map[string]**bool{"upstream_billing_probe_enabled": &c.UpstreamBillingProbeEnabled, "confirm_mixed_channel_risk": &c.ConfirmMixedChannelRisk} {
		if value, exists := account[key]; exists && value != nil {
			v, ok := value.(bool)
			if !ok {
				return c, errors.New("来源计费探测或混合渠道设置无效")
			}
			*destination = &v
		}
	}
	if value := text(account["expires_at"]); value != "" {
		v, err := accountExpirySeconds(value)
		if err != nil || v < 0 {
			return c, errors.New("来源到期时间无效")
		}
		c.ExpiresAt = &v
	}
	if value, exists := account["notes"]; exists && value != nil {
		var ok bool
		c.Notes, ok = value.(string)
		if !ok || len(c.Notes) > 16384 {
			return c, errors.New("来源备注无效")
		}
	}
	for _, group := range publicAccount(account).Groups {
		if _, err := strconv.ParseUint(group.ID, 10, 64); err != nil {
			return c, errors.New("来源分组 ID 无效")
		}
		c.GroupIDs = append(c.GroupIDs, group.ID)
	}
	for _, field := range []struct {
		source string
		keys   []string
		dest   map[string]json.RawMessage
	}{{"credentials", credentialConfigFields, c.CredentialExtras}, {"extra", extraConfigFields, c.Extra}} {
		for _, key := range field.keys {
			if value, exists := object(account[field.source])[key]; exists {
				raw, err := json.Marshal(value)
				if err != nil {
					return c, err
				}
				field.dest[key] = raw
			}
		}
	}
	return c, c.Validate()
}

func (c TemplateConfig) Validate() error {
	if len(c.Notes) > 16384 || strings.ContainsRune(c.Notes, 0) {
		return errors.New("模板备注无效")
	}
	if c.Concurrency < 0 || c.Priority < 0 {
		return errors.New("并发或优先级无效")
	}
	if c.ExpiresAt != nil && *c.ExpiresAt < 0 {
		return errors.New("账号到期时间无效")
	}
	ids := append([]string(nil), c.GroupIDs...)
	if c.ProxyID != nil {
		ids = append(ids, *c.ProxyID)
	}
	for _, id := range ids {
		if id == "" || strings.Trim(id, "0123456789") != "" {
			return errors.New("代理或分组 ID 无效")
		}
		if n, err := strconv.ParseUint(id, 10, 63); err != nil || n == 0 {
			return errors.New("代理或分组 ID 无效")
		}
	}
	for _, value := range []string{c.RateMultiplier} {
		v, ok := decimalutil.Parse(value)
		if !ok || v.Sign() < 0 {
			return errors.New("计费倍率无效")
		}
	}
	if c.LoadFactor != nil {
		v, ok := decimalutil.Parse(*c.LoadFactor)
		if !ok || v.Sign() <= 0 {
			return errors.New("负载因子无效")
		}
	}
	for key, raw := range c.CredentialExtras {
		switch key {
		case "model_mapping", "compact_model_mapping":
			var v map[string]string
			if json.Unmarshal(raw, &v) != nil || v == nil || len(v) > 1000 {
				return errors.New("模型映射无效")
			}
		case "intercept_warmup_requests":
			var v bool
			if json.Unmarshal(raw, &v) != nil {
				return errors.New("预热设置无效")
			}
		case "plan_type", "user_agent":
			var v string
			if json.Unmarshal(raw, &v) != nil || len(v) > 2048 || strings.ContainsAny(v, "\r\n\x00") {
				return errors.New("订阅档位或客户端标识无效")
			}
		default:
			return errors.New("来源包含不允许复制的凭据字段")
		}
	}
	for key, raw := range c.Extra {
		switch key {
		case "codex_fingerprint_mode":
			var v string
			if json.Unmarshal(raw, &v) != nil {
				return errors.New("Codex 指纹模式无效")
			}
			switch v {
			case "off", "device", "session", "full":
			default:
				return errors.New("Codex 指纹模式无效")
			}
		case "openai_oauth_responses_websockets_v2_mode":
			var v string
			if json.Unmarshal(raw, &v) != nil || len(v) > 128 {
				return errors.New("WebSocket 模式无效")
			}
		case "model_rate_limits":
			if len(raw) > 100000 || !json.Valid(raw) {
				return errors.New("模型限速配置无效")
			}
		case "openai_long_context_billing_enabled", "openai_oauth_responses_websockets_v2_enabled", "openai_responses_flatten_namespaces", "openai_ws_allow_store_recovery", "openai_ws_enabled", "openai_ws_force_http":
			var v bool
			if json.Unmarshal(raw, &v) != nil {
				return errors.New("附加开关无效")
			}
		default:
			return errors.New("来源包含不允许复制的附加字段")
		}
	}
	return nil
}

func accountExpirySeconds(value string) (int64, error) {
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		return seconds, nil
	}
	stamp, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return 0, err
	}
	return stamp.Unix(), nil
}
