package onboarding

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/naming"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type Repository interface {
	OnboardingCandidates(context.Context, string) ([]business.OnboardingCandidate, error)
	LocalOnboardingGroup(context.Context, string) (business.LocalOnboardingGroup, error)
	PendingOnboarding(context.Context, string, string, string, string) (*business.PendingOnboarding, error)
	SavePendingOnboarding(context.Context, business.PendingOnboarding) error
	CommitOnboardingProjection(context.Context, business.OnboardingProjection) error
	CommitAccountGroupsReadback(context.Context, string, []business.LocalOnboardingGroup, business.AccountOperation) error
	RecordAccountOperation(context.Context, business.AccountOperation) error
}

type PrivateStore interface {
	AuthRecord(context.Context, string) (*configstore.AuthRecord, error)
	TargetSettings(context.Context) (configstore.TargetSettings, error)
	AccountDefaults(context.Context) (configstore.AccountDefaultsSettings, error)
	SaveUpstreamKeySecret(context.Context, configstore.UpstreamKeySecret) error
}

type KeyClient interface {
	CreateKey(context.Context, configstore.AuthRecord, string, string) (upstreamsync.CreatedKey, error)
	RevealKey(context.Context, configstore.AuthRecord, string, string) (upstreamsync.CreatedKey, error)
}

type configurableKeyClient interface {
	CreateKeyWithVerification(context.Context, configstore.AuthRecord, string, string, bool) (upstreamsync.CreatedKey, error)
}

type TaskStore interface {
	Save(context.Context, taskstore.Task) error
}

type Request struct {
	Host            string
	UpstreamType    string
	PlatformPresent bool
	Platform        *string
	AccountType     *string
	Notes           *string
	Multiplier      string
	LocalGroupID    string
	LocalGroupIDs   []string
	UpstreamGroupID string
	AccountIDs      []string
	Extra           map[string]any
	Priority        *int64
	Concurrency     *int64
	Schedulable     bool
	Actor           string
}

type Service struct {
	repository Repository
	private    PrivateStore
	keys       KeyClient
	tasks      TaskStore
	timeout    time.Duration
}

type validatedRequest struct {
	request    Request
	multiplier string
	locals     []business.LocalOnboardingGroup
	candidate  business.OnboardingCandidate
	auth       configstore.AuthRecord
}

type batchItem struct {
	request           Request
	upstreamGroupName string
	localGroupName    string
	action            string
}

func New(repository Repository, private PrivateStore, keys KeyClient, tasks TaskStore) *Service {
	return &Service{repository: repository, private: private, keys: keys, tasks: tasks, timeout: 10 * time.Minute}
}

func (s *Service) Candidates(ctx context.Context, host string) ([]business.OnboardingCandidate, error) {
	if strings.TrimSpace(host) == "" {
		return nil, errors.New("上游 Host 不能为空")
	}
	return s.repository.OnboardingCandidates(ctx, host)
}

func (s *Service) Enqueue(ctx context.Context, request Request) (taskstore.Task, error) {
	if _, err := s.validate(ctx, request); err != nil {
		return taskstore.Task{}, err
	}
	task, err := s.newQueuedTask("onboard", "账号绑定变更已排队")
	if err != nil {
		return taskstore.Task{}, err
	}
	if err := s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	go s.execute(task, request)
	return task, nil
}

