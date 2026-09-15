package configstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// WorkbenchExecution is private restart state. Its credential fields must never
// be embedded in task results or API responses.
type WorkbenchExecution struct {
	Maintenance       *WorkbenchExecutionMaintenance `json:"maintenance,omitempty"`
	ID                string                         `json:"id"`
	SourceID          string                         `json:"source_id,omitempty"`
	TargetURL         string                         `json:"target_url"`
	TargetFingerprint string                         `json:"target_fingerprint"`
	Revision          int64                          `json:"revision"`
	CreatedAt         string                         `json:"created_at"`
	ExpiresAt         string                         `json:"expires_at"`
	Items             []WorkbenchExecutionItem       `json:"items"`
}

type WorkbenchExecutionMaintenance struct {
	SourceBatchID    string `json:"source_batch_id,omitempty"`
	Revision         int64  `json:"revision"`
	Model            string `json:"model"`
	CheckAfterImport bool   `json:"check_after_import"`
	CooldownMinutes  int    `json:"cooldown_minutes"`
}

type WorkbenchExecutionItem struct {
	UploadStatus             string             `json:"upload_status,omitempty"`
	UploadMessage            string             `json:"upload_message,omitempty"`
	UploadNextRetryAt        string             `json:"upload_next_retry_at,omitempty"`
	CredentialWriteOutcome   string             `json:"credential_write_outcome,omitempty"`
	LoginProfileID           string             `json:"login_profile_id,omitempty"`
	LoginProfileRevision     int64              `json:"login_profile_revision,omitempty"`
	LoginUserID              string             `json:"login_user_id,omitempty"`
	LoginWorkspaceID         string             `json:"login_workspace_id,omitempty"`
	LoginMaintenanceRevision int64              `json:"login_maintenance_revision,omitempty"`
	LoginMaintenanceOwner    string             `json:"login_maintenance_owner,omitempty"`
	Index                    int                `json:"index"`
	Kind                     string             `json:"kind"`
	Name                     string             `json:"name"`
	Email                    string             `json:"email"`
	PlanType                 string             `json:"plan_type"`
	Credentials              json.RawMessage    `json:"credentials,omitempty"`
	Identity                 string             `json:"identity"`
	Template                 *WorkbenchTemplate `json:"template,omitempty"`
	AccountID                string             `json:"account_id,omitempty"`
	OriginalAccountID        string             `json:"original_account_id,omitempty"`
	Marker                   string             `json:"marker,omitempty"`
	Phase                    string             `json:"phase"`
	Status                   string             `json:"status"`
	Snapshot                 json.RawMessage    `json:"snapshot,omitempty"`
	PendingSnapshot          json.RawMessage    `json:"pending_snapshot,omitempty"`
	RetryTaskID              string             `json:"retry_task_id,omitempty"`
}

var ErrWorkbenchExecution = errors.New("账号执行记录已变化或不可用，请刷新任务后重新预览")

const workbenchExecutionPrefix = "account_workbench.execution."
const workbenchExecutionRetention = 24 * time.Hour
const workbenchExecutionTimeLayout = "2006-01-02T15:04:05.000000000Z"

// UseWorkbenchExecutionClock replaces the time boundary for isolated tests.
func (s *Store) UseWorkbenchExecutionClock(now func() time.Time) {
	s.workbenchExecutionMu.Lock()
	defer s.workbenchExecutionMu.Unlock()
	s.workbenchExecutionClock = now
}

func (s *Store) workbenchExecutionNow() time.Time {
	s.workbenchExecutionMu.RLock()
	defer s.workbenchExecutionMu.RUnlock()
	if s.workbenchExecutionClock != nil {
		return s.workbenchExecutionClock().UTC()
	}
	return time.Now().UTC()
}

func (s *Store) WorkbenchExecution(ctx context.Context, id string) (WorkbenchExecution, error) {
	var result WorkbenchExecution
	if !workbenchIdentifier.MatchString(id) {
		return result, ErrWorkbenchExecution
	}
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, workbenchExecutionPrefix+id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return result, ErrWorkbenchExecution
	}
	if err != nil {
		return result, err
	}
	if json.Unmarshal([]byte(raw), &result) != nil || result.ID != id || result.Revision < 1 || !validExecutionLifetime(result) {
		if err := s.deleteExpiredExecution(ctx, id, raw, ""); err != nil {
			return WorkbenchExecution{}, err
		}
		return WorkbenchExecution{}, ErrWorkbenchExecution
	}
	if !executionUnexpired(result, s.workbenchExecutionNow()) {
		if err := s.deleteExpiredExecution(ctx, id, "", result.ExpiresAt); err != nil {
			return WorkbenchExecution{}, err
		}
		return WorkbenchExecution{}, ErrWorkbenchExecution
	}
	return result, nil
}

