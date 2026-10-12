package business_test

import (
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
)

func TestDatabaseContractUpstreamCatalogBalanceAndPendingOnboardingPreserveIdentity(t *testing.T) {
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
			created, err := store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{Host: "catalog.contract.test", BaseURL: "https://catalog.contract.test", UpstreamType: "sub2api", AuthMode: "apikey", RechargeRate: "0.3"})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				for _, table := range []string{"onboarding_pending", "upstream_keys", "upstream_groups", "recharge_rates", "billing_quota_unit_observations", "upstreams"} {
					column := "host"
					if table == "onboarding_pending" {
						column = "upstream_host"
					}
					if _, err := db.ExecContext(t.Context(), "DELETE FROM "+table+" WHERE "+column+"=?", "catalog.contract.test"); err != nil {
						t.Error(err)
					}
				}
				if _, err := db.ExecContext(t.Context(), `DELETE FROM upstream_identities WHERE upstream_id=?`, created.UpstreamID); err != nil {
					t.Error(err)
				}
			}()
			active, rate, balance, quota := "active", "0.2", "1", "500000"
			for range 2 {
				result, err := store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: "catalog.contract.test", AuthenticationOK: true, Catalog: &business.UpstreamCatalogSnapshot{Groups: []business.UpstreamCatalogGroup{{GroupID: "7", Name: "pro", Status: &active, RawRate: &rate}}, Keys: []business.UpstreamCatalogKey{{KeyID: "17", Name: "pro-key", UpstreamGroup: new("7"), Status: &active, Rate: &rate}}}, Balance: &business.UpstreamBalanceObservation{RawBalance: &balance, QuotaPerUnit: &quota, ProfileUserID: "42", Status: "已读取"}})
				if err != nil || result.GroupCount != 1 || result.KeyCount != 1 || result.Balance == nil || *result.Balance != "3.3333333333333333333333333333" {
					t.Fatalf("catalog/balance sync: %+v %v", result, err)
				}
			}
			marker := "contract-pending"
			pending := business.PendingOnboarding{OperationID: marker, UpstreamHost: "catalog.contract.test", UpstreamType: "sub2api", UpstreamKeyName: &marker, UpstreamGroupID: "7", UpstreamGroupName: "pro", LocalGroupID: "2", LocalGroupName: "two", LocalGroupIDs: []string{"10", "2"}, Multiplier: "0.2", IntentHash: strings.Repeat("a", 64), Reason: "contract", KeyCommitUnknown: true}
			if err := store.SavePendingOnboarding(t.Context(), pending); err != nil {
				t.Fatal(err)
			}
			got, err := store.PendingOnboarding(t.Context(), pending.UpstreamHost, "7", []string{"2", "10"})
			if err != nil || got == nil || got.OperationID != marker || !got.KeyCommitUnknown {
				t.Fatalf("pending identity: %+v %v", got, err)
			}
			pending.OperationID = "contract-duplicate"
			pending.UpstreamKeyID = "must-not-overwrite"
			if err := store.SavePendingOnboarding(t.Context(), pending); err == nil {
				t.Fatal("duplicate onboarding identity accepted")
			}
			got, err = store.PendingOnboarding(t.Context(), pending.UpstreamHost, "7", []string{"2", "10"})
			if err != nil || got == nil || got.OperationID != marker || got.UpstreamKeyID != "" {
				t.Fatalf("duplicate onboarding changed original: %+v %v", got, err)
			}
		})
	}
}
