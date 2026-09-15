package accountworkbench

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type maintenanceExecutionStore interface {
	executionStore
	WorkbenchMaintenanceExecutions(context.Context, string) ([]configstore.WorkbenchExecution, error)
}

type MaintenancePendingUploadView struct {
	ID           string `json:"id"`
	AccountID    string `json:"account_id"`
	Email        string `json:"email"`
	Status       string `json:"status"`
	Message      string `json:"message"`
	NextRetryAt  string `json:"next_retry_at,omitempty"`
	ExpiresAt    string `json:"expires_at"`
	SourceTaskID string `json:"source_task_id"`
}

func (s *Service) initializeMaintenanceExecution(ctx context.Context, prepared *preparedImport, record *configstore.WorkbenchExecution) error {
	if len(prepared.profiles) == 0 || prepared.profiles[0] == nil || prepared.profiles[0].maintenanceRevision == 0 {
		return nil
	}
	if prepared.retry == nil && prepared.profiles[0].uploadMaintenance != nil {
		config := *prepared.profiles[0].uploadMaintenance
		record.Maintenance = &config
	} else {
		store, ok := s.private.(maintenanceStore)
		if !ok {
			return errors.New("待上传维护配置存储未就绪")
		}
		config, err := store.WorkbenchMaintenance(ctx, prepared.target.BaseURL)
		if err != nil || !config.Enabled || !config.ReauthorizeWithProfiles || config.Revision != prepared.profiles[0].maintenanceRevision {
			return errors.New("自动维护配置已变化，未开始上传")
		}
		record.Maintenance = &configstore.WorkbenchExecutionMaintenance{Revision: config.Revision, Model: config.Model, CheckAfterImport: config.CheckAfterRepair, CooldownMinutes: config.CooldownMinutes}
	}
	record.Maintenance.SourceBatchID = prepared.maintenanceBatchID
	if prepared.retry != nil && prepared.retry.source.Maintenance != nil {
		record.Maintenance.SourceBatchID = prepared.retry.source.Maintenance.SourceBatchID
	}
	for i := range record.Items {
		item := &record.Items[i]
		if prepared.profiles[i] == nil || item.OriginalAccountID == "" || item.LoginProfileID == "" || item.LoginMaintenanceRevision != record.Maintenance.Revision {
			return errors.New("待上传资料缺少原账号及维护绑定")
		}
		item.AccountID = item.OriginalAccountID
		item.UploadStatus, item.UploadMessage = "pending", "授权结果已私有保存，等待上传"
		if prepared.retry == nil {
			if len(prepared.profiles[i].uploadSnapshot) == 0 {
				return errors.New("待上传账号缺少授权前配置快照")
			}
			item.Snapshot = prepared.profiles[i].uploadSnapshot
		}
	}
	return nil
}

// A completed OAuth exchange is saved before observing parent cancellation or
// starting another account. Later upload still requires a live delegation.
func (s *Service) persistMaintenanceUploadResult(ctx context.Context, batchID, childID string, target configstore.TargetSettings, profile *profileAuthorization, item InputItem) error {
	if profile == nil || profile.maintenanceRevision == 0 {
		return nil
	}
	persist, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	store, ok := s.private.(executionStore)
	if !ok {
		return errors.New("待上传私有执行存储未就绪")
	}
	if saved, err := store.WorkbenchExecution(persist, childID); err == nil {
		return validateMaintenanceUploadSeed(saved, batchID, target, profile, item)
	} else if !errors.Is(err, configstore.ErrWorkbenchExecution) {
		return err
	}
	prepared := &preparedImport{owner: profile.maintenanceOwner, target: target, maintenanceBatchID: batchID, view: Preview{ID: childID, Items: []PreviewItem{{Index: item.Index, AccountID: profile.identity.accountID}}}, items: []InputItem{item}, profiles: []*profileAuthorization{profile}, templates: []*configstore.WorkbenchTemplate{nil}}
	err := s.initializeExecution(persist, prepared, childID)
	if err != nil {
		if saved, readErr := store.WorkbenchExecution(persist, childID); readErr == nil && validateMaintenanceUploadSeed(saved, batchID, target, profile, item) == nil {
			return nil
		}
	}
	return err
}

