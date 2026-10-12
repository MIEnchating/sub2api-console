package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCatalogOptionsDoNotDependOnTrafficOrHealthHistory(t *testing.T) {
	f := newAccountHealthFixture(t, nil)
	if _, err := f.db.Exec(`DROP TABLE usage_records; DROP TABLE account_stability_samples; DROP TABLE health_samples; DROP TABLE account_health_evaluations; DROP TABLE routing_decisions`); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/catalog/groups", "/api/catalog/upstreams", "/api/catalog/accounts", "/api/catalog/group-configurations", "/api/catalog/group-memberships"} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			request.Header.Set("Authorization", "Bearer isolated-health-token")
			response := httptest.NewRecorder()
			f.router.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			var rows []map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &rows); err != nil {
				t.Fatal(err)
			}
			if rows == nil {
				t.Fatal("catalog must return an array")
			}
			for _, row := range rows {
				for _, field := range []string{"request_count", "account_request_usage", "health_score", "recent_results"} {
					if _, exists := row[field]; exists {
						t.Fatalf("statistics leaked into options: %s", field)
					}
				}
			}
		})
	}
}

func TestCatalogOptionsRequireAuthentication(t *testing.T) {
	f := newAccountHealthFixture(t, nil)
	for _, path := range []string{"/api/catalog/groups", "/api/catalog/upstreams", "/api/catalog/accounts", "/api/catalog/group-configurations", "/api/catalog/group-memberships"} {
		response := httptest.NewRecorder()
		f.router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s: %d", path, response.Code)
		}
	}
}
