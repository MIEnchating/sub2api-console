package accountdelete

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type Repository interface {
	Mode(context.Context) (string, error)
	Account(context.Context, string) (*business.AccountDetail, error)
	AccountMutationProtection(context.Context, string) (business.AccountMutationProtection, error)
	ConfirmAccountDeleteScope(context.Context, string, business.AccountDeleteScope) error
	ReconcileDeletedUpstreamKeyProjection(context.Context, string, business.AccountDeleteScope) error
	DeleteAccountProjection(context.Context, string, business.AccountOperation) error
	DeleteAccountProjectionWithScope(context.Context, string, business.AccountDeleteScope, business.AccountOperation) error
	RecordAccountOperation(context.Context, business.AccountOperation) error
}

type PrivateStore interface {
	TargetSettings(context.Context) (configstore.TargetSettings, error)
	AuthRecord(context.Context, string) (*configstore.AuthRecord, error)
	DeleteUpstreamKeySecrets(context.Context, string, string) error
}

type TaskStore interface {
	Save(context.Context, taskstore.Task) error
}

type KeyClient interface {
	ListKeys(context.Context, configstore.AuthRecord) ([]business.UpstreamCatalogKey, error)
	DeleteKey(context.Context, configstore.AuthRecord, string) error
}

type AuthResolver interface {
	ResolveAuth(context.Context, string, string) (*configstore.AuthRecord, error)
}

type Admin interface {
	DeleteAccountWithVerification(context.Context, string, bool) (map[string]any, error)
}

type Binding struct {
	ID              int64  `json:"id"`
	UpstreamID      string `json:"upstream_id"`
	UpstreamHost    string `json:"upstream_host"`
	AuthHost        string `json:"auth_host"`
	UpstreamKeyID   string `json:"upstream_key_id"`
	UpstreamKeyName string `json:"upstream_key_name"`
}

type Preview struct {
	AccountID                   string   `json:"account_id"`
	AccountName                 string   `json:"account_name"`
	Groups                      []string `json:"groups"`
	ManagementBaseURL           string   `json:"management_base_url"`
	Binding                     *Binding `json:"binding"`
	managementTargetFingerprint string
}

type Result struct {
	AccountID                         string `json:"account_id"`
	AccountName                       string `json:"account_name"`
	UpstreamHost                      string `json:"upstream_host"`
	UpstreamKeyID                     string `json:"upstream_key_id"`
	UpstreamKeyDeleteRequested        bool   `json:"upstream_key_delete_requested"`
	UpstreamKeyDeleted                bool   `json:"upstream_key_deleted"`
	UpstreamKeyAlreadyAbsent          bool   `json:"upstream_key_already_absent"`
	UpstreamKeyReadbackFailed         bool   `json:"upstream_key_readback_failed"`
	UpstreamKeyStillReadable          bool   `json:"upstream_key_still_readable"`
	UpstreamKeyProjectionDeleted      bool   `json:"upstream_key_projection_deleted"`
	UpstreamKeySecretDeleted          bool   `json:"upstream_key_secret_deleted"`
	ManagementAccountDeleteRequested  bool   `json:"management_account_delete_requested"`
	ManagementDeleteResponseConfirmed bool   `json:"management_delete_response_confirmed"`
	ManagementAccountStillReadable    bool   `json:"management_account_still_readable"`
	ManagementAccountReadbackFailed   bool   `json:"management_account_readback_failed"`
	ManagementAccountDeleted          bool   `json:"management_account_deleted"`
	LocalProjectionDeleted            bool   `json:"local_projection_deleted"`
	ReadbackConfirmed                 bool   `json:"readback_confirmed"`
}

type Service struct {
	repository   Repository
	private      PrivateStore
	keys         KeyClient
	tasks        TaskStore
	resolver     AuthResolver
	taskRunner   taskrunner.Runner
	timeout      time.Duration
	adminFactory func(configstore.TargetSettings) (Admin, error)
}

