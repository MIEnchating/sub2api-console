package routingwrite

import (
	"context"
	"errors"
	"fmt"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func (s *Service) restoreBalanceAccount(ctx context.Context, repository balanceRecoveryRepository, admin Admin, state business.BalanceRecoveryState, payload map[string]any, current values, actor string, remoteWrite *bool) (resultErr error) {
	operationID, err := randomOperationID("balance-recovery")
	if err != nil {
		return err
	}
	operation := business.AccountOperation{OperationID: operationID, OperationType: "account.balance_recovery", ObjectID: state.AccountID, Actor: actor, State: "pending", Phase: "balance-recovery", Before: current.asMap(), After: map[string]any{"schedulable": state.Schedulable, "status": "active"}}
	if err := s.repository.RecordAccountOperation(ctx, operation); err != nil {
		return err
	}
	defer func() {
		*remoteWrite = *remoteWrite || operation.Writeback
		if resultErr != nil {
			reason := resultErr.Error()
			operation.State, operation.Error = "failed", &reason
			resultErr = errors.Join(resultErr, s.repository.RecordAccountOperation(ctx, operation))
		}
	}()
	state.Started = true
	if err := repository.SaveBalanceRecoveryState(ctx, state); err != nil {
		return err
	}
	if balanceForbidden(payload) {
		operation.Writeback = true
		if _, err := admin.Mutate(ctx, "POST", "/admin/accounts/"+state.AccountID+"/clear-error", nil); err != nil {
			return err
		}
	}
	afterPayload, err := admin.Account(ctx, state.AccountID)
	if err != nil {
		return err
	}
	fingerprint, err := balanceRemoteFingerprint(afterPayload)
	if err != nil {
		return err
	}
	if fingerprint != state.RemoteFingerprint {
		return errors.New("清除错误期间账号绑定已变化，已停止恢复调度")
	}
	after, err := remoteValues(afterPayload)
	if err != nil {
		return err
	}
	if fmt.Sprint(afterPayload["id"]) != state.AccountID || after.status == nil || *after.status != "active" || after.schedulable == nil {
		return errors.New("清除余额错误后账号状态未确认，请重试余额同步")
	}
	if message, _ := afterPayload["error_message"].(string); message != "" {
		return errors.New("账号仍有错误信息，已停止恢复调度")
	}
	if *after.schedulable != state.Schedulable {
		operation.Writeback = true
		if err := writeSchedulable(ctx, admin, state.AccountID, state.Schedulable); err != nil {
			return err
		}
		afterPayload, err = admin.Account(ctx, state.AccountID)
		if err != nil {
			return err
		}
		after, err = remoteValues(afterPayload)
		if err != nil {
			return err
		}
	}
	if fmt.Sprint(afterPayload["id"]) != state.AccountID || after.schedulable == nil || *after.schedulable != state.Schedulable || after.status == nil || *after.status != "active" {
		return errors.New("余额恢复后的调度状态读回不一致，请重试余额同步")
	}
	fingerprint, err = balanceRemoteFingerprint(afterPayload)
	if err != nil {
		return err
	}
	message, _ := afterPayload["error_message"].(string)
	if fingerprint != state.RemoteFingerprint || message != "" {
		return errors.New("恢复后账号绑定或错误状态已变化，未确认恢复成功")
	}
	operation.State, operation.RemoteConfirmed, operation.ReadbackConfirmed = "succeeded", true, true
	operation.After = after.asMap()
	return repository.CommitBalanceRecovery(ctx, state, operation)
}
