package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/authrecovery"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestRemovedUpstreamBrowserRoutesReturnNotFound(t *testing.T) {
	private, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = private.Close() })
	ctx := context.Background()
	if err := private.Initialize(ctx, "operator", "isolated-password", "https://target.example", "test-key"); err != nil {
		t.Fatal(err)
	}
	token, err := private.CreateSession(ctx, "operator", time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	service := authrecovery.New(nil, private, nil, nil, nil, nil)
	router := api.New(config.Config{}, private, nil, api.Dependencies{AuthRecovery: service})
	for _, endpoint := range []struct {
		method, path string
	}{
		{http.MethodPost, "/api/auth-recovery/browser"},
		{http.MethodGet, "/api/auth-recovery/browser/missing"},
		{http.MethodPost, "/api/auth-recovery/browser/missing/input"},
		{http.MethodPost, "/api/auth-recovery/browser/missing/finish"},
		{http.MethodDelete, "/api/auth-recovery/browser/missing"},
	} {
		t.Run(endpoint.method+" "+endpoint.path, func(t *testing.T) {
			request := httptest.NewRequest(endpoint.method, "http://console.test"+endpoint.path, strings.NewReader(`{"host":"login.example.test"}`))
			request.AddCookie(&http.Cookie{Name: "sub2api_console_session", Value: token})
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", "http://console.test")
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			if response.Code != http.StatusNotFound {
				t.Fatalf("removed route returned %d, want 404: %s", response.Code, response.Body.String())
			}
		})
	}
}
