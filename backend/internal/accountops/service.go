package accountops

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type TargetStore interface {
	TargetSettings(context.Context) (configstore.TargetSettings, error)
}

type Repository interface {
	Mode(context.Context) (string, error)
	Account(context.Context, string) (*business.AccountDetail, error)
	SetAccountScopeControl(context.Context, string, string, string) (business.PolicySnapshot, error)
	CommitAccountControlReadback(context.Context, string, string, string, bool, business.AccountOperation) error
	CommitAccountFieldsReadback(context.Context, string, *string, *int64, *string, *int64, *string, bool, *string, business.AccountOperation) error
	RecordAccountOperation(context.Context, business.AccountOperation) error
	SaveAccountModels(context.Context, string, []string) error
}

type manualPriorityRepository interface {
	ManualPriorityConfig(context.Context) (business.ManualPriorityConfig, error)
	AssignManualPriority(context.Context, string, int64, string, int64, string) (business.ManualPriorityAssignment, error)
	RevertManualPriorityReservation(context.Context, string, string) error
	ManualPriorityRelease(context.Context, string) (business.ManualPriorityRelease, error)
	CommitManualPriorityRelease(context.Context, business.ManualPriorityRelease, string, business.AccountOperation) error
}

type TaskStore interface {
	Save(context.Context, taskstore.Task) error
}

type FieldPatch struct {
	NamePresent        bool
	Name               *string
	PriorityPresent    bool
	Priority           *int64
	LoadFactorPresent  bool
	LoadFactor         *string
	ConcurrencyPresent bool
	Concurrency        *int64
	MultiplierPresent  bool
	Multiplier         *string
	NotesPresent       bool
	Notes              *string
}

type OperationError struct {
	Message       string
	RemoteWritten bool
}

func (e *OperationError) Error() string { return e.Message }

func (e *OperationError) RemoteWriteSucceeded() bool { return e.RemoteWritten }

type Service struct {
	targets    TargetStore
	repository Repository
	tasks      TaskStore
	timeout    time.Duration
}

func New(targets TargetStore, repository Repository, tasks TaskStore) *Service {
	return &Service{targets: targets, repository: repository, tasks: tasks, timeout: 10 * time.Minute}
}

func (s *Service) SyncFields(ctx context.Context, accountID string, patch FieldPatch, actor string) (map[string]any, error) {
	return s.syncFields(ctx, accountID, patch, actor, false)
}

// SyncAccountMultiplier writes a multiplier obtained from the account's
// upstream billing probe through the same mandatory management readback path
// as other account field edits.
func (s *Service) SyncAccountMultiplier(ctx context.Context, accountID, multiplier, actor string) (map[string]any, error) {
	return s.syncFields(ctx, accountID, FieldPatch{
		MultiplierPresent: true,
		Multiplier:        &multiplier,
	}, actor, false)
}

// SyncAccountRate updates the rate-derived account name and multiplier in one
// management mutation and requires both fields to match on readback.
func (s *Service) SyncAccountRate(ctx context.Context, accountID, name, multiplier, actor string) (map[string]any, error) {
	return s.syncFields(ctx, accountID, FieldPatch{
		NamePresent:       true,
		Name:              &name,
		MultiplierPresent: true,
		Multiplier:        &multiplier,
	}, actor, false)
}

