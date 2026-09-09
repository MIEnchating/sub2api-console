package business

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

func TestManualPriorityReadModelDoesNotReuseAutomaticState(t *testing.T) {
	for _, mode := range []string{runtimepolicy.Full, runtimepolicy.Monitoring} {
		for _, previous := range []string{"cost_blocked", "fused", "degraded"} {
			t.Run(mode+"/"+previous, func(t *testing.T) {
				store := openReadModelFixture(t)
				ctx := context.Background()
				if _, err := store.SetMode(ctx, mode); err != nil {
					t.Fatal(err)
				}
				if _, err := store.db.ExecContext(ctx, `UPDATE accounts SET schedulable=1,routing_state=?,health_status=? WHERE id='41'`, previous, previous); err != nil {
					t.Fatal(err)
				}
				if _, err := store.AssignManualPriority(ctx, "41", 1, "100", 100, false, "operator"); err != nil {
					t.Fatal(err)
				}
				account, err := store.Account(ctx, "41")
				if err != nil {
					t.Fatal(err)
				}
				if account.Health != "manual_priority" || account.ApplyPending || account.DecisionState != nil || account.DesiredHealth != nil {
					t.Fatalf("manual account retained automatic state: %+v", account.AccountStatus)
				}
			})
		}
	}
}

func TestManualPriorityReadModelPreservesPauseAndPlatformDisable(t *testing.T) {
	for _, mode := range []string{runtimepolicy.Full, runtimepolicy.Monitoring} {
		for _, state := range []string{"paused", "disabled"} {
			t.Run(mode+"/"+state, func(t *testing.T) {
				store := openReadModelFixture(t)
				ctx := context.Background()
				if _, err := store.SetMode(ctx, mode); err != nil {
					t.Fatal(err)
				}
				if _, err := store.AssignManualPriority(ctx, "41", 1, "100", 100, false, "operator"); err != nil {
					t.Fatal(err)
				}
				metadata := `{"status":"active"}`
				if state == "disabled" {
					metadata = `{"status":"disabled"}`
				}
				if _, err := store.db.ExecContext(ctx, `UPDATE accounts SET schedulable=0,paused=?,metadata_json=?,health_status='cost_blocked' WHERE id='41'`, state == "paused", metadata); err != nil {
					t.Fatal(err)
				}
				account, err := store.Account(ctx, "41")
				if err != nil {
					t.Fatal(err)
				}
				if account.Health != state {
					t.Fatalf("manual priority hid current %s state: %+v", state, account.AccountStatus)
				}
			})
		}
	}
}

