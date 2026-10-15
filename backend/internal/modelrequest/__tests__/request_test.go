package modelrequest_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelrequest"
)

func TestProtocolSwitchChangesPayloadHeadersAndGeminiEndpoint(t *testing.T) {
	for _, protocol := range []modelrequest.Protocol{modelrequest.Responses, modelrequest.Chat, modelrequest.Anthropic, modelrequest.Gemini} {
		for _, stream := range []bool{false, true} {
			t.Run(string(protocol)+map[bool]string{true: "/stream", false: "/json"}[stream], func(t *testing.T) {
				client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if protocol == modelrequest.Gemini {
						if strings.Contains(r.URL.Path, "streamGenerateContent") != stream || (r.URL.Query().Get("alt") == "sse") != stream {
							t.Fatalf("incorrect Gemini endpoint: %s", r.URL)
						}
						if r.Header.Get("x-goog-api-key") != "test-secret" {
							t.Fatal("missing Gemini auth")
						}
					} else if body["stream"] != stream {
						t.Fatalf("stream option ignored: %#v", body)
					}
					if strings.Contains(r.Header.Get("Accept"), "text/event-stream") != stream {
						t.Fatal("Accept disagrees with stream option")
					}
					return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
				})}
				response, err := modelrequest.Open(t.Context(), client, modelrequest.Request{BaseURL: "https://example.invalid/v1", Secret: "test-secret", Protocol: protocol, Parameters: modelrequest.Parameters{Model: "model-test", Prompt: "hello", Stream: stream, MaxOutputTokens: 16}, Timeout: time.Second})
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
			})
		}
	}
}

func TestEndpointPreservesConfiguredPathButNeverForwardsBaseURLQuery(t *testing.T) {
	endpoint, err := modelrequest.Endpoint("https://example.invalid/proxy/v1?ignored=private#fragment", "/v1/responses")
	if err != nil || endpoint != "https://example.invalid/proxy/v1/responses" {
		t.Fatalf("endpoint=%s err=%v", endpoint, err)
	}
}
