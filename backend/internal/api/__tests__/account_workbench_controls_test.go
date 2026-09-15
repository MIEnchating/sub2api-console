package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccountWorkbenchNewControlsRequireConfirmationAndNeverCache(t *testing.T) {
	router, token := workbenchRouter(t)
	for _, item := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "login-profiles", "", 200},
		{"GET", "maintenance/authorization", "", 200},
		{"POST", "maintenance/authorization", `{"revision":1}`, 422},
		{"POST", "login-profiles", `{"account_id":"101","revision":0,"login":{"email":"owner@example.com"}}`, 409},
		{"DELETE", "login-profiles/profile", `{"revision":1}`, 409},
		{"POST", "login-profiles/security-result", `{"profile_id":"profile","revision":1,"security_id":"security"}`, 409},
		{"POST", "reauthorization/preview", `{"account_ids":["101"]}`, 409},
		{"POST", "history/delete", `{"items":[{"id":"task","updated_at":"2026-09-14T00:00:00Z"}]}`, 409},
		{"POST", "history/cancel", `{"items":[{"id":"task","updated_at":"2026-09-14T00:00:00Z"}]}`, 409},
	} {
		t.Run(item.method+item.path, func(t *testing.T) {
			req := httptest.NewRequest(item.method, "http://console.test/api/account-workbench/"+item.path, strings.NewReader(item.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://console.test")
			req.AddCookie(&http.Cookie{Name: "sub2api_console_session", Value: token})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != item.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("private controls can be cached")
			}
		})
	}
}
