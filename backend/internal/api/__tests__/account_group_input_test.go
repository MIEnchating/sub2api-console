package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccountGroupChangeRejectsMalformedEnvelopeBeforePreflight(t *testing.T) {
	for _, scenario := range []struct {
		name, suffix, contentType string
	}{
		{"unknown field", `,"unexpected":true}`, "application/json"},
		{"trailing object", `}{}`, "application/json"},
		{"non JSON content type", `}`, "text/plain"},
		{"missing content type", `}`, ""},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newAccountHealthFixture(t, nil)
			body := `{"group_ids":["8"],"expected_group_ids":["7"],"target_version":"preview-version"` + scenario.suffix
			request := httptest.NewRequest(http.MethodPut, "/api/accounts/41/groups", strings.NewReader(body))
			request.Header.Set("Content-Type", scenario.contentType)
			request.Header.Set("Authorization", "Bearer isolated-health-token")
			response := httptest.NewRecorder()

			f.router.ServeHTTP(response, request)

			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("malformed request reached preflight: status=%d body=%s", response.Code, response.Body.String())
			}
			f.assertSchedulingUnchanged(t)
		})
	}
}
