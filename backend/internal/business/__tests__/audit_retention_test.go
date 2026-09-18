package business_test

import (
	"database/sql"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func auditRetentionStore(t *testing.T) (*business.Store, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "audit.db")
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
	if _, err := db.Exec(`INSERT INTO accounts(id,name,updated_at) VALUES('41','fixture','now')`); err != nil {
		t.Fatal(err)
	}
	return store, db
}

func automaticConfirmation() business.AccountOperation {
	return business.AccountOperation{OperationID: "confirm", OperationType: "routing.writeback", ObjectID: "41",
		Actor: "自动巡检", State: "succeeded", Phase: "readback", ReadbackConfirmed: true,
		Before: map[string]any{"concurrency": 100}, After: map[string]any{"concurrency": 100}, GroupNames: []string{"fixture"}}
}

func TestIdenticalAutomaticReadbacksWithinHourKeepOneAudit(t *testing.T) {
	store, db := auditRetentionStore(t)
	op := automaticConfirmation()
	for i := 0; i < 5; i++ {
		op.OperationID = "confirm-" + strconv.Itoa(i)
		if err := store.RecordAccountOperation(t.Context(), op); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM operation_audit`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("identical readbacks accumulated %d audits; want 1", count)
	}
}

func TestAutomaticReadbackKeepsHourlyConfirmation(t *testing.T) {
	store, db := auditRetentionStore(t)
	op := automaticConfirmation()
	if err := store.RecordAccountOperation(t.Context(), op); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE operation_audit SET created_at=?`, time.Now().UTC().Add(-2*time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	op.OperationID = "later"
	if err := store.RecordAccountOperation(t.Context(), op); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM operation_audit`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("hourly confirmation missing: count=%d", count)
	}
}

func TestReadbackAfterFailureKeepsRecoveryEvidence(t *testing.T) {
	for _, kind := range []string{"routing.writeback", "routing.restore", "cleanup.delete"} {
		t.Run(kind, func(t *testing.T) {
			store, db := auditRetentionStore(t)
			op := automaticConfirmation()
			if err := store.RecordAccountOperation(t.Context(), op); err != nil {
				t.Fatal(err)
			}
			failure := automaticConfirmation()
			message := "fixture remote failure"
			failure.OperationType, failure.OperationID, failure.State = kind, "failure", "failed"
			failure.ReadbackConfirmed, failure.Error = false, &message
			if err := store.RecordAccountOperation(t.Context(), failure); err != nil {
				t.Fatal(err)
			}
			op.OperationID = "recovered"
			if err := store.RecordAccountOperation(t.Context(), op); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := db.QueryRow(`SELECT count(*) FROM operation_audit`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 3 {
				t.Fatalf("recovery confirmation missing: count=%d", count)
			}
			account, err := store.Account(t.Context(), "41")
			if err != nil {
				t.Fatal(err)
			}
			if account.ApplyError != nil {
				t.Fatalf("recovery retained apply error: %v", account.ApplyError)
			}
		})
	}
}

func TestDistinctReadbacksRemainAuditable(t *testing.T) {
	for _, kind := range []string{"manual", "remote-write", "changed-target", "changed-current", "changed-groups", "missing-payload"} {
		t.Run(kind, func(t *testing.T) {
			store, db := auditRetentionStore(t)
			op := automaticConfirmation()
			switch kind {
			case "manual":
				op.Actor = "operator"
			case "remote-write":
				op.RemoteConfirmed, op.Writeback = true, true
			case "changed-target":
				op.After = map[string]any{"concurrency": 50}
			case "missing-payload":
				op.Before, op.After = nil, nil
			}
			if err := store.RecordAccountOperation(t.Context(), op); err != nil {
				t.Fatal(err)
			}
			if kind == "changed-current" {
				op.Before, op.After = map[string]any{"concurrency": 50}, map[string]any{"concurrency": 50}
			}
			if kind == "changed-groups" {
				op.GroupNames = []string{"other"}
			}
			op.OperationID = "second"
			if err := store.RecordAccountOperation(t.Context(), op); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := db.QueryRow(`SELECT count(*) FROM operation_audit`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 2 {
				t.Fatalf("distinct operation lost: count=%d", count)
			}
		})
	}
}