func (s *Service) EnqueueBatch(ctx context.Context, requests []Request) (taskstore.Task, error) {
	if len(requests) == 0 {
		return taskstore.Task{}, errors.New("请至少选择一个要添加的账号")
	}
	if len(requests) > 50 {
		return taskstore.Task{}, errors.New("单次最多添加 50 个账号")
	}
	items := make([]batchItem, 0, len(requests))
	seen := map[string]struct{}{}
	for _, request := range requests {
		validated, err := s.validate(ctx, request)
		if err != nil {
			return taskstore.Task{}, err
		}
		key := strings.ToLower(strings.TrimSpace(validated.auth.Host)) + "\x00" + validated.candidateID()
		if _, found := seen[key]; found {
			return taskstore.Task{}, errors.New("同一个上游分组不能在一个批次中重复提交")
		}
		seen[key] = struct{}{}
		items = append(items, batchItem{
			request: request, upstreamGroupName: validated.candidate.GroupName,
			localGroupName: strings.Join(onboardingLocalNames(validated.locals), "、"), action: onboardingAction(validated),
		})
	}
	task, err := s.newQueuedTask("onboard-batch", fmt.Sprintf("%d 项账号绑定变更已排队", len(items)))
	if err != nil {
		return taskstore.Task{}, err
	}
	if err := s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	go s.executeBatch(task, items)
	return task, nil
}

