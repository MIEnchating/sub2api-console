package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

// AccountMutationSnapshot contains only the fields needed to validate and
// execute a single-account mutation. Display projections and evidence history
// must not delay writes or make them depend on unrelated read models.
type AccountMutationSnapshot struct {
	ID             string
	Name           string
	ManualPriority *int64
}

func (s *Store) AccountMutationSnapshot(ctx context.Context, accountID string) (*AccountMutationSnapshot, error) {
	accountID = strings.TrimSpace(accountID)
	if !positiveNumericID(accountID) || accountID[0] < '1' || accountID[0] > '9' {
		return nil, errors.New("账号必须使用有效的稳定 ID")
	}
	var snapshot AccountMutationSnapshot
	var manualPriority sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT a.id,a.name,m.priority FROM accounts a
		LEFT JOIN manual_priority_accounts m ON m.account_id=a.id WHERE a.id=?`, accountID).
		Scan(&snapshot.ID, &snapshot.Name, &manualPriority)
	if err != nil {
		return nil, err
	}
	snapshot.ManualPriority = nullInt(manualPriority)
	return &snapshot, nil
}

// AccountIDsExist validates a selection with one database read. It is used by
// live account streams, where resolving every full detail would scale linearly
// with the number of selected accounts.
func (s *Store) AccountIDsExist(ctx context.Context, accountIDs []string) (bool, error) {
	unique := make(map[string]struct{}, len(accountIDs))
	for _, accountID := range accountIDs {
		if !positiveNumericID(accountID) || accountID[0] < '1' || accountID[0] > '9' {
			return false, errors.New("账号必须使用有效的稳定 ID")
		}
		unique[accountID] = struct{}{}
	}
	if len(unique) == 0 {
		return true, nil
	}
	ids := make([]string, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return false, err
	}
	var count int
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts WHERE id IN
		(SELECT value FROM `+jsonScopeRows(s.db.Driver())+` scope)`, string(raw)).Scan(&count)
	return count == len(unique), err
}
