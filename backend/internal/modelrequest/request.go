// Package modelrequest is the sole wire entry point for upstream model generation.
// Callers own scheduling, credential resolution and domain-specific validation.
package modelrequest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Protocol string

const (
	Responses Protocol = "responses"
	Chat      Protocol = "chat_completions"
	Anthropic Protocol = "anthropic"
	Gemini    Protocol = "gemini"
)

// Parameters are shared by all generation features. Stream controls both the
// wire protocol and timeout policy; it is deliberately not a UI setting.
type Parameters struct {
	Model           string
	Prompt          string
	Stream          bool
	MaxOutputTokens int
	ReasoningEffort string
	IncludeUsage    bool
	Instructions    string
	InputMessages   bool
	Store           *bool
}

type Request struct {
	BaseURL string
	// Endpoint is only for fixed provider routes, such as the OAuth Responses API.
	Endpoint   string
	Secret     string
	Protocol   Protocol
	Parameters Parameters
	Headers    http.Header
	Timeout    time.Duration
}

// Open starts after callers acquire their concurrency slots. An existing
// execution budget is reused across protocol fallbacks; Close releases only
// the budget owned by this call. No automatic network retries are performed.
func Open(ctx context.Context, client *http.Client, input Request) (*http.Response, error) {
	path, body, err := Build(input.Protocol, input.Parameters)
	if err != nil {
		return nil, err
	}
	endpoint := input.Endpoint
	if endpoint == "" {
		endpoint, err = Endpoint(input.BaseURL, path)
		if err != nil {
			return nil, err
		}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	timeout := input.Timeout
	if timeout <= 0 && client != nil {
		timeout = client.Timeout
	}
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ctx, closeBudget := Start(ctx, timeout, input.Parameters.Stream)
	if input.Parameters.Stream {
		if budget, ok := ctx.Value(budgetKey{}).(*executionBudget); ok {
			budget.mu.Lock()
			budget.stream = true // A confirmed gateway rejection may require streaming fallback.
			budget.mu.Unlock()
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		closeBudget()
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if input.Parameters.Stream {
		request.Header.Set("Accept", "text/event-stream")
	}
	request.Header.Set("User-Agent", "Sub2API-Console/1.0")
	switch input.Protocol {
	case Anthropic:
		request.Header.Set("x-api-key", input.Secret)
		request.Header.Set("anthropic-version", "2023-06-01")
	case Gemini:
		request.Header.Set("x-goog-api-key", input.Secret)
	default:
		request.Header.Set("Authorization", "Bearer "+input.Secret)
	}
	for key, values := range input.Headers {
		request.Header[http.CanonicalHeaderKey(key)] = append([]string(nil), values...)
	}
	transportClient := http.Client{}
	if client != nil {
		transportClient = *client
	}
	// A fixed Client.Timeout would silently defeat the stream-event reset.
	transportClient.Timeout = 0
	transportClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := transportClient.Do(request)
	if err != nil {
		if context.Cause(ctx) == context.DeadlineExceeded {
			err = context.DeadlineExceeded
		}
		closeBudget()
		return nil, err
	}
	response.Body = &observedBody{ReadCloser: response.Body, ctx: ctx, closeBudget: closeBudget, observe: input.Parameters.Stream && response.StatusCode >= 200 && response.StatusCode < 300}
	return response, nil
}

func Endpoint(baseURL, path string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
		return "", errors.New("模型接口 Base URL 无效")
	}
	p, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	basePath := strings.TrimRight(u.Path, "/")
	for _, version := range []string{"/v1beta", "/v1"} {
		if strings.HasSuffix(basePath, version) && (strings.HasPrefix(p.Path, "/v1/") || strings.HasPrefix(p.Path, "/v1beta/")) {
			basePath = strings.TrimSuffix(basePath, version)
			break
		}
	}
	u.Path = basePath + p.Path
	u.RawPath = ""
	u.RawQuery = p.RawQuery
	u.ForceQuery = false
	u.Fragment = ""
	return u.String(), nil
}
