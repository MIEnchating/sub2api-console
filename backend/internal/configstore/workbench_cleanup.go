package configstore

import (
	"context"
	"encoding/json"
)

type WorkbenchExecutionSummary struct {
	ID       string                       `json:"id"`
	Revision int64                        `json:"revision"`
	Items    []WorkbenchExecutionIdentity `json:"items"`
}

type WorkbenchExecutionIdentity struct {
	AccountID         string `json:"account_id"`
	OriginalAccountID string `json:"original_account_id"`
	ProfileID         string `json:"profile_id"`
	Identity          string `json:"identity"`
	Status            string `json:"status"`
}

// SQL constructs only identity metadata; restart credentials and snapshots
// never leave the private store through this listing.
func (s *Store) WorkbenchExecutionSummaries(ctx context.Context, target string) ([]WorkbenchExecutionSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT json_extract(value,'$.id'),json_extract(value,'$.revision'),
 COALESCE((SELECT json_group_array(json_object('account_id',json_extract(item.value,'$.account_id'),'original_account_id',json_extract(item.value,'$.original_account_id'),'profile_id',json_extract(item.value,'$.login_profile_id'),'identity',json_extract(item.value,'$.identity'),'status',json_extract(item.value,'$.status'))) FROM json_each(settings.value,'$.items') item),'[]')
 FROM settings WHERE key LIKE ? AND json_valid(value) AND json_extract(value,'$.target_fingerprint')=? ORDER BY key LIMIT 10001`, workbenchExecutionPrefix+"%", target)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []WorkbenchExecutionSummary{}
	for rows.Next() {
		var item WorkbenchExecutionSummary
		var raw string
		if err := rows.Scan(&item.ID, &item.Revision, &raw); err != nil {
			return nil, err
		}
		if json.Unmarshal([]byte(raw), &item.Items) != nil || !workbenchIdentifier.MatchString(item.ID) || item.Revision < 1 {
			return nil, ErrWorkbenchExecution
		}
		result = append(result, item)
	}
	if len(result) > 10000 {
		return nil, ErrWorkbenchExecution
	}
	return result, rows.Err()
}

func (s *Store) DeleteWorkbenchExecution(ctx context.Context, target, id string, revision int64) error {
	if !workbenchIdentifier.MatchString(id) || revision < 1 {
		return ErrWorkbenchExecution
	}
	s.workbenchExecutionWriteMu.Lock()
	defer s.workbenchExecutionWriteMu.Unlock()
	result, err := s.db.ExecContext(ctx, `DELETE FROM settings WHERE key=? AND json_valid(value) AND json_extract(value,'$.target_fingerprint')=? AND json_extract(value,'$.revision')=?`, workbenchExecutionPrefix+id, target, revision)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrWorkbenchExecution
	}
	return nil
}
