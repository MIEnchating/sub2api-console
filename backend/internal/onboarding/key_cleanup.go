package onboarding

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

const (
	maximumKeyCleanupItems = 500
)

type UnboundUpstreamKey struct {
	KeyID   string  `json:"key_id"`
	Name    string  `json:"name"`
	GroupID *string `json:"group_id"`
	Status  *string `json:"status"`
}

type KeyCleanupPreview struct {
	Host string               `json:"host"`
	Keys []UnboundUpstreamKey `json:"keys"`
}

type keyCleanupClient interface {
	ListKeys(context.Context, configstore.AuthRecord) ([]business.UpstreamCatalogKey, error)
	DeleteKey(context.Context, configstore.AuthRecord, string) error
}

func (s *Service) PreviewUnboundKeys(ctx context.Context, host string) (KeyCleanupPreview, error) {
	auth, client, err := s.keyCleanupContext(ctx, host)
	if err != nil {
		return KeyCleanupPreview{}, err
	}
	keys, err := client.ListKeys(ctx, auth)
	if err != nil {
		return KeyCleanupPreview{}, fmt.Errorf("上游 Key 列表读取失败：%w", err)
	}
	protected, err := s.protectedKeySet(ctx, auth.Host)
	if err != nil {
		return KeyCleanupPreview{}, fmt.Errorf("本地 Key 绑定关系读取失败：%w", err)
	}
	result := make([]UnboundUpstreamKey, 0, len(keys))
	for _, key := range keys {
		keyID := strings.TrimSpace(key.KeyID)
		if keyID == "" {
			return KeyCleanupPreview{}, errors.New("上游 Key 列表包含缺少稳定 ID 的项目")
		}
		if _, found := protected[keyID]; found {
			continue
		}
		result = append(result, UnboundUpstreamKey{
			KeyID: keyID, Name: strings.TrimSpace(key.Name), GroupID: key.UpstreamGroup, Status: key.Status,
		})
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Name == result[right].Name {
			return result[left].KeyID < result[right].KeyID
		}
		return result[left].Name < result[right].Name
	})
	return KeyCleanupPreview{Host: auth.Host, Keys: result}, nil
}

