package accountworkbench

import (
	"context"
	"errors"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func (s *Service) DeleteOAuthCheckpoint(ctx context.Context, owner, id string, input OAuthCheckpointAction) error {
	if owner == "" || input.Revision < 1 || !input.Confirmed {
		return errors.New("请明确确认删除所选登录检查点")
	}
	store, err := s.checkpointStore()
	if err != nil {
		return err
	}
	input.Scope, err = normalizeWorkbenchScope(input.Scope)
	if err != nil {
		return err
	}
	ctx, target, err := s.bindWorkbenchScope(ctx, input.Scope, nil)
	if err != nil {
		return err
	}
	record, err := store.WorkbenchOAuthCheckpoint(ctx, exportHash(owner), workbenchScopeFingerprint(input.Scope, target), id)
	if err != nil || record.Revision != input.Revision || record.Scope != string(input.Scope) {
		return configstore.ErrWorkbenchOAuthCheckpoint
	}
	if record.Status == "saving" || record.Status == "restoring" {
		return errors.New("检查点仍在保存或恢复中，请等待任务结束后删除")
	}
	if record.Status != "deleting" {
		record.Status, record.Payload = "deleting", nil
		record, err = store.SaveWorkbenchOAuthCheckpoint(ctx, record)
		if err != nil {
			return err
		}
	}
	if record.WorkerID != "" {
		factory, supported := s.oauthFactory.(browserlogin.OAuthRecoveryFactory)
		if !supported {
			return errors.New("浏览器检查点清理未就绪，请恢复 browser 服务后重试")
		}
		err := factory.DeleteOAuthCheckpoint(ctx, browserlogin.OAuthCheckpointRef{ID: record.WorkerID, Owner: record.OwnerHash, Lease: record.WorkerLease})
		if err != nil && !errors.Is(err, browserlogin.ErrOAuthCheckpoint) {
			return errors.New("浏览器检查点清理未完成，请刷新后重试")
		}
	}
	return store.DeleteWorkbenchOAuthCheckpoint(ctx, record.OwnerHash, record.TargetFingerprint, record.ID, record.Revision)
}
