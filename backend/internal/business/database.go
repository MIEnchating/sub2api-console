package business

import (
	"context"
	"database/sql"
	"errors"

	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

// The single writer queues local mutations in database/sql, where waiting can
// be cancelled without competing for SQLite's busy timeout. WAL readers use a
// separate pool so queued writes cannot exhaust the connections used by pages.
type database struct {
	*sql.DB
	reader *sql.DB
}

func openDatabase(path string) (*database, error) {
	writer, err := sql.Open("sqlite", sqliteutil.DSN(path, "_txlock=immediate&_pragma=busy_timeout%2810000%29&_pragma=journal_mode%28WAL%29&_pragma=foreign_keys%28ON%29"))
	if err != nil {
		return nil, err
	}
	writer.SetMaxOpenConns(1)
	writer.SetMaxIdleConns(1)
	if err := writer.PingContext(context.Background()); err != nil {
		return nil, errors.Join(err, writer.Close())
	}
	reader, err := sql.Open("sqlite", sqliteutil.DSN(path, "_pragma=busy_timeout%2810000%29&_pragma=query_only%281%29&_pragma=foreign_keys%28ON%29"))
	if err != nil {
		return nil, errors.Join(err, writer.Close())
	}
	reader.SetMaxOpenConns(8)
	reader.SetMaxIdleConns(8)
	db := &database{DB: writer, reader: reader}
	if err := reader.PingContext(context.Background()); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return db, nil
}

func (db *database) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return db.reader.QueryContext(ctx, query, args...)
}

func (db *database) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return db.reader.QueryRowContext(ctx, query, args...)
}

func (db *database) Query(query string, args ...any) (*sql.Rows, error) {
	return db.QueryContext(context.Background(), query, args...)
}

func (db *database) QueryRow(query string, args ...any) *sql.Row {
	return db.QueryRowContext(context.Background(), query, args...)
}

func (db *database) BeginTx(ctx context.Context, options *sql.TxOptions) (*sql.Tx, error) {
	if options != nil && options.ReadOnly {
		return db.reader.BeginTx(ctx, options)
	}
	return db.DB.BeginTx(ctx, options)
}

func (db *database) Close() error {
	return errors.Join(db.reader.Close(), db.DB.Close())
}
