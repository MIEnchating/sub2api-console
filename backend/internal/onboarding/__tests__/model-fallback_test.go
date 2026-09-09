package onboarding_test

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
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/onboarding"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

const testKey = "isolated-onboarding-key"

type keyClient struct{ creates, reveals int }

func (k *keyClient) CreateKey(_ context.Context, _ configstore.AuthRecord, name, _ string) (upstreamsync.CreatedKey, error) {
	k.creates++
	return upstreamsync.CreatedKey{KeyID: "91", Name: name, Secret: testKey}, nil
}
func (k *keyClient) RevealKey(_ context.Context, _ configstore.AuthRecord, _, _ string) (upstreamsync.CreatedKey, error) {
	k.reveals++
	return upstreamsync.CreatedKey{KeyID: "91", Name: "existing-key", Secret: testKey}, nil
}

// Both HTTP endpoints and both databases belong exclusively to each test.
func newService(t *testing.T, adminURL, baseURL string) (*onboarding.Service, *business.Store, *keyClient, onboarding.Request) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "business.sqlite3")
	repo, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err := repo.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateUpstreamConfiguration(ctx, business.UpstreamConfigurationWrite{Host: "upstream.test", BaseURL: baseURL, UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1"}); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, query := range []string{
		`INSERT INTO upstream_groups(host,group_id,name,platform,status,raw_rate,effective_rate,updated_at) VALUES('upstream.test','6','Gemini','gemini','active','0.2','0.2','now')`,
		`INSERT INTO local_groups(name,remote_id,strategy,strategy_source,platform,updated_at) VALUES('gemini-平价','3','balanced','global_default','gemini','now')`,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	private, err := configstore.Open(filepath.Join(dir, "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Close() })
	if err := private.ConfigureTarget(ctx, adminURL, "isolated-admin-key", 5); err != nil {
		t.Fatal(err)
	}
	// The explicit account URL must override this otherwise unreachable auth URL.
	token := "isolated-user-token"
	if err := private.SaveAuthRecord(ctx, configstore.AuthRecord{Host: "upstream.test", BaseURL: "http://127.0.0.1:1", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", AccessToken: &token}, nil); err != nil {
		t.Fatal(err)
	}
	keys := &keyClient{}
	return onboarding.New(repo, private, keys, nil), repo, keys, onboarding.Request{Host: "upstream.test", UpstreamType: "sub2api", UpstreamGroupID: "6", LocalGroupID: "3", BaseURL: &baseURL, Actor: "test"}
}

func TestModelDiscoveryPrefersPreviewAndFallsBackToAccountURLOnFailure(t *testing.T) {
	for _, tc := range []struct {
		name          string
		status        int
		body          string
		prefix        string
		fallbackCalls int
	}{
		{"bad gateway", 502, `{"message":"unavailable"}`, "", 1},
		{"unsupported preview", 400, `{"message":"unsupported"}`, "/gateway", 1},
		{"invalid preview JSON", 200, `<html>unavailable</html>`, "", 1},
		{"empty preview models", 200, `{"data":{"models":[]}}`, "/gateway/v1", 1},
		{"preview succeeds without fallback", 200, `{"data":{"models":["gemini-2.5-flash","gemini-2.5-pro"]}}`, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fallbackCalls := 0
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fallbackCalls++
				expectedPath := strings.TrimSuffix(tc.prefix, "/v1") + "/v1/models"
				if r.Method != "GET" || r.URL.Path != expectedPath || r.Header.Get("Authorization") != "Bearer "+testKey {
					t.Errorf("unexpected fallback request: %s %s", r.Method, r.URL.Path)
					http.Error(w, "invalid request", 400)
					return
				}
				if r.Header.Get("X-API-Key") != "" {
					t.Error("admin key must not reach upstream")
				}
				_, _ = w.Write([]byte(`{"data":[{"id":"gemini-2.5-flash"},{"id":"gemini-2.5-pro"},{"id":"gemini-2.5-flash"}]}`))
			}))
			defer gateway.Close()
			posts := 0
			admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/api/v1/admin/accounts/models/sync-upstream-preview" {
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
					return
				}
				if r.Method == "POST" && r.URL.Path == "/api/v1/admin/accounts" {
					posts++
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
						return
					}
					credentials, _ := body["credentials"].(map[string]any)
					mapping, _ := credentials["model_mapping"].(map[string]any)
					if len(mapping) != 2 || mapping["gemini-2.5-flash"] != "gemini-2.5-flash" || mapping["gemini-2.5-pro"] != "gemini-2.5-pro" {
						t.Errorf("unexpected model whitelist: %v", mapping)
					}
					body["id"] = 77
					_ = json.NewEncoder(w).Encode(map[string]any{"data": body})
					return
				}
				http.NotFound(w, r)
			}))
			defer admin.Close()
			service, repo, keys, request := newService(t, admin.URL, gateway.URL+tc.prefix)
			result, err := service.Onboard(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if result["account_id"] != "77" || result["model_count"] != 2 || posts != 1 || fallbackCalls != tc.fallbackCalls || keys.creates != 1 {
				t.Fatalf("unexpected result=%v posts=%d fallback=%d creates=%d", result, posts, fallbackCalls, keys.creates)
			}
			pending, err := repo.PendingOnboarding(context.Background(), "upstream.test", "6", []string{"3"})
			if err != nil || pending != nil {
				t.Fatalf("completed onboarding left pending record: %v %v", pending, err)
			}
		})
	}
}

