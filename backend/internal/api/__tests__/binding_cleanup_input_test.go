package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestBindingCleanupRejectsMalformedEnvelopeWithoutDeletingBinding(t *testing.T) {
	for _, scenario := range []struct {
		name, suffix, contentType string
	}{
		{"unknown field", `,"unconfirmed":true}`, "application/json"},
		{"trailing object", `}{}`, "application/json"},
		{"non JSON content type", `}`, "text/plain"},
		{"missing content type", `}`, ""},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newAccountHealthFixture(t, nil)
			upstream, err := f.store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{
				Host: "cleanup.test", BaseURL: "https://cleanup.test", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1",
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.Exec(`INSERT INTO bindings(id,local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group_id,local_group,status,updated_at)
				VALUES(7,'142','cleanup.test','key-142','Key','7','local','missing','now')`); err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.Exec(`INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,upstream_group_id,updated_at)
				VALUES(7,?,'key-142','7','now')`, upstream.UpstreamID); err != nil {
				t.Fatal(err)
			}
			body := fmt.Sprintf(`{"upstream_id":%q,"account_id":"142","upstream_key_id":"key-142","upstream_group_id":"7"`, upstream.UpstreamID) + scenario.suffix
			request := httptest.NewRequest(http.MethodPost, "/api/upstreams/cleanup.test/bindings/7/cleanup", strings.NewReader(body))
			request.Header.Set("Content-Type", scenario.contentType)
			request.Header.Set("Authorization", "Bearer isolated-health-token")
			response := httptest.NewRecorder()

			f.router.ServeHTTP(response, request)

			if response.Code != http.StatusUnprocessableEntity {
				t.Errorf("invalid request status=%d body=%s", response.Code, response.Body.String())
			}
			var bindings, identities, audits int
			if err := f.db.QueryRow(`SELECT (SELECT COUNT(*) FROM bindings WHERE id=7),
				(SELECT COUNT(*) FROM binding_identities WHERE binding_id=7),
				(SELECT COUNT(*) FROM operation_audit WHERE operation_type='upstream.binding_cleanup')`).Scan(&bindings, &identities, &audits); err != nil {
				t.Fatal(err)
			}
			if bindings != 1 || identities != 1 || audits != 0 {
				t.Fatalf("invalid request mutated binding: bindings=%d identities=%d audits=%d", bindings, identities, audits)
			}
		})
	}
}
