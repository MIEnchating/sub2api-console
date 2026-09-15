package api_test

import (
	"net/http"
	"testing"
)

func TestWorkbenchSecuritySourceRequiresSessionAndDoesNotCacheIdentity(t *testing.T) {
	router, token := workbenchRouter(t)
	response := profileExportRequest(router, "", http.MethodGet, "oauth/missing/security-source", "")
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated source status = %d", response.Code)
	}
	response = profileExportRequest(router, token, http.MethodGet, "oauth/missing/security-source", "")
	if response.Code != http.StatusNotFound || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("missing source response = %d %s", response.Code, response.Body.String())
	}
}

func TestWorkbenchCheckpointSecurityRequiresAuthenticationAndConfirmation(t *testing.T) {
	router, token := workbenchRouter(t)
	for _, path := range []string{"oauth-checkpoints/missing/security", "security/missing/confirm-identity", "security/missing/oauth"} {
		t.Run(path, func(t *testing.T) {
			response := profileExportRequest(router, "", http.MethodPost, path, `{}`)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated action status = %d", response.Code)
			}
			response = profileExportRequest(router, token, http.MethodPost, path, `{}`)
			if response.Code != http.StatusConflict || response.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("unconfirmed action response = %d %s", response.Code, response.Body.String())
			}
		})
	}
}
