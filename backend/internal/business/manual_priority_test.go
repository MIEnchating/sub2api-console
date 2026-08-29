package business

import (
	"context"
	"strings"
	"testing"
)

func TestManualPriorityAssignmentIsIsolatedByGroupAndVisibleOnAccount(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	for _, values := range [][]any{{"41", "alpha"}, {"42", "beta"}, {"43", "gamma"}} {
		if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(
			id,name,priority,load_factor,concurrency,metadata_json,updated_at
		) VALUES(?,?,20,'5',8,'{}','now')`, values...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO account_groups(account_id,group_name) VALUES
		('41','codex'),('42','claude'),('43','codex')`); err != nil {
		t.Fatal(err)
	}
	assignment, err := store.AssignManualPriority(ctx, "41", 3, "100", 100, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if assignment.Priority != 3 || assignment.LoadFactor != "100" || assignment.Concurrency != 100 {
		t.Fatalf("unexpected assignment: %#v", assignment)
	}
	if _, err := store.AssignManualPriority(ctx, "42", 3, "80", 120, "operator"); err != nil {
		t.Fatalf("different group rejected shared slot: %v", err)
	}
	if _, err := store.AssignManualPriority(ctx, "43", 3, "100", 100, "operator"); err == nil || !strings.Contains(err.Error(), "分组 codex") || !strings.Contains(err.Error(), "已被账号 41 占用") {
		t.Fatalf("occupied slot was accepted: %v", err)
	}
	account, err := store.Account(ctx, "41")
	if err != nil || account.ManualPriority == nil || *account.ManualPriority != 3 {
		t.Fatalf("assignment is missing from account projection: account=%#v err=%v", account, err)
	}
	if err := store.RevertManualPriorityReservation(ctx, "41", "operator"); err != nil {
		t.Fatal(err)
	}
	account, err = store.Account(ctx, "41")
	if err != nil || account.ManualPriority != nil {
		t.Fatalf("cleared assignment remains visible: account=%#v err=%v", account, err)
	}
}

func TestRevertManualPriorityReservationMarksRoutingRecalculationPending(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(
		id,name,schedulable,priority,load_factor,concurrency,metadata_json,updated_at
	) VALUES('41','alpha',1,20,'5',8,'{}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AssignManualPriority(ctx, "41", 3, "100", 100, "operator"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE policy_nodes SET updated_at='2000-01-01T00:00:00Z'
		WHERE policy_key='control-plane';
		INSERT INTO app_state(key,value_json,updated_at) VALUES(?, '{}', '2001-01-01T00:00:00Z')
		ON CONFLICT(key) DO UPDATE SET updated_at=excluded.updated_at`, routingCalculationKey); err != nil {
		t.Fatal(err)
	}
	if pending, err := store.RoutingWritebackPending(ctx); err != nil || pending {
		t.Fatalf("routing unexpectedly pending before clear: pending=%v err=%v", pending, err)
	}

	if err := store.RevertManualPriorityReservation(ctx, "41", "operator"); err != nil {
		t.Fatal(err)
	}
	if pending, err := store.RoutingWritebackPending(ctx); err != nil || !pending {
		t.Fatalf("clear did not schedule recalculation: pending=%v err=%v", pending, err)
	}
}

func TestManualPriorityPolicyCannotShrinkBelowOccupiedSlot(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at)
		VALUES('41','alpha','{}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AssignManualPriority(ctx, "41", 8, "100", 100, "operator"); err != nil {
		t.Fatal(err)
	}
	_, err := store.UpdatePolicy(ctx, map[string]any{
		"advanced_policy": map[string]any{"manual_priority": map[string]any{"reserved_max": 7}},
	}, "operator")
	if err == nil || !strings.Contains(err.Error(), "不能低于当前已占用的 8 号位") {
		t.Fatalf("occupied slot did not protect reserved range: %v", err)
	}
	updated, err := store.UpdatePolicy(ctx, map[string]any{
		"advanced_policy": map[string]any{"manual_priority": map[string]any{"reserved_max": 12}},
	}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	section, ok := updated.AdvancedPolicy["manual_priority"].(map[string]any)
	if !ok || section["reserved_max"] != int64(12) {
		t.Fatalf("updated reserved range is missing: %#v", updated.AdvancedPolicy)
	}
}
