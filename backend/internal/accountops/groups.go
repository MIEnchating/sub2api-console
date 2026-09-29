package accountops

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type accountGroupsRepository interface {
	PricingCatalog(context.Context) (business.PricingCatalog, error)
	CommitAccountGroups(context.Context, string, []string, []string, business.AccountOperation) error
}

type AccountGroupOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type AccountGroupsPreview struct {
	AccountID       string               `json:"account_id"`
	CurrentGroupIDs []string             `json:"current_group_ids"`
	Groups          []AccountGroupOption `json:"groups"`
	TargetVersion   string               `json:"target_version"`
}

type AccountGroupsInput struct {
	GroupIDs         []string `json:"group_ids"`
	ExpectedGroupIDs []string `json:"expected_group_ids"`
	TargetVersion    string   `json:"target_version"`
}

func (s *Service) AccountGroups(ctx context.Context, accountID string) (AccountGroupsPreview, error) {
	if !stableID(accountID) {
		return AccountGroupsPreview{}, errors.New("账号必须使用有效的稳定 ID")
	}
	mode, local, err := s.localAccount(ctx, accountID)
	if err != nil {
		return AccountGroupsPreview{}, err
	}
	if local.GroupsLocked {
		return AccountGroupsPreview{}, business.ErrAccountGroupsLocked
	}
	if local.ManualPriority != nil {
		return AccountGroupsPreview{}, errors.New("账号处于手动控制，请先取消手动控制再切换分组")
	}
	if mode != runtimepolicy.Full {
		return AccountGroupsPreview{}, errors.New("切换分组需要完全模式")
	}
	repository, ok := s.repository.(accountGroupsRepository)
	if !ok {
		return AccountGroupsPreview{}, errors.New("账号分组服务尚未就绪")
	}
	target, err := s.targets.TargetSettings(ctx)
	if err != nil {
		return AccountGroupsPreview{}, err
	}
	catalog, err := repository.PricingCatalog(ctx)
	if err != nil {
		return AccountGroupsPreview{}, err
	}
	for _, account := range catalog.Accounts {
		if account.ID != accountID {
			continue
		}
		if !account.GroupsValid || account.Platform == "" {
			return AccountGroupsPreview{}, errors.New("账号平台或分组信息不完整，请先同步管理平台目录")
		}
		result := AccountGroupsPreview{AccountID: accountID, CurrentGroupIDs: append([]string{}, account.GroupIDs...), Groups: []AccountGroupOption{}, TargetVersion: targetguard.Fingerprint(target)}
		for _, group := range catalog.Groups {
			if business.AccountPlatformCanJoinGroup(account.Platform, group.Platform) {
				result.Groups = append(result.Groups, AccountGroupOption{ID: group.ID, Name: group.Name})
			}
		}
		return result, nil
	}
	return AccountGroupsPreview{}, errors.New("账号已不存在，请刷新后重试")
}

func validateGroupChange(preview AccountGroupsPreview, input AccountGroupsInput) error {
	if input.TargetVersion != preview.TargetVersion {
		return errors.New("管理目标已变化，请重新打开分组选择")
	}
	if input.ExpectedGroupIDs == nil || !sameAccountGroups(input.ExpectedGroupIDs, preview.CurrentGroupIDs) {
		return errors.New("账号分组已变化，请重新打开分组选择")
	}
	if len(input.GroupIDs) == 0 || len(input.GroupIDs) > 50 {
		return errors.New("请选择 1 到 50 个目标分组")
	}
	seen := map[string]bool{}
	for _, id := range input.GroupIDs {
		number, err := strconv.ParseInt(id, 10, 64)
		if err != nil || number <= 0 || strconv.FormatInt(number, 10) != id || seen[id] {
			return errors.New("目标分组必须使用有效且不重复的稳定 ID")
		}
		seen[id] = true
		if !slices.ContainsFunc(preview.Groups, func(group AccountGroupOption) bool { return group.ID == id }) {
			return fmt.Errorf("目标分组 %s 已不存在或与账号平台不兼容，请同步目录后重试", id)
		}
	}
	if sameAccountGroups(input.GroupIDs, preview.CurrentGroupIDs) {
		return errors.New("目标分组与当前分组相同，无需切换")
	}
	return nil
}

func sameAccountGroups(left, right []string) bool {
	left, right = slices.Clone(left), slices.Clone(right)
	slices.Sort(left)
	slices.Sort(right)
	return slices.Equal(left, right)
}

