package business_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
	"testing"
)

func TestDatabaseContractCutoverPreservesExclusionAndCleanupQueries(t *testing.T) {
	for _, cfg := range dbtest.Configs(t) {
		t.Run(string(cfg.Driver), func(t *testing.T) {
			store, err := business.OpenDatabase(t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			if err = store.Bootstrap(t.Context()); err != nil {
				t.Fatal(err)
			}
			created, err := store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{Host: "cutover.contract.test", BaseURL: "https://cutover.contract.test", UpstreamType: "sub2api", AuthMode: "apikey", RechargeRate: "1"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = store.AccountIDsByGroups(t.Context(), []string{"contract-group"}); err != nil {
				t.Fatal(err)
			}
			for _, excluded := range []bool{true, false} {
				if err = store.SetUpstreamExcluded(t.Context(), created.UpstreamID, excluded, "contract"); err != nil {
					t.Fatal(err)
				}
				got, err := store.UpstreamExcluded(t.Context(), "cutover.contract.test")
				if err != nil || got != excluded {
					t.Fatalf("exclusion=%v %v", got, err)
				}
				if _, err = store.AccountUpstreamExcluded(t.Context(), "900000001"); err != nil {
					t.Fatal(err)
				}
				if _, err = store.AccountMutationProtections(t.Context(), []string{"900000001"}); err != nil {
					t.Fatal(err)
				}
				if _, err = store.UpstreamSyncTargets(t.Context(), nil); err != nil {
					t.Fatal(err)
				}
				if _, err = store.ConfirmedMissingBindings(t.Context(), nil); err != nil {
					t.Fatal(err)
				}
				if _, err = store.GroupMembershipOptions(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
