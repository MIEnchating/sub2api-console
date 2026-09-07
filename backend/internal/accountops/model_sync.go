package accountops

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

const (
	modelDiscoveryConcurrency = 5
	modelApplyConcurrency     = 4
	maximumModelSyncAccounts  = 1000
	maximumUnifiedProbeModels = 20
)

var ErrModelCatalogChanged = errors.New("账号模型目录已变化，请重新获取预览后再应用")

type modelSyncRepository interface {
	AccountModelCatalogs(context.Context, []string) ([]business.AccountModelCatalog, error)
	AccountModelSyncPreview(context.Context, []string) (business.AccountModelSyncPreview, error)
	AccountModelExclusionList(context.Context) ([]string, error)
	SaveAccountModelExclusionList(context.Context, []string, string) error
	SaveAccountEnabledModels(context.Context, string, []string) error
}

type modelSyncProbeRunner interface {
	RunNow(context.Context, probe.Request) (probe.RunSummary, error)
}

type ModelApplyRequest struct {
	Accounts           []AccountModelSelection
	ProbeModels        []string
	CatalogFingerprint string
	Actor              string
}

type AccountModelSelection struct {
	AccountID string   `json:"account_id"`
	Models    []string `json:"models"`
}

type modelSyncItem struct {
	AccountID   string `json:"account_id"`
	AccountName string `json:"account_name"`
	Status      string `json:"status"`
	ModelCount  int    `json:"model_count"`
	Error       string `json:"error,omitempty"`
	RemoteWrite bool   `json:"remote_write"`
}

func (s *Service) UseModelSyncProbe(runner modelSyncProbeRunner) { s.modelSyncProbe = runner }

func (s *Service) ModelSyncSettings(ctx context.Context) (business.AccountModelSyncSettings, error) {
	repository, ok := s.repository.(modelSyncRepository)
	if !ok {
		return business.AccountModelSyncSettings{}, errors.New("账号模型同步存储尚未就绪")
	}
	patterns, err := repository.AccountModelExclusionList(ctx)
	if err != nil {
		return business.AccountModelSyncSettings{}, err
	}
	return business.AccountModelSyncSettings{BlockedPatterns: patterns}, nil
}

func (s *Service) ConfigureModelSyncSettings(ctx context.Context, patterns []string, actor string) (business.AccountModelSyncSettings, error) {
	repository, ok := s.repository.(modelSyncRepository)
	if !ok {
		return business.AccountModelSyncSettings{}, errors.New("账号模型同步存储尚未就绪")
	}
	if err := repository.SaveAccountModelExclusionList(ctx, patterns, strings.TrimSpace(actor)); err != nil {
		return business.AccountModelSyncSettings{}, err
	}
	return s.ModelSyncSettings(ctx)
}

func (s *Service) ModelSyncPreview(ctx context.Context, accountIDs []string) (business.AccountModelSyncPreview, error) {
	repository, ok := s.repository.(modelSyncRepository)
	if !ok {
		return business.AccountModelSyncPreview{}, errors.New("账号模型同步存储尚未就绪")
	}
	ids, err := s.prepareModelSyncAccounts(ctx, accountIDs)
	if err != nil {
		return business.AccountModelSyncPreview{}, err
	}
	return repository.AccountModelSyncPreview(ctx, ids)
}

