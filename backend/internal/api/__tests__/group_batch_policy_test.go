package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGroupBatchPolicyHTTPValidatesAuthBodyAndCommitsStableIDs(t *testing.T) {
	f := newAccountHealthFixture(t, nil)
	if _, err := f.db.Exec(`INSERT INTO local_groups(name,remote_id,updated_at) VALUES('batch-one','7','now'),('7','8','now')`); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		body   string
		auth   bool
		status int
	}{
		{`{"group_ids":["7"],"action":"exclude"}`, false, 401},
		{`null`, true, 422},
		{`{"group_ids":[],"action":"exclude"}`, true, 422},
		{`{"group_ids":[7],"action":"exclude"}`, true, 422},
		{`{"group_ids":["7"],"action":"bad"}`, true, 422},
		{`{"group_ids":["7"],"action":"exclude","extra":true}`, true, 422},
		{`{"group_ids":["7","999"],"action":"exclude"}`, true, 404},
		{`{"group_ids":["7","7"],"action":"exclude"}`, true, 409},
		{`{"group_ids":["batch-one"],"action":"exclude"}`, true, 409},
		{`{"group_ids":["7","8"],"action":"exclude"}`, true, 200},
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/groups/batch-policy", strings.NewReader(item.body))
		request.Header.Set("Content-Type", "application/json")
		if item.auth {
			request.Header.Set("Authorization", "Bearer isolated-health-token")
		}
		response := httptest.NewRecorder()
		f.router.ServeHTTP(response, request)
		if response.Code != item.status {
			t.Fatalf("body=%s status=%d response=%s", item.body, response.Code, response.Body.String())
		}
		if item.status == 200 && !strings.Contains(response.Body.String(), `"updated":2`) {
			t.Fatal(response.Body.String())
		}
	}
	var count int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM runtime_events WHERE event_type='group.scope.updated'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("events=%d err=%v", count, err)
	}
}
