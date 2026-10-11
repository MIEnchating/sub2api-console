package api_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
)

func TestModelBlockReplacementRejectsStaleSnapshotAndRequiresExpectedRules(t *testing.T) {
	f := newAccountHealthFixture(t, nil)
	router := api.New(config.Config{AdminToken: "isolated-health-token"}, f.private, f.store, api.Dependencies{AccountModelSync: accountops.New(f.private, f.store, nil)})
	if err := f.store.SaveAccountModelExclusionList(t.Context(), []string{"original", "concurrent"}, "seed"); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		body   string
		status int
	}{
		{`{"blocked_patterns":["draft"]}`, 422},
		{`{"blocked_patterns":["draft"],"expected_patterns":null}`, 422},
		{`{"blocked_patterns":["draft"],"expected_patterns":["original"]}`, 409},
		{`{"blocked_patterns":[],"expected_patterns":["concurrent","original"]}`, 200},
		{`{"blocked_patterns":["new"],"expected_patterns":[]}`, 200},
	} {
		request := httptest.NewRequest(http.MethodPut, "/api/config/model-sync", strings.NewReader(item.body))
		request.Header.Set("Authorization", "Bearer isolated-health-token")
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != item.status {
			t.Fatalf("body=%s status=%d response=%s", item.body, response.Code, response.Body.String())
		}
		if item.status == 409 && !strings.Contains(response.Body.String(), `"code":"model_sync_settings_changed"`) {
			t.Fatal(response.Body.String())
		}
	}
	settings, err := f.store.AccountModelSyncSettings(t.Context())
	if err != nil || !slices.Equal(settings.BlockedPatterns, []string{"new"}) {
		t.Fatalf("settings=%+v err=%v", settings, err)
	}
	var count int
	if err := f.db.QueryRow(`SELECT count(*) FROM runtime_events WHERE event_type='account_model_policy.updated'`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("audit=%d err=%v", count, err)
	}
}
