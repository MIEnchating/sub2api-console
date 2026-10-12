package dbconn_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

func TestConcurrentVersionConditionAllowsOnlyOneWinner(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			db := databaseFixture(t, config)
			var winners atomic.Int64
			var workers sync.WaitGroup
			for range 8 {
				workers.Go(func() {
					result, err := db.ExecContext(t.Context(), `UPDATE counter SET value=1 WHERE id=1 AND value=0`)
					if err != nil {
						t.Error(err)
						return
					}
					count, err := result.RowsAffected()
					if err != nil {
						t.Error(err)
						return
					}
					winners.Add(count)
				})
			}
			workers.Wait()
			if winners.Load() != 1 {
				t.Fatalf("expected one winner, got %d", winners.Load())
			}
		})
	}
}

func TestIndependentServerRowsCanCommitWhileAnotherWriterIsOpen(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		if config.Driver == dbconn.SQLite {
			continue
		}
		t.Run(string(config.Driver), func(t *testing.T) {
			db := databaseFixture(t, config)
			if _, err := db.ExecContext(t.Context(), `INSERT INTO counter VALUES (2, 0)`); err != nil {
				t.Fatal(err)
			}
			tx, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			if _, err := tx.ExecContext(t.Context(), `UPDATE counter SET value=1 WHERE id=1`); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			if _, err := db.ExecContext(ctx, `UPDATE counter SET value=2 WHERE id=2`); err != nil {
				t.Fatalf("independent writer blocked: %v", err)
			}
			var value int
			if err := db.QueryRowContext(ctx, `SELECT value FROM counter WHERE id=2`).Scan(&value); err != nil || value != 2 {
				t.Fatalf("independent commit: %d %v", value, err)
			}
		})
	}
}

func TestUnchangedConditionalUpdateReportsMatchedRow(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			db := databaseFixture(t, config)
			result, err := db.ExecContext(t.Context(), `UPDATE counter SET value=0 WHERE id=1 AND value=0`)
			if err != nil {
				t.Fatal(err)
			}
			count, err := result.RowsAffected()
			if err != nil || count != 1 {
				t.Fatalf("matched no-op update: %d %v", count, err)
			}
		})
	}
}
