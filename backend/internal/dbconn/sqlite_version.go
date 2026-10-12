package dbconn

import (
	"context"
	"errors"
	"fmt"
)

func (tx *Tx) CheckSQLiteVersion(ctx context.Context, supported int) error {
	if tx.driver != SQLite {
		return errors.New("SQLite 版本检查不能用于服务器数据库")
	}
	var version int
	if err := tx.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version < 0 || version > supported {
		return errors.New("SQLite 数据库版本不兼容，禁止降级启动")
	}
	return nil
}

func (tx *Tx) SetSQLiteVersion(ctx context.Context, version int) error {
	if tx.driver != SQLite || version < 1 {
		return errors.New("SQLite 版本设置无效")
	}
	_, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", version))
	return err
}
