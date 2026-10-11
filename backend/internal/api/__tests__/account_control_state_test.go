package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAccountControlStateRequiresSessionAndStableID(t *testing.T) {
	f := newAccountHealthFixture(t, nil)
	for _, fixture := range []struct {
		id, token string
		want      int
	}{
		{"41", "", http.StatusUnauthorized},
		{"name", "isolated-health-token", http.StatusUnprocessableEntity},
		{"041", "isolated-health-token", http.StatusUnprocessableEntity},
		{"999", "isolated-health-token", http.StatusNotFound},
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/accounts/"+fixture.id+"/control-state", nil)
		if fixture.token != "" {
			request.Header.Set("Authorization", "Bearer "+fixture.token)
		}
		response := httptest.NewRecorder()
		f.router.ServeHTTP(response, request)
		if response.Code != fixture.want {
			t.Fatalf("id=%s status=%d want=%d", fixture.id, response.Code, fixture.want)
		}
	}
}

func TestAccountControlStateOmitsDetailsAndDoesNotRequireEvidenceHistory(t *testing.T) {
	f := newAccountHealthFixture(t, nil)
	// A state-only read must not depend on the large evidence/history tables.
	if _, err := f.db.Exec(`DROP TABLE account_stability_samples; DROP TABLE health_samples`); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/accounts/41/control-state", nil)
	request.Header.Set("Authorization", "Bearer isolated-health-token")
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 8 || body["id"] != "41" || body["name"] != "health-snapshot" || body["schedulable"] != true {
		t.Fatalf("unexpected state projection: %#v", body)
	}
	for _, key := range []string{"id", "name", "health", "health_status", "routing_state", "paused", "schedulable", "manual_priority"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("missing state field: %s", key)
		}
	}
	f.assertSchedulingUnchanged(t)
}

func TestAccountControlStateStorageFailureDoesNotReturnStaleOrPartialState(t *testing.T) {
	f := newAccountHealthFixture(t, nil)
	if _, err := f.db.Exec(`DROP TABLE account_health_evaluations`); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/accounts/41/control-state", nil)
	request.Header.Set("Authorization", "Bearer isolated-health-token")
	response := httptest.NewRecorder()
	f.router.ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
