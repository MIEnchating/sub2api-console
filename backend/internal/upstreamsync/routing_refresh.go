package upstreamsync

import (
	"context"
	"fmt"

	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

type routingRepository interface {
	routing.Repository
	Mode(context.Context) (string, error)
}

// Manual sync tasks refresh their calculation after all Host leases are released.
// Scheduled SyncAllNow already calculates and applies through the inspection runner.
// This calculation uses the whole inventory to preserve shared and global budgets;
// remote recovery remains subject to the inspection writer and its policy checks.
func (s *Service) refreshRouting(ctx context.Context, scope Scope, batch BatchResult) (*routing.Result, error) {
	if !scope.Balance || batch.Succeeded == 0 {
		return nil, nil
	}
	repository, ok := s.repository.(routingRepository)
	if !ok {
		return nil, nil
	}
	mode, err := repository.Mode(ctx)
	if err != nil {
		return nil, err
	}
	capabilities, valid := runtimepolicy.For(mode)
	if !valid {
		return nil, fmt.Errorf("运行模式无效：%s", mode)
	}
	result, err := routing.NewService(repository).Calculate(ctx, routing.Scope{}, capabilities.PersistRoutingDecisions)
	if err != nil {
		return nil, err
	}
	return &result, nil
}
