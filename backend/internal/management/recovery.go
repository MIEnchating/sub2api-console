package management

import (
	"context"
	"errors"

	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type rateRecovery struct {
	AccountIDs []string `json:"account_ids"`
	Actor      string   `json:"actor"`
	Automatic  bool     `json:"automatic"`
	Target     string   `json:"target"`
}

// Rates are recovered by reading current upstream values and comparing current
// management state; an interrupted remote write is never replayed blindly.
func (s *Service) ResumeTask(ctx context.Context, task taskstore.Task) error {
	if task.Operation != "account-rate-sync" {
		return errors.New("此账号维护任务不支持自动恢复")
	}
	var input rateRecovery
	if err := taskstore.RecoveryInput(task, &input); err != nil {
		return err
	}
	ids, err := normalizeAccountIDs(input.AccountIDs)
	if err != nil {
		return err
	}
	target, err := targetguard.Expected(ctx, s.targets)
	if err != nil {
		return err
	}
	if input.Target == "" || targetguard.Fingerprint(target) != input.Target {
		return targetguard.ErrChanged
	}
	if input.Automatic {
		allowed, err := s.automaticRateSyncAllowed(ctx)
		if err != nil {
			return err
		}
		if !allowed {
			return errors.New("自动倍率同步已关闭或运行模式已变化")
		}
	}
	return taskrunner.GoTask(s.taskRunner, task.ID, func(parent context.Context) {
		parent = preserveAutomaticInspection(parent, input.Automatic)
		parent = targetguard.Expect(parent, target)
		s.executeMaintenanceContext(parent, task, task.Operation, ids, input.Actor)
	})
}
