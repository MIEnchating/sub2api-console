package taskstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"time"
)

func markTaskResult(raw, field string) (string, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		if json.Valid([]byte(raw)) {
			return raw, nil
		}
		return "", err
	}
	if fields == nil {
		return raw, nil
	}
	fields[field] = json.RawMessage("true")
	encoded, err := json.Marshal(fields)
	return string(encoded), err
}

func (s *Store) recoverServerInterrupted(ctx context.Context, cutoff time.Time) (int64, error) {
	var total int64
	cursor := ""
	for {
		query := `SELECT id FROM tasks WHERE id>? AND status IN ('queued','running','waiting_input') AND NOT EXISTS(SELECT 1 FROM task_recovery WHERE task_id=tasks.id)`
		args := []any{cursor}
		if !cutoff.IsZero() {
			query += ` AND updated_at<?`
			args = append(args, cutoff.UTC().Format(storageTimeLayout))
		}
		query += ` ORDER BY id LIMIT 200`
		rows, err := s.db.QueryContext(ctx, query, args...)
		if err != nil {
			return total, err
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return total, err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return total, err
		}
		for _, id := range ids {
			changed, err := s.failServerInterruptedTask(ctx, id, cutoff)
			if err != nil {
				return total, err
			}
			if changed {
				total++
			}
		}
		if len(ids) < 200 {
			return total, nil
		}
		cursor = ids[len(ids)-1]
	}
}

func (s *Store) failServerInterruptedTask(ctx context.Context, id string, cutoff time.Time) (bool, error) {
	tx, err := s.beginTaskTx(ctx, id)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	query := `SELECT skill,operation,result_json FROM tasks WHERE id=? AND status IN ('queued','running','waiting_input') AND NOT EXISTS(SELECT 1 FROM task_recovery WHERE task_id=tasks.id)`
	args := []any{id}
	if !cutoff.IsZero() {
		query += ` AND updated_at<?`
		args = append(args, cutoff.UTC().Format(storageTimeLayout))
	}
	var skill, operation, raw string
	err = tx.QueryRowContext(ctx, query+` FOR UPDATE`, args...).Scan(&skill, &operation, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	body := historyObjectFrom([]byte(raw))
	if skill == "account-workbench" && operation == "account-workbench-oauth-batch" && json.Valid([]byte(raw)) {
		items := []json.RawMessage{}
		for _, item := range historyValues(body["items"]) {
			fields := historyObjectFrom(item)
			status, _ := historyText(fields["status"])
			if status == "queued" || status == "running" {
				fields["status"] = json.RawMessage(`"cancelled"`)
				fields["message"] = json.RawMessage(`"原授权已中断，请重新授权"`)
				item, err = json.Marshal(fields)
				if err != nil {
					return false, err
				}
			}
			items = append(items, item)
		}
		encoded, err := json.Marshal(map[string]any{"error": "进程重启导致任务中断", "interrupted": true, "phase": "failed", "items": items})
		if err != nil {
			return false, err
		}
		raw = string(encoded)
	} else {
		if !json.Valid([]byte(raw)) {
			body = historyObject{}
		}
		if body != nil {
			body["error"] = json.RawMessage(`"进程重启导致任务中断，缺少安全恢复参数，请重新提交"`)
			body["interrupted"] = json.RawMessage("true")
			encoded, err := json.Marshal(body)
			if err != nil {
				return false, err
			}
			raw = string(encoded)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE tasks SET status='failed',progress=100,message='进程重启导致任务中断',result_json=?,updated_at=? WHERE id=?`, raw, time.Now().UTC().Format(storageTimeLayout), id); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM active_task_operations WHERE task_id=?`, id); err != nil {
		return false, err
	}
	if err := s.refreshTaskHistoryProjection(ctx, tx, id); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (s *Store) prepareServerRecovery(ctx context.Context, id string) (bool, error) {
	tx, err := s.beginTaskTx(ctx, id)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var task Task
	var raw string
	err = tx.QueryRowContext(ctx, `SELECT id,skill,operation,status,progress,message,created_at,updated_at,result_json FROM tasks WHERE id=? AND status IN ('queued','running') AND EXISTS(SELECT 1 FROM task_recovery WHERE task_id=tasks.id) AND NOT EXISTS(SELECT 1 FROM deleted_tasks WHERE id=tasks.id) FOR UPDATE`, id).Scan(&task.ID, &task.Skill, &task.Operation, &task.Status, &task.Progress, &task.Message, &task.CreatedAt, &task.UpdatedAt, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := s.reserveOperation(ctx, tx, activeOperationKey(task.Operation), id); err != nil {
		return false, err
	}
	if validateTaskColumns(task) != nil {
		_, err = tx.ExecContext(ctx, `UPDATE tasks SET status='queued' WHERE id=?`, id)
	} else {
		marked, markErr := markTaskResult(raw, "interrupted")
		if markErr == nil {
			raw = marked
		}
		_, err = tx.ExecContext(ctx, `UPDATE tasks SET status='queued',message='任务中断，等待恢复执行',updated_at=?,result_json=? WHERE id=?`, time.Now().UTC().Format(storageTimeLayout), raw, id)
	}
	if err != nil {
		return false, err
	}
	if err := s.refreshTaskHistoryProjection(ctx, tx, id); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (s *Store) cancelServerTask(ctx context.Context, id string, recoveryOnly bool) (bool, error) {
	tx, err := s.beginTaskTx(ctx, id)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	query := `SELECT result_json FROM tasks WHERE id=? AND status IN ('queued','running','waiting_input')`
	if recoveryOnly {
		query += ` AND status IN ('queued','running') AND EXISTS(SELECT 1 FROM task_recovery WHERE task_id=tasks.id)`
	}
	var raw string
	err = tx.QueryRowContext(ctx, query+` FOR UPDATE`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	raw, err = markTaskResult(raw, "cancelled")
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE tasks SET status='cancelled',message='任务已手动取消',updated_at=?,result_json=? WHERE id=?`, time.Now().UTC().Format(storageTimeLayout), raw, id); err != nil {
		return false, err
	}
	for _, table := range []string{"task_recovery", "active_task_operations"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE task_id=?", id); err != nil {
			return false, err
		}
	}
	if err := s.refreshTaskHistoryProjection(ctx, tx, id); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func (s *Store) refreshTaskHistoryProjection(ctx context.Context, tx *dbconn.Tx, id string) error {
	var task Task
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT id,skill,operation,status,result_json FROM tasks WHERE id=?`, id).Scan(&task.ID, &task.Skill, &task.Operation, &task.Status, &raw); err != nil {
		return err
	}
	return s.writeHistoryProjection(ctx, tx, task, []byte(raw))
}
