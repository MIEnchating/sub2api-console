package configstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"time"
)

// InitializeSession saves the first credentials, target and authenticated session
// atomically. A failed session insert leaves initialization available for retry.
func (s *Store) InitializeSession(ctx context.Context, username, password, baseURL, adminKey string, localExportOnly bool, ttl time.Duration, now time.Time) (string, error) {
	if localExportOnly && (strings.TrimSpace(baseURL) != "" || strings.TrimSpace(adminKey) != "") {
		return "", errors.New("独立导出初始化不能同时提交管理目标")
	}
	if ttl < time.Second || ttl > 31*24*time.Hour {
		return "", errors.New("会话有效期必须在 1 秒到 31 天之间")
	}
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(bytes)
	issued := now.UTC().Truncate(time.Second)
	err := s.initialize(ctx, username, password, baseURL, adminKey, localExportOnly, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO console_sessions(token_hash,username,expires_at,created_at) VALUES(?,?,?,?)`, hashSessionToken(token), strings.TrimSpace(username), formatTime(issued.Add(ttl)), formatTime(issued))
		return err
	})
	if err != nil {
		return "", err
	}
	return token, nil
}
