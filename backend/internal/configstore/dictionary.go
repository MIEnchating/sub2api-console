package configstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrDictionaryConflict = errors.New("字典记录已被其他操作修改")

type DictionaryEntry struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	SortOrder   int    `json:"sort_order"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	Version     int    `json:"version"`
}

// SyncDictionaryValues refreshes values owned by the management platform while
// preserving the local sort order for entries that still exist.
func (s *Store) SyncDictionaryValues(ctx context.Context, kind string, values []DictionaryEntry) error {
	if !validDictionaryKind(kind) {
		return fmt.Errorf("无效的字典类型")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var nextOrder int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sort_order),-1)+1 FROM dictionary_entries WHERE kind=?`, kind).Scan(&nextOrder); err != nil {
		return err
	}
	current := make(map[string]bool, len(values))
	for index := range values {
		name := strings.TrimSpace(values[index].Name)
		value := strings.TrimSpace(values[index].Value)
		if name == "" || value == "" {
			continue
		}
		current[value] = true
		var id string
		err := tx.QueryRowContext(ctx, `SELECT id FROM dictionary_entries WHERE kind=? AND value=?`, kind, value).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			id = uuid.NewString()
			now := time.Now().UTC().Format(time.RFC3339Nano)
			if _, err = tx.ExecContext(ctx, `INSERT INTO dictionary_entries(id,kind,name,value,sort_order,created_at,updated_at,version) VALUES(?,?,?,?,?,?,?,1)`, id, kind, name, value, nextOrder, now, now); err != nil {
				return err
			}
			nextOrder++
		} else if err != nil {
			return err
		} else if _, err = tx.ExecContext(ctx, `UPDATE dictionary_entries SET name=?,updated_at=? WHERE id=? AND name<>?`, name, time.Now().UTC().Format(time.RFC3339Nano), id, name); err != nil {
			return err
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT value FROM dictionary_entries WHERE kind=?`, kind)
	if err != nil {
		return err
	}
	defer rows.Close()
	stale := make([]string, 0)
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return err
		}
		if !current[value] {
			stale = append(stale, value)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, value := range stale {
		if _, err := tx.ExecContext(ctx, `DELETE FROM dictionary_entries WHERE kind=? AND value=?`, kind, value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func validDictionaryKind(k string) bool {
	switch k {
	case "platform", "group", "account_type", "upstream_type", "auth_status", "scheduling_strategy", "task_status", "account_status", "alert_status", "kuma_monitor_type":
		return true
	default:
		return false
	}
}

func (s *Store) ListDictionaries(ctx context.Context, kind string) ([]DictionaryEntry, error) {
	if !validDictionaryKind(kind) {
		return nil, fmt.Errorf("无效的字典类型")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,kind,name,value,description,enabled,sort_order,created_at,updated_at,version FROM dictionary_entries WHERE kind=? ORDER BY sort_order,name,id`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []DictionaryEntry{}
	for rows.Next() {
		var d DictionaryEntry
		var enabled int
		if err := rows.Scan(&d.ID, &d.Kind, &d.Name, &d.Value, &d.Description, &enabled, &d.SortOrder, &d.CreatedAt, &d.UpdatedAt, &d.Version); err != nil {
			return nil, err
		}
		d.Enabled = enabled != 0
		result = append(result, d)
	}
	return result, rows.Err()
}

func (s *Store) SaveDictionary(ctx context.Context, d DictionaryEntry, expectedVersion int) (DictionaryEntry, error) {
	d.Kind = strings.TrimSpace(d.Kind)
	d.Name = strings.TrimSpace(d.Name)
	d.Value = strings.TrimSpace(d.Value)
	d.Description = strings.TrimSpace(d.Description)
	if !validDictionaryKind(d.Kind) || d.Name == "" || d.Value == "" {
		return DictionaryEntry{}, errors.New("字典类型、名称和值不能为空")
	}
	if len([]rune(d.Name)) > 200 || len([]rune(d.Value)) > 500 {
		return DictionaryEntry{}, errors.New("字典名称或值过长")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if d.ID == "" {
		d.ID = uuid.NewString()
		d.CreatedAt = now
		d.UpdatedAt = now
		d.Version = 1
		_, err := s.db.ExecContext(ctx, `INSERT INTO dictionary_entries(id,kind,name,value,description,enabled,sort_order,created_at,updated_at,version) VALUES(?,?,?,?,?,?,?,?,?,?)`, d.ID, d.Kind, d.Name, d.Value, d.Description, boolInt(d.Enabled), d.SortOrder, now, now, 1)
		if err != nil {
			return DictionaryEntry{}, err
		}
		return d, nil
	}
	if expectedVersion <= 0 {
		expectedVersion = d.Version
	}
	result, err := s.db.ExecContext(ctx, `UPDATE dictionary_entries SET kind=?,name=?,value=?,description=?,enabled=?,sort_order=?,updated_at=?,version=version+1 WHERE id=? AND version=?`, d.Kind, d.Name, d.Value, d.Description, boolInt(d.Enabled), d.SortOrder, now, d.ID, expectedVersion)
	if err != nil {
		return DictionaryEntry{}, err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return DictionaryEntry{}, ErrDictionaryConflict
	}
	d.UpdatedAt = now
	d.Version = expectedVersion + 1
	return d, nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (s *Store) DeleteDictionary(ctx context.Context, id string, expectedVersion int) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("字典 ID 不能为空")
	}
	r, err := s.db.ExecContext(ctx, `DELETE FROM dictionary_entries WHERE id=? AND version=?`, id, expectedVersion)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return ErrDictionaryConflict
	}
	return nil
}

func (s *Store) ReorderDictionaries(ctx context.Context, kind string, ids []string) error {
	if !validDictionaryKind(kind) {
		return fmt.Errorf("无效的字典类型")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM dictionary_entries WHERE kind=?`, kind).Scan(&count); err != nil {
		return err
	}
	if count != len(ids) {
		return ErrDictionaryConflict
	}
	seen := make(map[string]bool, len(ids))
	for i, id := range ids {
		if seen[id] {
			return ErrDictionaryConflict
		}
		seen[id] = true
		result, err := tx.ExecContext(ctx, `UPDATE dictionary_entries SET sort_order=?,updated_at=?,version=version+1 WHERE id=? AND kind=?`, i, time.Now().UTC().Format(time.RFC3339Nano), id, kind)
		if err != nil {
			return err
		}
		if affected, err := result.RowsAffected(); err != nil || affected != 1 {
			return ErrDictionaryConflict
		}
	}
	return tx.Commit()
}
