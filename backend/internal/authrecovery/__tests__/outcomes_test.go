package authrecovery_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/authrecovery"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamauth"
)

func TestInvalidRefreshFollowedByBrowserChallengeReturnsFinalHostOutcome(t *testing.T) {
	var refreshCalls, loginCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/refresh":
			refreshCalls.Add(1)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"REFRESH_TOKEN_INVALID","message":"invalid refresh token"}`))
		case "/api/v1/auth/login":
			loginCalls.Add(1)
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"cloudflare challenge required"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)
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
	access, refresh, username, password := "old-access", "old-refresh", "user@example.test", "test-password"
	record := configstore.AuthRecord{
		Host: "challenge.example.test", BaseURL: upstream.URL, UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", AccessToken: &access, RefreshToken: &refresh,
	}
	if err := private.SaveAuthRecord(t.Context(), record, nil); err != nil {
		t.Fatal(err)
	}
	if err := private.SaveVaultEntry(t.Context(), configstore.VaultEntry{
		Entry: "challenge-test", Username: &username, Password: &password, Hosts: []string{record.Host},
	}, nil); err != nil {
		t.Fatal(err)
	}
	service := authrecovery.New(store, private, upstreamauth.New(upstream.Client()), nil, nil, nil)
	summary, err := service.RecoverInvalid(t.Context(), []string{record.Host}, "test")
	if err != nil || summary.Failed != 1 || refreshCalls.Load() != 1 || loginCalls.Load() != 1 {
		t.Fatalf("recovery must attempt refresh then vault login once: summary=%+v err=%v", summary, err)
	}
	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Results []business.AuthRecoveryOutcome `json:"results"`
	}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 1 {
		t.Fatalf("inspection result lost host recovery details: %s", encoded)
	}
	outcome := result.Results[0]
	if outcome.Host != record.Host || outcome.Success || outcome.Code == nil || *outcome.Code != "browser_challenge_required" || outcome.Reason == nil || !strings.Contains(*outcome.Reason, "人机验证") {
		t.Fatalf("final browser challenge must be retained: %+v", outcome)
	}
	for _, secret := range []string{access, refresh, username, password} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("recovery result exposed test credentials")
		}
	}
}
