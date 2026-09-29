package probe_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
)

func TestDirectProbeRequiresRealContentAcrossSupportedProtocols(t *testing.T) {
	for _, scenario := range []struct {
		name, platform, contentType, body, model string
		passed                                   bool
	}{
		{name: "Responses text and completion pass", platform: "openai", body: probeTextEvent + probeCompletedEvent, passed: true},
		{name: "Anthropic text and completion pass", platform: "anthropic", body: "data: {\"type\":\"message_start\",\"message\":{\"model\":\"actual-claude\"}}\n\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"pong\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n", model: "actual-claude", passed: true},
		{name: "Gemini completed text passes", platform: "gemini", body: "data: {\"modelVersion\":\"actual-gemini\",\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"pong\"}]},\"finishReason\":\"STOP\"}]}\n\n", model: "actual-gemini", passed: true},
		{name: "Chat text and completion pass", platform: "deepseek", body: "data: {\"model\":\"actual-chat\",\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\ndata: [DONE]\n\n", model: "actual-chat", passed: true},
		{name: "Formatted JSON text passes", platform: "openai", contentType: "application/json", body: "{\n \"model\":\"actual-json\",\n \"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"pong\"}]}]\n}", model: "actual-json", passed: true},
		{name: "Multiline SSE text and completion pass", platform: "openai", body: "data: {\"type\":\"response.output_text.delta\",\ndata: \"delta\":\"pong\"}\n\n" + probeCompletedEvent, passed: true},
		{name: "HTTP 200 error fails even with text", platform: "openai", body: "data: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\",\"error\":{\"message\":\"generation denied\"}}\n\n"},
		{name: "Failed response status fails even with text", platform: "openai", contentType: "application/json", body: `{"status":"failed","output":[{"type":"message","content":[{"type":"output_text","text":"partial"}]}]}`},
		{name: "Complete JSON with length termination fails", platform: "grok", contentType: "application/json", body: `{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}]}`},
		{name: "Complete JSON with nested failure fails", platform: "openai", contentType: "application/json", body: `{"type":"response.completed","response":{"status":"failed","output_text":"partial"}}`},
		{name: "Metadata and done without text fail", platform: "openai", body: "data: {\"type\":\"response.created\",\"response\":{\"model\":\"actual-model\"}}\n\ndata: [DONE]\n\n", model: "actual-model"},
		{name: "Thinking without text fails", platform: "anthropic", body: "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"thinking_delta\",\"thinking\":\"processing\"}}\n\n"},
		{name: "Gemini thoughts without answer fail", platform: "gemini", body: "data: {\"candidates\":[{\"content\":{\"parts\":[{\"thought\":true,\"text\":\"processing\"}]}}]}\n\n"},
		{name: "Tool arguments without text fail", platform: "openai", body: "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"function\":{\"arguments\":\"{}\"}}]}}]}\n\n"},
		{name: "Empty JSON response fails", platform: "openai", contentType: "application/json", body: `{}`},
		{name: "Malformed SSE fails", platform: "openai", body: "data: not-json\n\n"},
		{name: "Trailing JSON data fails", platform: "openai", contentType: "application/json", body: `{"type":"response.output_text.delta","delta":"pong"} {}`},
		{name: "Echoed credential is removed before truncation", platform: "openai", body: "data: {\"error\":{\"message\":\"" + strings.Repeat("x", 480) + strings.Repeat("secret", 10) + "\"}}\n\n"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := directProbeStore(t)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				contentType := scenario.contentType
				if contentType == "" {
					contentType = "text/event-stream"
				}
				w.Header().Set("Content-Type", contentType)
				_, _ = w.Write([]byte(scenario.body))
			}))
			defer upstream.Close()
			admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/accounts/41" {
					t.Errorf("unexpected management request %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 41, "type": "apikey", "platform": scenario.platform, "credentials": map[string]any{"api_key": strings.Repeat("secret", 10), "base_url": upstream.URL}}})
			}))
			defer admin.Close()
			summary, err := probe.New(store, protectionTarget{endpoint: admin.URL}, nil).RunNow(context.Background(), probe.Request{AccountID: pointer("41")})
			if err != nil {
				t.Fatal(err)
			}
			if len(summary.Results) != 1 || summary.Persisted != 1 || summary.Skipped != 0 {
				t.Fatalf("unexpected summary %+v", summary)
			}
			result := summary.Results[0]
			if (result.Result == "通过") != scenario.passed || result.ActualModel != scenario.model {
				t.Fatalf("unexpected probe result %+v", result)
			}
			if scenario.passed && scenario.contentType != "application/json" && result.LatencyP50 == nil {
				t.Fatal("successful probe lost first-content latency")
			}
			if scenario.passed && scenario.contentType == "application/json" && result.LatencyP50 != nil {
				t.Fatal("non-streamed response duration was recorded as first-content latency")
			}
			if !scenario.passed && (result.LatencyP50 != nil || result.FailureReason == nil || *result.FailureReason == "") {
				t.Fatalf("failed probe missing reason or recording latency: %+v", result)
			}
			raw, _ := json.Marshal(result)
			if strings.Contains(string(raw), "secret") {
				t.Fatal("probe result exposed a credential fragment")
			}
		})
	}
}
