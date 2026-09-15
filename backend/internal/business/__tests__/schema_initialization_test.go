package business_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestFailedSchemaCreationLeavesDatabaseRetryable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema-conflict.sqlite3")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// This isolated database has no application tables. The view allows the
	// first CREATE TABLE statements to succeed, then rejects its index.
	if _, err := db.Exec(`CREATE VIEW upstream_identity_hosts AS SELECT
		'host' AS host,'upstream' AS upstream_id,1 AS is_primary;
		PRAGMA user_version=123`); err != nil {
		t.Fatal(err)
	}
	store, err := business.Open(path)
	if err == nil {
		_ = store.Close()
		t.Fatal("schema creation unexpectedly accepted a conflicting view")
	}
	var tables, version int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%'`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Fatalf("failed schema creation left %d application tables; want the original empty schema", tables)
	}
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 123 {
		t.Fatalf("failed schema creation changed the prior version to %d", version)
	}
	if _, err := db.Exec(`DROP VIEW upstream_identity_hosts`); err != nil {
		t.Fatal(err)
	}
	store, err = business.Open(path)
	if err != nil {
		t.Fatalf("schema creation could not be retried after removing the conflict: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(context.Background()); err != nil {
		t.Fatalf("retried schema could not be initialized: %v", err)
	}
	ready, err := store.Ready(context.Background())
	if err != nil || !ready {
		t.Fatalf("retried database readiness = %t, %v", ready, err)
	}
}

func BenchmarkBusinessOpenFreshDatabase(b *testing.B) {
	directory := b.TempDir()
	index := 0
	b.ReportAllocs()
	for b.Loop() {
		path := filepath.Join(directory, "business-"+strconv.Itoa(index)+".sqlite3")
		index++
		store, err := business.Open(path)
		if err != nil {
			b.Fatal(err)
		}
		if err := store.Close(); err != nil {
			b.Fatal(err)
		}
	}
}
