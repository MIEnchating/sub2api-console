package dbconn

import (
	"context"
	"database/sql"
)

// Tx always executes against its own connection, including reads after writes.
type Tx struct {
	raw    *sql.Tx
	driver Driver
}

// LockClause is for reads that feed a write to the same existing row. It must
// not be used by read-only snapshots or aggregate queries.
func (tx *Tx) LockClause() string {
	if tx.driver == SQLite {
		return ""
	}
	return " FOR UPDATE"
}

func (tx *Tx) Commit() error   { return tx.raw.Commit() }
func (tx *Tx) Rollback() error { return tx.raw.Rollback() }
func (tx *Tx) Driver() Driver  { return tx.driver }

// InsertID uses RETURNING on PostgreSQL and driver identity on SQLite/MySQL.
func (tx *Tx) InsertID(ctx context.Context, query string, args ...any) (int64, error) {
	if tx.driver == Postgres {
		var id int64
		err := tx.QueryRowContext(ctx, query+` RETURNING id`, args...).Scan(&id)
		return id, err
	}
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
func (tx *Tx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return tx.raw.ExecContext(ctx, tx.driver.Bind(query), args...)
}
func (tx *Tx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return tx.raw.QueryContext(ctx, tx.driver.Bind(query), args...)
}
func (tx *Tx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return tx.raw.QueryRowContext(ctx, tx.driver.Bind(query), args...)
}
func (db *Database) Exec(query string, args ...any) (sql.Result, error) {
	return db.ExecContext(context.Background(), query, args...)
}
func (db *Database) Query(query string, args ...any) (*sql.Rows, error) {
	return db.QueryContext(context.Background(), query, args...)
}
func (db *Database) QueryRow(query string, args ...any) *sql.Row {
	return db.QueryRowContext(context.Background(), query, args...)
}
