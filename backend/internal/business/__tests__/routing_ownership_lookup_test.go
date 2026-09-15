package business_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func routingOwnershipStore(t *testing.T) *business.Store {
	t.Helper()
	store, err := business.Open(filepath.Join(t.TempDir(), "routing-ownership.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(t.Context(), []map[string]any{
		{"id": "41", "name": "managed"}, {"id": "42", "name": "released"},
	}, nil, "test"); err != nil {
		t.Fatal(err)
	}
	for index, id := range []string{"41", "42"} {
		if err := store.CaptureRoutingBaseline(t.Context(), business.RoutingBaseline{
			AccountID: id, TargetFingerprint: strings.Repeat("a", 64), OwnershipVersion: index + 1,
		}); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

func TestRoutingBaselineByIDRetainsReleasedOwnershipForWriteProtection(t *testing.T) {
	store := routingOwnershipStore(t)
	baseline, found, err := store.RoutingBaseline(t.Context(), "42")
	if err != nil || !found || baseline.AccountID != "42" || baseline.OwnershipVersion != 2 {
		t.Fatalf("released ownership is required to prevent stale targets from reacquiring control: baseline=%+v found=%t err=%v", baseline, found, err)
	}
}

func TestRoutingBaselineListExcludesAccountsAlreadyReleasedToExternalControl(t *testing.T) {
	store := routingOwnershipStore(t)
	baselines, err := store.RoutingBaselines(t.Context())
	if err != nil || len(baselines) != 1 || baselines[0].AccountID != "41" {
		t.Fatalf("bulk restoration must only include managed baselines: baselines=%+v err=%v", baselines, err)
	}
}
