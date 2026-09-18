package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAccountLiveTrafficRequiresConsoleAuthentication(t *testing.T) {
	f := newAccountHealthFixture(t, nil)
	for _, token := range []string{"", "isolated-health-token"} {
		request := httptest.NewRequest(http.MethodGet, "/api/accounts/traffic", nil)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		f.router.ServeHTTP(response, request)
		want := 401
		if token != "" {
			want = 503
		}
		if response.Code != want {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
	}
}
