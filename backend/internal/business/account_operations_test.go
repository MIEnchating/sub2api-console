package business

import (
	"context"
	"testing"
)

func TestAccountGroupReadbackCopiesAccountCostInsteadOfGroupSalePrice(t *testing.T) {
	store := openReadModelFixture(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `UPDATE local_groups SET rate_multiplier='3' WHERE remote_id='2'`); err != nil {
		t.Fatal(err)
	}
	operation := AccountOperation{
		OperationID: "groups-41", OperationType: "account.groups.sync", State: "succeeded", Phase: "readback",
		Actor: "operator", RemoteConfirmed: true, ReadbackConfirmed: true, ObjectID: "41", Writeback: true,
	}
	if err := store.CommitAccountGroupsReadback(ctx, "41", []LocalOnboardingGroup{{ID: "2", Name: "pro"}}, nil, operation); err != nil {
		t.Fatal(err)
	}
	var groupRate string
	if err := store.db.QueryRowContext(ctx, `SELECT group_rate FROM account_groups WHERE account_id='41' AND group_id='2'`).Scan(&groupRate); err != nil {
		t.Fatal(err)
	}
	if groupRate != "0.1" {
		t.Fatalf("group membership rate=%q, want account cost 0.1", groupRate)
	}
}

func TestAccountHostEditPreservesStableBindingIdentity(t *testing.T) {
	store := openReadModelFixture(t)
	ctx := context.Background()
	if err := store.ensureStableUpstreamRelations(ctx); err != nil {
		t.Fatal(err)
	}
	var beforeID string
	if err := store.db.QueryRowContext(ctx, `SELECT bi.upstream_id FROM binding_identities bi
		JOIN bindings b ON b.id=bi.binding_id WHERE b.local_account_id='41' LIMIT 1`).Scan(&beforeID); err != nil {
		t.Fatal(err)
	}
	host := "new-address.example.test"
	operation := AccountOperation{
		OperationID: "host-41", OperationType: "account.fields.sync", State: "succeeded", Phase: "readback",
		Actor: "operator", RemoteConfirmed: true, ReadbackConfirmed: true, ObjectID: "41", Writeback: true,
	}
	if err := store.CommitAccountFieldsReadback(ctx, "41", nil, nil, nil, nil, nil, &host, nil, false, nil, operation); err != nil {
		t.Fatal(err)
	}
	var recordedHost, afterID string
	if err := store.db.QueryRowContext(ctx, `SELECT upstream_host FROM accounts WHERE id='41'`).Scan(&recordedHost); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT bi.upstream_id FROM binding_identities bi
		JOIN bindings b ON b.id=bi.binding_id WHERE b.local_account_id='41' LIMIT 1`).Scan(&afterID); err != nil {
		t.Fatal(err)
	}
	if recordedHost != host || afterID != beforeID {
		t.Fatalf("recorded host=%q binding identity before=%q after=%q", recordedHost, beforeID, afterID)
	}
}