func (s *Service) newQueuedTask(operation, message string) (taskstore.Task, error) {
	id, err := randomID(12)
	if err != nil {
		return taskstore.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{
		ID: id, Skill: "sub2api-account-onboarding", Operation: operation, Status: "queued", Progress: 0,
		Message: message, Result: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}
	return task, nil
}

func (s *Service) execute(task taskstore.Task, request Request) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	task.Status, task.Progress, task.Message, task.UpdatedAt = "running", 10, "正在校验稳定 ID 与上游分组", time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.tasks.Save(ctx, task); err != nil {
		return
	}
	result, err := s.Onboard(ctx, request)
	task.Progress, task.UpdatedAt = 100, time.Now().UTC().Format(time.RFC3339Nano)
	if err != nil {
		task.Status, task.Message = "failed", "账号绑定变更失败："+err.Error()
		task.Result = map[string]any{"operation": "account.onboarding", "error": err.Error(), "remote_write": result["remote_write"], "pending": result["pending"]}
	} else {
		task.Status, task.Message, task.Result = "succeeded", "账号绑定变更已完成并写入 Console 业务库", result
	}
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) executeBatch(task taskstore.Task, items []batchItem) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout*time.Duration(len(items)))
	defer cancel()
	task.Status, task.Progress, task.Message = "running", 5, fmt.Sprintf("正在添加 1/%d 个账号", len(items))
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.tasks.Save(ctx, task); err != nil {
		return
	}
	results := make([]map[string]any, 0, len(items))
	succeeded := 0
	failed := 0
	for index, item := range items {
		task.Progress = 5 + index*90/len(items)
		task.Message = fmt.Sprintf("正在处理 %d/%d：%s，%s → %s", index+1, len(items), item.action, item.upstreamGroupName, item.localGroupName)
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := s.tasks.Save(ctx, task); err != nil {
			return
		}
		_, err := s.Onboard(ctx, item.request)
		row := map[string]any{
			"upstream_group": item.upstreamGroupName,
			"local_group":    item.localGroupName,
			"action":         item.action,
		}
		if err != nil {
			failed++
			row["status"] = "失败"
			row["error"] = safeError(err)
		} else {
			succeeded++
			row["status"] = "成功"
		}
		results = append(results, row)
	}
	task.Progress = 100
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	task.Result = map[string]any{
		"operation": "account.onboarding.batch", "total": len(items), "succeeded": succeeded, "failed": failed, "items": results,
	}
	if failed > 0 {
		task.Status = "failed"
		task.Message = fmt.Sprintf("批量绑定变更完成：成功 %d 项，失败 %d 项", succeeded, failed)
	} else {
		task.Status = "succeeded"
		task.Message = fmt.Sprintf("批量绑定变更完成：成功 %d 项", succeeded)
	}
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) Onboard(ctx context.Context, request Request) (map[string]any, error) {
	validated, err := s.validate(ctx, request)
	if err != nil {
		return map[string]any{"remote_write": false}, err
	}
	if len(validated.request.AccountIDs) > 0 {
		return s.updateAccountGroups(ctx, validated)
	}
	operationID, err := operationID()
	if err != nil {
		return map[string]any{"remote_write": false}, err
	}
	primaryLocal := validated.locals[0]
	pending, err := s.repository.PendingOnboarding(ctx, validated.auth.Host, validated.candidateID(), primaryLocal.ID, validated.multiplier)
	if err != nil {
		return map[string]any{"remote_write": false}, err
	}
	if pending != nil {
		operationID = pending.OperationID
	}
	keyName := validated.candidate.GroupName + "-" + operationID[len(operationID)-8:]
	var key upstreamsync.CreatedKey
	if pending == nil {
		key, err = s.keys.CreateKey(ctx, validated.auth, keyName, validated.candidateID())
	} else {
		key, err = s.keys.RevealKey(ctx, validated.auth, pending.UpstreamKeyID, validated.candidateID())
	}
	if err != nil {
		s.recordFailure(ctx, operationID, validated, "remote-write", false, err)
		return map[string]any{"remote_write": false}, err
	}
	result := map[string]any{"remote_write": "已创建上游 Key，等待续办", "upstream_key_created": pending == nil}
	if err := s.private.SaveUpstreamKeySecret(ctx, configstore.UpstreamKeySecret{
		Host: validated.auth.Host, KeyID: key.KeyID, GroupID: validated.candidateID(), Secret: key.Secret,
	}); err != nil {
		return s.pendingFailure(ctx, operationID, validated, key, result, fmt.Errorf("本地 Key 保存失败：%w", err))
	}
	accountName := naming.AccountName(validated.candidate.UpstreamName, validated.auth.BaseURL, validated.multiplier)
	remark := creationRemark(validated.candidate, validated.request.Notes)
	target, err := s.private.TargetSettings(ctx)
	if err != nil {
		return s.pendingFailure(ctx, operationID, validated, key, result, err)
	}
	client, err := adminclient.New(adminclient.Config{
		BaseURL: target.BaseURL, AdminKey: target.AdminKey,
		Timeout: time.Duration(target.TimeoutSeconds) * time.Second, Attempts: 3,
	}, nil)
	if err != nil {
		return s.pendingFailure(ctx, operationID, validated, key, result, err)
	}
	platform, err := accountPlatform(validated.request, validated.candidate, primaryLocal)
	if err != nil {
		return s.pendingFailure(ctx, operationID, validated, key, result, err)
	}
	accountType := "apikey"
	if validated.request.AccountType != nil {
		accountType = strings.ToLower(strings.TrimSpace(*validated.request.AccountType))
		if accountType == "" {
			return s.pendingFailure(ctx, operationID, validated, key, result, errors.New("账号类型不能为空"))
		}
	}
	defaults, err := s.private.AccountDefaults(ctx)
	if err != nil {
		return s.pendingFailure(ctx, operationID, validated, key, result, fmt.Errorf("账号默认参数读取失败：%w", err))
	}
	priority, concurrency := accountCreationParameters(defaults, validated.request)
	localGroupIDs := onboardingLocalNumericIDs(validated.locals)
	body := map[string]any{
		"name": accountName, "notes": remark, "platform": platform, "type": accountType,
		"credentials": map[string]any{"api_key": key.Secret, "base_url": validated.auth.BaseURL}, "extra": validated.request.Extra,
		"rate_multiplier": json.Number(validated.multiplier), "group_ids": localGroupIDs,
		"concurrency": concurrency, "priority": priority,
		"auto_pause_on_expired": true,
	}
	created, err := client.CreateAccount(ctx, body)
	if err != nil {
		return s.pendingFailure(ctx, operationID, validated, key, result, redactSecret(err, key.Secret))
	}
	accountID := textValue(firstPresent(created, "id", "account_id"))
	scheduleResponse, err := client.Mutate(ctx, "POST", "/admin/accounts/"+accountID+"/schedulable", map[string]any{"schedulable": validated.request.Schedulable})
	if err != nil {
		return s.pendingFailure(ctx, operationID, validated, key, result, redactSecret(err, key.Secret))
	}
	verified := validated.request.Schedulable
	if response, trusted := matchingOnboardingResponse(scheduleResponse, accountID, accountName, onboardingLocalIDs(validated.locals), validated.multiplier, validated.request.Schedulable); trusted {
		verified = response
	}
	projection := business.OnboardingProjection{
		OperationID: operationID, AccountID: accountID, AccountName: accountName,
		UpstreamHost: validated.auth.Host, UpstreamType: validated.request.UpstreamType, BaseURL: validated.auth.BaseURL,
		UpstreamKeyID: key.KeyID, UpstreamKeyName: key.Name, UpstreamGroupID: validated.candidateID(),
		UpstreamGroupName: validated.candidate.GroupName, LocalGroupID: primaryLocal.ID,
		LocalGroupName: primaryLocal.Name, LocalGroups: validated.locals, Multiplier: validated.multiplier, Schedulable: verified,
		Priority: &priority, Concurrency: &concurrency, Notes: remark, Actor: validated.request.Actor, ReadbackConfirmed: false,
	}
	if err := s.repository.CommitOnboardingProjection(ctx, projection); err != nil {
		return s.pendingFailure(ctx, operationID, validated, key, result, err)
	}
	return map[string]any{
		"operation_id": operationID, "account_id": accountID, "account_name": accountName,
		"local_group_ids": onboardingLocalIDs(validated.locals), "local_group_names": onboardingLocalNames(validated.locals),
		"upstream_group_id": validated.candidateID(), "upstream_group_name": validated.candidate.GroupName,
		"schedulable": verified, "credentials": "已保存到 Console 私有配置库", "upstream_key_created": pending == nil,
		"readback_confirmed": false, "concurrency": concurrency, "priority": priority,
	}, nil
}