func (s *Store) SaveWorkbenchExecution(ctx context.Context, input WorkbenchExecution) error {
	if !workbenchIdentifier.MatchString(input.ID) || input.TargetURL == "" || input.TargetFingerprint == "" || input.Revision < 0 || len(input.Items) == 0 || len(input.Items) > 500 || (input.SourceID != "" && (!workbenchIdentifier.MatchString(input.SourceID) || input.SourceID == input.ID)) {
		return ErrWorkbenchExecution
	}
	indexes := make(map[int]bool, len(input.Items))
	for _, item := range input.Items {
		if item.Index < 0 || item.Index >= 500 || indexes[item.Index] || (item.RetryTaskID != "" && !workbenchIdentifier.MatchString(item.RetryTaskID)) {
			return ErrWorkbenchExecution
		}
		indexes[item.Index] = true
	}
	s.workbenchExecutionWriteMu.Lock()
	defer s.workbenchExecutionWriteMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := s.workbenchExecutionNow()
	key := workbenchExecutionPrefix + input.ID
	previous, readErr := readExecutionTx(ctx, tx, input.ID)
	if readErr != nil && !errors.Is(readErr, sql.ErrNoRows) {
		return readErr
	}
	if errors.Is(readErr, sql.ErrNoRows) {
		if input.Revision != 0 {
			return ErrWorkbenchExecution
		}
		if input.SourceID != "" {
			source, err := readExecutionTx(ctx, tx, input.SourceID)
			if err != nil || !executionUnexpired(source, now) || source.TargetURL != input.TargetURL || source.TargetFingerprint != input.TargetFingerprint {
				if err == nil && !executionUnexpired(source, now) {
					if _, deleteErr := tx.ExecContext(ctx, `DELETE FROM settings WHERE key=?`, workbenchExecutionPrefix+source.ID); deleteErr != nil {
						return deleteErr
					}
					if commitErr := tx.Commit(); commitErr != nil {
						return commitErr
					}
				}
				return ErrWorkbenchExecution
			}
			if err := validateExecutionClaimTx(ctx, tx, source, now); err != nil {
				return err
			}
		}
		input.CreatedAt = now.Format(workbenchExecutionTimeLayout)
		input.ExpiresAt = now.Add(workbenchExecutionRetention).Format(workbenchExecutionTimeLayout)
	} else {
		if !executionUnexpired(previous, now) {
			if _, err := tx.ExecContext(ctx, `DELETE FROM settings WHERE key=?`, key); err != nil {
				return err
			}
			if err := tx.Commit(); err != nil {
				return err
			}
			return ErrWorkbenchExecution
		}
		if previous.Revision != input.Revision || previous.SourceID != input.SourceID || previous.TargetURL != input.TargetURL || previous.TargetFingerprint != input.TargetFingerprint || !sameExecutionIndexes(previous.Items, input.Items) {
			return ErrWorkbenchExecution
		}
		input.CreatedAt, input.ExpiresAt = previous.CreatedAt, previous.ExpiresAt
	}
	if err := saveExecutionClaims(ctx, tx, previous, input, now); err != nil {
		return err
	}
	input.Revision++
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, string(raw)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) deleteExpiredExecution(ctx context.Context, id, raw, expiresAt string) error {
	s.workbenchExecutionWriteMu.Lock()
	defer s.workbenchExecutionWriteMu.Unlock()
	if expiresAt == "" {
		_, err := s.db.ExecContext(ctx, `DELETE FROM settings WHERE key=? AND value=?`, workbenchExecutionPrefix+id, raw)
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM settings WHERE key=? AND CASE WHEN json_valid(value) THEN json_extract(value,'$.expires_at') END=?`, workbenchExecutionPrefix+id, expiresAt)
	return err
}

func readExecutionTx(ctx context.Context, tx *sql.Tx, id string) (WorkbenchExecution, error) {
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, workbenchExecutionPrefix+id).Scan(&raw); err != nil {
		return WorkbenchExecution{}, err
	}
	var record WorkbenchExecution
	if json.Unmarshal([]byte(raw), &record) != nil || record.ID != id || record.Revision < 1 {
		return WorkbenchExecution{}, ErrWorkbenchExecution
	}
	return record, nil
}

func validExecutionLifetime(record WorkbenchExecution) bool {
	created, err := time.Parse(workbenchExecutionTimeLayout, record.CreatedAt)
	if err != nil {
		return false
	}
	expires, err := time.Parse(workbenchExecutionTimeLayout, record.ExpiresAt)
	return err == nil && expires.Sub(created) == workbenchExecutionRetention
}

func executionUnexpired(record WorkbenchExecution, now time.Time) bool {
	if !validExecutionLifetime(record) {
		return false
	}
	expires, _ := time.Parse(workbenchExecutionTimeLayout, record.ExpiresAt)
	return now.Before(expires)
}

func sameExecutionIndexes(previous, current []WorkbenchExecutionItem) bool {
	if len(previous) != len(current) {
		return false
	}
	for i := range previous {
		if previous[i].Index != current[i].Index {
			return false
		}
	}
	return true
}
