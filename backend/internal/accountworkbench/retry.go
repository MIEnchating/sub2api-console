package accountworkbench

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type executionStore interface {
	WorkbenchExecution(context.Context, string) (configstore.WorkbenchExecution, error)
	SaveWorkbenchExecution(context.Context, configstore.WorkbenchExecution) error
}

type RetryPreviewInput struct {
	TaskID     string `json:"task_id"`
	Indexes    []int  `json:"indexes"`
	TemplateID string `json:"template_id"`
	Model      string `json:"model"`
}

type retryPreparation struct {
	source     configstore.WorkbenchExecution
	positions  []int
	resume     []bool
	reconciled []bool
	snapshots  []map[string]any
}

// RetryPreview materializes only a new, session-bound impact preview. Unknown
// creates are reconciled read-only; a missing marker never authorizes a replay.
func (s *Service) RetryPreview(ctx context.Context, owner string, input RetryPreviewInput) (Preview, error) {
	return s.retryPreview(ctx, owner, input, nil)
}

func (s *Service) retryPreview(ctx context.Context, owner string, input RetryPreviewInput, maintenance *configstore.WorkbenchMaintenance) (Preview, error) {
	view := Preview{Items: []PreviewItem{}, Errors: []InputError{}, CheckAfterImport: true, Model: strings.TrimSpace(input.Model)}
	if owner == "" || input.TaskID == "" {
		return view, ErrPreview
	}
	if view.Model == "" {
		view.Model = "gpt-5.6-sol"
	}
	if len(view.Model) > 200 || strings.ContainsAny(view.Model, "\r\n") {
		return view, errors.New("检测模型无效")
	}
	store, ok := s.private.(executionStore)
	if !ok {
		return view, errors.New("私有执行记录存储未就绪")
	}
	finished := s.requireFinishedImport
	if maintenance != nil {
		finished = s.requireFinishedUploadSource
	}
	if err := finished(ctx, input.TaskID); err != nil {
		return view, err
	}
	record, err := store.WorkbenchExecution(ctx, input.TaskID)
	if err != nil {
		return view, err
	}
	if maintenance != nil && (record.Maintenance == nil || record.Maintenance.Revision != maintenance.Revision || record.Maintenance.Model != maintenance.Model || record.Maintenance.CheckAfterImport != maintenance.CheckAfterRepair) {
		return view, errors.New("待上传任务的维护配置已变化，请人工核对")
	}
	if err := validateExecutionClaim(ctx, store, record); err != nil {
		return view, err
	}
	ctx, err = targetguard.Pin(ctx, s.private)
	if err != nil {
		return view, err
	}
	client, target, err := s.client(ctx)
	if err != nil {
		return view, err
	}
	if target.BaseURL != record.TargetURL || executionTargetFingerprint(target) != record.TargetFingerprint {
		return view, errors.New("管理目标或凭据已变化，不能重用原任务的执行记录")
	}
	view.Target = target.BaseURL
	positions, err := retryPositions(record.Items, input.Indexes)
	if err != nil {
		return view, err
	}
	prepared := &preparedImport{owner: owner, target: target, expires: time.Now().Add(10 * time.Minute), retry: &retryPreparation{source: record, positions: positions}}
	prepared.maintenanceRetry = maintenance != nil
	if maintenance != nil {
		view.CheckAfterImport = maintenance.CheckAfterRepair
	}
	if expires, err := time.Parse(time.RFC3339Nano, record.ExpiresAt); err == nil && expires.Before(prepared.expires) {
		prepared.expires = expires
	}
	var directory []map[string]any
	directoryLoaded := false
	for _, position := range positions {
		saved := record.Items[position]
		item := InputItem{Index: saved.Index, Kind: saved.Kind, Name: saved.Name, Email: saved.Email, PlanType: saved.PlanType}
		if len(saved.Credentials) > 0 {
			decoded, decodeErr := decodeInputJSON(string(saved.Credentials))
			if decodeErr != nil {
				return view, configstore.ErrWorkbenchExecution
			}
			item.Credentials = inputObject(decoded)
		}
		accountID := saved.AccountID
		discovered := false
		if accountID == "" && saved.Phase == "create_submitted" {
			account, found, reconcileErr := client.ReconcileAccountWithMarker(ctx, saved.Marker)
			if reconcileErr != nil || !found {
				return view, fmt.Errorf("第 %d 项创建结果仍未确定，请核对原任务标记，不能重复创建", saved.Index+1)
			}
			accountID = stringValue(account["id"])
		}
		if accountID == "" && saved.Phase == "prepared" {
			if !directoryLoaded {
				directory, err = client.Accounts(ctx)
				if err != nil {
					return view, publicError(err)
				}
				directoryLoaded = true
			}
			for _, account := range directory {
				if !StableIdentityMatch(item, account) {
					continue
				}
				if accountID != "" {
					return view, errors.New("线上稳定身份重复，请先核对账号")
				}
				accountID, discovered = stringValue(account["id"]), true
			}
		}
		var snapshot map[string]any
		resume := false
		reconciled := false
		if accountID != "" {
			if err := s.checkProtection(ctx, accountID); err != nil {
				return view, err
			}
			account, readErr := client.Account(ctx, accountID)
			if readErr != nil {
				return view, publicError(readErr)
			}
			currentCredentials := inputObject(account["credentials"])
			identityCredentials := cloneInputMap(currentCredentials)
			copyInputIdentity(identityCredentials, inputObject(account["extra"]))
			copyInputIdentity(identityCredentials, account)
			if account["platform"] != "openai" || account["type"] != "oauth" || saved.Identity == "" || saved.Identity != IdentityKey(InputItem{Credentials: identityCredentials}) {
				return view, errors.New("原任务账号的稳定身份已变化，请在账号管理核对")
			}
			resume = !discovered && (len(saved.Credentials) == 0 || credentialsContain(currentCredentials, item.Credentials))
			if maintenance != nil {
				if !executionSnapshotMatches(saved, account) {
					return view, errors.New("待上传账号配置已变化，请人工核对原任务")
				}
				if !resume && saved.Phase == "credentials_submitted" && saved.CredentialWriteOutcome != "rejected" {
					return view, errors.New("原凭据写入结果不明且线上尚未确认新凭据，已停止自动重传，请人工核对")
				}
			}
			if resume {
				matchesSnapshot := executionSnapshotMatches(saved, account)
				reconciled = account["schedulable"] == true && matchesSnapshot && (saved.Phase == "promoting" || saved.Phase == "promoted")
				if account["schedulable"] != false && !reconciled {
					return view, errors.New("原任务账号当前未停止调度，请先在账号管理核对状态")
				}
				if len(saved.Snapshot) > 0 && !matchesSnapshot {
					return view, errors.New("隔离账号配置已变化，请核对后重新导入并确认影响范围")
				}
				if saved.OriginalAccountID == "" && (account["status"] != "inactive" || !accountGroupsMatch(account, []any{})) && saved.Phase != "promoting" && !reconciled {
					return view, errors.New("原任务新账号的隔离状态尚未确认，请核对停止调度和空分组")
				}
				item.Credentials = currentCredentials
				snapshot = account
			}
		} else if len(item.Credentials) == 0 {
			return view, errors.New("原任务缺少可重试的凭据，请重新授权或导入")
		}
		templateID := strings.TrimSpace(input.TemplateID)
		if templateID == "" && saved.Template != nil {
			templateID = saved.Template.ID
		}
		if reconciled || (resume && saved.OriginalAccountID != "") {
			templateID = ""
		}
		var template *configstore.WorkbenchTemplate
		if templateID != "" {
			current, readErr := s.private.WorkbenchTemplate(ctx, target.BaseURL, templateID)
			if readErr != nil {
				return view, readErr
			}
			template = &current
		}
		row := PreviewItem{ID: strconv.Itoa(item.Index), Index: item.Index, Name: item.Name, Email: item.Email, PlanType: item.PlanType, AccountID: accountID, Duplicate: accountID != "", GroupIDs: []string{}, Action: "import"}
		if resume {
			row.Action = "check"
		}
		if reconciled {
			row.Action = "reconcile"
		}
		if template != nil {
			row.TemplateID, row.TemplateName, row.TemplateRevision = template.ID, template.Name, template.Revision
			row.GroupIDs = configIDs(template.Config["group_ids"])
		}
		if (saved.OriginalAccountID != "" || reconciled) && snapshot != nil {
			config, extractErr := ExtractTemplate(snapshot)
			if extractErr != nil {
				return view, extractErr
			}
			row.GroupIDs = configIDs(config["group_ids"])
			row.TemplateName = "线上现有配置"
		}
		view.Items = append(view.Items, row)
		prepared.items = append(prepared.items, item)
		var profile *profileAuthorization
		if saved.LoginProfileID != "" {
			profile = &profileAuthorization{id: saved.LoginProfileID, revision: saved.LoginProfileRevision, maintenanceRevision: saved.LoginMaintenanceRevision, maintenanceOwner: saved.LoginMaintenanceOwner, identity: securityBatchItem{accountID: saved.OriginalAccountID, identity: browserlogin.SecurityIdentity{Email: saved.Email, UserID: saved.LoginUserID}, workspaceID: saved.LoginWorkspaceID}}
			if maintenance != nil {
				profile.maintenanceOwner = owner
			}
			if err := s.validateReauthorizationProfiles(ctx, target, []*profileAuthorization{profile}); err != nil {
				return view, err
			}
		}
		prepared.profiles = append(prepared.profiles, profile)
		prepared.templates = append(prepared.templates, template)
		prepared.retry.resume = append(prepared.retry.resume, resume)
		prepared.retry.reconciled = append(prepared.retry.reconciled, reconciled)
		prepared.retry.snapshots = append(prepared.retry.snapshots, snapshot)
	}
	if _, err := targetguard.Pin(targetguard.Expect(ctx, target), s.private); err != nil {
		return Preview{}, err
	}
	view.ID, err = randomID()
	if err != nil {
		return view, err
	}
	view.ExpiresAt = prepared.expires.UTC().Format(time.RFC3339)
	prepared.view = view
	s.mu.Lock()
	for id, value := range s.previews {
		if time.Now().After(value.expires) || value.owner == owner {
			delete(s.previews, id)
		}
	}
	if len(s.previews) >= 20 {
		s.mu.Unlock()
		return Preview{}, errors.New("预览任务已满，请稍后重试")
	}
	s.previews[view.ID] = prepared
	s.mu.Unlock()
	time.AfterFunc(time.Until(prepared.expires), func() { s.DeletePreview(owner, view.ID) })
	return view, nil
}

