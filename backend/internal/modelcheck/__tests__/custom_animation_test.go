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
			if strings.Contains(string(raw), fixtureSecret) || strings.Contains(string(raw), "api_key") {
				t.Fatal("custom credentials were persisted")
			}
			var snapshots []struct {
				Result struct {
					Targets []struct {
						Endpoint string `json:"endpoint"`
						Platform string `json:"platform"`
						Model    string `json:"model"`
					} `json:"targets"`
					Animations []struct {
						Endpoint string `json:"endpoint"`
						Platform string `json:"platform"`
					} `json:"animations"`
				} `json:"result"`
			}
			if err := json.Unmarshal(raw, &snapshots); err != nil {
				t.Fatal(err)
			}
			for _, snapshot := range snapshots {
				if len(snapshot.Result.Targets) != 1 || snapshot.Result.Targets[0].Endpoint != server.URL+"/proxy/v1" || snapshot.Result.Targets[0].Platform != platform || snapshot.Result.Targets[0].Model != "custom-model" {
					t.Fatalf("custom task must retain endpoint and requested model: %s", raw)
				}
			}
			if len(snapshots[1].Result.Animations) != 1 || snapshots[1].Result.Animations[0].Endpoint != server.URL+"/proxy/v1" || snapshots[1].Result.Animations[0].Platform != platform {
				t.Fatalf("completed record lost custom endpoint: %s", raw)
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
		{"key in URL path", "https://example.invalid/" + fixtureSecret, "openai", fixtureSecret, "model"},
		{"encoded key in URL path", "https://example.invalid/%73ecret", "openai", "secret", "model"},
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

func TestCustomFailedChecksRetainEndpointMetadata(t *testing.T) {
	for _, mode := range []string{"animation", "precheck"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = fmt.Fprint(w, `{"error":{"message":"invalid credential"}}`)
			}))
			defer server.Close()
			f := setup(t, 0, "openai", func(http.ResponseWriter, *http.Request) { t.Error("unexpected bound request") })
			input := customRequest(t, server.URL+"/v1", "openai", fixtureSecret, "requested-model")
			input.Mode = mode
			if _, err := f.service.EnqueueAnimation(context.Background(), input); err != nil {
				t.Fatal(err)
			}
			final := finished(t, f)
			rows := final.Result["animations"].([]modelcheck.AnimationResult)
			if len(rows) != 1 || rows[0].Status != "failed" || rows[0].Endpoint != server.URL+"/v1" || rows[0].Platform != "openai" || rows[0].Model != "requested-model" {
				t.Fatalf("failed check lost its source: %#v", final)
			}
		})
	}
}
