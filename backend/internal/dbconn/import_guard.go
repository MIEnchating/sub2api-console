package dbconn

import (
	"context"
	"errors"
)

type importContextKey struct{}

func (db *Database) RequireCompletedImport(ctx context.Context) error {
	var state string
	var version int
	if err := db.QueryRowContext(ctx, `SELECT state,format_version FROM console_data_import WHERE id=1`).Scan(&state, &version); err != nil || state != "complete" || version != 1 {
		return errors.New("检测到旧 SQLite 文件，但目标库未完成显式导入；请先运行 db-migrate，或显式选择 sqlite 继续使用旧库")
	}
	return nil
}

// ImportContext is reserved for schema initialization by the offline importer.
// Service startup must use an ordinary context so incomplete imports are refused.
func ImportContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, importContextKey{}, true)
}

func (db *Database) CheckImport(ctx context.Context) error {
	if db.driver == SQLite || ctx.Value(importContextKey{}) == true {
		return nil
	}
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=current_schema() AND table_name='console_data_import')`
	if db.driver == MySQL {
		query = `SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='console_data_import')`
	}
	if err := db.QueryRowContext(ctx, query).Scan(&exists); err != nil {
		return errors.New("无法检查数据库导入状态")
	}
	if !exists {
		return nil
	}
	var state string
	var version int
	if err := db.QueryRowContext(ctx, `SELECT state,format_version FROM console_data_import WHERE id=1`).Scan(&state, &version); err != nil || state != "complete" || version != 1 {
		return errors.New("数据库导入未完成或版本不兼容；请检查迁移报告，不能启动业务")
	}
	return nil
}

// BeginImport refuses every pre-existing target table, including a previous
// failed import. It never clears or adopts a populated database.
func (db *Database) BeginImport(ctx context.Context) error {
	if db.driver == SQLite {
		return errors.New("导入目标必须为 PostgreSQL 或 MySQL")
	}
	query := `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema()`
	if db.driver == MySQL {
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()`
	}
	var count int
	if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return errors.New("无法检查导入目标")
	}
	if count != 0 {
		return errors.New("导入目标不是空数据库；请使用新的专用空库")
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE console_data_import(id INTEGER PRIMARY KEY,state VARCHAR(16) NOT NULL,format_version INTEGER NOT NULL)`); err != nil {
		return errors.New("无法建立数据库导入标记")
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO console_data_import(id,state,format_version) VALUES(1,'pending',1)`); err != nil {
		return errors.New("无法保存数据库导入标记")
	}
	return nil
}
