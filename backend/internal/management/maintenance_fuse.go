package management

import (
	"context"
	"errors"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

// Called with the account lease held. Repairing connection information must
// not turn a manual or automatic fuse into an implicit recovery operation.
func (s *Service) accountMaintenanceFused(ctx context.Context, accountID string) (bool, error) {
	if repository, ok := s.repository.(mutationProtectionRepository); ok {
		protection, err := repository.AccountMutationProtection(ctx, accountID)
		if err != nil || protection.ManualFused {
			return protection.ManualFused, err
		}
	}
	if repository, ok := s.repository.(interface {
		Account(context.Context, string) (*business.AccountDetail, error)
	}); ok {
		account, err := repository.Account(ctx, accountID)
		if err != nil {
			return false, err
		}
		if account == nil || account.ID != accountID {
			return false, errors.New("无法按稳定账号 ID 核对熔断状态")
		}
		return account.Health == "fused" || account.RoutingState != nil && *account.RoutingState == "fused", nil
	}
	return false, nil
}