func accountCreationParameters(defaults configstore.AccountDefaultsSettings, request Request) (int64, int64) {
	priority, concurrency := defaults.Priority, defaults.Concurrency
	if request.Priority != nil {
		priority = *request.Priority
	}
	if request.Concurrency != nil {
		concurrency = *request.Concurrency
	}
	return priority, concurrency
}

func createKey(ctx context.Context, client KeyClient, record configstore.AuthRecord, name, groupID string, verification bool) (upstreamsync.CreatedKey, error) {
	if configurable, ok := client.(configurableKeyClient); ok {
		return configurable.CreateKeyWithVerification(ctx, record, name, groupID, verification)
	}
	return client.CreateKey(ctx, record, name, groupID)
}

func matchingOnboardingResponse(payload map[string]any, accountID, name string, groupIDs []string, multiplier string, schedulable bool) (bool, bool) {
	if raw, present := payload["data"]; present {
		var ok bool
		payload, ok = raw.(map[string]any)
		if !ok {
			return false, false
		}
	}
	value, err := verifyReadback(payload, accountID, name, groupIDs, multiplier, schedulable)
	return value, err == nil
}

func (s *Service) validate(ctx context.Context, request Request) (validatedRequest, error) {
	if request.Priority != nil && (*request.Priority < 1 || *request.Priority > 10_000_000) {
		return validatedRequest{}, errors.New("优先级必须是 1 到 10000000 之间的整数")
	}
	if request.Concurrency != nil && (*request.Concurrency < 1 || *request.Concurrency > 10_000_000) {
		return validatedRequest{}, errors.New("并发必须是 1 到 10000000 之间的整数")
	}
	multiplier, err := positiveDecimal(request.Multiplier)
	if err != nil {
		return validatedRequest{}, err
	}
	localGroupIDs := append([]string{}, request.LocalGroupIDs...)
	if len(localGroupIDs) == 0 && strings.TrimSpace(request.LocalGroupID) != "" {
		localGroupIDs = []string{request.LocalGroupID}
	}
	if len(localGroupIDs) == 0 || len(localGroupIDs) > 50 {
		return validatedRequest{}, errors.New("本地分组数量必须在 1 到 50 之间")
	}
	locals := make([]business.LocalOnboardingGroup, 0, len(localGroupIDs))
	seenLocalGroups := map[string]struct{}{}
	for _, localGroupID := range localGroupIDs {
		localGroupID = strings.TrimSpace(localGroupID)
		if _, duplicate := seenLocalGroups[localGroupID]; duplicate {
			continue
		}
		local, err := s.repository.LocalOnboardingGroup(ctx, localGroupID)
		if err != nil {
			return validatedRequest{}, err
		}
		if _, err := strconv.ParseInt(local.ID, 10, 64); err != nil {
			return validatedRequest{}, errors.New("本地分组稳定 ID 超出支持范围")
		}
		seenLocalGroups[localGroupID] = struct{}{}
		locals = append(locals, local)
	}
	auth, err := s.private.AuthRecord(ctx, request.Host)
	if err != nil {
		return validatedRequest{}, err
	}
	if auth == nil {
		return validatedRequest{}, errors.New("账号添加前必须先配置该 Host 的鉴权记录")
	}
	candidates, err := s.repository.OnboardingCandidates(ctx, auth.Host)
	if err != nil {
		return validatedRequest{}, err
	}
	var candidate *business.OnboardingCandidate
	for index := range candidates {
		if candidates[index].GroupID != nil && *candidates[index].GroupID == strings.TrimSpace(request.UpstreamGroupID) {
			candidate = &candidates[index]
			break
		}
	}
	if candidate == nil {
		return validatedRequest{}, errors.New("上游分组不存在或不在 Console 业务库中")
	}
	accountIDs, err := validatedBoundAccountIDs(request.AccountIDs, *candidate)
	if err != nil {
		return validatedRequest{}, err
	}
	if len(accountIDs) == 0 && !candidate.CanCreateKey {
		if candidate.UnavailableReason != nil {
			return validatedRequest{}, errors.New(*candidate.UnavailableReason)
		}
		return validatedRequest{}, errors.New("上游分组当前不可创建 Key")
	}
	request.LocalGroupID = locals[0].ID
	request.LocalGroupIDs = onboardingLocalIDs(locals)
	request.AccountIDs = accountIDs
	validated := validatedRequest{request: request, multiplier: multiplier, locals: locals, candidate: *candidate, auth: *auth}
	if len(accountIDs) == 0 {
		if _, err := accountPlatform(request, *candidate, locals[0]); err != nil {
			return validatedRequest{}, err
		}
	}
	return validated, nil
}

