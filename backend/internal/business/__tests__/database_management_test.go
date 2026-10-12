package business_test

import (
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
)

func TestDatabaseContractManagementSnapshotRefreshesMembershipCosts(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store, err := business.OpenDatabase(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			if err := store.Bootstrap(t.Context()); err != nil {
				t.Fatal(err)
			}
			db, err := dbconn.Open(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			groups := []map[string]any{{"id": "7", "name": "first"}, {"id": "8", "name": "second"}}
			for _, rate := range []string{"0.1", "0.1234567890123456789"} {
				result, err := store.SyncManagementSnapshot(t.Context(), []map[string]any{{"id": "41", "name": "member", "rate_multiplier": rate, "schedulable": true, "group_ids": []any{"8", "7", "8"}}}, groups, "contract")
				if err != nil {
					t.Fatal(err)
				}
				if result.Accounts != 1 || result.GroupLinks != 2 || result.Groups != 2 {
					t.Fatalf("snapshot: %+v", result)
				}
				var links, counts int
				if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM account_groups WHERE account_id='41' AND group_rate=?`, rate).Scan(&links); err != nil || links != 2 {
					t.Fatalf("membership costs: %d %v", links, err)
				}
				if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM local_groups WHERE remote_id IN ('7','8') AND account_count=1`).Scan(&counts); err != nil || counts != 2 {
					t.Fatalf("group counts: %d %v", counts, err)
				}
			}
			ids, err := store.ManagementAccountIDs(t.Context())
			if err != nil || !reflect.DeepEqual(ids, []string{"41"}) {
				t.Fatalf("account IDs: %v %v", ids, err)
			}
			if _, err := store.SyncCompleteManagementSnapshot(t.Context(), nil, nil, "contract"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
