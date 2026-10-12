package taskstore

import (
	"context"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

// RebuildImportedState uses the same projection writer as normal task updates.
// Each batch is bounded; imported raw results and recovery inputs stay intact.
func RebuildImportedState(ctx context.Context, tx *dbconn.Tx) error {
	store := &Store{}
	cursor := ""
	first := true
	for {
		query := `SELECT id,skill,operation,status,result_json FROM tasks`
		args := []any{}
		if !first {
			query += ` WHERE id>?`
			args = append(args, cursor)
		}
		rows, err := tx.QueryContext(ctx, query+` ORDER BY id LIMIT 100`, args...)
		if err != nil {
			return err
		}
		type entry struct {
			task Task
			raw  string
		}
		entries := []entry{}
		for rows.Next() {
			var value entry
			if err := rows.Scan(&value.task.ID, &value.task.Skill, &value.task.Operation, &value.task.Status, &value.raw); err != nil {
				rows.Close()
				return err
			}
			entries = append(entries, value)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, value := range entries {
			if err := store.writeHistoryProjection(ctx, tx, value.task, []byte(value.raw)); err != nil {
				return err
			}
		}
		if len(entries) < 100 {
			return nil
		}
		cursor = entries[len(entries)-1].task.ID
		first = false
	}
}