func validatedBoundAccountIDs(requested []string, candidate business.OnboardingCandidate) ([]string, error) {
	if len(requested) == 0 {
		return nil, nil
	}
	available := map[string]struct{}{}
	for _, account := range candidate.BoundAccounts {
		if account.AccountExists && pointerValue(account.BindingStatus) != "missing" {
			available[account.AccountID] = struct{}{}
		}
	}
	result := make([]string, 0, len(requested))
	seen := map[string]struct{}{}
	for _, accountID := range requested {
		accountID = strings.TrimSpace(accountID)
		if !stableID(accountID) {
			return nil, errors.New("已有绑定账号必须使用稳定数字 ID")
		}
		if _, ok := available[accountID]; !ok {
			return nil, errors.New("已有绑定账号与所选上游分组不匹配")
		}
		if _, duplicate := seen[accountID]; duplicate {
			continue
		}
		seen[accountID] = struct{}{}
		result = append(result, accountID)
	}
	if len(result) == 0 {
		return nil, errors.New("请至少选择一个已有绑定账号")
	}
	return result, nil
}

func onboardingAction(value validatedRequest) string {
	if len(value.request.AccountIDs) > 0 {
		return "更新绑定"
	}
	return "添加账号"
}

func onboardingLocalIDs(groups []business.LocalOnboardingGroup) []string {
	result := make([]string, 0, len(groups))
	for _, group := range groups {
		result = append(result, group.ID)
	}
	sort.Strings(result)
	return result
}

