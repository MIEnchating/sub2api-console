package adminclient_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
)

const directProbePrompt = "  只回复 pong\n保留这一行  "

func TestDirectProbeSendsOnlyConfiguredUserPromptWithProtocolAuthentication(t *testing.T) {
	for _, item := range []struct {
		name, platform, basePath, endpoint, query, expectedBody, authHeader, authValue string
	}{
		{"openai", "openai", "", "/v1/responses", "", `{"model":"model-1","input":"  只回复 pong\n保留这一行  ","stream":true,"max_output_tokens":4096}`, "Authorization", "Bearer upstream-test-secret"},
		{"openai-versioned", "openai", "/proxy/v1/", "/proxy/v1/responses", "", `{"model":"model-1","input":"  只回复 pong\n保留这一行  ","stream":true,"max_output_tokens":4096}`, "Authorization", "Bearer upstream-test-secret"},
		{"anthropic", "anthropic", "/v1", "/v1/messages", "", `{"model":"model-1","messages":[{"role":"user","content":"  只回复 pong\n保留这一行  "}],"stream":true,"max_tokens":4096}`, "X-Api-Key", "upstream-test-secret"},
		{"claude", "claude", "/gateway", "/gateway/v1/messages", "", `{"model":"model-1","messages":[{"role":"user","content":"  只回复 pong\n保留这一行  "}],"stream":true,"max_tokens":4096}`, "X-Api-Key", "upstream-test-secret"},
		{"gemini", "gemini", "", "/v1beta/models/model-1:streamGenerateContent", "alt=sse", `{"contents":[{"role":"user","parts":[{"text":"  只回复 pong\n保留这一行  "}]}],"generationConfig":{"maxOutputTokens":4096}}`, "X-Goog-Api-Key", "upstream-test-secret"},
		{"google", "google", "/proxy/v1beta", "/proxy/v1beta/models/model-1:streamGenerateContent", "alt=sse", `{"contents":[{"role":"user","parts":[{"text":"  只回复 pong\n保留这一行  "}]}],"generationConfig":{"maxOutputTokens":4096}}`, "X-Goog-Api-Key", "upstream-test-secret"},
	} {
		t.Run(item.name, func(t *testing.T) {
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.Method != http.MethodPost || r.URL.Path != item.endpoint || r.URL.RawQuery != item.query {
					t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL)
				}
				if r.Header.Get(item.authHeader) != item.authValue {
					t.Error("missing protocol authentication")
				}
				if item.platform == "anthropic" || item.platform == "claude" {
					if r.Header.Get("Anthropic-Version") != "2023-06-01" {
						t.Error("missing Anthropic protocol version")
					}
				}
				if r.Header.Get("Accept") != "text/event-stream" || r.Header.Get("Content-Type") != "application/json" {
					t.Error("probe must request an SSE JSON response")
				}
				for key, values := range r.Header {
					if strings.Contains(strings.Join(values, " "), "management-test-secret") {
						t.Errorf("management credential leaked through header %s", key)
					}
				}
				for _, header := range []string{"Authorization", "X-Api-Key", "X-Goog-Api-Key", "Cookie"} {
					if header != item.authHeader && r.Header.Get(header) != "" {
						t.Errorf("unrelated authentication header forwarded: %s", header)
					}
				}
				var actual, expected any
				if err := json.NewDecoder(r.Body).Decode(&actual); err != nil {
					t.Error(err)
				}
				if err := json.Unmarshal([]byte(item.expectedBody), &expected); err != nil {
					t.Error(err)
				}
				if !reflect.DeepEqual(actual, expected) {
					t.Errorf("probe must preserve the user prompt without system messages: got %#v", actual)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: [DONE]\n\n")
			}))
			defer upstream.Close()
			client := newDirectProbeClient(t, map[string]any{
				"id": 41, "type": "apikey", "platform": item.platform,
				"credentials": map[string]any{"api_key": "upstream-test-secret", "base_url": upstream.URL + item.basePath},
			})
			response, err := client.OpenAccountProbe(context.Background(), "41", "model-1", directProbePrompt)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK || calls.Load() != 1 {
				t.Fatalf("expected one direct probe, status=%d calls=%d", response.StatusCode, calls.Load())
			}
		})
	}
}

