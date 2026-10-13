package api_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestRememberedAccountRoutesRequireSessionAndValidateRequests(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Initialize(t.Context(), "operator", "isolated-password", "https://target.invalid", "test-key"); err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(t.Context(), "operator", time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	router := api.New(config.Config{}, store, nil, api.Dependencies{AccountWorkbench: accountworkbench.New(store)})
	base := "/api/account-workbench/remembered-accounts"
	for _, tc := range []struct {
		name, method, path, body string
		auth                     bool
		status                   int
	}{
		{"action preview without session", "GET", "/api/account-workbench/accounts/41/action-preview", "", false, http.StatusUnauthorized},
		{"account action without session", "POST", "/api/account-workbench/accounts/41/actions", `{}`, false, http.StatusUnauthorized},
		{"invalid action account", "GET", "/api/account-workbench/accounts/invalid/action-preview", "", true, http.StatusUnprocessableEntity},
		{"missing action version", "POST", "/api/account-workbench/accounts/41/actions", `{"action":"delete"}`, true, http.StatusUnprocessableEntity},
		{"invalid scheduling value", "POST", "/api/account-workbench/accounts/41/actions", `{"action":"scheduling","target_version":"t","account_version":"a","schedulable":"yes"}`, true, http.StatusUnprocessableEntity},
		{"list without session", "GET", base, "", false, http.StatusUnauthorized},
		{"delete without session", "DELETE", base + "/missing", `{"revision":1}`, false, http.StatusUnauthorized},
		{"login without session", "POST", base + "/missing/preview", `{"revision":1}`, false, http.StatusUnauthorized},
		{"empty saved list", "GET", base, "", true, http.StatusOK},
		{"invalid delete revision", "DELETE", base + "/missing", `{"revision":"bad"}`, true, http.StatusUnprocessableEntity},
		{"invalid login revision", "POST", base + "/missing/preview", `{"revision":"bad"}`, true, http.StatusUnprocessableEntity},
		{"missing saved delete target", "DELETE", base + "/" + strings.Repeat("a", 64), `{"revision":1}`, true, http.StatusConflict},
		{"missing saved login target", "POST", base + "/" + strings.Repeat("a", 64) + "/preview", `{"revision":1}`, true, http.StatusConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "http://console.test"+tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://console.test")
			if tc.auth {
				req.AddCookie(&http.Cookie{Name: "sub2api_console_session", Value: session})
			}
			res := httptest.NewRecorder()
			router.ServeHTTP(res, req)
			if res.Code != tc.status {
				t.Fatalf("status=%d want=%d", res.Code, tc.status)
			}
			if tc.auth && res.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("private response can be cached")
			}
			if tc.method == "GET" && tc.path == base && tc.auth && strings.TrimSpace(res.Body.String()) != "[]" {
				t.Fatal("empty saved list contract changed")
			}
		})
	}
}