func onboardingLocalNumericIDs(groups []business.LocalOnboardingGroup) []int64 {
	ids := onboardingLocalIDs(groups)
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		result = append(result, mustInt64(id))
	}
	return result
}

func onboardingLocalNames(groups []business.LocalOnboardingGroup) []string {
	result := make([]string, 0, len(groups))
	for _, group := range groups {
		result = append(result, group.Name)
	}
	sort.Strings(result)
	return result
}

func (s *Service) updateAccountGroups(ctx context.Context, validated validatedRequest) (map[string]any, error) {
	target, err := s.private.TargetSettings(ctx)
	if err != nil {
		return map[string]any{"remote_write": false}, err
	}
	client, err := adminclient.New(adminclient.Config{
		BaseURL: target.BaseURL, AdminKey: target.AdminKey,
		Timeout: time.Duration(target.TimeoutSeconds) * time.Second, Attempts: 3,
	}, nil)
	if err != nil {
		return map[string]any{"remote_write": false}, err
	}
	desiredIDs := onboardingLocalIDs(validated.locals)
	desiredNumericIDs := onboardingLocalNumericIDs(validated.locals)
	results := make([]map[string]any, 0, len(validated.request.AccountIDs))
	for _, accountID := range validated.request.AccountIDs {
		before, err := client.Account(ctx, accountID)
		if err != nil {
			return map[string]any{"remote_write": false}, err
		}
		beforeIDs, err := stableIDs(before["group_ids"])
		if err != nil {
			return map[string]any{"remote_write": false}, errors.New("管理平台账号当前分组不可读")
		}
		operationID, err := operationID()
		if err != nil {
			return map[string]any{"remote_write": false}, err
		}
		accountName := textValue(before["name"])
		field := "group_ids"
		after, err := client.UpdateAccountGroups(ctx, accountID, desiredNumericIDs)
		if err != nil {
			detail := safeError(err)
			operation := business.AccountOperation{
				OperationID: operationID, OperationType: "account.groups", State: "failed", Phase: "remote-readback",
				Actor: validated.request.Actor, Error: &detail, RemoteConfirmed: true, ReadbackConfirmed: false,
				ObjectID: accountID, ObjectName: &accountName, GroupNames: onboardingLocalNames(validated.locals),
				FieldName: &field, Before: beforeIDs, After: desiredIDs, Writeback: true,
			}
			_ = s.repository.RecordAccountOperation(ctx, operation)
			return map[string]any{"remote_write": true}, err
		}
		confirmedIDs, err := stableIDs(after["group_ids"])
		if err != nil || strings.Join(confirmedIDs, ",") != strings.Join(desiredIDs, ",") {
			return map[string]any{"remote_write": true}, errors.New("管理平台账号分组写后确认不一致")
		}
		operation := business.AccountOperation{
			OperationID: operationID, OperationType: "account.groups", State: "succeeded", Phase: "readback",
			Actor: validated.request.Actor, RemoteConfirmed: true, ReadbackConfirmed: true,
			ObjectID: accountID, ObjectName: &accountName, GroupNames: onboardingLocalNames(validated.locals),
			FieldName: &field, Before: beforeIDs, After: confirmedIDs, Writeback: true,
		}
		if err := s.repository.CommitAccountGroupsReadback(ctx, accountID, validated.locals, operation); err != nil {
			return map[string]any{"remote_write": true}, errors.New("管理平台分组已更新，但本地绑定提交失败：" + err.Error())
		}
		results = append(results, map[string]any{"account_id": accountID, "before": beforeIDs, "after": confirmedIDs})
	}
	return map[string]any{
		"operation": "account.groups", "remote_write": true, "readback_confirmed": true,
		"upstream_group_id": validated.candidateID(), "upstream_group_name": validated.candidate.GroupName,
		"local_group_ids": desiredIDs, "local_group_names": onboardingLocalNames(validated.locals), "accounts": results,
	}, nil
}

