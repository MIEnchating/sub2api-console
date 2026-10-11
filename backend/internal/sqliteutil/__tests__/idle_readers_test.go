package sqliteutil_test

import (
	"database/sql"
	"testing"
	"testing/synctest"
	"time"
)

func TestIdleReadersReleaseConnectionCachesAndReopenReadOnly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := databaseFixture(t)
		reader, err := db.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		// This connection-local setting identifies the original reader. Closing
		// the connection releases its SQLite page cache and resets the setting.
		if _, err := reader.Exec(`PRAGMA cache_size=-12345`); err != nil {
			t.Fatal(err)
		}
		if err := reader.Rollback(); err != nil {
			t.Fatal(err)
		}
		// synctest advances the pool cleaner's clock without a wall-clock wait.
		time.Sleep(2 * time.Minute)
		synctest.Wait()
		var cacheSize int
		if err := db.QueryRow(`PRAGMA cache_size`).Scan(&cacheSize); err != nil {
			t.Fatal(err)
		}
		if cacheSize == -12345 {
			t.Fatal("idle reader retained its connection and page cache")
		}
		var value int
		if err := db.QueryRow(`SELECT value FROM counter`).Scan(&value); err != nil || value != 0 {
			t.Fatalf("reopened reader lost committed data: value=%d err=%v", value, err)
		}
		rows, err := db.Query(`UPDATE counter SET value=1`)
		if rows != nil {
			rows.Close()
		}
		if err == nil {
			t.Fatal("reopened reader allowed a write")
		}
	})
}

func TestIdleReaderCleanupPreservesActiveSnapshotAndConcurrentReaders(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := databaseFixture(t)
		readers := make([]*sql.Tx, 8)
		for i := range readers {
			reader, err := db.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Rollback()
			readers[i] = reader
			var value int
			if err := reader.QueryRow(`SELECT value FROM counter`).Scan(&value); err != nil || value != 0 {
				t.Fatalf("initial snapshot: value=%d err=%v", value, err)
			}
		}
		for _, reader := range readers[1:] {
			if err := reader.Rollback(); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := db.Exec(`UPDATE counter SET value=1`); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Minute)
		synctest.Wait()
		var value int
		if err := readers[0].QueryRow(`SELECT value FROM counter`).Scan(&value); err != nil || value != 0 {
			t.Fatalf("active snapshot interrupted by idle cleanup: value=%d err=%v", value, err)
		}
		if err := db.QueryRow(`SELECT value FROM counter`).Scan(&value); err != nil || value != 1 {
			t.Fatalf("new reader missed committed write: value=%d err=%v", value, err)
		}
	})
}
