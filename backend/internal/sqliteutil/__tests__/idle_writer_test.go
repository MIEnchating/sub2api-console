package sqliteutil_test

import (
	"database/sql"
	"testing"
	"testing/synctest"
	"time"
)

func TestIdleWriterReleasesCacheAndReopensWithRequiredPragmas(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := databaseFixture(t)
		if _, err := db.Exec(`PRAGMA cache_size=-12345`); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Minute)
		synctest.Wait()
		var cacheSize int
		if err := db.DB.QueryRow(`PRAGMA cache_size`).Scan(&cacheSize); err != nil {
			t.Fatal(err)
		}
		if cacheSize == -12345 {
			t.Fatal("idle writer retained its connection and page cache")
		}
		var mode string
		if err := db.DB.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" {
			t.Fatalf("reopened writer journal mode=%q err=%v", mode, err)
		}
		for pragma, expected := range map[string]int{"foreign_keys": 1, "query_only": 0, "busy_timeout": 10000} {
			var actual int
			if err := db.DB.QueryRow(`PRAGMA ` + pragma).Scan(&actual); err != nil || actual != expected {
				t.Fatalf("reopened writer %s=%d want=%d err=%v", pragma, actual, expected, err)
			}
		}
		if _, err := db.Exec(`UPDATE counter SET value=1`); err != nil {
			t.Fatal(err)
		}
		var value int
		if err := db.QueryRow(`SELECT value FROM counter`).Scan(&value); err != nil || value != 1 {
			t.Fatalf("reopened writer lost update: value=%d err=%v", value, err)
		}
	})
}

func TestIdleCleanupPreservesActiveWriteTransaction(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := databaseFixture(t)
		tx, err := db.BeginTx(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		if _, err := tx.Exec(`UPDATE counter SET value=1`); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Minute)
		synctest.Wait()
		var value int
		if err := tx.QueryRow(`SELECT value FROM counter`).Scan(&value); err != nil || value != 1 {
			t.Fatalf("active write interrupted: value=%d err=%v", value, err)
		}
		if err := db.QueryRow(`SELECT value FROM counter`).Scan(&value); err != nil || value != 0 {
			t.Fatalf("reader saw uncommitted write: value=%d err=%v", value, err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`SELECT value FROM counter`).Scan(&value); err != nil || value != 1 {
			t.Fatalf("reader missed committed write: value=%d err=%v", value, err)
		}
	})
}

func TestWriterReopensWhileReaderKeepsOldSnapshot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := databaseFixture(t)
		reader, err := db.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Rollback()
		var value int
		if err := reader.QueryRow(`SELECT value FROM counter`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Minute)
		synctest.Wait()
		if _, err := db.Exec(`UPDATE counter SET value=2`); err != nil {
			t.Fatal(err)
		}
		if err := reader.QueryRow(`SELECT value FROM counter`).Scan(&value); err != nil || value != 0 {
			t.Fatalf("snapshot changed across writer reopen: value=%d err=%v", value, err)
		}
		if err := reader.Rollback(); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(`SELECT value FROM counter`).Scan(&value); err != nil || value != 2 {
			t.Fatalf("fresh read missed reopened writer: value=%d err=%v", value, err)
		}
	})
}
