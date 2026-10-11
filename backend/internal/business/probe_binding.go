package business

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// ProbeAccountBinding contains only the current, non-secret inputs needed to
// verify a probe credential. Display scores, history and policy are unrelated.
type ProbeAccountBinding struct {
	ID              string
	BaseURL         *string
	UpstreamBaseURL *string
	Bindings        []AccountBinding
}

func (s *Store) ProbeAccountBinding(ctx context.Context, accountID string) (*ProbeAccountBinding, error) {
	accountID = strings.TrimSpace(accountID)
	if !positiveNumericID(accountID) {
		return nil, errors.New("账号必须使用有效的稳定 ID")
	}
	result := &ProbeAccountBinding{}
	err := s.WithReadSnapshot(ctx, func(ctx context.Context) error {
		var metadataRaw string
		var upstreamBaseURL sql.NullString
		// Match Account's stable ownership resolution, but restrict the binding
		// aggregation to this account instead of projecting the entire directory.
		err := s.db.QueryRowContext(ctx, `SELECT a.id,a.metadata_json,u.base_url
			FROM accounts a LEFT JOIN upstreams u ON u.host=COALESCE((
				SELECT MIN(COALESCE(primary_host.host,b.upstream_host))
				FROM bindings b JOIN binding_identities bi ON bi.binding_id=b.id
				LEFT JOIN upstream_identity_hosts primary_host
				 ON primary_host.upstream_id=bi.upstream_id AND primary_host.is_primary=1
				WHERE b.local_account_id=a.id AND TRIM(b.upstream_host)<>''
			),a.upstream_host) WHERE a.id=?`, accountID).Scan(&result.ID, &metadataRaw, &upstreamBaseURL)
		if err != nil {
			return err
		}
		metadata, err := decodeObject(metadataRaw)
		if err != nil {
			return err
		}
		result.BaseURL = accountMetadataValueText(metadata, "base_url")
		result.UpstreamBaseURL = nullString(upstreamBaseURL)
		result.Bindings, err = s.accountBindings(ctx, accountID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
