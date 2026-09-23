package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

func (s *Store) LoadDetectionTasks(ctx context.Context) ([]byte, error) {
	return s.loadModelCheckState(ctx, "model-detection-tasks")
}
func (s *Store) SaveDetectionTasks(ctx context.Context, raw []byte, actor, action string) error {
	return s.saveModelCheckState(ctx, raw, actor, action, "model-detection-tasks")
}

// DetectionTaskScope is the directory snapshot used for one execution.
type DetectionTaskScope struct {
	AccountIDs        []string
	GroupIDsByAccount map[string][]string
	GroupNamesByID    map[string]string
}

func (s *Store) DetectionTaskAccountIDs(ctx context.Context, groups []string) ([]string, error) {
	scope, err := s.DetectionTaskScope(ctx, groups)
	return scope.AccountIDs, err
}

// Resolve accounts and group labels together, so a directory sync cannot split the snapshot.
func (s *Store) DetectionTaskScope(ctx context.Context, groups []string) (DetectionTaskScope, error) {
	scope := DetectionTaskScope{AccountIDs: []string{}, GroupIDsByAccount: map[string][]string{}, GroupNamesByID: map[string]string{}}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return scope, err
	}
	defer tx.Rollback()
	for _, id := range groups {
		var name string
		if err := tx.QueryRowContext(ctx, "SELECT name FROM local_groups WHERE remote_id=?", id).Scan(&name); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return scope, fmt.Errorf("分组 %s 不存在，请重新选择", id)
			}
			return scope, err
		}
		scope.GroupNamesByID[id] = name
	}
	raw, err := json.Marshal(groups)
	if err != nil {
		return scope, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT DISTINCT ag.account_id, ag.group_id FROM account_groups ag JOIN accounts a ON a.id=ag.account_id WHERE ag.group_id IN (SELECT value FROM json_each(?)) ORDER BY ag.account_id, ag.group_id", string(raw))
	if err != nil {
		return scope, err
	}
	defer rows.Close()
	for rows.Next() {
		var accountID, groupID string
		if err := rows.Scan(&accountID, &groupID); err != nil {
			return scope, err
		}
		if _, exists := scope.GroupIDsByAccount[accountID]; !exists {
			scope.AccountIDs = append(scope.AccountIDs, accountID)
		}
		if !slices.Contains(scope.GroupIDsByAccount[accountID], groupID) {
			scope.GroupIDsByAccount[accountID] = append(scope.GroupIDsByAccount[accountID], groupID)
		}
	}
	if err := rows.Err(); err != nil {
		return scope, err
	}
	return scope, tx.Commit()
}