func (s *Service) Control(ctx context.Context, accountID, action, actor string) (map[string]any, error) {
	if !stableID(accountID) {
		return nil, errors.New("账号必须使用有效的稳定 ID")
	}
	schedulable, remoteAction := controlSchedulable(action)
	if !remoteAction {
		return nil, errors.New("账号远端控制 action 无效")
	}
	mode, local, err := s.localAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if mode != runtimepolicy.Full {
		return nil, errors.New("暂停、恢复和熔断操作需要完全模式")
	}
	client, err := s.adminClient(ctx)
	if err != nil {
		return nil, err
	}
	before, err := client.Account(ctx, accountID)
	if err != nil {
		return nil, err
	}
	beforeSchedulable, err := accountSchedulable(before)
	if err != nil {
		return nil, err
	}
	operationID, err := operationID("account-control")
	if err != nil {
		return nil, err
	}
	accountName := remoteName(before, local.Name)
	beforeValues := map[string]any{"schedulable": beforeSchedulable}
	afterValues := map[string]any{"schedulable": schedulable}
	if err := writeAccountSchedulable(ctx, client, accountID, schedulable); err != nil {
		operation := controlOperation(operationID, actor, accountID, accountName, action, beforeValues, afterValues, false, false)
		s.recordFailure(ctx, operation, "remote-write", err, false)
		return nil, &OperationError{Message: err.Error()}
	}
	after, err := client.Account(ctx, accountID)
	if err != nil {
		operation := controlOperation(operationID, actor, accountID, accountName, action, beforeValues, afterValues, true, false)
		s.recordFailure(ctx, operation, "readback", err, true)
		return nil, &OperationError{Message: "管理平台调度状态写入成功，但账号读回失败：" + err.Error(), RemoteWritten: true}
	}
	confirmed, err := accountSchedulable(after)
	if err != nil || confirmed != schedulable {
		if err == nil {
			err = errors.New("账号可调度状态读回不一致")
		}
		operation := controlOperation(operationID, actor, accountID, remoteName(after, accountName), action, beforeValues, afterValues, true, false)
		s.recordFailure(ctx, operation, "readback", err, true)
		return nil, &OperationError{Message: "管理平台写入成功，但" + err.Error(), RemoteWritten: true}
	}
	operation := controlOperation(operationID, actor, accountID, remoteName(after, accountName), action, beforeValues, afterValues, true, true)
	if err := s.repository.CommitAccountControlReadback(ctx, accountID, action, actor, confirmed, operation); err != nil {
		return nil, &OperationError{Message: "管理平台写入并读回成功，但本地状态提交失败：" + err.Error(), RemoteWritten: true}
	}
	warnings := []string{}
	if schedulable && (action == "resume" || action == "recover") {
		warnings = recoverAccountRuntime(ctx, client, accountID)
	}
	return map[string]any{
		"operation_id": operationID, "account_id": accountID, "action": action,
		"before": beforeValues, "after": afterValues, "remote_write": true,
		"readback_confirmed": true, "cleanup_warnings": warnings,
	}, nil
}

func (s *Service) EnqueueControl(ctx context.Context, accountID, action, actor string) (taskstore.Task, error) {
	if !stableID(accountID) {
		return taskstore.Task{}, errors.New("账号必须使用有效的稳定 ID")
	}
	if _, valid := accountControlActions[action]; !valid {
		return taskstore.Task{}, errors.New("账号控制 action 无效")
	}
	mode, _, err := s.localAccount(ctx, accountID)
	if err != nil {
		return taskstore.Task{}, err
	}
	_, remoteAction := controlSchedulable(action)
	if remoteAction && mode != runtimepolicy.Full {
		return taskstore.Task{}, errors.New("暂停、恢复和熔断操作需要完全模式")
	}
	return s.enqueue(ctx, "sub2api-account-control", "account-control", "账号控制操作已排队", func(run context.Context) (map[string]any, error) {
		if remoteAction {
			return s.Control(run, accountID, action, actor)
		}
		if _, err := s.repository.SetAccountScopeControl(run, accountID, action, actor); err != nil {
			return nil, err
		}
		return map[string]any{
			"account_id": accountID, "action": action, "saved": true,
			"remote_write": false, "readback_confirmed": false,
		}, nil
	})
}

var accountControlActions = map[string]struct{}{
	"pause": {}, "resume": {}, "exclude": {}, "include": {}, "fuse": {}, "recover": {},
}

func controlSchedulable(action string) (bool, bool) {
	switch action {
	case "pause", "fuse":
		return false, true
	case "resume", "recover":
		return true, true
	default:
		return false, false
	}
}

