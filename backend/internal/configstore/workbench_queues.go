package configstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrWorkbenchQueue = errors.New("批量恢复资料已变化或过期，请刷新后重新确认")

type WorkbenchQueue struct {
	ID        string
	Owner     string
	Target    string
	Kind      string
	TaskID    string
	Status    string
	Revision  int64
	CreatedAt string
	ExpiresAt string
	Payload   json.RawMessage `json:"-"`
}

const workbenchQueueColumns = `id,owner,target,kind,task_id,status,revision,created_at,expires_at`

func (s *Store) WorkbenchQueues(ctx context.Context, owner, target string) ([]WorkbenchQueue, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+workbenchQueueColumns+` FROM workbench_queues WHERE owner=? AND target=? AND expires_at>? ORDER BY created_at DESC,id`, owner, target, time.Now().UTC().Format(workbenchExecutionTimeLayout))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []WorkbenchQueue{}
	for rows.Next() {
		value, err := scanWorkbenchQueue(rows, false)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func scanWorkbenchQueue(row interface{ Scan(...any) error }, private bool) (WorkbenchQueue, error) {
	var value WorkbenchQueue
	fields := []any{&value.ID, &value.Owner, &value.Target, &value.Kind, &value.TaskID, &value.Status, &value.Revision, &value.CreatedAt, &value.ExpiresAt}
	if private {
		fields = append(fields, &value.Payload)
	}
	if err := row.Scan(fields...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = ErrWorkbenchQueue
		}
		return WorkbenchQueue{}, err
	}
	return value, nil
}

func (s *Store) WorkbenchQueue(ctx context.Context, owner, target, id string) (WorkbenchQueue, error) {
	return scanWorkbenchQueue(s.db.QueryRowContext(ctx, `SELECT `+workbenchQueueColumns+`,payload FROM workbench_queues WHERE id=? AND owner=? AND target=? AND expires_at>?`, id, owner, target, time.Now().UTC().Format(workbenchExecutionTimeLayout)), true)
}

// SaveWorkbenchQueue keeps the original lifetime and increments a single CAS
// revision before any resumed runner can replace the previous execution owner.
func (s *Store) SaveWorkbenchQueue(ctx context.Context, value WorkbenchQueue) (WorkbenchQueue, error) {
	expires, expiryErr := time.Parse(time.RFC3339Nano, value.ExpiresAt)
	created, createdErr := time.Parse(time.RFC3339Nano, value.CreatedAt)
	if !workbenchIdentifier.MatchString(value.ID) || !workbenchIdentifier.MatchString(value.TaskID) || len(value.Owner) != 64 || len(value.Target) != 64 || strings.Trim(value.Owner+value.Target, "0123456789abcdef") != "" || value.Revision < 0 || value.Revision >= 1<<53-1 || expiryErr != nil || createdErr != nil || !expires.After(time.Now()) || !expires.After(created) || expires.Sub(created) > 2*time.Hour || len(value.Payload) == 0 || len(value.Payload) > 8<<20 || !json.Valid(value.Payload) {
		return WorkbenchQueue{}, ErrWorkbenchQueue
	}
	if value.Kind != "oauth-batch" && value.Kind != "mixed" {
		return WorkbenchQueue{}, ErrWorkbenchQueue
	}
	if value.Status != "running" && value.Status != "interrupted" && value.Status != "completed" {
		return WorkbenchQueue{}, ErrWorkbenchQueue
	}
	value.CreatedAt, value.ExpiresAt = created.UTC().Format(workbenchExecutionTimeLayout), expires.UTC().Format(workbenchExecutionTimeLayout)
	previous := value.Revision
	value.Revision++
	var result sql.Result
	var err error
	if previous == 0 {
		if value.Status != "running" {
			return WorkbenchQueue{}, ErrWorkbenchQueue
		}
		result, err = s.db.ExecContext(ctx, `INSERT INTO workbench_queues(`+workbenchQueueColumns+`,payload) SELECT ?,?,?,?,?,?,?,?,?,? WHERE (SELECT count(*) FROM workbench_queues WHERE owner=? AND expires_at>?)<20 ON CONFLICT(id) DO NOTHING`, value.ID, value.Owner, value.Target, value.Kind, value.TaskID, value.Status, value.Revision, value.CreatedAt, value.ExpiresAt, []byte(value.Payload), value.Owner, time.Now().UTC().Format(workbenchExecutionTimeLayout))
	} else {
		result, err = s.db.ExecContext(ctx, `UPDATE workbench_queues SET task_id=?,status=?,revision=?,payload=? WHERE id=? AND owner=? AND target=? AND kind=? AND revision=? AND created_at=? AND expires_at=? AND (status<>'running' OR task_id=?)`, value.TaskID, value.Status, value.Revision, []byte(value.Payload), value.ID, value.Owner, value.Target, value.Kind, previous, value.CreatedAt, value.ExpiresAt, value.TaskID)
	}
	if err != nil {
		return WorkbenchQueue{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return WorkbenchQueue{}, err
	}
	if count != 1 {
		return WorkbenchQueue{}, ErrWorkbenchQueue
	}
	value.Payload = nil
	return value, nil
}

func (s *Store) DeleteWorkbenchQueue(ctx context.Context, owner, target, id string, revision int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM workbench_queues WHERE id=? AND owner=? AND target=? AND revision=?`, id, owner, target, revision)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrWorkbenchQueue
	}
	return nil
}

func (s *Store) PurgeExpiredWorkbenchQueues(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM workbench_queues WHERE expires_at<=?`, now.UTC().Format(workbenchExecutionTimeLayout))
	return err
}

func (s *Store) RecoverInterruptedWorkbenchQueues(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE workbench_queues SET status='interrupted',revision=revision+1 WHERE status='running'`)
	return err
}