func retryPositions(items []configstore.WorkbenchExecutionItem, indexes []int) ([]int, error) {
	selected := make(map[int]bool, len(indexes))
	for _, index := range indexes {
		if index < 0 || selected[index] {
			return nil, errors.New("重试项目编号无效或重复")
		}
		selected[index] = true
	}
	positions := []int{}
	for position, item := range items {
		if len(indexes) > 0 && !selected[item.Index] {
			continue
		}
		if item.Status == "succeeded" || item.Status == "skipped" || item.RetryTaskID != "" {
			if len(indexes) > 0 {
				return nil, errors.New("所选项目已完成或已有重试任务，请查看最新任务")
			}
			continue
		}
		positions = append(positions, position)
	}
	if len(positions) == 0 || (len(indexes) > 0 && len(positions) != len(indexes)) {
		return nil, errors.New("没有可重试的失败或待复核项目")
	}
	return positions, nil
}

func (s *Service) requireFinishedImport(ctx context.Context, id string) error {
	reader, ok := s.tasks.(interface {
		Get(context.Context, string) (taskstore.Task, error)
	})
	if !ok {
		return errors.New("任务记录读取未就绪")
	}
	task, err := reader.Get(ctx, id)
	if err != nil || task.Skill != Skill || (task.Operation != "account-workbench-import" && task.Operation != "account-workbench-retry") {
		return errors.New("原导入任务不存在或不能重试")
	}
	if task.Status == "queued" || task.Status == "running" {
		return errors.New("原导入任务仍在执行，请等待完成后重试")
	}
	return nil
}

