package adminclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"github.com/MIEnchating/sub2api-console/backend/internal/redact"
)

// ProbeUnavailableError means no generation was attempted. Callers must not
// turn missing credentials, unsupported configuration or a failed management
// read into an upstream health failure. Messages never include private data.
type ProbeUnavailableError struct {
	Code    string
	Message string
	cause   error
}

func (e *ProbeUnavailableError) Error() string { return e.Message }
func (e *ProbeUnavailableError) Unwrap() error { return e.cause }

// AccountProbe holds one verified account snapshot only for the current probe.
// All credential fields are private and must never be persisted or returned by
// a public API. Preparing it separately keeps management latency out of TTFT.
type AccountProbe struct {
	http         *http.Client
	baseURL      string
	secret       string
	adminSecret  string
	protocol     string
	modelMapping map[string]string
}

// ProbeCredentialSource separates the account's binding URL from an optional
// protocol-specific generation URL in the same verified management snapshot.
type ProbeCredentialSource struct {
	BindingBaseURL    string
	GenerationBaseURL string
}

// ProbeKeyResolver obtains a key for an exact account and verified URL source.
// Expected domain failures should return ProbeUnavailableError with a safe message.
type ProbeKeyResolver func(context.Context, string, ProbeCredentialSource) (string, error)

func (c *Client) PrepareAccountProbe(ctx context.Context, accountID string, resolvers ...ProbeKeyResolver) (*AccountProbe, error) {
	account, err := c.Account(ctx, accountID)
	if err != nil {
		return nil, &ProbeUnavailableError{
			Code: "probe_account_unavailable", Message: "无法读取并核对账号探活配置，请检查管理接口后重试", cause: ctx.Err(),
		}
	}
	if directProbeString(account["type"]) != "apikey" {
		return nil, probeUnavailable("probe_account_type_unsupported", "当前账号类型不支持直连探活，请使用 API Key 账号")
	}
	if proxyID := account["proxy_id"]; proxyID != nil && proxyID != "" {
		return nil, probeUnavailable("probe_proxy_unsupported", "账号配置了出站代理，当前直连探活尚不支持该代理，请核对账号配置")
	}
	credentials, ok := account["credentials"].(map[string]any)
	if !ok {
		return nil, probeUnavailable("probe_credentials_missing", "账号缺少探活凭据，请补充账号的 API Key 与 Base URL")
	}
	protocol, err := directProbeProtocol(directProbeString(account["platform"]), credentials)
	if err != nil {
		return nil, err
	}
	baseURL, err := directProbeBaseURL(credentials, protocol)
	if err != nil {
		return nil, err
	}
	mapping, err := directProbeModelMapping(credentials["model_mapping"])
	if err != nil {
		return nil, err
	}
	source := ProbeCredentialSource{BindingBaseURL: directProbeString(credentials["base_url"]), GenerationBaseURL: baseURL}
	secret, err := resolveDirectProbeKey(ctx, accountID, source, account, credentials, resolvers)
	if err != nil {
		return nil, err
	}
	return &AccountProbe{
		http: &http.Client{
			Transport: c.http.Transport, Timeout: c.http.Timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		baseURL: baseURL, secret: secret, adminSecret: c.adminKey, protocol: protocol, modelMapping: mapping,
	}, nil
}

// Redact strips exact credentials before generic redaction or caller-side
// truncation, including bare credential echoes from upstream error messages.
func (probe *AccountProbe) Redact(value string) string {
	for _, secret := range []string{probe.secret, probe.adminSecret} {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[已隐藏]")
		}
	}
	return redact.Secrets(value)
}

