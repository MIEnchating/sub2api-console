package sqliteutil

import (
	"context"
	"database/sql"
	"errors"
	"time"

	_ "modernc.org/sqlite"
)

// Database queues writes on one connection while WAL readers use a separate
// read-only pool. Write transactions acquire their lock before reading state.
type Database struct {
	*sql.DB
	reader *sql.DB
}

func OpenDatabase(path string) (*Database, error) {
	writer, err := sql.Open("sqlite", DSN(path, "_txlock=immediate&_pragma=busy_timeout%2810000%29&_pragma=journal_mode%28WAL%29&_pragma=foreign_keys%28ON%29"))
	if err != nil {
		return nil, err
	}
	writer.SetMaxOpenConns(1)
	writer.SetMaxIdleConns(1)
	// An inactive store should not retain the writer's page/statement caches.
	// database/sql only closes idle connections; in-flight transactions survive.
	writer.SetConnMaxIdleTime(time.Minute)
	if err := writer.PingContext(context.Background()); err != nil {
		return nil, errors.Join(err, writer.Close())
	}
	reader, err := sql.Open("sqlite", DSN(path, "_pragma=busy_timeout%2810000%29&_pragma=query_only%281%29&_pragma=foreign_keys%28ON%29"))
	if err != nil {
		return nil, errors.Join(err, writer.Close())
	}
	reader.SetMaxOpenConns(8)
	// Keep the full read concurrency available, but retain only half of the
	// reader connections after a burst so their SQLite page and statement caches
	// do not remain resident while the store is idle.
	reader.SetMaxIdleConns(4)
	// Release per-connection SQLite caches after bursts, while keeping active
	// snapshots and the full read concurrency available. Busy readers are reused.
	reader.SetConnMaxIdleTime(time.Minute)
	db := &Database{DB: writer, reader: reader}
	if err := reader.PingContext(context.Background()); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return db, nil
}

func (db *Database) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return db.reader.QueryContext(ctx, query, args...)
}

func (db *Database) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return db.reader.QueryRowContext(ctx, query, args...)
}

func (db *Database) Query(query string, args ...any) (*sql.Rows, error) {
	return db.QueryContext(context.Background(), query, args...)
}

func (db *Database) QueryRow(query string, args ...any) *sql.Row {
	return db.QueryRowContext(context.Background(), query, args...)
}

func (db *Database) BeginTx(ctx context.Context, options *sql.TxOptions) (*sql.Tx, error) {
	if options != nil && options.ReadOnly {
		return db.reader.BeginTx(ctx, options)
	}
	return db.DB.BeginTx(ctx, options)
}

func (db *Database) Close() error { return errors.Join(db.reader.Close(), db.DB.Close()) }
