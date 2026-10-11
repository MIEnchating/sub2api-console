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
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestProfileUpdateKeepsCurrentSessionAndRevokesOthers(t *testing.T) {
	store, err := configstore.Open(filepath.Join(t.TempDir(), "profile.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Initialize(ctx, "operator", "current-password", "https://isolated.example", "fixture-key"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	current, err := store.CreateAuthenticatedSession(ctx, "operator", "current-password", time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateAuthenticatedSession(ctx, "operator", "current-password", time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	router := api.New(config.Config{}, store, nil, api.Dependencies{})
	req := httptest.NewRequest(http.MethodPut, "http://localhost/api/profile", strings.NewReader(`{"username":"renamed","current_password":"current-password","new_password":"replacement-password"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost")
	req.AddCookie(&http.Cookie{Name: "sub2api_console_session", Value: current})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	user, err := store.SessionUser(ctx, current, now.Add(time.Second))
	if err != nil || user == nil || *user != "renamed" {
		t.Fatalf("current session interrupted: user=%v err=%v", user, err)
	}
	user, err = store.SessionUser(ctx, other, now.Add(time.Second))
	if err != nil || user != nil {
		t.Fatalf("other session was not revoked: %v %v", user, err)
	}
	if len(response.Result().Cookies()) != 0 {
		t.Fatal("existing session expiry must not be extended")
	}
	if ok, err := store.Authenticate(ctx, "renamed", "replacement-password"); err != nil || !ok {
		t.Fatalf("new credentials unavailable: %v", err)
	}
}
