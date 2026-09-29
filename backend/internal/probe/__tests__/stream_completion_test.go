package probe_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
)

const probeTextEvent = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\n"
const probeCompletedEvent = "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n"

func TestProbeChecksCompletionAfterReceivingText(t *testing.T) {
	for _, scenario := range []struct {
		name, platform, body, reason string
		passed                       bool
	}{
		{name: "Responses completes after text", body: probeTextEvent + probeCompletedEvent, passed: true},
		{name: "Responses terminal event alone contains complete text", body: "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"pong\"}]}]}}\n\n", passed: true},
		{name: "Responses SSE event name carries completion type", body: probeTextEvent + "event: response.completed\ndata: {\"response\":{\"status\":\"completed\"}}\n\n", passed: true},
		{name: "Responses fails after text", body: probeTextEvent + "data: {\"type\":\"response.failed\",\"response\":{\"error\":{\"message\":\"generation failed\",\"status_code\":503}}}\n\n", reason: "generation failed"},
		{name: "Responses incomplete after text", body: probeTextEvent + "data: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\"}}\n\n", reason: "incomplete"},
		{name: "Responses completed envelope contains failure", body: probeTextEvent + "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"failed\",\"error\":{\"message\":\"nested failure\"}}}\n\n", reason: "nested failure"},
		{name: "Responses EOF after text is incomplete", body: probeTextEvent, reason: "未完整结束"},
		{name: "Responses DONE cannot replace completion", body: probeTextEvent + "data: [DONE]\n\n", reason: "未完整结束"},
		{name: "Malformed event after text fails", body: probeTextEvent + "data: not-json\n\n", reason: "无效 JSON"},
		{name: "Oversized stream after text fails before completion", body: probeTextEvent + strings.Repeat(": padding\n", (4<<20)/10+1) + probeCompletedEvent, reason: "过大"},
		{name: "Anthropic completes after text", platform: "anthropic", body: "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"pong\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n", passed: true},
		{name: "Anthropic token limit fails after text", platform: "anthropic", body: "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"pong\"}}\n\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"max_tokens\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n", reason: "max_tokens"},
		{name: "Chat DONE completes text", platform: "grok", body: "data: {\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\ndata: [DONE]\n\n", passed: true},
		{name: "Chat EOF without completion fails", platform: "grok", body: "data: {\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\n", reason: "未完整结束"},
		{name: "Chat stop and normal EOF complete text", platform: "grok", body: "data: {\"choices\":[{\"delta\":{\"content\":\"pong\"},\"finish_reason\":\"stop\"}]}\n\n", passed: true},
		{name: "Chat error after stop still fails", platform: "grok", body: "data: {\"choices\":[{\"delta\":{\"content\":\"pong\"},\"finish_reason\":\"stop\"}]}\n\ndata: {\"error\":{\"message\":\"late failure\"}}\n\ndata: [DONE]\n\n", reason: "late failure"},
		{name: "Chat length limit fails", platform: "grok", body: "data: {\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\ndata: {\"choices\":[{\"delta\":{},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n\n", reason: "length"},
		{name: "Chat checks usage block before DONE", platform: "grok", body: "data: {\"choices\":[{\"delta\":{\"content\":\"pong\"},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"completion_tokens\":1}}\n\ndata: [DONE]\n\n", passed: true},
		{name: "Gemini STOP completes text", platform: "gemini", body: "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"pong\"}]},\"finishReason\":\"STOP\"}]}\n\n", passed: true},
		{name: "Gemini safety failure after text fails", platform: "gemini", body: "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"pong\"}]}}]}\n\ndata: {\"candidates\":[{\"finishReason\":\"SAFETY\"}]}\n\n", reason: "SAFETY"},
		{name: "Completion without text fails", body: probeCompletedEvent, reason: "未返回有效文本"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := directProbeStore(t)
			_, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"probe": map[string]any{
				"retry_enabled": true, "retry_source": "fixed", "retry_count": 1, "retry_status_codes": []any{503},
			}}}, "test")
			if err != nil {
				t.Fatal(err)
			}
			var generated atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				generated.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, scenario.body)
			}))
			t.Cleanup(upstream.Close)
			platform := scenario.platform
			if platform == "" {
				platform = "openai"
			}
			admin := probeCompletionAdmin(t, platform, upstream.URL)
			summary, err := probe.New(store, protectionTarget{endpoint: admin.URL}, nil).RunNow(t.Context(), probe.Request{AccountID: pointer("41")})
			if err != nil || len(summary.Results) != 1 {
				t.Fatalf("probe result: %+v, %v", summary, err)
			}
			result := summary.Results[0]
			if (result.Result == "通过") != scenario.passed || generated.Load() != 1 {
				t.Fatalf("completion result=%+v, requests=%d", result, generated.Load())
			}
			if scenario.reason != "" && (result.FailureReason == nil || !strings.Contains(*result.FailureReason, scenario.reason)) {
				t.Fatalf("expected failure containing %q, got %+v", scenario.reason, result)
			}
			if scenario.reason == "generation failed" && (result.StatusCode == nil || *result.StatusCode != 503) {
				t.Fatalf("late upstream error status was lost: %+v", result)
			}
			samples, err := store.RoutingSamples(context.Background(), pointer("41"), nil, "active-probe", 10)
			if err != nil || len(samples) != 1 || (samples[0].Result == "通过") != scenario.passed {
				t.Fatalf("completion must determine persisted health evidence: %+v, %v", samples, err)
			}
			if !scenario.passed && (result.LatencyP50 != nil || result.MeasuredFirstToken) {
				t.Fatalf("failed generation must not enter successful latency statistics: %+v", result)
			}
		})
	}
}

func probeCompletionAdmin(t *testing.T, platform, upstreamURL string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/accounts/41" {
			t.Errorf("unexpected management request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"id": 41, "type": "apikey", "platform": platform,
			"credentials": map[string]any{"api_key": "completion-test-key", "base_url": upstreamURL},
		}})
	}))
	t.Cleanup(server.Close)
	return server
}
