package probe_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
)

func directProbeStore(t *testing.T) *business.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "direct-probe.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','probe account','{"known_models":["probe-model"]}','now'); INSERT INTO account_groups(account_id,group_name) VALUES('41','codex')`); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestDirectProbeUsesAccountCredentialAndConfiguredPromptWithoutAdminTest(t *testing.T) {
	store := directProbeStore(t)
	_, err := store.UpdatePolicy(context.Background(), map[string]any{"advanced_policy": map[string]any{"probe": map[string]any{"prompt": "Reply only with pong", "retry_enabled": false}}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	var generated, tested atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		generated.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" || r.Header.Get("Authorization") != "Bearer bound-secret" || r.Header.Get("X-API-Key") != "" {
			t.Errorf("unexpected direct request method=%s path=%s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if body["model"] != "probe-model" || body["input"] != "Reply only with pong" || body["stream"] != true || body["instructions"] != nil || body["system"] != nil {
			t.Errorf("unexpected probe body: %#v", body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"model\":\"probe-model\"}}\n\nevent: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\n"))
	}))
	defer upstream.Close()
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/accounts/41" {
			tested.Add(1)
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Error("management credential missing")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": 41, "type": "apikey", "platform": "openai", "credentials": map[string]any{"api_key": "bound-secret", "base_url": upstream.URL}}})
	}))
	defer admin.Close()
	service := probe.New(store, protectionTarget{endpoint: admin.URL}, nil)
	summary, err := service.RunNow(context.Background(), probe.Request{AccountID: pointer("41")})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Passed != 1 || summary.Persisted != 1 || generated.Load() != 1 || tested.Load() != 0 {
		t.Fatalf("direct probe failed: summary=%+v generated=%d adminTest=%d", summary, generated.Load(), tested.Load())
	}
	raw, _ := json.Marshal(summary)
	if strings.Contains(string(raw), "bound-secret") || strings.Contains(string(raw), "test-key") {
		t.Fatal("probe summary exposed credentials")
	}
}

func TestUnavailableDirectProbeSkipsWithoutPersistingHealthFailure(t *testing.T) {
	for _, scenario := range []string{"oauth", "missing_key", "identity_mismatch", "management_failure"} {
		t.Run(scenario, func(t *testing.T) {
			store := directProbeStore(t)
			var generated atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				generated.Add(1)
				http.Error(w, "unexpected generation", 500)
			}))
			defer upstream.Close()
			admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/admin/accounts/41" {
					http.NotFound(w, r)
					return
				}
				if scenario == "management_failure" {
					http.Error(w, "management unavailable", 503)
					return
				}
				row := map[string]any{"id": 41, "type": "apikey", "platform": "openai", "credentials": map[string]any{"base_url": upstream.URL, "api_key": "private-key"}}
				if scenario == "oauth" {
					row["type"] = "oauth"
				}
				if scenario == "missing_key" {
					row["credentials"] = map[string]any{"base_url": upstream.URL}
				}
				if scenario == "identity_mismatch" {
					row["id"] = 42
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": row})
			}))
			defer admin.Close()
			summary, err := probe.New(store, protectionTarget{endpoint: admin.URL}, nil).RunNow(context.Background(), probe.Request{AccountID: pointer("41")})
			if err != nil {
				t.Fatal(err)
			}
			if summary.Skipped != 1 || summary.Failed != 0 || summary.Persisted != 0 || generated.Load() != 0 {
				t.Fatalf("unavailable probe affected health: %+v", summary)
			}
			result := summary.Results[0]
			if result.Attempts != 0 || result.FailureReason == nil || *result.FailureReason == "" {
				t.Fatalf("missing actionable skip reason: %+v", result)
			}
		})
	}
}

func pointer(value string) *string { return &value }
