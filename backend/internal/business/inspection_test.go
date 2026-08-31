package business

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInspectionHeartbeatsNormalizesMissingLegacyArrays(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "inspection.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	legacy := `[{"checked_at":"2026-08-27T00:00:00Z","completed_at":"2026-08-27T00:00:01Z","status":"succeeded","task_id":null,"error":null,"skipped":true}]`
	if _, err := store.db.Exec(
		`INSERT INTO app_state(key,value_json,updated_at) VALUES(?,?,?)`,
		inspectionHistoryKey,
		legacy,
		"2026-08-27T00:00:01Z",
	); err != nil {
		t.Fatal(err)
	}

	records, err := store.InspectionHeartbeats(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records=%d", len(records))
	}
	if records[0].Operations == nil || records[0].OperationTiming == nil {
		t.Fatalf("legacy arrays were not normalized: %#v", records[0])
	}
	encoded, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != `[{"checked_at":"2026-08-27T00:00:00Z","completed_at":"2026-08-27T00:00:01Z","status":"succeeded","operations":[],"operation_timings":[],"task_id":null,"error":null,"skipped":true}]` {
		t.Fatalf("unexpected JSON: %s", encoded)
	}
}

func TestClearInspectionHeartbeatsKeepsSchedulingState(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "inspection.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO app_state(key,value_json,updated_at) VALUES(?,?,?)`,
		inspectionHistoryKey, `[{"checked_at":"2026-08-27T00:00:00Z","status":"succeeded"}]`, "now"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO app_state(key,value_json,updated_at) VALUES(?,?,?)`,
		inspectionHeartbeatKey, `{"traffic":"2026-08-27T00:00:00Z"}`, "now"); err != nil {
		t.Fatal(err)
	}
	deleted, err := store.ClearInspectionHeartbeats(ctx)
	if err != nil || deleted != 1 {
		t.Fatalf("deleted=%d err=%v", deleted, err)
	}
	history, err := store.InspectionHeartbeats(ctx, 20)
	if err != nil || len(history) != 0 {
		t.Fatalf("history=%#v err=%v", history, err)
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM app_state WHERE key=?`, inspectionHeartbeatKey).Scan(&count); err != nil || count != 1 {
		t.Fatalf("scheduling heartbeat was deleted: count=%d err=%v", count, err)
	}
}

func TestRecordInspectionHeartbeatRejectsCorruptHistoryWithoutOverwritingIt(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "inspection.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	const corrupt = `{not-json}`
	if _, err := store.db.Exec(`INSERT INTO app_state(key,value_json,updated_at) VALUES(?,?,?)`, inspectionHistoryKey, corrupt, "now"); err != nil {
		t.Fatal(err)
	}
	err = store.RecordInspectionHeartbeat(ctx, InspectionHeartbeat{CheckedAt: "2026-08-27T00:00:00Z", Status: "succeeded"})
	if err == nil || !strings.Contains(err.Error(), "心跳历史损坏") {
		t.Fatalf("corrupt history error = %v", err)
	}
	var stored string
	if err := store.db.QueryRow(`SELECT value_json FROM app_state WHERE key=?`, inspectionHistoryKey).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != corrupt {
		t.Fatalf("corrupt history was overwritten: %q", stored)
	}
}

func TestReconcileWithoutStaleStateDoesNotWaitForWriterLock(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "inspection-readonly-reconcile.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	writer, err := store.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if _, err := writer.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	defer writer.ExecContext(context.Background(), "ROLLBACK")

	checkContext, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	interrupted, err := store.ReconcileInterruptedInspections(checkContext, time.Now().UTC())
	if err != nil || interrupted != 0 {
		t.Fatalf("read-only reconciliation waited for writer lock: interrupted=%d err=%v", interrupted, err)
	}
}

func TestRoutingWritebackPendingTracksAttemptsAfterCurrentDecision(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "writeback-pending.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
		INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','alpha','{}','2026-08-28T12:00:00Z');
		INSERT INTO routing_decisions(account_id,group_name,routing_state,updated_at,payload_json)
		VALUES('41','codex','fused','2026-08-28T12:00:00Z','{}');
		INSERT INTO app_state(key,value_json,updated_at) VALUES('routing-decision-epoch','{}','2026-08-28T11:00:00Z');
		INSERT INTO app_state(key,value_json,updated_at) VALUES('routing-calculation-at','{}','2026-08-28T12:00:00Z');
		UPDATE policy_nodes SET updated_at='2026-08-28T11:00:00Z' WHERE policy_key='control-plane';
	`); err != nil {
		t.Fatal(err)
	}
	if pending, err := store.RoutingWritebackPending(ctx); err != nil || !pending {
		t.Fatalf("decision without attempt: pending=%v err=%v", pending, err)
	}
	insertAttempt := func(sourceID int, state, createdAt string) {
		t.Helper()
		if _, err := store.db.Exec(`INSERT INTO operation_audit(
			source_id,operation_id,operation_type,state,phase,actor,source,object_type,object_id,
			group_names_json,writeback,created_at
		) VALUES(?,?, 'routing.writeback',?, 'remote-write','test','console','account','41','[]',1,?)`,
			sourceID, "operation-"+state, state, createdAt); err != nil {
			t.Fatal(err)
		}
	}
	insertAttempt(-1, "failed", "2026-08-28T12:01:00Z")
	if pending, err := store.RoutingWritebackPending(ctx); err != nil || !pending {
		t.Fatalf("failed attempt: pending=%v err=%v", pending, err)
	}
	insertAttempt(-2, "succeeded", "2026-08-28T12:02:00Z")
	if pending, err := store.RoutingWritebackPending(ctx); err != nil || pending {
		t.Fatalf("successful retry: pending=%v err=%v", pending, err)
	}
}

func TestRoutingWritebackPendingTreatsNewerPolicyAsCalculationDue(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "policy-calculation-due.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	calculatedAt := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.PersistRoutingRound(ctx, nil, nil, nil, nil, nil, nil, nil, true, calculatedAt); err != nil {
		t.Fatal(err)
	}
	if pending, err := store.RoutingWritebackPending(ctx); err != nil || pending {
		t.Fatalf("fresh calculation: pending=%v err=%v", pending, err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE policy_nodes SET updated_at=? WHERE policy_key='control-plane'`,
		calculatedAt.Add(time.Minute).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if pending, err := store.RoutingWritebackPending(ctx); err != nil || !pending {
		t.Fatalf("newer policy must make routing calculation due: pending=%v err=%v", pending, err)
	}
	if err := store.PersistRoutingRound(ctx, nil, nil, nil, nil, nil, nil, nil, true, calculatedAt.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if pending, err := store.RoutingWritebackPending(ctx); err != nil || pending {
		t.Fatalf("recalculation must clear policy due state even without decisions: pending=%v err=%v", pending, err)
	}
}
