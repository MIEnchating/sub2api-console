package taskstore

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskcontext"
)

// Recovery contains only versioned, non-secret execution inputs. It is never
// serialized by the public Task API. Domain handlers must revalidate inputs.
type Recovery struct {
	Version int             `json:"version"`
	Input   json.RawMessage `json:"input"`
}

func WithRecovery(task *Task, input any) error {
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	task.Recovery = &Recovery{Version: 1, Input: raw}
	return nil
}

func RecoveryInput(task Task, into any) error {
	if task.Recovery == nil || task.Recovery.Version != 1 {
		return errors.New("不支持的任务恢复版本，请重新提交")
	}
	return json.Unmarshal(task.Recovery.Input, into)
}

// PendingRecovery is called once after this process owns its listening socket,
// before schedulers start. RecoverInterrupted leaves these records intact.
func (s *Store) PendingRecovery(ctx context.Context) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.task_id,r.payload FROM task_recovery r JOIN tasks t ON t.id=r.task_id WHERE t.status IN ('queued','running') ORDER BY t.created_at,t.id`)
	if err != nil {
		return nil, err
	}
	type record struct{ id, payload string }
	records := []record{}
	for rows.Next() {
		var r record
		if err := rows.Scan(&r.id, &r.payload); err != nil {
			rows.Close()
			return nil, err
		}
		records = append(records, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	tasks := make([]Task, 0, len(records))
	for _, record := range records {
		task, err := s.Get(ctx, record.id)
		if err != nil {
			return nil, err
		}
		var recovery Recovery
		if err := json.Unmarshal([]byte(record.payload), &recovery); err != nil {
			return nil, err
		}
		task.Recovery = &recovery
		task.Status = "queued"
		task.Message = "任务中断，等待恢复执行"
		if task.Result == nil {
			task.Result = map[string]any{}
		}
		task.Result["interrupted"] = true
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := s.Save(ctx, task); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

// CancelRecovery persists intent before cancelling the worker, so a crash
// between the HTTP response and worker finalization cannot restart manual work.
func (s *Store) CancelRecovery(ctx context.Context, id string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE tasks SET status='cancelled',message='任务已手动取消',updated_at=?,result_json=json_set(result_json,'$.cancelled',json('true')) WHERE id=? AND status IN ('queued','running') AND EXISTS(SELECT 1 FROM task_recovery WHERE task_id=tasks.id)`, time.Now().UTC().Format(storageTimeLayout), id)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if count == 0 {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM task_recovery WHERE task_id=?`, id); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM active_task_operations WHERE task_id=?`, id); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// Interrupted is true only for process shutdown, not manual cancellation or a
// request timeout. In-flight interrupted generation is not a completed result.
func Interrupted(ctx context.Context) bool {
	return errors.Is(context.Cause(ctx), taskcontext.ErrInterrupted)
}
