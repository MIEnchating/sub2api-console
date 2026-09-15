package accountworkbench

import (
	"context"
	"errors"

	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

func (s *Service) batchExportAccounts(ctx context.Context, taskID string) ([]string, map[string]string, error) {
	if err := s.requireFinishedImport(ctx, taskID); err != nil {
		return nil, nil, err
	}
	store, ok := s.private.(executionStore)
	if !ok {
		return nil, nil, errors.New("本批执行记录不可用")
	}
	source, err := store.WorkbenchExecution(ctx, taskID)
	if err != nil {
		return nil, nil, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return nil, nil, err
	}
	if source.TargetURL != target.BaseURL || source.TargetFingerprint != executionTargetFingerprint(target) {
		return nil, nil, errors.New("管理目标已变化，不能导出原批次账号")
	}
	ids := []string{}
	identities := map[string]string{}
	for _, item := range source.Items {
		if item.AccountID == "" || item.Identity == "" {
			continue
		}
		if previous, exists := identities[item.AccountID]; exists {
			if previous != item.Identity {
				return nil, nil, errors.New("本批账号身份记录冲突，请核对原任务")
			}
			continue
		}
		ids = append(ids, item.AccountID)
		identities[item.AccountID] = item.Identity
	}
	return ids, identities, nil
}