func (s *Service) EnqueueGroups(ctx context.Context, accountID string, input AccountGroupsInput, actor string) (taskstore.Task, error) {
	preview, err := s.AccountGroups(ctx, accountID)
	if err != nil {
		return taskstore.Task{}, err
	}
	if err := validateGroupChange(preview, input); err != nil {
		return taskstore.Task{}, err
	}
	target, err := s.targets.TargetSettings(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	if targetguard.Fingerprint(target) != input.TargetVersion {
		return taskstore.Task{}, targetguard.ErrChanged
	}
	input.GroupIDs, input.ExpectedGroupIDs = slices.Clone(input.GroupIDs), slices.Clone(input.ExpectedGroupIDs)
	return s.enqueue(ctx, "sub2api-account-groups", "account-groups", "账号切换分组已排队", func(run context.Context) (map[string]any, error) {
		return s.applyGroups(targetguard.Expect(run, target), accountID, input, actor)
	})
}

func (s *Service) applyGroups(ctx context.Context, accountID string, input AccountGroupsInput, actor string) (map[string]any, error) {
	ctx, release, err := s.acquireAccountMutation(ctx, accountID)
	if err != nil {
		return nil, err
	}
	defer release()
	preview, err := s.AccountGroups(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if err := validateGroupChange(preview, input); err != nil {
		return nil, err
	}
	ctx, err = targetguard.Bind(ctx, s.targets)
	if err != nil {
		return nil, err
	}
	client, err := s.adminClient(ctx)
	if err != nil {
		return nil, err
	}
	before, err := client.Account(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if fmt.Sprint(before["id"]) != accountID || before["group_ids"] == nil {
		return nil, errors.New("远端账号 ID 或分组信息不可确认，请同步后重试")
	}
	current, err := poolModeGroupIDs(before["group_ids"])
	if err != nil {
		return nil, err
	}
	for _, id := range current {
		if value, parseErr := strconv.ParseInt(id, 10, 64); parseErr != nil || value <= 0 {
			return nil, errors.New("当前分组 ID 超出支持范围，请同步目录后重试")
		}
	}
	if !sameAccountGroups(current, input.ExpectedGroupIDs) {
		return nil, errors.New("管理平台账号分组已变化，请同步目录后重新选择")
	}
	platform, _ := before["platform"].(string)
	remoteGroups, err := client.Groups(ctx)
	if err != nil {
		return nil, err
	}
	for _, id := range input.GroupIDs {
		valid := slices.ContainsFunc(remoteGroups, func(group map[string]any) bool {
			groupPlatform, _ := group["platform"].(string)
			status, _ := group["status"].(string)
			return fmt.Sprint(group["id"]) == id && business.AccountPlatformCanJoinGroup(platform, groupPlatform) && status == "active"
		})
		if !valid {
			return nil, fmt.Errorf("目标分组 %s 已停用、不存在或平台不兼容，请同步目录后重试", id)
		}
	}
	id, err := operationID("account-groups")
	if err != nil {
		return nil, err
	}
	field := "group_ids"
	operation := business.AccountOperation{OperationID: id, OperationType: "account.groups", State: "succeeded", Phase: "readback", Actor: actor, ObjectID: accountID, FieldName: &field, Before: current, After: input.GroupIDs, Writeback: true, RemoteConfirmed: true, ReadbackConfirmed: true}
	toNumbers := func(ids []string) []int64 {
		values := make([]int64, 0, len(ids))
		for _, id := range ids {
			value, _ := strconv.ParseInt(id, 10, 64)
			values = append(values, value)
		}
		return values
	}
	if _, err := client.Mutate(ctx, http.MethodPut, "/admin/accounts/"+accountID, map[string]any{"group_ids": toNumbers(input.GroupIDs)}); err != nil {
		s.recordFailure(ctx, operation, "write", err, false)
		return nil, err
	}
	rollback := func(cause error) error {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		_, rollbackErr := client.UpdateAccountGroups(rollbackCtx, accountID, toNumbers(current))
		if rollbackErr != nil {
			cause = fmt.Errorf("%w；原分组恢复失败：%v，请同步管理平台核对", cause, rollbackErr)
		}
		s.recordFailure(rollbackCtx, operation, "readback", cause, true)
		return &OperationError{Message: cause.Error(), RemoteWritten: true}
	}
	after, err := client.Account(ctx, accountID)
	if err != nil {
		return nil, rollback(err)
	}
	actual, err := poolModeGroupIDs(after["group_ids"])
	if err != nil || fmt.Sprint(after["id"]) != accountID || !sameAccountGroups(actual, input.GroupIDs) {
		return nil, rollback(errors.New("账号分组写后确认不一致"))
	}
	repository := s.repository.(accountGroupsRepository)
	if err := repository.CommitAccountGroups(ctx, accountID, current, input.GroupIDs, operation); err != nil {
		return nil, rollback(err)
	}
	return map[string]any{"operation_id": id, "account_id": accountID, "before": current, "after": input.GroupIDs, "remote_write": true, "readback_confirmed": true}, nil
}
