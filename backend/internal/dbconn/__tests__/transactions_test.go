package dbconn_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

func databaseFixture(t *testing.T, config dbconn.Config) *dbconn.Database {
	t.Helper()
	db, err := dbconn.Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE counter (id INTEGER PRIMARY KEY, value INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.ExecContext(context.Background(), `DROP TABLE counter`); err != nil {
			t.Error(err)
		}
	})
	if _, err := db.ExecContext(t.Context(), `INSERT INTO counter VALUES (1, 0)`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestRollbackDiscardsUncommittedWriteVisibleInsideTransaction(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			db := databaseFixture(t, config)
			tx, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err := tx.ExecContext(t.Context(), `UPDATE counter SET value=7 WHERE id=1`); err != nil {
				t.Fatal(err)
			}
			var value int
			if err := tx.QueryRowContext(t.Context(), `SELECT value FROM counter WHERE id=1`).Scan(&value); err != nil || value != 7 {
				t.Fatalf("transaction read: %d %v", value, err)
			}
			if err := tx.Rollback(); err != nil {
				t.Fatal(err)
			}
			if err := db.QueryRowContext(t.Context(), `SELECT value FROM counter WHERE id=1`).Scan(&value); err != nil || value != 0 {
				t.Fatalf("rollback read: %d %v", value, err)
			}
		})
	}
}

func TestReadSnapshotSurvivesConcurrentWriterAndRejectsWrites(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			db := databaseFixture(t, config)
			tx, err := db.BeginSnapshot(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			var value int
			if err := tx.QueryRowContext(t.Context(), `SELECT value FROM counter WHERE id=1`).Scan(&value); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(t.Context(), `UPDATE counter SET value=1 WHERE id=1`); err != nil {
				t.Fatal(err)
			}
			if err := tx.QueryRowContext(t.Context(), `SELECT value FROM counter WHERE id=1`).Scan(&value); err != nil || value != 0 {
				t.Fatalf("snapshot changed: %d %v", value, err)
			}
			if _, err := tx.ExecContext(t.Context(), `UPDATE counter SET value=2`); err == nil {
				t.Fatal("read-only snapshot allowed mutation")
			}
			if err := db.QueryRowContext(t.Context(), `SELECT value FROM counter WHERE id=1`).Scan(&value); err != nil || value != 1 {
				t.Fatalf("fresh reader: %d %v", value, err)
			}
		})
	}
}

func TestReadOnlyTransactionRejectsMutation(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			db := databaseFixture(t, config)
			tx, err := db.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err := tx.ExecContext(t.Context(), `DELETE FROM counter`); err == nil {
				t.Fatal("read-only transaction allowed mutation")
			}
		})
	}
}
