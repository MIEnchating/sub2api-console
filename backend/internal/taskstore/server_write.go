package taskstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

func (s *Store) beginTaskTx(ctx context.Context, id string) (*dbconn.Tx, error) {
	return s.beginTasksTx(ctx, []string{id})
}

func (s *Store) beginTasksTx(ctx context.Context, ids []string) (*dbconn.Tx, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	locks := make([]string, 0, len(ids))
	for _, id := range ids {
		digest := sha256.Sum256([]byte(id))
		locks = append(locks, hex.EncodeToString(digest[:]))
	}
	slices.Sort(locks)
	query := `INSERT INTO task_write_locks(id) VALUES(?) ON CONFLICT(id) DO UPDATE SET id=excluded.id`
	if s.db.Driver() == dbconn.MySQL {
		query = `INSERT INTO task_write_locks(id) VALUES(?) ON DUPLICATE KEY UPDATE id=VALUES(id)`
	}
	for _, lock := range slices.Compact(locks) {
		if _, err := tx.ExecContext(ctx, query, lock); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	return tx, nil
}

func (s *Store) reserveOperation(ctx context.Context, tx *dbconn.Tx, operation, id string) error {
	if operation == "" {
		return nil
	}
	query := `INSERT INTO active_task_operations(operation,task_id) VALUES(?,?) ON CONFLICT(operation) DO UPDATE SET operation=excluded.operation`
	if s.db.Driver() == dbconn.MySQL {
		query = `INSERT INTO active_task_operations(operation,task_id) VALUES(?,?) ON DUPLICATE KEY UPDATE operation=VALUES(operation)`
	}
	if _, err := tx.ExecContext(ctx, query, operation, id); err != nil {
		return err
	}
	var owner string
	if err := tx.QueryRowContext(ctx, `SELECT task_id FROM active_task_operations WHERE operation=? FOR UPDATE`, operation).Scan(&owner); err != nil {
		return err
	}
	if owner != id {
		return ErrOperationActive
	}
	return nil
}

func (s *Store) saveServerTask(ctx context.Context, task Task, encoded []byte) error {
	tx, err := s.beginTaskTx(ctx, task.ID)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var deleted bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM deleted_tasks WHERE id=?)`, task.ID).Scan(&deleted); err != nil {
		return err
	}
	if deleted {
		return ErrTaskDeleted
	}
	var status string
	var skill, operation, created string
	err = tx.QueryRowContext(ctx, `SELECT status,skill,operation,created_at FROM tasks WHERE id=? FOR UPDATE`, task.ID).Scan(&status, &skill, &operation, &created)
	exists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if exists && !activeTaskStatus(status) && status != task.Status {
		return ErrTaskTerminal
	}
	if exists {
		task.Skill, task.Operation, task.CreatedAt = skill, operation, created
	}
	if activeTaskStatus(task.Status) {
		if err := s.reserveOperation(ctx, tx, activeOperationKey(task.Operation), task.ID); err != nil {
			return err
		}
	}
	if exists {
		_, err = tx.ExecContext(ctx, `UPDATE tasks SET status=?,progress=?,message=?,result_json=?,updated_at=? WHERE id=?`, task.Status, task.Progress, task.Message, string(encoded), task.UpdatedAt, task.ID)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO tasks(id,skill,operation,status,progress,message,result_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?)`, task.ID, task.Skill, task.Operation, task.Status, task.Progress, task.Message, string(encoded), task.CreatedAt, task.UpdatedAt)
	}
	if err != nil {
		return err
	}
	if err := s.writeHistoryProjection(ctx, tx, task, encoded); err != nil {
		return err
	}
	if task.Recovery != nil && activeTaskStatus(task.Status) {
		raw, err := json.Marshal(task.Recovery)
		if err != nil {
			return err
		}
		if len(raw) > maximumTaskResultBytes {
			return errors.New("任务恢复参数过大")
		}
		query := `INSERT INTO task_recovery(task_id,payload) VALUES(?,?) ON CONFLICT(task_id) DO UPDATE SET payload=excluded.payload`
		if s.db.Driver() == dbconn.MySQL {
			query = `INSERT INTO task_recovery(task_id,payload) VALUES(?,?) ON DUPLICATE KEY UPDATE payload=VALUES(payload)`
		}
		if _, err := tx.ExecContext(ctx, query, task.ID, string(raw)); err != nil {
			return err
		}
	}
	if !activeTaskStatus(task.Status) {
		for _, table := range []string{"task_recovery", "active_task_operations"} {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE task_id=?", task.ID); err != nil {
				return err
			}
		}
	}
	if task.Operation == "automatic-inspection" && !activeTaskStatus(task.Status) {
		if _, err := s.compactServerHistory(ctx, tx, 100, 1); err != nil {
			return err
		}
	}

	return tx.Commit()
}
