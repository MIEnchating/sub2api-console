package probe_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
)

func TestProbeReturnsJoinedOutputAndRedactsCredentialAcrossChunks(t *testing.T) {
	secret := strings.Repeat("secret", 10)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, part := range []string{"你好，", secret[:20], secret[20:], "连接正常。"} {
			fmt.Fprintf(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":%q}\n\n", part)
		}
		fmt.Fprint(w, probeCompletedEvent)
	}))
	defer upstream.Close()
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 41, "type": "apikey", "platform": "openai", "credentials": map[string]any{"api_key": secret, "base_url": upstream.URL}}})
	}))
	defer admin.Close()
	summary, err := probe.New(directProbeStore(t), protectionTarget{endpoint: admin.URL}, nil).RunNow(context.Background(), probe.Request{AccountID: pointer("41")})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(summary.Results[0])
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		ResponseText string `json:"response_text"`
	}
	json.Unmarshal(raw, &result)
	if result.ResponseText != "你好，[已隐藏]连接正常。" {
		t.Fatalf("unexpected redacted response: %s", result.ResponseText)
	}
}
