package business

import (
	"context"
	"encoding/json"
	"fmt"
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
	var scope DetectionTaskScope
	err := s.WithReadSnapshot(ctx, func(ctx context.Context) error {
		var err error
		scope, err = s.detectionTaskScope(ctx, groups)
		return err
	})
	return scope, err
}

func (s *Store) detectionTaskScope(ctx context.Context, groups []string) (DetectionTaskScope, error) {
	scope := DetectionTaskScope{AccountIDs: []string{}, GroupIDsByAccount: map[string][]string{}, GroupNamesByID: map[string]string{}}
	if len(groups) == 0 {
		return scope, nil
	}
	raw, err := json.Marshal(groups)
	if err != nil {
		return scope, err
	}
	// Scan the group directory once rather than once per requested group.
	// The membership read shares this exact snapshot.
	labels, err := s.db.QueryContext(ctx, `SELECT remote_id,name FROM local_groups
		WHERE remote_id IN (SELECT value FROM json_each(?)) ORDER BY rowid`, string(raw))
	if err != nil {
		return scope, err
	}
	defer labels.Close()
	names := make(map[string]string, len(groups))
	for labels.Next() {
		var id, name string
		if err := labels.Scan(&id, &name); err != nil {
			return scope, err
		}
		if _, found := names[id]; !found {
			names[id] = name
		}
	}
	if err := labels.Err(); err != nil {
		return scope, err
	}
	if err := labels.Close(); err != nil {
		return scope, err
	}
	// Match the previous first-missing-ID error in request order.
	for _, id := range groups {
		name, found := names[id]
		if !found {
			return scope, fmt.Errorf("分组 %s 不存在，请重新选择", id)
		}
		scope.GroupNamesByID[id] = name
	}
	rows, err := s.db.QueryContext(ctx, "SELECT DISTINCT ag.account_id, ag.group_id FROM account_groups ag JOIN accounts a ON a.id=ag.account_id WHERE a.id NOT IN ("+excludedAccountIDsSQL+") AND ag.group_id IN (SELECT value FROM json_each(?)) ORDER BY ag.account_id, ag.group_id", string(raw))
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
		// DISTINCT already guarantees each account/group pair appears once.
		scope.GroupIDsByAccount[accountID] = append(scope.GroupIDsByAccount[accountID], groupID)
	}
	if err := rows.Err(); err != nil {
		return scope, err
	}
	return scope, nil
}
