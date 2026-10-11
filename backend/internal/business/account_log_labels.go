package business

import (
	"context"
	"database/sql"
	"strings"
)

// AccountLogLabel contains only display metadata; reading logs does not need
// account health, policy, binding credentials, or request history projections.
type AccountLogLabel struct {
	Name     string
	GroupIDs map[string]*string
}

func (s *Store) AccountLogLabels(ctx context.Context, accountIDs []string) (map[string]*AccountLogLabel, error) {
	result := make(map[string]*AccountLogLabel)
	seen := make(map[string]struct{}, len(accountIDs))
	arguments := make([]any, 0, len(accountIDs))
	for _, rawID := range accountIDs {
		id := strings.TrimSpace(rawID)
		if _, duplicate := seen[id]; id == "" || duplicate {
			continue
		}
		seen[id] = struct{}{}
		arguments = append(arguments, id)
	}
	if len(arguments) == 0 {
		return result, ctx.Err()
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(arguments)), ",")
	rows, err := s.db.QueryContext(ctx, `SELECT a.id,a.name,ag.group_name,ag.group_id
		FROM accounts a LEFT JOIN account_groups ag ON ag.account_id=a.id
		WHERE a.id IN (`+placeholders+`) ORDER BY a.id,ag.group_name`, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		var groupName, groupID sql.NullString
		if err := rows.Scan(&id, &name, &groupName, &groupID); err != nil {
			return nil, err
		}
		label := result[id]
		if label == nil {
			label = &AccountLogLabel{Name: name, GroupIDs: make(map[string]*string)}
			result[id] = label
		}
		if groupName.Valid {
			label.GroupIDs[groupName.String] = nullString(groupID)
		}
	}
	return result, rows.Err()
}
