package accountworkbench

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

type AccountGroupsPreview struct {
	AccountID       string   `json:"account_id"`
	CurrentGroupIDs []string `json:"current_group_ids"`
	Groups          []Group  `json:"groups"`
	TargetVersion   string   `json:"target_version"`
	AccountVersion  string   `json:"account_version"`
}

type MoveGroupsInput struct {
	GroupIDs         []string `json:"group_ids"`
	ExpectedGroupIDs []string `json:"expected_group_ids"`
	TargetVersion    string   `json:"target_version"`
	AccountVersion   string   `json:"account_version"`
}

func validGroupID(id string) bool {
	n, err := strconv.ParseInt(id, 10, 64)
	return err == nil && n > 0 && strconv.FormatInt(n, 10) == id
}

func currentGroupIDs(account map[string]any) ([]string, error) {
	raw, ok := account["group_ids"].([]any)
	if !ok {
		return nil, errors.New("账号分组信息不完整，请在管理平台核对后重试")
	}
	ids := make([]string, 0, len(raw))
	for _, value := range raw {
		id := text(value)
		if !validGroupID(id) || slices.Contains(ids, id) {
			return nil, errors.New("账号分组 ID 无效或重复，请在管理平台核对后重试")
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (s *Service) groupsMoveAllowed(ctx context.Context, id string) error {
	if s.groupProtection == nil {
		return nil
	}
	protection, err := s.groupProtection.AccountMutationProtection(ctx, id)
	if err != nil {
		return err
	}
	if protection.UpstreamExcluded {
		return business.ErrUpstreamExcluded
	}
	if protection.GroupsLocked {
		return business.ErrAccountGroupsLocked
	}
	if protection.ManualPriority {
		return errors.New("账号处于手动控制，请先取消手动控制再移动分组")
	}
	return nil
}

func (s *Service) AccountGroups(ctx context.Context, id string) (AccountGroupsPreview, error) {
	if !validGroupID(id) {
		return AccountGroupsPreview{}, errors.New("账号必须使用有效的稳定 ID")
	}
	if err := s.groupsMoveAllowed(ctx, id); err != nil {
		return AccountGroupsPreview{}, err
	}
	ctx, err := targetguard.Capture(ctx, s.private)
	if err != nil {
		return AccountGroupsPreview{}, err
	}
	ctx, err = targetguard.Pin(ctx, s.private)
	if err != nil {
		return AccountGroupsPreview{}, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return AccountGroupsPreview{}, err
	}
	client, err := s.client(target)
	if err != nil {
		return AccountGroupsPreview{}, err
	}
	account, err := client.Account(ctx, id)
	if err != nil {
		return AccountGroupsPreview{}, errors.New("账号读取失败，请检查站点连接后重试")
	}
	if text(account["id"]) != id || text(account["platform"]) != "openai" || text(account["type"]) != "oauth" {
		return AccountGroupsPreview{}, errors.New("账号 ID 或类型已变化，请刷新列表后重试")
	}
	ids, err := currentGroupIDs(account)
	if err != nil {
		return AccountGroupsPreview{}, err
	}
	groups, err := client.Groups(ctx)
	if err != nil {
		return AccountGroupsPreview{}, errors.New("远端分组读取失败，请检查站点连接后重试")
	}
	result := AccountGroupsPreview{AccountID: id, CurrentGroupIDs: ids, Groups: []Group{}, TargetVersion: targetguard.Fingerprint(target), AccountVersion: accountVersion(account)}
	for _, group := range groups {
		if text(group["status"]) == "active" && business.AccountPlatformCanJoinGroup("openai", text(group["platform"])) && validGroupID(text(group["id"])) {
			result.Groups = append(result.Groups, Group{ID: text(group["id"]), Name: text(group["name"])})
		}
	}
	if _, err = targetguard.Pin(ctx, s.private); err != nil {
		return AccountGroupsPreview{}, err
	}
	return result, nil
}

func sameGroups(left, right []string) bool {
	left, right = slices.Clone(left), slices.Clone(right)
	slices.Sort(left)
	slices.Sort(right)
	return slices.Equal(left, right)
}

func validateMoveGroups(preview AccountGroupsPreview, input MoveGroupsInput) error {
	if input.TargetVersion != preview.TargetVersion {
		return errors.New("管理目标已变化，请重新打开分组选择")
	}
	if input.AccountVersion != preview.AccountVersion || input.ExpectedGroupIDs == nil || !sameGroups(input.ExpectedGroupIDs, preview.CurrentGroupIDs) {
		return errors.New("账号配置或分组已变化，请重新打开分组选择")
	}
	if len(input.GroupIDs) < 1 || len(input.GroupIDs) > 50 {
		return errors.New("请选择 1 到 50 个目标分组")
	}
	seen := map[string]bool{}
	for _, id := range input.GroupIDs {
		if !validGroupID(id) || seen[id] {
			return errors.New("目标分组必须使用有效且不重复的稳定 ID")
		}
		seen[id] = true
		if !slices.ContainsFunc(preview.Groups, func(group Group) bool { return group.ID == id }) {
			return fmt.Errorf("目标分组 %s 已停用、不存在或平台不兼容，请重新选择", id)
		}
	}
	if sameGroups(input.GroupIDs, preview.CurrentGroupIDs) {
		return errors.New("目标分组与当前分组相同，无需移动")
	}
	return nil
}
