package business_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestAccountMutationSnapshotReadsOnlyControlFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mutation.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','control-account','not-json','now');
		INSERT INTO manual_priority_accounts(account_id,priority,created_at,updated_at) VALUES('41',3,'now','now');
		DROP TABLE health_samples`); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.AccountMutationSnapshot(t.Context(), "41")
	if err != nil || snapshot.ID != "41" || snapshot.Name != "control-account" || snapshot.ManualPriority == nil || *snapshot.ManualPriority != 3 {
		t.Fatalf("snapshot=%+v error=%v", snapshot, err)
	}
	if _, err := store.AccountMutationSnapshot(t.Context(), "999"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing account error=%v", err)
	}
	if _, err := store.AccountMutationSnapshot(t.Context(), "041"); err == nil {
		t.Fatal("leading-zero ID was accepted")
	}
}

func TestAccountIDsExistValidatesTheWholeSelection(t *testing.T) {
	store, db := accountOwnershipFixture(t, 1)
	if _, err := db.Exec(`DROP TABLE account_stability_samples; DROP TABLE health_samples`); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		name    string
		ids     []string
		want    bool
		invalid bool
	}{
		{"empty", nil, true, false},
		{"single", []string{"41"}, true, false},
		{"multiple", []string{"41", "10001"}, true, false},
		{"duplicate", []string{"41", "41"}, true, false},
		{"missing", []string{"41", "99"}, false, false},
		{"invalid", []string{"041"}, false, true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			exists, err := store.AccountIDsExist(t.Context(), fixture.ids)
			if exists != fixture.want || (err != nil) != fixture.invalid {
				t.Fatalf("exists=%t error=%v", exists, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.AccountIDsExist(ctx, []string{"41"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled read: %v", err)
	}
}

func TestConfirmedAccountControlDoesNotReadGlobalPolicyDisplay(t *testing.T) {
	store, db := accountOwnershipFixture(t, 0)
	if _, err := db.Exec(`DROP TABLE local_groups`); err != nil {
		t.Fatal(err)
	}
	err := store.CommitAccountControlReadback(t.Context(), "41", "fuse", "test", false, business.AccountOperation{
		OperationID: "confirmed-control", OperationType: "account.control", State: "succeeded", Phase: "readback", RemoteConfirmed: true, ReadbackConfirmed: true,
	})
	if err != nil {
		t.Fatalf("confirmed control depended on policy display: %v", err)
	}
	var enabled bool
	if err := db.QueryRow(`SELECT schedulable FROM accounts WHERE id='41'`).Scan(&enabled); err != nil || enabled {
		t.Fatalf("enabled=%t error=%v", enabled, err)
	}
}
