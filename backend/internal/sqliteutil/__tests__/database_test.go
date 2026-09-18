package sqliteutil_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

func databaseFixture(t *testing.T) *sqliteutil.Database {
	t.Helper()
	db, err := sqliteutil.OpenDatabase(filepath.Join(t.TempDir(), "pool.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE counter(value INTEGER); INSERT INTO counter VALUES(0)`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestReaderKeepsSnapshotWhileWriterCommits(t *testing.T) {
	db := databaseFixture(t)
	tx, err := db.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var value int
	if err := tx.QueryRow(`SELECT value FROM counter`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE counter SET value=1`); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(`SELECT value FROM counter`).Scan(&value); err != nil || value != 0 {
		t.Fatalf("snapshot changed: %d %v", value, err)
	}
	if err := db.QueryRow(`SELECT value FROM counter`).Scan(&value); err != nil || value != 1 {
		t.Fatalf("fresh read missed commit: %d %v", value, err)
	}
	if _, err := tx.Exec(`UPDATE counter SET value=2`); err == nil {
		t.Fatal("read-only transaction allowed mutation")
	}
}

func TestConcurrentTransactionsSerializeReadModifyWrite(t *testing.T) {
	db := databaseFixture(t)
	const workers = 24
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			tx, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				t.Error(err)
				return
			}
			defer tx.Rollback()
			var value int
			if err := tx.QueryRow(`SELECT value FROM counter`).Scan(&value); err != nil {
				t.Error(err)
				return
			}
			if _, err := tx.Exec(`UPDATE counter SET value=?`, value+1); err != nil {
				t.Error(err)
				return
			}
			if err := tx.Commit(); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	var value int
	if err := db.QueryRow(`SELECT value FROM counter`).Scan(&value); err != nil || value != workers {
		t.Fatalf("lost concurrent updates: %d %v", value, err)
	}
}

func TestCancelledWriterDoesNotBlockReadersOrLaterWrites(t *testing.T) {
	db := databaseFixture(t)
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := db.ExecContext(ctx, `UPDATE counter SET value=1`); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled writer: %v", err)
	}
	var value int
	if err := db.QueryRowContext(t.Context(), `SELECT value FROM counter`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE counter SET value=2`); err != nil {
		t.Fatal(err)
	}
}
