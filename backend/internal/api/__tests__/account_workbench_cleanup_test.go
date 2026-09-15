package api_test

import (
	"net/http"
	"testing"
)

func TestWorkbenchCleanupRoutesRequireLoginAndDoNotCacheConfirmationScope(t *testing.T) {
	router, token := workbenchRouter(t)
	for _, route := range []struct{ method, path string }{{"POST", "cleanup/preview"}, {"DELETE", "cleanup/preview/missing"}, {"POST", "cleanup"}} {
		response := profileExportRequest(router, "", route.method, route.path, `{}`)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated cleanup %s = %d", route.path, response.Code)
		}
	}
	for _, attempt := range []struct {
		path, body string
		status     int
	}{
		{"cleanup/preview", `{"items":"invalid"}`, http.StatusUnprocessableEntity},
		{"cleanup/preview", `{"items":[]}`, http.StatusConflict},
		{"cleanup/preview", `{"items":[{"id":"missing","revision":1}]}`, http.StatusConflict},
		{"cleanup", `{"preview_id":"missing","confirmed":"true"}`, http.StatusUnprocessableEntity},
		{"cleanup", `{"preview_id":"missing","confirmed":false}`, http.StatusConflict},
	} {
		response := profileExportRequest(router, token, http.MethodPost, attempt.path, attempt.body)
		if response.Code != attempt.status || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("cleanup response = %d %s", response.Code, response.Body.String())
		}
	}
}
