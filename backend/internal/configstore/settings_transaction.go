package configstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"slices"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

type privateRowReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Lock rows coordinate absent settings without creating placeholder values.
// Scopes acquire locks in a stable order; unrelated scopes remain concurrent.
func (s *Store) beginSettingsTx(ctx context.Context, keys ...string) (*dbconn.Tx, error) {
	options := &sql.TxOptions{}
	if s.db.Driver() != dbconn.SQLite {
		options.Isolation = sql.LevelReadCommitted
	}
	tx, err := s.db.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	if s.db.Driver() == dbconn.SQLite {
		return tx, nil
	}
	locks := make([]string, 0, len(keys))
	for _, key := range keys {
		digest := sha256.Sum256([]byte(key))
		locks = append(locks, hex.EncodeToString(digest[:]))
	}
	slices.Sort(locks)
	for _, key := range slices.Compact(locks) {
		_, err := tx.ExecContext(ctx, s.dialect(`INSERT INTO config_setting_locks(id) VALUES(?) ON CONFLICT(id) DO UPDATE SET id=excluded.id`, `INSERT INTO config_setting_locks(id) VALUES(?) ON DUPLICATE KEY UPDATE id=VALUES(id)`), key)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	return tx, nil
}
