package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestUpstreamListRemainsAvailableWhenUsageCannotBeRead(t *testing.T) {
	f := newAccountHealthFixture(t, nil)
	if _, err := f.store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{
		Host: "fixture.example", BaseURL: "https://fixture.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`DROP TABLE usage_records`); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/upstreams", nil)
	request.Header.Set("Authorization", "Bearer isolated-health-token")
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("list blocked by usage: status=%d body=%s", response.Code, response.Body.String())
	}
	var result struct {
		Hosts []map[string]json.RawMessage `json:"hosts"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Hosts) != 1 {
		t.Fatalf("list lost upstream: %s", response.Body.String())
	}
	for _, field := range []string{"request_count", "account_request_usage", "request_window_days"} {
		if _, exists := result.Hosts[0][field]; exists {
			t.Fatalf("unread usage exposed as data: %s", field)
		}
	}
	request = httptest.NewRequest(http.MethodGet, "/api/upstreams/usage", nil)
	request.Header.Set("Authorization", "Bearer isolated-health-token")
	response = httptest.NewRecorder()
	f.router.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("usage error was hidden: %d", response.Code)
	}
}

func TestUpstreamUsageRequiresAuthentication(t *testing.T) {
	f := newAccountHealthFixture(t, nil)
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/upstreams/usage", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated usage status=%d", response.Code)
	}
}

func TestUpstreamUsageReturnsEmptyArrayWithoutUpstreams(t *testing.T) {
	f := newAccountHealthFixture(t, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/upstreams/usage", nil)
	request.Header.Set("Authorization", "Bearer isolated-health-token")
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "[]" {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
