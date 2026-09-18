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

func TestProbeModelMappingRecordsOnlyExecutedGeneration(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mapping map[string]string
		passed  int
		skipped int
	}{
		{"valid_wildcard_records_success", map[string]string{"probe-*": "grok-4", "other[1m]": "other[1m]"}, 1, 0},
		{"unrelated_wildcard_target_records_success", map[string]string{"probe-model": "grok-4", "claude-*": "claude-*", "gpt-*": "gpt-*"}, 1, 0},
		{"invalid_target_skips_without_health_failure", map[string]string{"probe-*": "*"}, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := directProbeStore(t)
			var generated atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				generated.Add(1)
				var body struct {
					Model string `json:"model"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body.Model != "grok-4" || r.URL.Path != "/v1/chat/completions" {
					t.Errorf("unexpected generation: model=%q path=%q", body.Model, r.URL.Path)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"model\":\"grok-4\",\"choices\":[{\"delta\":{\"content\":\"pong\"}}]}\n\ndata: [DONE]\n\n")
			}))
			t.Cleanup(upstream.Close)
			admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/accounts/41" {
					t.Error("probe must only read the account by stable ID")
					http.NotFound(w, r)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
					"id": 41, "type": "apikey", "platform": "grok",
					"credentials": map[string]any{"api_key": "test-only-key", "base_url": upstream.URL, "model_mapping": tc.mapping},
				}})
			}))
			t.Cleanup(admin.Close)

			summary, err := probe.New(store, protectionTarget{endpoint: admin.URL}, nil).RunNow(context.Background(), probe.Request{AccountID: pointer("41")})
			if err != nil {
				t.Fatal(err)
			}
			if summary.Passed != tc.passed || summary.Skipped != tc.skipped || summary.Failed != 0 || summary.Persisted != tc.passed || int(generated.Load()) != tc.passed {
				t.Fatalf("unexpected mapping outcome: summary=%+v requests=%d", summary, generated.Load())
			}
			if tc.skipped == 1 && (summary.Results[0].Attempts != 0 || summary.Results[0].FailureCode != "probe_mapping_unsupported") {
				t.Fatalf("invalid mapping must preserve its skip reason without attempts: %+v", summary.Results[0])
			}
		})
	}
}
