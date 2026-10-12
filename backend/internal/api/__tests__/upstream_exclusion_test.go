package api_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/api"
	"github.com/MIEnchating/sub2api-console/backend/internal/config"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamexclusion"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpstreamExclusionRejectsAnonymousAndIncompleteScope(t *testing.T) {
	f := newAccountHealthFixture(t, nil)
	router := api.New(config.Config{AdminToken: "isolated-health-token"}, f.private, f.store, api.Dependencies{UpstreamExclusion: upstreamexclusion.New(f.store, f.private, nil, nil)})
	for _, tc := range []struct {
		name, body    string
		authenticated bool
		status        int
	}{
		{"anonymous", `{}`, false, 401},
		{"missing explicit choice", `{"upstream_id":"up_test","expected_account_ids":[]}`, true, 422},
		{"duplicate IDs", `{"upstream_id":"up_test","excluded":true,"stop_scheduling":false,"expected_account_ids":["41","41"]}`, true, 422},
		{"invalid ID", `{"upstream_id":"up_test","excluded":true,"stop_scheduling":true,"expected_account_ids":["name"]}`, true, 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/upstreams/fixture.example/exclusion", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			if tc.authenticated {
				req.Header.Set("Authorization", "Bearer isolated-health-token")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}
