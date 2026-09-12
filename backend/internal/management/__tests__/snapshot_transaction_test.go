package management_test

import (
	"context"
	"reflect"
	"testing"
)

func TestCompleteSnapshotRemovesDeletedAccountPolicyReferencesAndRecordsIDs(t *testing.T) {
	store, db := snapshotStore(t)
	ctx := context.Background()
	_, err := store.UpdatePolicy(ctx, map[string]any{"advanced_policy": map[string]any{
		"scope": map[string]any{"excluded_account_ids": []any{"11", "12"}},
	}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.SyncCompleteManagementSnapshot(ctx, []map[string]any{{"id": "12"}},
		[]map[string]any{{"id": "7", "name": "codex"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	policy, err := store.PolicySnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	scope := policy.AdvancedPolicy["scope"].(map[string]any)
	if !reflect.DeepEqual(scope["excluded_account_ids"], []any{"12"}) {
		t.Fatalf("account policy references = %#v", scope["excluded_account_ids"])
	}
	var deletedID string
	if err := db.QueryRow(`SELECT json_extract(payload_json,'$.deleted_account_ids[0]')
		FROM runtime_events WHERE source_id=?`, result.EventID).Scan(&deletedID); err != nil || deletedID != "11" {
		t.Fatalf("missing deleted account audit ID: %q, %v", deletedID, err)
	}
}

func TestCompleteSnapshotEventFailureRollsBackDeletionAndAccountUpdates(t *testing.T) {
	store, db := snapshotStore(t)
	if _, err := db.Exec(`CREATE TRIGGER reject_snapshot_event BEFORE INSERT ON runtime_events
		WHEN NEW.event_type='management.snapshot.synced' BEGIN SELECT RAISE(ABORT,'event rejected'); END`); err != nil {
		t.Fatal(err)
	}
	_, err := store.SyncCompleteManagementSnapshot(context.Background(), []map[string]any{{"id": "12", "name": "changed"}},
		[]map[string]any{{"id": "7", "name": "codex"}}, "test")
	if err == nil {
		t.Fatal("expected event failure")
	}
	ids, err := store.ManagementAccountIDs(context.Background())
	if err != nil || !reflect.DeepEqual(ids, []string{"11", "12"}) {
		t.Fatalf("deletion was not rolled back: %v, %v", ids, err)
	}
	var name string
	var membershipCount int
	if err := db.QueryRow(`SELECT name,(SELECT COUNT(*) FROM account_groups WHERE account_id='11')
		FROM accounts WHERE id='12'`).Scan(&name, &membershipCount); err != nil {
		t.Fatal(err)
	}
	if name != "same-name" || membershipCount != 1 {
		t.Fatalf("snapshot changes were not rolled back: name=%q memberships=%d", name, membershipCount)
	}
}
