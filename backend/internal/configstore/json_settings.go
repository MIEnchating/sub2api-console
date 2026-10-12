package configstore

import (
	"context"
	"errors"
	"maps"
	"strings"
)

// UpdateJSONSettings reads and updates only the requested keys in one write
// transaction. The callback must perform local computation only; it must not
// call the store again or make external requests while the writer is held.
func (s *Store) UpdateJSONSettings(ctx context.Context, keys []string, update func(map[string]string) (map[string]string, error)) error {
	if len(keys) == 0 || update == nil {
		return errors.New("设置更新范围不能为空")
	}
	allowed := make(map[string]bool, len(keys))
	for _, key := range keys {
		if strings.TrimSpace(key) == "" || allowed[key] {
			return errors.New("设置更新范围无效")
		}
		allowed[key] = true
	}
	tx, err := s.beginSettingsTx(ctx, keys...)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	current, err := settingsFrom(ctx, tx, keys...)
	if err != nil {
		return err
	}
	changes, err := update(maps.Clone(current))
	if err != nil {
		return err
	}
	for key, value := range changes {
		if !allowed[key] || len(value) > 4<<20 {
			return errors.New("设置更新超出范围或内容过大")
		}
	}
	// Preserve the caller's key order and avoid writes for unchanged values.
	for _, key := range keys {
		value, changed := changes[key]
		old, exists := current[key]
		if !changed || exists && value == old {
			continue
		}
		if _, err := tx.ExecContext(ctx, s.dialect(`INSERT INTO settings("key",value) VALUES(?,?) ON CONFLICT("key") DO UPDATE SET value=excluded.value`, `INSERT INTO settings("key",value) VALUES(?,?) ON DUPLICATE KEY UPDATE value=VALUES(value)`), key, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}
