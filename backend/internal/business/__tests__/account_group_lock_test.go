package business_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func seedGroupLock(t *testing.T, store *business.Store) {
	t.Helper()
	_, err := store.SyncManagementSnapshot(t.Context(), []map[string]any{
		{"id": "41", "name": "same-name", "platform": "openai", "schedulable": true, "priority": 50, "group_ids": []any{"7", "8"}},
		{"id": "42", "name": "same-name", "platform": "openai", "group_ids": []any{"7"}},
	}, []map[string]any{{"id": "7", "name": "first", "platform": "openai"}, {"id": "8", "name": "second", "platform": "openai"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
}

func TestAccountGroupLockPersistsAcrossSyncPolicyEditAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	seedGroupLock(t, store)
	initial, err := store.Account(t.Context(), "41")
	if err != nil || initial.GroupsLocked {
		t.Fatalf("default: %+v %v", initial, err)
	}
	if err := store.SetAccountGroupsLocked(t.Context(), "41", true, "operator"); err != nil {
		t.Fatal(err)
	}
	seedGroupLock(t, store)
	if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"scope": map[string]any{"manage_all_accounts": true}}}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"scope": map[string]any{"group_locked_account_ids": []any{}}}}, "test"); err == nil {
		t.Fatal("generic policy bypass accepted")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	locked, err := store.Account(t.Context(), "41")
	if err != nil || !locked.GroupsLocked || len(locked.Groups) != 2 {
		t.Fatalf("lock lost: %+v %v", locked, err)
	}
	peer, err := store.Account(t.Context(), "42")
	if err != nil || peer.GroupsLocked {
		t.Fatalf("same name peer changed: %+v %v", peer, err)
	}
	if err := store.SetAccountGroupsLocked(t.Context(), "41", false, "operator"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncPricingAccountGroups(t.Context(), map[string][]string{"41": {"8"}}, "test"); err != nil {
		t.Fatal(err)
	}
}

func TestAccountGroupLockOnlyProtectsGroupWrites(t *testing.T) {
	store, db := concurrencyStore(t)
	seedGroupLock(t, store)
	if err := store.SetAccountGroupsLocked(t.Context(), "41", true, "operator"); err != nil {
		t.Fatal(err)
	}
	protection, err := store.AccountMutationProtection(t.Context(), "41")
	if err != nil || !protection.GroupsLocked || protection.Protected() || len(protection.Reasons()) != 0 {
		t.Fatalf("unrelated protection: %+v %v", protection, err)
	}
	account, err := store.Account(t.Context(), "41")
	if err != nil || account.ManualPriority != nil || account.Schedulable == nil || !*account.Schedulable || account.Priority == nil || *account.Priority != 50 {
		t.Fatalf("scheduling changed: %+v %v", account, err)
	}
	operation := business.AccountOperation{OperationID: "blocked-write", OperationType: "account.groups", ObjectID: "41", State: "succeeded", Phase: "readback", Actor: "test"}
	for _, err := range []error{
		store.CommitAccountGroups(t.Context(), "41", []string{"7", "8"}, []string{"8"}, operation),
		store.CommitAccountGroupsReadback(t.Context(), "41", []business.LocalOnboardingGroup{{ID: "8", Name: "second"}}, nil, operation),
	} {
		if !errors.Is(err, business.ErrAccountGroupsLocked) {
			t.Fatalf("write not blocked: %v", err)
		}
	}
	if _, err := store.SyncPricingAccountGroups(t.Context(), map[string][]string{"41": {"8"}}, "test"); !errors.Is(err, business.ErrAccountGroupsLocked) {
		t.Fatalf("pricing not blocked: %v", err)
	}
	if err := store.SetAccountGroupsLocked(t.Context(), "41", true, "operator"); err != nil {
		t.Fatal(err)
	}
	var audits int
	if err := db.QueryRow(`SELECT COUNT(*) FROM operation_audit WHERE operation_type='account.groups_lock'`).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("audit count=%d err=%v", audits, err)
	}
}

func TestAccountGroupLockRejectsMissingGroupsButAllowsUnlockAfterGroupsDisappear(t *testing.T) {
	store, db := concurrencyStore(t)
	seedGroupLock(t, store)
	for _, id := range []string{"", "same-name", "999"} {
		if err := store.SetAccountGroupsLocked(t.Context(), id, true, "test"); err == nil {
			t.Fatalf("accepted %q", id)
		}
	}
	if _, err := db.Exec(`DELETE FROM account_groups WHERE account_id='42'`); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAccountGroupsLocked(t.Context(), "42", true, "test"); err == nil {
		t.Fatal("locked ungrouped account")
	}
	if err := store.SetAccountGroupsLocked(t.Context(), "41", true, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM account_groups WHERE account_id='41'`); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAccountGroupsLocked(t.Context(), "41", false, "test"); err != nil {
		t.Fatal(err)
	}
}

func TestDeletingGroupLockedAccountClearsLockWithoutAffectingPeer(t *testing.T) {
	store, _ := concurrencyStore(t)
	seedGroupLock(t, store)
	for _, id := range []string{"41", "42"} {
		if err := store.SetAccountGroupsLocked(t.Context(), id, true, "test"); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.DeleteAccountProjection(t.Context(), "41", business.AccountOperation{OperationID: "delete-lock", OperationType: "account.delete", ObjectID: "41", State: "succeeded", Phase: "readback", Actor: "test"}); err != nil {
		t.Fatal(err)
	}
	seedGroupLock(t, store)
	replacement, err := store.Account(t.Context(), "41")
	if err != nil || replacement.GroupsLocked {
		t.Fatalf("replacement inherited lock: %v", err)
	}
	peer, err := store.Account(t.Context(), "42")
	if err != nil || !peer.GroupsLocked {
		t.Fatalf("peer lock removed: %v", err)
	}
}
