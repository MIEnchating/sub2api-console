package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

func (s *Store) MarkCleanupPaused(ctx context.Context, accountID, reason string) error {
	if !positiveNumericID(accountID) {
		return errors.New("账号必须使用有效的稳定 ID")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE accounts SET paused=1,paused_reason=?,routing_state='paused',updated_at=? WHERE id=?`, reason, now, accountID)
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return sql.ErrNoRows
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO paused_accounts(account_id,reason,enabled,updated_at) VALUES(?,?,1,?)
		ON CONFLICT(account_id) DO UPDATE SET reason=excluded.reason,enabled=1,updated_at=excluded.updated_at`, accountID, reason, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MarkCleanupDisabled(ctx context.Context, accountID string) error {
	if !positiveNumericID(accountID) {
		return errors.New("账号必须使用有效的稳定 ID")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT metadata_json FROM accounts WHERE id=?`, accountID).Scan(&raw); err != nil {
		return err
	}
	metadata, err := decodeJSONObject(raw)
	if err != nil {
		return errors.New("账号元数据损坏，无法记录自动停用")
	}
	metadata["status"] = "inactive"
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE accounts SET schedulable=0,routing_state='disabled',metadata_json=?,updated_at=? WHERE id=?`, string(encoded), now, accountID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteAccountProjection(ctx context.Context, accountID string, operation AccountOperation) error {
	if !positiveNumericID(accountID) {
		return errors.New("账号必须使用有效的稳定 ID")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts WHERE id=?`, accountID).Scan(&exists); err != nil {
		return err
	}
	if exists != 1 {
		return sql.ErrNoRows
	}
	for _, table := range []string{"account_groups", "health_samples", "routing_decisions", "account_health_evaluations", "paused_accounts", "routing_baselines", "cleanup_states"} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE account_id=?", accountID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM bindings WHERE local_account_id=?`, accountID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM accounts WHERE id=?`, accountID); err != nil {
		return err
	}
	if err := insertAccountOperation(ctx, tx, operation); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE local_groups SET account_count=(
		SELECT COUNT(*) FROM account_groups WHERE group_name=local_groups.name
	),updated_at=?`, now); err != nil {
		return err
	}
	return tx.Commit()
}
