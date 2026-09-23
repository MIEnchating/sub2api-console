package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAccountLiveTrafficEndpointIsRemoved(t *testing.T) {
	f := newAccountHealthFixture(t, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/accounts/traffic", nil)
	request.Header.Set("Authorization", "Bearer isolated-health-token")
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
