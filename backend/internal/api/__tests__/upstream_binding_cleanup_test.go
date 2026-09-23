package api_test

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestUpstreamBindingCleanupValidatesScopeAndMissingAccount(t *testing.T) {
	for _, tc := range []struct {
		name                                                                 string
		auth                                                                 bool
		accountID, status, host, expectedAccount, expectedKey, expectedGroup string
		bindingID, want                                                      int
	}{
		{"missing account cleans selected binding", true, "142", "active", "cleanup.test", "142", "key-142", "codex", 7, 200},
		{"audit failure rolls back binding cleanup", true, "142", "missing", "cleanup.test", "142", "key-142", "codex", 7, 500},
		{"missing marker preserves account", true, "41", "missing", "cleanup.test", "41", "key-142", "codex", 7, 200},
		{"restored account rejects cleanup", true, "41", "active", "cleanup.test", "41", "key-142", "codex", 7, 409},
		{"anonymous rejected", false, "142", "missing", "cleanup.test", "142", "key-142", "codex", 7, 401},
		{"wrong host rejected", true, "142", "missing", "other.test", "142", "key-142", "codex", 7, 409},
		{"changed account rejected", true, "142", "missing", "cleanup.test", "800", "key-142", "codex", 7, 409},
		{"changed key rejected", true, "142", "missing", "cleanup.test", "142", "old-key", "codex", 7, 409},
		{"changed group rejected", true, "142", "missing", "cleanup.test", "142", "key-142", "old-group", 7, 409},
		{"unknown binding rejected", true, "142", "missing", "cleanup.test", "142", "key-142", "codex", 99, 404},
		{"invalid binding rejected", true, "142", "missing", "cleanup.test", "142", "key-142", "codex", 0, 422},
		{"missing account ID rejected", true, "142", "missing", "cleanup.test", "", "key-142", "codex", 7, 422},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newAccountHealthFixture(t, nil)
			up, err := f.store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{Host: "cleanup.test", BaseURL: "https://cleanup.test", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1"})
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.db.Exec(`INSERT INTO bindings(id,local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group_id,local_group,status,updated_at) VALUES(7,?,'cleanup.test','key-142','Key','codex','local',?,'now'),(8,'800','cleanup.test','other-key','Other','codex','local','missing','now')`, tc.accountID, tc.status)
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.db.Exec(`INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,upstream_group_id,updated_at) VALUES(7,?,'key-142','codex','now'),(8,?,'other-key','codex','now')`, up.UpstreamID, up.UpstreamID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.Exec(`INSERT INTO upstream_keys(host,key_id,name,updated_at) VALUES('cleanup.test','key-142','Key','now');
                CREATE TRIGGER protect_cleanup_key BEFORE DELETE ON upstream_keys BEGIN SELECT RAISE(ABORT,'cleanup must preserve keys'); END;`); err != nil {
				t.Fatal(err)
			}
			if tc.want == 500 {
				if _, err := f.db.Exec(`CREATE TRIGGER reject_cleanup_audit BEFORE INSERT ON operation_audit BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`); err != nil {
					t.Fatal(err)
				}
			}
			body := fmt.Sprintf(`{"upstream_id":%q,"account_id":%q,"upstream_key_id":%q,"upstream_group_id":%q}`, up.UpstreamID, tc.expectedAccount, tc.expectedKey, tc.expectedGroup)
			req := httptest.NewRequest("POST", fmt.Sprintf("/api/upstreams/%s/bindings/%d/cleanup", tc.host, tc.bindingID), strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			if tc.auth {
				req.Header.Set("Authorization", "Bearer isolated-health-token")
			}
			res := httptest.NewRecorder()
			f.router.ServeHTTP(res, req)
			if res.Code != tc.want {
				t.Fatalf("status=%d want=%d body=%s", res.Code, tc.want, res.Body.String())
			}
			wantRemaining := 1
			if tc.want == 200 {
				wantRemaining = 0
			}
			for _, table := range []string{"bindings", "binding_identities"} {
				column := "id"
				if table == "binding_identities" {
					column = "binding_id"
				}
				var remaining int
				if err := f.db.QueryRow("SELECT COUNT(*) FROM " + table + " WHERE " + column + "=7").Scan(&remaining); err != nil || remaining != wantRemaining {
					t.Fatalf("%s remaining=%d err=%v", table, remaining, err)
				}
				if err := f.db.QueryRow("SELECT COUNT(*) FROM " + table + " WHERE " + column + "=8").Scan(&remaining); err != nil || remaining != 1 {
					t.Fatalf("unrelated binding changed: %s remaining=%d err=%v", table, remaining, err)
				}
			}
			var audits int
			if err := f.db.QueryRow(`SELECT COUNT(*) FROM operation_audit WHERE operation_type='upstream.binding_cleanup'`).Scan(&audits); err != nil || audits != 1-wantRemaining {
				t.Fatalf("audits=%d err=%v", audits, err)
			}
			f.assertSchedulingUnchanged(t)
		})
	}
}
