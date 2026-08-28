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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/writebackpolicy"
)

type TargetStore interface {
	TargetSettings(context.Context) (configstore.TargetSettings, error)
}

type Repository interface {
	Mode(context.Context) (string, error)
	ControlPolicy(context.Context) (map[string]any, error)
	Account(context.Context, string) (*business.AccountDetail, error)
	CommitAccountFieldsReadback(context.Context, string, *string, *int64, *string, *int64, *string, bool, *string, business.AccountOperation) error
	RecordAccountOperation(context.Context, business.AccountOperation) error
	SaveAccountModels(context.Context, string, []string) error
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
	policyDocument, err := s.repository.ControlPolicy(ctx)
	if err != nil {
		return nil, err
	}
	verification, err := writebackpolicy.Verification(policyDocument)
	if err != nil {
		return nil, err
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
	writeResponse, err := client.Mutate(ctx, "PUT", "/admin/accounts/"+accountID, body)
	if err != nil {
		operation := fieldOperation(operationID, actor, accountID, accountName, fieldName, beforeValues, requested, false, false)
		s.recordFailure(ctx, operation, "remote-write", err, false)
		return nil, &OperationError{Message: err.Error()}
	}
	remoteWritten = true
	after, readbackConfirmed := body, false
	if response, trusted := matchingMutationResponse(writeResponse, accountID, normalizedName, normalizedPriority, normalizedLoadFactor, normalizedConcurrency, normalizedMultiplier, patch); trusted {
		after = response
		accountName = remoteName(response, accountName)
	}
	if verification {
		after, err = client.Account(ctx, accountID)
		if err != nil {
			operation := fieldOperation(operationID, actor, accountID, accountName, fieldName, beforeValues, requested, true, false)
			s.recordFailure(ctx, operation, "remote-readback", err, true)
			return nil, &OperationError{Message: err.Error(), RemoteWritten: true}
		}
		if err := verifyFieldReadback(after, normalizedName, normalizedPriority, normalizedLoadFactor, normalizedConcurrency, normalizedMultiplier, patch); err != nil {
			operation := fieldOperation(operationID, actor, accountID, accountName, fieldName, beforeValues, requested, true, false)
			s.recordFailure(ctx, operation, "remote-readback", err, true)
			return nil, &OperationError{Message: err.Error(), RemoteWritten: true}
		}
		readbackConfirmed = true
	}
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
	go func() {
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
	}()
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

func matchingMutationResponse(
	payload map[string]any,
	accountID string,
	name *string,
	priority *int64,
	loadFactor *string,
	concurrency *int64,
	multiplier *string,
	patch FieldPatch,
) (map[string]any, bool) {
	if raw, present := payload["data"]; present {
		var ok bool
		payload, ok = raw.(map[string]any)
		if !ok {
			return nil, false
		}
	}
	rawID, present := firstPresent(payload, "id", "account_id")
	if !present || strings.TrimSpace(fmt.Sprint(rawID)) != accountID {
		return nil, false
	}
	if err := verifyFieldReadback(payload, name, priority, loadFactor, concurrency, multiplier, patch); err != nil {
		return nil, false
	}
	return payload, true
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
