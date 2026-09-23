package probe_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
)

func TestModelSyncManualBatchProbesSelectedModelOutsideAutomaticScope(t *testing.T) {
	store := directProbeStore(t)
	now := time.Now().UTC()
	_, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{
		"scope": map[string]any{"managed_group_mode": "selected", "managed_group_ids": []any{"7"}},
		"probe": map[string]any{"enabled": false, "retry_enabled": false, "pause_window": map[string]any{"enabled": true, "start": now.Add(-time.Hour).Format("15:04"), "end": now.Add(time.Hour).Format("15:04"), "timezone": "UTC"}},
	}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	var generated atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts/41" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 41, "type": "apikey", "platform": "openai", "credentials": map[string]any{"base_url": "http://" + r.Host, "api_key": "test-only"}}})
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/v1/responses" {
			generated.Add(1)
			var body struct {
				Model string `json:"model"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Model != "probe-model" {
				t.Errorf("model=%s", body.Model)
			}
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\n"))
			return
		}
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}))
	defer server.Close()
	service := probe.New(store, protectionTarget{endpoint: server.URL}, nil)
	summary, err := service.RunNow(t.Context(), probe.Request{SelectedAccountIDs: []string{"41"}, ProbeModel: "probe-model", OnePerAccount: true})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Passed != 1 || generated.Load() != 1 {
		t.Fatalf("summary=%+v generated=%d", summary, generated.Load())
	}
}

func TestModelSyncManualBatchSkipsUnsupportedModelWithoutGeneration(t *testing.T) {
	store := directProbeStore(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unsupported model must not contact upstream: %s", r.URL.Path)
		http.NotFound(w, r)
	}))
	defer server.Close()
	summary, err := probe.New(store, protectionTarget{endpoint: server.URL}, nil).RunNow(t.Context(), probe.Request{SelectedAccountIDs: []string{"41"}, ProbeModel: "unsupported", OnePerAccount: true})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Skipped != 1 || summary.Failed != 0 || summary.Persisted != 0 {
		t.Fatalf("summary=%+v", summary)
	}
}