func executionTargetFingerprint(target configstore.TargetSettings) string {
	raw, _ := json.Marshal([]string{target.BaseURL, target.AdminKey})
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func validateExecutionClaim(ctx context.Context, store executionStore, record configstore.WorkbenchExecution) error {
	if validator, ok := store.(interface {
		ValidateWorkbenchExecutionClaim(context.Context, configstore.WorkbenchExecution) error
	}); ok {
		return validator.ValidateWorkbenchExecutionClaim(ctx, record)
	}
	if record.SourceID == "" {
		return nil
	}
	source, err := store.WorkbenchExecution(ctx, record.SourceID)
	if err != nil {
		return configstore.ErrWorkbenchExecution
	}
	claims := make(map[int]string, len(source.Items))
	for _, item := range source.Items {
		claims[item.Index] = item.RetryTaskID
	}
	for _, item := range record.Items {
		if claims[item.Index] != record.ID {
			return errors.New("该重试任务未取得原任务执行权，请查看原任务的最新处理记录")
		}
	}
	return nil
}

func executionSnapshot(account map[string]any) json.RawMessage {
	filtered := map[string]any{}
	for _, key := range append([]string{"id", "name", "platform", "type", "credentials", "groups", "account_groups", "extra", "status", "schedulable", "error", "error_message", "temp_unschedulable_reason", "temp_unschedulable_until", "rate_limit_reset_at", "overload_until"}, transferableAccountFields...) {
		filtered[key] = account[key]
	}
	for _, key := range []string{"error", "error_message", "temp_unschedulable_reason", "temp_unschedulable_until", "rate_limit_reset_at", "overload_until"} {
		value := stringValue(account[key])
		if value == "0" {
			value = ""
		}
		filtered[key] = value
	}
	if config, err := ExtractTemplate(account); err == nil {
		filtered["group_ids"] = configIDs(config["group_ids"])
		delete(filtered, "groups")
		delete(filtered, "account_groups")
	}
	raw, err := json.Marshal(filtered)
	if err != nil {
		return nil
	}
	digest := sha256.Sum256(raw)
	encoded, _ := json.Marshal(hex.EncodeToString(digest[:]))
	return encoded
}

func executionSnapshotMatches(item configstore.WorkbenchExecutionItem, account map[string]any) bool {
	actual := executionSnapshot(account)
	return len(actual) > 0 && (reflect.DeepEqual(item.Snapshot, actual) || reflect.DeepEqual(item.PendingSnapshot, actual))
}

func (s *Service) preparePromotionWrite(ctx context.Context, prepared *preparedImport, index int, current, expected map[string]any) error {
	prepared.execution.Items[index].Phase = "promoting"
	prepared.execution.Items[index].Snapshot = executionSnapshot(current)
	prepared.execution.Items[index].PendingSnapshot = executionSnapshot(expected)
	return s.saveExecution(ctx, prepared)
}

func (s *Service) saveExecution(ctx context.Context, prepared *preparedImport) error {
	store, ok := s.private.(executionStore)
	if !ok || prepared.execution == nil {
		return errors.New("私有执行记录存储未就绪，未继续修改账号")
	}
	if err := store.SaveWorkbenchExecution(ctx, *prepared.execution); err != nil {
		return errors.New("私有执行记录保存失败，已停止操作，请核对任务和线上账号")
	}
	prepared.execution.Revision++
	return nil
}

func (s *Service) initializeExecution(ctx context.Context, prepared *preparedImport, taskID string) error {
	record := &configstore.WorkbenchExecution{ID: taskID, TargetURL: prepared.target.BaseURL, TargetFingerprint: executionTargetFingerprint(prepared.target)}
	if prepared.retry != nil {
		record.SourceID = prepared.retry.source.ID
	}
	for index, item := range prepared.items {
		credentials, err := json.Marshal(item.Credentials)
		if err != nil {
			return err
		}
		saved := configstore.WorkbenchExecutionItem{Index: item.Index, Kind: item.Kind, Name: item.Name, Email: item.Email, PlanType: item.PlanType, Credentials: credentials, Identity: IdentityKey(item), Template: prepared.templates[index], OriginalAccountID: prepared.view.Items[index].AccountID, Phase: "prepared", Status: "queued"}
		if len(prepared.profiles) > index && prepared.profiles[index] != nil {
			profile := prepared.profiles[index]
			saved.LoginProfileID, saved.LoginProfileRevision, saved.LoginUserID, saved.LoginWorkspaceID = profile.id, profile.revision, profile.identity.identity.UserID, profile.identity.workspaceID
			saved.LoginMaintenanceRevision = profile.maintenanceRevision
			saved.LoginMaintenanceOwner = profile.maintenanceOwner
		}
		if prepared.retry != nil {
			original := prepared.retry.source.Items[prepared.retry.positions[index]]
			saved.OriginalAccountID, saved.Marker, saved.Phase = original.OriginalAccountID, original.Marker, original.Phase
			saved.Snapshot, saved.PendingSnapshot = original.Snapshot, original.PendingSnapshot
			saved.CredentialWriteOutcome = original.CredentialWriteOutcome
			saved.AccountID = prepared.view.Items[index].AccountID
			if saved.OriginalAccountID == "" && saved.Marker == "" && saved.AccountID != "" {
				saved.OriginalAccountID = saved.AccountID
			}
			if prepared.retry.resume[index] {
				saved.Credentials = nil
				saved.Snapshot = executionSnapshot(prepared.retry.snapshots[index])
			}
		}
		record.Items = append(record.Items, saved)
	}
	if err := s.initializeMaintenanceExecution(ctx, prepared, record); err != nil {
		return err
	}
	prepared.execution = record
	if err := s.saveExecution(ctx, prepared); err != nil {
		return err
	}
	return nil
}

func (s *Service) claimRetryExecution(ctx context.Context, prepared *preparedImport) error {
	if prepared.retry == nil {
		return nil
	}
	finished := s.requireFinishedImport
	if prepared.maintenanceRetry {
		finished = s.requireFinishedUploadSource
	}
	if err := finished(ctx, prepared.retry.source.ID); err != nil {
		return err
	}
	for _, position := range prepared.retry.positions {
		prepared.retry.source.Items[position].RetryTaskID = prepared.execution.ID
		// The successor execution is already persisted before this claim.
		prepared.retry.source.Items[position].Credentials = nil
	}
	store := s.private.(executionStore)
	if err := store.SaveWorkbenchExecution(ctx, prepared.retry.source); err != nil {
		return configstore.ErrWorkbenchExecution
	}
	return nil
}

func (s *Service) resumeImported(ctx context.Context, prepared *preparedImport, index int, item InputItem) ResultItem {
	row := ResultItem{Index: item.Index, Name: item.Name, AccountID: prepared.view.Items[index].AccountID, Status: "review"}
	guarded, release, err := targetguard.Acquire(ctx, s.repository, mutationguard.Account(row.AccountID))
	if err != nil {
		row.Message = publicError(err).Error()
		return row
	}
	guarded, err = targetguard.Bind(guarded, s.private)
	if err == nil {
		err = s.validateImportWrite(guarded, prepared, index, row.AccountID)
	}
	client, clientErr := s.clientFor(prepared.target)
	if err == nil {
		err = clientErr
	}
	var current map[string]any
	if err == nil {
		current, err = client.Account(guarded, row.AccountID)
	}
	if err != nil {
		_ = release()
		row.Message = publicError(err).Error()
		return row
	}
	if !sameAccountConfiguration(prepared.retry.snapshots[index], current) || (current["schedulable"] != false && !prepared.retry.reconciled[index]) {
		_ = release()
		row.Message = "重试预览后账号配置发生变化，未检测或启用，请重新预览"
		return row
	}
	if prepared.retry.reconciled[index] {
		prepared.execution.Items[index].Phase = "promoted"
		prepared.execution.Items[index].Snapshot = executionSnapshot(current)
		if err := s.audit(guarded, row.AccountID, "workbench.reconcile", "succeeded", true); err != nil {
			_ = release()
			row.Message = "启用状态已核对，但审计保存失败，请查看任务记录"
			return row
		}
		_ = release()
		row.Status, row.Message = "succeeded", "已核对原任务的账号启用状态，无需重复写入"
		return row
	}
	payload, err := ApplyTemplate(item, prepared.templates[index])
	if err != nil {
		_ = release()
		row.Message = err.Error()
		return row
	}
	_ = release()
	isNew := prepared.execution.Items[index].OriginalAccountID == ""
	payload["credentials"] = inputObject(current["credentials"])
	return s.checkAndPromote(ctx, prepared, index, row, current, payload, isNew)
}
