package configstore

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type AuthRecoveryPreference struct {
	Host           string
	AuthMode       string
	RecoveryMethod string
	VaultEntryID   *string
	SucceededAt    string
}

func (s *Store) AuthRecoveryPreference(ctx context.Context, host string) (*AuthRecoveryPreference, error) {
	host = CanonicalHost(host)
	if host == "" {
		return nil, errors.New("上游 Host 不能为空")
	}
	var result AuthRecoveryPreference
	var vaultEntry sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT host,auth_mode,recovery_method,vault_entry_id,succeeded_at
		FROM auth_recovery_preferences WHERE host=?`, host).Scan(
		&result.Host, &result.AuthMode, &result.RecoveryMethod, &vaultEntry, &result.SucceededAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result.VaultEntryID = nullableText(vaultEntry)
	return &result, nil
}

func (s *Store) SaveAuthRecoveryPreference(ctx context.Context, value AuthRecoveryPreference) error {
	value.Host = CanonicalHost(value.Host)
	value.AuthMode = strings.TrimSpace(value.AuthMode)
	value.RecoveryMethod = strings.TrimSpace(value.RecoveryMethod)
	if value.Host == "" || value.AuthMode == "" || value.RecoveryMethod == "" {
		return errors.New("鉴权恢复偏好缺少 Host、鉴权方式或恢复来源")
	}
	if value.VaultEntryID != nil {
		entry := strings.TrimSpace(*value.VaultEntryID)
		if entry == "" || textLength(entry) > 255 {
			return errors.New("鉴权恢复偏好的密码箱项无效")
		}
		value.VaultEntryID = &entry
	}
	if value.SucceededAt == "" {
		value.SucceededAt = time.Now().UTC().Format(time.RFC3339Nano)
	} else if _, err := time.Parse(time.RFC3339Nano, value.SucceededAt); err != nil {
		return errors.New("鉴权恢复偏好的成功时间无效")
	}
	tx, err := s.beginSettingsTx(ctx, "private.credentials")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, s.dialect(`INSERT INTO auth_recovery_preferences(
		host,auth_mode,recovery_method,vault_entry_id,succeeded_at
	) VALUES(?,?,?,?,?) ON CONFLICT(host) DO UPDATE SET
		auth_mode=excluded.auth_mode,recovery_method=excluded.recovery_method,
		vault_entry_id=COALESCE(excluded.vault_entry_id,auth_recovery_preferences.vault_entry_id),
		succeeded_at=excluded.succeeded_at`, `INSERT INTO auth_recovery_preferences(
		host,auth_mode,recovery_method,vault_entry_id,succeeded_at
	) VALUES(?,?,?,?,?) ON DUPLICATE KEY UPDATE
		auth_mode=VALUES(auth_mode),recovery_method=VALUES(recovery_method),
		vault_entry_id=COALESCE(VALUES(vault_entry_id),auth_recovery_preferences.vault_entry_id),
		succeeded_at=VALUES(succeeded_at)`), value.Host, value.AuthMode, value.RecoveryMethod,
		value.VaultEntryID, value.SucceededAt)
	if err != nil {
		return err
	}
	return tx.Commit()
}
