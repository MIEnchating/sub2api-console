package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
)

// Coordinate a real session revocation after authentication, during the
// account read. Keep all business persistence and authentication code real.
type actorSessionBoundary struct {
	*business.Store
	afterRead func() error
}

func (s *actorSessionBoundary) Account(ctx context.Context, id string) (*business.AccountDetail, error) {
	account, err := s.Store.Account(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.afterRead(); err != nil {
		return nil, err
	}
	return account, nil
}

func TestAuthenticatedWriteKeepsItsActorWhenSessionIsRevokedDuringRequest(t *testing.T) {
	f := newAccountHealthFixture(t, nil)
	if err := f.private.Initialize(t.Context(), "operator", "isolated-password", "https://target.invalid", "fixture-key"); err != nil {
		t.Fatal(err)
	}
	token, err := f.private.CreateSession(t.Context(), "operator", time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	store := &actorSessionBoundary{Store: f.store, afterRead: func() error {
		return f.private.RevokeSession(t.Context(), token)
	}}
	router := api.New(config.Config{}, f.private, store)
	for _, expected := range []int{http.StatusOK, http.StatusUnauthorized} {
		request := httptest.NewRequest(http.MethodPut, "http://console.test/api/accounts/41/cost-wall", strings.NewReader(`{"ignore_cost_wall":false}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", "http://console.test")
		request.AddCookie(&http.Cookie{Name: "sub2api_console_session", Value: token})
		response := httptest.NewRecorder()

		router.ServeHTTP(response, request)

		if response.Code != expected {
			t.Fatalf("status=%d want=%d body=%s", response.Code, expected, response.Body.String())
		}
	}
	var actor string
	var count int
	if err := f.db.QueryRow(`SELECT json_extract(payload_json,'$.actor'), COUNT(*) FROM runtime_events WHERE event_type='account.control'`).Scan(&actor, &count); err != nil {
		t.Fatal(err)
	}
	if actor != "operator" || count != 1 {
		t.Fatalf("authenticated actor lost or revoked request wrote an event: actor=%q count=%d", actor, count)
	}
	f.assertSchedulingUnchanged(t)
}

func TestWriteActorMatchesAuthenticatedCookieAndBearerRequests(t *testing.T) {
	for _, scenario := range []struct {
		name, cookie, bearer, actor string
		status                      int
	}{
		{"cookie", "valid", "", "operator", http.StatusOK},
		{"bearer", "", "isolated-admin", "console", http.StatusOK},
		{"bearer with cookie", "valid", "isolated-admin", "operator", http.StatusOK},
		{"bearer with invalid cookie", "invalid", "isolated-admin", "console", http.StatusOK},
		{"invalid cookie", "invalid", "", "", http.StatusUnauthorized},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newAccountHealthFixture(t, nil)
			if err := f.private.Initialize(t.Context(), "operator", "isolated-password", "https://target.invalid", "fixture-key"); err != nil {
				t.Fatal(err)
			}
			cookie := scenario.cookie
			if cookie == "valid" {
				var err error
				cookie, err = f.private.CreateSession(t.Context(), "operator", time.Hour, time.Now())
				if err != nil {
					t.Fatal(err)
				}
			}
			router := api.New(config.Config{AdminToken: "isolated-admin"}, f.private, f.store)
			request := httptest.NewRequest(http.MethodPut, "http://console.test/api/accounts/41/cost-wall", strings.NewReader(`{"ignore_cost_wall":false}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", "http://console.test")
			if cookie != "" {
				request.AddCookie(&http.Cookie{Name: "sub2api_console_session", Value: cookie})
			}
			if scenario.bearer != "" {
				request.Header.Set("Authorization", "Bearer "+scenario.bearer)
			}
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			if response.Code != scenario.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, scenario.status, response.Body.String())
			}
			var actor string
			if err := f.db.QueryRow(`SELECT COALESCE((SELECT json_extract(payload_json,'$.actor') FROM runtime_events WHERE event_type='account.control'),'')`).Scan(&actor); err != nil {
				t.Fatal(err)
			}
			if actor != scenario.actor {
				t.Fatalf("actor=%q want=%q", actor, scenario.actor)
			}
		})
	}
}
