package api_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExcludedUpstreamRejectsLocalAccountMutations(t *testing.T) {
	f := newAccountHealthFixture(t, nil)
	if _, err := f.db.Exec(`DROP TRIGGER reject_health_read_accounts_UPDATE; UPDATE accounts SET upstream_host='fixture.example' WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{Host: "fixture.example", BaseURL: "https://fixture.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1"}); err != nil {
		t.Fatal(err)
	}
	preview, err := f.store.UpstreamExclusionPreview(t.Context(), "fixture.example")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.SetUpstreamExcluded(t.Context(), preview.UpstreamID, true, "test"); err != nil {
		t.Fatal(err)
	}
	for _, sample := range []struct{ path, body string }{
		{"cost-wall", `{"ignore_cost_wall":true}`},
		{"group-lock", `{"groups_locked":true}`},
	} {
		t.Run(sample.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/api/accounts/41/"+sample.path, strings.NewReader(sample.body))
			req.Header.Set("Authorization", "Bearer isolated-health-token")
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			f.router.ServeHTTP(response, req)
			if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "上游已排除") {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
	account, err := f.store.Account(t.Context(), "41")
	if err != nil {
		t.Fatal(err)
	}
	if account.IgnoreCostWall || account.GroupsLocked {
		t.Fatal("excluded account policy changed")
	}
	f.assertSchedulingUnchanged(t)
}