func (s *Service) syncFields(ctx context.Context, accountID string, patch FieldPatch, actor string, allowReservedPriority bool) (map[string]any, error) {
	if !stableID(accountID) {
		return nil, errors.New("账号必须使用有效的稳定 ID")
	}
	if !patch.NamePresent && !patch.PriorityPresent && !patch.LoadFactorPresent && !patch.ConcurrencyPresent && !patch.MultiplierPresent && !patch.NotesPresent {
		return nil, errors.New("至少提供一个需要同步的账号字段")
	}
	mode, local, err := s.localAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if mode == runtimepolicy.Scheduling {
		return nil, errors.New("调度模式只允许计算，不允许同步账号字段；请切换到完全模式")
	}
	if mode == runtimepolicy.Monitoring && (!patch.MultiplierPresent || patch.NamePresent || patch.PriorityPresent || patch.LoadFactorPresent || patch.ConcurrencyPresent || patch.NotesPresent) {
		return nil, errors.New("监控模式只允许同步账号倍率")
	}
	if !runtimepolicy.Valid(mode) {
		return nil, fmt.Errorf("运行模式无效：%s", mode)
	}
	if patch.PriorityPresent && patch.Priority != nil {
		manualRepository, ok := s.repository.(manualPriorityRepository)
		if ok {
			config, configErr := manualRepository.ManualPriorityConfig(ctx)
			if configErr != nil {
				return nil, configErr
			}
			insideReservedRange := *patch.Priority <= config.ReservedMax
			if insideReservedRange && !allowReservedPriority {
				return nil, fmt.Errorf("优先级 1 到 %d 为人工优先位，请使用账号操作中的人工优先位设置", config.ReservedMax)
			}
		}
	}
	body := map[string]any{}
	var normalizedName, normalizedLoadFactor, normalizedMultiplier *string
	var normalizedPriority, normalizedConcurrency *int64
	if patch.NamePresent {
		if patch.Name == nil || strings.TrimSpace(*patch.Name) == "" {
			return nil, errors.New("账号名称不能为空")
		}
		value := strings.TrimSpace(*patch.Name)
		normalizedName = &value
		body["name"] = value
	}
	if patch.PriorityPresent {
		if patch.Priority == nil || *patch.Priority < 1 {
			return nil, errors.New("优先级必须是正整数")
		}
		value := *patch.Priority
		normalizedPriority = &value
		body["priority"] = value
	}
	if patch.LoadFactorPresent {
		if patch.LoadFactor == nil {
			return nil, errors.New("负载因子不能为 null；省略字段表示不修改")
		}
		value, err := decimalAtLeastOne(*patch.LoadFactor)
		if err != nil {
			return nil, errors.New("负载因子必须大于或等于 1")
		}
		normalizedLoadFactor = &value
		body["load_factor"] = json.Number(value)
	}
	if patch.ConcurrencyPresent {
		if patch.Concurrency == nil || *patch.Concurrency < 1 || *patch.Concurrency > 10_000_000 {
			return nil, errors.New("并发上限必须是 1 到 10000000 之间的整数")
		}
		value := *patch.Concurrency
		normalizedConcurrency = &value
		body["concurrency"] = value
	}
	if patch.MultiplierPresent {
		if patch.Multiplier == nil {
			return nil, errors.New("账号倍率不能为 null；省略字段表示不修改")
		}
		value, err := positiveDecimal(*patch.Multiplier)
		if err != nil {
			return nil, errors.New("倍率必须大于 0")
		}
		normalizedMultiplier = &value
		body["rate_multiplier"] = json.Number(value)
	}
	if patch.NotesPresent {
		if patch.Notes == nil {
			body["notes"] = nil
		} else {
			body["notes"] = *patch.Notes
		}
	}
	client, err := s.adminClient(ctx)
	if err != nil {
		return nil, err
	}
	before, err := client.Account(ctx, accountID)
	if err != nil {
		return nil, err
	}
	operationID, err := operationID("account-sync")
	if err != nil {
		return nil, err
	}
	beforeValues, requested := fieldAuditValues(before, normalizedName, normalizedPriority, normalizedLoadFactor, normalizedConcurrency, normalizedMultiplier, patch)
	fieldName := strings.Join(sortedKeys(beforeValues), ",")
	accountName := remoteName(before, local.Name)
	remoteWritten := false
	_, err = client.Mutate(ctx, "PUT", "/admin/accounts/"+accountID, body)
	if err != nil {
		operation := fieldOperation(operationID, actor, accountID, accountName, fieldName, beforeValues, requested, false, false)
		s.recordFailure(ctx, operation, "remote-write", err, false)
		return nil, &OperationError{Message: err.Error()}
	}
	remoteWritten = true
	after, err := client.Account(ctx, accountID)
	if err != nil {
		operation := fieldOperation(operationID, actor, accountID, accountName, fieldName, beforeValues, requested, true, false)
		s.recordFailure(ctx, operation, "readback", err, true)
		return nil, &OperationError{Message: "管理平台写入成功，但账号字段读回失败：" + err.Error(), RemoteWritten: true}
	}
	if err := verifyFieldReadback(after, normalizedName, normalizedPriority, normalizedLoadFactor, normalizedConcurrency, normalizedMultiplier, patch); err != nil {
		operation := fieldOperation(operationID, actor, accountID, remoteName(after, accountName), fieldName, beforeValues, requested, true, false)
		s.recordFailure(ctx, operation, "readback", err, true)
		return nil, &OperationError{Message: "管理平台写入成功，但" + err.Error(), RemoteWritten: true}
	}
	readbackConfirmed := true
	accountName = remoteName(after, accountName)
	effective := fieldEffectiveValues(after, normalizedName, normalizedPriority, normalizedLoadFactor, normalizedConcurrency, normalizedMultiplier, patch)
	operation := fieldOperation(operationID, actor, accountID, remoteName(after, accountName), fieldName, beforeValues, effective, remoteWritten, readbackConfirmed)
	if err := s.repository.CommitAccountFieldsReadback(ctx, accountID, normalizedName, normalizedPriority, normalizedLoadFactor, normalizedConcurrency, normalizedMultiplier, patch.NotesPresent, patch.Notes, operation); err != nil {
		return nil, &OperationError{Message: err.Error(), RemoteWritten: true}
	}
	return map[string]any{
		"operation_id": operationID, "account_id": accountID,
		"before": beforeValues, "after": effective, "remote_write": true,
		"readback_confirmed": readbackConfirmed,
	}, nil
}

