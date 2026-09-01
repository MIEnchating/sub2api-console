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
	if err := store.CommitAccountGroupsReadback(ctx, "41", []LocalOnboardingGroup{{ID: "2", Name: "pro"}}, operation); err != nil {
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
