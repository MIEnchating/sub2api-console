package modelcheck

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelrequest"
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
	stream     bool
	client     *http.Client
	credential directCredential
	requestID  string
	usage      *AnimationUsage
}

func (sender directBundleSender) Send(ctx context.Context, _ string, model, prompt string, timeoutSeconds int) (string, string, error) {
	return sender.SendWithReasoning(ctx, "", model, prompt, timeoutSeconds, "")
}

func (sender directBundleSender) SendWithReasoning(ctx context.Context, _ string, model, prompt string, timeoutSeconds int, effort string) (string, string, error) {
	requestContext, cancel := modelrequest.Start(ctx, time.Duration(timeoutSeconds)*time.Second, sender.stream)
	defer cancel()
	if sender.credential.Platform == "gemini" {
		return sendGeminiGeneration(requestContext, sender.client, sender.credential, model, prompt, sender.requestID, 4096, sender.usage, nil, sender.stream)
	}
	if strings.EqualFold(strings.TrimSpace(sender.credential.Platform), "anthropic") {
		if effort != "" && sender.usage == nil {
			return "", "", visibleRequestError{message: "Astra 检测需要支持思考等级的 OpenAI 接口，请检查账号平台配置"}
		}
		return sender.sendAnthropic(requestContext, model, prompt, effort)
	}
	return sender.sendOpenAI(requestContext, model, prompt, effort)
}

func (sender directBundleSender) sendAnthropic(ctx context.Context, model, prompt, effort string) (string, string, error) {
	parameters := modelrequest.Parameters{Model: model, Prompt: prompt, MaxOutputTokens: 4096, Stream: sender.stream, ReasoningEffort: effort}
	payload, status, raw, err := sender.request(ctx, modelrequest.Anthropic, parameters)
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

func (sender directBundleSender) sendOpenAI(ctx context.Context, model, prompt, effort string) (string, string, error) {
	parameters := modelrequest.Parameters{Model: model, Prompt: prompt, MaxOutputTokens: 4096, Stream: sender.stream, ReasoningEffort: effort, IncludeUsage: true}
	payload, status, raw, err := sender.request(ctx, modelrequest.Responses, parameters)
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
	payload, status, raw, err = sender.request(ctx, modelrequest.Chat, parameters)
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

func (sender directBundleSender) request(ctx context.Context, protocol modelrequest.Protocol, parameters modelrequest.Parameters) (map[string]any, int, []byte, error) {
	result, status, raw, err := sender.sendRequest(ctx, sender.credential.BaseURL, protocol, parameters)
	if err != nil {
		return nil, status, raw, err
	}
	if shouldRetryOnFallbackHost(status, raw) && sender.credential.FallbackBaseURL != "" && sender.credential.FallbackBaseURL != sender.credential.BaseURL {
		return sender.sendRequest(ctx, sender.credential.FallbackBaseURL, protocol, parameters)
	}
	return result, status, raw, nil
}

func (sender directBundleSender) sendRequest(ctx context.Context, baseURL string, protocol modelrequest.Protocol, parameters modelrequest.Parameters) (map[string]any, int, []byte, error) {
	path, _, err := modelrequest.Build(protocol, parameters)
	if err != nil {
		return nil, 0, nil, err
	}
	response, err := modelrequest.Open(ctx, sender.client, modelrequest.Request{BaseURL: baseURL, Secret: sender.credential.Secret, Protocol: protocol, Parameters: parameters, Headers: http.Header{"X-Request-Id": []string{sender.requestID}}})
	if err != nil {
		return nil, 0, nil, safeTransportError(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	if sender.stream && response.StatusCode >= 200 && response.StatusCode < 300 {
		jsonResponse, err := generationResponseIsJSON(reader)
		if err != nil {
			return nil, response.StatusCode, nil, safeTransportError(err)
		}
		if !jsonResponse {
			return sender.readStream(reader, path, response.StatusCode)
		}
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maximumDirectResponseBytes+1))
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
	if result == nil || result["error"] != nil || result["success"] == false || stringField(result, "type") == "error" {
		return nil, response.StatusCode, nil, visibleRequestError{message: "上游报告检测失败，请检查模型权限、余额或稍后重试"}
	}
	if status := stringField(result, "status"); status != "" && status != "completed" {
		return nil, response.StatusCode, nil, visibleRequestError{message: "上游检测响应未完整生成，请稍后重试"}
	}
	if stringField(result, "stop_reason") == "max_tokens" {
		return nil, response.StatusCode, nil, visibleRequestError{message: "上游检测响应被截断，请检查模型后重试"}
	}
	if choices, ok := result["choices"].([]any); ok && len(choices) > 0 {
		choice, _ := choices[0].(map[string]any)
		if reason := stringField(choice, "finish_reason"); reason == "length" || reason == "content_filter" {
			return nil, response.StatusCode, nil, visibleRequestError{message: "上游检测响应被截断或拦截，请检查模型后重试"}
		}
	}
	if model := stringField(result, "model"); model != "" {
		if sender.credential.Secret != "" {
			model = strings.ReplaceAll(model, sender.credential.Secret, "[已隐藏]")
		}
		result["model"] = safeCredentialText(model)
	}
	sender.usage.read(result)
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
	if strings.HasPrefix(endpointPath, "/v1beta/") {
		for _, suffix := range []string{"/v1beta", "/v1"} {
			if strings.HasSuffix(basePath, suffix) {
				basePath = strings.TrimSuffix(basePath, suffix)
				break
			}
		}
	}
	if strings.HasSuffix(strings.ToLower(basePath), "/v1") && strings.HasPrefix(endpointPath, "/v1/") {
		endpointPath = strings.TrimPrefix(endpointPath, "/v1")
	}
	parsed.Path = basePath + endpointPath
	parsed.RawPath, parsed.RawQuery, parsed.Fragment = "", "", ""
	if isGeminiGenerationPath(endpointPath) {
		parsed.RawQuery = "alt=sse"
	}
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
