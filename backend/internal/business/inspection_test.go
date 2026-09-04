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

func TestAutoInspectionConfigKeepsLegacyDefaultsAndPersistsRateBatch(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "inspection-config.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO app_state(key,value_json,updated_at) VALUES(?,?,?)`, inspectionConfigKey, `{"enabled":true,"interval_seconds":30}`, "now"); err != nil {
		t.Fatal(err)
	}
	legacy, err := store.AutoInspectionConfig(ctx)
	if err != nil || legacy.AccountRateSyncIntervalSeconds != 120 || legacy.AccountRateSyncBatchSize != 0 {
		t.Fatalf("legacy config=%#v err=%v", legacy, err)
	}
	updated, err := store.UpdateAutoInspectionConfig(ctx, AutoInspectionConfig{Enabled: true, IntervalSeconds: 30, AccountRateSyncIntervalSeconds: 300, AccountRateSyncBatchPercent: 20})
	if err != nil || updated.AccountRateSyncBatchPercent != 20 {
		t.Fatalf("updated config=%#v err=%v", updated, err)
	}
	loaded, err := store.AutoInspectionConfig(ctx)
	if err != nil || loaded.AccountRateSyncIntervalSeconds != 300 || loaded.AccountRateSyncBatchPercent != 20 {
		t.Fatalf("loaded config=%#v err=%v", loaded, err)
	}
}

func TestResetInspectionTaskOnlyClearsMatchingAttempt(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "inspection-reset.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	markedAt := time.Date(2026, 9, 3, 7, 33, 16, 0, time.UTC)
	if err := store.MarkInspectionTask(ctx, "price-management", markedAt); err != nil {
		t.Fatal(err)
	}
	if err := store.ResetInspectionTask(ctx, "price-management", markedAt.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if due, err := store.InspectionTaskDue(ctx, "price-management", 36000, markedAt.Add(time.Hour)); err != nil || due {
		t.Fatalf("mismatched reset changed cooldown: due=%v err=%v", due, err)
	}
	if err := store.ResetInspectionTask(ctx, "price-management", markedAt); err != nil {
		t.Fatal(err)
	}
	if due, err := store.InspectionTaskDue(ctx, "price-management", 36000, markedAt.Add(time.Hour)); err != nil || !due {
		t.Fatalf("matching reset did not restore due state: due=%v err=%v", due, err)
	}
}

func TestInspectionHeartbeatsNormalizesLegacyBatchFailuresAsPartial(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "inspection-partial.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	legacy := `[` +
		`{"checked_at":"2026-08-27T00:00:02Z","status":"failed","error":"账号倍率与名称同步部分失败：缺失 0，失败 23；调度计算：数据库不可用"},` +
		`{"checked_at":"2026-08-27T00:00:01Z","status":"failed","error":"账号倍率与名称同步部分失败：缺失 0，失败 23"}` +
		`]`
	if _, err := store.db.Exec(
		`INSERT INTO app_state(key,value_json,updated_at) VALUES(?,?,?)`,
		inspectionHistoryKey,
		legacy,
		"2026-08-27T00:00:02Z",
	); err != nil {
		t.Fatal(err)
	}

	records, err := store.InspectionHeartbeats(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].Status != "failed" || records[1].Status != "partial" {
		t.Fatalf("legacy failure statuses were normalized incorrectly: %#v", records)
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

func TestInspectionHeartbeatsReportsCorruptHistoryInsteadOfReturningEmpty(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "inspection-corrupt-read.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO app_state(key,value_json,updated_at) VALUES(?,?,?)`,
		inspectionHistoryKey, `{not-json}`, "now"); err != nil {
		t.Fatal(err)
	}

	records, err := store.InspectionHeartbeats(ctx, 20)
	if err == nil || !strings.Contains(err.Error(), "心跳历史损坏") {
		t.Fatalf("records=%#v err=%v", records, err)
	}
}

func TestReconcileInterruptedInspectionsReportsCorruptHistory(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "inspection-corrupt-reconcile.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO app_state(key,value_json,updated_at) VALUES(?,?,?)`,
		inspectionHistoryKey, `{not-json}`, "now"); err != nil {
		t.Fatal(err)
	}

	interrupted, err := store.ReconcileInterruptedInspections(ctx, time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "心跳历史损坏") {
		t.Fatalf("interrupted=%d err=%v", interrupted, err)
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

func TestInspectionTransactionsReturnContextCancellationAndLeaveConnectionReusable(t *testing.T) {
	testCases := map[string]func(context.Context, *Store, time.Time) error{
		"record heartbeat": func(ctx context.Context, store *Store, now time.Time) error {
			return store.RecordInspectionHeartbeat(ctx, InspectionHeartbeat{
				CheckedAt: now.Add(time.Minute).Format(time.RFC3339Nano),
				Status:    "running",
			})
		},
		"acquire lease": func(ctx context.Context, store *Store, now time.Time) error {
			_, err := store.AcquireInspectionLease(ctx, "cancelled-owner", 10001, "remote.example", now, now, time.Minute)
			return err
		},
		"reconcile interrupted": func(ctx context.Context, store *Store, now time.Time) error {
			_, err := store.ReconcileInterruptedInspections(ctx, now)
			return err
		},
	}
	for name, operation := range testCases {
		t.Run(name, func(t *testing.T) {
			store, err := Open(filepath.Join(t.TempDir(), "inspection-cancelled-transaction.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			background := context.Background()
			if err := store.Bootstrap(background); err != nil {
				t.Fatal(err)
			}
			store.db.SetMaxOpenConns(2)
			store.db.SetMaxIdleConns(2)
			now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
			if err := store.RecordInspectionHeartbeat(background, InspectionHeartbeat{
				CheckedAt: now.Add(-time.Minute).Format(time.RFC3339Nano),
				Status:    "running",
			}); err != nil {
				t.Fatal(err)
			}
			writer, err := store.db.Conn(background)
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Close()
			if _, err := writer.ExecContext(background, "BEGIN IMMEDIATE"); err != nil {
				t.Fatal(err)
			}
			defer writer.ExecContext(context.Background(), "ROLLBACK")

			operationContext, cancel := context.WithCancel(background)
			result := make(chan error, 1)
			go func() { result <- operation(operationContext, store, now) }()
			deadline := time.Now().Add(time.Second)
			blockedSince := time.Time{}
			for time.Now().Before(deadline) {
				if store.db.Stats().InUse < 2 {
					blockedSince = time.Time{}
				} else if blockedSince.IsZero() {
					blockedSince = time.Now()
				} else if time.Since(blockedSince) >= 10*time.Millisecond {
					break
				}
				time.Sleep(time.Millisecond)
			}
			if blockedSince.IsZero() || time.Since(blockedSince) < 10*time.Millisecond {
				cancel()
				t.Fatal("inspection operation did not reach the blocked transaction")
			}
			cancel()
			if _, err := writer.ExecContext(background, "ROLLBACK"); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-result:
				if err != context.Canceled {
					t.Fatalf("cancelled transaction returned %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("cancelled transaction did not return")
			}
			reuseContext, cancelReuse := context.WithTimeout(background, time.Second)
			defer cancelReuse()
			tx, err := store.db.BeginTx(reuseContext, nil)
			if err != nil {
				t.Fatalf("connection was not reusable after cancellation: %v", err)
			}
			if err := tx.Rollback(); err != nil {
				t.Fatal(err)
			}
		})
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
