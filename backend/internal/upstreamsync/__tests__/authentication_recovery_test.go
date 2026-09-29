package upstreamsync_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamauth"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

func TestBusinessAuthenticationFailureUsesExistingRefreshAndPreservesRecoveryGuidance(t *testing.T) {
	for _, refreshAvailable := range []bool{true, false} {
		name := "without refresh token reports authentication failure"
		if refreshAvailable {
			name = "with refresh token recovers and persists rotated credentials"
		}
		t.Run(name, func(t *testing.T) {
			var refreshRequests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/user/self":
					if r.Header.Get("Authorization") != "Bearer rotated-test-token" {
						_, _ = w.Write([]byte(`{"success":false,"message":"Unauthorized, invalid access token"}`))
						return
					}
					_, _ = w.Write([]byte(`{"success":true,"data":{"id":17,"quota":1000000}}`))
				case "/api/user/auth/refresh":
					refreshRequests.Add(1)
					cookie, err := r.Cookie("new_api_refresh")
					if r.Method != http.MethodPost || err != nil || cookie.Value != "fixture-refresh-token" {
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					_, _ = w.Write([]byte(`{"success":true,"data":{"access_token":"rotated-test-token","refresh_token":"rotated-refresh-token"}}`))
				case "/api/status":
					_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
				default:
					t.Errorf("unexpected fallback request: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			t.Cleanup(server.Close)
			store, err := business.Open(filepath.Join(t.TempDir(), "business.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			if err := store.Bootstrap(t.Context()); err != nil {
				t.Fatal(err)
			}
			private, err := configstore.Open(filepath.Join(t.TempDir(), "private.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = private.Close() })
			token, refresh, userID := "expired-test-token", "fixture-refresh-token", "17"
			record := configstore.AuthRecord{
				Host: "fixture.example", BaseURL: server.URL, UpstreamType: "newapi", AuthMode: "newapi_user_token",
				AccessToken: &token, UserID: &userID,
			}
			if refreshAvailable {
				record.RefreshToken = &refresh
			}
			if err := private.SaveAuthRecord(t.Context(), record, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{
				Host: record.Host, BaseURL: record.BaseURL, UpstreamType: record.UpstreamType,
				AuthMode: record.AuthMode, RechargeRate: "1",
			}); err != nil {
				t.Fatal(err)
			}
			service := upstreamsync.New(store, private, upstreamsync.NewReader(server.Client()), upstreamauth.New(server.Client()), nil)
			result, err := service.SyncHost(t.Context(), record.Host, upstreamsync.Scope{Balance: true}, "test")
			if err != nil {
				t.Fatal(err)
			}
			if !refreshAvailable {
				if result.Status != "auth_failed" || result.Reason == nil || !strings.Contains(*result.Reason, "鉴权恢复") || !strings.Contains(*result.Reason, "invalid access token") {
					t.Fatalf("missing actionable authentication failure: %+v", result)
				}
				if refreshRequests.Load() != 0 {
					t.Fatal("refresh was requested without credentials")
				}
				return
			}
			if result.Status != "succeeded" || !result.AuthRecovered || result.Balance == nil || *result.Balance != "2" {
				t.Fatalf("authentication recovery did not complete balance sync: %+v", result)
			}
			saved, err := private.AuthRecord(t.Context(), record.Host)
			if err != nil || saved == nil || saved.AccessToken == nil || *saved.AccessToken != "rotated-test-token" || saved.RefreshToken == nil || *saved.RefreshToken != "rotated-refresh-token" {
				t.Fatalf("rotated credentials were not persisted: err=%v", err)
			}
			if refreshRequests.Load() != 1 {
				t.Fatalf("refresh must run once: %d requests", refreshRequests.Load())
			}
		})
	}
}
