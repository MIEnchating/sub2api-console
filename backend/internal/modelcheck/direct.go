package modelcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/MIEnchating/sub2api-console/backend/internal/redact"
)

const maximumDirectResponseBytes = 4 << 20

type directCredential struct {
	BaseURL         string
	FallbackBaseURL string
	Secret          string
	Platform        string
}

type directBundleSender struct {
	client     *http.Client
	credential directCredential
}

func (sender directBundleSender) Send(ctx context.Context, _ string, model, prompt string, timeoutSeconds int) (string, string, error) {
	requestContext, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	if strings.EqualFold(strings.TrimSpace(sender.credential.Platform), "anthropic") {
		return sender.sendAnthropic(requestContext, model, prompt)
	}
	return sender.sendOpenAI(requestContext, model, prompt)
}

func (sender directBundleSender) sendAnthropic(ctx context.Context, model, prompt string) (string, string, error) {
	body := map[string]any{
		"model": model, "max_tokens": 4096, "stream": false,
		"messages": []map[string]any{{"role": "user", "content": prompt}},
	}
	headers := map[string]string{
		"x-api-key": sender.credential.Secret, "anthropic-version": "2023-06-01",
	}
	payload, status, raw, err := sender.request(ctx, "/v1/messages", body, headers)
	if err != nil {
		return "", "", err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return "", "", directStatusError(status, raw, sender.credential.Secret)
	}
	text := anthropicResponseText(payload)
	if text == "" {
		return "", stringField(payload, "model"), visibleRequestError{message: "上游直连接口未返回有效文本"}
	}
	return text, stringField(payload, "model"), nil
}

func (sender directBundleSender) sendOpenAI(ctx context.Context, model, prompt string) (string, string, error) {
	headers := map[string]string{"Authorization": "Bearer " + sender.credential.Secret}
	responsesBody := map[string]any{
		"model": model, "input": prompt, "max_output_tokens": 4096, "stream": false,
	}
	payload, status, raw, err := sender.request(ctx, "/v1/responses", responsesBody, headers)
	if err != nil {
		return "", "", err
	}
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		text := openAIResponseText(payload)
		if text == "" {
			return "", stringField(payload, "model"), visibleRequestError{message: "上游直连接口未返回有效文本"}
		}
		return text, stringField(payload, "model"), nil
	}
	if !responsesEndpointUnsupported(status, raw) {
		return "", "", directStatusError(status, raw, sender.credential.Secret)
	}
	chatBody := map[string]any{
		"model": model, "messages": []map[string]any{{"role": "user", "content": prompt}},
		"max_tokens": 4096, "stream": false,
	}
	payload, status, raw, err = sender.request(ctx, "/v1/chat/completions", chatBody, headers)
	if err != nil {
		return "", "", err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return "", "", directStatusError(status, raw, sender.credential.Secret)
	}
	text := openAIChatText(payload)
	if text == "" {
		return "", stringField(payload, "model"), visibleRequestError{message: "上游直连接口未返回有效文本"}
	}
	return text, stringField(payload, "model"), nil
}

func (sender directBundleSender) request(ctx context.Context, path string, payload map[string]any, headers map[string]string) (map[string]any, int, []byte, error) {
	endpoint, err := directEndpoint(sender.credential.BaseURL, path)
	if err != nil {
		return nil, 0, nil, visibleRequestError{message: "账号 Base URL 无效"}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, nil, errors.New("模型检测请求编码失败")
	}
	result, status, raw, err := sender.sendRequest(ctx, endpoint, encoded, headers)
	if err != nil {
		return nil, status, raw, err
	}
	if shouldRetryOnFallbackHost(status, raw) && sender.credential.FallbackBaseURL != "" {
		fallbackEndpoint, fallbackErr := directEndpoint(sender.credential.FallbackBaseURL, path)
		if fallbackErr == nil && fallbackEndpoint != endpoint {
			return sender.sendRequest(ctx, fallbackEndpoint, encoded, headers)
		}
	}
	return result, status, raw, nil
}

func (sender directBundleSender) sendRequest(ctx context.Context, endpoint string, encoded []byte, headers map[string]string) (map[string]any, int, []byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, 0, nil, errors.New("模型检测请求创建失败")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "Sub2API-Console/1.0")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := sender.client.Do(request)
	if err != nil {
		return nil, 0, nil, safeTransportError(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maximumDirectResponseBytes+1))
	if err != nil {
		return nil, response.StatusCode, nil, safeTransportError(err)
	}
	if len(raw) > maximumDirectResponseBytes {
		return nil, response.StatusCode, nil, visibleRequestError{message: "上游直连接口响应过大"}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, response.StatusCode, raw, nil
	}
	var result map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		return nil, response.StatusCode, raw, visibleRequestError{message: "上游直连接口返回的不是有效 JSON"}
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, response.StatusCode, raw, visibleRequestError{message: "上游直连接口响应包含尾随数据"}
	}
	return result, response.StatusCode, raw, nil
}

