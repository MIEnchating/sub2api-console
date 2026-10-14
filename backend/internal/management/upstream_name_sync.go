package management

import (
	"context"
	"fmt"
)

func (s *Service) hostNameSyncAccounts(ctx context.Context, host string) ([]string, error) {
	allowed, err := s.automaticRateSyncAllowed(ctx)
	if err != nil || !allowed {
		return nil, err
	}
	ids, err := s.maintenanceAccountIDsForHost(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("读取上游绑定账号失败：%w", err)
	}
	ids, err = s.managedMaintenanceAccounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	ids, _, err = s.excludeManualPriorityAccounts(ctx, ids)
	return ids, err
}

// SyncHostAccountNames joins the parent upstream task so completion includes
// confirmed account names instead of reporting success before writeback.
func (s *Service) SyncHostAccountNames(ctx context.Context, host, actor string) (bool, error) {
	ids, err := s.hostNameSyncAccounts(ctx, host)
	if err != nil || len(ids) == 0 {
		return false, err
	}
	result, err := s.repairAccountNames(ctx, ids, actor)
	written, _ := result["remote_write"].(bool)
	if err == nil && resultInteger(result, "failed") > 0 {
		err = fmt.Errorf("账号名称同步失败 %d 项，请在操作审计中查看原因并重试名称修复", resultInteger(result, "failed"))
	}
	return written, err
}

func (s *Service) EnqueueHostAccountNameSync(ctx context.Context, host, actor string) (string, error) {
	ids, err := s.hostNameSyncAccounts(ctx, host)
	if err != nil || len(ids) == 0 {
		return "", err
	}
	task, err := s.EnqueueAccountNameRepair(ctx, ids, actor)
	return task.ID, err
}
