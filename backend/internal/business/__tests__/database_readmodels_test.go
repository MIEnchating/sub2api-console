package business_test

import (
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
)

func TestDatabaseContractAccountViewsPreserveLongNamesAndLatestPlatform(t *testing.T) {
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
			name := strings.Repeat("中文账号", 150)
			if _, err := store.SyncManagementSnapshot(t.Context(), []map[string]any{{"id": "831", "name": name, "rate_multiplier": "0.1234567890123456789", "group_ids": []any{"931"}}}, []map[string]any{{"id": "931", "name": "view-group", "platform": "openai"}}, "contract"); err != nil {
				t.Fatal(err)
			}
			defer store.SyncCompleteManagementSnapshot(t.Context(), nil, nil, "contract")
			db, err := dbconn.Open(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.ExecContext(t.Context(), `UPDATE accounts SET metadata_json='{"platform":"wrong","platform":"openai"}' WHERE id='831'`); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			if _, err := store.PersistTrafficSamples(t.Context(), []business.TrafficSample{{AccountID: "831", GroupName: "view-group", EvidenceKey: "view-request", Result: "通过", ObservedAt: now.Format(time.RFC3339Nano), Payload: map[string]any{}}}); err != nil {
				t.Fatal(err)
			}
			for _, table := range []string{"usage_records", "health_samples"} {
				if _, err := db.ExecContext(t.Context(), "UPDATE "+table+" SET source='TRAFFIC' WHERE account_id='831'"); err != nil {
					t.Fatal(err)
				}
			}
			catalog, err := store.PricingCatalogForAccounts(t.Context(), []string{"831", "831"})
			if err != nil || len(catalog.Accounts) != 1 {
				t.Errorf("pricing catalog: %+v %v", catalog, err)
			} else if got := catalog.Accounts[0]; got.Name != name || got.Platform != "openai" || got.Multiplier == nil || *got.Multiplier != "0.1234567890123456789" {
				t.Errorf("pricing projection lost values: %+v", got)
			}
			accounts, err := store.Accounts(t.Context())
			if err != nil || len(accounts) != 1 || accounts[0].ID != "831" || accounts[0].Name != name {
				t.Errorf("account view: count=%d %v", len(accounts), err)
			}
			ranking, err := store.TrafficRanking(t.Context(), business.TrafficRankingQuery{StartAt: now.Add(-time.Minute), EndAt: now.Add(time.Minute), SortBy: business.TrafficRankingSortTraffic})
			if err != nil || ranking.TotalRequests != 1 || len(ranking.Accounts) != 1 || ranking.Accounts[0].Platform != "openai" {
				t.Errorf("traffic view: %+v %v", ranking, err)
			}
			if _, err := db.ExecContext(t.Context(), `UPDATE account_groups SET group_id=NULL,group_name='VIEW-GROUP' WHERE account_id='831'`); err != nil {
				t.Fatal(err)
			}
			allocation, err := store.GroupAllocation(t.Context(), "931")
			if err != nil || len(allocation.Channels) != 1 {
				t.Fatalf("legacy group-name lookup: %+v %v", allocation, err)
			}
		})
	}
}
