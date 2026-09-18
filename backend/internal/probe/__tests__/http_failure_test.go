package probe_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
)

func TestHTTPProbeFailurePreservesSpecificRedactedError(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"structured error on HTTP 200", 200, `{"error":{"code":"insufficient_balance"}}`, "insufficient_balance"},
		{"rate limit", 429, `{"error":{"code":"rate_limit_exceeded","message":"Too many requests"}}`, "Too many requests"},
		{"balance on 429", 429, `{"error":{"code":"insufficient_balance","message":"余额不足"}}`, "余额不足"},
		{"unclassified quota", 429, `{"error":{"message":"Quota policy Q17 rejected this request"}}`, "Quota policy Q17 rejected this request"},
		{"plain text", 403, "Account review pending", "Account review pending"},
		{"no detail", 429, "", "上游未返回错误详情"},
		{"credential echo", 429, `{"error":{"message":"余额不足 isolated-secret test-key"}}`, "余额不足"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := directProbeStore(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts/41" {
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 41, "type": "apikey", "platform": "openai", "credentials": map[string]any{"api_key": "isolated-secret", "base_url": "http://" + r.Host}}})
					return
				}
				if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
					t.Errorf("unexpected path: %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				w.WriteHeader(scenario.status)
				_, _ = w.Write([]byte(scenario.body))
			}))
			t.Cleanup(server.Close)
			result, err := probe.New(store, protectionTarget{endpoint: server.URL}, nil).RunNow(t.Context(), probe.Request{AccountID: pointer("41")})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Results) != 1 || result.Results[0].FailureReason == nil {
				t.Fatalf("missing failure: %+v", result)
			}
			reason := *result.Results[0].FailureReason
			if !strings.Contains(reason, scenario.want) || (scenario.status >= 300 && !strings.Contains(reason, fmt.Sprintf("HTTP %d", scenario.status))) {
				t.Fatalf("lost upstream detail: %s", reason)
			}
			if strings.Contains(reason, "isolated-secret") || strings.Contains(reason, "test-key") || strings.Contains(reason, "限流或") {
				t.Fatalf("unsafe or ambiguous error: %s", reason)
			}
			if result.Persisted != 1 || result.Results[0].StatusCode == nil || *result.Results[0].StatusCode != scenario.status {
				t.Fatalf("lost evidence or status: %+v", result)
			}
		})
	}
}
