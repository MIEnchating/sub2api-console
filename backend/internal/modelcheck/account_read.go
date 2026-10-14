package modelcheck

import (
	"context"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

// Production stores supply a credential snapshot without display history. Keep
// the detail adapter for catalogs that only implement the public AccountCatalog.
func (s *Service) detectionAccount(ctx context.Context, id string) (*business.AccountDetail, error) {
	reader, ok := s.accounts.(interface {
		ProbeAccountBinding(context.Context, string) (*business.ProbeAccountBinding, error)
	})
	if !ok {
		return s.accounts.Account(ctx, id)
	}
	value, err := reader.ProbeAccountBinding(ctx, id)
	if err != nil || value == nil {
		return nil, err
	}
	return &business.AccountDetail{
		AccountStatus: business.AccountStatus{
			ID: value.ID, Name: value.Name, Platform: value.Platform,
			AccountType: value.AccountType, BaseURL: value.BaseURL, UpstreamBaseURL: value.UpstreamBaseURL,
		},
		Bindings: value.Bindings,
	}, nil
}