func (s *Service) pendingFailure(ctx context.Context, operationID string, validated validatedRequest, key upstreamsync.CreatedKey, result map[string]any, cause error) (map[string]any, error) {
	reason := safeError(cause)
	primaryLocal := validated.locals[0]
	pending := business.PendingOnboarding{
		OperationID: operationID, UpstreamHost: validated.auth.Host, UpstreamType: validated.request.UpstreamType,
		UpstreamKeyID: key.KeyID, UpstreamKeyName: &key.Name, UpstreamGroupID: validated.candidateID(),
		UpstreamGroupName: validated.candidate.GroupName, LocalGroupID: primaryLocal.ID,
		LocalGroupName: primaryLocal.Name, Multiplier: validated.multiplier, Reason: reason,
	}
	if err := s.repository.SavePendingOnboarding(ctx, pending); err != nil {
		reason += "；待续记录保存失败：" + safeError(err)
	}
	s.recordFailure(ctx, operationID, validated, "remote-readback", true, errors.New(reason))
	result["pending"] = map[string]any{
		"operation_id": operationID, "upstream_host": validated.auth.Host, "upstream_key_id": key.KeyID,
		"upstream_group_id": validated.candidateID(), "local_group_id": primaryLocal.ID,
	}
	return result, errors.New(reason + "；上游 Key 已创建，已保存待续记录并禁止重复创建")
}

func (s *Service) recordFailure(ctx context.Context, operationID string, validated validatedRequest, phase string, remote bool, cause error) {
	field, name, reason := "created", naming.AccountName(validated.candidate.UpstreamName, validated.auth.BaseURL, validated.multiplier), safeError(cause)
	operation := business.AccountOperation{
		OperationID: operationID, OperationType: "account.onboarding", State: "failed", Phase: phase,
		Actor: validated.request.Actor, Error: &reason, RemoteConfirmed: remote, ReadbackConfirmed: false,
		ObjectID: "", ObjectName: &name, GroupNames: onboardingLocalNames(validated.locals), FieldName: &field,
		After: map[string]any{"name": name, "group_ids": onboardingLocalIDs(validated.locals), "rate_multiplier": validated.multiplier}, Writeback: true,
	}
	if err := s.repository.RecordAccountOperation(ctx, operation); err != nil {
		slog.Error("账号添加失败记录保存失败", "operation_id", operationID, "host", validated.auth.Host, "error", err)
	}
}

func (v validatedRequest) candidateID() string { return pointerValue(v.candidate.GroupID) }

func verifyReadback(value map[string]any, accountID, name string, groupIDs []string, multiplier string, schedulable bool) (bool, error) {
	if textValue(firstPresent(value, "id", "account_id")) != accountID {
		return false, errors.New("远程账号 ID 读回不一致")
	}
	if textValue(value["name"]) != name {
		return false, errors.New("远程账号名称读回不一致")
	}
	effective, ok := value["schedulable"].(bool)
	if !ok || effective != schedulable {
		return false, errors.New("远程账号调度状态读回不一致")
	}
	groups, err := stableIDs(value["group_ids"])
	if err != nil || strings.Join(groups, ",") != strings.Join(groupIDs, ",") {
		return false, errors.New("远程账号分组读回不一致")
	}
	actual, err := positiveDecimal(textValue(firstPresent(value, "rate_multiplier", "multiplier")))
	if err != nil || actual != multiplier {
		return false, errors.New("远程账号倍率读回不一致")
	}
	return effective, nil
}

func stableIDs(value any) ([]string, error) {
	rows, ok := value.([]any)
	if !ok {
		return nil, errors.New("远程账号分组读回格式不可读")
	}
	result := make([]string, 0, len(rows))
	for _, raw := range rows {
		id := textValue(raw)
		if !stableID(id) {
			return nil, errors.New("远程账号分组读回包含无效稳定 ID")
		}
		result = append(result, id)
	}
	sort.Strings(result)
	return result, nil
}