const failureAuditTimeout = 5 * time.Second

func New(repository Repository, private PrivateStore, keys KeyClient, tasks TaskStore) *Service {
	service := &Service{repository: repository, private: private, keys: keys, tasks: tasks, timeout: 10 * time.Minute}
	service.adminFactory = service.newAdmin
	return service
}

func (s *Service) UseTaskRunner(runner taskrunner.Runner) { s.taskRunner = runner }

func (s *Service) SetAuthResolver(resolver AuthResolver) { s.resolver = resolver }

func (s *Service) Preview(ctx context.Context, accountID string) (Preview, error) {
	if !stableID(accountID) {
		return Preview{}, errors.New("账号必须使用有效的稳定 ID")
	}
	if err := s.rejectProtected(ctx, accountID); err != nil {
		return Preview{}, err
	}
	account, err := s.repository.Account(ctx, strings.TrimSpace(accountID))
	if err != nil {
		return Preview{}, err
	}
	if len(account.Bindings) > 1 {
		return Preview{}, fmt.Errorf("账号存在 %d 个上游 Key 绑定，无法执行单账号精确删除", len(account.Bindings))
	}
	var binding *Binding
	if len(account.Bindings) == 1 {
		value, bindingErr := previewBinding(account.Bindings[0])
		if bindingErr != nil {
			return Preview{}, bindingErr
		}
		binding = &value
	}
	target, err := s.private.TargetSettings(ctx)
	if err != nil {
		return Preview{}, fmt.Errorf("管理目标读取失败：%w", err)
	}
	managementBaseURL, err := configstore.ValidateBaseURL(target.BaseURL)
	if err != nil {
		return Preview{}, fmt.Errorf("管理目标地址无效：%w", err)
	}
	return Preview{
		AccountID: account.ID, AccountName: account.Name, ManagementBaseURL: managementBaseURL,
		Groups: append([]string{}, account.Groups...), Binding: binding,
		managementTargetFingerprint: managementTargetFingerprint(managementBaseURL, target.AdminKey),
	}, nil
}