func validateMaintenanceUploadSeed(record configstore.WorkbenchExecution, batchID string, target configstore.TargetSettings, profile *profileAuthorization, item InputItem) error {
	if record.TargetURL != target.BaseURL || record.TargetFingerprint != executionTargetFingerprint(target) || record.Maintenance == nil || record.Maintenance.SourceBatchID != batchID || record.Maintenance.Revision != profile.maintenanceRevision || len(record.Items) != 1 {
		return configstore.ErrWorkbenchExecution
	}
	saved := record.Items[0]
	raw, err := json.Marshal(item.Credentials)
	if err != nil || saved.Index != item.Index || saved.OriginalAccountID != profile.identity.accountID || saved.LoginProfileID != profile.id || saved.LoginProfileRevision != profile.revision || saved.LoginUserID != profile.identity.identity.UserID || saved.LoginWorkspaceID != profile.identity.workspaceID || saved.LoginMaintenanceOwner != profile.maintenanceOwner || saved.Identity != IdentityKey(item) || !bytes.Equal(saved.Credentials, raw) || !bytes.Equal(saved.Snapshot, profile.uploadSnapshot) {
		return configstore.ErrWorkbenchExecution
	}
	return nil
}

func (s *Service) maintenanceBatchUploads(ctx context.Context, job *oauthBatch) ([]configstore.WorkbenchExecution, error) {
	values, err := s.maintenanceExecutions(ctx, job.target)
	if err != nil {
		return nil, err
	}
	result := []configstore.WorkbenchExecution{}
	for _, value := range values {
		if value.Maintenance.SourceBatchID != job.view.ID {
			continue
		}
		for _, item := range value.Items {
			if pendingUploadItem(item) {
				result = append(result, value)
				break
			}
		}
	}
	return result, nil
}

func (s *Service) requireFinishedUploadSource(ctx context.Context, id string) error {
	getter, ok := s.tasks.(interface {
		Get(context.Context, string) (taskstore.Task, error)
	})
	if !ok {
		return errors.New("上传来源任务不可读取")
	}
	task, err := getter.Get(ctx, id)
	if err != nil || task.Skill != Skill || (task.Operation != "account-workbench-import" && task.Operation != "account-workbench-retry" && task.Operation != "account-workbench-oauth") || task.Status == "running" || task.Status == "queued" || task.Status == "waiting_input" {
		return errors.New("上传来源任务尚未结束或不可读取")
	}
	return nil
}

func updateMaintenanceUploadItem(record *configstore.WorkbenchExecution, index int, row ResultItem) {
	if record.Maintenance == nil {
		return
	}
	item := &record.Items[index]
	if row.Status == "succeeded" || len(item.Credentials) == 0 {
		item.UploadStatus, item.UploadMessage, item.UploadNextRetryAt = "done", "新凭据已上传并读回确认", ""
		return
	}
	item.UploadStatus, item.UploadMessage = "pending", "新授权已保存，冷却后核对并继续原上传"
	if item.Phase == "credentials_submitted" && item.CredentialWriteOutcome != "rejected" {
		item.UploadMessage = "上传结果尚未确认，下一次维护先读取线上状态核对"
	}
	item.UploadNextRetryAt = time.Now().Add(time.Duration(record.Maintenance.CooldownMinutes) * time.Minute).UTC().Format(time.RFC3339Nano)
}

func (s *Service) maintenanceExecutions(ctx context.Context, target configstore.TargetSettings) ([]configstore.WorkbenchExecution, error) {
	store, ok := s.private.(maintenanceExecutionStore)
	if !ok {
		return nil, errors.New("待上传私有执行存储未就绪")
	}
	values, err := store.WorkbenchMaintenanceExecutions(ctx, executionTargetFingerprint(target))
	if err != nil {
		return nil, err
	}
	result := []configstore.WorkbenchExecution{}
	for _, value := range values {
		if err := validateExecutionClaim(ctx, store, value); err == nil {
			result = append(result, value)
		}
	}
	return result, nil
}

func pendingUploadItem(item configstore.WorkbenchExecutionItem) bool {
	return item.UploadStatus != "" && item.UploadStatus != "done" && item.RetryTaskID == ""
}

func (s *Service) pendingUploadViews(ctx context.Context, target configstore.TargetSettings, attached bool) ([]MaintenancePendingUploadView, error) {
	records, err := s.maintenanceExecutions(ctx, target)
	if err != nil {
		return nil, err
	}
	result := []MaintenancePendingUploadView{}
	for _, record := range records {
		for _, item := range record.Items {
			if !pendingUploadItem(item) {
				continue
			}
			view := MaintenancePendingUploadView{ID: fmt.Sprintf("%s:%d", record.ID, item.Index), SourceTaskID: record.ID, AccountID: item.OriginalAccountID, Email: item.Email, Status: item.UploadStatus, Message: item.UploadMessage, NextRetryAt: item.UploadNextRetryAt, ExpiresAt: record.ExpiresAt}
			if view.Status == "pending" {
				until, _ := time.Parse(time.RFC3339Nano, item.UploadNextRetryAt)
				if !attached {
					view.Status, view.Message = "waiting_session", "请在当前登录会话重新连接自动维护后继续上传"
				} else if time.Now().Before(until) {
					view.Status = "cooldown"
				} else if err := s.requireFinishedUploadSource(ctx, record.ID); err != nil {
					view.Status, view.Message = "running", "原上传任务仍在执行或等待任务核对"
				}
			}
			result = append(result, view)
		}
	}
	return result, nil
}