func TestDirectProbeRejectsUnavailableAccountConfigurationWithoutUpstreamRequest(t *testing.T) {
	for name, account := range map[string]map[string]any{
		"identity-mismatch": {"id": 42, "type": "apikey", "platform": "openai"},
		"oauth":             {"id": 41, "type": "oauth", "platform": "openai"},
		"missing-type":      {"id": 41, "platform": "openai"},
		"unsupported":       {"id": 41, "type": "apikey", "platform": "unsupported"},
		"missing-key":       {"id": 41, "type": "apikey", "platform": "openai", "credentials": map[string]any{"base_url": "https://unused.invalid"}},
		"missing-base-url":  {"id": 41, "type": "apikey", "platform": "openai", "credentials": map[string]any{"api_key": "upstream-test-secret"}},
		"invalid-key":       {"id": 41, "type": "apikey", "platform": "openai", "credentials": map[string]any{"api_key": "upstream-test-secret\nX-Injected: true", "base_url": "https://unused.invalid"}},
	} {
		t.Run(name, func(t *testing.T) {
			client := newDirectProbeClient(t, account)
			response, err := client.OpenAccountProbe(context.Background(), "41", "model-1", "hi")
			var unavailable *adminclient.ProbeUnavailableError
			if response != nil || !errors.As(err, &unavailable) {
				t.Fatalf("invalid account must be skipped, response=%v error=%v", response, err)
			}
			if strings.Contains(err.Error(), "upstream-test-secret") {
				t.Fatal("configuration error exposed credentials")
			}
		})
	}
}

func TestDirectProbeRejectsBaseURLCredentialsQueryAndFragment(t *testing.T) {
	for _, suffix := range []string{"?key=private-secret", "#private-secret", "?", "#"} {
		t.Run(suffix, func(t *testing.T) {
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
			defer upstream.Close()
			client := newDirectProbeClient(t, map[string]any{
				"id": 41, "type": "apikey", "platform": "openai",
				"credentials": map[string]any{"api_key": "upstream-test-secret", "base_url": upstream.URL + "/v1" + suffix},
			})
			_, err := client.OpenAccountProbe(context.Background(), "41", "model-1", "hi")
			var unavailable *adminclient.ProbeUnavailableError
			if !errors.As(err, &unavailable) || calls.Load() != 0 {
				t.Fatalf("unsafe URL must fail before sending: error=%v calls=%d", err, calls.Load())
			}
			if strings.Contains(err.Error(), "private-secret") {
				t.Fatal("URL error exposed a secret")
			}
		})
	}
	client := newDirectProbeClient(t, map[string]any{
		"id": 41, "type": "apikey", "platform": "openai",
		"credentials": map[string]any{"api_key": "upstream-test-secret", "base_url": "https://user:private-secret@unused.invalid/v1"},
	})
	_, err := client.OpenAccountProbe(context.Background(), "41", "model-1", "hi")
	var unavailable *adminclient.ProbeUnavailableError
	if !errors.As(err, &unavailable) || strings.Contains(err.Error(), "private-secret") {
		t.Fatalf("URL userinfo must be safely rejected: %v", err)
	}
}

func TestDirectProbeRefusesRedirectWithoutSendingCredentialsToDestination(t *testing.T) {
	var redirectedCalls atomic.Int64
	redirected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirectedCalls.Add(1) }))
	defer redirected.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirected.URL+"/capture", http.StatusTemporaryRedirect)
	}))
	defer upstream.Close()
	client := newDirectProbeClient(t, map[string]any{
		"id": 41, "type": "apikey", "platform": "anthropic",
		"credentials": map[string]any{"api_key": "upstream-test-secret", "base_url": upstream.URL},
	})
	response, err := client.OpenAccountProbe(context.Background(), "41", "model-1", "hi")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusTemporaryRedirect || redirectedCalls.Load() != 0 {
		t.Fatalf("redirect must never be followed: status=%d redirected=%d", response.StatusCode, redirectedCalls.Load())
	}
}

