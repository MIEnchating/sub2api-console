package business

import (
	"context"
	"database/sql"
	"errors"
)

type UpstreamAuthSeed struct {
	Host         string
	BaseURL      string
	UpstreamType string
	AuthMode     *string
}

func (s *Store) UpstreamAuthSeed(ctx context.Context, host string) (*UpstreamAuthSeed, error) {
	var result UpstreamAuthSeed
	var authMode sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT host,base_url,upstream_type,auth_mode FROM upstreams WHERE host=?`, canonicalHost(host)).Scan(
		&result.Host, &result.BaseURL, &result.UpstreamType, &authMode,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if authMode.Valid {
		result.AuthMode = &authMode.String
	}
	return &result, nil
}
