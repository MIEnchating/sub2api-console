package uptimekuma

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

const claudeTemplateID = "cccccccccccccccccccccccccccccccccccccccccccccccc"

func applyTemplateMonitoring(o *MonitorOptions, m *configstore.KumaTemplateMonitoring) {
	o.Timeout = m.Timeout
	o.RetryInterval = m.RetryInterval
	o.MaxRetries = m.MaxRetries
	o.MaxRedirects = m.MaxRedirects
	o.AcceptedStatusCodes = append([]string(nil), m.AcceptedStatusCodes...)
	o.IgnoreTLS = m.IgnoreTLS
	o.UpsideDown = m.UpsideDown
	o.Hostname = m.Hostname
	o.Port = m.Port
	o.Keyword = m.Keyword
	o.DNSRecordType = m.DNSRecordType
	o.DNSResolver = m.DNSResolver
}

// Standard profiles follow the official Messages, Chat Completions and Responses APIs.
// The CLI profile additionally follows Sub2API migration 129 and claude_code_validator.go.
func prepareTemplateProfile(item *configstore.KumaTemplate) error {
	if item.RequestProfile == "" {
		return nil
	}
	if item.Monitoring == nil || !isHTTP(item.Monitoring.Type) {
		return failure("kuma_invalid_template_profile", "内置 API 请求仅适用于 HTTP 或关键字监控", 422)
	}
	if item.Model == "" || len(item.Model) > 200 || strings.ContainsAny(item.Model, "\r\n") {
		return failure("kuma_invalid_template_model", "请输入有效模型名称，最多 200 个字符", 422)
	}
	headers := map[string]string{}
	if item.Headers != "" {
		if err := json.Unmarshal([]byte(item.Headers), &headers); err != nil || headers == nil {
			return failure("kuma_invalid_template_headers", "请求头须为 JSON 对象，值为字符串", 422)
		}
	}
	// Rebuild protocol-owned headers on mode changes, retaining private custom headers.
	for key := range headers {
		switch strings.ToLower(key) {
		case "content-type", "user-agent", "x-app", "anthropic-version", "anthropic-beta", "anthropic-dangerous-direct-browser-access":
			delete(headers, key)
		}
	}
	headers["Content-Type"] = "application/json"
	body := map[string]any{"model": item.Model, "stream": false}
	messages := []map[string]string{{"role": "user", "content": "Reply with OK."}}
	switch item.RequestProfile {
	case "claude-cli", "claude-messages":
		headers["anthropic-version"] = "2023-06-01"
		body["max_tokens"] = 16
		body["messages"] = messages
		if item.RequestProfile == "claude-cli" {
			headers["User-Agent"] = "claude-cli/2.1.114 (external, sdk-cli)"
			headers["X-App"] = "cli"
			headers["anthropic-beta"] = "claude-code-20250219,interleaved-thinking-2025-05-14,context-management-2025-06-27,prompt-caching-scope-2026-01-05,advisor-tool-2026-03-01"
			headers["anthropic-dangerous-direct-browser-access"] = "true"
			body["system"] = []map[string]string{{"type": "text", "text": "You are Claude Code, Anthropic's official CLI for Claude."}}
			body["metadata"] = map[string]string{"user_id": "user_" + strings.Repeat("0", 64) + "_account_00000000-0000-0000-0000-000000000000_session_00000000-0000-0000-0000-000000000000"}
		}
	case "openai-chat":
		body["messages"] = messages
		body["max_completion_tokens"] = 16
		body["store"] = false
	case "openai-responses":
		body["input"] = "Reply with OK."
		body["max_output_tokens"] = 16
		body["store"] = false
	default:
		return failure("kuma_invalid_template_profile", "请选择支持的请求模式", 422)
	}
	h, err := json.Marshal(headers)
	if err != nil {
		return err
	}
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	item.Method = "POST"
	item.Headers = string(h)
	item.Body = string(b)
	return nil
}

func SeedTemplates(ctx context.Context, store *configstore.Store) error {
	for _, seed := range []struct{ id, name, profile, model string }{
		{claudeTemplateID, "Claude CLI 请求", "claude-cli", "claude-sonnet-4-6"},
		{strings.Repeat("d", 48), "Claude Messages 请求", "claude-messages", "claude-sonnet-4-6"},
		{strings.Repeat("e", 48), "OpenAI Chat Completions 请求", "openai-chat", "gpt-4.1-mini"},
		{strings.Repeat("f", 48), "OpenAI Responses 请求", "openai-responses", "gpt-4.1-mini"},
	} {
		item := configstore.KumaTemplate{ID: seed.id, Name: seed.name, Method: "POST", AuthMethod: "none", RequestProfile: seed.profile, Model: seed.model, Monitoring: &configstore.KumaTemplateMonitoring{Type: "http", Interval: 300, Timeout: 120, RetryInterval: 60, MaxRetries: 0, MaxRedirects: 10, AcceptedStatusCodes: []string{"200-299"}, DNSRecordType: "A", DNSResolver: "1.1.1.1", Port: 443}}
		if err := prepareTemplateProfile(&item); err != nil {
			return err
		}
		if err := store.SeedKumaTemplate(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

// TemplatePreset generates public preset content without reading stored templates.
func TemplatePreset(profile, model string) (TemplateDetail, error) {
	item := configstore.KumaTemplate{RequestProfile: profile, Model: strings.TrimSpace(model), Monitoring: &configstore.KumaTemplateMonitoring{Type: "http"}}
	if profile == "" {
		return TemplateDetail{}, failure("kuma_invalid_template_profile", "请选择内置请求模式", 422)
	}
	if err := prepareTemplateProfile(&item); err != nil {
		return TemplateDetail{}, err
	}
	item.BodyEncoding = "json"
	return TemplateDetail{TemplateSummary: templateSummary(item), Headers: item.Headers, Body: item.Body}, nil
}
