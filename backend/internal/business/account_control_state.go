package business

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// AccountControlState shares the account list's effective scheduling state but
// excludes detail, evidence history, bindings, and health score responses.
type AccountControlState struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Health         string  `json:"health"`
	HealthStatus   *string `json:"health_status"`
	RoutingState   *string `json:"routing_state"`
	Paused         *bool   `json:"paused"`
	Schedulable    *bool   `json:"schedulable"`
	ManualPriority *int64  `json:"manual_priority"`
}

func (s *Store) AccountControlState(ctx context.Context, accountID string) (*AccountControlState, error) {
	accountID = strings.TrimSpace(accountID)
	if !positiveNumericID(accountID) {
		return nil, errors.New("账号必须使用有效的稳定 ID")
	}
	projections, err := s.accountProjectionsWithOptions(ctx, accountProjectionOptions{accountID: accountID})
	if err != nil {
		return nil, err
	}
	if len(projections) == 0 {
		return nil, sql.ErrNoRows
	}
	account := projections[0]
	return &AccountControlState{
		ID: account.ID, Name: account.Name, Health: account.Health,
		HealthStatus: account.HealthStatus, RoutingState: account.RoutingState,
		Paused: account.Paused, Schedulable: account.Schedulable, ManualPriority: account.ManualPriority,
	}, nil
}
