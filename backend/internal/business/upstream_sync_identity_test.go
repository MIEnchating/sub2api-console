package business

import (
	"context"
	"testing"
)

func TestPartialKeySyncPrefersStableGroupIDOverCollidingName(t *testing.T) {
	store := upstreamSyncTestStore(t)
	active, rate, keyID, reference := "active", "0.2", "key-17", "group-7"
	_, err := store.ApplyUpstreamSync(context.Background(), UpstreamSyncWrite{
		Host: "api.example", KeyID: &keyID, AuthenticationOK: true,
		Catalog: &UpstreamCatalogSnapshot{
			Groups: []UpstreamCatalogGroup{{GroupID: "wrong-group", Name: reference, Status: &active, RawRate: &rate}, {GroupID: reference, Name: "Correct group", Status: &active, RawRate: &rate}},
			Keys:   []UpstreamCatalogKey{{KeyID: keyID, Name: "Selected key", UpstreamGroup: &reference, Status: &active, Rate: &rate}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var storedGroupID string
	if err := store.db.QueryRow(`SELECT group_id FROM upstream_groups WHERE host='api.example'`).Scan(&storedGroupID); err != nil {
		t.Fatal(err)
	}
	if storedGroupID != reference {
		t.Fatalf("partial sync selected name collision instead of stable ID: %q", storedGroupID)
	}
}
