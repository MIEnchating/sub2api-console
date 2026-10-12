package dbconn

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
)

type unchangedResult struct{}

func (unchangedResult) RowsAffected() (int64, error) { return 0, nil }
func (unchangedResult) LastInsertId() (int64, error) { return 0, errors.New("没有插入新记录") }

// InsertIfAbsentContext accepts an INSERT ending in ON CONFLICT(...) DO NOTHING.
// MySQL uses a normal INSERT and handles only duplicate-key errors; INSERT IGNORE
// would also suppress truncation and invalid-value errors.
func insertIfAbsent(ctx context.Context, d Driver, exec interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, query string, args ...any) (sql.Result, error) {
	if d == MySQL {
		index := strings.LastIndex(query, " ON CONFLICT(")
		if index < 0 || !strings.HasSuffix(strings.TrimSpace(query), "DO NOTHING") {
			return nil, errors.New("缺少显式唯一冲突范围")
		}
		query = query[:index]
	}
	result, err := exec.ExecContext(ctx, query, args...)
	var duplicate *mysql.MySQLError
	if d == MySQL && errors.As(err, &duplicate) && duplicate.Number == 1062 {
		return unchangedResult{}, nil
	}
	return result, err
}
func (db *Database) InsertIfAbsentContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return insertIfAbsent(ctx, db.driver, db, query, args...)
}
func (tx *Tx) InsertIfAbsentContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return insertIfAbsent(ctx, tx.driver, tx, query, args...)
}
