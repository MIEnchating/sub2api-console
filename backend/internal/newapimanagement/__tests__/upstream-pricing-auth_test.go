package newapimanagement_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
)

func TestUpstreamPricingUsesConfiguredAuthentication(t *testing.T) {
	access, admin, userID := "stale-access", "admin-key", "7"
	for _, tc := range []struct {
		name          string
		mode          string
		access        *string
		headers       map[string]string
		authorization string
		user          string
	}{
		{"cookie ignores stale bearer credentials", "cookie", &access, nil, "", ""},
		{"custom headers preserve explicit user", "custom_headers", &access, map[string]string{"Authorization": "Custom test-key", "New-Api-User": "42"}, "Custom test-key", "42"},
		{"custom headers do not inject bearer", "custom_headers", &access, map[string]string{"X-Api-Key": "custom-key"}, "", ""},
		{"token mode uses admin fallback", "newapi_user_token", nil, nil, "Bearer admin-key", "7"},
		{"admin mode uses admin key", "newapi_admin_key", &access, nil, "Bearer admin-key", "7"},
		{"token mode uses access token", "newapi_user_token", &access, nil, "Bearer stale-access", "7"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := upstreamPricingAuthSnapshot(t, configstore.AuthRecord{
				UpstreamType: "newapi", AuthMode: tc.mode, AccessToken: tc.access, AdminKey: &admin, UserID: &userID,
				Headers: tc.headers, Cookies: map[string]string{"session": "valid-session"},
			}, func(r *http.Request) (int, string) {
				if r.URL.Path != "/api/pricing" {
					t.Errorf("unexpected pricing path: %s", r.URL.Path)
				}
				cookie, err := r.Cookie("session")
				if err != nil || cookie.Value != "valid-session" {
					t.Error("configured session cookie not forwarded")
				}
				if r.Header.Get("Authorization") != tc.authorization || r.Header.Get("New-Api-User") != tc.user {
					return http.StatusUnauthorized, `{"message":"invalid authentication"}`
				}
				return http.StatusOK, `{"success":true,"data":[{"model_name":"model-a","quota_type":0,"model_ratio":1,"completion_ratio":2}]}`
			})
			if snapshot.UpstreamPriceWarning != "" || len(snapshot.UpstreamPrices) != 1 {
				t.Fatalf("configured authentication should read catalog: %s", snapshot.UpstreamPriceWarning)
			}
		})
	}
}

func TestUpstreamPricingUnknownPlatformDoesNotGuessSub2APIEndpoint(t *testing.T) {
	snapshot := upstreamPricingAuthSnapshot(t, configstore.AuthRecord{UpstreamType: "custom", AuthMode: "cookie"}, func(r *http.Request) (int, string) {
		t.Errorf("unsupported platform must not request guessed endpoint: %s", r.URL.Path)
		return http.StatusNotFound, `{}`
	})
	if !strings.Contains(snapshot.UpstreamPriceWarning, "平台类型") || !strings.Contains(snapshot.UpstreamPriceWarning, "不支持") {
		t.Fatalf("missing unsupported platform explanation: %s", snapshot.UpstreamPriceWarning)
	}
}

func TestUpstreamPricingFailureIdentifiesEndpointAndRecoveryAction(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		action string
	}{
		{"expired login", http.StatusUnauthorized, "鉴权恢复"},
		{"disabled plaza", http.StatusNotFound, "模型广场"},
		{"forbidden plaza", http.StatusForbidden, "访问权限"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := upstreamPricingAuthSnapshot(t, configstore.AuthRecord{UpstreamType: "sub2api", AuthMode: "sub2api_user_token"}, func(r *http.Request) (int, string) {
				return tc.status, `{"message":"private-response"}`
			})
			warning := snapshot.UpstreamPriceWarning
			for _, want := range []string{"upstream.test", "/api/v1/model-plaza", tc.action} {
				if !strings.Contains(warning, want) {
					t.Errorf("warning missing %q: %s", want, warning)
				}
			}
		})
	}
}

func upstreamPricingAuthSnapshot(t *testing.T, record configstore.AuthRecord, respond func(*http.Request) (int, string)) newapimanagement.RemoteSnapshot {
	t.Helper()
	store, err := configstore.Open(filepath.Join(t.TempDir(), "pricing-auth.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	record.Host, record.BaseURL = "upstream.test", "https://upstream.test"
	if err := store.SaveAuthRecord(context.Background(), record, nil); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		status, body := http.StatusOK, `{"success":true,"data":[]}`
		switch r.URL.Host {
		case "newapi.test":
			if r.URL.Path != "/api/option/" && r.URL.Path != "/api/channel/models_enabled" && r.URL.Path != "/api/pricing" {
				return nil, fmt.Errorf("unexpected platform path %s", r.URL.Path)
			}
		case "upstream.test":
			status, body = respond(r)
		default:
			return nil, fmt.Errorf("test refuses external endpoint %s", r.URL.Host)
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	service := newapimanagement.New(&catalogStore{store}, nil, client, nil, nil)
	snapshot, err := service.Refresh(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
