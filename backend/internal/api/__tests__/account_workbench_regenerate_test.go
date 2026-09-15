package api_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestWorkbenchRegenerationRoutesRequireAuthentication(t *testing.T) {
	router, _ := workbenchRouter(t)
	for _, path := range []string{"exports/regenerate/preview", "exports/regenerate"} {
		response := profileExportRequest(router, "", http.MethodPost, path, `{}`)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s status = %d", path, response.Code)
		}
	}
}

func TestWorkbenchRegenerationValidatesSourceAndExplicitConfirmationWithoutCaching(t *testing.T) {
	router, token := workbenchRouter(t)
	for _, attempt := range []struct {
		path, body string
		status     int
	}{
		{"exports/regenerate/preview", `{"account_ids":"101"}`, http.StatusUnprocessableEntity},
		{"exports/regenerate/preview", `{"account_ids":[]}`, http.StatusConflict},
		{"exports/regenerate/preview", `{"account_ids":["01"]}`, http.StatusConflict},
		{"exports/regenerate/preview", `{"source_task_id":"` + strings.Repeat("a", 32) + `"}`, http.StatusConflict},
		{"exports/regenerate/preview", `{"scope":"local-export","account_ids":["101"]}`, http.StatusConflict},
		{"exports/regenerate/preview", `{"scope":"local-export","artifact_id":"` + strings.Repeat("a", 32) + `"}`, http.StatusConflict},
		{"exports/regenerate/preview", `{"scope":"local-export","artifact_id":123}`, http.StatusUnprocessableEntity},
		{"exports/regenerate", `{"preview_id":"missing","confirmed":"true"}`, http.StatusUnprocessableEntity},
		{"exports/regenerate", `{"preview_id":"missing","confirmed":false}`, http.StatusConflict},
		{"exports/regenerate", `{"preview_id":"missing","confirmed":true}`, http.StatusConflict},
	} {
		response := profileExportRequest(router, token, http.MethodPost, attempt.path, attempt.body)
		if response.Code != attempt.status || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s response = %d %s", attempt.path, response.Code, response.Body.String())
		}
	}
}
