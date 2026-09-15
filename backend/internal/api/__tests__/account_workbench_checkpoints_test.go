package api_test

import (
	"net/http"
	"testing"
)

func TestWorkbenchOAuthCheckpointRoutesRequireAuthentication(t *testing.T) {
	router, _ := workbenchRouter(t)
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "oauth-checkpoints"},
		{http.MethodPost, "oauth/id/checkpoint"},
		{http.MethodPost, "oauth-checkpoints/id/restore"},
		{http.MethodDelete, "oauth-checkpoints/id"},
	} {
		response := profileExportRequest(router, "", route.method, route.path, `{}`)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s = %d", route.path, response.Code)
		}
	}
}

func TestWorkbenchOAuthCheckpointListAndInvalidWritesNeverCachePrivateState(t *testing.T) {
	router, token := workbenchRouter(t)
	for _, attempt := range []struct {
		method, path, body string
		status             int
	}{
		{http.MethodGet, "oauth-checkpoints", "", http.StatusOK},
		{http.MethodPost, "oauth/id/checkpoint", `{"confirmed":"true"}`, http.StatusUnprocessableEntity},
		{http.MethodPost, "oauth/id/checkpoint", `{"confirmed":false}`, http.StatusConflict},
		{http.MethodPost, "oauth-checkpoints/id/restore", `{"revision":"1","confirmed":true}`, http.StatusUnprocessableEntity},
		{http.MethodPost, "oauth-checkpoints/id/restore", `{"revision":1,"confirmed":false}`, http.StatusConflict},
		{http.MethodDelete, "oauth-checkpoints/id", `{"revision":"1","confirmed":true}`, http.StatusUnprocessableEntity},
		{http.MethodDelete, "oauth-checkpoints/id", `{"revision":1,"confirmed":false}`, http.StatusConflict},
	} {
		response := profileExportRequest(router, token, attempt.method, attempt.path, attempt.body)
		if response.Code != attempt.status || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s response = %d %s", attempt.path, response.Code, response.Body.String())
		}
		if attempt.method == http.MethodGet && response.Body.String() != "[]" {
			t.Fatalf("empty checkpoint list = %s", response.Body.String())
		}
	}
}
