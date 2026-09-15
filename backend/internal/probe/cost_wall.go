package probe

import (
	"context"
	"fmt"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

// AutomaticCostWallBlocks is shared by inspection planning and execution.
func AutomaticCostWallBlocks(ctx context.Context, repository any, policy map[string]any) (map[string]bool, error) {
	reader, ok := repository.(interface {
		RoutingAccounts(context.Context, *string, *string) ([]business.RoutingAccount, error)
	})
	if !ok {
		return nil, nil
	}
	accounts, err := reader.RoutingAccounts(ctx, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("自动探活前读取成本墙账号失败：%w", err)
	}
	return routing.CostWallProbeBlocks(accounts, policy)
}

func (s *Service) applyCostWallProtections(ctx context.Context, prepared *preparedRun) error {
	if !prepared.request.Automatic && len(prepared.request.AccountIDs) == 0 {
		return nil
	}
	policy, err := s.repository.ControlPolicy(ctx)
	if err != nil {
		return err
	}
	blocked, err := AutomaticCostWallBlocks(ctx, s.repository, policy)
	if err != nil {
		return err
	}
	for index := range prepared.targets {
		target := &prepared.targets[index]
		if blocked[target.AccountID] && target.SkipReason == nil {
			target.SkipReason = textPointer("倍率达到或超过成本墙，已停止自动探活；仅已启用的保底账号恢复自动探活")
		}
	}
	return nil
}
