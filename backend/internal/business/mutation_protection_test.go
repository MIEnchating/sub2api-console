package business

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"
)

func TestAccountMutationProtectionsCombineManualControls(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "mutation-protection.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	for accountID, name := range map[string]string{"41": "manual", "42": "paused", "43": "excluded", "44": "fused"} {
		if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES(?,?,'{}','now')`, accountID, name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.AssignManualPriority(ctx, "41", 1, "100", 100, false, "tester"); err != nil {
		t.Fatal(err)
	}
	for accountID, action := range map[string]string{"42": "pause", "43": "exclude", "44": "fuse"} {
		if _, err := store.commitAccountControl(ctx, accountID, action, "tester", nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	protections, err := store.AccountMutationProtections(ctx, []string{"41", "42", "43", "44"})
	if err != nil {
		t.Fatal(err)
	}
	if !protections["41"].ManualPriority || !protections["42"].Paused || !protections["43"].Excluded || !protections["44"].ManualFused {
		t.Fatalf("unexpected protections: %#v", protections)
	}
}

func TestAccountMutationProtectionsSupportsMoreIDsThanSQLiteVariableLimit(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "mutation-protection-large.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	const accountCount = 33_000
	accountIDs := make([]string, accountCount)
	for index := range accountIDs {
		accountIDs[index] = strconv.Itoa(index + 1)
	}
	protectedID := accountIDs[len(accountIDs)-1]
	if _, err := store.db.ExecContext(ctx,
		`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES(?,?,'{}','now')`,
		protectedID, "large-scope-manual-priority"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AssignManualPriority(ctx, protectedID, 1, "100", 100, false, "tester"); err != nil {
		t.Fatal(err)
	}

	protections, err := store.AccountMutationProtections(ctx, accountIDs)
	if err != nil {
		t.Fatal(err)
	}
	if len(protections) != accountCount {
		t.Fatalf("protection count = %d, want %d", len(protections), accountCount)
	}
	if !protections[protectedID].ManualPriority {
		t.Fatalf("manual priority protection missing for account %s", protectedID)
	}
}