func (s *Service) Models(ctx context.Context, accountID string) ([]string, error) {
	if !stableID(accountID) {
		return nil, errors.New("账号必须使用有效的稳定 ID")
	}
	if _, _, err := s.localAccount(ctx, accountID); err != nil {
		return nil, err
	}
	client, err := s.adminClient(ctx)
	if err != nil {
		return nil, err
	}
	models, err := client.SyncAccountModels(ctx, accountID)
	if err != nil {
		models, err = client.AccountModels(ctx, accountID)
	}
	if err != nil {
		return nil, err
	}
	if err := s.repository.SaveAccountModels(ctx, accountID, models); err != nil {
		return nil, err
	}
	return models, nil
}

func (s *Service) EnqueueFields(ctx context.Context, accountID string, patch FieldPatch, actor string) (taskstore.Task, error) {
	return s.enqueue(ctx, "sub2api-account-sync", "account-fields-sync", "账号字段同步已排队", func(run context.Context) (map[string]any, error) {
		return s.SyncFields(run, accountID, patch, actor)
	})
}

func (s *Service) EnqueueManualPriority(ctx context.Context, accountID string, priority int64, loadFactor string, concurrency int64, actor string) (taskstore.Task, error) {
	if !stableID(accountID) {
		return taskstore.Task{}, errors.New("账号必须使用有效的稳定 ID")
	}
	manualRepository, ok := s.repository.(manualPriorityRepository)
	if !ok {
		return taskstore.Task{}, errors.New("人工优先位服务尚未就绪")
	}
	config, err := manualRepository.ManualPriorityConfig(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	if priority < 1 || priority > config.ReservedMax {
		return taskstore.Task{}, fmt.Errorf("人工优先位必须在 1 到 %d 之间", config.ReservedMax)
	}
	loadFactor, err = decimalAtLeastOne(loadFactor)
	if err != nil {
		return taskstore.Task{}, errors.New("负载因子必须大于或等于 1")
	}
	if concurrency < 1 || concurrency > 10_000_000 {
		return taskstore.Task{}, errors.New("并发上限必须是 1 到 10000000 之间的整数")
	}
	mode, err := s.repository.Mode(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	if mode != runtimepolicy.Full {
		return taskstore.Task{}, errors.New("设置人工优先位需要完全模式")
	}
	return s.enqueue(ctx, "sub2api-account-manual-priority", "account-manual-priority", "人工优先位设置已排队", func(run context.Context) (map[string]any, error) {
		before, err := s.repository.Account(run, accountID)
		if err != nil {
			return nil, err
		}
		previousManualPriority := cloneInt64(before.ManualPriority)
		rollbackLoadFactor := config.DefaultLoadFactor
		if before.LoadFactor != nil {
			if normalized, normalizeErr := decimalAtLeastOne(*before.LoadFactor); normalizeErr == nil {
				rollbackLoadFactor = normalized
			}
		}
		rollbackConcurrency := config.DefaultConcurrency
		if before.Concurrency != nil && *before.Concurrency >= 1 && *before.Concurrency <= 10_000_000 {
			rollbackConcurrency = *before.Concurrency
		}
		assignment, err := manualRepository.AssignManualPriority(run, accountID, priority, loadFactor, concurrency, actor)
		if err != nil {
			return nil, err
		}
		loadFactor := assignment.LoadFactor
		concurrency := assignment.Concurrency
		patch := FieldPatch{
			PriorityPresent: true, Priority: &assignment.Priority,
			LoadFactorPresent: true, LoadFactor: &loadFactor,
			ConcurrencyPresent: true, Concurrency: &concurrency,
		}
		result, err := s.syncFields(run, accountID, patch, actor, true)
		if err != nil {
			var operationError *OperationError
			if !errors.As(err, &operationError) || !operationError.RemoteWritten {
				rollbackErr := rollbackManualPriority(run, manualRepository, accountID, previousManualPriority, rollbackLoadFactor, rollbackConcurrency, actor)
				if rollbackErr != nil {
					return nil, fmt.Errorf("%w；人工优先位回滚失败：%v", err, rollbackErr)
				}
			}
			return nil, err
		}
		result["manual_priority"] = assignment.Priority
		return result, nil
	})
}

func (s *Service) EnqueueClearManualPriority(ctx context.Context, accountID, actor string) (taskstore.Task, error) {
	if !stableID(accountID) {
		return taskstore.Task{}, errors.New("账号必须使用有效的稳定 ID")
	}
	if _, ok := s.repository.(manualPriorityRepository); !ok {
		return taskstore.Task{}, errors.New("人工优先位服务尚未就绪")
	}
	mode, err := s.repository.Mode(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	if mode != runtimepolicy.Full {
		return taskstore.Task{}, errors.New("取消人工优先位需要完全模式")
	}
	return s.enqueue(ctx, "sub2api-account-manual-priority", "account-manual-priority-clear", "人工优先位取消已排队", func(run context.Context) (map[string]any, error) {
		return s.clearManualPriority(run, accountID, actor)
	})
}

func (s *Service) clearManualPriority(ctx context.Context, accountID, actor string) (map[string]any, error) {
	repository := s.repository.(manualPriorityRepository)
	release, err := repository.ManualPriorityRelease(ctx, accountID)
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
	operationID, err := operationID("manual-priority-clear")
	if err != nil {
		return nil, err
	}
	beforeValues := manualPriorityValues(before)
	body := map[string]any{
		"priority":    release.Priority,
		"concurrency": release.Concurrency,
		"load_factor": json.Number("0"),
	}
	if release.LoadFactor != nil {
		body["load_factor"] = json.Number(*release.LoadFactor)
	}
	expected := map[string]any{
		"priority":    release.Priority,
		"concurrency": release.Concurrency,
		"load_factor": release.LoadFactor,
	}
	if _, err := client.Mutate(ctx, http.MethodPut, "/admin/accounts/"+accountID, body); err != nil {
		operation := manualPriorityClearOperation(operationID, actor, accountID, release.AccountName, beforeValues, expected, false, false)
		s.recordFailure(ctx, operation, "remote-write", err, false)
		return nil, &OperationError{Message: err.Error()}
	}
	after, err := client.Account(ctx, accountID)
	if err != nil {
		operation := manualPriorityClearOperation(operationID, actor, accountID, release.AccountName, beforeValues, expected, true, false)
		s.recordFailure(ctx, operation, "readback", err, true)
		return nil, &OperationError{Message: "管理平台写入成功，但人工优先位恢复结果读回失败：" + err.Error(), RemoteWritten: true}
	}
	if err := verifyManualPriorityRelease(after, release); err != nil {
		operation := manualPriorityClearOperation(operationID, actor, accountID, remoteName(after, release.AccountName), beforeValues, expected, true, false)
		s.recordFailure(ctx, operation, "readback", err, true)
		return nil, &OperationError{Message: "管理平台写入成功，但" + err.Error(), RemoteWritten: true}
	}
	effective := manualPriorityValues(after)
	operation := manualPriorityClearOperation(operationID, actor, accountID, remoteName(after, release.AccountName), beforeValues, effective, true, true)
	if err := repository.CommitManualPriorityRelease(ctx, release, actor, operation); err != nil {
		return nil, &OperationError{Message: "管理平台已恢复原参数，但本地人工优先位提交失败：" + err.Error(), RemoteWritten: true}
	}
	return map[string]any{
		"operation_id": operationID, "account_id": accountID, "manual_priority": nil,
		"before": beforeValues, "after": effective, "remote_write": true, "readback_confirmed": true,
	}, nil
}

func rollbackManualPriority(ctx context.Context, repository manualPriorityRepository, accountID string, previous *int64, loadFactor string, concurrency int64, actor string) error {
	if previous == nil {
		return repository.RevertManualPriorityReservation(ctx, accountID, actor)
	}
	_, err := repository.AssignManualPriority(ctx, accountID, *previous, loadFactor, concurrency, actor)
	return err
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func (s *Service) enqueue(
	ctx context.Context,
	skill, operation, message string,
	execute func(context.Context) (map[string]any, error),
) (taskstore.Task, error) {
	id, err := taskID()
	if err != nil {
		return taskstore.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: id, Skill: skill, Operation: operation, Status: "queued", Progress: 0, Message: message, Result: map[string]any{}, CreatedAt: now, UpdatedAt: now}
	if err := s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	go func(task taskstore.Task) {
		run, cancel := context.WithTimeout(context.Background(), s.timeout)
		defer cancel()
		task.Status, task.Progress, task.Message, task.UpdatedAt = "running", 20, "正在执行"+message[:len(message)-len("已排队")], time.Now().UTC().Format(time.RFC3339Nano)
		if err := s.tasks.Save(run, task); err != nil {
			return
		}
		result, err := execute(run)
		task.Progress, task.UpdatedAt = 100, time.Now().UTC().Format(time.RFC3339Nano)
		if err != nil {
			remoteWritten := false
			var operationError *OperationError
			if errors.As(err, &operationError) {
				remoteWritten = operationError.RemoteWritten
			}
			task.Status, task.Message, task.Result = "failed", strings.TrimSuffix(message, "已排队")+"失败："+err.Error(), map[string]any{"error": err.Error(), "remote_write": remoteWritten}
		} else {
			task.Status, task.Message, task.Result = "succeeded", strings.TrimSuffix(message, "已排队")+"完成", result
		}
		taskstore.PersistFinal(s.tasks, task)
	}(task)
	return task, nil
}

func (s *Service) localAccount(ctx context.Context, accountID string) (string, *business.AccountDetail, error) {
	mode, err := s.repository.Mode(ctx)
	if err != nil {
		return "", nil, err
	}
	detail, err := s.repository.Account(ctx, accountID)
	if err != nil {
		return "", nil, err
	}
	return mode, detail, nil
}

func (s *Service) adminClient(ctx context.Context) (*adminclient.Client, error) {
	target, err := s.targets.TargetSettings(ctx)
	if err != nil {
		return nil, err
	}
	return adminclient.New(adminclient.Config{
		BaseURL: target.BaseURL, AdminKey: target.AdminKey,
		Timeout: time.Duration(target.TimeoutSeconds) * time.Second, Attempts: 3,
	}, nil)
}

func (s *Service) recordFailure(ctx context.Context, operation business.AccountOperation, phase string, cause error, remoteWritten bool) {
	detail := cause.Error()
	operation.State, operation.Phase, operation.Error = "failed", phase, &detail
	operation.RemoteConfirmed, operation.ReadbackConfirmed, operation.Writeback = remoteWritten, false, true
	if err := s.repository.RecordAccountOperation(ctx, operation); err != nil {
		slog.Error("账号操作失败记录保存失败", "operation_id", operation.OperationID, "account_id", operation.ObjectID, "error", err)
	}
}

func writeAccountSchedulable(ctx context.Context, client *adminclient.Client, accountID string, schedulable bool) error {
	body := map[string]any{"schedulable": schedulable}
	_, err := client.Mutate(ctx, http.MethodPost, "/admin/accounts/"+accountID+"/schedulable", body)
	if err == nil {
		return nil
	}
	var httpError *adminclient.HTTPError
	if !errors.As(err, &httpError) || (httpError.StatusCode != http.StatusNotFound && httpError.StatusCode != http.StatusMethodNotAllowed) {
		return err
	}
	_, fallbackErr := client.Mutate(ctx, http.MethodPut, "/admin/accounts/"+accountID, body)
	return fallbackErr
}

func recoverAccountRuntime(ctx context.Context, client *adminclient.Client, accountID string) []string {
	warnings := []string{}
	for _, endpoint := range []struct {
		path  string
		label string
	}{
		{path: "/admin/accounts/" + accountID + "/clear-error", label: "清除错误信息"},
		{path: "/admin/accounts/" + accountID + "/recover-state", label: "复位运行状态"},
	} {
		if _, err := client.Mutate(ctx, http.MethodPost, endpoint.path, nil); err != nil {
			warnings = append(warnings, endpoint.label+"失败："+err.Error())
		}
	}
	return warnings
}

func verifyManualPriorityRelease(after map[string]any, release business.ManualPriorityRelease) error {
	priority, err := readbackInteger(after["priority"])
	if err != nil || priority != release.Priority {
		return errors.New("账号原优先级读回不一致")
	}
	concurrency, err := readbackInteger(after["concurrency"])
	if err != nil || concurrency != release.Concurrency {
		return errors.New("账号原并发上限读回不一致")
	}
	loadFactor, err := optionalLoadFactor(after)
	if err != nil || !sameOptionalText(loadFactor, release.LoadFactor) {
		return errors.New("账号原负载因子读回不一致")
	}
	return nil
}

func optionalLoadFactor(account map[string]any) (*string, error) {
	raw, present := account["load_factor"]
	if !present || raw == nil || strings.TrimSpace(fmt.Sprint(raw)) == "" {
		return nil, nil
	}
	value, err := decimal(fmt.Sprint(raw))
	if err != nil {
		return nil, err
	}
	parsed, _ := new(big.Rat).SetString(value)
	if parsed.Sign() <= 0 {
		return nil, nil
	}
	return &value, nil
}

func sameOptionalText(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func manualPriorityValues(account map[string]any) map[string]any {
	result := map[string]any{
		"priority":    account["priority"],
		"load_factor": account["load_factor"],
		"concurrency": account["concurrency"],
	}
	return result
}

func manualPriorityClearOperation(id, actor, accountID, name string, before, after any, remote, readback bool) business.AccountOperation {
	field := "priority,load_factor,concurrency"
	phase := "readback"
	if remote && !readback {
		phase = "remote-write"
	}
	return business.AccountOperation{
		OperationID: id, OperationType: "account.manual_priority.clear", State: "succeeded", Phase: phase,
		Actor: actor, RemoteConfirmed: remote, ReadbackConfirmed: readback, ObjectID: accountID,
		ObjectName: &name, GroupNames: []string{}, FieldName: &field, Before: before, After: after, Writeback: true,
	}
}

func accountSchedulable(account map[string]any) (bool, error) {
	raw, present := account["schedulable"]
	if !present {
		return false, errors.New("管理平台账号未返回 schedulable")
	}
	value, ok := raw.(bool)
	if !ok {
		return false, errors.New("管理平台账号 schedulable 格式无效")
	}
	return value, nil
}

func controlOperation(id, actor, accountID, name, action string, before, after any, remote, readback bool) business.AccountOperation {
	phase := "remote-write"
	if remote {
		phase = "readback"
	}
	field := "schedulable"
	return business.AccountOperation{
		OperationID: id, OperationType: "account.control", State: "succeeded", Phase: phase, Actor: actor,
		RemoteConfirmed: remote, ReadbackConfirmed: readback, ObjectID: accountID, ObjectName: &name,
		GroupNames: []string{}, FieldName: &field, Before: before,
		After: map[string]any{"action": action, "values": after}, Writeback: remote,
	}
}

func fieldOperation(id, actor, accountID, name, field string, before, after any, remote, readback bool) business.AccountOperation {
	phase := "readback"
	if remote && !readback {
		phase = "remote-write"
	}
	return business.AccountOperation{
		OperationID: id, OperationType: "account.sync", State: "succeeded", Phase: phase, Actor: actor,
		RemoteConfirmed: remote, ReadbackConfirmed: readback, ObjectID: accountID, ObjectName: &name,
		GroupNames: []string{}, FieldName: &field, Before: before, After: after, Writeback: remote,
	}
}

func stableID(value string) bool {
	parsed, err := strconv.ParseUint(value, 10, 64)
	return err == nil && parsed > 0 && strconv.FormatUint(parsed, 10) == value
}

func operationID(prefix string) (string, error) {
	value := make([]byte, 8)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return prefix + "-" + hex.EncodeToString(value), nil
}

func taskID() (string, error) {
	value := make([]byte, 6)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func positiveDecimal(value string) (string, error) {
	normalized, err := decimal(value)
	if err != nil {
		return "", err
	}
	parsed, _ := new(big.Rat).SetString(normalized)
	if parsed.Sign() <= 0 {
		return "", errors.New("not positive")
	}
	return normalized, nil
}

func decimalAtLeastOne(value string) (string, error) {
	normalized, err := decimal(value)
	if err != nil {
		return "", err
	}
	parsed, _ := new(big.Rat).SetString(normalized)
	if parsed.Cmp(big.NewRat(1, 1)) < 0 {
		return "", errors.New("less than one")
	}
	return normalized, nil
}

func decimal(value string) (string, error) {
	parsed, ok := new(big.Rat).SetString(strings.TrimSpace(value))
	if !ok {
		return "", errors.New("not decimal")
	}
	text := parsed.FloatString(28)
	text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	if text == "" || text == "-0" {
		text = "0"
	}
	return text, nil
}

func firstPresent(value map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if current, present := value[key]; present {
			return current, true
		}
	}
	return nil, false
}

func remoteName(value map[string]any, fallback string) string {
	raw, present := firstPresent(value, "name", "username")
	if present {
		if name, ok := raw.(string); ok && strings.TrimSpace(name) != "" {
			return strings.TrimSpace(name)
		}
	}
	return fallback
}

func fieldAuditValues(before map[string]any, name *string, priority *int64, loadFactor *string, concurrency *int64, multiplier *string, patch FieldPatch) (map[string]any, map[string]any) {
	previous, requested := map[string]any{}, map[string]any{}
	if name != nil {
		previous["name"], requested["name"] = before["name"], *name
	}
	if priority != nil {
		previous["priority"], requested["priority"] = before["priority"], *priority
	}
	if loadFactor != nil {
		previous["load_factor"], requested["load_factor"] = before["load_factor"], *loadFactor
	}
	if concurrency != nil {
		previous["concurrency"], requested["concurrency"] = before["concurrency"], *concurrency
	}
	if multiplier != nil {
		previous["rate_multiplier"], _ = firstPresent(before, "rate_multiplier", "multiplier")
		requested["rate_multiplier"] = *multiplier
	}
	if patch.NotesPresent {
		previous["notes"] = "已设置"
		if raw, present := before["notes"]; !present || raw == nil || raw == "" {
			previous["notes"] = "空值"
		}
		if patch.Notes == nil {
			requested["notes"] = "已清空"
		} else {
			requested["notes"] = "已更新"
		}
	}
	return previous, requested
}

func verifyFieldReadback(after map[string]any, name *string, priority *int64, loadFactor *string, concurrency *int64, multiplier *string, patch FieldPatch) error {
	if name != nil {
		value, ok := after["name"].(string)
		if !ok || value != *name {
			return errors.New("账号名称读回不一致")
		}
	}
	if priority != nil {
		value, err := readbackInteger(after["priority"])
		if err != nil || value != *priority {
			return errors.New("账号优先级读回不一致")
		}
	}
	if loadFactor != nil {
		value, err := decimal(fmt.Sprint(after["load_factor"]))
		if err != nil || value != *loadFactor {
			return errors.New("账号负载因子读回不一致")
		}
	}
	if concurrency != nil {
		value, err := readbackInteger(after["concurrency"])
		if err != nil || value != *concurrency {
			return errors.New("账号并发上限读回不一致")
		}
	}
	if multiplier != nil {
		raw, present := firstPresent(after, "rate_multiplier", "multiplier")
		if !present || raw == nil {
			return errors.New("账号倍率读回不可判定")
		}
		value, err := decimal(fmt.Sprint(raw))
		if err != nil || value != *multiplier {
			return errors.New("账号倍率读回不一致")
		}
	}
	if patch.NotesPresent {
		raw, present := after["notes"]
		if !present || (patch.Notes == nil && raw != nil) || (patch.Notes != nil && raw != *patch.Notes) {
			return errors.New("账号备注读回不一致")
		}
	}
	return nil
}

func fieldEffectiveValues(after map[string]any, name *string, priority *int64, loadFactor *string, concurrency *int64, multiplier *string, patch FieldPatch) map[string]any {
	result := map[string]any{}
	if name != nil {
		result["name"] = after["name"]
	}
	if priority != nil {
		result["priority"] = after["priority"]
	}
	if loadFactor != nil {
		result["load_factor"] = after["load_factor"]
	}
	if concurrency != nil {
		result["concurrency"] = after["concurrency"]
	}
	if multiplier != nil {
		result["rate_multiplier"], _ = firstPresent(after, "rate_multiplier", "multiplier")
	}
	if patch.NotesPresent {
		result["notes_updated"] = true
	}
	return result
}

func readbackInteger(raw any) (int64, error) {
	text := strings.TrimSpace(fmt.Sprint(raw))
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, errors.New("not integer")
	}
	return value, nil
}

func sortedKeys(value map[string]any) []string {
	result := make([]string, 0, len(value))
	for key := range value {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
