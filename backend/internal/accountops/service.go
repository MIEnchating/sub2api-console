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
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
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
	CommitAccountFieldsReadback(context.Context, string, *string, *int64, *string, *int64, *string, *string, *string, bool, *string, business.AccountOperation) error
	RecordAccountOperation(context.Context, business.AccountOperation) error
	SaveAccountModels(context.Context, string, []string) error
}

type manualPriorityRepository interface {
	ManualPriorityConfig(context.Context) (business.ManualPriorityConfig, error)
	AssignManualPriority(context.Context, string, int64, string, int64, bool, string) (business.ManualPriorityAssignment, error)
	RevertManualPriorityReservation(context.Context, string, string) error
	ManualPriorityRelease(context.Context, string) (business.ManualPriorityRelease, error)
	CommitManualPriorityRelease(context.Context, business.ManualPriorityRelease, string, business.AccountOperation) error
}

type accountSettingsRepository interface {
	CommitAccountSettings(context.Context, string, string, business.AccountSettingsUpdate) error
}

type TaskStore interface {
	Save(context.Context, taskstore.Task) error
}

type FieldPatch struct {
	NamePresent         bool
	Name                *string
	PriorityPresent     bool
	Priority            *int64
	LoadFactorPresent   bool
	LoadFactor          *string
	ConcurrencyPresent  bool
	Concurrency         *int64
	MultiplierPresent   bool
	Multiplier          *string
	UpstreamHostPresent bool
	UpstreamHost        *string
	BaseURLPresent      bool
	BaseURL             *string
	NotesPresent        bool
	Notes               *string
}

type SettingsInput struct {
	Priority    int64
	LoadFactor  string
	Concurrency int64
	TestModel   *string
	Paused      bool
	Excluded    bool
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
	taskRunner taskrunner.Runner
	timeout    time.Duration
}

func New(targets TargetStore, repository Repository, tasks TaskStore) *Service {
	return &Service{targets: targets, repository: repository, tasks: tasks, timeout: 10 * time.Minute}
}

func (s *Service) UseTaskRunner(runner taskrunner.Runner) { s.taskRunner = runner }

func (s *Service) SyncFields(ctx context.Context, accountID string, patch FieldPatch, actor string) (map[string]any, error) {
	return s.syncFields(ctx, accountID, patch, actor, false, "", nil)
}

// SyncAccountMultiplier writes a multiplier obtained from the account's
// upstream billing probe through the mandatory management readback path.
func (s *Service) SyncAccountMultiplier(ctx context.Context, accountID, multiplier, actor string) (map[string]any, error) {
	return s.SyncAccountMultiplierIfCurrent(ctx, accountID, multiplier, actor, "", nil)
}

// SyncAccountMultiplierIfCurrent evaluates check after atomically acquiring the
// account lease and rate source lease. A failure prevents remote access.
func (s *Service) SyncAccountMultiplierIfCurrent(
	ctx context.Context,
	accountID, multiplier, actor string,
	rateSourceHost string,
	check func(context.Context) error,
) (map[string]any, error) {
	return s.syncFields(ctx, accountID, FieldPatch{
		MultiplierPresent: true,
		Multiplier:        &multiplier,
	}, actor, false, rateSourceHost, check)
}

// SyncAccountRate updates the rate-derived account name and multiplier in one
// management mutation and requires both fields to match on readback.
func (s *Service) SyncAccountRate(ctx context.Context, accountID, name, multiplier, actor string) (map[string]any, error) {
	return s.SyncAccountRateIfCurrent(ctx, accountID, name, multiplier, actor, "", nil)
}

// SyncAccountRateIfCurrent evaluates check after atomically acquiring the
// account lease and rate source lease. A failure prevents remote access.
func (s *Service) SyncAccountRateIfCurrent(
	ctx context.Context,
	accountID, name, multiplier, actor string,
	rateSourceHost string,
	check func(context.Context) error,
) (map[string]any, error) {
	return s.syncFields(ctx, accountID, FieldPatch{
		NamePresent:       true,
		Name:              &name,
		MultiplierPresent: true,
		Multiplier:        &multiplier,
	}, actor, false, rateSourceHost, check)
}

func (s *Service) Control(ctx context.Context, accountID, action, actor string) (map[string]any, error) {
	if !stableID(accountID) {
		return nil, errors.New("账号必须使用有效的稳定 ID")
	}
	schedulable, remoteAction := controlSchedulable(action)
	if !remoteAction {
		return nil, errors.New("账号远端控制 action 无效")
	}
	ctx, releaseMutation, err := s.acquireAccountMutation(ctx, accountID)
	if err != nil {
		return nil, err
	}
	defer releaseMutation()
	return s.controlLocked(ctx, accountID, action, actor, schedulable)
}

