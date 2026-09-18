package probe_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
)

type performanceProbeCase struct {
	protocol    string
	prompt      string
	model       string
	body        string
	contentType string
	retry       bool
}

func collectPerformanceProbe(t *testing.T, test performanceProbeCase) business.RoutingSample {
	t.Helper()
	store := directProbeStore(t)
	_, err := store.UpdatePolicy(context.Background(), map[string]any{"advanced_policy": map[string]any{"probe": map[string]any{
		"prompt": test.prompt, "retry_enabled": test.retry, "retry_source": "fixed", "retry_count": 1,
		"retry_status_codes": []any{503},
	}}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if test.model != "" {
		if err := store.SetAccountTestModels(context.Background(), "41", []string{test.model}, "test"); err != nil {
			t.Fatal(err)
		}
	}
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 && test.retry {
			http.Error(w, "temporarily unavailable", 503)
			return
		}
		contentType := test.contentType
		if contentType == "" {
			contentType = "text/event-stream"
		}
		body := test.body
		if body == "" {
			body = "data: {\"model\":\"actual-model\",\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\n"
		}
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(upstream.Close)
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/accounts/41" {
			t.Errorf("unexpected management request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		credentials := map[string]any{"api_key": "private-probe-key", "base_url": upstream.URL}
		if test.protocol != "" {
			credentials["api_protocol"] = test.protocol
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 41, "type": "apikey", "platform": "openai", "credentials": credentials}})
	}))
	t.Cleanup(admin.Close)
	summary, err := probe.New(store, protectionTarget{endpoint: admin.URL}, nil).RunNow(context.Background(), probe.Request{Automatic: true, AccountIDs: []string{"41"}})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Passed != 1 || summary.Persisted != 1 {
		t.Fatalf("probe did not succeed: %+v", summary)
	}
	samples, err := store.RoutingSamples(context.Background(), pointer("41"), nil, "active-probe", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 {
		t.Fatalf("persisted samples=%d", len(samples))
	}
	return samples[0]
}

func TestStreamingProbePersistsComparablePerformanceWithoutPromptOrCredentials(t *testing.T) {
	for _, protocol := range []string{"responses", "chat_completions", "anthropic", "gemini"} {
		t.Run(protocol, func(t *testing.T) {
			sample := collectPerformanceProbe(t, performanceProbeCase{protocol: protocol, prompt: "private  prompt"})
			if sample.Payload["probe_protocol"] != protocol || sample.Payload["request_model"] != "probe-model" || sample.Payload["actual_model"] != "actual-model" || sample.Payload["measured_first_token"] != true || sample.Payload["performance_eligible"] != true {
				t.Fatalf("missing comparable probe identity: %+v", sample.Payload)
			}
			for _, key := range []string{"probe_prompt_fingerprint", "probe_request_fingerprint"} {
				value, ok := sample.Payload[key].(string)
				if !ok || !strings.HasPrefix(value, "v1:") || len(value) != 67 {
					t.Fatalf("invalid %s: %v", key, sample.Payload[key])
				}
			}
			raw, err := json.Marshal(sample.Payload)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "private  prompt") || strings.Contains(string(raw), "private-probe-key") {
				t.Fatal("performance identity exposed private request values")
			}
		})
	}
}

func TestProbeWithoutComparableFirstTokenKeepsHealthSuccessButCannotRank(t *testing.T) {
	for _, test := range []struct {
		name     string
		input    performanceProbeCase
		measured bool
	}{
		{name: "actual model missing", input: performanceProbeCase{body: "data: {\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\n"}, measured: true},
		{name: "complete JSON", input: performanceProbeCase{contentType: "application/json", body: `{"model":"actual-model","choices":[{"message":{"content":"pong"}}]}`}},
		{name: "retry recovered", input: performanceProbeCase{retry: true}, measured: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			sample := collectPerformanceProbe(t, test.input)
			if sample.Payload["performance_eligible"] != false || sample.Payload["measured_first_token"] != test.measured {
				t.Fatalf("incomparable sample can rank: %+v", sample.Payload)
			}
		})
	}
}

func TestProbePerformanceFingerprintsTrackExactPromptModelAndProtocol(t *testing.T) {
	base := collectPerformanceProbe(t, performanceProbeCase{prompt: "One  word"})
	same := collectPerformanceProbe(t, performanceProbeCase{prompt: "  One  word  "})
	if base.Payload["probe_request_fingerprint"] == nil {
		t.Fatal("request fingerprint was not persisted")
	}
	if base.Payload["probe_request_fingerprint"] != same.Payload["probe_request_fingerprint"] {
		t.Fatal("identical outgoing requests cannot be compared across accounts")
	}
	for _, test := range []struct {
		name          string
		input         performanceProbeCase
		promptChanged bool
	}{
		{name: "prompt case", input: performanceProbeCase{prompt: "one  word"}, promptChanged: true},
		{name: "prompt inner spacing", input: performanceProbeCase{prompt: "One word"}, promptChanged: true},
		{name: "request model", input: performanceProbeCase{prompt: "One  word", model: "other-model"}},
		{name: "wire protocol", input: performanceProbeCase{prompt: "One  word", protocol: "chat_completions"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := collectPerformanceProbe(t, test.input)
			if base.Payload["probe_request_fingerprint"] == changed.Payload["probe_request_fingerprint"] {
				t.Fatal("changed outgoing request reused performance identity")
			}
			if (base.Payload["probe_prompt_fingerprint"] != changed.Payload["probe_prompt_fingerprint"]) != test.promptChanged {
				t.Fatal("prompt fingerprint does not reflect exact sent prompt")
			}
		})
	}
}