func (s *Service) EnqueueKeyCleanup(ctx context.Context, host string, keyIDs []string, actor string) (taskstore.Task, error) {
	requested, err := normalizeCleanupKeyIDs(keyIDs)
	if err != nil {
		return taskstore.Task{}, err
	}
	preview, err := s.PreviewUnboundKeys(ctx, host)
	if err != nil {
		return taskstore.Task{}, err
	}
	available := make(map[string]struct{}, len(preview.Keys))
	for _, key := range preview.Keys {
		available[key.KeyID] = struct{}{}
	}
	for _, keyID := range requested {
		if _, found := available[keyID]; !found {
			return taskstore.Task{}, fmt.Errorf("Key %s 已绑定、已不存在或不属于该上游，请重新扫描", keyID)
		}
	}
	task, err := s.newQueuedTask("upstream-key-cleanup", fmt.Sprintf("%d 个无绑定上游 Key 清理已排队", len(requested)))
	if err != nil {
		return taskstore.Task{}, err
	}
	if err := s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	if err := taskrunner.Go(s.taskRunner, func(parent context.Context) {
		s.executeKeyCleanup(parent, task, preview.Host, requested, strings.TrimSpace(actor))
	}); err != nil {
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
	return task, nil
}

func (s *Service) executeKeyCleanup(parent context.Context, task taskstore.Task, host string, requested []string, actor string) {
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()
	task.Status, task.Progress, task.Message = "running", 5, "正在重新复核上游 Key 与本地绑定关系"
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if !taskstore.SaveRunning(ctx, s.tasks, task) {
		return
	}
	guardedCtx, release, err := mutationguard.Acquire(
		ctx, s.repository, mutationguard.Upstream(host), mutationguard.UpstreamKeyCatalog(host),
	)
	if err != nil {
		s.finishKeyCleanupFailure(ctx, task, err)
		return
	}
	defer func() {
		if err := release(); err != nil {
			slog.Error("上游 Key 清理租约释放失败", "host", host, "error", err)
		}
	}()
	ctx = guardedCtx
	preview, err := s.PreviewUnboundKeys(ctx, host)
	if err != nil {
		s.finishKeyCleanupFailure(ctx, task, err)
		return
	}
	available := make(map[string]UnboundUpstreamKey, len(preview.Keys))
	for _, key := range preview.Keys {
		available[key.KeyID] = key
	}
	auth, client, err := s.keyCleanupContext(ctx, host)
	if err != nil {
		s.finishKeyCleanupFailure(ctx, task, err)
		return
	}
	items := make([]map[string]any, 0, len(requested))
	deleted, skipped, failed, auditFailed := 0, 0, 0, 0
	remoteWrite := false
	appendAuditedResult := func(
		key UnboundUpstreamKey,
		status, reason string,
		attempted, succeeded, readbackConfirmed bool,
		cause error,
	) {
		item := cleanupResultItem(key.KeyID, key.Name, status, reason)
		if auditErr := s.recordKeyCleanup(ctx, task.ID, actor, host, key, attempted, succeeded, readbackConfirmed, cause); auditErr != nil {
			auditFailed++
			item["audit_error"] = safeError(auditErr)
			slog.Error("上游 Key 清理审计保存失败", "task_id", task.ID, "host", host, "key_id", key.KeyID, "error", auditErr)
		}
		items = append(items, item)
	}
	for index, keyID := range requested {
		if ctx.Err() != nil {
			break
		}
		task.Progress = 5 + index*90/len(requested)
		task.Message = fmt.Sprintf("正在清理 %d/%d：Key %s", index+1, len(requested), keyID)
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		taskstore.PersistProgress(s.tasks, task)
		key, found := available[keyID]
		if !found {
			skipped++
			items = append(items, cleanupResultItem(keyID, "", "skipped", "Key 已绑定或已不存在，已跳过"))
			continue
		}
		protected, protectErr := s.repository.UpstreamKeyProtected(ctx, host, keyID)
		if protectErr != nil {
			failed++
			appendAuditedResult(key, "failed", protectErr.Error(), false, false, false, protectErr)
			continue
		}
		if protected {
			skipped++
			items = append(items, cleanupResultItem(keyID, key.Name, "skipped", "Key 已建立绑定或进入开户待续，已跳过"))
			continue
		}
		remoteWrite = true
		deleteErr := client.DeleteKey(ctx, auth, keyID)
		keysAfterDelete, readbackErr := client.ListKeys(ctx, auth)
		if readbackErr != nil {
			cause := fmt.Errorf("上游 Key 删除后读回失败，删除结果未知：%w", readbackErr)
			if deleteErr != nil {
				cause = fmt.Errorf("上游 Key 删除请求返回错误：%w；删除后读回失败，删除结果未知：%v", deleteErr, readbackErr)
			}
			failed++
			appendAuditedResult(key, "failed", safeError(cause), true, false, false, cause)
			continue
		}
		if cleanupKeyPresent(keysAfterDelete, keyID) {
			cause := fmt.Errorf("上游 Key %s 删除后仍可读", keyID)
			if deleteErr != nil {
				cause = fmt.Errorf("上游 Key 删除请求返回错误：%w；删除后仍可读", deleteErr)
			}
			failed++
			appendAuditedResult(key, "failed", safeError(cause), true, false, true, cause)
			continue
		}
		if projectionErr := s.repository.ReconcileDeletedUnboundUpstreamKeyProjection(ctx, host, keyID); projectionErr != nil {
			cause := fmt.Errorf("上游 Key 已删除，但本地投影清理失败：%w", projectionErr)
			failed++
			appendAuditedResult(key, "failed", safeError(cause), true, true, true, cause)
			continue
		}
		deleted++
		reason := ""
		if deleteErr != nil {
			reason = "删除响应返回错误，但读回确认 Key 已不存在：" + safeError(deleteErr)
		}
		appendAuditedResult(key, "deleted", reason, true, true, true, deleteErr)
	}
	task.Progress = 100
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	task.Result = map[string]any{
		"operation": "upstream-key-cleanup", "host": host, "total": len(requested),
		"deleted": deleted, "skipped": skipped, "failed": failed, "items": items,
		"remote_write": remoteWrite, "readback_confirmed": remoteWrite && failed == 0,
	}
	if auditFailed > 0 {
		task.Result["audit_failed"] = auditFailed
	}
	if failed > 0 || auditFailed > 0 {
		task.Status = "failed"
		task.Message = fmt.Sprintf("无绑定 Key 清理完成：删除 %d 个，跳过 %d 个，失败 %d 个", deleted, skipped, failed)
		if auditFailed > 0 {
			task.Message += fmt.Sprintf("，审计失败 %d 个", auditFailed)
		}
	} else {
		task.Status = "succeeded"
		task.Message = fmt.Sprintf("无绑定 Key 清理完成：删除 %d 个，跳过 %d 个", deleted, skipped)
	}
	if ctx.Err() != nil {
		task.Status = "failed"
	}
	if cause := taskstore.ContextFailureCause(ctx); cause != nil {
		task.Message = "无绑定 Key 清理失败：" + safeError(cause)
		task.Result["error"] = safeError(cause)
	}
	taskstore.MarkCancelled(ctx, &task, "无绑定 Key 清理已取消")
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) finishKeyCleanupFailure(ctx context.Context, task taskstore.Task, cause error) {
	if contextCause := taskstore.ContextFailureCause(ctx); contextCause != nil {
		cause = contextCause
	}
	task.Status, task.Progress = "failed", 100
	task.Message = "无绑定 Key 清理失败：" + safeError(cause)
	task.Result = map[string]any{"operation": "upstream-key-cleanup", "error": safeError(cause)}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	taskstore.MarkCancelled(ctx, &task, "无绑定 Key 清理已取消")
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) keyCleanupContext(ctx context.Context, host string) (configstore.AuthRecord, keyCleanupClient, error) {
	if strings.TrimSpace(host) == "" {
		return configstore.AuthRecord{}, nil, errors.New("上游 Host 不能为空")
	}
	auth, err := s.private.AuthRecord(ctx, host)
	if err != nil {
		return configstore.AuthRecord{}, nil, err
	}
	if auth == nil {
		return configstore.AuthRecord{}, nil, errors.New("清理 Key 前必须先配置该 Host 的鉴权记录")
	}
	client, ok := s.keys.(keyCleanupClient)
	if !ok {
		return configstore.AuthRecord{}, nil, errors.New("当前上游客户端不支持读取和删除 Key")
	}
	return *auth, client, nil
}

func (s *Service) protectedKeySet(ctx context.Context, host string) (map[string]struct{}, error) {
	ids, err := s.repository.ProtectedUpstreamKeyIDs(ctx, host)
	if err != nil {
		return nil, err
	}
	result := make(map[string]struct{}, len(ids))
	for _, keyID := range ids {
		if keyID = strings.TrimSpace(keyID); keyID != "" {
			result[keyID] = struct{}{}
		}
	}
	return result, nil
}

func normalizeCleanupKeyIDs(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, errors.New("请至少选择一个要清理的上游 Key")
	}
	if len(values) > maximumKeyCleanupItems {
		return nil, fmt.Errorf("单次最多清理 %d 个上游 Key", maximumKeyCleanupItems)
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 255 {
			return nil, errors.New("上游 Key ID 无效")
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil, errors.New("请至少选择一个要清理的上游 Key")
	}
	return result, nil
}

func cleanupResultItem(keyID, name, status, reason string) map[string]any {
	item := map[string]any{"key_id": keyID, "name": name, "status": status}
	if reason != "" {
		item["reason"] = reason
	}
	return item
}

func cleanupKeyPresent(keys []business.UpstreamCatalogKey, keyID string) bool {
	keyID = strings.TrimSpace(keyID)
	for _, key := range keys {
		if strings.TrimSpace(key.KeyID) == keyID {
			return true
		}
	}
	return false
}

func (s *Service) recordKeyCleanup(
	ctx context.Context,
	taskID, actor, host string,
	key UnboundUpstreamKey,
	attempted, succeeded, readbackConfirmed bool,
	cause error,
) error {
	state := "succeeded"
	summary := fmt.Sprintf("上游 Key 清理成功：%s / %s（%s）", host, key.Name, key.KeyID)
	payload := map[string]any{
		"task_id": taskID, "actor": actor, "host": host, "key_id": key.KeyID,
		"key_name": key.Name, "group_id": key.GroupID, "remote_write": attempted,
		"remote_confirmed": succeeded, "readback_confirmed": readbackConfirmed,
	}
	if !succeeded {
		state = "failed"
		reason := safeError(cause)
		summary = fmt.Sprintf("上游 Key 清理失败：%s / %s（%s）", host, key.Name, key.KeyID)
		payload["error"] = reason
	} else if cause != nil {
		payload["delete_response_error"] = safeError(cause)
	}
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), onboardingAuditTimeout)
	defer cancel()
	_, err := s.repository.RecordRuntimeEvent(auditCtx, "upstream.key.cleanup", state, summary, payload)
	return err
}