func (s *Service) Enqueue(
	ctx context.Context,
	accountID string,
	expected *Binding,
	expectedManagementBaseURL string,
	actor string,
) (taskstore.Task, error) {
	mode, err := s.repository.Mode(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	if mode != runtimepolicy.Full {
		return taskstore.Task{}, errors.New("账号删除只能在完全模式执行")
	}
	preview, err := s.Preview(ctx, accountID)
	if err != nil {
		return taskstore.Task{}, err
	}
	if expected != nil {
		normalized := *expected
		normalized.UpstreamHost = strings.TrimSpace(normalized.UpstreamHost)
		normalized.AuthHost = strings.TrimSpace(normalized.AuthHost)
		normalized.UpstreamID = strings.TrimSpace(normalized.UpstreamID)
		normalized.UpstreamKeyID = strings.TrimSpace(normalized.UpstreamKeyID)
		expected = &normalized
	}
	expectedManagementBaseURL, err = configstore.ValidateBaseURL(expectedManagementBaseURL)
	if err != nil {
		return taskstore.Task{}, errors.New("删除预览中的管理目标地址无效")
	}
	if preview.ManagementBaseURL != expectedManagementBaseURL {
		return taskstore.Task{}, errors.New("删除预览后的管理目标已变化，请重新确认")
	}
	if !sameBinding(preview.Binding, expected) {
		return taskstore.Task{}, errors.New("删除预览后的账号绑定已变化，请重新确认")
	}
	if err := s.rejectProtected(ctx, accountID); err != nil {
		return taskstore.Task{}, err
	}
	id, err := randomID()
	if err != nil {
		return taskstore.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{
		ID: id, Skill: "sub2api-account-management", Operation: "account-delete", Status: "queued", Progress: 0,
		Message: accountDeleteQueuedMessage(preview.Binding != nil), Result: map[string]any{
			"account_id": preview.AccountID, "management_base_url": preview.ManagementBaseURL,
		}, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	if err := taskrunner.Go(s.taskRunner, func(parent context.Context) {
		s.execute(parent, task, preview, actor)
	}); err != nil {
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
	return task, nil
}

func (s *Service) execute(parent context.Context, task taskstore.Task, expected Preview, actor string) {
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()
	hasBinding := expected.Binding != nil
	task.Status, task.Progress, task.Message, task.UpdatedAt = "running", 10, accountDeleteRunningMessage(hasBinding), time.Now().UTC().Format(time.RFC3339Nano)
	if !taskstore.SaveRunning(ctx, s.tasks, task) {
		return
	}
	result, err := s.Delete(ctx, expected, actor)
	task.Progress, task.UpdatedAt = 100, time.Now().UTC().Format(time.RFC3339Nano)
	task.Result = resultMap(result)
	if err != nil {
		task.Status, task.Message = "failed", accountDeleteFailurePrefix(hasBinding)+err.Error()
		task.Result["error"] = err.Error()
	} else {
		task.Status, task.Message = "succeeded", accountDeleteSuccessMessage(hasBinding)
	}
	cancelledMessage := "管理平台账号删除已取消"
	if hasBinding {
		cancelledMessage = "账号双端删除已取消"
	}
	taskstore.MarkCancelled(ctx, &task, cancelledMessage)
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) Delete(ctx context.Context, expected Preview, actor string) (Result, error) {
	result := Result{AccountID: expected.AccountID, AccountName: expected.AccountName}
	resources := []string{
		mutationguard.ManagementTarget(), mutationguard.AccountCatalog(), mutationguard.Account(expected.AccountID),
	}
	if expected.Binding != nil {
		result.UpstreamHost = expected.Binding.UpstreamHost
		result.UpstreamKeyID = expected.Binding.UpstreamKeyID
		resources = append(resources,
			mutationguard.Upstream(expected.Binding.AuthHost), mutationguard.Upstream(expected.Binding.UpstreamHost))
	}
	guarded, release, err := mutationguard.Acquire(ctx, s.repository, resources...)
	if err != nil {
		return result, err
	}
	defer func() {
		if releaseErr := release(); releaseErr != nil {
			slog.Error("账号删除租约释放失败", "account_id", expected.AccountID, "error", releaseErr)
		}
	}()
	ctx = guarded
	current, err := s.Preview(ctx, expected.AccountID)
	if err != nil {
		return result, err
	}
	if expected.ManagementBaseURL != current.ManagementBaseURL {
		return result, errors.New("获取删除锁后管理目标已变化，请重新确认")
	}
	if expected.managementTargetFingerprint == "" ||
		expected.managementTargetFingerprint != current.managementTargetFingerprint {
		return result, errors.New("获取删除锁后管理目标凭据已变化，请重新确认")
	}
	if !samePreview(expected, current) {
		return result, errors.New("获取删除锁后账号或上游 Key 绑定已变化，请重新确认")
	}
	if err := s.rejectProtected(ctx, expected.AccountID); err != nil {
		return result, err
	}
	target, err := s.confirmManagementTarget(ctx, current.ManagementBaseURL, current.managementTargetFingerprint)
	if err != nil {
		return result, err
	}
	admin, err := s.adminFactory(target)
	if err != nil {
		return result, fmt.Errorf("管理平台客户端创建失败：%w", err)
	}
	opID, err := randomID()
	if err != nil {
		return result, fmt.Errorf("账号删除审计 ID 创建失败：%w", err)
	}
	var scope *business.AccountDeleteScope
	if current.Binding != nil {
		value := business.AccountDeleteScope{
			BindingID: current.Binding.ID, UpstreamID: current.Binding.UpstreamID,
			UpstreamKeyID: current.Binding.UpstreamKeyID,
		}
		scope = &value
		if err := s.repository.ConfirmAccountDeleteScope(ctx, current.AccountID, value); err != nil {
			return result, fmt.Errorf("上游 Key 独占绑定复核失败：%w", err)
		}
		auth, authErr := s.resolveAuth(ctx, current.Binding.AuthHost, actor)
		if authErr != nil {
			return result, fmt.Errorf("上游鉴权读取失败：%w", authErr)
		}
		keys, listErr := s.keys.ListKeys(ctx, *auth)
		if listErr != nil {
			return result, fmt.Errorf("上游 Key 删除前读回失败：%w", listErr)
		}
		if containsKey(keys, current.Binding.UpstreamKeyID) {
			result.UpstreamKeyDeleteRequested = true
			deleteErr := s.keys.DeleteKey(ctx, *auth, current.Binding.UpstreamKeyID)
			keys, listErr = s.keys.ListKeys(ctx, *auth)
			if listErr != nil {
				result.UpstreamKeyReadbackFailed = true
				cause := fmt.Errorf("上游 Key 删除请求已发送，但删除后读回失败：%w", listErr)
				if deleteErr != nil {
					cause = fmt.Errorf(
						"上游 Key %s 删除请求返回错误：%w；删除后读回失败，删除结果未知：%w",
						current.Binding.UpstreamKeyID,
						deleteErr,
						listErr,
					)
				}
				return result, s.recordFailureAudit(ctx, opID, current, actor, result, cause)
			}
			if containsKey(keys, current.Binding.UpstreamKeyID) {
				result.UpstreamKeyStillReadable = true
				cause := fmt.Errorf("上游 Key %s 删除后仍可读，已停止删除管理平台账号", current.Binding.UpstreamKeyID)
				if deleteErr != nil {
					cause = fmt.Errorf(
						"上游 Key %s 删除请求返回错误：%w；删除后仍可读，已停止删除管理平台账号",
						current.Binding.UpstreamKeyID,
						deleteErr,
					)
				}
				return result, s.recordFailureAudit(ctx, opID, current, actor, result, cause)
			}
			result.UpstreamKeyDeleted = true
		} else {
			result.UpstreamKeyDeleted = true
			result.UpstreamKeyAlreadyAbsent = true
		}
		if err := s.repository.ReconcileDeletedUpstreamKeyProjection(ctx, current.AccountID, value); err != nil {
			cause := fmt.Errorf("上游 Key 已确认不存在，但 Key 本地投影对账失败：%w", err)
			return result, s.recordFailureAudit(ctx, opID, current, actor, result, cause)
		}
		result.UpstreamKeyProjectionDeleted = true
		if err := s.private.DeleteUpstreamKeySecrets(
			ctx,
			current.Binding.AuthHost,
			current.Binding.UpstreamKeyID,
		); err != nil {
			cause := fmt.Errorf("上游 Key 已确认不存在且业务投影已清理，但 Key 私有密钥清理失败：%w", err)
			return result, s.recordFailureAudit(ctx, opID, current, actor, result, cause)
		}
		result.UpstreamKeySecretDeleted = true
		if _, err := s.confirmManagementTarget(ctx, current.ManagementBaseURL, current.managementTargetFingerprint); err != nil {
			cause := fmt.Errorf("上游 Key 已删除且本地投影和私有密钥已清理，但%s", err)
			return result, s.recordFailureAudit(ctx, opID, current, actor, result, cause)
		}
	}
	result.ManagementAccountDeleteRequested = true
	managementResult, err := admin.DeleteAccountWithVerification(ctx, current.AccountID, true)
	if err != nil {
		cause := managementDeleteFailure(current.Binding != nil, current.AccountID, err, &result)
		return result, s.recordFailureAudit(ctx, opID, current, actor, result, cause)
	}
	result.ManagementDeleteResponseConfirmed = boolValue(managementResult, "delete_response_confirmed", true)
	result.ManagementAccountDeleted = true
	result.ReadbackConfirmed = true
	committed := result
	committed.LocalProjectionDeleted = true
	op := accountDeleteOperation(opID, current, actor, committed, nil)
	if scope == nil {
		err = s.repository.DeleteAccountProjection(ctx, current.AccountID, op)
	} else {
		err = s.repository.DeleteAccountProjectionWithScope(ctx, current.AccountID, *scope, op)
	}
	if err != nil {
		message := "管理平台账号已删除，但本地记录清理失败："
		if scope != nil {
			message = "上游 Key 和管理平台账号已删除，但本地记录清理失败："
		}
		cause := errors.New(message + err.Error())
		return result, s.recordFailureAudit(ctx, opID, current, actor, result, cause)
	}
	result.LocalProjectionDeleted = true
	return result, nil
}

func (s *Service) rejectProtected(ctx context.Context, accountID string) error {
	protection, err := s.repository.AccountMutationProtection(ctx, accountID)
	if err != nil {
		return err
	}
	if protection.Protected() {
		return fmt.Errorf("账号处于%s，删除前请先解除人工管控", strings.Join(protection.Reasons(), "、"))
	}
	return nil
}

func (s *Service) resolveAuth(ctx context.Context, host, actor string) (*configstore.AuthRecord, error) {
	if s.resolver != nil {
		record, err := s.resolver.ResolveAuth(ctx, host, actor)
		if err != nil || record != nil {
			return record, err
		}
	}
	record, err := s.private.AuthRecord(ctx, host)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, fmt.Errorf("上游 %s 没有可用鉴权记录", host)
	}
	return record, nil
}

func (s *Service) confirmManagementTarget(
	ctx context.Context,
	expectedBaseURL string,
	expectedFingerprint string,
) (configstore.TargetSettings, error) {
	expectedBaseURL, err := configstore.ValidateBaseURL(expectedBaseURL)
	if err != nil {
		return configstore.TargetSettings{}, errors.New("删除预览中的管理目标地址无效")
	}
	target, err := s.private.TargetSettings(ctx)
	if err != nil {
		return configstore.TargetSettings{}, fmt.Errorf("管理目标读取失败：%w", err)
	}
	target.BaseURL, err = configstore.ValidateBaseURL(target.BaseURL)
	if err != nil {
		return configstore.TargetSettings{}, fmt.Errorf("管理目标地址无效：%w", err)
	}
	if target.BaseURL != expectedBaseURL {
		return configstore.TargetSettings{}, errors.New("管理目标已变化，已停止删除管理平台账号")
	}
	if expectedFingerprint == "" || managementTargetFingerprint(target.BaseURL, target.AdminKey) != expectedFingerprint {
		return configstore.TargetSettings{}, errors.New("管理目标凭据已变化，已停止删除管理平台账号")
	}
	return target, nil
}

func managementTargetFingerprint(baseURL, adminKey string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(baseURL) + "\x00" + strings.TrimSpace(adminKey)))
	return hex.EncodeToString(digest[:])
}

func (s *Service) newAdmin(target configstore.TargetSettings) (Admin, error) {
	return adminclient.New(adminclient.Config{
		BaseURL: target.BaseURL, AdminKey: target.AdminKey,
		Timeout: time.Duration(target.TimeoutSeconds) * time.Second, Attempts: 3,
	}, nil)
}

func previewBinding(binding business.AccountBinding) (Binding, error) {
	authHost := binding.UpstreamHost
	if binding.SourceAuthHost != nil && strings.TrimSpace(*binding.SourceAuthHost) != "" {
		authHost = *binding.SourceAuthHost
	}
	result := Binding{
		ID: binding.ID, UpstreamID: strings.TrimSpace(binding.UpstreamID),
		UpstreamHost: strings.TrimSpace(binding.UpstreamHost), AuthHost: strings.TrimSpace(authHost),
		UpstreamKeyID: strings.TrimSpace(binding.UpstreamKeyID), UpstreamKeyName: strings.TrimSpace(binding.UpstreamKeyName),
	}
	if result.ID <= 0 || result.UpstreamID == "" || result.UpstreamHost == "" || result.AuthHost == "" || result.UpstreamKeyID == "" {
		return Binding{}, errors.New("账号上游绑定缺少稳定 Binding ID、上游身份 ID、上游地址或 Key ID")
	}
	return result, nil
}

func (s *Service) recordFailureAudit(
	ctx context.Context,
	operationID string,
	preview Preview,
	actor string,
	result Result,
	cause error,
) error {
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), failureAuditTimeout)
	defer cancel()
	if err := s.repository.RecordAccountOperation(
		auditCtx, accountDeleteOperation(operationID, preview, actor, result, cause),
	); err != nil {
		return fmt.Errorf("%w；账号删除失败审计保存失败：%v", cause, err)
	}
	return cause
}

