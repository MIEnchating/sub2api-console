package configstore

import (
	"context"
	"database/sql"
	"errors"
)

func (s *Store) TaskLimits(ctx context.Context) (string, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='tasks.limits'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return raw, err
}

// SaveTaskLimits atomically prevents overwriting another editor's saved limits.
func (s *Store) SaveTaskLimits(ctx context.Context, previous, next string) error {
	var result sql.Result
	var err error
	if previous == "" {
		result, err = s.db.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES('tasks.limits',?) ON CONFLICT(key) DO NOTHING`, next)
	} else {
		result, err = s.db.ExecContext(ctx, `UPDATE settings SET value=? WHERE key='tasks.limits' AND value=?`, next, previous)
	}
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("任务并发设置已更新，请刷新后重试")
	}
	return nil
}