func (c *Client) OpenAccountProbe(ctx context.Context, accountID, model, prompt string) (*http.Response, error) {
	probe, err := c.PrepareAccountProbe(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return probe.Open(ctx, model, prompt)
}

// Open sends exactly one generation request. It deliberately never falls back
// to another protocol, another host or Sub2API's account test endpoint.
func (probe *AccountProbe) Open(ctx context.Context, model, prompt string) (*http.Response, error) {
	model, err := probe.resolveModel(model)
	if err != nil {
		return nil, err
	}
	endpoint, body, err := probe.requestPayload(model, prompt)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, probeUnavailable("probe_request_invalid", "探活请求编码失败，请检查探活配置")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, probeUnavailable("probe_request_invalid", "探活请求创建失败，请检查账号 Base URL")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("User-Agent", "Sub2API-Console/1.0")
	switch probe.protocol {
	case "anthropic":
		request.Header.Set("X-Api-Key", probe.secret)
		request.Header.Set("Anthropic-Version", directProbeAnthropicVersion)
	case "gemini":
		request.Header.Set("X-Goog-Api-Key", probe.secret)
	default:
		request.Header.Set("Authorization", "Bearer "+probe.secret)
	}
	response, err := probe.http.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, context.Canceled
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, context.DeadlineExceeded
		}
		return nil, errors.New("上游直连探活连接失败，请检查账号接口地址与网络后重试")
	}
	return response, nil
}

func (probe *AccountProbe) resolveModel(model string) (string, error) {
	model = strings.TrimSpace(model)
	if model == "" || strings.ContainsFunc(model, unicode.IsControl) {
		return "", probeUnavailable("probe_model_invalid", "探活模型缺失或格式无效，请检查模型配置")
	}
	if mapped, ok := probe.modelMapping[model]; ok {
		model = mapped
	} else {
		// Sub2API resolves exact names first, then the longest trailing-* prefix.
		longest := -1
		mapped := model
		for source, target := range probe.modelMapping {
			if !strings.HasSuffix(source, "*") {
				continue
			}
			prefix := strings.TrimSuffix(source, "*")
			if len(prefix) > longest && strings.HasPrefix(model, prefix) {
				longest, mapped = len(prefix), target
			}
		}
		model = mapped
	}
	if strings.Contains(model, "*") {
		return "", probeUnavailable("probe_mapping_unsupported", "本次探活命中的模型映射目标包含 *，请将该规则的上游模型改为具体模型名，或为本次探活模型添加精确映射后重试")
	}
	return model, nil
}

type directProbeResponsesRequest struct {
	Model           string `json:"model"`
	Input           string `json:"input"`
	Stream          bool   `json:"stream"`
	MaxOutputTokens int    `json:"max_output_tokens"`
}

type directProbeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type directProbeMessagesRequest struct {
	Model     string               `json:"model"`
	Messages  []directProbeMessage `json:"messages"`
	Stream    bool                 `json:"stream"`
	MaxTokens int                  `json:"max_tokens"`
}

type directProbeGeminiPart struct {
	Text string `json:"text"`
}

type directProbeGeminiContent struct {
	Role  string                  `json:"role"`
	Parts []directProbeGeminiPart `json:"parts"`
}

type directProbeGeminiGenerationConfig struct {
	MaxOutputTokens int `json:"maxOutputTokens"`
}

type directProbeGeminiRequest struct {
	Contents         []directProbeGeminiContent        `json:"contents"`
	GenerationConfig directProbeGeminiGenerationConfig `json:"generationConfig"`
}

func (probe *AccountProbe) requestPayload(model, prompt string) (string, any, error) {
	path := "/v1/responses"
	var body any
	switch probe.protocol {
	case "responses":
		body = directProbeResponsesRequest{Model: model, Input: prompt, Stream: true, MaxOutputTokens: 4096}
	case "anthropic", "chat_completions":
		path = "/v1/messages"
		if probe.protocol == "chat_completions" {
			path = "/v1/chat/completions"
		}
		body = directProbeMessagesRequest{Model: model, Messages: []directProbeMessage{{Role: "user", Content: prompt}}, Stream: true, MaxTokens: 4096}
	case "gemini":
		model = strings.TrimPrefix(model, "models/")
		if model == "" || strings.ContainsAny(model, "/\\?#%") || model == "." || model == ".." {
			return "", nil, probeUnavailable("probe_model_invalid", "Gemini 探活模型必须是有效模型 ID，请检查模型配置")
		}
		path = "/v1beta/models/" + model + ":streamGenerateContent"
		body = directProbeGeminiRequest{
			Contents:         []directProbeGeminiContent{{Role: "user", Parts: []directProbeGeminiPart{{Text: prompt}}}},
			GenerationConfig: directProbeGeminiGenerationConfig{MaxOutputTokens: 4096},
		}
	}
	endpoint, err := directProbeEndpoint(probe.baseURL, path)
	if err != nil {
		return "", nil, err
	}
	if probe.protocol == "gemini" {
		endpoint += "?alt=sse"
	}
	return endpoint, body, nil
}

