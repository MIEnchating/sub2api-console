package business_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"testing"
)

func TestAccountCostWallDefaultsOffAndPersistsWithoutChangingRemoteState(t *testing.T) {
	store, db := concurrencyStore(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,schedulable,paused,routing_state,metadata_json,updated_at) VALUES('41','same-name',0,1,'fused','{}','now'),('42','same-name',1,0,'healthy','{}','now')`); err != nil {
		t.Fatal(err)
	}
	initial, err := store.Account(t.Context(), "41")
	if err != nil {
		t.Fatal(err)
	}
	if initial.IgnoreCostWall {
		t.Fatal("new account must respect cost wall")
	}
	for _, enabled := range []bool{true, false} {
		before, err := store.ControlPolicy(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		beforeHash, _ := business.RoutingPolicyFingerprint(before)
		if err := store.SetAccountIgnoreCostWall(t.Context(), "41", enabled, "test"); err != nil {
			t.Fatal(err)
		}
		detail, err := store.Account(t.Context(), "41")
		if err != nil {
			t.Fatal(err)
		}
		if detail.IgnoreCostWall != enabled || detail.Schedulable == nil || *detail.Schedulable || detail.Paused == nil || !*detail.Paused || detail.RoutingState == nil || *detail.RoutingState != "fused" {
			t.Fatalf("unexpected account state: %+v", detail)
		}
		peer, err := store.Account(t.Context(), "42")
		if err != nil || peer.IgnoreCostWall {
			t.Fatalf("same-name peer changed: %v", err)
		}
		after, err := store.ControlPolicy(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		afterHash, _ := business.RoutingPolicyFingerprint(after)
		if beforeHash == afterHash {
			t.Fatal("stale routing calculations must be invalidated")
		}
	}
	var events int
	if err := db.QueryRow(`SELECT COUNT(*) FROM runtime_events WHERE event_type='account.control'`).Scan(&events); err != nil || events != 2 {
		t.Fatalf("audit events=%d err=%v", events, err)
	}
}

func TestAccountCostWallRejectsMissingOrUnstableAccountID(t *testing.T) {
	store, _ := concurrencyStore(t)
	for _, id := range []string{"same-name", "", "999"} {
		if err := store.SetAccountIgnoreCostWall(t.Context(), id, true, "test"); err == nil {
			t.Fatalf("accepted %q", id)
		}
	}
}

func TestDeletingAccountRemovesCostWallExemptionWithoutChangingPeer(t *testing.T) {
	store, db := concurrencyStore(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,updated_at) VALUES('41','first','now'),('42','peer','now')`); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"41", "42"} {
		if err := store.SetAccountIgnoreCostWall(t.Context(), id, true, "test"); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.DeleteAccountProjection(t.Context(), "41", business.AccountOperation{
		OperationID: "delete-cost-wall-fixture", OperationType: "account.delete", State: "succeeded", Phase: "readback", Actor: "test", ObjectID: "41", Before: map[string]any{}, After: map[string]any{},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO accounts(id,name,updated_at) VALUES('41','replacement','now')`); err != nil {
		t.Fatal(err)
	}
	replacement, err := store.Account(t.Context(), "41")
	if err != nil || replacement.IgnoreCostWall {
		t.Fatalf("deleted exemption leaked into replacement: %v", err)
	}
	peer, err := store.Account(t.Context(), "42")
	if err != nil || !peer.IgnoreCostWall {
		t.Fatalf("peer exemption lost: %v", err)
	}
}
