package api_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestWorkbenchSourceProfileRoutesRequireSession(t *testing.T) {
	router, _ := workbenchRouter(t, true)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "source-profiles?scope=local-export"},
		{http.MethodPost, "source-profiles/source"},
		{http.MethodPost, "source-profiles"},
		{http.MethodDelete, "source-profiles/" + strings.Repeat("a", 32)},
		{http.MethodPost, "source-profiles/" + strings.Repeat("a", 32) + "/security"},
		{http.MethodPost, "source-profiles/reauthorization/preview"},
		{http.MethodPost, "source-profiles/exports/preview"},
		{http.MethodPost, "source-profiles/exports"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			response := profileExportRequest(router, "", route.method, route.path, `{}`)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("source profile route returned %d without a session", response.Code)
			}
		})
	}
}

func TestWorkbenchSourceProfileListNeedsExplicitLocalScopeWithoutManagement(t *testing.T) {
	router, token := workbenchRouter(t, true)
	for _, scope := range []string{"", "managed", "other", "local-export"} {
		t.Run(scope, func(t *testing.T) {
			response := profileExportRequest(router, token, http.MethodGet, "source-profiles?scope="+scope, "")
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("source profile metadata can be cached")
			}
			if scope == "local-export" {
				if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "[]" {
					t.Fatalf("local source list = %d %s", response.Code, response.Body.String())
				}
			} else if response.Code != http.StatusConflict {
				t.Fatal("source profile list accepted an implicit or invalid scope")
			}
		})
	}
}

func TestWorkbenchSourceProfileMutationsRejectMalformedOrUnconfirmedInput(t *testing.T) {
	router, token := workbenchRouter(t, true)
	for _, attempt := range []struct {
		path, body string
		status     int
	}{
		{"source-profiles", `{"scope":"local-export","confirmed":"true"}`, http.StatusUnprocessableEntity},
		{"source-profiles", `{"scope":"local-export","confirmed":false,"login":{"email":"owner@example.com","password":"private-never-persisted"}}`, http.StatusConflict},
		{"source-profiles/source", `{"scope":"local-export","source":{"artifact_id":"missing","index":0}}`, http.StatusConflict},
		{"source-profiles/reauthorization/preview", `{"scope":"local-export","items":[],"fresh_login":false}`, http.StatusConflict},
		{"source-profiles/exports/preview", `{"scope":"local-export","items":[]}`, http.StatusConflict},
		{"source-profiles/exports", `{"preview_id":"missing","confirmed":false}`, http.StatusConflict},
		{"source-profiles/missing/security", `{"scope":"local-export","revision":1,"confirmed":false}`, http.StatusConflict},
	} {
		t.Run(attempt.path, func(t *testing.T) {
			response := profileExportRequest(router, token, http.MethodPost, attempt.path, attempt.body)
			if response.Code != attempt.status || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("source profile mutation = %d %s", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "private-never-persisted") {
				t.Fatal("source profile error exposed input credentials")
			}
		})
	}
}