func directProbeProtocol(platform string, credentials map[string]any) (string, error) {
	var protocol string
	switch platform {
	case "openai":
		protocol = "responses"
	case "anthropic", "claude":
		protocol = "anthropic"
	case "gemini", "google":
		protocol = "gemini"
	case "zhipu", "kimi", "deepseek", "grok":
		protocol = "chat_completions"
	default:
		return "", probeUnavailable("probe_platform_unsupported", "当前账号平台不支持直连探活，请检查账号平台配置")
	}
	if raw, present := credentials["api_protocol"]; present && raw != nil {
		configured, ok := raw.(string)
		if !ok {
			return "", probeUnavailable("probe_protocol_unsupported", "账号接口协议配置无效，请检查 api_protocol")
		}
		switch strings.TrimSpace(configured) {
		case "", "adaptive":
		case "responses", "chat_completions", "anthropic", "gemini":
			protocol = strings.TrimSpace(configured)
		default:
			return "", probeUnavailable("probe_protocol_unsupported", "账号接口协议尚不支持直连探活，请检查 api_protocol")
		}
	}
	return protocol, nil
}

func directProbeBaseURL(credentials map[string]any, protocol string) (string, error) {
	baseURL := directProbeString(credentials["base_url"])
	if raw, present := credentials["api_base_urls"]; present && raw != nil {
		urls, ok := raw.(map[string]any)
		if !ok {
			return "", probeUnavailable("probe_url_invalid", "账号协议地址配置无效，请检查 api_base_urls")
		}
		if override, present := urls[protocol]; present {
			baseURL = directProbeString(override)
		}
	}
	if _, err := directProbeEndpoint(baseURL, ""); err != nil {
		return "", err
	}
	return baseURL, nil
}

func directProbeEndpoint(baseURL, endpoint string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil ||
		strings.ContainsAny(baseURL, "?#") || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.ForceQuery {
		return "", probeUnavailable("probe_url_invalid", "账号 Base URL 缺失或无效，须配置不含用户信息、查询参数及片段的 HTTP(S) 地址")
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	for _, version := range []string{"/v1", "/v1beta"} {
		if strings.HasSuffix(basePath, version) && strings.HasPrefix(endpoint, version+"/") {
			endpoint = strings.TrimPrefix(endpoint, version)
			break
		}
	}
	parsed.Path = basePath + endpoint
	parsed.RawPath = ""
	return parsed.String(), nil
}

func directProbeModelMapping(raw any) (map[string]string, error) {
	if raw == nil {
		return nil, nil
	}
	values, ok := raw.(map[string]any)
	if !ok {
		return nil, probeUnavailable("probe_mapping_unsupported", "账号模型映射格式无效，请配置请求模型到上游模型的字符串映射后重试")
	}
	mapping := make(map[string]string, len(values))
	for source, rawTarget := range values {
		target, ok := rawTarget.(string)
		if !ok || strings.TrimSpace(source) == "" || strings.TrimSpace(target) == "" ||
			strings.Contains(strings.TrimSuffix(strings.TrimSpace(source), "*"), "*") ||
			strings.ContainsFunc(source+target, unicode.IsControl) {
			return nil, probeUnavailable("probe_mapping_unsupported", "账号模型映射无效，请使用精确模型名或仅在请求模型末尾使用 *，上游模型须为非空的具体模型名")
		}
		mapping[strings.TrimSpace(source)] = strings.TrimSpace(target)
	}
	return mapping, nil
}

func directProbeString(raw any) string {
	value, _ := raw.(string)
	return strings.TrimSpace(value)
}

func probeUnavailable(code, message string) *ProbeUnavailableError {
	return &ProbeUnavailableError{Code: code, Message: message}
}
