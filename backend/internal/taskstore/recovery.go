package taskstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
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

// RecoveryRef keeps the waiting queue independent of result and checkpoint size.
// The complete current task is loaded only when a dispatcher attempts it.
type RecoveryRef struct {
	ID string `json:"id"`
}

var ErrInvalidRecovery = errors.New("任务恢复数据损坏")

// PendingRecovery is called once after this process owns its listening socket,
// before schedulers start. Preparation updates local state without decoding
// results or checkpoints. The dispatcher validates each complete task separately.
func (s *Store) PendingRecovery(ctx context.Context) ([]RecoveryRef, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.id FROM task_recovery r JOIN tasks t ON t.id=r.task_id WHERE t.status IN ('queued','running') ORDER BY t.created_at,t.id`)
	if err != nil {
		return nil, err
	}
	refs := []RecoveryRef{}
	for rows.Next() {
		var ref RecoveryRef
		if err := rows.Scan(&ref.ID); err != nil {
			rows.Close()
			return nil, err
		}
		refs = append(refs, ref)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	pending := make([]RecoveryRef, 0, len(refs))
	for _, ref := range refs {
		prepared, err := s.prepareRecovery(ctx, ref.ID)
		if err != nil {
			return nil, err
		}
		if prepared {
			pending = append(pending, ref)
		}
	}
	return pending, nil
}

func (s *Store) prepareRecovery(ctx context.Context, id string) (bool, error) {
	if s.db.Driver() != dbconn.SQLite {
		return s.prepareServerRecovery(ctx, id)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var task Task
	err = tx.QueryRowContext(ctx, `SELECT id,skill,operation,status,progress,message,created_at,updated_at
		FROM tasks WHERE id=? AND status IN ('queued','running')
		AND EXISTS(SELECT 1 FROM task_recovery WHERE task_id=tasks.id)
		AND NOT EXISTS(SELECT 1 FROM deleted_tasks WHERE id=tasks.id)`, id).Scan(
		&task.ID, &task.Skill, &task.Operation, &task.Status, &task.Progress, &task.Message, &task.CreatedAt, &task.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if key := activeOperationKey(task.Operation); key != "" {
		reservation, err := tx.ExecContext(ctx, `INSERT INTO active_task_operations(operation,task_id) VALUES(?,?)
			ON CONFLICT(operation) DO UPDATE SET task_id=excluded.task_id
			WHERE active_task_operations.task_id=excluded.task_id`, key, id)
		if err != nil {
			return false, err
		}
		count, err := reservation.RowsAffected()
		if err != nil {
			return false, err
		}
		if count == 0 {
			return false, fmt.Errorf("%w：%s", ErrOperationActive, task.Operation)
		}
	}
	if validateTaskColumns(task) != nil {
		// Keep corrupt fields intact so dispatch cannot mistake them for valid inputs.
		_, err = tx.ExecContext(ctx, `UPDATE tasks SET status='queued' WHERE id=?`, id)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE tasks SET status='queued',message='任务中断，等待恢复执行',updated_at=?,
			result_json=CASE WHEN json_valid(result_json) THEN
				CASE WHEN json_type(result_json)='object' THEN json_set(result_json,'$.interrupted',json('true')) ELSE result_json END
				ELSE result_json END WHERE id=?`, time.Now().UTC().Format(storageTimeLayout), id)
	}
	if err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// RecoveryTask reads the task and its checkpoint from the same database
// snapshot. Only queued tasks with a checkpoint are dispatchable.
func (s *Store) RecoveryTask(ctx context.Context, id string) (Task, error) {
	row := s.db.QueryRowContext(ctx, `SELECT t.id,t.skill,t.operation,t.status,t.progress,t.message,t.result_json,t.created_at,t.updated_at,r.payload
	 FROM tasks t JOIN task_recovery r ON r.task_id=t.id WHERE t.id=? AND t.status='queued'`, id)
	var payload string
	task, err := scanTask(recoveryTaskScanner{row: row, payload: &payload})
	if err != nil {
		return Task{}, err
	}
	if task.Result["storage_corrupt"] == true {
		return task, fmt.Errorf("%w：%s", ErrInvalidRecovery, task.Message)
	}
	var recovery Recovery
	if err := json.Unmarshal([]byte(payload), &recovery); err != nil {
		return task, fmt.Errorf("%w：恢复参数不是有效 JSON", ErrInvalidRecovery)
	}
	task.Recovery = &recovery
	return task, nil
}

type recoveryTaskScanner struct {
	row     scanner
	payload *string
}

func (r recoveryTaskScanner) Scan(dest ...any) error {
	return r.row.Scan(append(dest, r.payload)...)
}

// CancelRecovery persists intent before cancelling the worker, so a crash
// between the HTTP response and worker finalization cannot restart manual work.
func (s *Store) CancelRecovery(ctx context.Context, id string) (bool, error) {
	return s.cancelTask(ctx, id, true)
}

// CancelTask also persists cancellation for non-resumable orchestration tasks.
// Their children may have recovery checkpoints, so the parent's intent must
// survive a crash before its runner receives the cancellation signal.
func (s *Store) CancelTask(ctx context.Context, id string) (bool, error) {
	return s.cancelTask(ctx, id, false)
}

func (s *Store) cancelTask(ctx context.Context, id string, recoveryOnly bool) (bool, error) {
	if s.db.Driver() != dbconn.SQLite {
		return s.cancelServerTask(ctx, id, recoveryOnly)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	query := `UPDATE tasks SET status='cancelled',message='任务已手动取消',updated_at=?,result_json=json_set(result_json,'$.cancelled',json('true')) WHERE id=?`
	if recoveryOnly {
		query += ` AND status IN ('queued','running') AND EXISTS(SELECT 1 FROM task_recovery WHERE task_id=tasks.id)`
	} else {
		query += ` AND status IN ('queued','running','waiting_input')`
	}
	result, err := tx.ExecContext(ctx, query, time.Now().UTC().Format(storageTimeLayout), id)
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
