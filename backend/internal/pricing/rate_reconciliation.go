package pricing

import (
	"context"
	"fmt"

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
	value, err := s.buildPlan(ctx, config)
	if err != nil {
		return err
	}
	if err := validateExchangeCatalog(config, value.snapshot.Groups); err != nil {
		return err
	}
	selected := make(map[string]bool, len(accountIDs))
	for _, id := range accountIDs {
		selected[id] = true
	}
	decisions := []Decision{}
	value.snapshot.Accounts, value.snapshot.Changes, value.snapshot.Skipped = 0, 0, 0
	for _, d := range value.snapshot.Decisions {
		if !selected[d.AccountID] {
			continue
		}
		decisions = append(decisions, d)
		value.snapshot.Accounts++
		if d.Skipped {
			value.snapshot.Skipped++
		}
		if d.Changed {
			value.snapshot.Changes++
		}
	}
	value.snapshot.Decisions = decisions
	result, err := s.applyPlan(ctx, value, config, actor)
	if err != nil {
		return err
	}
	if result.Failed > 0 {
		return fmt.Errorf("倍率同步后的价格分组迁移失败 %d 项，请查看价格管理记录", result.Failed)
	}
	return nil
}
