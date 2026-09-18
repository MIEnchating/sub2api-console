package business

import (
	"context"
	"errors"
	"strings"
)

type UpstreamGroupChange struct {
	ID            int64   `json:"id"`
	UpstreamID    string  `json:"upstream_id"`
	GroupID       string  `json:"group_id"`
	GroupName     string  `json:"group_name"`
	EffectiveRate *string `json:"effective_rate"`
	ChangeType    string  `json:"change_type"`
	ChangedAt     string  `json:"changed_at"`
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
	return s.readUpstreamGroupHistory(ctx, &upstreamID, limit)
}

func (s *Store) AllUpstreamGroupHistory(ctx context.Context, limit int) ([]UpstreamGroupChange, error) {
	if limit < 1 || limit > 500 {
		return nil, errors.New("limit 必须在 1 到 500 之间")
	}
	return s.readUpstreamGroupHistory(ctx, nil, limit)
}

func (s *Store) ClearUpstreamGroupHistory(ctx context.Context) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM upstream_group_change_events`)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) readUpstreamGroupHistory(ctx context.Context, upstreamID *string, limit int) ([]UpstreamGroupChange, error) {
	query := `SELECT e.id,e.upstream_id,e.group_id,e.group_name,e.change_type,e.changed_at,
		(SELECT g.effective_rate FROM upstream_groups g
		 JOIN upstream_identity_hosts h ON h.host=g.host
		 WHERE h.upstream_id=e.upstream_id AND g.group_id=e.group_id
		 ORDER BY g.updated_at DESC,h.is_primary DESC,h.host LIMIT 1)
		FROM upstream_group_change_events e`
	args := []any{}
	if upstreamID != nil {
		query += ` WHERE e.upstream_id=?`
		args = append(args, *upstreamID)
	}
	query += ` ORDER BY e.changed_at DESC,e.id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []UpstreamGroupChange{}
	for rows.Next() {
		var item UpstreamGroupChange
		if err := rows.Scan(&item.ID, &item.UpstreamID, &item.GroupID, &item.GroupName, &item.ChangeType, &item.ChangedAt, &item.EffectiveRate); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
