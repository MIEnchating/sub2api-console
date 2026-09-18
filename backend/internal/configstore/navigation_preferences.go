package configstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
)

var ErrNavigationConflict = errors.New("菜单设置已在其他页面修改，请重新读取后再操作")
var navigationItemPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,79}$`)

// A nil list denotes an unset preference, allowing one-time browser preference import.
type NavigationPreferences struct {
	HiddenItemIDs []string `json:"hidden_item_ids"`
	Version       string   `json:"version"`
}

func navigationPreferences(raw string) (NavigationPreferences, error) {
	result := NavigationPreferences{}
	if raw == "" {
		return result, nil
	}
	if err := json.Unmarshal([]byte(raw), &result.HiddenItemIDs); err != nil {
		return result, errors.New("菜单设置不可读，请检查控制台配置")
	}
	digest := sha256.Sum256([]byte(raw))
	result.Version = hex.EncodeToString(digest[:])
	return result, nil
}

func (s *Store) NavigationPreferences(ctx context.Context) (NavigationPreferences, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='console.navigation_hidden_items'`).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return NavigationPreferences{}, err
	}
	return navigationPreferences(raw)
}

func (s *Store) SaveNavigationPreferences(ctx context.Context, input NavigationPreferences) (NavigationPreferences, error) {
	if input.HiddenItemIDs == nil || len(input.HiddenItemIDs) > 100 {
		return NavigationPreferences{}, errors.New("菜单设置必须包含最多 100 个路由入口")
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, id := range input.HiddenItemIDs {
		if !navigationItemPattern.MatchString(id) {
			return NavigationPreferences{}, errors.New("菜单路由标识无效")
		}
		if id != "config" && !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	sort.Strings(ids)
	raw, err := json.Marshal(ids)
	if err != nil {
		return NavigationPreferences{}, err
	}
	// Atomic comparison prevents a stale browser from overwriting a newer preference.
	result, err := s.NavigationPreferences(ctx)
	if err != nil {
		return NavigationPreferences{}, err
	}
	if result.Version != input.Version {
		return NavigationPreferences{}, ErrNavigationConflict
	}
	if input.Version == "" {
		inserted, err := s.db.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES('console.navigation_hidden_items',?) ON CONFLICT(key) DO NOTHING`, string(raw))
		if err != nil {
			return NavigationPreferences{}, err
		}
		count, err := inserted.RowsAffected()
		if err != nil {
			return NavigationPreferences{}, err
		}
		if count != 1 {
			return NavigationPreferences{}, ErrNavigationConflict
		}
	} else {
		previous, _ := json.Marshal(result.HiddenItemIDs)
		updated, err := s.db.ExecContext(ctx, `UPDATE settings SET value=? WHERE key='console.navigation_hidden_items' AND value=?`, string(raw), string(previous))
		if err != nil {
			return NavigationPreferences{}, err
		}
		count, err := updated.RowsAffected()
		if err != nil {
			return NavigationPreferences{}, err
		}
		if count != 1 {
			return NavigationPreferences{}, ErrNavigationConflict
		}
	}
	return navigationPreferences(string(raw))
}
