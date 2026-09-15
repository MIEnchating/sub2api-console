package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestWorkbenchBearerRequiresAnActiveConsoleSession(t *testing.T) {
	for _, state := range []string{"forged", "revoked", "expired", "active"} {
		t.Run(state, func(t *testing.T) {
			private, err := configstore.Open(filepath.Join(t.TempDir(), "config.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = private.Close() })
			ctx := context.Background()
			if err := private.Initialize(ctx, "operator", "isolated-password", "https://target.example", "test-admin-key"); err != nil {
				t.Fatal(err)
			}
			token := "forged-session"
			if state != "forged" {
				createdAt := time.Now()
				if state == "expired" {
					createdAt = createdAt.Add(-2 * time.Hour)
				}
				token, err = private.CreateSession(ctx, "operator", time.Hour, createdAt)
				if err != nil {
					t.Fatal(err)
				}
				if state == "revoked" {
					if err := private.RevokeSession(ctx, token); err != nil {
						t.Fatal(err)
					}
				}
			}
			service := accountworkbench.New(private, nil, nil, nil, nil)
			router := api.New(config.Config{AdminToken: "test-admin"}, private, nil, api.Dependencies{AccountWorkbench: service})
			request := httptest.NewRequest(http.MethodGet, "/api/account-workbench/templates", nil)
			request.Header.Set("Authorization", "Bearer test-admin")
			request.AddCookie(&http.Cookie{Name: "sub2api_console_session", Value: token})
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			expected := http.StatusForbidden
			if state == "active" {
				expected = http.StatusOK
			}
			if response.Code != expected {
				t.Fatalf("session %s returned %d, want %d: %s", state, response.Code, expected, response.Body.String())
			}
		})
	}
}
