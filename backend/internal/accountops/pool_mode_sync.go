package accountops

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

const maximumPoolModeSyncAccounts = 1000

type accountCreationSettingsProvider interface {
	AccountCreationSettings(context.Context) (configstore.AccountCreationSettings, error)
}

type poolModePolicy struct {
	Enabled     bool  `json:"pool_mode"`
	RetryCount  int   `json:"pool_mode_retry_count"`
	StatusCodes []int `json:"pool_mode_retry_status_codes"`
}

type poolModeSyncItem struct {
	AccountID         string         `json:"account_id"`
	AccountName       string         `json:"account_name,omitempty"`
	Status            string         `json:"status"`
	Policy            poolModePolicy `json:"policy"`
	RemoteWrite       bool           `json:"remote_write"`
	ReadbackConfirmed bool           `json:"readback_confirmed"`
	Error             string         `json:"error,omitempty"`
}

func (s *Service) EnqueuePoolModeSync(ctx context.Context, accountIDs []string, actor string) (taskstore.Task, error) {
	if len(accountIDs) == 0 || len(accountIDs) > maximumPoolModeSyncAccounts {
		return taskstore.Task{}, fmt.Errorf("请选择 1 到 %d 个已有账号", maximumPoolModeSyncAccounts)
	}
	provider, ok := s.targets.(accountCreationSettingsProvider)
	if !ok {
		return taskstore.Task{}, errors.New("账号池模式设置服务尚未就绪")
	}
	if mode, err := s.repository.Mode(ctx); err != nil {
		return taskstore.Task{}, err
	} else if mode != runtimepolicy.Full {
		return taskstore.Task{}, errors.New("同步已有账号池模式需要完全模式")
	}
	ids := make([]string, 0, len(accountIDs))
	seen := make(map[string]struct{}, len(accountIDs))
	for _, rawID := range accountIDs {
		accountID := strings.TrimSpace(rawID)
		if !stableID(accountID) {
			return taskstore.Task{}, errors.New("账号必须全部使用稳定数字 ID")
		}
		if _, duplicate := seen[accountID]; duplicate {
			return taskstore.Task{}, fmt.Errorf("账号 ID 不能重复：%s", accountID)
		}
		if _, _, err := s.localAccount(ctx, accountID); err != nil {
			return taskstore.Task{}, fmt.Errorf("账号 %s 不存在：%w", accountID, err)
		}
		seen[accountID] = struct{}{}
		ids = append(ids, accountID)
	}
	settings, err := provider.AccountCreationSettings(ctx)
	if err != nil {
		return taskstore.Task{}, fmt.Errorf("账号池模式设置读取失败：%w", err)
	}
	expectedTarget, err := s.targets.TargetSettings(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	id, err := taskID()
	if err != nil {
		return taskstore.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{
		ID: id, Skill: "sub2api-account-pool-mode", Operation: "account-pool-mode-sync",
		Status: "queued", Progress: 0, Message: "已有账号池模式同步已排队",
		Result:    map[string]any{"account_ids": ids, "completed": 0, "total": len(ids)},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	if err := taskrunner.GoTask(s.taskRunner, task.ID, func(parent context.Context) {
		s.executePoolModeSync(targetguard.Expect(parent, expectedTarget), task, ids, settings, strings.TrimSpace(actor))
	}); err != nil {
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
	return task, nil
}

func (s *Service) executePoolModeSync(ctx context.Context, task taskstore.Task, accountIDs []string, settings configstore.AccountCreationSettings, actor string) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	task.Status, task.Progress, task.Message = "running", 2, "正在同步已有账号池模式"
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if !taskstore.SaveRunning(ctx, s.tasks, task) {
		return
	}
	type outcome struct {
		index int
		item  poolModeSyncItem
	}
	items := make([]poolModeSyncItem, len(accountIDs))
	jobs := make(chan int)
	outcomes := make(chan outcome, len(accountIDs))
	var workers sync.WaitGroup
	for range min(4, len(accountIDs)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				if ctx.Err() != nil {
					return
				}
				outcomes <- outcome{index: index, item: s.syncPoolModeAccount(ctx, accountIDs[index], settings, actor)}
			}
		}()
	}
	go sendModelSyncJobs(ctx, len(accountIDs), jobs)
	go func() { workers.Wait(); close(outcomes) }()
	completed := 0
	for outcome := range outcomes {
		items[outcome.index] = outcome.item
		completed++
		task.Progress = 2 + completed*94/len(accountIDs)
		task.Message = fmt.Sprintf("池模式同步进度：已处理 %d/%d 个账号", completed, len(accountIDs))
		task.Result = poolModeTaskResult(accountIDs, completed, items)
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		taskstore.PersistProgress(s.tasks, task)
	}
	if completed != len(accountIDs) {
		task.Status, task.Progress, task.Message = "failed", 100, "已有账号池模式同步被中断"
		task.Result = poolModeTaskResult(accountIDs, completed, items)
		if cause := taskstore.ContextFailureCause(ctx); cause != nil {
			task.Result["error"] = cause.Error()
		}
	} else {
		result := poolModeTaskResult(accountIDs, completed, items)
		failed := result["failed"].(int)
		skipped := result["skipped"].(int)
		task.Status, task.Progress = "succeeded", 100
		task.Message = fmt.Sprintf("已有账号池模式同步完成：成功 %d，未变更 %d", result["succeeded"], result["unchanged"])
		if failed > 0 || skipped > 0 {
			task.Status = "partial"
			task.Message = fmt.Sprintf("已有账号池模式部分同步：成功 %d，跳过 %d，失败 %d", result["succeeded"], skipped, failed)
		}
		task.Result = result
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	taskstore.MarkCancelled(ctx, &task, "已有账号池模式同步已取消")
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) syncPoolModeAccount(ctx context.Context, accountID string, settings configstore.AccountCreationSettings, actor string) poolModeSyncItem {
	item := poolModeSyncItem{AccountID: accountID, Status: "failed"}
	guarded, release, err := s.acquireAccountMutation(ctx, accountID)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	defer release()
	if mode, local, err := s.localAccount(guarded, accountID); err != nil {
		item.Error = err.Error()
		return item
	} else if mode != runtimepolicy.Full {
		item.Error = "任务执行前已退出完全模式"
		return item
	} else if local.ManualPriority != nil {
		item.AccountName, item.Status, item.Error = local.Name, "skipped", "账号处于人工优先位，已跳过池模式同步"
		return item
	} else {
		item.AccountName = local.Name
	}
	guarded, err = targetguard.Bind(guarded, s.targets)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	client, err := s.adminClient(guarded)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	before, err := client.Account(guarded, accountID)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	returnedIDValue, _ := firstPresent(before, "id", "account_id")
	if returnedID := strings.TrimSpace(fmt.Sprint(returnedIDValue)); returnedID != accountID {
		item.Error = fmt.Sprintf("账号详情返回的稳定 ID 为 %q，与请求账号 %s 不一致", returnedID, accountID)
		return item
	}
	policy, err := effectivePoolModePolicy(settings, before["group_ids"])
	item.Policy = policy
	if err != nil {
		item.Status, item.Error = "skipped", err.Error()
		return item
	}
	credentials, err := accountCredentials(before)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	if poolModePolicyMatches(credentials, policy) {
		item.Status, item.ReadbackConfirmed = "unchanged", true
		return item
	}
	beforePolicy := poolModePolicyFromCredentials(credentials)
	credentials["pool_mode"] = policy.Enabled
	credentials["pool_mode_retry_count"] = policy.RetryCount
	credentials["pool_mode_retry_status_codes"] = append([]int{}, policy.StatusCodes...)
	if _, err := client.UpdateAccount(guarded, accountID, map[string]any{"credentials": credentials}); err != nil {
		item.Error = err.Error()
		return item
	}
	item.RemoteWrite = true
	readback, err := client.Account(guarded, accountID)
	if err != nil {
		item.Error = "管理平台写入成功，但池模式读回失败：" + err.Error()
		return item
	}
	confirmedCredentials, err := accountCredentials(readback)
	if err != nil || !poolModePolicyMatches(confirmedCredentials, policy) {
		if err == nil {
			err = errors.New("管理平台账号池模式写后确认不一致")
		}
		item.Error = err.Error()
		return item
	}
	item.ReadbackConfirmed = true
	operationID, err := operationID("account-pool-mode-sync")
	if err != nil {
		item.Error = "池模式已写入，但审计 ID 创建失败：" + err.Error()
		return item
	}
	field := "credentials.pool_mode,credentials.pool_mode_retry_count,credentials.pool_mode_retry_status_codes"
	name := item.AccountName
	if err := s.repository.RecordAccountOperation(guarded, business.AccountOperation{
		OperationID: operationID, OperationType: "account.pool_mode.sync", State: "succeeded", Phase: "readback",
		Actor: actor, RemoteConfirmed: true, ReadbackConfirmed: true, ObjectID: accountID,
		ObjectName: &name, FieldName: &field, Before: beforePolicy, After: policy, Writeback: true,
	}); err != nil {
		item.Error = "池模式已写入，但审计保存失败：" + err.Error()
		return item
	}
	item.Status = "succeeded"
	return item
}

func effectivePoolModePolicy(settings configstore.AccountCreationSettings, rawGroupIDs any) (poolModePolicy, error) {
	groupIDs, err := poolModeGroupIDs(rawGroupIDs)
	if err != nil {
		return poolModePolicy{}, err
	}
	configured := make(map[string]configstore.AccountCreationPolicy, len(settings.Groups))
	for _, group := range settings.Groups {
		configured[group.GroupID] = group.AccountCreationPolicy
	}
	policies := make([]poolModePolicy, 0, len(groupIDs))
	matchedIDs := make([]string, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		if policy, exists := configured[groupID]; exists {
			policies = append(policies, poolModePolicyFromConfig(policy))
			matchedIDs = append(matchedIDs, groupID)
		}
	}
	if len(policies) == 0 {
		return poolModePolicyFromConfig(settings.Default), nil
	}
	for _, policy := range policies[1:] {
		if !reflect.DeepEqual(policy, policies[0]) {
			return poolModePolicy{}, fmt.Errorf("账号同时命中池模式设置不同的分组 %s，请先统一这些分组设置", strings.Join(matchedIDs, "、"))
		}
	}
	return policies[0], nil
}

func poolModeGroupIDs(raw any) ([]string, error) {
	if raw == nil {
		return []string{}, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, errors.New("管理平台账号分组列表不可读")
	}
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		text := strings.TrimSpace(fmt.Sprint(value))
		parsed, err := strconv.ParseUint(text, 10, 64)
		if err != nil || parsed == 0 || strconv.FormatUint(parsed, 10) != text {
			return nil, errors.New("管理平台账号分组列表包含无效稳定 ID")
		}
		if _, duplicate := seen[text]; !duplicate {
			seen[text] = struct{}{}
			result = append(result, text)
		}
	}
	sort.Slice(result, func(left, right int) bool {
		leftID, _ := strconv.ParseUint(result[left], 10, 64)
		rightID, _ := strconv.ParseUint(result[right], 10, 64)
		return leftID < rightID
	})
	return result, nil
}

func poolModePolicyFromConfig(policy configstore.AccountCreationPolicy) poolModePolicy {
	return poolModePolicy{Enabled: policy.PoolMode, RetryCount: policy.PoolModeRetryCount, StatusCodes: append([]int{}, policy.PoolModeRetryStatusCodes...)}
}

func poolModePolicyFromCredentials(credentials map[string]any) poolModePolicy {
	policy := poolModePolicy{}
	policy.Enabled, _ = credentials["pool_mode"].(bool)
	if value, err := poolModeInteger(credentials["pool_mode_retry_count"]); err == nil {
		policy.RetryCount = value
	}
	policy.StatusCodes, _ = poolModeStatusCodes(credentials["pool_mode_retry_status_codes"])
	return policy
}

func poolModePolicyMatches(credentials map[string]any, policy poolModePolicy) bool {
	return reflect.DeepEqual(poolModePolicyFromCredentials(credentials), policy)
}

func poolModeInteger(raw any) (int, error) {
	if raw == nil {
		return 0, errors.New("数值为空")
	}
	value, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(raw)))
	if err != nil {
		return 0, err
	}
	return value, nil
}

func poolModeStatusCodes(raw any) ([]int, error) {
	if raw == nil {
		return []int{}, nil
	}
	values, ok := raw.([]any)
	if !ok {
		if typed, typedOK := raw.([]int); typedOK {
			return append([]int{}, typed...), nil
		}
		return nil, errors.New("池模式重试状态码不可读")
	}
	result := make([]int, 0, len(values))
	for _, rawValue := range values {
		value, err := poolModeInteger(rawValue)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}

func poolModeTaskResult(accountIDs []string, completed int, items []poolModeSyncItem) map[string]any {
	succeeded, unchanged, skipped, failed := 0, 0, 0, 0
	completedItems := make([]poolModeSyncItem, 0, completed)
	for _, item := range items {
		if item.AccountID == "" {
			continue
		}
		completedItems = append(completedItems, item)
		switch item.Status {
		case "succeeded":
			succeeded++
		case "unchanged":
			unchanged++
		case "skipped":
			skipped++
		default:
			failed++
		}
	}
	return map[string]any{
		"account_ids": accountIDs, "completed": completed, "total": len(accountIDs),
		"succeeded": succeeded, "unchanged": unchanged, "skipped": skipped, "failed": failed,
		"items": completedItems,
	}
}