func managementDeleteFailure(hasBinding bool, accountID string, cause error, result *Result) error {
	var stillReadable *adminclient.AccountStillReadableError
	if errors.As(cause, &stillReadable) {
		result.ManagementDeleteResponseConfirmed = stillReadable.DeleteErr == nil
		result.ManagementAccountStillReadable = true
		cause = fmt.Errorf("管理平台账号 %s 删除后仍可读：%w", accountID, cause)
	} else {
		var readbackUnknown *adminclient.AccountReadbackUnknownError
		if errors.As(cause, &readbackUnknown) {
			result.ManagementDeleteResponseConfirmed = readbackUnknown.DeleteErr == nil
		}
		result.ManagementAccountReadbackFailed = true
		cause = fmt.Errorf("管理平台账号删除结果未知：%w", cause)
	}
	if hasBinding {
		return fmt.Errorf("上游 Key 已删除且本地投影和私有密钥已清理，但%w", cause)
	}
	return cause
}

func accountDeleteOperation(
	operationID string,
	preview Preview,
	actor string,
	result Result,
	cause error,
) business.AccountOperation {
	state, phase := "succeeded", "readback"
	var detail *string
	if cause != nil {
		state = "failed"
		switch {
		case result.ManagementAccountDeleted:
			phase = "local-commit"
		case result.ManagementAccountStillReadable:
			phase = "management-readback-still-readable"
		case result.ManagementAccountReadbackFailed || result.ManagementAccountDeleteRequested:
			phase = "management-readback"
		case result.UpstreamKeyDeleted && !result.UpstreamKeyProjectionDeleted:
			phase = "upstream-key-reconcile"
		case result.UpstreamKeyDeleted && !result.UpstreamKeySecretDeleted:
			phase = "upstream-key-secret-reconcile"
		case result.UpstreamKeyDeleted:
			phase = "management-target-check"
		case result.UpstreamKeyDeleteRequested:
			phase = "upstream-key-readback"
		default:
			phase = "remote-delete"
		}
		message := cause.Error()
		detail = &message
	}
	field := "deleted"
	name := preview.AccountName
	remoteConfirmed := result.ManagementAccountDeleted &&
		(preview.Binding == nil || result.UpstreamKeyDeleted)
	readbackConfirmed := remoteConfirmed && result.ReadbackConfirmed
	return business.AccountOperation{
		OperationID: operationID, OperationType: "account.delete", State: state, Phase: phase,
		Actor: actor, Error: detail,
		RemoteConfirmed:   remoteConfirmed,
		ReadbackConfirmed: readbackConfirmed,
		ObjectID:          preview.AccountID,
		ObjectName:        &name,
		GroupNames:        append([]string{}, preview.Groups...),
		FieldName:         &field,
		Before: map[string]any{
			"management_base_url": preview.ManagementBaseURL,
			"binding":             preview.Binding,
		},
		After: map[string]any{
			"upstream_key_delete_requested":        result.UpstreamKeyDeleteRequested,
			"upstream_key_deleted":                 result.UpstreamKeyDeleted,
			"upstream_key_already_absent":          result.UpstreamKeyAlreadyAbsent,
			"upstream_key_readback_failed":         result.UpstreamKeyReadbackFailed,
			"upstream_key_still_readable":          result.UpstreamKeyStillReadable,
			"upstream_key_projection_deleted":      result.UpstreamKeyProjectionDeleted,
			"upstream_key_secret_deleted":          result.UpstreamKeySecretDeleted,
			"management_account_delete_requested":  result.ManagementAccountDeleteRequested,
			"management_delete_response_confirmed": result.ManagementDeleteResponseConfirmed,
			"management_account_still_readable":    result.ManagementAccountStillReadable,
			"management_account_readback_failed":   result.ManagementAccountReadbackFailed,
			"management_account_deleted":           result.ManagementAccountDeleted,
			"local_projection_deleted":             result.LocalProjectionDeleted,
		},
		Writeback: result.UpstreamKeyDeleteRequested || result.ManagementAccountDeleteRequested,
	}
}

