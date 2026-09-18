package routingwrite

import (
	"context"
	"errors"
	"fmt"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

// A known configuration change invalidates the old plan. Storage and API
// errors must remain failures, even when they also prevent a remote write.
type writeAuthorizationChanged struct{ reason string }

func mutationPrevented(err error) bool {
	var denied *adminclient.MutationAuthorizationError
	return errors.As(err, &denied) && !denied.Attempted
}

func (change *writeAuthorizationChanged) Error() string {
	return change.reason + "；本轮未执行，等待按最新策略重新计算调度"
}

func (s *Service) skipAuthorizationChange(ctx context.Context, result AccountResult, target business.AccountRoutingTarget, operationID, actor string, change *writeAuthorizationChanged) AccountResult {
	if cause := contextCause(ctx); cause != nil {
		return failedResult(result, cause)
	}
	reason := change.Error()
	kind := operationType(target.ReleaseControl)
	if target.CleanupAction != nil && *target.CleanupAction == "delete" {
		kind = "cleanup.delete"
	}
	op := operation(operationID, kind, target, actor, result.Before, map[string]any{
		"desired": result.Desired, "reason": reason,
	}, false, false, nil)
	op.State, op.FieldName = "skipped", nil
	if err := s.repository.RecordAccountOperation(ctx, op); err != nil {
		return failedResult(result, fmt.Errorf("保存调度跳过记录失败：%w", err))
	}
	result.Skipped, result.Reason, result.Effective = true, &reason, result.Before
	return result
}

func (s *Service) skipUnauthorizedTargets(ctx context.Context, result Result, targets map[string]business.AccountRoutingTarget, orderedIDs []string, actor string, change *writeAuthorizationChanged) Result {
	for _, accountID := range orderedIDs {
		item := AccountResult{AccountID: accountID}
		operationID, err := randomOperationID("routing-writeback")
		if err != nil {
			item = failedResult(item, err)
		} else {
			item = s.skipAuthorizationChange(ctx, item, targets[accountID], operationID, actor, change)
		}
		result.Results = append(result.Results, item)
		if item.Error != nil {
			result.Failed++
		} else {
			result.Succeeded++
		}
	}
	reason := change.Error()
	result.Reason = &reason
	return result
}
