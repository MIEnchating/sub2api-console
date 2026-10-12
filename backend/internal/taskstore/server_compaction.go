package taskstore

import (
	"context"
	"encoding/json"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

func (s *Store) compactServerHistory(ctx context.Context, tx *dbconn.Tx, keep, batch int) (int64, error) {
	query := `SELECT id FROM tasks WHERE operation='automatic-inspection' AND status NOT IN ('queued','running','waiting_input') AND ` + s.objectResultSQL() + ` AND COALESCE(` + s.resultTextSQL("compacted") + `,'') NOT IN ('true','1') AND id NOT IN (SELECT id FROM (SELECT id FROM tasks WHERE operation='automatic-inspection' AND status NOT IN ('queued','running','waiting_input') ORDER BY updated_at DESC,id DESC LIMIT ?) AS recent) ORDER BY updated_at,id LIMIT ? FOR UPDATE SKIP LOCKED`
	rows, err := tx.QueryContext(ctx, query, keep, batch)
	if err != nil {
		return 0, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		var raw string
		if err := tx.QueryRowContext(ctx, `SELECT result_json FROM tasks WHERE id=?`, id).Scan(&raw); err != nil {
			return 0, err
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &fields); err != nil {
			return 0, err
		}
		if fields == nil {
			fields = map[string]json.RawMessage{}
		}
		for section, keys := range map[string][]string{"routing": {"account_targets", "account_decisions"}, "account_rate_sync": {"items"}, "writeback": {"results"}, "upstream_sync": {"hosts"}, "price_management": {"items"}} {
			if value, exists := fields[section]; exists {
				var nested map[string]json.RawMessage
				if err := json.Unmarshal(value, &nested); err != nil || nested == nil {
					continue
				}
				for _, key := range keys {
					delete(nested, key)
				}
				value, err := json.Marshal(nested)
				if err != nil {
					return 0, err
				}
				fields[section] = value
			}
		}
		fields["compacted"] = json.RawMessage("true")
		encoded, err := json.Marshal(fields)
		if err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE tasks SET result_json=? WHERE id=?`, string(encoded), id); err != nil {
			return 0, err
		}
	}
	return int64(len(ids)), nil
}

func (s *Store) compactServerBatch(ctx context.Context, keep, batch int) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	changed, err := s.compactServerHistory(ctx, tx, keep, batch)
	if err != nil {
		return 0, err
	}
	return changed, tx.Commit()
}