func (s *Service) controlLocked(ctx context.Context, accountID, action, actor string, schedulable bool) (map[string]any, error) {
	mode, local, err := s.localAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if local.ManualPriority != nil {
		return nil, errors.New("账号处于人工优先位，平台控制操作已禁用；请先取消人工优先位")
	}
	if mode != runtimepolicy.Full {
		return nil, errors.New("暂停、恢复和熔断操作需要完全模式")
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
	var expectedTarget configstore.TargetSettings
	if remoteAction {
		expectedTarget, err = s.targets.TargetSettings(ctx)
		if err != nil {
			return taskstore.Task{}, err
		}
	}
	return s.enqueue(ctx, "sub2api-account-control", "account-control", "账号控制操作已排队", func(run context.Context) (map[string]any, error) {
		if remoteAction {
			return s.Control(targetguard.Expect(run, expectedTarget), accountID, action, actor)
		}
		return s.scopeControl(run, accountID, action, actor)
	})
}

func (s *Service) scopeControl(ctx context.Context, accountID, action, actor string) (map[string]any, error) {
	if !stableID(accountID) {
		return nil, errors.New("账号必须使用有效的稳定 ID")
	}
	if action != "exclude" && action != "include" {
		return nil, errors.New("账号受管范围只允许 exclude 或 include")
	}
	ctx, releaseMutation, err := s.acquireLocalAccountMutation(ctx, accountID)
	if err != nil {
		return nil, err
	}
	defer releaseMutation()
	_, local, err := s.localAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if local.ManualPriority != nil {
		return nil, errors.New("账号处于人工优先位，平台控制操作已禁用；请先取消人工优先位")
	}
	if _, err := s.repository.SetAccountScopeControl(ctx, accountID, action, actor); err != nil {
		return nil, err
	}
	return map[string]any{
		"account_id": accountID, "action": action, "saved": true,
		"remote_write": false, "readback_confirmed": false,
	}, nil
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

func (s *Service) syncFields(
	ctx context.Context,
	accountID string,
	patch FieldPatch,
	actor string,
	allowReservedPriority bool,
	rateSourceHost string,
	check func(context.Context) error,
) (map[string]any, error) {
	if !stableID(accountID) {
		return nil, errors.New("账号必须使用有效的稳定 ID")
	}
	rateSourceResource := mutationguard.Upstream(rateSourceHost)
	if check != nil && rateSourceResource == "" {
		return nil, errors.New("账号倍率同步来源 Host 无效")
	}
	additionalResources := []string{}
	if rateSourceResource != "" {
		additionalResources = append(additionalResources, rateSourceResource)
	}
	ctx, releaseMutation, err := s.acquireAccountMutation(ctx, accountID, additionalResources...)
	if err != nil {
		return nil, err
	}
	defer releaseMutation()
	if check != nil {
		if err := check(ctx); err != nil {
			return nil, err
		}
	}
	return s.syncFieldsLocked(ctx, accountID, patch, actor, allowReservedPriority)
}

func (s *Service) syncFieldsLocked(ctx context.Context, accountID string, patch FieldPatch, actor string, allowReservedPriority bool) (map[string]any, error) {
	if !patch.NamePresent && !patch.PriorityPresent && !patch.LoadFactorPresent && !patch.ConcurrencyPresent && !patch.MultiplierPresent && !patch.NotesPresent {
		return nil, errors.New("至少提供一个需要同步的账号字段")
	}
	mode, local, err := s.localAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if local.ManualPriority != nil && !allowReservedPriority {
		multiplierOnly := patch.MultiplierPresent && !patch.NamePresent && !patch.PriorityPresent &&
			!patch.LoadFactorPresent && !patch.ConcurrencyPresent && !patch.NotesPresent
		if !multiplierOnly || !local.ManualSyncBalanceMultiplier {
			return nil, errors.New("账号处于人工优先位，仅允许按人工控制设置同步余额与倍率")
		}
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
	var normalizedName, normalizedLoadFactor, normalizedMultiplier, normalizedUpstreamHost, normalizedBaseURL *string
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
	if patch.UpstreamHostPresent {
		if patch.UpstreamHost == nil {
			return nil, errors.New("归属上游 Host 不能为 null；省略字段表示不修改")
		}
		value := configstore.CanonicalHost(*patch.UpstreamHost)
		if value == "" || strings.ContainsAny(value, "/\\?#") {
			return nil, errors.New("归属上游 Host 无效")
		}
		normalizedUpstreamHost = &value
	}
	if patch.BaseURLPresent {
		if patch.BaseURL == nil {
			return nil, errors.New("账号 Base URL 不能为 null；省略字段表示不修改")
		}
		value, err := normalizedAccountBaseURL(*patch.BaseURL)
		if err != nil {
			return nil, err
		}
		normalizedBaseURL = &value
		body["credentials"] = map[string]any{"base_url": value}
	}
	if patch.NotesPresent {
		if patch.Notes == nil {
			body["notes"] = nil
		} else {
			body["notes"] = *patch.Notes
		}
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
	operationID, err := operationID("account-sync")
	if err != nil {
		return nil, err
	}
	beforeValues, requested := fieldAuditValues(before, normalizedName, normalizedPriority, normalizedLoadFactor, normalizedConcurrency, normalizedMultiplier, normalizedBaseURL, patch)
	if normalizedUpstreamHost != nil {
		beforeHost := local.UpstreamHost
		if local.RecordedUpstreamHost != nil {
			beforeHost = local.RecordedUpstreamHost
		}
		beforeValues["upstream_host"] = beforeHost
		requested["upstream_host"] = *normalizedUpstreamHost
	}
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
	if err := verifyFieldReadback(after, normalizedName, normalizedPriority, normalizedLoadFactor, normalizedConcurrency, normalizedMultiplier, normalizedBaseURL, patch); err != nil {
		operation := fieldOperation(operationID, actor, accountID, remoteName(after, accountName), fieldName, beforeValues, requested, true, false)
		s.recordFailure(ctx, operation, "readback", err, true)
		return nil, &OperationError{Message: "管理平台写入成功，但" + err.Error(), RemoteWritten: true}
	}
	readbackConfirmed := true
	accountName = remoteName(after, accountName)
	effective := fieldEffectiveValues(after, normalizedName, normalizedPriority, normalizedLoadFactor, normalizedConcurrency, normalizedMultiplier, normalizedBaseURL, patch)
	if normalizedUpstreamHost != nil {
		effective["upstream_host"] = *normalizedUpstreamHost
	}
	operation := fieldOperation(operationID, actor, accountID, remoteName(after, accountName), fieldName, beforeValues, effective, remoteWritten, readbackConfirmed)
	if err := s.repository.CommitAccountFieldsReadback(ctx, accountID, normalizedName, normalizedPriority, normalizedLoadFactor, normalizedConcurrency, normalizedMultiplier, normalizedUpstreamHost, normalizedBaseURL, patch.NotesPresent, patch.Notes, operation); err != nil {
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
	ctx, releaseMutation, err := s.acquireAccountMutation(ctx, accountID)
	if err != nil {
		return nil, err
	}
	defer releaseMutation()
	if _, _, err := s.localAccount(ctx, accountID); err != nil {
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
	expectedTarget, err := s.targets.TargetSettings(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	return s.enqueue(ctx, "sub2api-account-sync", "account-fields-sync", "账号字段同步已排队", func(run context.Context) (map[string]any, error) {
		return s.SyncFields(targetguard.Expect(run, expectedTarget), accountID, patch, actor)
	})
}

func (s *Service) EnqueueSettings(ctx context.Context, accountID string, input SettingsInput, actor string) (taskstore.Task, error) {
	if !stableID(accountID) {
		return taskstore.Task{}, errors.New("账号必须使用有效的稳定 ID")
	}
	if input.Priority < 1 || input.Priority > 10_000_000 {
		return taskstore.Task{}, errors.New("优先级必须是 1 到 10000000 之间的整数")
	}
	loadFactor, err := decimalAtLeastOne(input.LoadFactor)
	if err != nil {
		return taskstore.Task{}, errors.New("负载因子必须大于或等于 1")
	}
	input.LoadFactor = loadFactor
	if input.Concurrency < 1 || input.Concurrency > 10_000_000 {
		return taskstore.Task{}, errors.New("并发上限必须是 1 到 10000000 之间的整数")
	}
	if input.TestModel != nil {
		model := strings.TrimSpace(*input.TestModel)
		if len(model) > 256 {
			return taskstore.Task{}, errors.New("探测模型长度不能超过 256")
		}
		input.TestModel = &model
	}
	if _, ok := s.repository.(accountSettingsRepository); !ok {
		return taskstore.Task{}, errors.New("账号设置服务尚未就绪")
	}
	mode, local, err := s.localAccount(ctx, accountID)
	if err != nil {
		return taskstore.Task{}, err
	}
	if mode != runtimepolicy.Full {
		return taskstore.Task{}, errors.New("账号设置需要完全模式")
	}
	if local.ManualPriority != nil {
		return taskstore.Task{}, errors.New("账号处于人工优先位，平台设置已禁用；请先取消人工优先位")
	}
	expectedTarget, err := s.targets.TargetSettings(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	return s.enqueue(ctx, "sub2api-account-settings", "account-settings", "账号设置已排队", func(run context.Context) (map[string]any, error) {
		return s.applySettings(targetguard.Expect(run, expectedTarget), accountID, input, actor)
	})
}

func (s *Service) applySettings(ctx context.Context, accountID string, input SettingsInput, actor string) (map[string]any, error) {
	repository, ok := s.repository.(accountSettingsRepository)
	if !ok {
		return nil, errors.New("账号设置服务尚未就绪")
	}
	ctx, releaseMutation, err := s.acquireAccountMutation(ctx, accountID)
	if err != nil {
		return nil, err
	}
	defer releaseMutation()
	mode, local, err := s.localAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if mode != runtimepolicy.Full || local.ManualPriority != nil {
		return nil, errors.New("账号设置执行条件已变化，请刷新后重试")
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
	rollbackBody, err := accountSettingsRollbackBody(before)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"priority": input.Priority, "load_factor": json.Number(input.LoadFactor),
		"concurrency": input.Concurrency, "schedulable": !input.Paused,
	}
	if _, err := client.Mutate(ctx, http.MethodPut, "/admin/accounts/"+accountID, body); err != nil {
		return nil, &OperationError{Message: err.Error()}
	}
	rollback := func(cause error) error {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		_, rollbackErr := client.Mutate(rollbackCtx, http.MethodPut, "/admin/accounts/"+accountID, rollbackBody)
		if rollbackErr != nil {
			return fmt.Errorf("%w；账号设置远端回滚失败：%v", cause, rollbackErr)
		}
		return cause
	}
	after, err := client.Account(ctx, accountID)
	if err != nil {
		return nil, rollback(fmt.Errorf("账号设置写入后读回失败：%w", err))
	}
	patch := FieldPatch{
		PriorityPresent: true, Priority: &input.Priority, LoadFactorPresent: true, LoadFactor: &input.LoadFactor,
		ConcurrencyPresent: true, Concurrency: &input.Concurrency,
	}
	if err := verifyFieldReadback(after, nil, &input.Priority, &input.LoadFactor, &input.Concurrency, nil, nil, patch); err != nil {
		return nil, rollback(err)
	}
	schedulable, err := accountSchedulable(after)
	if err != nil || schedulable != !input.Paused {
		if err == nil {
			err = errors.New("账号可调度状态读回不一致")
		}
		return nil, rollback(err)
	}
	operationID, err := operationID("account-settings")
	if err != nil {
		return nil, rollback(err)
	}
	field := "priority,load_factor,concurrency,schedulable,test_model,excluded"
	beforeValues := map[string]any{
		"priority": rollbackBody["priority"], "load_factor": rollbackBody["load_factor"],
		"concurrency": rollbackBody["concurrency"], "schedulable": rollbackBody["schedulable"],
		"test_model": local.TestModel,
	}
	afterValues := map[string]any{
		"priority": input.Priority, "load_factor": input.LoadFactor, "concurrency": input.Concurrency,
		"schedulable": !input.Paused, "test_model": input.TestModel, "excluded": input.Excluded,
	}
	operation := business.AccountOperation{
		OperationID: operationID, OperationType: "account.settings", State: "succeeded", Phase: "readback",
		Actor: actor, RemoteConfirmed: true, ReadbackConfirmed: true, ObjectID: accountID,
		ObjectName: &local.Name, GroupNames: []string{}, FieldName: &field, Before: beforeValues,
		After: afterValues, Writeback: true,
	}
	if err := repository.CommitAccountSettings(ctx, accountID, actor, business.AccountSettingsUpdate{
		Priority: input.Priority, LoadFactor: input.LoadFactor, Concurrency: input.Concurrency,
		TestModel: input.TestModel, Paused: input.Paused, Excluded: input.Excluded, Operation: operation,
	}); err != nil {
		return nil, rollback(fmt.Errorf("远端设置已写入，但本地原子提交失败：%w", err))
	}
	return map[string]any{
		"operation_id": operationID, "account_id": accountID, "before": beforeValues,
		"after": afterValues, "remote_write": true, "readback_confirmed": true,
	}, nil
}

func accountSettingsRollbackBody(account map[string]any) (map[string]any, error) {
	priority, err := readbackInteger(account["priority"])
	if err != nil || priority < 1 {
		return nil, errors.New("账号原优先级不可读")
	}
	loadFactor, err := decimal(fmt.Sprint(account["load_factor"]))
	if err != nil {
		return nil, errors.New("账号原负载因子不可读")
	}
	concurrency, err := readbackInteger(account["concurrency"])
	if err != nil || concurrency < 1 {
		return nil, errors.New("账号原并发上限不可读")
	}
	schedulable, err := accountSchedulable(account)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"priority": priority, "load_factor": json.Number(loadFactor),
		"concurrency": concurrency, "schedulable": schedulable,
	}, nil
}

func (s *Service) EnqueueManualPriority(ctx context.Context, accountID string, priority int64, loadFactor string, concurrency int64, syncBalanceMultiplier bool, actor string) (taskstore.Task, error) {
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
	expectedTarget, err := s.targets.TargetSettings(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	return s.enqueue(ctx, "sub2api-account-manual-priority", "account-manual-priority", "人工优先位设置已排队", func(run context.Context) (map[string]any, error) {
		return s.setManualPriority(targetguard.Expect(run, expectedTarget), manualRepository, config, accountID, priority, loadFactor, concurrency, syncBalanceMultiplier, actor)
	})
}

func (s *Service) setManualPriority(
	ctx context.Context,
	repository manualPriorityRepository,
	config business.ManualPriorityConfig,
	accountID string,
	priority int64,
	loadFactor string,
	concurrency int64,
	syncBalanceMultiplier bool,
	actor string,
) (map[string]any, error) {
	if !stableID(accountID) {
		return nil, errors.New("账号必须使用有效的稳定 ID")
	}
	ctx, releaseMutation, err := s.acquireAccountMutation(ctx, accountID)
	if err != nil {
		return nil, err
	}
	defer releaseMutation()

	before, err := s.repository.Account(ctx, accountID)
	if err != nil {
		return nil, err
	}
	previousManualPriority := cloneInt64(before.ManualPriority)
	previousSyncBalanceMultiplier := before.ManualSyncBalanceMultiplier
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
	assignment, err := repository.AssignManualPriority(ctx, accountID, priority, loadFactor, concurrency, syncBalanceMultiplier, actor)
	if err != nil {
		return nil, err
	}
	loadFactor = assignment.LoadFactor
	concurrency = assignment.Concurrency
	patch := FieldPatch{
		PriorityPresent: true, Priority: &assignment.Priority,
		LoadFactorPresent: true, LoadFactor: &loadFactor,
		ConcurrencyPresent: true, Concurrency: &concurrency,
	}
	result, err := s.syncFieldsLocked(ctx, accountID, patch, actor, true)
	if err != nil {
		var operationError *OperationError
		if !errors.As(err, &operationError) || !operationError.RemoteWritten {
			rollbackErr := rollbackManualPriority(ctx, repository, accountID, previousManualPriority, rollbackLoadFactor, rollbackConcurrency, previousSyncBalanceMultiplier, actor)
			if rollbackErr != nil {
				return nil, fmt.Errorf("%w；人工优先位回滚失败：%v", err, rollbackErr)
			}
		}
		return nil, err
	}
	result["manual_priority"] = assignment.Priority
	result["sync_balance_multiplier"] = assignment.SyncBalanceMultiplier
	return result, nil
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
	expectedTarget, err := s.targets.TargetSettings(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	return s.enqueue(ctx, "sub2api-account-manual-priority", "account-manual-priority-clear", "人工优先位取消已排队", func(run context.Context) (map[string]any, error) {
		return s.clearManualPriority(targetguard.Expect(run, expectedTarget), accountID, actor)
	})
}

func (s *Service) clearManualPriority(ctx context.Context, accountID, actor string) (map[string]any, error) {
	if !stableID(accountID) {
		return nil, errors.New("账号必须使用有效的稳定 ID")
	}
	ctx, releaseMutation, err := s.acquireAccountMutation(ctx, accountID)
	if err != nil {
		return nil, err
	}
	defer releaseMutation()
	return s.clearManualPriorityLocked(ctx, accountID, actor)
}

func (s *Service) clearManualPriorityLocked(ctx context.Context, accountID, actor string) (map[string]any, error) {
	repository, ok := s.repository.(manualPriorityRepository)
	if !ok {
		return nil, errors.New("人工优先位服务尚未就绪")
	}
	release, err := repository.ManualPriorityRelease(ctx, accountID)
	if err != nil {
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

func rollbackManualPriority(ctx context.Context, repository manualPriorityRepository, accountID string, previous *int64, loadFactor string, concurrency int64, syncBalanceMultiplier bool, actor string) error {
	if previous == nil {
		return repository.RevertManualPriorityReservation(ctx, accountID, actor)
	}
	_, err := repository.AssignManualPriority(ctx, accountID, *previous, loadFactor, concurrency, syncBalanceMultiplier, actor)
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
	queuedTask := task
	if err := taskrunner.Go(s.taskRunner, func(parent context.Context) {
		task := queuedTask
		run, cancel := context.WithTimeout(parent, s.timeout)
		defer cancel()
		task.Status, task.Progress, task.Message, task.UpdatedAt = "running", 20, "正在执行"+message[:len(message)-len("已排队")], time.Now().UTC().Format(time.RFC3339Nano)
		if !taskstore.SaveRunning(run, s.tasks, task) {
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
		taskstore.MarkCancelled(run, &task, strings.TrimSuffix(message, "已排队")+"已取消")
		taskstore.PersistFinal(s.tasks, task)
	}); err != nil {
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
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

func (s *Service) acquireAccountMutation(ctx context.Context, accountID string, additionalResources ...string) (context.Context, func(), error) {
	resources := append([]string{mutationguard.Account(accountID)}, additionalResources...)
	guarded, release, err := targetguard.Acquire(ctx, s.repository, resources...)
	if err != nil {
		return nil, nil, err
	}
	return guarded, func() {
		if err := release(); err != nil {
			slog.Error("账号变更租约释放失败", "account_id", accountID, "error", err)
		}
	}, nil
}

func (s *Service) acquireLocalAccountMutation(ctx context.Context, accountID string) (context.Context, func(), error) {
	guarded, release, err := mutationguard.Acquire(ctx, s.repository, mutationguard.Account(accountID))
	if err != nil {
		return nil, nil, err
	}
	return guarded, func() {
		if err := release(); err != nil {
			slog.Error("账号本地变更租约释放失败", "account_id", accountID, "error", err)
		}
	}, nil
}

func (s *Service) adminClient(ctx context.Context) (*adminclient.Client, error) {
	target, err := targetguard.Settings(ctx, s.targets)
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

func fieldAuditValues(before map[string]any, name *string, priority *int64, loadFactor *string, concurrency *int64, multiplier, baseURL *string, patch FieldPatch) (map[string]any, map[string]any) {
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
	if baseURL != nil {
		previous["base_url"], requested["base_url"] = remoteAccountBaseURL(before), *baseURL
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

func verifyFieldReadback(after map[string]any, name *string, priority *int64, loadFactor *string, concurrency *int64, multiplier, baseURL *string, patch FieldPatch) error {
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
	if baseURL != nil && remoteAccountBaseURL(after) != *baseURL {
		return errors.New("账号 Base URL 读回不一致")
	}
	if patch.NotesPresent {
		raw, present := after["notes"]
		if !present || (patch.Notes == nil && raw != nil) || (patch.Notes != nil && raw != *patch.Notes) {
			return errors.New("账号备注读回不一致")
		}
	}
	return nil
}

func fieldEffectiveValues(after map[string]any, name *string, priority *int64, loadFactor *string, concurrency *int64, multiplier, baseURL *string, patch FieldPatch) map[string]any {
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
	if baseURL != nil {
		result["base_url"] = remoteAccountBaseURL(after)
	}
	if patch.NotesPresent {
		result["notes_updated"] = true
	}
	return result
}

func normalizedAccountBaseURL(raw string) (string, error) {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return "", errors.New("账号 Base URL 必须是完整的 HTTP/HTTPS 地址")
	}
	return value, nil
}

func remoteAccountBaseURL(account map[string]any) string {
	raw := account["base_url"]
	if credentials, ok := account["credentials"].(map[string]any); ok {
		if value, present := credentials["base_url"]; present {
			raw = value
		}
	}
	return strings.TrimRight(strings.TrimSpace(fmt.Sprint(raw)), "/")
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
