package probe_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
)

func TestPlaceholderMarkerPersistsAcrossStreamingProtocols(t *testing.T) {
	for _, scenario := range []struct{ protocol, prefix, tail string }{
		{"responses", `{"type":"response.output_text.delta","delta":"__-"}`, probeTextEvent + probeCompletedEvent},
		{"chat_completions", `{"choices":[{"delta":{"content":"__-"}}]}`, "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\ndata: [DONE]\n\n"},
		{"anthropic", `{"type":"content_block_start","content_block":{"type":"text","text":"__-"}}`, "data: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"42\"}}\n\ndata: {\"type\":\"message_stop\"}\n\n"},
		{"gemini", `{"candidates":[{"content":{"parts":[{"text":"__-"}]}}]}`, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"你好\"}]},\"finishReason\":\"STOP\"}]}\n\n"},
	} {
		t.Run(scenario.protocol, func(t *testing.T) {
			sample := collectPerformanceProbe(t, performanceProbeCase{protocol: scenario.protocol, body: "data: " + scenario.prefix + "\n\n" + scenario.tail})
			if sample.Payload["ignored_placeholder"] != true {
				t.Fatalf("marker missing: %+v", sample.Payload)
			}
		})
	}
}

func TestCleanProbeDoesNotMarkFormattingAfterFirstText(t *testing.T) {
	sample := collectPerformanceProbe(t, performanceProbeCase{body: probeTextEvent + "data: {\"type\":\"response.output_text.delta\",\"delta\":\"__-\"}\n\n" + probeCompletedEvent})
	if sample.Payload["ignored_placeholder"] != false {
		t.Fatalf("normal trailing punctuation marked: %+v", sample.Payload)
	}
}

func TestProbeIgnoresPlaceholderUntilMeaningfulText(t *testing.T) {
	for _, text := range []string{"_", "__-", " \t---___\n", "...！？", "\u200b\ufeff"} {
		t.Run(text, func(t *testing.T) {
			store := directProbeStore(t)
			encoded, _ := json.Marshal(text)
			body := &probeStreamBody{chunks: []probeStreamChunk{
				{after: 100 * time.Millisecond, data: "data: {\"type\":\"response.output_text.delta\",\"delta\":" + string(encoded) + "}\n\n"},
				{after: 900 * time.Millisecond, data: probeTextEvent},
				{after: time.Second, data: probeCompletedEvent},
			}}
			original := http.DefaultTransport
			http.DefaultTransport = probeLifecycleTransport{body: body}
			t.Cleanup(func() { http.DefaultTransport = original })
			synctest.Test(t, func(t *testing.T) {
				summary, err := probe.New(store, protectionTarget{endpoint: "https://probe-lifecycle.test"}, nil).RunNow(t.Context(), probe.Request{AccountID: pointer("41")})
				if err != nil || len(summary.Results) != 1 {
					t.Fatalf("result: %+v, %v", summary, err)
				}
				result := summary.Results[0]
				if result.Result != "通过" || result.LatencyP50 == nil || *result.LatencyP50 != "1000" {
					t.Fatalf("placeholder gained first-token credit: %+v", result)
				}
				samples, err := store.RoutingSamples(t.Context(), pointer("41"), nil, "active-probe", 10)
				if err != nil || len(samples) != 1 || samples[0].Payload["ignored_placeholder"] != true {
					t.Fatalf("missing persisted placeholder marker: %+v, %v", samples, err)
				}
			})
		})
	}
}

func TestProbePlaceholderOnlyCannotSucceedOrGainLatency(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		tail   probeStreamChunk
		result string
	}{
		{"completed", probeStreamChunk{data: probeCompletedEvent}, "失败"},
		{"timeout", probeStreamChunk{wait: true}, "超时"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := directProbeStore(t)
			body := &probeStreamBody{chunks: []probeStreamChunk{{data: "data: {\"type\":\"response.output_text.delta\",\"delta\":\"__-\"}\n\n"}, scenario.tail}}
			original := http.DefaultTransport
			http.DefaultTransport = probeLifecycleTransport{body: body}
			t.Cleanup(func() { http.DefaultTransport = original })
			synctest.Test(t, func(t *testing.T) {
				summary, err := probe.New(store, protectionTarget{endpoint: "https://probe-lifecycle.test"}, nil).RunNow(t.Context(), probe.Request{AccountID: pointer("41")})
				if err != nil || len(summary.Results) != 1 {
					t.Fatalf("result: %+v, %v", summary, err)
				}
				result := summary.Results[0]
				if result.Result != scenario.result || result.LatencyP50 != nil || result.MeasuredFirstToken {
					t.Fatalf("placeholder was accepted: %+v", result)
				}
				if scenario.name == "completed" && (result.FailureReason == nil || !strings.Contains(*result.FailureReason, "占位")) {
					t.Fatalf("missing actionable reason: %+v", result)
				}
			})
		})
	}
}
