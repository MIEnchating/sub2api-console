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

func TestBrowserInputRejectsMalformedJSONBeforeLookingUpBrowserSession(t *testing.T) {
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
	for _, input := range []struct {
		name, body, contentType string
		status                  int
	}{
		{"trailing JSON", `{"kind":"key","key":"Tab"}{}`, "application/json", http.StatusUnprocessableEntity},
		{"unknown field", `{"kind":"key","key":"Tab","unexpected":true}`, "application/json", http.StatusUnprocessableEntity},
		{"non JSON content type", `{"kind":"key","key":"Tab"}`, "text/plain", http.StatusUnprocessableEntity},
		{"valid input", `{"kind":"key","key":"Tab"}`, "application/json", http.StatusNotFound},
	} {
		t.Run(input.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "http://console.test/api/auth-recovery/browser/missing/input", strings.NewReader(input.body))
			request.AddCookie(&http.Cookie{Name: "sub2api_console_session", Value: token})
			request.Header.Set("Content-Type", input.contentType)
			request.Header.Set("Origin", "http://console.test")
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			if response.Code != input.status {
				t.Fatalf("browser input returned %d, want %d: %s", response.Code, input.status, response.Body.String())
			}
		})
	}
}
