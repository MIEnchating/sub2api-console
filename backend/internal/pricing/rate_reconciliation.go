package pricing

import (
	"context"
	"fmt"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

// ReconcileAccountRates runs the configured exchange sets immediately after a
// confirmed rate synchronization. Only the confirmed accounts may be migrated.
func (s *Service) ReconcileAccountRates(ctx context.Context, accountIDs []string, actor string) error {
	if len(accountIDs) == 0 {
		return nil
	}
	reader, ok := s.repository.(interface {
		Mode(context.Context) (string, error)
	})
	if !ok {
		return fmt.Errorf("价格管理运行模式读取服务不可用")
	}
	mode, err := reader.Mode(ctx)
	if err != nil || mode != runtimepolicy.Full {
		return err
	}
	ctx, err = targetguard.Capture(ctx, s.targets)
	if err != nil {
		return err
	}
	policy, err := s.repository.ControlPolicy(ctx)
	if err != nil {
		return err
	}
	config, err := ConfigFromPolicy(policy)
	if err != nil || !config.Enabled {
		return err
	}
	repository, ok := s.repository.(interface {
		PricingCatalogForAccounts(context.Context, []string) (business.PricingCatalog, error)
	})
	if !ok {
		return fmt.Errorf("价格管理范围目录读取服务不可用")
	}
	catalog, err := repository.PricingCatalogForAccounts(ctx, accountIDs)
	if err != nil {
		return fmt.Errorf("Console 本地价格目录读取失败：%w", err)
	}
	snapshot, err := evaluate(config, catalog)
	if err != nil {
		return err
	}
	if err := validateExchangeCatalog(config, snapshot.Groups); err != nil {
		return err
	}
	result, err := s.applyPlan(ctx, plan{snapshot: snapshot}, config, actor)
	if err != nil {
		return err
	}
	if result.Failed > 0 {
		return fmt.Errorf("倍率同步后的价格分组迁移失败 %d 项，请查看价格管理记录", result.Failed)
	}
	return nil
}
