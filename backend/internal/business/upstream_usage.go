package business

import "context"

// UpstreamList reads the management catalog without scanning request history.
func (s *Store) UpstreamList(ctx context.Context) (UpstreamSummary, error) {
	return s.readUpstreams(ctx, nil, false)
}

type UpstreamUsage struct {
	UpstreamID          string                        `json:"upstream_id"`
	RequestCount        int64                         `json:"request_count"`
	RequestWindowDays   int                           `json:"request_window_days"`
	AccountRequestUsage []UpstreamAccountRequestUsage `json:"account_request_usage"`
}

func (s *Store) UpstreamUsage(ctx context.Context) ([]UpstreamUsage, error) {
	usage, counts, err := s.upstreamAccountRequestUsage(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT h.upstream_id FROM upstream_identity_hosts h JOIN upstreams u ON u.host=h.host WHERE h.is_primary=1 ORDER BY h.upstream_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []UpstreamUsage{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		accounts := usage[id]
		if accounts == nil {
			accounts = []UpstreamAccountRequestUsage{}
		}
		result = append(result, UpstreamUsage{UpstreamID: id, RequestCount: counts[id], RequestWindowDays: 30, AccountRequestUsage: accounts})
	}
	return result, rows.Err()
}
