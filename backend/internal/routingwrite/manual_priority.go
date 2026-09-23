package routingwrite

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

type manualOrderRepository interface {
	RoutingAccounts(context.Context, *string, *string) ([]business.RoutingAccount, error)
	RoutingSamples(context.Context, *string, *string, string, int) ([]business.RoutingSample, error)
	ManualPriorityConfig(context.Context) (business.ManualPriorityConfig, error)
	CommitManualPriorityReadback(context.Context, string, int64, int64, int64, business.AccountOperation) error
}

// ReorderManualPriorities is a separate, narrowly authorized write path. Ordinary
// routing still refuses manual accounts, even while this option is enabled.
func (s *Service) ReorderManualPriorities(ctx context.Context, actor string) (Result, error) {
	result := Result{Results: []AccountResult{}}
	repository, ok := s.repository.(manualOrderRepository)
	if !ok {
		return result, nil
	}
	config, err := repository.ManualPriorityConfig(ctx)
	if err != nil || !config.LatencyPriorityEnabled {
		return result, err
	}
	mode, err := s.repository.Mode(ctx)
	result.Mode = mode
	if err != nil {
		return result, err
	}
	if mode != runtimepolicy.Full {
		result.CalculationOnly = true
		return result, nil
	}
	policy, err := s.repository.ControlPolicy(ctx)
	if err != nil {
		return result, err
	}
	accounts, err := repository.RoutingAccounts(ctx, nil, nil)
	if err != nil {
		return result, err
	}
	resources := []string{mutationguard.AccountCatalog()}
	for _, account := range accounts {
		if account.ManualPriority != nil {
			resources = append(resources, mutationguard.Account(account.ID))
		}
	}
	if len(resources) == 1 {
		return result, nil
	}
	if s.admin == nil {
		ctx, err = targetguard.Capture(ctx, s.targets)
		if err != nil {
			return result, err
		}
	}
	var release func() error
	if s.admin == nil {
		ctx, release, err = targetguard.Acquire(ctx, s.repository, resources...)
	} else {
		ctx, release, err = mutationguard.Acquire(ctx, s.repository, resources...)
	}
	if err != nil {
		return result, err
	}
	defer release()
	if s.admin == nil {
		ctx, err = targetguard.Bind(ctx, s.targets)
		if err != nil {
			return result, err
		}
	}
	latest, err := repository.RoutingAccounts(ctx, nil, nil)
	if err != nil {
		return result, err
	}
	if !reflect.DeepEqual(accounts, latest) {
		return result, errors.New("等待重排期间账号或分组已变化，请下轮重新计算")
	}
	authorize := func(checkCtx context.Context) error {
		mode, err := s.recheckWriteAuthorization(checkCtx, policy)
		if err != nil {
			return err
		}
		if mode != runtimepolicy.Full {
			return errors.New("手动控制优先级重排仅允许完全模式")
		}
		return nil
	}
	if err := authorize(ctx); err != nil {
		return result, err
	}
	// RoutingSamples returns bounded, de-duplicated persisted evidence. Explicitly
	// use traffic: manual control never authorizes an automatic active probe.
	samples, err := repository.RoutingSamples(ctx, nil, nil, "traffic", 200)
	if err != nil {
		return result, err
	}
	changes, err := routing.PlanManualPriorityOrder(policy, latest, samples, s.now().UTC())
	if err != nil || len(changes) == 0 {
		return result, err
	}
	admin, err := s.adminClient(ctx)
	if err != nil {
		return result, err
	}
	ctx = adminclient.WithMutationAuthorization(ctx, authorize)
	// Validate the complete remote range before starting a permutation.
	for _, change := range changes {
		if _, err := readManualPriority(ctx, admin, change); err != nil {
			return result, err
		}
	}
	for _, change := range changes {
		item := s.applyManualPriority(ctx, admin, repository, change, actor)
		result.Results = append(result.Results, item)
		result.RemoteWrite = result.RemoteWrite || item.RemoteWrite
		if item.Error != nil {
			result.Failed++
			break
		}
		result.Succeeded++
		if item.Changed {
			result.Changed++
		}
	}
	return result, nil
}

func readManualPriority(ctx context.Context, admin Admin, change routing.ManualPriorityChange) (values, error) {
	raw, err := admin.Account(ctx, change.AccountID)
	if err != nil {
		return values{}, err
	}
	if fmt.Sprint(raw["id"]) != change.AccountID {
		return values{}, errors.New("手动控制账号稳定 ID 读回不一致")
	}
	value, err := remoteValues(raw)
	if err != nil {
		return values{}, err
	}
	if value.priority == nil || *value.priority != change.BeforePriority || value.schedulable == nil || !*value.schedulable {
		return values{}, fmt.Errorf("账号 %s 的优先级或调度状态已变化，请同步后重排", change.AccountID)
	}
	return value, nil
}

func (s *Service) applyManualPriority(ctx context.Context, admin Admin, repository manualOrderRepository, change routing.ManualPriorityChange, actor string) AccountResult {
	result := AccountResult{AccountID: change.AccountID, Desired: map[string]any{"priority": change.Priority}}
	id, err := randomOperationID("manual-priority-order")
	if err != nil {
		return failedResult(result, err)
	}
	before, err := readManualPriority(ctx, admin, change)
	if err != nil {
		return failedResult(result, err)
	}
	result.Before = before.asMap()
	target := business.AccountRoutingTarget{AccountID: change.AccountID, GroupNames: change.Groups, DesiredHealth: "manual_priority", Priority: &change.Priority}
	op := operation(id, "account.manual_priority.reorder", target, actor, result.Before, result.Desired, false, false, nil)
	field := "priority"
	op.FieldName = &field
	_, err = admin.Mutate(ctx, "PUT", "/admin/accounts/"+change.AccountID, result.Desired)
	result.RemoteWrite = !mutationPrevented(err)
	if err == nil {
		var raw map[string]any
		raw, err = admin.Account(ctx, change.AccountID)
		if err == nil {
			var after values
			after, err = remoteValues(raw)
			if err == nil && (fmt.Sprint(raw["id"]) != change.AccountID || after.priority == nil || *after.priority != change.Priority) {
				err = errors.New("手动控制优先级写入后读回不一致")
			}
			if err == nil {
				result.Effective = after.asMap()
				op.RemoteConfirmed, op.ReadbackConfirmed = true, true
				op.After = result.Effective
				err = repository.CommitManualPriorityReadback(ctx, change.AccountID, change.ReservedPriority, change.BeforePriority, change.Priority, op)
			}
		}
	}
	if err != nil {
		failed := operation(id, "account.manual_priority.reorder", target, actor, result.Before, result.Desired, result.RemoteWrite, false, err)
		failed.FieldName = &field
		s.recordOperation(ctx, failed)
		return failedResult(result, err)
	}
	result.Changed = true
	return result
}