func TestManualPriorityReadModelIgnoresStaleDecisionUntilOwnershipIsReleased(t *testing.T) {
	store := openReadModelFixture(t)
	ctx := context.Background()
	if _, err := store.AssignManualPriority(ctx, "41", 1, "100", 100, false, "operator"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE accounts SET schedulable=1,routing_state='cost_blocked',
		health_status='cost_blocked',target_schedulable=0 WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO routing_decisions VALUES
		('41','codex',20,0,'cost_blocked','cost_blocked',1,'旧成本墙决策','2099-01-01T00:00:00Z','{"weight":0}')`); err != nil {
		t.Fatal(err)
	}
	account, err := store.Account(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if account.Health != "manual_priority" || account.DecisionReason != nil || account.ApplyPending ||
		account.TargetSchedulable != nil || account.Weight != nil || account.Recovery != nil {
		t.Fatalf("stale automatic result exposed for manual account: %+v", account.AccountStatus)
	}
	if account.SampleCount != 4 || account.HealthScore == nil || *account.HealthScore != 82.5 {
		t.Fatalf("manual control discarded health evidence: %+v", account.AccountStatus)
	}
	if err := store.RevertManualPriorityReservation(ctx, "41", "operator"); err != nil {
		t.Fatal(err)
	}
	account, err = store.Account(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if account.ManualPriority != nil || account.Health != "cost_blocked" {
		t.Fatalf("released account did not return to automatic state: %+v", account.AccountStatus)
	}
}

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
	assignment, err := store.AssignManualPriority(ctx, "41", 3, "100", 100, true, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if assignment.Priority != 3 || assignment.LoadFactor != "100" || assignment.Concurrency != 100 || !assignment.SyncBalanceMultiplier {
		t.Fatalf("unexpected assignment: %#v", assignment)
	}
	if _, err := store.AssignManualPriority(ctx, "42", 3, "80", 120, false, "operator"); err != nil {
		t.Fatalf("different group rejected shared slot: %v", err)
	}
	if _, err := store.AssignManualPriority(ctx, "43", 3, "100", 100, false, "operator"); err == nil || !strings.Contains(err.Error(), "分组 codex") || !strings.Contains(err.Error(), "已被账号 41 占用") {
		t.Fatalf("occupied slot was accepted: %v", err)
	}
	account, err := store.Account(ctx, "41")
	if err != nil || account.ManualPriority == nil || *account.ManualPriority != 3 || !account.ManualSyncBalanceMultiplier {
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

func TestHostBalanceSyncRequiresAtLeastOneEligibleBoundAccount(t *testing.T) {
	store := openReadModelFixture(t)
	ctx := context.Background()
	if _, err := store.AssignManualPriority(ctx, "41", 3, "100", 100, false, "operator"); err != nil {
		t.Fatal(err)
	}
	allowed, err := store.HostBalanceSyncAllowed(ctx, "api.example")
	if err != nil || allowed {
		t.Fatalf("manual-only host balance allowed=%v err=%v", allowed, err)
	}
	if _, err := store.AssignManualPriority(ctx, "41", 3, "100", 100, true, "operator"); err != nil {
		t.Fatal(err)
	}
	allowed, err = store.HostBalanceSyncAllowed(ctx, "api.example")
	if err != nil || !allowed {
		t.Fatalf("sync-enabled host balance allowed=%v err=%v", allowed, err)
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
	if _, err := store.AssignManualPriority(ctx, "41", 3, "100", 100, false, "operator"); err != nil {
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
	if _, err := store.AssignManualPriority(ctx, "41", 8, "100", 100, false, "operator"); err != nil {
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

func TestManualPriorityReleaseRestoresPreviousSchedulingState(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(
		id,name,schedulable,priority,load_factor,concurrency,paused,paused_reason,metadata_json,updated_at
	) VALUES('41','alpha',0,20,'5',8,1,'人工暂停','{}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AssignManualPriority(ctx, "41", 3, "100", 100, false, "operator"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE accounts SET schedulable=1,priority=3,load_factor='100',
		concurrency=100,paused=0,paused_reason=NULL WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	release, err := store.ManualPriorityRelease(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if release.Schedulable == nil || *release.Schedulable || release.Paused == nil || !*release.Paused ||
		release.PausedReason == nil || *release.PausedReason != "人工暂停" {
		t.Fatalf("release did not preserve scheduling baseline: %#v", release)
	}
	if err := store.CommitManualPriorityRelease(ctx, release, "operator", AccountOperation{
		OperationID: "manual-clear-1", OperationType: "account.manual_priority.clear", State: "succeeded",
		Phase: "readback", Actor: "operator", RemoteConfirmed: true, ReadbackConfirmed: true,
	}); err != nil {
		t.Fatal(err)
	}
	var schedulable, paused int64
	var pausedReason sql.NullString
	if err := store.db.QueryRowContext(ctx, `SELECT schedulable,paused,paused_reason FROM accounts WHERE id='41'`).
		Scan(&schedulable, &paused, &pausedReason); err != nil {
		t.Fatal(err)
	}
	if schedulable != 0 || paused != 1 || !pausedReason.Valid || pausedReason.String != "人工暂停" {
		t.Fatalf("schedulable=%d paused=%d reason=%#v", schedulable, paused, pausedReason)
	}
}
