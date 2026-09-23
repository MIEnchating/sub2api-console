package routingwrite

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

type manualCostRepository interface {
	RoutingAccounts(context.Context, *string, *string) ([]business.RoutingAccount, error)
	CommitManualCostWallReadback(context.Context, string, int64, bool, bool, string, business.AccountOperation) error
}

func (s *Service) EnforceManualCostWall(ctx context.Context, accountIDs []string, actor string) (Result, error) {
	result := Result{Results: []AccountResult{}}
	repository, ok := s.repository.(manualCostRepository)
	if !ok {
		return result, nil
	}
	mode, err := s.repository.Mode(ctx)
	result.Mode = mode
	if err != nil || mode != runtimepolicy.Full {
		result.CalculationOnly = true
		return result, err
	}
	accounts, err := repository.RoutingAccounts(ctx, nil, nil)
	if err != nil {
		return result, err
	}
	selected := map[string]bool{}
	for _, id := range accountIDs {
		selected[id] = true
	}
	resources := []string{mutationguard.AccountCatalog()}
	for _, a := range accounts {
		if a.ManualPriority != nil && (len(accountIDs) == 0 || selected[a.ID]) {
			resources = append(resources, mutationguard.Account(a.ID))
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
		return result, errors.New("成本墙检查等待期间账号或分组已变化，请重新计算")
	}
	policy, err := s.repository.ControlPolicy(ctx)
	if err != nil {
		return result, err
	}
	changes, err := routing.PlanManualCostWall(policy, latest, s.now().UTC())
	if err != nil {
		return result, err
	}
	authorize := func(checkCtx context.Context) error {
		mode, err := s.recheckWriteAuthorization(checkCtx, policy)
		if err != nil {
			return err
		}
		if mode != runtimepolicy.Full {
			return errors.New("成本墙写回需要完全模式")
		}
		return nil
	}
	if err := authorize(ctx); err != nil {
		return result, err
	}
	ctx = adminclient.WithMutationAuthorization(ctx, authorize)
	for _, change := range changes {
		if len(accountIDs) > 0 && !selected[change.Account.ID] {
			continue
		}
		admin, err := s.adminClient(ctx)
		if err != nil {
			return result, err
		}
		item := s.applyManualCostWall(ctx, admin, repository, change, actor)
		result.Results = append(result.Results, item)
		result.RemoteWrite = result.RemoteWrite || item.RemoteWrite
		if item.Error != nil {
			result.Failed++
		} else {
			result.Succeeded++
			if item.Changed {
				result.Changed++
			}
		}
	}
	return result, nil
}

func manualCostRemoteMatches(raw map[string]any, change routing.ManualCostWallChange, enabled bool) error {
	if fmt.Sprint(raw["id"]) != change.Account.ID || raw["schedulable"] != enabled {
		return errors.New("账号稳定 ID 或调度开关与成本墙计划不一致")
	}
	rate, err := decimal(raw["rate_multiplier"])
	if err != nil || change.Account.Multiplier == nil {
		return errors.New("账号成本倍率无法复核")
	}
	expected, err := decimal(*change.Account.Multiplier)
	if err != nil || rate.Cmp(expected) != 0 {
		return errors.New("账号倍率已变化，请重新计算成本墙")
	}
	rawGroups, ok := raw["group_ids"].([]any)
	if !ok {
		return errors.New("账号分组稳定 ID 无法复核")
	}
	groups := make([]string, 0, len(rawGroups))
	for _, id := range rawGroups {
		groups = append(groups, fmt.Sprint(id))
	}
	sort.Strings(groups)
	if !reflect.DeepEqual(groups, change.GroupIDs) {
		return errors.New("账号分组已变化，请重新计算成本墙")
	}
	return nil
}

func (s *Service) applyManualCostWall(ctx context.Context, admin Admin, repository manualCostRepository, change routing.ManualCostWallChange, actor string) AccountResult {
	result := AccountResult{AccountID: change.Account.ID, Desired: map[string]any{"schedulable": change.Schedulable}}
	raw, err := admin.Account(ctx, result.AccountID)
	if err != nil {
		return failedResult(result, err)
	}
	if err := manualCostRemoteMatches(raw, change, *change.Account.Schedulable); err != nil {
		return failedResult(result, err)
	}
	result.Before = map[string]any{"schedulable": *change.Account.Schedulable}
	id, err := randomOperationID("manual-cost-wall")
	if err != nil {
		return failedResult(result, err)
	}
	target := business.AccountRoutingTarget{AccountID: result.AccountID, GroupNames: change.GroupNames, DesiredHealth: change.State, Schedulable: &change.Schedulable}
	err = writeSchedulable(ctx, admin, result.AccountID, change.Schedulable)
	result.RemoteWrite = !mutationPrevented(err)
	if err == nil {
		raw, err = admin.Account(ctx, result.AccountID)
		if err == nil {
			err = manualCostRemoteMatches(raw, change, change.Schedulable)
		}
		if err == nil {
			result.Effective = map[string]any{"schedulable": change.Schedulable}
			op := operation(id, "account.manual_cost_wall", target, actor, result.Before, result.Effective, true, true, nil)
			field := "schedulable"
			op.FieldName = &field
			err = repository.CommitManualCostWallReadback(ctx, result.AccountID, *change.Account.ManualPriority, *change.Account.Schedulable, change.Schedulable, change.State, op)
		}
	}
	if err != nil {
		s.recordOperation(ctx, operation(id, "account.manual_cost_wall", target, actor, result.Before, result.Desired, result.RemoteWrite, false, err))
		return failedResult(result, err)
	}
	result.Changed = true
	return result
}