func samePreview(left, right Preview) bool {
	return left.AccountID == right.AccountID && left.AccountName == right.AccountName &&
		left.ManagementBaseURL == right.ManagementBaseURL &&
		left.managementTargetFingerprint == right.managementTargetFingerprint &&
		sameBinding(left.Binding, right.Binding)
}

func sameBinding(left, right *Binding) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.ID == right.ID && left.UpstreamID == right.UpstreamID &&
		left.UpstreamHost == right.UpstreamHost && left.AuthHost == right.AuthHost &&
		left.UpstreamKeyID == right.UpstreamKeyID
}

func accountDeleteQueuedMessage(hasBinding bool) string {
	if hasBinding {
		return "账号双端删除已排队"
	}
	return "管理平台账号删除已排队"
}

func accountDeleteRunningMessage(hasBinding bool) string {
	if hasBinding {
		return "正在删除上游 Key 和管理平台账号"
	}
	return "正在确认管理平台账号并清理本地记录"
}

func accountDeleteFailurePrefix(hasBinding bool) string {
	if hasBinding {
		return "账号双端删除失败："
	}
	return "管理平台账号删除失败："
}

func accountDeleteSuccessMessage(hasBinding bool) string {
	if hasBinding {
		return "上游 Key、管理平台账号和本地记录已删除"
	}
	return "管理平台账号和本地记录已删除；该账号没有可确认的上游 Key 绑定"
}

