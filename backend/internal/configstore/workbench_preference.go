package configstore

import (
	"context"
	"database/sql"
	"encoding/json"
)

func clearOtherWorkbenchPreferences(ctx context.Context, tx *sql.Tx, preferred WorkbenchTemplate) error {
	prefix := workbenchTargetPrefix(preferred.TargetURL)
	rows, err := tx.QueryContext(ctx, `SELECT value FROM settings WHERE key LIKE ? AND key<>? AND json_extract(value,'$.preferred')=1`, prefix+"%", prefix+preferred.ID)
	if err != nil {
		return err
	}
	others := []WorkbenchTemplate{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			_ = rows.Close()
			return err
		}
		item, err := decodeWorkbenchTemplate(raw, preferred.TargetURL)
		if err != nil {
			_ = rows.Close()
			return err
		}
		item.Preferred = false
		item.Revision++
		if item.Revision >= 1<<53 {
			_ = rows.Close()
			return ErrWorkbenchTemplateConflict
		}
		others = append(others, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range others {
		raw, err := json.Marshal(item)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE settings SET value=? WHERE key=?`, string(raw), prefix+item.ID); err != nil {
			return err
		}
	}
	return nil
}
