package taskstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"time"
)

var ErrTaskStillActive = errors.New("任务尚未结束，请等待本批检测完成后再标记")

// UpdateTerminalResult serializes local result edits with deletion and other
// writes. edit must only change task.Result and must not perform network I/O.
func (s *Store) UpdateTerminalResult(ctx context.Context, id string, edit func(Task) error) (Task, error) {
	var tx *dbconn.Tx
	var err error
	if s.db.Driver() == dbconn.SQLite {
		tx, err = s.db.BeginTx(ctx, nil)
	} else {
		tx, err = s.beginTaskTx(ctx, id)
	}
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	query := `SELECT id,skill,operation,status,progress,message,result_json,created_at,updated_at FROM tasks WHERE id=?`
	if s.db.Driver() != dbconn.SQLite {
		query += ` FOR UPDATE`
	}
	task, err := scanTask(tx.QueryRowContext(ctx, query, id))
	if err != nil {
		return Task{}, err
	}
	if activeTaskStatus(task.Status) {
		return Task{}, ErrTaskStillActive
	}
	if err := edit(task); err != nil {
		return Task{}, err
	}
	raw, err := json.Marshal(task.Result)
	if err != nil {
		return Task{}, err
	}
	if len(raw) > maximumTaskResultBytes {
		return Task{}, fmt.Errorf("任务结果不能超过 %d MiB", maximumTaskResultBytes>>20)
	}
	task.UpdatedAt = time.Now().UTC().Format(storageTimeLayout)
	if _, err := tx.ExecContext(ctx, `UPDATE tasks SET result_json=?,updated_at=? WHERE id=?`, string(raw), task.UpdatedAt, id); err != nil {
		return Task{}, err
	}
	if s.db.Driver() != dbconn.SQLite {
		if err := s.writeHistoryProjection(ctx, tx, task, raw); err != nil {
			return Task{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Task{}, err
	}
	return task, nil
}