func containsKey(keys []business.UpstreamCatalogKey, keyID string) bool {
	for _, key := range keys {
		if strings.TrimSpace(key.KeyID) == keyID {
			return true
		}
	}
	return false
}

func boolValue(values map[string]any, key string, fallback bool) bool {
	value, ok := values[key].(bool)
	if !ok {
		return fallback
	}
	return value
}

func stableID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || value == "0" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func randomID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func resultMap(result Result) map[string]any {
	return map[string]any{
		"account_id": result.AccountID, "account_name": result.AccountName,
		"upstream_host": result.UpstreamHost, "upstream_key_id": result.UpstreamKeyID,
		"upstream_key_delete_requested":        result.UpstreamKeyDeleteRequested,
		"upstream_key_deleted":                 result.UpstreamKeyDeleted,
		"upstream_key_already_absent":          result.UpstreamKeyAlreadyAbsent,
		"upstream_key_readback_failed":         result.UpstreamKeyReadbackFailed,
		"upstream_key_still_readable":          result.UpstreamKeyStillReadable,
		"upstream_key_projection_deleted":      result.UpstreamKeyProjectionDeleted,
		"upstream_key_secret_deleted":          result.UpstreamKeySecretDeleted,
		"management_account_delete_requested":  result.ManagementAccountDeleteRequested,
		"management_delete_response_confirmed": result.ManagementDeleteResponseConfirmed,
		"management_account_still_readable":    result.ManagementAccountStillReadable,
		"management_account_readback_failed":   result.ManagementAccountReadbackFailed,
		"management_account_deleted":           result.ManagementAccountDeleted,
		"local_projection_deleted":             result.LocalProjectionDeleted,
		"readback_confirmed":                   result.ReadbackConfirmed,
	}
}
