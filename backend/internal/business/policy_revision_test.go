package business

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func snapshotRevision(t *testing.T, snapshot PolicySnapshot) string {
	t.Helper()
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	revision, _ := document["revision"].(string)
	if revision == "" {
		t.Fatal("policy snapshot must include a revision for concurrent edits")
	}
	return revision
}

func TestPolicyRevisionRejectsStaleSaveWithoutOverwritingManualProtection(t *testing.T) {
	ctx := context.Background()
	store := openPolicyStore(t)
	initial, err := store.PolicySnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	revision := snapshotRevision(t, initial)
	updated, err := store.UpdatePolicy(ctx, map[string]any{
		"advanced_policy": map[string]any{"scope": map[string]any{"paused_account_ids": []any{"41"}}},
	}, "other-operator")
	if err != nil {
		t.Fatal(err)
	}
	if snapshotRevision(t, updated) == revision {
		t.Fatal("manual protection update must change the revision")
	}
	_, err = store.UpdatePolicy(ctx, map[string]any{
		"expected_revision": revision,
		"global_strategy":   "speed_first",
		"advanced_policy":   map[string]any{"scope": map[string]any{"paused_account_ids": []any{}}},
	}, "stale-operator")
	if !errors.Is(err, ErrPolicyRevisionConflict) {
		t.Fatalf("stale policy save must return a revision conflict: %v", err)
	}
	current, err := store.PolicySnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshotRevision(t, current) != snapshotRevision(t, updated) {
		t.Fatal("rejected stale save modified the policy")
	}
}

func TestPolicyRevisionAcceptsCurrentRevisionAndChangesAfterSave(t *testing.T) {
	ctx := context.Background()
	store := openPolicyStore(t)
	initial, err := store.PolicySnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	revision := snapshotRevision(t, initial)
	updated, err := store.UpdatePolicy(ctx, map[string]any{
		"expected_revision": revision, "global_strategy": "speed_first",
	}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if snapshotRevision(t, updated) == revision || updated.GlobalStrategy == nil || *updated.GlobalStrategy != "speed_first" {
		t.Fatalf("versioned update was not applied: %#v", updated)
	}
}
