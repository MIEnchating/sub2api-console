package configstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrWorkbenchOAuthCheckpoint = errors.New("授权检查点已变化、到期或不能恢复，请刷新后重新确认")

type WorkbenchOAuthCheckpoint struct {
	ID                string
	OwnerHash         string
	TargetFingerprint string
	TargetURL         string
	Scope             string
	SourceTaskID      string
	TaskID            string
	Status            string
	Stage             string
	Revision          int64
	CreatedAt         string
	ExpiresAt         string
	WorkerID          string
	WorkerLease       string
	Automatic         bool
	WorkerRevision    int64
	ParentID          string
	Payload           json.RawMessage `json:"-"`
}

const oauthCheckpointColumns = `id,owner_hash,target_fingerprint,target_url,scope,source_task_id,task_id,status,stage,revision,created_at,expires_at,worker_id,worker_lease,automatic,worker_revision,parent_id`

func scanOAuthCheckpoint(row interface{ Scan(...any) error }, private bool) (WorkbenchOAuthCheckpoint, error) {
	var value WorkbenchOAuthCheckpoint
	var payload []byte
	fields := []any{&value.ID, &value.OwnerHash, &value.TargetFingerprint, &value.TargetURL, &value.Scope, &value.SourceTaskID, &value.TaskID, &value.Status, &value.Stage, &value.Revision, &value.CreatedAt, &value.ExpiresAt, &value.WorkerID, &value.WorkerLease, &value.Automatic, &value.WorkerRevision, &value.ParentID}
	if private {
		fields = append(fields, &payload)
	}
	err := row.Scan(fields...)
	value.Payload = payload
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrWorkbenchOAuthCheckpoint
	}
	return value, err
}

func (s *Store) WorkbenchOAuthCheckpoints(ctx context.Context, owner, target string) ([]WorkbenchOAuthCheckpoint, error) {
	if err := s.PurgeExpiredWorkbenchOAuthCheckpoints(ctx, time.Now()); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+oauthCheckpointColumns+` FROM workbench_oauth_checkpoints WHERE owner_hash=? AND target_fingerprint=? ORDER BY created_at DESC,id`, owner, target)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []WorkbenchOAuthCheckpoint{}
	for rows.Next() {
		value, err := scanOAuthCheckpoint(rows, false)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) WorkbenchOAuthCheckpoint(ctx context.Context, owner, target, id string) (WorkbenchOAuthCheckpoint, error) {
	return scanOAuthCheckpoint(s.db.QueryRowContext(ctx, `SELECT `+oauthCheckpointColumns+`,payload FROM workbench_oauth_checkpoints WHERE id=? AND owner_hash=? AND target_fingerprint=? AND expires_at>?`, id, owner, target, time.Now().UTC().Format(workbenchExecutionTimeLayout)), true)
}