func shouldRetryOnFallbackHost(status int, raw []byte) bool {
	if status != http.StatusForbidden {
		return false
	}
	lower := strings.ToLower(string(raw))
	return (strings.Contains(lower, "<html") || strings.Contains(lower, "<!doctype html")) &&
		(strings.Contains(lower, "cloudflare") || strings.Contains(lower, "sorry, you have been blocked"))
}

func directFallbackBaseURL(baseURL, authHost string) string {
	primary, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || primary.Scheme == "" || primary.Host == "" {
		return ""
	}
	raw := strings.TrimSpace(authHost)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = primary.Scheme + "://" + raw
	}
	fallback, err := url.Parse(raw)
	if err != nil || (fallback.Scheme != "http" && fallback.Scheme != "https") || fallback.Host == "" || fallback.User != nil {
		return ""
	}
	if strings.EqualFold(primary.Host, fallback.Host) {
		return ""
	}
	fallback.Path, fallback.RawPath, fallback.RawQuery, fallback.Fragment = "", "", "", ""
	return strings.TrimRight(fallback.String(), "/")
}

func directEndpoint(baseURL, endpointPath string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return "", errors.New("invalid Base URL")
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	if strings.HasSuffix(strings.ToLower(basePath), "/v1") && strings.HasPrefix(endpointPath, "/v1/") {
		endpointPath = strings.TrimPrefix(endpointPath, "/v1")
	}
	parsed.Path = basePath + endpointPath
	parsed.RawPath, parsed.RawQuery, parsed.Fragment = "", "", ""
	return parsed.String(), nil
}

func anthropicResponseText(payload map[string]any) string {
	content, _ := payload["content"].([]any)
	parts := make([]string, 0, len(content))
	for _, raw := range content {
		item, _ := raw.(map[string]any)
		if text := stringField(item, "text"); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func openAIResponseText(payload map[string]any) string {
	if text := stringField(payload, "output_text"); text != "" {
		return text
	}
	output, _ := payload["output"].([]any)
	parts := make([]string, 0)
	for _, rawOutput := range output {
		item, _ := rawOutput.(map[string]any)
		content, _ := item["content"].([]any)
		for _, rawContent := range content {
			part, _ := rawContent.(map[string]any)
			if text := stringField(part, "text"); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func openAIChatText(payload map[string]any) string {
	choices, _ := payload["choices"].([]any)
	if len(choices) == 0 {
		return ""
	}
	choice, _ := choices[0].(map[string]any)
	message, _ := choice["message"].(map[string]any)
	if text := stringField(message, "content"); text != "" {
		return text
	}
	content, _ := message["content"].([]any)
	parts := make([]string, 0, len(content))
	for _, raw := range content {
		item, _ := raw.(map[string]any)
		if text := stringField(item, "text"); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func stringField(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}

func responsesEndpointUnsupported(status int, raw []byte) bool {
	if status == http.StatusNotFound || status == http.StatusMethodNotAllowed || status == http.StatusNotImplemented {
		return true
	}
	if status != http.StatusBadRequest {
		return false
	}
	message := strings.ToLower(responseErrorDetail(raw))
	return strings.Contains(message, "responses") && (strings.Contains(message, "unsupported") ||
		strings.Contains(message, "not support") || strings.Contains(message, "not found") || strings.Contains(message, "不支持"))
}

func directStatusError(status int, raw []byte, secret string) error {
	detail := responseErrorDetail(raw)
	if secret != "" {
		detail = strings.ReplaceAll(detail, secret, "[已隐藏]")
	}
	detail = safeCredentialText(detail)
	message := fmt.Sprintf("上游直连接口返回 HTTP %d", status)
	if detail != "" {
		message += "：" + detail
	}
	return visibleRequestError{message: message}
}

func responseErrorDetail(raw []byte) string {
	var payload map[string]any
	if json.Unmarshal(raw, &payload) == nil {
		if nested, ok := payload["error"].(map[string]any); ok {
			if message := stringField(nested, "message"); message != "" {
				return message
			}
		}
		for _, key := range []string{"message", "detail", "error"} {
			if message := stringField(payload, key); message != "" {
				return message
			}
		}
	}
	text := strings.TrimSpace(string(raw))
	lower := strings.ToLower(text)
	if strings.Contains(lower, "<html") || strings.Contains(lower, "<!doctype html") {
		if strings.Contains(lower, "cloudflare") || strings.Contains(lower, "sorry, you have been blocked") {
			return "Cloudflare/WAF 拒绝了当前服务器的访问，请检查上游防火墙、代理或出口 IP"
		}
		return "上游返回 HTML 拒绝页面，请检查上游防火墙或代理配置"
	}
	return text
}

func safeCredentialError(err error) string {
	if err == nil {
		return ""
	}
	return safeCredentialText(err.Error())
}

func safeCredentialText(value string) string {
	value = strings.TrimSpace(redact.Secrets(value))
	if utf8.RuneCountInString(value) > 300 {
		runes := []rune(value)
		value = string(runes[:300]) + "..."
	}
	return value
}
