package probe_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
)

func TestStreamCapacityFailureUsesSemanticStatusAndConfiguredRetry(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		event      string
		retry      bool
		wantStatus int
	}{
		{name: "nested status is preserved", event: `{"response":{"error":{"code":"server_error","message":"Our servers are currently overloaded. Please try again later.","status_code":503,"type":"server_error"},"id":"resp_overloaded","status":"failed"},"sequence_number":2,"type":"response.failed"}`},
		{name: "busy without status uses 503", event: `{"request_id":"busy-request","response":{"error":{"code":"server_error","message":"The service is busy. Please retry later.","type":"server_error"},"model":"gpt-5.6-sol","status":"failed"},"sequence_number":3,"type":"response.failed"}`},
		{name: "nested status permits configured retry", event: `{"type":"response.failed","response":{"error":{"status_code":503,"message":"Upstream temporarily unavailable"},"status":"failed"}}`, retry: true},
		{name: "busy without status permits configured retry", event: `{"type":"response.failed","response":{"error":{"code":"server_error","message":"The service is busy. Please retry later."},"status":"failed"}}`, retry: true},
		{name: "explicit authentication status does not permit 503 retry", event: `{"type":"response.failed","response":{"error":{"status_code":401,"message":"The service is busy. Please retry later."},"status":"failed"}}`, retry: true, wantStatus: 401},
		{name: "unrecognized failure keeps transport status without guessing", event: `{"type":"response.failed","response":{"error":{"status_code":"invalid","message":"Generation rejected"},"status":"failed"}}`, retry: true, wantStatus: 200},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := directProbeStore(t)
			if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"probe": map[string]any{
				"retry_enabled": scenario.retry, "retry_source": "fixed", "retry_count": 1, "retry_status_codes": []any{503},
			}}}, "test"); err != nil {
				t.Fatal(err)
			}
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodGet && request.URL.Path == "/api/v1/admin/accounts/41" {
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
						"id": 41, "type": "apikey", "platform": "openai",
						"credentials": map[string]any{"api_key": "isolated-probe-key", "base_url": "http://" + request.Host},
					}})
					return
				}
				if request.Method != http.MethodPost || request.URL.Path != "/v1/responses" {
					t.Errorf("unexpected request %s %s", request.Method, request.URL.Path)
					http.NotFound(w, request)
					return
				}
				attempt := attempts.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				if scenario.retry && attempt == 2 {
					_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\n"))
					return
				}
				_, _ = w.Write([]byte("data: " + scenario.event + "\n\n"))
			}))
			t.Cleanup(server.Close)
			summary, err := probe.New(store, protectionTarget{endpoint: server.URL}, nil).RunNow(t.Context(), probe.Request{AccountID: pointer("41")})
			if err != nil {
				t.Fatal(err)
			}
			if len(summary.Results) != 1 || summary.Persisted != 1 {
				t.Fatalf("failure was not retained as one evidence result: %+v", summary)
			}
			result := summary.Results[0]
			if scenario.retry && scenario.wantStatus == 0 {
				if result.Result != "通过" || !result.RetryRecovered || attempts.Load() != 2 || !slices.Equal(result.AttemptStatusCodes, []int{503, 200}) {
					t.Fatalf("configured 503 retry did not recover: %+v", result)
				}
			} else {
				wantStatus := scenario.wantStatus
				if wantStatus == 0 {
					wantStatus = 503
				}
				if result.Result != "失败" || result.StatusCode == nil || *result.StatusCode != wantStatus || attempts.Load() != 1 || result.LatencyP50 != nil {
					t.Fatalf("HTTP 200 stream failure lost its semantic status: %+v", result)
				}
			}
		})
	}
}
