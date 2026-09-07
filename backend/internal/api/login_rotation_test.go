package api

import (
	"context"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLoginCannotIssueOldCredentialSessionAfterPasswordRotation(t *testing.T) {
	_, store := testRouter(t, config.Config{}, fakeBusiness{})
	if err := store.Initialize(context.Background(), "admin", "old test password", "https://isolated.example", "test-key"); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	calls := 0
	server := &Server{private: store, loginThrottle: newLoginThrottle(nil), now: func() time.Time {
		calls++
		if calls == 2 {
			replacement := "replacement test password"
			if _, err := store.UpdateCredentials(context.Background(), "old test password", "admin", &replacement); err != nil {
				t.Fatal(err)
			}
		}
		return now
	}}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"old test password"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	server.login(ctx)
	if calls != 2 {
		t.Fatalf("rotation boundary not reached: clock calls=%d", calls)
	}
	if recorder.Code != http.StatusUnauthorized || len(recorder.Result().Cookies()) != 0 {
		t.Fatalf("old credentials issued a session after password rotation: status=%d cookies=%d", recorder.Code, len(recorder.Result().Cookies()))
	}
}