func TestDirectProbeReturnsHTTPFailureWithoutAutomaticFallbackOrReplay(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"message":"responses unsupported"}}`)
	}))
	defer upstream.Close()
	client := newDirectProbeClient(t, map[string]any{
		"id": 41, "type": "apikey", "platform": "openai",
		"credentials": map[string]any{"api_key": "upstream-test-secret", "base_url": upstream.URL},
	})
	response, err := client.OpenAccountProbe(context.Background(), "41", "model-1", "hi")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound || calls.Load() != 1 {
		t.Fatalf("unsupported endpoint must not cause another generation: status=%d calls=%d", response.StatusCode, calls.Load())
	}
}

func TestDirectProbeManagementFailureIsUnavailableAndDoesNotExposeServerError(t *testing.T) {
	management := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":"management-test-secret upstream-test-secret"}`)
	}))
	defer management.Close()
	client, err := adminclient.New(adminclient.Config{BaseURL: management.URL, AdminKey: "management-test-secret", Attempts: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.OpenAccountProbe(context.Background(), "41", "model-1", "hi")
	var unavailable *adminclient.ProbeUnavailableError
	if !errors.As(err, &unavailable) || strings.Contains(err.Error(), "test-secret") {
		t.Fatalf("management failure must be a safe skipped probe: %v", err)
	}
}

func TestDirectProbeTransportFailureIsSafeAndPreservesCancellation(t *testing.T) {
	for _, cause := range []error{errors.New("upstream-test-secret at https://private-host.invalid"), context.Canceled, context.DeadlineExceeded} {
		t.Run(cause.Error(), func(t *testing.T) {
			var generationCalls atomic.Int64
			client, err := adminclient.New(adminclient.Config{BaseURL: "https://management.invalid", AdminKey: "management-test-secret", Attempts: 1}, responseTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "management.invalid" {
					return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":{"id":41,"type":"apikey","platform":"openai","credentials":{"api_key":"upstream-test-secret","base_url":"https://private-host.invalid"}}}`))}, nil
				}
				generationCalls.Add(1)
				return nil, cause
			}))
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.OpenAccountProbe(context.Background(), "41", "model-1", "hi")
			if err == nil || strings.Contains(err.Error(), "test-secret") || strings.Contains(err.Error(), "private-host") || generationCalls.Load() != 1 {
				t.Fatalf("unsafe transport failure or replay: error=%v calls=%d", err, generationCalls.Load())
			}
			var unavailable *adminclient.ProbeUnavailableError
			if errors.As(err, &unavailable) {
				t.Fatal("upstream transport errors must remain probe failures")
			}
			if (errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded)) && !errors.Is(err, cause) {
				t.Fatalf("lost context error: %v", err)
			}
		})
	}
}

func TestPreparedAccountProbeReadsCredentialsOnceAcrossMultipleGenerations(t *testing.T) {
	var managementCalls, generationCalls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		generationCalls.Add(1)
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()
	management := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		managementCalls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"id": 41, "type": "apikey", "platform": "openai",
			"credentials": map[string]any{"api_key": "upstream-test-secret", "base_url": upstream.URL},
		}})
	}))
	defer management.Close()
	client, err := adminclient.New(adminclient.Config{BaseURL: management.URL, AdminKey: "management-test-secret", Attempts: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	probe, err := client.PrepareAccountProbe(context.Background(), "41")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		response, err := probe.Open(context.Background(), "model-1", "hi")
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
	}
	if managementCalls.Load() != 1 || generationCalls.Load() != 2 {
		t.Fatalf("preparation must be independent of generation attempts: reads=%d generations=%d", managementCalls.Load(), generationCalls.Load())
	}
	encoded, err := json.Marshal(probe)
	if err != nil || string(encoded) != "{}" {
		t.Fatalf("prepared private credentials must not serialize: error=%v", err)
	}
	redacted := probe.Redact("bare upstream-test-secret and management-test-secret echoed by upstream")
	if strings.Contains(redacted, "test-secret") || !strings.Contains(redacted, "[已隐藏]") {
		t.Fatal("bare upstream and management credential echoes must be redacted")
	}
}

