package sqliteutil

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestReaderPoolBoundsIdleConnectionsWithoutReducingConcurrency(t *testing.T) {
	db, err := OpenDatabase(filepath.Join(t.TempDir(), "pool.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE values_table(value INTEGER); INSERT INTO values_table VALUES(1)`); err != nil {
		t.Fatal(err)
	}

	readers := make([]*sql.Tx, 8)
	for index := range readers {
		reader, err := db.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
		if err != nil {
			t.Fatalf("reader %d: %v", index, err)
		}
		readers[index] = reader
		var value int
		if err := reader.QueryRow(`SELECT value FROM values_table`).Scan(&value); err != nil || value != 1 {
			t.Fatalf("reader %d snapshot: value=%d err=%v", index, value, err)
		}
	}
	for _, reader := range readers {
		if err := reader.Rollback(); err != nil {
			t.Fatal(err)
		}
	}

	stats := db.reader.Stats()
	if stats.MaxOpenConnections != 8 {
		t.Fatalf("read concurrency changed: max open=%d", stats.MaxOpenConnections)
	}
	if stats.Idle > 4 {
		t.Fatalf("reader idle pool exceeded bound: idle=%d", stats.Idle)
	}
}
