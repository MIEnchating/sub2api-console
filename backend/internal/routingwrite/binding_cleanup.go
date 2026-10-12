package routingwrite

import (
	"context"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func (s *Service) checkMissingBindingCleanup(ctx context.Context, target business.AccountRoutingTarget) error {
	if target.CleanupReason != business.MissingBindingCleanupReason {
		return nil
	}
	policy, err := s.repository.ControlPolicy(ctx)
	if err != nil {
		return err
	}
	section, _ := policy["binding_cleanup"].(map[string]any)
	if section["enabled"] != true {
		return &writeAuthorizationChanged{reason: "失效绑定账号自动删除已关闭"}
	}
	reader, ok := s.repository.(interface {
		ConfirmedMissingBindings(context.Context, *string) (map[string]string, error)
	})
	if !ok {
		return errors.New("失效绑定复核不可用，已阻止自动删除")
	}
	bindings, err := reader.ConfirmedMissingBindings(ctx, &target.AccountID)
	if err != nil {
		return err
	}
	if target.MissingBindingFingerprint == "" || bindings[target.AccountID] != target.MissingBindingFingerprint {
		return &writeAuthorizationChanged{reason: "账号绑定或上游目录已变化，已取消自动删除"}
	}
	return nil
}
