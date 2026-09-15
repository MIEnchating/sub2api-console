package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func (s *Service) historicalReauthorizationIDs(ctx context.Context, target configstore.TargetSettings, input ReauthorizationPreviewInput) ([]string, error) {
	if len(input.SourceTaskID) > 255 {
		return nil, errors.New("原授权任务 ID 无效")
	}
	tasks, ok := s.tasks.(interface {
		Get(context.Context, string) (taskstore.Task, error)
	})
	if !ok {
		return nil, errors.New("原授权任务读取服务尚未就绪")
	}
	task, err := tasks.Get(ctx, input.SourceTaskID)
	if err != nil {
		return nil, err
	}
	if task.Skill != Skill || task.Operation != "account-workbench-oauth-batch" || task.Status == "running" || task.Status == "queued" || task.Status == "waiting_input" {
		return nil, errors.New("请选择已结束的批量重新授权任务")
	}
	raw, err := json.Marshal(task.Result["items"])
	if err != nil {
		return nil, errors.New("原授权账号范围无法读取")
	}
	var rows []OAuthBatchRow
	if err := json.Unmarshal(raw, &rows); err != nil || len(rows) == 0 || len(rows) > 500 {
		return nil, errors.New("原任务缺少稳定账号范围，请在登录资料中重新选择")
	}
	store, err := s.profileStore()
	if err != nil {
		return nil, err
	}
	available := make(map[string]OAuthBatchRow, len(rows))
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.AccountID == "" || row.ProfileID == "" || row.UserID == "" || row.WorkspaceID == "" {
			continue
		}
		if _, duplicate := available[row.AccountID]; duplicate {
			return nil, errors.New("原任务存在重复账号，请在登录资料中重新选择")
		}
		available[row.AccountID] = row
		ids = append(ids, row.AccountID)
	}
	if len(input.AccountIDs) > 0 {
		ids = input.AccountIDs
	}
	for _, id := range ids {
		row, ok := available[id]
		if !ok {
			return nil, errors.New("选择范围包含原任务之外的账号")
		}
		profile, err := store.WorkbenchLoginProfile(ctx, executionTargetFingerprint(target), row.ProfileID)
		if err != nil || profile.AccountID != id || profile.UserID != row.UserID || profile.WorkspaceID != row.WorkspaceID || !strings.EqualFold(profile.Email, row.Email) {
			return nil, errors.New("原授权账号与当前管理目标或登录资料身份不一致，请重新选择")
		}
	}
	return ids, nil
}
