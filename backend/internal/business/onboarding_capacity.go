package business

import (
	"context"
	"errors"
)

// OnboardingCapacity uses the primary stable identity, including alias bindings
// and accounts outside the selected groups. Each account is returned once.
type OnboardingCapacity struct {
	UpstreamID string
	Limit      *int64
	Status     string
	Accounts   []RoutingAccount
}

func (s *Store) OnboardingCapacity(ctx context.Context, upstreamID string) (OnboardingCapacity, error) {
	result := OnboardingCapacity{UpstreamID: upstreamID}
	if upstreamID == "" {
		return result, errors.New("上游稳定身份未确认，请同步上游后重试")
	}
	var platform, raw string
	err := s.db.QueryRowContext(ctx, `SELECT u.upstream_type,u.metadata_json FROM upstream_identity_hosts h JOIN upstreams u ON u.host=h.host WHERE h.upstream_id=? AND h.is_primary=1`, upstreamID).Scan(&platform, &raw)
	if err != nil {
		return result, err
	}
	metadata, err := decodeObject(raw)
	if err != nil {
		return result, err
	}
	observation := readUpstreamConcurrency(platform, metadata)
	result.Limit, result.Status = observation.limit, observation.status
	inventory, err := s.RoutingCapacityAccounts(ctx)
	if err != nil {
		return result, err
	}
	for _, account := range inventory {
		if account.UpstreamID == upstreamID {
			result.Accounts = append(result.Accounts, account)
		}
	}
	return result, nil
}