func (s *Store) SaveWorkbenchOAuthCheckpoint(ctx context.Context, value WorkbenchOAuthCheckpoint) (WorkbenchOAuthCheckpoint, error) {
	if value.Scope == "" {
		value.Scope = "managed"
	}
	if value.Scope != "managed" && value.Scope != "local-export" || value.Scope == "managed" && value.TargetURL == "" || value.Scope == "local-export" && value.TargetURL != "" {
		return WorkbenchOAuthCheckpoint{}, ErrWorkbenchOAuthCheckpoint
	}
	if value.WorkerRevision < 0 || value.WorkerRevision >= 1<<53-1 || value.Status == "watching" && !value.Automatic || value.ParentID != "" && !workbenchIdentifier.MatchString(value.ParentID) {
		return WorkbenchOAuthCheckpoint{}, ErrWorkbenchOAuthCheckpoint
	}
	expires, err := time.Parse(time.RFC3339Nano, value.ExpiresAt)
	created, createdErr := time.Parse(time.RFC3339Nano, value.CreatedAt)
	if !workbenchIdentifier.MatchString(value.ID) || !workbenchIdentifier.MatchString(value.SourceTaskID) || len(value.OwnerHash) != 64 || strings.Trim(value.OwnerHash, "0123456789abcdef") != "" || len(value.TargetFingerprint) != 64 || strings.Trim(value.TargetFingerprint, "0123456789abcdef") != "" || value.Revision < 0 || value.Revision >= 1<<53-1 || err != nil || createdErr != nil || !expires.After(time.Now()) || !expires.After(created) || expires.Sub(created) > 15*time.Minute || len(value.Payload) > 32<<10 {
		return WorkbenchOAuthCheckpoint{}, ErrWorkbenchOAuthCheckpoint
	}
	if len(value.Payload) > 0 && !json.Valid(value.Payload) {
		return WorkbenchOAuthCheckpoint{}, ErrWorkbenchOAuthCheckpoint
	}
	if (value.Status == "watching" || value.Status == "saving" || value.Status == "ready" || value.Status == "restoring") && len(value.Payload) == 0 {
		return WorkbenchOAuthCheckpoint{}, ErrWorkbenchOAuthCheckpoint
	}
	if (value.Automatic || value.Status == "ready" || value.Status == "restoring" || value.Status == "restored") && (len(value.WorkerID) != 48 || strings.Trim(value.WorkerID, "0123456789abcdef") != "" || len(value.WorkerLease) < 32 || len(value.WorkerLease) > 128 || strings.Trim(value.WorkerLease, "0123456789abcdef") != "") {
		return WorkbenchOAuthCheckpoint{}, ErrWorkbenchOAuthCheckpoint
	}
	value.CreatedAt, value.ExpiresAt = created.UTC().Format(workbenchExecutionTimeLayout), expires.UTC().Format(workbenchExecutionTimeLayout)
	previous := value.Revision
	value.Revision++
	var result sql.Result
	if previous == 0 {
		if value.Status != "saving" && value.Status != "watching" {
			return WorkbenchOAuthCheckpoint{}, ErrWorkbenchOAuthCheckpoint
		}
		result, err = s.db.ExecContext(ctx, `INSERT INTO workbench_oauth_checkpoints(`+oauthCheckpointColumns+`,payload)
 SELECT ?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,? WHERE (SELECT count(*) FROM workbench_oauth_checkpoints WHERE owner_hash=? AND status IN ('watching','saving','ready','restoring') AND expires_at>?)<20
 ON CONFLICT(id) DO NOTHING`, value.ID, value.OwnerHash, value.TargetFingerprint, value.TargetURL, value.Scope, value.SourceTaskID, value.TaskID, value.Status, value.Stage, value.Revision, value.CreatedAt, value.ExpiresAt, value.WorkerID, value.WorkerLease, value.Automatic, value.WorkerRevision, value.ParentID, []byte(value.Payload), value.OwnerHash, time.Now().UTC().Format(workbenchExecutionTimeLayout))
	} else {
		result, err = s.db.ExecContext(ctx, `UPDATE workbench_oauth_checkpoints SET task_id=?,status=?,stage=?,revision=?,worker_id=?,worker_lease=?,worker_revision=?,payload=?
 WHERE id=? AND owner_hash=? AND target_fingerprint=? AND target_url=? AND scope=? AND source_task_id=? AND created_at=? AND expires_at=? AND revision=? AND automatic=? AND parent_id=?
 AND ((status='watching' AND ? IN ('saving','restoring','failed','deleting')) OR (status='saving' AND ? IN ('ready','failed','deleting')) OR (status='ready' AND ? IN ('restoring','deleting')) OR (status='restoring' AND ? IN ('restored','failed','deleting')) OR (status IN ('restored','failed') AND ?='deleting'))`, value.TaskID, value.Status, value.Stage, value.Revision, value.WorkerID, value.WorkerLease, value.WorkerRevision, []byte(value.Payload), value.ID, value.OwnerHash, value.TargetFingerprint, value.TargetURL, value.Scope, value.SourceTaskID, value.CreatedAt, value.ExpiresAt, previous, value.Automatic, value.ParentID, value.Status, value.Status, value.Status, value.Status, value.Status)
	}
	if err != nil {
		return WorkbenchOAuthCheckpoint{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return WorkbenchOAuthCheckpoint{}, err
	}
	if count != 1 {
		return WorkbenchOAuthCheckpoint{}, ErrWorkbenchOAuthCheckpoint
	}
	value.Payload = nil
	return value, nil
}

func (s *Store) DeleteWorkbenchOAuthCheckpoint(ctx context.Context, owner, target, id string, revision int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM workbench_oauth_checkpoints WHERE id=? AND owner_hash=? AND target_fingerprint=? AND revision=? AND status='deleting'`, id, owner, target, revision)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrWorkbenchOAuthCheckpoint
	}
	return nil
}

func (s *Store) PurgeExpiredWorkbenchOAuthCheckpoints(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM workbench_oauth_checkpoints WHERE expires_at<=?`, now.UTC().Format(workbenchExecutionTimeLayout))
	return err
}

func (s *Store) RecoverInterruptedWorkbenchOAuthCheckpoints(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE workbench_oauth_checkpoints SET status='failed',revision=revision+1,payload=NULL WHERE status IN ('saving','restoring')`)
	return err
}