func (s *Service) processMaintenanceUploads(ctx context.Context, target configstore.TargetSettings, config configstore.WorkbenchMaintenance, update func([]ResultItem) error) ([]ResultItem, map[string]bool, error) {
	records, err := s.maintenanceExecutions(ctx, target)
	if err != nil {
		return nil, nil, err
	}
	owner, ownerErr := s.currentMaintenanceOwner(ctx, target, config.Revision)
	blocked := map[string]bool{}
	rows := []ResultItem{}
	for _, record := range records {
		for position, item := range record.Items {
			if !pendingUploadItem(item) {
				continue
			}
			blocked[item.OriginalAccountID] = true
			row := ResultItem{Index: len(rows), AccountID: item.OriginalAccountID, Email: item.Email, Name: item.Name, Status: "review", Message: item.UploadMessage}
			until, _ := time.Parse(time.RFC3339Nano, item.UploadNextRetryAt)
			if ownerErr != nil {
				row.Message = "新授权已私有保存，请重新连接自动维护后继续上传"
			} else if time.Now().Before(until) {
				row.Message = "待上传账号处于冷却期，本轮不刷新或重新登录"
			} else if err := s.requireFinishedUploadSource(ctx, record.ID); err != nil {
				row.Message = "原上传任务仍在执行或无法核对，本轮跳过"
			} else {
				row = s.retryMaintenanceUpload(ctx, target, config, owner, &record, position)
				row.Index = len(rows)
			}
			rows = append(rows, row)
			if err := update(rows); err != nil {
				return rows, blocked, err
			}
		}
	}
	return rows, blocked, nil
}

func (s *Service) retryMaintenanceUpload(ctx context.Context, target configstore.TargetSettings, config configstore.WorkbenchMaintenance, owner *maintenanceOwner, record *configstore.WorkbenchExecution, position int) ResultItem {
	item := record.Items[position]
	row := ResultItem{Index: item.Index, AccountID: item.OriginalAccountID, Name: item.Name, Email: item.Email, Status: "review"}
	bound, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(owner.ctx, cancel)
	defer cancel()
	defer stop()
	if owner.ctx.Err() != nil {
		cancel()
	}
	preview, err := s.retryPreview(targetguard.Expect(bound, target), owner.owner, RetryPreviewInput{TaskID: record.ID, Indexes: []int{item.Index}, Model: config.Model}, &config)
	if err != nil {
		row.Message = publicError(err).Error()
		item.UploadStatus, item.UploadMessage = "review", row.Message
		item.UploadNextRetryAt = time.Now().Add(time.Duration(config.CooldownMinutes) * time.Minute).UTC().Format(time.RFC3339Nano)
		record.Items[position] = item
		if err := s.private.(executionStore).SaveWorkbenchExecution(bound, *record); err == nil {
			record.Revision++
		} else {
			row.Message = "待上传核对结果保存失败，请查看原任务后重试"
		}
		return row
	}
	completion := make(chan taskstore.Task, 1)
	s.mu.Lock()
	prepared := s.previews[preview.ID]
	if prepared != nil {
		prepared.completion, prepared.lifecycle = completion, bound
	}
	s.mu.Unlock()
	task, err := s.Import(bound, owner.owner, preview.ID, true)
	if err != nil {
		row.Message = publicError(err).Error()
		return row
	}
	s.maintenanceMu.Lock()
	if s.maintenanceOwner == owner {
		owner.importID = task.ID
	}
	s.maintenanceMu.Unlock()
	finished := <-completion
	s.maintenanceMu.Lock()
	if s.maintenanceOwner == owner && owner.importID == task.ID {
		owner.importID = ""
	}
	s.maintenanceMu.Unlock()
	if current, err := s.private.(executionStore).WorkbenchExecution(ctx, record.ID); err == nil {
		*record = current
	}
	if items, ok := finished.Result["items"].([]ResultItem); ok && len(items) == 1 {
		if finished.Status == "failed" || finished.Status == "cancelled" {
			items[0].Status, items[0].Message = "failed", finished.Message
		}
		return items[0]
	}
	row.Message = "待上传任务已结束，请查看原任务核对结果"
	return row
}
