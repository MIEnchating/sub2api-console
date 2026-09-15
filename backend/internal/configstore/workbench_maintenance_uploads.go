package configstore

import (
	"context"
	"errors"
)

// Only the Go maintenance domain reads these private records. Public summaries
// are constructed separately and never include credentials or account snapshots.
func (s *Store) WorkbenchMaintenanceExecutions(ctx context.Context, target string) ([]WorkbenchExecution, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT json_extract(value,'$.id') FROM settings
WHERE key LIKE ? AND json_valid(value) AND json_extract(value,'$.target_fingerprint')=?
AND json_extract(value,'$.maintenance') IS NOT NULL ORDER BY key LIMIT 10001`, workbenchExecutionPrefix+"%", target)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if len(ids) > 10000 {
		return nil, ErrWorkbenchExecution
	}
	result := []WorkbenchExecution{}
	for _, id := range ids {
		record, err := s.WorkbenchExecution(ctx, id)
		if errors.Is(err, ErrWorkbenchExecution) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if record.Maintenance != nil && record.TargetFingerprint == target {
			result = append(result, record)
		}
	}
	return result, nil
}
