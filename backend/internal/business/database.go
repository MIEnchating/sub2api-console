package business

import (
	"context"
	"database/sql"
	"sync"

	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

// The single writer queues local mutations in database/sql, where waiting can
// be cancelled without competing for SQLite's busy timeout. WAL readers use a
// separate pool so queued writes cannot exhaust the connections used by pages.
type database struct {
	*sqliteutil.Database
	snapshotLock sync.Mutex
}

func openDatabase(path string) (*database, error) {
	db, err := sqliteutil.OpenDatabase(path)
	if err != nil {
		return nil, err
	}
	return &database{Database: db}, nil
}

func (db *database) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if tx := db.readSnapshot(ctx); tx != nil {
		return tx.QueryContext(ctx, query, args...)
	}
	return db.Database.QueryContext(ctx, query, args...)
}

func (db *database) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if tx := db.readSnapshot(ctx); tx != nil {
		return tx.QueryRowContext(ctx, query, args...)
	}
	return db.Database.QueryRowContext(ctx, query, args...)
}

func (db *database) Query(query string, args ...any) (*sql.Rows, error) {
	return db.QueryContext(context.Background(), query, args...)
}

func (db *database) QueryRow(query string, args ...any) *sql.Row {
	return db.QueryRowContext(context.Background(), query, args...)
}