func (s *Service) EnqueueModelDiscovery(ctx context.Context, accountIDs []string, actor string) (taskstore.Task, error) {
	ids, err := s.prepareModelSyncAccounts(ctx, accountIDs)
	if err != nil {
		return taskstore.Task{}, err
	}
	expectedTarget, err := s.targets.TargetSettings(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	return s.enqueueModelSyncTask(ctx, "account-model-discovery", "账号模型发现已排队", ids, func(run context.Context, task taskstore.Task) {
		s.executeModelDiscovery(targetguard.Expect(run, expectedTarget), task, ids, actor)
	})
}

func (s *Service) EnqueueModelApply(ctx context.Context, request ModelApplyRequest) (taskstore.Task, error) {
	request.Actor = strings.TrimSpace(request.Actor)
	requestedIDs := make([]string, len(request.Accounts))
	for index, selection := range request.Accounts {
		requestedIDs[index] = selection.AccountID
	}
	ids, err := s.prepareModelSyncAccounts(ctx, requestedIDs)
	if err != nil {
		return taskstore.Task{}, err
	}
	mode, err := s.repository.Mode(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	if mode != runtimepolicy.Full {
		return taskstore.Task{}, errors.New("应用账号模型需要完全模式")
	}
	if s.modelSyncProbe == nil {
		return taskstore.Task{}, errors.New("账号模型探测服务尚未就绪")
	}
	repository, ok := s.repository.(modelSyncRepository)
	if !ok {
		return taskstore.Task{}, errors.New("账号模型同步存储尚未就绪")
	}
	preview, err := repository.AccountModelSyncPreview(ctx, ids)
	if err != nil {
		return taskstore.Task{}, err
	}
	if strings.TrimSpace(request.CatalogFingerprint) == "" || request.CatalogFingerprint != preview.Fingerprint {
		return taskstore.Task{}, ErrModelCatalogChanged
	}
	request.Accounts, err = validateAccountModelSelections(preview.Accounts, preview.BlockedPatterns, request.Accounts)
	if err != nil {
		return taskstore.Task{}, err
	}
	request.ProbeModels, err = validateUnifiedProbeModels(request.Accounts, request.ProbeModels)
	if err != nil {
		return taskstore.Task{}, err
	}
	expectedTarget, err := s.targets.TargetSettings(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	return s.enqueueModelSyncTask(ctx, "account-model-apply", "账号模型应用与探测已排队", ids, func(run context.Context, task taskstore.Task) {
		s.executeModelApply(targetguard.Expect(run, expectedTarget), task, request)
	})
}

func validateAccountModelSelections(
	accounts []business.AccountModelSyncAccount,
	blockedPatterns []string,
	requested []AccountModelSelection,
) ([]AccountModelSelection, error) {
	if len(accounts) != len(requested) {
		return nil, errors.New("账号模型选择必须覆盖本次同步的全部账号")
	}
	catalogs := make(map[string]business.AccountModelSyncAccount, len(accounts))
	for _, account := range accounts {
		catalogs[account.AccountID] = account
	}
	result := make([]AccountModelSelection, 0, len(requested))
	seenAccounts := map[string]struct{}{}
	for _, selection := range requested {
		account, exists := catalogs[selection.AccountID]
		if !exists {
			return nil, fmt.Errorf("账号 %s 不在本次模型目录中", selection.AccountID)
		}
		if _, duplicate := seenAccounts[selection.AccountID]; duplicate {
			return nil, fmt.Errorf("账号 ID %s 重复", selection.AccountID)
		}
		seenAccounts[selection.AccountID] = struct{}{}
		if len(selection.Models) > 500 {
			return nil, fmt.Errorf("账号 %s 选择的模型不能超过 500 个", selection.AccountID)
		}
		available := make(map[string]string, len(account.Models))
		for _, model := range account.Models {
			available[strings.ToLower(model)] = model
		}
		models := make([]string, 0, len(selection.Models))
		seenModels := map[string]struct{}{}
		for _, rawModel := range selection.Models {
			model := strings.TrimSpace(rawModel)
			if model == "" || utf8.RuneCountInString(model) > 256 {
				return nil, fmt.Errorf("账号 %s 的模型名称无效", selection.AccountID)
			}
			key := strings.ToLower(model)
			if _, duplicate := seenModels[key]; duplicate {
				continue
			}
			canonical, exists := available[key]
			if !exists {
				return nil, fmt.Errorf("账号 %s 未返回模型 %s", selection.AccountID, model)
			}
			if business.ModelMatchesBlockPatterns(canonical, blockedPatterns) {
				return nil, fmt.Errorf("模型 %s 已被全局屏蔽", canonical)
			}
			seenModels[key] = struct{}{}
			models = append(models, canonical)
		}
		sort.Slice(models, func(left, right int) bool {
			return strings.ToLower(models[left]) < strings.ToLower(models[right])
		})
		result = append(result, AccountModelSelection{AccountID: selection.AccountID, Models: models})
	}
	return result, nil
}

func validateUnifiedProbeModels(accounts []AccountModelSelection, requested []string) ([]string, error) {
	if len(requested) == 0 || len(requested) > maximumUnifiedProbeModels {
		return nil, fmt.Errorf("请选择 1 到 %d 个统一探活模型", maximumUnifiedProbeModels)
	}
	availableModels := map[string]string{}
	for _, account := range accounts {
		for _, candidate := range account.Models {
			key := strings.ToLower(candidate)
			if _, exists := availableModels[key]; !exists {
				availableModels[key] = candidate
			}
		}
	}
	result := make([]string, 0, len(requested))
	seen := map[string]struct{}{}
	for _, rawModel := range requested {
		model := strings.TrimSpace(rawModel)
		if model == "" || utf8.RuneCountInString(model) > 256 {
			return nil, errors.New("统一探活模型必须是长度不超过 256 的非空字符串")
		}
		key := strings.ToLower(model)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		canonical, exists := availableModels[key]
		if !exists {
			return nil, fmt.Errorf("统一探活模型 %s 不在本次要同步的模型中", model)
		}
		seen[key] = struct{}{}
		result = append(result, canonical)
	}
	return result, nil
}

func (s *Service) prepareModelSyncAccounts(ctx context.Context, accountIDs []string) ([]string, error) {
	if len(accountIDs) == 0 || len(accountIDs) > maximumModelSyncAccounts {
		return nil, fmt.Errorf("请选择 1 到 %d 个账号", maximumModelSyncAccounts)
	}
	ids := make([]string, 0, len(accountIDs))
	seen := map[string]struct{}{}
	for _, raw := range accountIDs {
		accountID := strings.TrimSpace(raw)
		if !stableID(accountID) {
			return nil, errors.New("账号必须使用有效的稳定 ID")
		}
		if _, duplicate := seen[accountID]; duplicate {
			return nil, fmt.Errorf("账号 ID %s 重复", accountID)
		}
		seen[accountID] = struct{}{}
		_, account, err := s.localAccount(ctx, accountID)
		if err != nil {
			return nil, err
		}
		if account.ManualPriority != nil {
			return nil, fmt.Errorf("账号 %s 处于人工优先位，模型同步已禁用", accountID)
		}
		ids = append(ids, accountID)
	}
	return ids, nil
}

func (s *Service) enqueueModelSyncTask(
	ctx context.Context,
	operation string,
	message string,
	accountIDs []string,
	run func(context.Context, taskstore.Task),
) (taskstore.Task, error) {
	id, err := operationID(operation)
	if err != nil {
		return taskstore.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{
		ID: id, Skill: "sub2api-account-model-sync", Operation: operation,
		Status: "queued", Progress: 0, Message: message,
		Result:    map[string]any{"account_ids": accountIDs, "completed": 0, "total": len(accountIDs)},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	if err := taskrunner.GoTask(s.taskRunner, task.ID, func(parent context.Context) { run(parent, task) }); err != nil {
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
	return task, nil
}

func (s *Service) executeModelDiscovery(ctx context.Context, task taskstore.Task, accountIDs []string, _ string) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	task.Status, task.Progress, task.Message = "running", 2, "正在逐账号获取上游模型目录"
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if !taskstore.SaveRunning(ctx, s.tasks, task) {
		return
	}
	type outcome struct {
		index int
		item  modelSyncItem
	}
	items := make([]modelSyncItem, len(accountIDs))
	jobs := make(chan int)
	outcomes := make(chan outcome, len(accountIDs))
	var workers sync.WaitGroup
	for range min(modelDiscoveryConcurrency, len(accountIDs)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				if ctx.Err() != nil {
					return
				}
				accountID := accountIDs[index]
				item := s.discoverAccountModels(ctx, accountID)
				outcomes <- outcome{index: index, item: item}
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
		task.Message = fmt.Sprintf("模型发现进度：已处理 %d/%d 个账号", completed, len(accountIDs))
		task.Result = modelSyncTaskResult(accountIDs, completed, items)
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		taskstore.PersistProgress(s.tasks, task)
	}
	if completed != len(accountIDs) {
		s.failModelSyncTask(ctx, &task, accountIDs, errors.New("账号模型发现被中断"))
		return
	}
	s.finishModelSyncTask(ctx, &task, accountIDs, items, "账号模型发现")
}

func sendModelSyncJobs(ctx context.Context, total int, jobs chan<- int) {
	defer close(jobs)
	for index := 0; index < total; index++ {
		select {
		case jobs <- index:
		case <-ctx.Done():
			return
		}
	}
}

func (s *Service) discoverAccountModels(ctx context.Context, accountID string) modelSyncItem {
	item := modelSyncItem{AccountID: accountID, Status: "failed"}
	_, local, err := s.localAccount(ctx, accountID)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	item.AccountName = local.Name
	guarded, release, err := s.acquireAccountMutation(ctx, accountID)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	defer release()
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
	models, err := client.SyncAccountModels(guarded, accountID)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	if len(models) == 0 {
		item.Error = "上游未返回可用模型"
		return item
	}
	if err := s.repository.SaveAccountModels(guarded, accountID, models); err != nil {
		item.Error = err.Error()
		return item
	}
	item.Status, item.ModelCount = "succeeded", len(models)
	return item
}

func (s *Service) executeModelApply(ctx context.Context, task taskstore.Task, request ModelApplyRequest) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	task.Status, task.Progress, task.Message = "running", 2, "正在确认账号模型目录"
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if !taskstore.SaveRunning(ctx, s.tasks, task) {
		return
	}
	repository := s.repository.(modelSyncRepository)
	accountIDs := modelApplyAccountIDs(request.Accounts)
	preview, err := repository.AccountModelSyncPreview(ctx, accountIDs)
	if err != nil || preview.Fingerprint != request.CatalogFingerprint {
		if err == nil {
			err = ErrModelCatalogChanged
		}
		s.failModelSyncTask(ctx, &task, accountIDs, err)
		return
	}
	selections, err := validateAccountModelSelections(preview.Accounts, preview.BlockedPatterns, request.Accounts)
	if err != nil {
		s.failModelSyncTask(ctx, &task, accountIDs, err)
		return
	}
	selectedModels := make(map[string][]string, len(selections))
	for _, selection := range selections {
		selectedModels[selection.AccountID] = selection.Models
	}
	catalogs, err := repository.AccountModelCatalogs(ctx, accountIDs)
	if err != nil {
		s.failModelSyncTask(ctx, &task, accountIDs, err)
		return
	}
	type outcome struct {
		index int
		item  modelSyncItem
	}
	items := make([]modelSyncItem, len(catalogs))
	jobs := make(chan int)
	outcomes := make(chan outcome, len(catalogs))
	var workers sync.WaitGroup
	for range min(modelApplyConcurrency, len(catalogs)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				if ctx.Err() != nil {
					return
				}
				catalog := catalogs[index]
				item := s.applyAccountModels(ctx, catalog, selectedModels[catalog.AccountID], request.Actor)
				outcomes <- outcome{index: index, item: item}
			}
		}()
	}
	go sendModelSyncJobs(ctx, len(catalogs), jobs)
	go func() { workers.Wait(); close(outcomes) }()
	completed := 0
	for outcome := range outcomes {
		items[outcome.index] = outcome.item
		completed++
		task.Progress = 2 + completed*70/len(catalogs)
		task.Message = fmt.Sprintf("模型应用进度：已处理 %d/%d 个账号", completed, len(catalogs))
		task.Result = modelSyncTaskResult(accountIDs, completed, items)
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		taskstore.PersistProgress(s.tasks, task)
	}
	if completed != len(catalogs) {
		s.failModelSyncTask(ctx, &task, accountIDs, errors.New("账号模型应用被中断"))
		return
	}
	probeSummary := probe.RunSummary{}
	probeError := ""
	appliedIDs := appliedModelSyncAccountIDs(items)
	if len(appliedIDs) > 0 && s.modelSyncProbe != nil {
		probeSummary, err = s.runUnifiedProbeModels(ctx, appliedIDs, request.ProbeModels, func(index, total int) {
			task.Progress = 78 + index*18/total
			task.Message = fmt.Sprintf("正在使用统一探活模型验证连接（%d/%d）", index+1, total)
			task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			taskstore.PersistProgress(s.tasks, task)
		})
		if err != nil {
			probeError = err.Error()
		}
	}
	s.finishModelApplyTask(ctx, &task, accountIDs, items, probeSummary, probeError)
}

func modelApplyAccountIDs(selections []AccountModelSelection) []string {
	result := make([]string, len(selections))
	for index, selection := range selections {
		result[index] = selection.AccountID
	}
	return result
}

func (s *Service) runUnifiedProbeModels(
	ctx context.Context,
	accountIDs, models []string,
	onStart func(index, total int),
) (probe.RunSummary, error) {
	result := probe.RunSummary{}
	probeErrors := make([]error, 0)
	for index, model := range models {
		if onStart != nil {
			onStart(index, len(models))
		}
		summary, err := s.modelSyncProbe.RunNow(ctx, probe.Request{
			AccountIDs: accountIDs, Automatic: true, OnePerAccount: true, ProbeModel: model,
		})
		mergeProbeSummary(&result, summary)
		if err != nil {
			probeErrors = append(probeErrors, fmt.Errorf("%s: %w", model, err))
		}
		if ctx.Err() != nil {
			break
		}
	}
	return result, errors.Join(probeErrors...)
}

func mergeProbeSummary(target *probe.RunSummary, source probe.RunSummary) {
	target.Targets += source.Targets
	target.Persisted += source.Persisted
	target.Passed += source.Passed
	target.Failed += source.Failed
	target.Skipped += source.Skipped
	target.Results = append(target.Results, source.Results...)
}

func (s *Service) applyAccountModels(ctx context.Context, catalog business.AccountModelCatalog, desired []string, actor string) modelSyncItem {
	item := modelSyncItem{AccountID: catalog.AccountID, AccountName: catalog.AccountName, Status: "failed"}
	if len(desired) == 0 {
		item.Status = "skipped"
		item.Error = "该账号没有选中的模型；为避免空映射放行全部，未执行写入"
		return item
	}
	guarded, release, err := s.acquireAccountMutation(ctx, catalog.AccountID)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	defer release()
	if mode, account, err := s.localAccount(guarded, catalog.AccountID); err != nil {
		item.Error = err.Error()
		return item
	} else if mode != runtimepolicy.Full {
		item.Error = "任务执行前已退出完全模式"
		return item
	} else if account.ManualPriority != nil {
		item.Error = "账号在任务执行前进入人工优先位"
		return item
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
	beforeAccount, err := client.Account(guarded, catalog.AccountID)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	credentials, err := accountCredentials(beforeAccount)
	if err != nil {
		item.Error = err.Error()
		return item
	}
	beforeMapping, err := accountModelMapping(credentials["model_mapping"])
	if err != nil {
		item.Error = err.Error()
		return item
	}
	afterMapping := filteredAccountModelMapping(beforeMapping, desired)
	credentials["model_mapping"] = stringMapToAny(afterMapping)
	operationID, operationErr := operationID("account-model-apply")
	if operationErr != nil {
		item.Error = operationErr.Error()
		return item
	}
	if _, err := client.UpdateAccount(guarded, catalog.AccountID, map[string]any{"credentials": credentials}); err != nil {
		s.recordModelSyncFailure(guarded, operationID, actor, catalog, beforeMapping, afterMapping, "remote-write", err, false)
		item.Error = err.Error()
		return item
	}
	item.RemoteWrite = true
	readback, err := client.Account(guarded, catalog.AccountID)
	if err != nil {
		s.recordModelSyncFailure(guarded, operationID, actor, catalog, beforeMapping, afterMapping, "readback", err, true)
		item.Error = "管理平台写入成功，但账号模型读回失败：" + err.Error()
		return item
	}
	readbackCredentials, err := accountCredentials(readback)
	if err != nil {
		s.recordModelSyncFailure(guarded, operationID, actor, catalog, beforeMapping, afterMapping, "readback", err, true)
		item.Error = "管理平台写入成功，但账号模型读回不可用：" + err.Error()
		return item
	}
	confirmed, err := accountModelMapping(readbackCredentials["model_mapping"])
	if err != nil || !equalStringMap(confirmed, afterMapping) {
		if err == nil {
			err = errors.New("管理平台账号模型写后确认不一致")
		}
		s.recordModelSyncFailure(guarded, operationID, actor, catalog, beforeMapping, afterMapping, "readback", err, true)
		item.Error = err.Error()
		return item
	}
	if err := s.repository.(modelSyncRepository).SaveAccountEnabledModels(guarded, catalog.AccountID, desired); err != nil {
		s.recordModelSyncFailure(guarded, operationID, actor, catalog, beforeMapping, afterMapping, "local-commit", err, true)
		item.Error = "账号模型已写入，但本地启用目录保存失败：" + err.Error()
		return item
	}
	field := "credentials.model_mapping"
	name := catalog.AccountName
	err = s.repository.RecordAccountOperation(guarded, business.AccountOperation{
		OperationID: operationID, OperationType: "account.models.sync", State: "succeeded", Phase: "readback",
		Actor: actor, RemoteConfirmed: true, ReadbackConfirmed: true,
		ObjectID: catalog.AccountID, ObjectName: &name, FieldName: &field,
		Before: beforeMapping, After: afterMapping, Writeback: true,
	})
	if err != nil {
		item.Error = "账号模型已写入，但审计保存失败：" + err.Error()
		return item
	}
	item.Status, item.ModelCount = "succeeded", len(afterMapping)
	return item
}

func (s *Service) recordModelSyncFailure(ctx context.Context, operationID, actor string, catalog business.AccountModelCatalog, before, after map[string]string, phase string, cause error, remote bool) {
	field, name, message := "credentials.model_mapping", catalog.AccountName, cause.Error()
	_ = s.repository.RecordAccountOperation(ctx, business.AccountOperation{
		OperationID: operationID, OperationType: "account.models.sync", State: "failed", Phase: phase,
		Actor: actor, Error: &message, RemoteConfirmed: remote, ReadbackConfirmed: false,
		ObjectID: catalog.AccountID, ObjectName: &name, FieldName: &field,
		Before: before, After: after, Writeback: true,
	})
}

func accountCredentials(account map[string]any) (map[string]any, error) {
	raw, present := account["credentials"]
	if !present || raw == nil {
		return map[string]any{}, nil
	}
	credentials, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("管理平台账号凭据配置不可读")
	}
	copy := make(map[string]any, len(credentials)+1)
	for key, value := range credentials {
		copy[key] = value
	}
	return copy, nil
}

func accountModelMapping(raw any) (map[string]string, error) {
	result := map[string]string{}
	if raw == nil {
		return result, nil
	}
	switch mapping := raw.(type) {
	case map[string]any:
		for source, rawTarget := range mapping {
			target, ok := rawTarget.(string)
			if !ok || strings.TrimSpace(source) == "" || strings.TrimSpace(target) == "" {
				return nil, errors.New("管理平台账号模型映射不可读")
			}
			result[strings.TrimSpace(source)] = strings.TrimSpace(target)
		}
	case map[string]string:
		for source, target := range mapping {
			result[strings.TrimSpace(source)] = strings.TrimSpace(target)
		}
	default:
		return nil, errors.New("管理平台账号模型映射不可读")
	}
	return result, nil
}

func filteredAccountModelMapping(current map[string]string, desired []string) map[string]string {
	desiredSet := make(map[string]struct{}, len(desired))
	result := make(map[string]string, len(desired))
	for _, model := range desired {
		desiredSet[strings.ToLower(model)] = struct{}{}
	}
	for source, target := range current {
		_, sourceAllowed := desiredSet[strings.ToLower(source)]
		_, targetAllowed := desiredSet[strings.ToLower(target)]
		if source != "*" && (sourceAllowed || targetAllowed) {
			result[source] = target
		}
	}
	for _, model := range desired {
		if _, present := result[model]; !present {
			result[model] = model
		}
	}
	return result
}

func stringMapToAny(input map[string]string) map[string]any {
	result := make(map[string]any, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func equalStringMap(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func appliedModelSyncAccountIDs(items []modelSyncItem) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item.Status == "succeeded" {
			result = append(result, item.AccountID)
		}
	}
	return result
}

func modelSyncTaskResult(accountIDs []string, completed int, items []modelSyncItem) map[string]any {
	visible := make([]modelSyncItem, 0, completed)
	remoteWrite := false
	for _, item := range items {
		remoteWrite = remoteWrite || item.RemoteWrite
		if item.AccountID != "" {
			visible = append(visible, item)
		}
	}
	return map[string]any{
		"account_ids": accountIDs, "completed": completed, "total": len(accountIDs), "items": visible, "remote_write": remoteWrite,
	}
}

func (s *Service) finishModelSyncTask(ctx context.Context, task *taskstore.Task, accountIDs []string, items []modelSyncItem, label string) {
	succeeded, failed := 0, 0
	for _, item := range items {
		if item.Status == "succeeded" {
			succeeded++
		} else {
			failed++
		}
	}
	task.Status = "succeeded"
	if failed > 0 {
		task.Status = "partial"
	}
	task.Progress = 100
	task.Message = fmt.Sprintf("%s完成：成功 %d，失败 %d", label, succeeded, failed)
	task.Result = modelSyncTaskResult(accountIDs, len(items), items)
	task.Result["succeeded"] = succeeded
	task.Result["failed"] = failed
	task.Result["remote_write"] = false
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	taskstore.MarkCancelled(ctx, task, label+"已取消")
	taskstore.PersistFinal(s.tasks, *task)
}

func (s *Service) finishModelApplyTask(ctx context.Context, task *taskstore.Task, accountIDs []string, items []modelSyncItem, summary probe.RunSummary, probeError string) {
	applied, skipped, failed := 0, 0, 0
	for _, item := range items {
		switch item.Status {
		case "succeeded":
			applied++
		case "skipped":
			skipped++
		default:
			failed++
		}
	}
	task.Status = "succeeded"
	if skipped > 0 || failed > 0 || probeError != "" || summary.Failed > 0 || summary.Skipped > 0 {
		task.Status = "partial"
	}
	task.Progress = 100
	task.Message = fmt.Sprintf("账号模型应用完成：写入 %d，跳过 %d，失败 %d；验证通过 %d，跳过 %d，失败 %d", applied, skipped, failed, summary.Passed, summary.Skipped, summary.Failed)
	task.Result = modelSyncTaskResult(accountIDs, len(items), items)
	task.Result["applied"], task.Result["skipped"], task.Result["failed"] = applied, skipped, failed
	task.Result["probe"] = summary
	if probeError != "" {
		task.Result["probe_error"] = probeError
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	taskstore.MarkCancelled(ctx, task, "账号模型应用已取消")
	taskstore.PersistFinal(s.tasks, *task)
}

func (s *Service) failModelSyncTask(ctx context.Context, task *taskstore.Task, accountIDs []string, err error) {
	task.Status, task.Progress = "failed", 100
	task.Message = "账号模型同步失败：" + err.Error()
	if task.Result == nil {
		task.Result = map[string]any{}
	}
	task.Result["account_ids"], task.Result["error"] = accountIDs, err.Error()
	if _, present := task.Result["remote_write"]; !present {
		task.Result["remote_write"] = false
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	taskstore.MarkCancelled(ctx, task, "账号模型同步已取消")
	taskstore.PersistFinal(s.tasks, *task)
}
