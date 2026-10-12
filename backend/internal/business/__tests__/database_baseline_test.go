package business_test

import (
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
)

func TestDatabaseContractChangingRoutingTargetRecapturesBaselineAndClearsOwnership(t *testing.T) {
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
			defer db.ExecContext(t.Context(), `DELETE FROM routing_baselines WHERE account_id='771'`)
			original, next, managed := int64(100), int64(50), int64(80)
			baseline := business.RoutingBaseline{AccountID: "771", TargetFingerprint: strings.Repeat("a", 64), Priority: &original, OwnershipVersion: 1}
			if err := store.CaptureRoutingBaseline(t.Context(), baseline); err != nil {
				t.Fatal(err)
			}
			if err := store.UpdateRoutingManagedIntent(t.Context(), "771", baseline.TargetFingerprint, business.RoutingManagedIntent{Priority: &managed}); err != nil {
				t.Fatal(err)
			}
			baseline.Priority = &next
			if err := store.CaptureRoutingBaseline(t.Context(), baseline); err != nil {
				t.Fatal(err)
			}
			got, found, err := store.RoutingBaseline(t.Context(), "771")
			if err != nil || !found || got.Priority == nil || *got.Priority != original || got.ManagedPriority == nil || *got.ManagedPriority != managed {
				t.Fatalf("same-target baseline overwritten: %+v %v", got, err)
			}
			baseline.TargetFingerprint = strings.Repeat("b", 64)
			if err := store.CaptureRoutingBaseline(t.Context(), baseline); err != nil {
				t.Fatal(err)
			}
			got, found, err = store.RoutingBaseline(t.Context(), "771")
			if err != nil || !found || got.Priority == nil || *got.Priority != next || got.ManagedPriority != nil || got.OwnershipVersion != 1 {
				t.Fatalf("new target inherited old ownership: %+v %v", got, err)
			}
		})
	}
}
