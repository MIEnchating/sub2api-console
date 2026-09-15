package configstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

const workbenchExecutionClaimsPrefix = "account_workbench.execution_claims."

type workbenchExecutionClaims struct {
	SourceID          string                    `json:"source_id"`
	TargetFingerprint string                    `json:"target_fingerprint"`
	Claims            []workbenchExecutionClaim `json:"claims"`
}

type workbenchExecutionClaim struct {
	Index       int    `json:"index"`
	RetryTaskID string `json:"retry_task_id"`
	ExpiresAt   string `json:"expires_at"`
}

// Claims contain no credential, snapshot, account name or email. They remain
// verifiable until each successor expires, independently of its parent record.
func (s *Store) ValidateWorkbenchExecutionClaim(ctx context.Context, record WorkbenchExecution) error {
	current, err := s.WorkbenchExecution(ctx, record.ID)
	if err != nil {
		return err
	}
	if current.Revision != record.Revision || current.SourceID != record.SourceID || current.TargetFingerprint != record.TargetFingerprint {
		return ErrWorkbenchExecution
	}
	if current.SourceID == "" {
		return nil
	}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, workbenchExecutionClaimsPrefix+current.SourceID).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrWorkbenchExecution
		}
		return err
	}
	claims, err := parseExecutionClaims(raw, current.SourceID)
	if err != nil {
		return ErrWorkbenchExecution
	}
	return executionClaimsAuthorize(claims, current, s.workbenchExecutionNow())
}

func validateExecutionClaimTx(ctx context.Context, tx *sql.Tx, record WorkbenchExecution, now time.Time) error {
	if record.SourceID == "" {
		return nil
	}
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, workbenchExecutionClaimsPrefix+record.SourceID).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrWorkbenchExecution
		}
		return err
	}
	claims, err := parseExecutionClaims(raw, record.SourceID)
	if err != nil {
		return err
	}
	return executionClaimsAuthorize(claims, record, now)
}

func executionClaimsAuthorize(claims workbenchExecutionClaims, current WorkbenchExecution, now time.Time) error {
	if claims.TargetFingerprint != current.TargetFingerprint {
		return ErrWorkbenchExecution
	}
	indexClaims := make(map[int]workbenchExecutionClaim, len(claims.Claims))
	for _, claim := range claims.Claims {
		indexClaims[claim.Index] = claim
	}
	for _, item := range current.Items {
		claim := indexClaims[item.Index]
		expires, parseErr := time.Parse(workbenchExecutionTimeLayout, claim.ExpiresAt)
		if parseErr != nil || claim.RetryTaskID != current.ID || claim.ExpiresAt != current.ExpiresAt || !now.Before(expires) {
			return ErrWorkbenchExecution
		}
	}
	return nil
}

func saveExecutionClaims(ctx context.Context, tx *sql.Tx, previous, current WorkbenchExecution, now time.Time) error {
	previousClaims := make(map[int]string, len(previous.Items))
	for _, item := range previous.Items {
		previousClaims[item.Index] = item.RetryTaskID
	}
	items := make(map[int]WorkbenchExecutionItem, len(current.Items))
	for _, item := range current.Items {
		items[item.Index] = item
	}
	claims := workbenchExecutionClaims{SourceID: current.ID, TargetFingerprint: current.TargetFingerprint}
	children := make(map[string]WorkbenchExecution)
	for _, item := range current.Items {
		prior := previousClaims[item.Index]
		if prior != "" && prior != item.RetryTaskID {
			return ErrWorkbenchExecution
		}
		if item.RetryTaskID == "" {
			continue
		}
		child, found := children[item.RetryTaskID]
		if !found {
			var err error
			child, err = readExecutionTx(ctx, tx, item.RetryTaskID)
			if err != nil || !executionUnexpired(child, now) || child.SourceID != current.ID || child.TargetURL != current.TargetURL || child.TargetFingerprint != current.TargetFingerprint {
				return ErrWorkbenchExecution
			}
			for _, childItem := range child.Items {
				if items[childItem.Index].RetryTaskID != child.ID {
					return ErrWorkbenchExecution
				}
			}
			children[item.RetryTaskID] = child
		}
		matched := false
		for _, childItem := range child.Items {
			if childItem.Index == item.Index {
				matched = true
				break
			}
		}
		if !matched {
			return ErrWorkbenchExecution
		}
		claims.Claims = append(claims.Claims, workbenchExecutionClaim{Index: item.Index, RetryTaskID: child.ID, ExpiresAt: child.ExpiresAt})
	}
	if len(claims.Claims) == 0 {
		return nil
	}
	encoded, err := json.Marshal(claims)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, workbenchExecutionClaimsPrefix+current.ID, string(encoded))
	return err
}

func parseExecutionClaims(raw, sourceID string) (workbenchExecutionClaims, error) {
	var result workbenchExecutionClaims
	if json.Unmarshal([]byte(raw), &result) != nil || result.SourceID != sourceID || !workbenchIdentifier.MatchString(sourceID) || result.TargetFingerprint == "" || len(result.Claims) == 0 || len(result.Claims) > 500 {
		return workbenchExecutionClaims{}, ErrWorkbenchExecution
	}
	indexes := make(map[int]bool, len(result.Claims))
	for _, claim := range result.Claims {
		_, err := time.Parse(workbenchExecutionTimeLayout, claim.ExpiresAt)
		if err != nil || claim.Index < 0 || claim.Index >= 500 || indexes[claim.Index] || !workbenchIdentifier.MatchString(claim.RetryTaskID) || claim.RetryTaskID == sourceID {
			return workbenchExecutionClaims{}, ErrWorkbenchExecution
		}
		indexes[claim.Index] = true
	}
	return result, nil
}
