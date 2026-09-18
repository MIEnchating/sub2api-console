package probe_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
)

func TestGrokProbeRecordsGenerationOutcomeInsteadOfSkippingPlatform(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		stream string
		passed int
		failed int
	}{
		{name: "text_response_passes", stream: "data: {\"model\":\"grok-test\",\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\ndata: [DONE]\n\n", passed: 1},
		{name: "business_error_fails", stream: "data: {\"error\":{\"message\":\"model unavailable\",\"type\":\"invalid_request_error\"}}\n\n", failed: 1},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := directProbeStore(t)
			_, err := store.UpdatePolicy(context.Background(), map[string]any{"advanced_policy": map[string]any{"probe": map[string]any{"retry_enabled": false}}}, "test")
			if err != nil {
				t.Fatal(err)
			}
			var generated atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				generated.Add(1)
				if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer grok-test-secret" {
					t.Error("Grok probe must use authenticated Chat Completions")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, scenario.stream)
			}))
			t.Cleanup(upstream.Close)
			admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/accounts/41" {
					t.Error("probe must only read the account by stable ID from management")
					http.NotFound(w, r)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
					"id": 41, "type": "apikey", "platform": "grok",
					"credentials": map[string]any{"api_key": "grok-test-secret", "base_url": upstream.URL + "/v1"},
				}})
			}))
			t.Cleanup(admin.Close)

			summary, err := probe.New(store, protectionTarget{endpoint: admin.URL}, nil).RunNow(context.Background(), probe.Request{AccountID: pointer("41")})
			if err != nil {
				t.Fatal(err)
			}
			if summary.Skipped != 0 || summary.Passed != scenario.passed || summary.Failed != scenario.failed || summary.Persisted != 1 || generated.Load() != 1 {
				t.Fatalf("Grok generation outcome must be recorded: summary=%+v requests=%d", summary, generated.Load())
			}
		})
	}
}
