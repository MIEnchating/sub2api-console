package business

import (
	"context"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func (s *Store) BoundAccountIDsForMaintenance(ctx context.Context) ([]string, error) {
	return s.boundMaintenanceAccountIDs(ctx, "", false)
}

func (s *Store) BoundAccountIDsForMaintenanceHost(ctx context.Context, host string) ([]string, error) {
	host = configstore.CanonicalHost(host)
	if host == "" {
		return []string{}, nil
	}
	return s.boundMaintenanceAccountIDs(ctx, host, true)
}

func (s *Store) boundMaintenanceAccountIDs(ctx context.Context, host string, filterHost bool) ([]string, error) {
	// Keep the execution reader's required joins, without loading rates or JSON.
	// Host normalization stays in Go for bindings written in legacy URL formats.
	rows, err := s.db.QueryContext(ctx, `SELECT a.id,u.host,COALESCE(b.source_auth_host,'')
		FROM accounts a JOIN bindings b ON b.local_account_id=a.id
		JOIN binding_identities bi ON bi.binding_id=b.id
		JOIN upstream_identity_hosts primary_host ON primary_host.upstream_id=bi.upstream_id AND primary_host.is_primary=1
		JOIN upstreams u ON u.host=primary_host.host
		WHERE a.id NOT IN (`+excludedAccountIDsSQL+`)
		ORDER BY CASE WHEN a.id GLOB '[0-9]*' THEN CAST(a.id AS INTEGER) ELSE 0 END,a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]string, 0)
	seen := make(map[string]struct{})
	for rows.Next() {
		var accountID, primaryHost, sourceHost string
		if err := rows.Scan(&accountID, &primaryHost, &sourceHost); err != nil {
			return nil, err
		}
		if filterHost && configstore.CanonicalHost(primaryHost) != host && configstore.CanonicalHost(sourceHost) != host {
			continue
		}
		accountID = strings.TrimSpace(accountID)
		if accountID == "" {
			continue
		}
		if _, exists := seen[accountID]; exists {
			continue
		}
		seen[accountID] = struct{}{}
		result = append(result, accountID)
	}
	return result, rows.Err()
}
