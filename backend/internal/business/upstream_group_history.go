package business

import (
	"context"
	"errors"
	"strings"
)

type UpstreamGroupChange struct {
	ID         int64  `json:"id"`
	UpstreamID string `json:"upstream_id"`
	GroupID    string `json:"group_id"`
	GroupName  string `json:"group_name"`
	ChangeType string `json:"change_type"`
	ChangedAt  string `json:"changed_at"`
}

func (s *Store) UpstreamGroupHistory(ctx context.Context, host string, limit int) ([]UpstreamGroupChange, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, errors.New("上游 Host 不能为空")
	}
	if limit < 1 || limit > 500 {
		return nil, errors.New("limit 必须在 1 到 500 之间")
	}
	upstreamID, _, err := upstreamIdentityHostsForQueryer(ctx, s.db, host)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,upstream_id,group_id,group_name,change_type,changed_at
		FROM upstream_group_change_events WHERE upstream_id=? ORDER BY changed_at DESC,id DESC LIMIT ?`, upstreamID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []UpstreamGroupChange{}
	for rows.Next() {
		var item UpstreamGroupChange
		if err := rows.Scan(&item.ID, &item.UpstreamID, &item.GroupID, &item.GroupName, &item.ChangeType, &item.ChangedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
