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
	VaultEntry     *string
	SucceededAt    string
}

func (s *Store) AuthRecoveryPreference(ctx context.Context, host string) (*AuthRecoveryPreference, error) {
	host = CanonicalHost(host)
	if host == "" {
		return nil, errors.New("上游 Host 不能为空")
	}
	var result AuthRecoveryPreference
	var vaultEntry sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT host,auth_mode,recovery_method,vault_entry,succeeded_at
		FROM auth_recovery_preferences WHERE host=?`, host).Scan(
		&result.Host, &result.AuthMode, &result.RecoveryMethod, &vaultEntry, &result.SucceededAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result.VaultEntry = nullableText(vaultEntry)
	return &result, nil
}

func (s *Store) SaveAuthRecoveryPreference(ctx context.Context, value AuthRecoveryPreference) error {
	value.Host = CanonicalHost(value.Host)
	value.AuthMode = strings.TrimSpace(value.AuthMode)
	value.RecoveryMethod = strings.TrimSpace(value.RecoveryMethod)
	if value.Host == "" || value.AuthMode == "" || value.RecoveryMethod == "" {
		return errors.New("鉴权恢复偏好缺少 Host、鉴权方式或恢复来源")
	}
	if value.VaultEntry != nil {
		entry := strings.TrimSpace(*value.VaultEntry)
		if entry == "" || len(entry) > 255 {
			return errors.New("鉴权恢复偏好的密码箱项无效")
		}
		value.VaultEntry = &entry
	}
	if value.SucceededAt == "" {
		value.SucceededAt = time.Now().UTC().Format(time.RFC3339Nano)
	} else if _, err := time.Parse(time.RFC3339Nano, value.SucceededAt); err != nil {
		return errors.New("鉴权恢复偏好的成功时间无效")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO auth_recovery_preferences(
		host,auth_mode,recovery_method,vault_entry,succeeded_at
	) VALUES(?,?,?,?,?) ON CONFLICT(host) DO UPDATE SET
		auth_mode=excluded.auth_mode,recovery_method=excluded.recovery_method,
		vault_entry=COALESCE(excluded.vault_entry,auth_recovery_preferences.vault_entry),
		succeeded_at=excluded.succeeded_at`, value.Host, value.AuthMode, value.RecoveryMethod,
		value.VaultEntry, value.SucceededAt)
	return err
}
