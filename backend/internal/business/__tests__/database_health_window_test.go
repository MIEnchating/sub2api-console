package business_test

import (
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
	"testing"
)

func TestDatabaseContractHealthWindowKeepsUniqueEvidenceBeyondDuplicatePage(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store, err := business.OpenDatabase(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			db, err := dbconn.Open(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			defer db.ExecContext(t.Context(), `DELETE FROM health_samples WHERE account_id IN ('790041','790042')`)
			for _, id := range []string{"790041", "790042"} {
				for i := 0; i < 70; i++ {
					key := "duplicate"
					if i == 0 {
						key = "older-unique"
					}
					if _, err := db.ExecContext(t.Context(), `INSERT INTO health_samples(account_id,group_name,source,evidence_key,observed_at,result,payload_json) VALUES(?,?,'traffic',?,?,'通过','{}')`, id, fmt.Sprint("group-", i), key, fmt.Sprintf("2026-10-13T00:%02d:%02dZ", i/60, i%60)); err != nil {
						t.Fatal(err)
					}
				}
			}
			rows, err := store.RoutingSamples(t.Context(), nil, nil, "traffic", 2)
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"790041", "790042"} {
				var keys []string
				for _, row := range rows {
					if row.AccountID == id {
						keys = append(keys, row.EvidenceKey)
					}
				}
				if len(keys) != 2 || keys[0] != "duplicate" || keys[1] != "older-unique" {
					t.Fatalf("account %s window=%v", id, keys)
				}
				recent, err := store.RecentAccountResults(t.Context(), id, 2)
				if err != nil || len(recent) != 2 {
					t.Fatalf("recent count=%d err=%v", len(recent), err)
				}
			}
		})
	}
}
