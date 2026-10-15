package routingwrite

import (
	"context"
	"errors"
	"reflect"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountdelete"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
)

// UseAccountDeletion shares the verified two-sided deletion workflow with manual deletion.
func (s *Service) UseAccountDeletion(service *accountdelete.Service) { s.accountDeletion = service }

type cleanupDeletePlan struct {
	preview accountdelete.Preview
	err     error
}
type cleanupDeletePlansKey struct{}

func (s *Service) prepareCleanupDeletion(ctx context.Context, targets map[string]business.AccountRoutingTarget) (context.Context, []string) {
	plans := map[string]cleanupDeletePlan{}
	resources := []string{}
	for id, target := range targets {
		if target.CleanupAction == nil || *target.CleanupAction != "delete" || !cleanupDeletesUpstreamKey(target) {
			continue
		}
		plan := s.previewCleanupDeletion(ctx, id)
		plans[id] = plan
		if plan.err != nil || plan.preview.Binding == nil {
			continue
		}
		binding := plan.preview.Binding
		resources = append(resources, mutationguard.ManagementTarget(), mutationguard.Upstream(binding.AuthHost), mutationguard.Upstream(binding.UpstreamHost))
	}
	return context.WithValue(ctx, cleanupDeletePlansKey{}, plans), resources
}

func cleanupDeletesUpstreamKey(target business.AccountRoutingTarget) bool {
	return target.CleanupReason == "长期异常" || target.CleanupReason == business.MissingBindingCleanupReason
}

type cleanupAccountReader interface {
	Account(context.Context, string) (*business.AccountDetail, error)
}

func (s *Service) previewCleanupDeletion(ctx context.Context, accountID string) cleanupDeletePlan {
	reader, ok := s.repository.(cleanupAccountReader)
	if !ok {
		return cleanupDeletePlan{err: errors.New("账号绑定读取不可用，已阻止自动删除")}
	}
	account, err := reader.Account(ctx, accountID)
	if err != nil {
		return cleanupDeletePlan{err: err}
	}
	if len(account.Bindings) == 0 {
		return cleanupDeletePlan{}
	}
	if s.accountDeletion == nil {
		return cleanupDeletePlan{err: errors.New("上游 Key 删除服务不可用，已阻止自动删除账号")}
	}
	preview, err := s.accountDeletion.Preview(ctx, accountID)
	return cleanupDeletePlan{preview: preview, err: err}
}

func (s *Service) cleanupDeletionPlan(ctx context.Context, accountID string) (cleanupDeletePlan, error) {
	plans, _ := ctx.Value(cleanupDeletePlansKey{}).(map[string]cleanupDeletePlan)
	plan, found := plans[accountID]
	if !found {
		return plan, errors.New("缺少账号删除范围，已阻止自动删除")
	}
	if plan.err != nil {
		return plan, plan.err
	}
	// Recheck before removing traffic as well as before deleting the Key.
	if plan.preview.Binding != nil {
		current, err := s.accountDeletion.Preview(ctx, accountID)
		if err != nil {
			return plan, err
		}
		if !reflect.DeepEqual(current.Binding, plan.preview.Binding) {
			return plan, &writeAuthorizationChanged{reason: "账号上游 Key 绑定已变化，已取消自动删除"}
		}
		return plan, nil
	}
	// A binding added while waiting for the reservation must never use account-only deletion.
	if plan.preview.Binding == nil {
		reader, ok := s.repository.(cleanupAccountReader)
		if !ok {
			return plan, errors.New("账号绑定读取不可用，已阻止自动删除")
		}
		account, err := reader.Account(ctx, accountID)
		if err != nil {
			return plan, err
		}
		if len(account.Bindings) > 0 {
			return plan, errors.New("账号绑定已变化，请重新计算自动删除范围")
		}
	}
	return plan, nil
}

func (s *Service) deleteBoundCleanupAccount(ctx context.Context, plan cleanupDeletePlan, target business.AccountRoutingTarget, actor, operationID string, before, payload map[string]any, result AccountResult) AccountResult {
	deleted, err := s.accountDeletion.Delete(ctx, plan.preview, actor)
	result.RemoteWrite = result.RemoteWrite || deleted.UpstreamKeyDeleteRequested || deleted.ManagementAccountDeleteRequested
	result.Effective = map[string]any{
		"deleted":                       deleted.LocalProjectionDeleted,
		"upstream_key_deleted":          deleted.UpstreamKeyDeleted,
		"upstream_key_already_absent":   deleted.UpstreamKeyAlreadyAbsent,
		"upstream_key_delete_requested": deleted.UpstreamKeyDeleteRequested,
		"management_account_deleted":    deleted.ManagementAccountDeleted,
	}
	payload["result"] = result.Effective
	if err != nil && !result.RemoteWrite && mutationPrevented(err) {
		var change *writeAuthorizationChanged
		if errors.As(err, &change) {
			return s.skipAuthorizationChange(ctx, result, target, operationID, actor, change)
		}
	}
	op := operation(operationID, "cleanup.delete", target, actor, before, result.Effective, deleted.ManagementAccountDeleted, deleted.ReadbackConfirmed, err)
	field := "deleted"
	op.FieldName = &field
	s.recordOperation(ctx, op)
	if err != nil {
		payload["error"] = err.Error()
		s.recordRuntimeEvent(ctx, "cleanup_delete_failed", "failed", "自动删除账号及上游 Key 失败 "+target.AccountID, payload)
		return failedResult(result, err)
	}
	s.recordRuntimeEvent(ctx, "cleanup_deleted", "succeeded", "账号 "+target.AccountID+" 及绑定的上游 Key 已自动删除；删除后无法据此重建", payload)
	result.Changed = true
	return result
}
