package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccountCostWallAPIValidatesAndPersistsOnlyAuthenticatedBoolean(t *testing.T) {
	for _, test := range []struct {
		name, id, body, token string
		status                int
		enabled               bool
	}{
		{"enable", "41", `{"ignore_cost_wall":true}`, "isolated-health-token", 200, true},
		{"disable", "41", `{"ignore_cost_wall":false}`, "isolated-health-token", 200, false},
		{"unauthenticated", "41", `{"ignore_cost_wall":true}`, "", 401, false},
		{"string rejected", "41", `{"ignore_cost_wall":"true"}`, "isolated-health-token", 422, false},
		{"null rejected", "41", `{"ignore_cost_wall":null}`, "isolated-health-token", 422, false},
		{"missing flag", "41", `{}`, "isolated-health-token", 422, false},
		{"unexpected remote field", "41", `{"ignore_cost_wall":true,"schedulable":true}`, "isolated-health-token", 422, false},
		{"name rejected", "health-snapshot", `{"ignore_cost_wall":true}`, "isolated-health-token", 422, false},
		{"unknown account", "999", `{"ignore_cost_wall":true}`, "isolated-health-token", 404, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAccountHealthFixture(t, nil)
			if test.status == 200 && !test.enabled {
				if err := fixture.store.SetAccountIgnoreCostWall(t.Context(), "41", true, "test"); err != nil {
					t.Fatal(err)
				}
			}
			request := httptest.NewRequest(http.MethodPut, "/api/accounts/"+test.id+"/cost-wall", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			if test.token != "" {
				request.Header.Set("Authorization", "Bearer "+test.token)
			}
			response := httptest.NewRecorder()
			fixture.router.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			detail, err := fixture.store.Account(t.Context(), "41")
			if err != nil || detail.IgnoreCostWall != test.enabled {
				t.Fatalf("stored exemption mismatch: %+v err=%v", detail, err)
			}
			fixture.assertSchedulingUnchanged(t)
		})
	}
}