func accountPlatform(request Request, candidate business.OnboardingCandidate, local business.LocalOnboardingGroup) (string, error) {
	if request.PlatformPresent {
		if request.Platform == nil || strings.TrimSpace(*request.Platform) == "" {
			return "", errors.New("平台不能为空")
		}
		return normalizePlatform(*request.Platform), nil
	}
	if candidate.Platform != nil && strings.TrimSpace(*candidate.Platform) != "" {
		return normalizePlatform(*candidate.Platform), nil
	}
	identity := strings.ToLower(candidate.GroupName + " " + local.Name)
	for _, item := range []struct {
		markers  []string
		platform string
	}{
		{[]string{"gemini"}, "gemini"}, {[]string{"grok"}, "grok"}, {[]string{"deepseek"}, "deepseek"},
		{[]string{"glm", "zhipu"}, "zhipu"}, {[]string{"claude", "ccmax", "kiro", "anthropic"}, "anthropic"},
		{[]string{"codex", "pro", "gpt", "openai"}, "openai"},
	} {
		for _, marker := range item.markers {
			if strings.Contains(identity, marker) {
				return item.platform, nil
			}
		}
	}
	return "", errors.New("上游分组目录缺少平台且无法从已选分组识别")
}

func normalizePlatform(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "sub2api" || value == "newapi" || value == "oneapi" {
		return "openai"
	}
	return value
}

func creationRemark(candidate business.OnboardingCandidate, supplied *string) string {
	description := "未提供"
	if candidate.Description != nil && strings.TrimSpace(*candidate.Description) != "" {
		description = strings.ReplaceAll(strings.Join(strings.Fields(*candidate.Description), " "), "|", "/")
	}
	value := fmt.Sprintf("【添加账号】：%s，分组：%s | 介绍：%s", time.Now().UTC().Truncate(time.Second).Format(time.RFC3339), candidate.GroupName, description)
	if supplied != nil && strings.TrimSpace(*supplied) != "" {
		value += "\n" + strings.TrimSpace(*supplied)
	}
	return value
}

func positiveDecimal(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 128 || strings.ContainsAny(value, "eE") {
		return "", errors.New("倍率必须是有效的有限正数")
	}
	parsed, ok := new(big.Rat).SetString(value)
	if !ok || parsed.Sign() <= 0 {
		return "", errors.New("倍率必须大于 0")
	}
	text := strings.TrimRight(strings.TrimRight(parsed.FloatString(28), "0"), ".")
	if text == "" {
		return "", errors.New("倍率必须大于 0")
	}
	return text, nil
}

func stableID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value[0] == '0' {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func mustInt64(value string) int64 {
	result := int64(0)
	for _, character := range value {
		result = result*10 + int64(character-'0')
	}
	return result
}

func operationID() (string, error) {
	id, err := randomID(8)
	if err != nil {
		return "", err
	}
	return "account-onboarding-" + id, nil
}

func randomID(bytesCount int) (string, error) {
	value := make([]byte, bytesCount)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func redactSecret(err error, secret string) error {
	if err == nil {
		return nil
	}
	return errors.New(strings.ReplaceAll(err.Error(), secret, "[已隐藏]"))
}

func safeError(err error) string {
	if err == nil {
		return "账号添加失败"
	}
	value := strings.TrimSpace(err.Error())
	if len(value) > 500 {
		value = value[:500]
	}
	return value
}

func firstPresent(value map[string]any, fields ...string) any {
	for _, field := range fields {
		if item, found := value[field]; found {
			return item
		}
	}
	return nil
}

func textValue(value any) string {
	if value == nil {
		return ""
	}
	if number, ok := value.(json.Number); ok {
		return number.String()
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func pointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
