package api_test

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestAllocationSettingAPIRequiresAuthenticationAndVersionedExplicitOverride(t *testing.T) {
	f := newAccountHealthFixture(t, nil)
	upstream, err := f.store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{Host: "allocation.test", BaseURL: "https://allocation.test", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1"})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/policy/upstream-concurrency/upstreams/" + upstream.UpstreamID
	read, err := f.store.UpstreamAllocationSetting(t.Context(), "upstreams", upstream.UpstreamID)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name, method, body string
		auth               bool
		status             int
	}{
		{"anonymous read", "GET", "", false, 401},
		{"anonymous write", "PUT", `{"override":true}`, false, 401},
		{"authenticated read", "GET", "", true, 200},
		{"missing override cannot reset", "PUT", fmt.Sprintf(`{"expected_revision":%q}`, read.Revision), true, 422},
		{"string is not boolean", "PUT", fmt.Sprintf(`{"override":"true","expected_revision":%q}`, read.Revision), true, 422},
		{"unknown field", "PUT", fmt.Sprintf(`{"override":true,"enabled":true,"expected_revision":%q}`, read.Revision), true, 422},
		{"stale revision", "PUT", `{"override":true,"expected_revision":"old"}`, true, 409},
		{"save explicit on", "PUT", fmt.Sprintf(`{"override":true,"expected_revision":%q}`, read.Revision), true, 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(test.method, path, strings.NewReader(test.body))
			r.Header.Set("Content-Type", "application/json")
			if test.auth {
				r.Header.Set("Authorization", "Bearer isolated-health-token")
			}
			w := httptest.NewRecorder()
			f.router.ServeHTTP(w, r)
			if w.Code != test.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if test.name == "save explicit on" {
				var value business.UpstreamAllocationSetting
				if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil || value.Override == nil || !*value.Override {
					t.Fatalf("saved=%+v err=%v", value, err)
				}
			}
			f.assertSchedulingUnchanged(t)
		})
	}
}
