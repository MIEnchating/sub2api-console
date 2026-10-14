package business_test

import "testing"

func TestGroupsRefreshSharedAccountMetadataAcrossMemberships(t *testing.T) {
	store, db := accountOwnershipFixture(t, 0)
	if _, err := db.Exec(`INSERT INTO local_groups(name,remote_id,updated_at) VALUES('first','1','now'),('second','2','now');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','first','1'),('41','second','2');
		UPDATE accounts SET schedulable=1 WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		name, metadata     string
		available, limited int64
	}{
		{"active limit", `{"status":"active","platform":"openai","type":"apikey","rate_limit_reset_at":"2099-01-01T00:00:00Z"}`, 0, 1},
		{"cleared limit", `{"status":"active","platform":"openai","type":"apikey"}`, 1, 0},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			if _, err := db.Exec(`UPDATE accounts SET metadata_json=? WHERE id='41'`, scenario.metadata); err != nil {
				t.Fatal(err)
			}
			groups, err := store.Groups(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if len(groups) != 2 {
				t.Fatalf("groups=%v", groups)
			}
			for _, group := range groups {
				if group.AccountCount != 1 || group.AvailableAccounts != scenario.available || group.RateLimitedAccounts != scenario.limited {
					t.Fatalf("group did not reflect current metadata: %+v", group)
				}
			}
		})
	}
}
