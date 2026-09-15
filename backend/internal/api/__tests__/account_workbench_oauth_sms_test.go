package api_test

import (
	"net/http"
	"testing"
)

func TestWorkbenchLiveSMSRoutesRequireSessionAndRejectMissingAuthorization(t *testing.T) {
	router, token := workbenchRouter(t, true)
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		response := profileExportRequest(router, "", method, "oauth/missing/sms", `{}`)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated SMS route = %d", response.Code)
		}
		response = profileExportRequest(router, token, method, "oauth/missing/sms", `{}`)
		if response.Code != http.StatusNotFound || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("missing authorization SMS route = %d, cache = %s", response.Code, response.Header().Get("Cache-Control"))
		}
	}
	invalid := profileExportRequest(router, token, http.MethodPost, "oauth/missing/sms", `{"sms":`)
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid attachment body = %d", invalid.Code)
	}
}
