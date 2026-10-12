package business

import "context"

// UpstreamSyncTargets reads only task scope and failure-result account counts.
// A non-nil host never expands into unrelated upstreams.
func (s *Store) UpstreamSyncTargets(ctx context.Context, host *string) (UpstreamSummary, error) {
	query := `SELECT h.host,(SELECT COUNT(DISTINCT b.local_account_id) FROM binding_identities bi JOIN bindings b ON b.id=bi.binding_id WHERE bi.upstream_id=h.upstream_id) FROM upstream_identity_hosts h JOIN upstreams u ON u.host=h.host WHERE h.is_primary=1 AND h.upstream_id NOT IN (SELECT upstream_id FROM upstream_exclusions)`
	args := []any{}
	if host != nil {
		query += ` AND h.upstream_id=(SELECT upstream_id FROM upstream_identity_hosts WHERE host=?)`
		args = append(args, canonicalHost(*host))
	}
	rows, err := s.db.QueryContext(ctx, query+` ORDER BY h.host`, args...)
	if err != nil {
		return UpstreamSummary{}, err
	}
	defer rows.Close()
	result := UpstreamSummary{Hosts: []UpstreamHost{}}
	for rows.Next() {
		var item UpstreamHost
		if err := rows.Scan(&item.Host, &item.AccountCount); err != nil {
			return UpstreamSummary{}, err
		}
		result.Hosts = append(result.Hosts, item)
	}
	return result, rows.Err()
}