func TestDirectProbeUsesAdaptiveChatEndpointAndExactModelMapping(t *testing.T) {
	for _, platform := range []string{"zhipu", "kimi", "deepseek"} {
		t.Run(platform, func(t *testing.T) {
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/custom/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer upstream-test-secret" {
					t.Error("adaptive chat probe used an incorrect endpoint or authentication")
				}
				var body struct {
					Model    string `json:"model"`
					Messages []struct {
						Role, Content string
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body.Model != "actual-model" || len(body.Messages) != 1 || body.Messages[0].Role != "user" || body.Messages[0].Content != directProbePrompt {
					t.Errorf("exact model mapping must preserve prompt: %#v", body)
				}
				_, _ = io.WriteString(w, "data: [DONE]\n\n")
			}))
			defer upstream.Close()
			client := newDirectProbeClient(t, map[string]any{
				"id": 41, "type": "apikey", "platform": platform,
				"credentials": map[string]any{
					"api_key": "upstream-test-secret", "base_url": "https://unused.invalid", "api_protocol": "adaptive",
					"api_base_urls": map[string]any{"chat_completions": upstream.URL + "/custom/v1", "responses": "https://unused.invalid"},
					"model_mapping": map[string]any{"model-1": "actual-model"},
				},
			})
			response, err := client.OpenAccountProbe(context.Background(), "41", "model-1", directProbePrompt)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if calls.Load() != 1 {
				t.Fatalf("expected configured chat endpoint, calls=%d", calls.Load())
			}
		})
	}
}

func TestDirectProbeUsesExplicitProtocolInsteadOfPlatformDefault(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages/v1/messages" || r.Header.Get("X-Api-Key") != "upstream-test-secret" || r.Header.Get("Authorization") != "" {
			t.Error("explicit protocol and its own base URL must determine the request")
		}
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()
	client := newDirectProbeClient(t, map[string]any{
		"id": 41, "type": "apikey", "platform": "kimi",
		"credentials": map[string]any{
			"api_key": "upstream-test-secret", "base_url": "https://unused.invalid", "api_protocol": "anthropic",
			"api_base_urls": map[string]any{"anthropic": upstream.URL + "/messages"},
		},
	})
	response, err := client.OpenAccountProbe(context.Background(), "41", "model-1", "hi")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
}

func TestDirectProbeSkipsProxyAndUnsupportedMappingOrProtocolBeforeGeneration(t *testing.T) {
	for name, patch := range map[string]map[string]any{
		"proxy":              {"proxy_id": 7},
		"wildcard-mapping":   {"model_mapping": map[string]any{"*": "actual-model"}},
		"structured-mapping": {"model_mapping": map[string]any{"model-1": map[string]any{"model": "actual-model"}}},
		"invalid-mapping":    {"model_mapping": "model-1"},
		"invalid-protocol":   {"api_protocol": "unsupported"},
		"malformed-protocol": {"api_protocol": true},
		"malformed-urls":     {"api_base_urls": "https://unused.invalid"},
	} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int64
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
			defer upstream.Close()
			credentials := map[string]any{"api_key": "upstream-test-secret", "base_url": upstream.URL}
			account := map[string]any{"id": 41, "type": "apikey", "platform": "openai", "credentials": credentials}
			for key, value := range patch {
				if key == "proxy_id" {
					account[key] = value
				} else {
					credentials[key] = value
				}
			}
			client := newDirectProbeClient(t, account)
			_, err := client.PrepareAccountProbe(context.Background(), "41")
			var unavailable *adminclient.ProbeUnavailableError
			if !errors.As(err, &unavailable) || calls.Load() != 0 {
				t.Fatalf("unsupported settings must be skipped without generation: error=%v calls=%d", err, calls.Load())
			}
		})
	}
}

func newDirectProbeClient(t *testing.T, account map[string]any) *adminclient.Client {
	t.Helper()
	management := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/accounts/41" {
			t.Errorf("probe must read credentials by stable ID without using the management test API: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-Api-Key") != "management-test-secret" {
			t.Error("management read is missing its own authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"data": account}); err != nil {
			t.Error(fmt.Errorf("encode account fixture: %w", err))
		}
	}))
	t.Cleanup(management.Close)
	client, err := adminclient.New(adminclient.Config{BaseURL: management.URL, AdminKey: "management-test-secret", Attempts: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return client
}