func TestBothModelSourcesFailPreservesKeyForFallbackRetry(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"empty models", 200, `{"data":[]}`},
		{"HTML success", 200, `<html>域名已停用</html>`},
		{"trailing JSON", 200, `{"data":[{"id":"gemini-2.5-flash"}]} {}`},
		{"business failure with models", 200, `{"success":false,"data":[{"id":"gemini-2.5-flash"}]}`},
		{"unauthorized", 401, `{"error":"invalid isolated-onboarding-key"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var recovered atomic.Bool
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if recovered.Load() {
					_, _ = w.Write([]byte(`{"data":[{"id":"gemini-2.5-flash"}]}`))
					return
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer gateway.Close()
			posts := 0
			admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/admin/accounts/models/sync-upstream-preview" {
					http.Error(w, `{"message":"preview unavailable"}`, 502)
					return
				}
				if r.URL.Path == "/api/v1/admin/accounts" && r.Method == "POST" {
					posts++
					var body map[string]any
					_ = json.NewDecoder(r.Body).Decode(&body)
					body["id"] = 77
					_ = json.NewEncoder(w).Encode(map[string]any{"data": body})
					return
				}
				http.NotFound(w, r)
			}))
			defer admin.Close()
			service, repo, keys, request := newService(t, admin.URL, gateway.URL)
			result, err := service.Onboard(context.Background(), request)
			if err == nil || !strings.Contains(err.Error(), "preview unavailable") || !strings.Contains(err.Error(), "/v1/models") || strings.Contains(err.Error(), testKey) || posts != 0 || result["pending"] == nil {
				t.Fatalf("failure must preserve both safe causes and avoid account creation: result=%v err=%v posts=%d", result, err, posts)
			}
			pending, err := repo.PendingOnboarding(context.Background(), "upstream.test", "6", []string{"3"})
			if err != nil || pending == nil || pending.UpstreamKeyID != "91" {
				t.Fatalf("missing reusable key: %v %v", pending, err)
			}
			recovered.Store(true)
			result, err = service.Onboard(context.Background(), request)
			if err != nil || result["account_id"] != "77" || keys.creates != 1 || keys.reveals != 1 || posts != 1 {
				t.Fatalf("retry must reuse key: result=%v err=%v keys=%+v posts=%d", result, err, keys, posts)
			}
		})
	}
}

func TestCancelledPreviewDoesNotStartFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var fallbackCalls atomic.Int32
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackCalls.Add(1)
		_, _ = w.Write([]byte(`{"data":[{"id":"gemini-2.5-flash"}]}`))
	}))
	defer gateway.Close()
	admin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/admin/accounts/models/sync-upstream-preview" {
			cancel()
			http.Error(w, "cancelled", 502)
			return
		}
		t.Errorf("unexpected management request: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}))
	defer admin.Close()
	service, _, _, request := newService(t, admin.URL, gateway.URL)
	_, err := service.Onboard(ctx, request)
	if err == nil || !strings.Contains(err.Error(), "context canceled") || fallbackCalls.Load() != 0 {
		t.Fatalf("cancelled onboarding must not start fallback: err=%v calls=%d", err, fallbackCalls.Load())
	}
}
