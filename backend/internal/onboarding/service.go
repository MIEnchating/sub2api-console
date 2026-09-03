package onboarding

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
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
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/naming"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

const onboardingAuditTimeout = 5 * time.Second

type Repository interface {
	OnboardingCandidates(context.Context, string) ([]business.OnboardingCandidate, error)
	ProtectedUpstreamKeyIDs(context.Context, string) ([]string, error)
	UpstreamKeyProtected(context.Context, string, string) (bool, error)
	ReconcileDeletedUnboundUpstreamKeyProjection(context.Context, string, string) error
	LocalOnboardingGroup(context.Context, string) (business.LocalOnboardingGroup, error)
	PendingOnboarding(context.Context, string, string, []string) (*business.PendingOnboarding, error)
	SavePendingOnboarding(context.Context, business.PendingOnboarding) error
	CommitOnboardingProjection(context.Context, business.OnboardingProjection) error
	CommitAccountGroupsReadback(context.Context, string, []business.LocalOnboardingGroup, *string, business.AccountOperation) error
	RecordAccountOperation(context.Context, business.AccountOperation) error
	RecordRuntimeEvent(context.Context, string, string, string, map[string]any) (int64, error)
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

type reconcilingKeyClient interface {
	ReconcileCreatedKey(context.Context, configstore.AuthRecord, string, string) (upstreamsync.CreatedKey, bool, error)
}

type TaskStore interface {
	Save(context.Context, taskstore.Task) error
}

type Request struct {
	Host            string
	UpstreamType    string
	BaseURL         *string
	PlatformPresent bool
	Platform        *string
	AccountType     *string
	Notes           *string
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
	taskRunner taskrunner.Runner
	timeout    time.Duration
}

const schedulableWriteTimeout = 2 * time.Second

type validatedRequest struct {
	request        Request
	accountBaseURL string
	multiplier     string
	locals         []business.LocalOnboardingGroup
	candidate      business.OnboardingCandidate
	auth           configstore.AuthRecord
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

func (s *Service) UseTaskRunner(runner taskrunner.Runner) { s.taskRunner = runner }

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
	expectedTarget, err := s.private.TargetSettings(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	task, err := s.newQueuedTask("onboard", "账号绑定变更已排队")
	if err != nil {
		return taskstore.Task{}, err
	}
	if err := s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	if err := taskrunner.Go(s.taskRunner, func(parent context.Context) {
		s.execute(targetguard.Expect(parent, expectedTarget), task, request)
	}); err != nil {
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
	return task, nil
}

func (s *Service) EnqueueBatch(ctx context.Context, requests []Request) (taskstore.Task, error) {
	if len(requests) == 0 {
		return taskstore.Task{}, errors.New("请至少选择一个要添加的账号")
	}
	if len(requests) > 50 {
		return taskstore.Task{}, errors.New("单次最多添加 50 个账号")
	}
	requests = expandBatchRequests(requests)
	if len(requests) > 50 {
		return taskstore.Task{}, errors.New("单次最多添加或更新 50 个账号")
	}
	items := make([]batchItem, 0, len(requests))
	seen := map[string]struct{}{}
	for _, request := range requests {
		validated, err := s.validate(ctx, request)
		if err != nil {
			return taskstore.Task{}, err
		}
		keyParts := []string{
			strings.ToLower(strings.TrimSpace(validated.auth.Host)), validated.candidateID(),
		}
		if len(validated.request.AccountIDs) == 0 {
			keyParts = append(keyParts, strings.Join(onboardingLocalIDs(validated.locals), ","))
		}
		key := strings.Join(keyParts, "\x00")
		if _, found := seen[key]; found {
			return taskstore.Task{}, errors.New("同一个账号绑定目标不能在一个批次中重复提交")
		}
		seen[key] = struct{}{}
		items = append(items, batchItem{
			request: request, upstreamGroupName: validated.candidate.GroupName,
			localGroupName: strings.Join(onboardingLocalNames(validated.locals), "、"), action: onboardingAction(validated),
		})
	}
	expectedTarget, err := s.private.TargetSettings(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	task, err := s.newQueuedTask("onboard-batch", fmt.Sprintf("%d 项账号绑定变更已排队", len(items)))
	if err != nil {
		return taskstore.Task{}, err
	}
	if err := s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	if err := taskrunner.Go(s.taskRunner, func(parent context.Context) {
		s.executeBatch(targetguard.Expect(parent, expectedTarget), task, items)
	}); err != nil {
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
	return task, nil
}

func expandBatchRequests(requests []Request) []Request {
	result := make([]Request, 0, len(requests))
	for _, request := range requests {
		localGroupIDs := request.LocalGroupIDs
		if len(localGroupIDs) == 0 && strings.TrimSpace(request.LocalGroupID) != "" {
			localGroupIDs = []string{request.LocalGroupID}
		}
		if len(request.AccountIDs) > 0 || len(localGroupIDs) <= 1 {
			result = append(result, request)
			continue
		}
		for _, localGroupID := range localGroupIDs {
			expanded := request
			expanded.LocalGroupID = localGroupID
			expanded.LocalGroupIDs = []string{localGroupID}
			result = append(result, expanded)
		}
	}
	return result
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

func (s *Service) execute(parent context.Context, task taskstore.Task, request Request) {
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()
	task.Status, task.Progress, task.Message, task.UpdatedAt = "running", 10, "正在校验稳定 ID 与上游分组", time.Now().UTC().Format(time.RFC3339Nano)
	if !taskstore.SaveRunning(ctx, s.tasks, task) {
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
	taskstore.MarkCancelled(ctx, &task, "账号绑定变更已取消")
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) executeBatch(parent context.Context, task taskstore.Task, items []batchItem) {
	ctx, cancel := context.WithTimeout(parent, s.timeout*time.Duration(len(items)))
	defer cancel()
	task.Status, task.Progress, task.Message = "running", 5, fmt.Sprintf("正在添加 1/%d 个账号", len(items))
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if !taskstore.SaveRunning(ctx, s.tasks, task) {
		return
	}
	results := make([]map[string]any, 0, len(items))
	succeeded := 0
	failed := 0
	for index, item := range items {
		if ctx.Err() != nil {
			break
		}
		task.Progress = 5 + index*90/len(items)
		task.Message = fmt.Sprintf("正在处理 %d/%d：%s，%s → %s", index+1, len(items), item.action, item.upstreamGroupName, item.localGroupName)
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		taskstore.PersistProgress(s.tasks, task)
		itemResult, err := s.Onboard(ctx, item.request)
		row := map[string]any{
			"upstream_group": item.upstreamGroupName,
			"local_group":    item.localGroupName,
			"action":         item.action,
		}
		mergeBatchItemResult(row, itemResult)
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
	if ctx.Err() != nil {
		task.Status = "failed"
	}
	taskstore.MarkCancelled(ctx, &task, "批量账号绑定变更已取消")
	taskstore.PersistFinal(s.tasks, task)
}

func mergeBatchItemResult(row, result map[string]any) {
	if remoteWrite, present := result["remote_write"]; present {
		row["remote_write"] = remoteWrite
	}
	if pending, present := result["pending"]; present && pending != nil {
		row["pending"] = pending
	}
}

func (s *Service) Onboard(ctx context.Context, request Request) (map[string]any, error) {
	ctx, err := targetguard.Capture(ctx, s.private)
	if err != nil {
		return map[string]any{"remote_write": false}, err
	}
	validated, err := s.validate(ctx, request)
	if err != nil {
		return map[string]any{"remote_write": false}, err
	}
	lockedHost := configstore.CanonicalHost(validated.auth.Host)
	lockedUpstreamID := strings.TrimSpace(validated.candidate.UpstreamID)
	lockedAccountIDs := append([]string{}, validated.request.AccountIDs...)
	resources := onboardingMutationResources(lockedHost, lockedAccountIDs)
	guardedCtx, releaseMutation, err := mutationguard.Acquire(ctx, s.repository, resources...)
	if err != nil {
		return map[string]any{"remote_write": false}, err
	}
	defer func() {
		if err := releaseMutation(); err != nil {
			slog.Error("账号添加变更租约释放失败", "host", lockedHost, "error", err)
		}
	}()
	ctx = guardedCtx
	validated, err = s.validate(ctx, request)
	if err != nil {
		return map[string]any{"remote_write": false}, fmt.Errorf("获取变更租约后重新校验失败：%w", err)
	}
	if lockedHost != configstore.CanonicalHost(validated.auth.Host) ||
		lockedUpstreamID != strings.TrimSpace(validated.candidate.UpstreamID) ||
		strings.Join(lockedAccountIDs, "\x00") != strings.Join(validated.request.AccountIDs, "\x00") {
		return map[string]any{"remote_write": false}, errors.New("获取变更租约后开户目标身份已变化，请重试")
	}
	ctx, err = targetguard.Pin(ctx, s.private)
	if err != nil {
		return map[string]any{"remote_write": false}, err
	}
	if len(validated.request.AccountIDs) > 0 {
		return s.updateAccountGroups(ctx, validated)
	}
	primaryLocal := validated.locals[0]
	accountName := naming.AccountName(validated.candidate.UpstreamName, validated.accountBaseURL, validated.multiplier)
	platform, err := accountPlatform(validated.request, validated.candidate, validated.locals)
	if err != nil {
		return map[string]any{"remote_write": false}, err
	}
	accountType := "apikey"
	if validated.request.AccountType != nil {
		accountType = strings.ToLower(strings.TrimSpace(*validated.request.AccountType))
		if accountType == "" {
			return map[string]any{"remote_write": false}, errors.New("账号类型不能为空")
		}
	}
	defaults, err := s.private.AccountDefaults(ctx)
	if err != nil {
		return map[string]any{"remote_write": false}, fmt.Errorf("账号默认参数读取失败：%w", err)
	}
	priority, concurrency := accountCreationParameters(defaults, validated.request)
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return map[string]any{"remote_write": false}, err
	}
	intentHash, err := onboardingIntentHash(validated, target.BaseURL, accountName, platform, accountType, priority, concurrency)
	if err != nil {
		return map[string]any{"remote_write": false}, err
	}
	operationID, err := operationID()
	if err != nil {
		return map[string]any{"remote_write": false}, err
	}
	localGroupIDs := onboardingLocalIDs(validated.locals)
	pending, err := s.repository.PendingOnboarding(ctx, validated.auth.Host, validated.candidateID(), localGroupIDs)
	if err != nil {
		return map[string]any{"remote_write": false}, err
	}
	if pending != nil {
		remoteWritten := pendingRemoteWrite(*pending)
		if strings.TrimSpace(pending.IntentHash) == "" {
			return map[string]any{"remote_write": remoteWritten, "pending": pendingResult(*pending)}, errors.New("待续开户记录缺少首次冻结意图，已拒绝远端写入")
		}
		if pending.IntentHash != intentHash {
			return map[string]any{"remote_write": remoteWritten, "pending": pendingResult(*pending)}, errors.New("续办参数与首次冻结的开户意图不一致，已拒绝远端写入")
		}
		operationID = pending.OperationID
	}
	keyMarker := operationID
	if pending == nil {
		pending = &business.PendingOnboarding{
			OperationID: operationID, UpstreamHost: validated.auth.Host, UpstreamType: validated.request.UpstreamType,
			UpstreamKeyName: &keyMarker, UpstreamGroupID: validated.candidateID(), UpstreamGroupName: validated.candidate.GroupName,
			LocalGroupID: primaryLocal.ID, LocalGroupName: primaryLocal.Name, LocalGroupIDs: localGroupIDs, Multiplier: validated.multiplier,
			IntentHash: intentHash, Reason: "开户创建意图已保存", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}
		if err := s.repository.SavePendingOnboarding(ctx, *pending); err != nil {
			return map[string]any{"remote_write": false}, fmt.Errorf("开户创建意图保存失败：%w", err)
		}
	}
	var key upstreamsync.CreatedKey
	keyCreatedNow := false
	keyWasUnknown := pending.KeyCommitUnknown
	if pending.UpstreamKeyID != "" {
		key, err = s.keys.RevealKey(ctx, validated.auth, pending.UpstreamKeyID, validated.candidateID())
	} else if pending.KeyCommitUnknown {
		reconciler, supported := s.keys.(reconcilingKeyClient)
		if !supported {
			err = &upstreamsync.CommitUnknownError{Marker: keyMarker, Cause: errors.New("当前 Key 客户端不支持只读 marker 对账")}
		} else {
			var found bool
			key, found, err = reconciler.ReconcileCreatedKey(ctx, validated.auth, keyMarker, validated.candidateID())
			if err == nil && !found {
				err = &upstreamsync.CommitUnknownError{Marker: keyMarker, Cause: errors.New("Key marker 尚未在上游目录中可见")}
			}
		}
	} else {
		pending.KeyCommitUnknown = true
		pending.Reason = "准备按不可变 marker 创建上游 Key"
		if err := s.repository.SavePendingOnboarding(ctx, *pending); err != nil {
			return map[string]any{"remote_write": false, "pending": pendingResult(*pending)}, fmt.Errorf("Key 创建前状态保存失败：%w", err)
		}
		key, err = createKey(ctx, s.keys, validated.auth, keyMarker, validated.candidateID(), true)
		keyCreatedNow = err == nil
	}
	if err != nil {
		if keyWasUnknown {
			pending.KeyCommitUnknown = true
		} else {
			var unknown *upstreamsync.CommitUnknownError
			pending.KeyCommitUnknown = errors.As(err, &unknown)
		}
		pending.Reason = safeError(err)
		if saveErr := s.repository.SavePendingOnboarding(ctx, *pending); saveErr != nil {
			err = fmt.Errorf("%w；待续状态保存失败：%v", err, saveErr)
		}
		remoteWritten := pendingRemoteWrite(*pending)
		if auditErr := s.recordFailure(ctx, operationID, validated, "remote-write", remoteWritten, err); auditErr != nil {
			err = fmt.Errorf("%w；账号添加失败审计保存失败：%v", err, auditErr)
		}
		result := map[string]any{"remote_write": remoteWritten, "pending": pendingResult(*pending)}
		if pending.KeyCommitUnknown {
			return result, fmt.Errorf("%w；提交结果不确定，后续续办只会按 marker 对账，不会再次创建", err)
		}
		return result, fmt.Errorf("%w；开户创建意图已保存，可在明确失败后安全重试", err)
	}
	pending.UpstreamKeyID = key.KeyID
	pending.UpstreamKeyName = &key.Name
	pending.KeyCommitUnknown = false
	pending.Reason = "上游 Key 已按稳定 ID 确认"
	if err := s.repository.SavePendingOnboarding(ctx, *pending); err != nil {
		return map[string]any{"remote_write": true, "pending": pendingResult(*pending)}, fmt.Errorf("上游 Key 已确认但待续状态保存失败：%w", err)
	}
	result := map[string]any{"remote_write": true, "upstream_key_created": keyCreatedNow}
	if err := s.private.SaveUpstreamKeySecret(ctx, configstore.UpstreamKeySecret{
		Host: validated.auth.Host, KeyID: key.KeyID, GroupID: validated.candidateID(), Secret: key.Secret,
	}); err != nil {
		return s.pendingFailure(ctx, validated, pending, result, fmt.Errorf("本地 Key 保存失败：%w", err))
	}
	accountMarker := accountCreationMarker(operationID)
	remark := creationRemark(validated.candidate, validated.request.Notes, accountMarker, pending.CreatedAt)
	client, err := adminclient.New(adminclient.Config{
		BaseURL: target.BaseURL, AdminKey: target.AdminKey,
		Timeout: time.Duration(target.TimeoutSeconds) * time.Second, Attempts: 1,
	}, nil)
	if err != nil {
		return s.pendingFailure(ctx, validated, pending, result, err)
	}
	localGroupNumericIDs, err := onboardingLocalNumericIDs(validated.locals)
	if err != nil {
		return s.pendingFailure(ctx, validated, pending, result, err)
	}
	models, err := client.PreviewAccountModels(ctx, platform, accountType, validated.accountBaseURL, key.Secret)
	if err != nil {
		return s.pendingFailure(ctx, validated, pending, result, fmt.Errorf("开户模型同步失败：%w", redactSecret(err, key.Secret)))
	}
	body := map[string]any{
		"name": accountName, "notes": remark, "platform": platform, "type": accountType,
		"credentials": map[string]any{
			"api_key": key.Secret, "base_url": validated.accountBaseURL, "model_mapping": identityModelMapping(models),
		}, "extra": validated.request.Extra,
		"rate_multiplier": json.Number(validated.multiplier), "group_ids": localGroupNumericIDs,
		"concurrency": concurrency, "priority": priority, "schedulable": validated.request.Schedulable,
		"auto_pause_on_expired": true,
	}
	var created map[string]any
	accountWasUnknown := pending.AccountCommitUnknown
	if pending.UpstreamAccountID != "" {
		created = map[string]any{"id": pending.UpstreamAccountID}
	} else if pending.AccountCommitUnknown {
		var found bool
		created, found, err = client.ReconcileAccountWithMarker(ctx, accountMarker)
		if err == nil && !found {
			err = &adminclient.CommitUnknownError{Marker: accountMarker, Cause: errors.New("账号 marker 尚未在管理目录中可见")}
		}
	} else {
		pending.AccountCommitUnknown = true
		pending.Reason = "准备按不可变 marker 创建管理账号"
		if err := s.repository.SavePendingOnboarding(ctx, *pending); err != nil {
			return result, fmt.Errorf("账号创建前状态保存失败：%w", err)
		}
		created, err = client.CreateAccountWithMarker(ctx, body, accountMarker)
	}
	if err != nil {
		if accountWasUnknown {
			pending.AccountCommitUnknown = true
		} else {
			var unknown *adminclient.CommitUnknownError
			pending.AccountCommitUnknown = errors.As(err, &unknown)
		}
		return s.pendingFailure(ctx, validated, pending, result, redactSecret(err, key.Secret))
	}
	accountID := textValue(firstPresent(created, "id", "account_id"))
	if !stableID(accountID) {
		pending.AccountCommitUnknown = true
		return s.pendingFailure(ctx, validated, pending, result, errors.New("账号创建结果缺少稳定 ID，已停止后续写入"))
	}
	pending.UpstreamAccountID = accountID
	pending.AccountCommitUnknown = false
	pending.Reason = "管理账号已按稳定 ID 确认"
	if err := s.repository.SavePendingOnboarding(ctx, *pending); err != nil {
		return result, fmt.Errorf("管理账号 %s 已确认但待续状态保存失败：%w", accountID, err)
	}
	readbackConfirmed := false
	if completeCreatedAccountResponse(created) {
		if err := verifyCreatedAccount(created, accountID, accountName, platform, accountType, accountMarker, onboardingLocalIDs(validated.locals), validated.multiplier, priority, concurrency); err != nil {
			return s.pendingFailure(ctx, validated, pending, result, err)
		}
		if value, ok := created["schedulable"].(bool); ok && value == validated.request.Schedulable {
			readbackConfirmed = true
		}
	}
	// Current targets persist schedulable during creation. Older targets omit it,
	// so retain one short compatibility write but never wait for a cache readback.
	createdSchedulable, schedulablePresent := created["schedulable"].(bool)
	if !schedulablePresent || createdSchedulable != validated.request.Schedulable {
		writeCtx, cancelWrite := context.WithTimeout(ctx, schedulableWriteTimeout)
		scheduleResponse, schedulableWriteErr := client.SetAccountSchedulable(writeCtx, accountID, validated.request.Schedulable)
		cancelWrite()
		if schedulableWriteErr != nil {
			result["schedulable_warning"] = "账号已创建，调度状态兼容写未确认：" + safeError(redactSecret(schedulableWriteErr, key.Secret))
		} else if completeCreatedAccountResponse(scheduleResponse) {
			if verifyErr := verifyCreatedAccount(scheduleResponse, accountID, accountName, platform, accountType, accountMarker, onboardingLocalIDs(validated.locals), validated.multiplier, priority, concurrency); verifyErr == nil {
				value, ok := scheduleResponse["schedulable"].(bool)
				readbackConfirmed = ok && value == validated.request.Schedulable
			}
		}
	}
	projection := business.OnboardingProjection{
		OperationID: operationID, AccountID: accountID, AccountName: accountName,
		Platform:     platform,
		UpstreamHost: validated.auth.Host, UpstreamType: validated.request.UpstreamType, BaseURL: validated.accountBaseURL,
		UpstreamKeyID: key.KeyID, UpstreamKeyName: key.Name, UpstreamGroupID: validated.candidateID(),
		UpstreamGroupName: validated.candidate.GroupName, LocalGroupID: primaryLocal.ID,
		LocalGroupName: primaryLocal.Name, LocalGroups: validated.locals, Multiplier: validated.multiplier, Schedulable: validated.request.Schedulable,
		Priority: &priority, Concurrency: &concurrency, Models: models, Notes: remark, Actor: validated.request.Actor, ReadbackConfirmed: readbackConfirmed,
	}
	if err := s.repository.CommitOnboardingProjection(ctx, projection); err != nil {
		return s.pendingFailure(ctx, validated, pending, result, err)
	}
	result["operation_id"] = operationID
	result["account_id"] = accountID
	result["account_name"] = accountName
	result["local_group_ids"] = onboardingLocalIDs(validated.locals)
	result["local_group_names"] = onboardingLocalNames(validated.locals)
	result["upstream_group_id"] = validated.candidateID()
	result["upstream_group_name"] = validated.candidate.GroupName
	result["schedulable"] = validated.request.Schedulable
	result["credentials"] = "已保存到 Console 私有配置库"
	result["readback_confirmed"] = readbackConfirmed
	result["concurrency"] = concurrency
	result["priority"] = priority
	result["model_count"] = len(models)
	return result, nil
}

func identityModelMapping(models []string) map[string]string {
	mapping := make(map[string]string, len(models))
	for _, model := range models {
		mapping[model] = model
	}
	return mapping
}

func onboardingMutationResources(host string, accountIDs []string) []string {
	resources := make([]string, 0, len(accountIDs)+2)
	if len(accountIDs) == 0 {
		return append(resources, mutationguard.UpstreamKeyCatalog(host))
	} else {
		for _, accountID := range accountIDs {
			resources = append(resources, mutationguard.Account(accountID))
		}
	}
	return resources
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

type frozenLocalGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type frozenOnboardingIntent struct {
	UpstreamHost        string             `json:"upstream_host"`
	UpstreamType        string             `json:"upstream_type"`
	UpstreamBaseURL     string             `json:"upstream_base_url"`
	AccountBaseURL      string             `json:"account_base_url"`
	UpstreamGroupID     string             `json:"upstream_group_id"`
	UpstreamGroupName   string             `json:"upstream_group_name"`
	UpstreamDescription string             `json:"upstream_description"`
	TargetBaseURL       string             `json:"target_base_url"`
	AccountName         string             `json:"account_name"`
	AccountType         string             `json:"account_type"`
	Platform            string             `json:"platform"`
	Notes               string             `json:"notes"`
	Extra               map[string]any     `json:"extra"`
	LocalGroups         []frozenLocalGroup `json:"local_groups"`
	Multiplier          string             `json:"multiplier"`
	Priority            int64              `json:"priority"`
	Concurrency         int64              `json:"concurrency"`
	Schedulable         bool               `json:"schedulable"`
}

func onboardingIntentHash(validated validatedRequest, targetBaseURL, accountName, platform, accountType string, priority, concurrency int64) (string, error) {
	locals := make([]frozenLocalGroup, 0, len(validated.locals))
	for _, local := range validated.locals {
		locals = append(locals, frozenLocalGroup{ID: local.ID, Name: local.Name})
	}
	sort.Slice(locals, func(left, right int) bool { return numericIDLess(locals[left].ID, locals[right].ID) })
	notes := ""
	if validated.request.Notes != nil {
		notes = strings.TrimSpace(*validated.request.Notes)
	}
	description := ""
	if validated.candidate.Description != nil {
		description = strings.Join(strings.Fields(*validated.candidate.Description), " ")
	}
	intent := frozenOnboardingIntent{
		UpstreamHost: strings.ToLower(strings.TrimSpace(validated.auth.Host)), UpstreamType: strings.ToLower(strings.TrimSpace(validated.request.UpstreamType)),
		UpstreamBaseURL: strings.TrimRight(strings.TrimSpace(validated.auth.BaseURL), "/"), UpstreamGroupID: validated.candidateID(),
		UpstreamGroupName: validated.candidate.GroupName, UpstreamDescription: description, AccountBaseURL: validated.accountBaseURL,
		TargetBaseURL: strings.TrimRight(strings.TrimSpace(targetBaseURL), "/"),
		AccountName:   accountName, AccountType: accountType, Platform: platform, Notes: notes, Extra: validated.request.Extra,
		LocalGroups: locals, Multiplier: validated.multiplier, Priority: priority, Concurrency: concurrency, Schedulable: validated.request.Schedulable,
	}
	encoded, err := json.Marshal(intent)
	if err != nil {
		return "", errors.New("开户意图编码失败")
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func createKey(ctx context.Context, client KeyClient, record configstore.AuthRecord, name, groupID string, verification bool) (upstreamsync.CreatedKey, error) {
	if configurable, ok := client.(configurableKeyClient); ok {
		return configurable.CreateKeyWithVerification(ctx, record, name, groupID, verification)
	}
	return client.CreateKey(ctx, record, name, groupID)
}

func (s *Service) validate(ctx context.Context, request Request) (validatedRequest, error) {
	if request.Priority != nil && (*request.Priority < 1 || *request.Priority > 10_000_000) {
		return validatedRequest{}, errors.New("优先级必须是 1 到 10000000 之间的整数")
	}
	if request.Concurrency != nil && (*request.Concurrency < 1 || *request.Concurrency > 10_000_000) {
		return validatedRequest{}, errors.New("并发必须是 1 到 10000000 之间的整数")
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
		if !stableID(local.ID) {
			return validatedRequest{}, errors.New("本地分组稳定 ID 超出支持范围")
		}
		seenLocalGroups[localGroupID] = struct{}{}
		locals = append(locals, local)
	}
	sort.Slice(locals, func(left, right int) bool { return numericIDLess(locals[left].ID, locals[right].ID) })
	auth, err := s.private.AuthRecord(ctx, request.Host)
	if err != nil {
		return validatedRequest{}, err
	}
	if auth == nil {
		return validatedRequest{}, errors.New("账号添加前必须先配置该 Host 的鉴权记录")
	}
	accountBaseURL := auth.BaseURL
	if request.BaseURL != nil {
		accountBaseURL, err = configstore.ValidateBaseURL(*request.BaseURL)
		if err != nil {
			return validatedRequest{}, fmt.Errorf("账号 Base URL 无效：%w", err)
		}
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
	if candidate.Multiplier == nil {
		return validatedRequest{}, errors.New("上游分组缺少换算后的账号成本")
	}
	multiplier, err := positiveDecimal(*candidate.Multiplier)
	if err != nil {
		return validatedRequest{}, errors.New("上游分组换算后的账号成本无效")
	}
	accountIDs, err := validatedBoundAccountIDs(request.AccountIDs, *candidate)
	if err != nil {
		return validatedRequest{}, err
	}
	if len(accountIDs) == 0 && len(locals) != 1 {
		return validatedRequest{}, errors.New("每个新增账号只能绑定一个本地分组，多选分组请使用批量添加")
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
	validated := validatedRequest{
		request: request, accountBaseURL: accountBaseURL, multiplier: multiplier,
		locals: locals, candidate: *candidate, auth: *auth,
	}
	platform, err := accountPlatform(request, *candidate, locals)
	if err != nil {
		return validatedRequest{}, err
	}
	if err := validateLocalGroupPlatforms(platform, *candidate, locals); err != nil {
		return validatedRequest{}, err
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
	sort.Slice(result, func(left, right int) bool { return numericIDLess(result[left], result[right]) })
	return result
}

func numericIDLess(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if len(left) == len(right) {
		return left < right
	}
	return len(left) < len(right)
}

func onboardingLocalNumericIDs(groups []business.LocalOnboardingGroup) ([]int64, error) {
	ids := onboardingLocalIDs(groups)
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		parsed, err := strconv.ParseInt(id, 10, 64)
		if err != nil || parsed <= 0 {
			return nil, errors.New("本地分组稳定 ID 超出支持范围")
		}
		result = append(result, parsed)
	}
	return result, nil
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
	target, err := targetguard.Settings(ctx, s.private)
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
	desiredNumericIDs, err := onboardingLocalNumericIDs(validated.locals)
	if err != nil {
		return map[string]any{"remote_write": false}, err
	}
	results := make([]map[string]any, 0, len(validated.request.AccountIDs))
	remoteWritten := false
	result := func(readbackConfirmed bool) map[string]any {
		return map[string]any{
			"operation": "account.groups", "remote_write": remoteWritten, "readback_confirmed": readbackConfirmed,
			"upstream_group_id": validated.candidateID(), "upstream_group_name": validated.candidate.GroupName,
			"local_group_ids": desiredIDs, "local_group_names": onboardingLocalNames(validated.locals),
			"base_url": validated.request.BaseURL, "accounts": results,
		}
	}
	for _, accountID := range validated.request.AccountIDs {
		before, err := client.Account(ctx, accountID)
		if err != nil {
			return result(false), err
		}
		beforeIDs, err := stableIDs(before["group_ids"])
		if err != nil {
			return result(false), errors.New("管理平台账号当前分组不可读")
		}
		operationID, err := operationID()
		if err != nil {
			return result(false), err
		}
		accountName := textValue(before["name"])
		field := "group_ids"
		beforeValue, afterValue := any(beforeIDs), any(desiredIDs)
		if validated.request.BaseURL != nil {
			field = "group_ids,credentials.base_url"
			beforeValue = map[string]any{"group_ids": beforeIDs, "base_url": onboardingAccountBaseURL(before)}
			afterValue = map[string]any{"group_ids": desiredIDs, "base_url": validated.accountBaseURL}
		}
		after, err := client.UpdateAccountGroupsAndBaseURL(ctx, accountID, desiredNumericIDs, validated.request.BaseURL)
		remoteWritten = true
		if err != nil {
			detail := safeError(err)
			operation := business.AccountOperation{
				OperationID: operationID, OperationType: "account.groups", State: "failed", Phase: "remote-readback",
				Actor: validated.request.Actor, Error: &detail, RemoteConfirmed: true, ReadbackConfirmed: false,
				ObjectID: accountID, ObjectName: &accountName, GroupNames: onboardingLocalNames(validated.locals),
				FieldName: &field, Before: beforeValue, After: afterValue, Writeback: true,
			}
			auditCtx, cancelAudit := context.WithTimeout(context.WithoutCancel(ctx), onboardingAuditTimeout)
			auditErr := s.repository.RecordAccountOperation(auditCtx, operation)
			cancelAudit()
			if auditErr != nil {
				return result(false), fmt.Errorf("%w；账号分组失败审计保存失败：%v", err, auditErr)
			}
			return result(false), err
		}
		confirmedIDs, err := stableIDs(after["group_ids"])
		if err != nil || strings.Join(confirmedIDs, ",") != strings.Join(desiredIDs, ",") {
			return result(false), errors.New("管理平台账号分组写后确认不一致")
		}
		operation := business.AccountOperation{
			OperationID: operationID, OperationType: "account.groups", State: "succeeded", Phase: "readback",
			Actor: validated.request.Actor, RemoteConfirmed: true, ReadbackConfirmed: true,
			ObjectID: accountID, ObjectName: &accountName, GroupNames: onboardingLocalNames(validated.locals),
			FieldName: &field, Before: beforeValue, After: afterValue, Writeback: true,
		}
		if err := s.repository.CommitAccountGroupsReadback(ctx, accountID, validated.locals, validated.request.BaseURL, operation); err != nil {
			return result(false), errors.New("管理平台分组已更新，但本地绑定提交失败：" + err.Error())
		}
		results = append(results, map[string]any{"account_id": accountID, "before": beforeIDs, "after": confirmedIDs})
	}
	return result(true), nil
}

func onboardingAccountBaseURL(account map[string]any) string {
	raw := account["base_url"]
	if credentials, ok := account["credentials"].(map[string]any); ok {
		if value, present := credentials["base_url"]; present {
			raw = value
		}
	}
	if raw == nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(fmt.Sprint(raw)), "/")
}

func (s *Service) pendingFailure(ctx context.Context, validated validatedRequest, pending *business.PendingOnboarding, result map[string]any, cause error) (map[string]any, error) {
	reason := safeError(cause)
	pending.Reason = reason
	if err := s.repository.SavePendingOnboarding(ctx, *pending); err != nil {
		reason += "；待续记录保存失败：" + safeError(err)
	}
	remoteWritten := pendingRemoteWrite(*pending)
	if auditErr := s.recordFailure(ctx, pending.OperationID, validated, "remote-readback", remoteWritten, errors.New(reason)); auditErr != nil {
		reason += "；账号添加失败审计保存失败：" + safeError(auditErr)
	}
	result["remote_write"] = remoteWritten
	result["pending"] = pendingResult(*pending)
	return result, errors.New(reason + "；上游 Key 已创建，已保存待续记录并禁止重复创建")
}

func pendingRemoteWrite(pending business.PendingOnboarding) bool {
	return strings.TrimSpace(pending.UpstreamKeyID) != "" ||
		strings.TrimSpace(pending.UpstreamAccountID) != "" ||
		pending.KeyCommitUnknown || pending.AccountCommitUnknown
}

func pendingResult(pending business.PendingOnboarding) map[string]any {
	return map[string]any{
		"operation_id": pending.OperationID, "upstream_host": pending.UpstreamHost,
		"upstream_key_id": pending.UpstreamKeyID, "upstream_account_id": pending.UpstreamAccountID,
		"upstream_group_id": pending.UpstreamGroupID, "local_group_id": pending.LocalGroupID, "local_group_ids": pending.LocalGroupIDs,
		"key_commit_unknown": pending.KeyCommitUnknown, "account_commit_unknown": pending.AccountCommitUnknown,
	}
}

func (s *Service) recordFailure(ctx context.Context, operationID string, validated validatedRequest, phase string, remote bool, cause error) error {
	field, name, reason := "created", naming.AccountName(validated.candidate.UpstreamName, validated.accountBaseURL, validated.multiplier), safeError(cause)
	operation := business.AccountOperation{
		OperationID: operationID, OperationType: "account.onboarding", State: "failed", Phase: phase,
		Actor: validated.request.Actor, Error: &reason, RemoteConfirmed: remote, ReadbackConfirmed: false,
		ObjectID: "", ObjectName: &name, GroupNames: onboardingLocalNames(validated.locals), FieldName: &field,
		After: map[string]any{"name": name, "group_ids": onboardingLocalIDs(validated.locals), "rate_multiplier": validated.multiplier}, Writeback: true,
	}
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), onboardingAuditTimeout)
	defer cancel()
	if err := s.repository.RecordAccountOperation(auditCtx, operation); err != nil {
		slog.Error("账号添加失败记录保存失败", "operation_id", operationID, "host", validated.auth.Host, "error", err)
		return err
	}
	return nil
}

func (v validatedRequest) candidateID() string { return pointerValue(v.candidate.GroupID) }

func verifyCreatedAccount(value map[string]any, accountID, name, platform, accountType, marker string, groupIDs []string, multiplier string, priority, concurrency int64) error {
	if textValue(firstPresent(value, "id", "account_id")) != accountID {
		return errors.New("管理账号完整读回的稳定 ID 不一致")
	}
	if textValue(value["name"]) != name {
		return errors.New("管理账号完整读回的名称不一致")
	}
	if strings.ToLower(textValue(value["platform"])) != platform {
		return errors.New("管理账号完整读回的平台不一致")
	}
	if strings.ToLower(textValue(value["type"])) != accountType {
		return errors.New("管理账号完整读回的类型不一致")
	}
	if !markerLinePresent(textValue(value["notes"]), marker) {
		return errors.New("管理账号完整读回缺少开户 marker")
	}
	groups, err := stableIDs(value["group_ids"])
	if err != nil || strings.Join(groups, ",") != strings.Join(groupIDs, ",") {
		return errors.New("管理账号完整读回的分组不一致")
	}
	actualMultiplier, err := positiveDecimal(textValue(firstPresent(value, "rate_multiplier", "multiplier")))
	if err != nil || actualMultiplier != multiplier {
		return errors.New("管理账号完整读回的倍率不一致")
	}
	if textValue(value["priority"]) != strconv.FormatInt(priority, 10) {
		return errors.New("管理账号完整读回的优先级不一致")
	}
	if textValue(value["concurrency"]) != strconv.FormatInt(concurrency, 10) {
		return errors.New("管理账号完整读回的并发不一致")
	}
	return nil
}

func completeCreatedAccountResponse(value map[string]any) bool {
	for _, field := range []string{"id", "name", "platform", "type", "notes", "group_ids", "priority", "concurrency"} {
		if _, present := value[field]; !present {
			return false
		}
	}
	_, multiplierPresent := value["rate_multiplier"]
	if !multiplierPresent {
		_, multiplierPresent = value["multiplier"]
	}
	return multiplierPresent
}

func markerLinePresent(notes, marker string) bool {
	notes = strings.ReplaceAll(notes, "\r\n", "\n")
	for _, line := range strings.Split(notes, "\n") {
		if strings.TrimSpace(line) == marker {
			return true
		}
	}
	return false
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
	sort.Slice(result, func(left, right int) bool { return numericIDLess(result[left], result[right]) })
	return result, nil
}

func accountPlatform(request Request, candidate business.OnboardingCandidate, locals []business.LocalOnboardingGroup) (string, error) {
	if len(locals) == 0 || locals[0].Platform == nil || strings.TrimSpace(*locals[0].Platform) == "" {
		return "", errors.New("所选本地分组缺少平台，无法确定账号平台")
	}
	localPlatform := normalizePlatform(*locals[0].Platform)
	candidatePlatform := ""
	if candidate.Platform != nil {
		candidatePlatform = normalizePlatform(*candidate.Platform)
	}
	resolvedPlatform := localPlatform
	if candidatePlatform != "" {
		resolvedPlatform = candidatePlatform
	}
	if request.PlatformPresent {
		if request.Platform == nil || strings.TrimSpace(*request.Platform) == "" {
			return "", errors.New("平台不能为空")
		}
		requestedPlatform := normalizePlatform(*request.Platform)
		if requestedPlatform != resolvedPlatform {
			if candidatePlatform == "" {
				return "", fmt.Errorf("请求平台 %s 与所选本地分组平台 %s 不一致", requestedPlatform, localPlatform)
			}
			return "", fmt.Errorf(
				"请求平台 %s 与上游分组平台 %s 不一致，已拒绝覆盖目录平台",
				requestedPlatform,
				candidatePlatform,
			)
		}
		return requestedPlatform, nil
	}
	return resolvedPlatform, nil
}

func validateLocalGroupPlatforms(platform string, candidate business.OnboardingCandidate, locals []business.LocalOnboardingGroup) error {
	for _, local := range locals {
		if local.Platform == nil || strings.TrimSpace(*local.Platform) == "" {
			return fmt.Errorf("本地分组「%s」缺少平台，无法安全绑定账号", local.Name)
		}
		localPlatform := normalizePlatform(*local.Platform)
		if localPlatform != platform {
			return fmt.Errorf(
				"平台不匹配：上游分组「%s」为 %s，本地分组「%s」为 %s",
				candidate.GroupName,
				platform,
				local.Name,
				localPlatform,
			)
		}
	}
	return nil
}

func normalizePlatform(value string) string {
	return business.NormalizePlatform(value)
}

func accountCreationMarker(operationID string) string {
	return "[sub2api-console:onboarding:" + operationID + "]"
}

func creationRemark(candidate business.OnboardingCandidate, supplied *string, marker, createdAt string) string {
	description := "未提供"
	if candidate.Description != nil && strings.TrimSpace(*candidate.Description) != "" {
		description = strings.ReplaceAll(strings.Join(strings.Fields(*candidate.Description), " "), "|", "/")
	}
	timestamp := strings.TrimSpace(createdAt)
	if parsed, err := time.Parse(time.RFC3339Nano, timestamp); err == nil {
		timestamp = parsed.UTC().Truncate(time.Second).Format(time.RFC3339)
	}
	value := fmt.Sprintf("【添加账号】：%s，分组：%s | 介绍：%s\n%s", timestamp, candidate.GroupName, description, marker)
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
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed > 0
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
