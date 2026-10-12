package configstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"strings"
	"time"
)

// UpdateSessionCredentials commits credentials and session revocation together.
// A browser keeps its authenticated session and original expiry; a bearer-only
// caller receives a new session in the same transaction, without rehashing the password.
func (s *Store) UpdateSessionCredentials(ctx context.Context, token, currentPassword, username string, newPassword *string, ttl time.Duration, now time.Time) (string, string, error) {
	token = strings.TrimSpace(token)
	preserve := token != ""
	if !preserve {
		if ttl < time.Second || ttl > 31*24*time.Hour {
			return "", "", errors.New("会话有效期必须在 1 秒到 31 天之间")
		}
		bytes := make([]byte, 32)
		if _, err := rand.Read(bytes); err != nil {
			return "", "", err
		}
		token = base64.RawURLEncoding.EncodeToString(bytes)
	}
	hash := hashSessionToken(token)
	name, err := s.updateCredentials(ctx, currentPassword, username, newPassword, func(tx *dbconn.Tx, previousUsername string) error {
		if preserve {
			var sessionName, expiry string
			err := tx.QueryRowContext(ctx, `SELECT username,expires_at FROM console_sessions WHERE token_hash=?`, hash).Scan(&sessionName, &expiry)
			if errors.Is(err, sql.ErrNoRows) {
				return errors.New("当前会话已失效，请重新登录后修改资料")
			}
			if err != nil {
				return err
			}
			expires, parseErr := parseTime(expiry)
			if parseErr != nil || !expires.After(now.UTC()) || sessionName != previousUsername {
				return errors.New("当前会话已失效，请重新登录后修改资料")
			}
			if _, err := tx.ExecContext(ctx, `UPDATE console_sessions SET username=? WHERE token_hash=?`, strings.TrimSpace(username), hash); err != nil {
				return err
			}
		} else {
			issued := now.UTC().Truncate(time.Second)
			if _, err := tx.ExecContext(ctx, `INSERT INTO console_sessions(token_hash,username,expires_at,created_at) VALUES(?,?,?,?)`, hash, strings.TrimSpace(username), formatTime(issued.Add(ttl)), formatTime(issued)); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM console_sessions WHERE token_hash<>?`, hash)
		return err
	})
	if err != nil {
		return "", "", err
	}
	return name, token, nil
}
