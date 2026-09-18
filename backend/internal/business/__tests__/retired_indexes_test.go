package business_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestReopenRemovesReplacedAuditIndexAndPreservesHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "indexes.db")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE INDEX ix_operation_audit_log_recent ON operation_audit(created_at DESC,source_id);
		INSERT INTO operation_audit(source_id,operation_id,operation_type,state,phase,created_at)
		VALUES(-1,'preserved','routing.writeback','failed','remote-write','now')`); err != nil {
		t.Fatal(err)
	}
	store, err = business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name='ix_operation_audit_log_recent'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("replaced audit index still consumes space and write IO")
	}
	limit := 10
	rows, err := store.AuditEvents(t.Context(), &limit, true)
	if err != nil || len(rows) != 1 || rows[0].OperationID != "preserved" {
		t.Fatalf("history changed after index cleanup: %+v %v", rows, err)
	}
}
