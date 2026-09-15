package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func customRequest(t *testing.T, baseURL, platform, key, model string) modelcheck.AnimationRequest {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"targets": []any{}, "timeout_seconds": 5,
		"custom": map[string]string{"base_url": baseURL, "platform": platform, "api_key": key, "model": model},
	})
	if err != nil {
		t.Fatal(err)
	}
	var request modelcheck.AnimationRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	return request
}

func TestCustomAnimationUsesProvidedEndpointAndKeyWithoutBoundAccount(t *testing.T) {
	for _, platform := range []string{"openai", "anthropic"} {
		t.Run(platform, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Model string `json:"model"`
				}
				if json.NewDecoder(r.Body).Decode(&body) != nil || body.Model != "custom-model" {
					t.Error("custom model was not sent")
				}
				if platform == "anthropic" {
					if r.URL.Path != "/proxy/v1/messages" || r.Header.Get("x-api-key") != fixtureSecret || r.Header.Get("Authorization") != "" {
						t.Error("unexpected Anthropic endpoint or credentials")
					}
					_, _ = fmt.Fprintf(w, `{"content":[{"type":"text","text":%q}]}`, fixtureSVG)
					return
				}
				if r.URL.Path != "/proxy/v1/responses" || r.Header.Get("Authorization") != "Bearer "+fixtureSecret {
					t.Error("unexpected OpenAI endpoint or credentials")
				}
				_, _ = fmt.Fprintf(w, `{"output_text":%q}`, fixtureSVG)
			}))
			defer server.Close()
			f := setup(t, 0, platform, func(http.ResponseWriter, *http.Request) { t.Error("bound upstream must not be contacted") })
			queued, err := f.service.EnqueueAnimation(context.Background(), customRequest(t, server.URL+"/proxy/v1/", platform, fixtureSecret, " custom-model "))
			if err != nil {
				t.Fatal(err)
			}
			final := finished(t, f)
			if final.Status != "succeeded" {
				t.Fatalf("custom animation failed: %#v", final)
			}
			rows := final.Result["animations"].([]modelcheck.AnimationResult)
			if len(rows) != 1 || !strings.HasPrefix(rows[0].AccountID, "custom-") || rows[0].SVG == "" {
				t.Fatalf("custom result missing: %#v", rows)
			}
			stored, err := f.tasks.Get(context.Background(), queued.ID)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal([]any{queued, stored})
			if strings.Contains(string(raw), fixtureSecret) || strings.Contains(string(raw), server.URL) || strings.Contains(string(raw), "api_key") {
				t.Fatal("custom credentials or URL were persisted")
			}
			statuses, err := f.service.AccountStatuses(context.Background())
			if err != nil || len(statuses) != 0 {
				t.Fatal("custom animation must not create behavior verdicts")
			}
		})
	}
}

func TestCustomAnimationRejectsInvalidCredentialsAndMixedTargets(t *testing.T) {
	f := setup(t, 0, "openai", func(http.ResponseWriter, *http.Request) { t.Error("invalid request reached upstream") })
	for _, test := range []struct{ name, url, platform, key, model string }{
		{"empty URL", "", "openai", "key", "model"},
		{"unsupported scheme", "file:///tmp/test", "openai", "key", "model"},
		{"embedded credentials", "https://user:password@example.invalid", "openai", "key", "model"},
		{"query credentials", "https://example.invalid?key=secret", "openai", "key", "model"},
		{"unsupported platform", "https://example.invalid", "gemini", "key", "model"},
		{"empty key", "https://example.invalid", "openai", "  ", "model"},
		{"header injection", "https://example.invalid", "openai", "key\r\nx-injected: true", "model"},
		{"empty model", "https://example.invalid", "openai", "key", " "},
		{"key in model", "https://example.invalid", "openai", fixtureSecret, fixtureSecret},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := f.service.EnqueueAnimation(context.Background(), customRequest(t, test.url, test.platform, test.key, test.model)); err == nil {
				t.Fatal("invalid custom request accepted")
			}
		})
	}
	mixed := customRequest(t, "https://example.invalid", "openai", "key", "model")
	mixed.Targets = request("1").Targets
	if _, err := f.service.EnqueueAnimation(context.Background(), mixed); err == nil {
		t.Fatal("custom credentials must not override account bindings")
	}
}

func TestCustomAnimationRedactsKeyInUpstreamFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprintf(w, `{"error":{"message":%q}}`, "invalid "+fixtureSecret)
	}))
	defer server.Close()
	f := setup(t, 0, "openai", func(http.ResponseWriter, *http.Request) { t.Error("unexpected bound request") })
	if _, err := f.service.EnqueueAnimation(context.Background(), customRequest(t, server.URL, "openai", fixtureSecret, "model")); err != nil {
		t.Fatal(err)
	}
	final := finished(t, f)
	raw, _ := json.Marshal(final)
	if final.Status != "failed" || strings.Contains(string(raw), fixtureSecret) {
		t.Fatal("upstream failure must be reported without credentials")
	}
}
