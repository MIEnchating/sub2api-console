package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/redact"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type ResultItem struct {
	Index     int            `json:"index"`
	Name      string         `json:"name"`
	Email     string         `json:"email,omitempty"`
	Status    string         `json:"status"`
	Message   string         `json:"message"`
	AccountID string         `json:"account_id,omitempty"`
	Report    map[string]any `json:"report,omitempty"`
}

func (s *Service) Import(ctx context.Context, owner, previewID string, confirmed bool) (taskstore.Task, error) {
	if !confirmed {
		return taskstore.Task{}, errors.New("请先确认导入影响范围")
	}
	s.mu.Lock()
	prepared := s.previews[previewID]
	if prepared == nil || prepared.owner != owner || time.Now().After(prepared.expires) {
		s.mu.Unlock()
		return taskstore.Task{}, ErrPreview
	}
	if prepared.view.ExportOnly || prepared.view.Scope == ScopeLocalExport {
		s.mu.Unlock()
		return taskstore.Task{}, errors.New("此预览仅用于生成私有文件，请使用导出操作")
	}
	delete(s.previews, previewID)
	s.mu.Unlock()
	if _, err := targetguard.Pin(targetguard.Expect(ctx, prepared.target), s.private); err != nil {
		return taskstore.Task{}, err
	}
	if err := s.validateReauthorizationProfiles(ctx, prepared.target, prepared.profiles); err != nil {
		return taskstore.Task{}, err
	}
	for _, template := range prepared.templates {
		if template == nil {
			continue
		}
		current, err := s.private.WorkbenchTemplate(ctx, prepared.target.BaseURL, template.ID)
		if err != nil || current.Revision != template.Revision {
			return taskstore.Task{}, errors.New("模板已修改，请重新解析并确认影响范围")
		}
	}
	taskID, err := randomID()
	if err != nil {
		return taskstore.Task{}, err
	}
	if err := s.initializeExecution(ctx, prepared, taskID); err != nil {
		return taskstore.Task{}, err
	}
	operation := "account-workbench-import"
	if prepared.retry != nil {
		operation = "account-workbench-retry"
	}
	return s.enqueueWithID(ctx, taskID, operation, "等待导入账号", func(run context.Context, update func([]ResultItem) error) ([]ResultItem, error) {
		if prepared.lifecycle != nil {
			bound, cancel := context.WithCancel(run)
			stop := context.AfterFunc(prepared.lifecycle, cancel)
			defer stop()
			defer cancel()
			if prepared.lifecycle.Err() != nil {
				cancel()
			}
			run = bound
		}
		run = targetguard.Expect(run, prepared.target)
		rows := make([]ResultItem, len(prepared.items))
		for i, item := range prepared.items {
			rows[i] = ResultItem{Index: item.Index, Name: item.Name, Email: item.Email, Status: "queued", Message: "等待处理"}
		}
		if err := s.claimRetryExecution(run, prepared); err != nil {
			return rows, err
		}
		for i, item := range prepared.items {
			if run.Err() != nil {
				return rows, run.Err()
			}
			rows[i].Status, rows[i].Message = "running", "正在验证身份与隔离导入"
			if err := update(rows); err != nil {
				return rows, err
			}
			if prepared.retry != nil && prepared.retry.resume[i] {
				rows[i] = s.resumeImported(run, prepared, i, item)
			} else {
				rows[i] = s.importItem(run, prepared, i, item)
			}
			prepared.execution.Items[i].Status = rows[i].Status
			updateMaintenanceUploadItem(prepared.execution, i, rows[i])
			if rows[i].AccountID != "" {
				prepared.execution.Items[i].AccountID = rows[i].AccountID
			}
			persist, cancel := context.WithTimeout(context.WithoutCancel(run), 5*time.Second)
			persistErr := s.saveExecution(persist, prepared)
			cancel()
			prepared.items[i].Credentials = nil
			if persistErr != nil {
				return rows, persistErr
			}
			if err := update(rows); err != nil {
				return rows, err
			}
		}
		if s.syncAccounts != nil {
			if _, err := s.syncAccounts(run, "account-workbench"); err != nil {
				return rows, errors.New("远端操作已记录，但账号目录同步失败，请在账号管理重新同步")
			}
		}
		return rows, nil
	}, prepared.completion)
}

func (s *Service) importItem(ctx context.Context, prepared *preparedImport, index int, item InputItem) ResultItem {
	row := ResultItem{Index: item.Index, Name: item.Name, Email: item.Email, Status: "failed"}
	guarded, release, err := targetguard.Acquire(ctx, s.repository, mutationguard.AccountCatalog())
	if err != nil {
		row.Message = publicError(err).Error()
		return row
	}
	defer func() {
		if release != nil {
			_ = release()
		}
	}()
	guarded, err = targetguard.Bind(guarded, s.private)
	if err != nil {
		row.Message = err.Error()
		return row
	}
	validate := func() error { return s.validateImportWrite(guarded, prepared, index, row.AccountID) }
	if err := validate(); err != nil {
		row.Message = err.Error()
		return row
	}
	client, err := s.clientFor(prepared.target)
	if err != nil {
		row.Message = publicError(err).Error()
		return row
	}
	item, err = materialize(guarded, client, item)
	if err != nil {
		row.Message = publicError(err).Error()
		return row
	}
	payload, err := ApplyTemplate(item, prepared.templates[index])
	if err != nil {
		row.Message = err.Error()
		return row
	}
	accounts, err := client.Accounts(guarded)
	if err != nil {
		row.Message = publicError(err).Error()
		return row
	}
	var existing map[string]any
	for _, account := range accounts {
		if StableIdentityMatch(item, account) {
			if existing != nil {
				row.Message = "线上稳定身份重复，请先核对账号"
				return row
			}
			existing = account
		}
	}
	if existing != nil {
		row.AccountID = stringValue(existing["id"])
		if prepared.view.Items[index].AccountID != row.AccountID {
			row.Status, row.Message = "review", "授权解析后发现已有账号，请重新预览并确认目标后更新凭据"
			return row
		}
		if err := s.checkProtection(guarded, row.AccountID); err != nil {
			row.Message = err.Error()
			return row
		}
		// Reacquire the complete resource set before touching an existing account.
		_ = release()
		release = nil
		guarded, release, err = targetguard.Acquire(ctx, s.repository, mutationguard.AccountCatalog(), mutationguard.Account(row.AccountID))
		if err == nil {
			guarded, err = targetguard.Bind(guarded, s.private)
		}
		if err != nil {
			row.Message = publicError(err).Error()
			return row
		}
		existing, err = client.Account(guarded, row.AccountID)
		if err != nil {
			row.Message = publicError(err).Error()
			return row
		}
		if !StableIdentityMatch(item, existing) {
			row.Message = "线上账号身份已变化，请重新预览"
			return row
		}
		if prepared.execution.Maintenance != nil && !executionSnapshotMatches(prepared.execution.Items[index], existing) {
			row.Message = "待上传账号配置已变化，未更新凭据，请人工核对"
			return row
		}
		if currentCredentials, ok := existing["credentials"].(map[string]any); ok {
			mergedCredentials := copyObject(currentCredentials)
			for key, value := range item.Credentials {
				mergedCredentials[key] = value
			}
			payload["credentials"] = mergedCredentials
		}
		if err := validate(); err != nil {
			row.Message = err.Error()
			return row
		}
		if err := s.audit(guarded, row.AccountID, "workbench.credentials", "started", false); err != nil {
			row.Message = "审计保存失败，未更新凭据"
			return row
		}
		if err := validate(); err != nil {
			row.Message = err.Error()
			return row
		}
		if _, err := client.SetAccountSchedulable(guarded, row.AccountID, false); err != nil {
			row.Message = publicError(err).Error()
			return row
		}
		paused, readErr := client.Account(guarded, row.AccountID)
		if readErr != nil || paused["schedulable"] != false {
			row.Message = "未能确认账号已停止调度，未更新凭据"
			return row
		}
		expectedPaused := copyObject(existing)
		expectedPaused["schedulable"] = false
		if !sameAccountConfiguration(expectedPaused, paused) {
			row.Message = "暂停期间账号配置发生变化，未更新凭据，请重新预览"
			return row
		}
		if err := validate(); err != nil {
			row.Message = err.Error()
			return row
		}
		prepared.execution.Items[index].AccountID = row.AccountID
		prepared.execution.Items[index].Phase = "credentials_submitted"
		prepared.execution.Items[index].CredentialWriteOutcome = "unknown"
		prepared.execution.Items[index].Snapshot = executionSnapshot(paused)
		expectedCredentialsWrite := copyObject(paused)
		expectedCredentialsWrite["credentials"] = payload["credentials"]
		prepared.execution.Items[index].PendingSnapshot = executionSnapshot(expectedCredentialsWrite)
		if err := s.saveExecution(guarded, prepared); err != nil {
			row.Message = err.Error()
			return row
		}
		if _, err = client.UpdateAccount(guarded, row.AccountID, map[string]any{"credentials": payload["credentials"]}); err != nil {
			var rejected *adminclient.HTTPError
			if errors.As(err, &rejected) && rejected.StatusCode >= 400 && rejected.StatusCode < 500 && rejected.StatusCode != http.StatusRequestTimeout {
				prepared.execution.Items[index].CredentialWriteOutcome = "rejected"
			}
			row.Message = publicError(err).Error()
			return row
		}
		prepared.execution.Items[index].CredentialWriteOutcome = "accepted"
	} else {
		if prepared.view.Items[index].AccountID != "" {
			row.Message = "预览中的账号已不存在，请重新解析"
			return row
		}
		create := copyObject(payload)
		create["status"], create["schedulable"], create["group_ids"], create["skip_default_group_bind"] = "inactive", false, []int64{}, true
		marker := "[console-workbench:" + prepared.view.ID + ":" + fmt.Sprint(item.Index) + "]"
		create["notes"] = strings.TrimSpace(stringValue(create["notes"]) + "\n" + marker)
		if err := validate(); err != nil {
			row.Message = err.Error()
			return row
		}
		prepared.execution.Items[index].Marker, prepared.execution.Items[index].Phase = marker, "create_submitted"
		if err := s.saveExecution(guarded, prepared); err != nil {
			row.Message = err.Error()
			return row
		}
		account, createErr := client.CreateAccountWithMarker(guarded, create, marker)
		if createErr != nil {
			row.Message = publicError(createErr).Error()
			return row
		}
		row.AccountID = stringValue(account["id"])
		prepared.execution.Items[index].AccountID, prepared.execution.Items[index].Phase = row.AccountID, "created"
		if err := s.saveExecution(guarded, prepared); err != nil {
			row.Message = err.Error()
			return row
		}
		if err := validate(); err != nil {
			row.Message = err.Error()
			return row
		}
		if _, err := client.UpdateAccount(guarded, row.AccountID, map[string]any{"status": "inactive", "group_ids": []int64{}}); err != nil {
			row.Message = publicError(err).Error()
			return row
		}
		if err := validate(); err != nil {
			row.Message = err.Error()
			return row
		}
		if _, err := client.SetAccountSchedulable(guarded, row.AccountID, false); err != nil {
			row.Message = publicError(err).Error()
			return row
		}
	}
	account, err := client.Account(guarded, row.AccountID)
	if err != nil || account["schedulable"] != false {
		row.Message = "隔离状态读回失败，请立即核对线上账号"
		return row
	}
	if existing == nil && (account["status"] != "inactive" || !accountGroupsMatch(account, []any{})) {
		row.Message = "账号停用状态或空分组未读回确认，请核对线上隔离设置"
		return row
	}
	credentials, _ := account["credentials"].(map[string]any)
	expectedCredentials, _ := payload["credentials"].(map[string]any)
	if !credentialsContain(credentials, expectedCredentials) {
		row.Message = "凭据写入读回不一致，账号保持不可调度"
		return row
	}
	prepared.execution.Items[index].Phase = "staged"
	prepared.execution.Items[index].Credentials = nil
	prepared.execution.Items[index].Snapshot = executionSnapshot(account)
	prepared.execution.Items[index].PendingSnapshot = nil
	if err := s.saveExecution(guarded, prepared); err != nil {
		row.Message = err.Error()
		return row
	}
	if err := s.audit(guarded, row.AccountID, "workbench.import", "succeeded", true); err != nil {
		row.Message = "远端已导入但审计保存失败，账号保持不可调度"
		return row
	}
	_ = release()
	release = nil
	return s.checkAndPromote(ctx, prepared, index, row, account, payload, existing == nil)
}

func (s *Service) checkAndPromote(ctx context.Context, prepared *preparedImport, index int, row ResultItem, account, payload map[string]any, isNew bool) ResultItem {
	credentials := inputObject(account["credentials"])
	expectedCredentials := inputObject(payload["credentials"])
	row.Status, row.Message = "review", "已隔离导入，尚未通过行为检测，保持不可调度"
	if !prepared.view.CheckAfterImport {
		return row
	}
	if s.checker == nil {
		row.Message = "已隔离导入，行为检测服务尚未就绪"
		return row
	}
	report, checkErr := s.checker.CheckOAuth(ctx, row.AccountID, row.Name, credentials, prepared.view.Model, 60)
	row.Report = publicReport(report)
	for _, key := range []string{"access_token", "refresh_token", "id_token"} {
		if secret := stringValue(credentials[key]); secret != "" {
			scrubReportSecret(row.Report, secret)
		}
	}
	if checkErr != nil {
		row.Message = "行为检测未完成，账号保持不可调度，请重试检测"
		return row
	}
	if report["verdict"] != "SOL_CONSISTENT" {
		row.Message = "行为检测结果需要复核，账号保持不可调度"
		return row
	}
	guarded, release, err := targetguard.Acquire(ctx, s.repository, mutationguard.Account(row.AccountID))
	defer func() {
		if release != nil {
			_ = release()
		}
	}()
	if err == nil {
		guarded, err = targetguard.Bind(guarded, s.private)
	}
	if err != nil {
		row.Message = "检测后管理目标已变化，未启用账号"
		return row
	}
	validate := func() error { return s.validateImportWrite(guarded, prepared, index, row.AccountID) }
	if err := validate(); err != nil {
		row.Message = err.Error()
		return row
	}
	client, err := s.clientFor(prepared.target)
	if err != nil {
		row.Message = publicError(err).Error()
		return row
	}
	current, err := client.Account(guarded, row.AccountID)
	if err != nil {
		row.Message = publicError(err).Error()
		return row
	}
	if !sameAccountConfiguration(account, current) {
		row.Message = "检测期间账号配置发生变化，未自动启用，请复核"
		return row
	}
	if err := s.audit(guarded, row.AccountID, "workbench.promote", "started", false); err != nil {
		row.Message = "审计保存失败，未启用账号"
		return row
	}
	final := map[string]any{"status": "active"}
	if isNew {
		final = copyObject(payload)
		delete(final, "credentials")
		delete(final, "platform")
		delete(final, "type")
		final["status"] = "active"
	} else if stringValue(current["error_message"]) != "" || stringValue(current["error"]) != "" || current["status"] == "error" {
		if err := validate(); err != nil {
			row.Message = err.Error()
			return row
		}
		expected := copyObject(current)
		expected["status"], expected["error"], expected["error_message"] = "active", "", ""
		if err := s.preparePromotionWrite(guarded, prepared, index, current, expected); err != nil {
			row.Message = err.Error()
			return row
		}
		if _, err := client.Mutate(guarded, http.MethodPost, "/admin/accounts/"+row.AccountID+"/clear-error", nil); err != nil {
			row.Message = publicError(err).Error()
			return row
		}
		cleared, readErr := client.Account(guarded, row.AccountID)
		if readErr != nil || cleared["schedulable"] != false || !accountErrorClear(cleared) || !sameAccountStaticConfiguration(current, cleared) {
			row.Message = "清除错误后的账号状态未读回确认，未继续启用"
			return row
		}
		current = cleared
	}
	if !isNew {
		if err := validate(); err != nil {
			row.Message = err.Error()
			return row
		}
		expected := copyObject(current)
		expected["status"] = "active"
		for _, key := range []string{"error", "error_message", "temp_unschedulable_reason", "temp_unschedulable_until", "rate_limit_reset_at", "overload_until"} {
			delete(expected, key)
		}
		if err := s.preparePromotionWrite(guarded, prepared, index, current, expected); err != nil {
			row.Message = err.Error()
			return row
		}
		if _, err := client.RecoverAccountState(guarded, row.AccountID); err != nil {
			row.Message = publicError(err).Error()
			return row
		}
		recovered, readErr := client.Account(guarded, row.AccountID)
		if readErr != nil || recovered["schedulable"] != false || !accountRuntimeReady(recovered, time.Now()) || !sameAccountStaticConfiguration(current, recovered) {
			row.Message = "恢复后的账号状态未读回确认，未继续启用"
			return row
		}
		current = recovered
	}
	if err := validate(); err != nil {
		row.Message = err.Error()
		return row
	}
	expectedConfigured := copyObject(current)
	for key, value := range final {
		expectedConfigured[key] = value
	}
	if err := s.preparePromotionWrite(guarded, prepared, index, current, expectedConfigured); err != nil {
		row.Message = err.Error()
		return row
	}
	if _, err := client.UpdateAccount(guarded, row.AccountID, final); err != nil {
		row.Message = publicError(err).Error()
		return row
	}
	configured, err := client.Account(guarded, row.AccountID)
	if err != nil || configured["schedulable"] != false || !accountSettingsMatch(configured, final) || !accountRuntimeReady(configured, time.Now()) || !credentialsContain(inputObject(configured["credentials"]), expectedCredentials) || (!isNew && !sameAccountStaticConfiguration(current, configured)) {
		row.Message = "启用前配置未读回确认，账号保持不可调度，请核对模板和错误状态"
		return row
	}
	expectedEnabled := copyObject(configured)
	expectedEnabled["schedulable"] = true
	if err := s.preparePromotionWrite(guarded, prepared, index, configured, expectedEnabled); err != nil {
		row.Message = err.Error()
		return row
	}
	if err := validate(); err != nil {
		row.Message = err.Error()
		return row
	}
	if _, err := client.SetAccountSchedulable(guarded, row.AccountID, true); err != nil {
		row.Message = publicError(err).Error()
		return row
	}
	confirmed, err := client.Account(guarded, row.AccountID)
	if err != nil || confirmed["schedulable"] != true || !accountSettingsMatch(confirmed, final) || !accountRuntimeReady(confirmed, time.Now()) || !credentialsContain(inputObject(confirmed["credentials"]), expectedCredentials) || (!isNew && !sameAccountStaticConfiguration(current, confirmed)) {
		row.Message = "启用状态尚未读回确认，请核对线上账号"
		return row
	}
	prepared.execution.Items[index].Phase = "promoted"
	prepared.execution.Items[index].Snapshot = executionSnapshot(confirmed)
	prepared.execution.Items[index].PendingSnapshot = nil
	if err := s.audit(guarded, row.AccountID, "workbench.promote", "succeeded", true); err != nil {
		row.Message = "账号已启用，但审计保存失败，请核对日志"
		return row
	}
	row.Status, row.Message = "succeeded", "行为检测通过，已确认账号可调度"
	return row
}

func publicReport(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	// Normalize every serializable container, including typed slices, before
	// applying the same redaction rules recursively.
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	normalized, err := decodeInputJSON(string(raw))
	if err != nil {
		return nil
	}
	return cleanReportObject(inputObject(normalized))
}

func cleanReportObject(value map[string]any) map[string]any {
	clean := make(map[string]any, len(value))
	for key, item := range value {
		lower := strings.ToLower(strings.TrimSpace(key))
		if strings.Contains(lower, "token") || strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "credential") {
			continue
		}
		clean[key] = publicReportValue(item)
	}
	return clean
}

func publicReportValue(value any) any {
	switch item := value.(type) {
	case string:
		return redact.Secrets(item)
	case map[string]any:
		return cleanReportObject(item)
	case []any:
		result := make([]any, len(item))
		for i, child := range item {
			result[i] = publicReportValue(child)
		}
		return result
	default:
		return value
	}
}

func scrubReportSecret(value any, secret string) {
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			cleanKey := strings.ReplaceAll(key, secret, "[已隐藏]")
			if cleanKey != key {
				delete(item, key)
			}
			if text, ok := child.(string); ok {
				item[cleanKey] = strings.ReplaceAll(text, secret, "[已隐藏]")
			} else {
				scrubReportSecret(child, secret)
				item[cleanKey] = child
			}
		}
	case []any:
		for i, child := range item {
			if text, ok := child.(string); ok {
				item[i] = strings.ReplaceAll(text, secret, "[已隐藏]")
			} else {
				scrubReportSecret(child, secret)
			}
		}
	}
}

func materialize(ctx context.Context, client *adminclient.Client, item InputItem) (InputItem, error) {
	if stringValue(item.Credentials["access_token"]) != "" {
		return item, nil
	}
	refresh := stringValue(item.Credentials["refresh_token"])
	if refresh == "" {
		return item, errors.New("缺少有效授权凭据")
	}
	response, err := client.Mutate(ctx, http.MethodPost, "/admin/openai/refresh-token", map[string]any{"refresh_token": refresh})
	if err != nil {
		return item, err
	}
	data := responseData(response)
	if stringValue(data["access_token"]) == "" {
		return item, errors.New("刷新接口未返回访问令牌")
	}
	if stringValue(data["refresh_token"]) == "" {
		data["refresh_token"] = refresh
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return item, err
	}
	parsed, failures := Parse(string(encoded))
	if len(failures) > 0 || len(parsed) != 1 {
		return item, errors.New("刷新接口返回的身份无效")
	}
	result := parsed[0]
	result.Index = item.Index
	if item.Name != "" {
		result.Name = item.Name
	}
	return result, nil
}

func (s *Service) checkProtection(ctx context.Context, id string) error {
	if repo, ok := s.repository.(interface {
		AccountMutationProtection(context.Context, string) (business.AccountMutationProtection, error)
	}); ok {
		protection, err := repo.AccountMutationProtection(ctx, id)
		if err != nil {
			return errors.New("人工控制状态读取失败，未修改账号")
		}
		if protection.Protected() {
			return errors.New("账号受人工控制保护，请先在账号管理核对")
		}
	}
	return nil
}

func responseData(value map[string]any) map[string]any {
	if data, ok := value["data"].(map[string]any); ok {
		return data
	}
	return value
}
func copyObject(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, v := range value {
		result[key] = v
	}
	return result
}
func credentialsContain(current, expected map[string]any) bool {
	for key, value := range expected {
		if !reflect.DeepEqual(current[key], value) {
			return false
		}
	}
	return stringValue(current["access_token"]) != ""
}
func sameAccountConfiguration(before, after map[string]any) bool {
	if !sameAccountStaticConfiguration(before, after) {
		return false
	}
	for _, key := range []string{"status", "schedulable", "error", "error_message", "temp_unschedulable_reason", "temp_unschedulable_until", "rate_limit_reset_at", "overload_until"} {
		if !reflect.DeepEqual(before[key], after[key]) {
			return false
		}
	}
	return true
}

func sameAccountStaticConfiguration(before, after map[string]any) bool {
	for _, key := range append([]string{"id", "name", "platform", "type", "credentials", "groups", "account_groups", "extra"}, transferableAccountFields...) {
		if !reflect.DeepEqual(before[key], after[key]) {
			return false
		}
	}
	return true
}

func (s *Service) validateImportWrite(ctx context.Context, prepared *preparedImport, index int, accountID string) error {
	if _, err := targetguard.Pin(ctx, s.private); err != nil {
		return err
	}
	if len(prepared.profiles) > index && prepared.profiles[index] != nil {
		if err := s.validateReauthorizationProfiles(ctx, prepared.target, []*profileAuthorization{prepared.profiles[index]}); err != nil {
			return err
		}
		if accountID != "" && accountID != prepared.profiles[index].identity.accountID {
			return errors.New("重新授权更新目标与原稳定账号不一致")
		}
	}
	if template := prepared.templates[index]; template != nil {
		current, err := s.private.WorkbenchTemplate(ctx, prepared.target.BaseURL, template.ID)
		if err != nil || current.Revision != template.Revision {
			return errors.New("模板已修改，已停止执行旧预览，请重新解析")
		}
	}
	if accountID != "" {
		return s.checkProtection(ctx, accountID)
	}
	return ctx.Err()
}

func accountErrorClear(account map[string]any) bool {
	return strings.TrimSpace(stringValue(account["error_message"])+stringValue(account["error"])) == ""
}

func accountRuntimeReady(account map[string]any, now time.Time) bool {
	if !accountErrorClear(account) || stringValue(account["temp_unschedulable_reason"]) != "" {
		return false
	}
	for _, key := range []string{"temp_unschedulable_until", "rate_limit_reset_at", "overload_until"} {
		text := stringValue(account[key])
		if text == "" || text == "0" {
			continue
		}
		until := credentialExpiry(account[key])
		if until.IsZero() || until.After(now) {
			return false
		}
	}
	return true
}

func accountSettingsMatch(account, expected map[string]any) bool {
	for key, value := range expected {
		if key == "group_ids" {
			if !accountGroupsMatch(account, value) {
				return false
			}
			continue
		}
		if slices.Contains([]string{"rate_multiplier", "load_factor", "priority", "concurrency", "proxy_id"}, key) && value != nil {
			left, leftOK := accountDecimal(account[key])
			right, rightOK := accountDecimal(value)
			if !leftOK || !rightOK || left.Cmp(right) != 0 {
				return false
			}
			continue
		}
		if !reflect.DeepEqual(account[key], value) {
			return false
		}
	}
	return true
}

var accountDecimalPattern = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]{1,3})?$`)

func accountDecimal(value any) (*big.Rat, bool) {
	text := stringValue(value)
	if len(text) > 96 || !accountDecimalPattern.MatchString(text) {
		return nil, false
	}
	return new(big.Rat).SetString(text)
}

func accountGroupsMatch(account map[string]any, expected any) bool {
	wantedRaw, err := json.Marshal(expected)
	if err != nil {
		return false
	}
	wanted := configIDs(wantedRaw)
	slices.Sort(wanted)
	found := false
	for _, field := range []string{"group_ids", "groups", "account_groups"} {
		raw, exists := account[field]
		if !exists || raw == nil {
			continue
		}
		found = true
		values, ok := raw.([]any)
		if !ok {
			return false
		}
		actual := make([]string, 0, len(values))
		for _, item := range values {
			id := item
			if group, ok := item.(map[string]any); ok {
				id = group["id"]
				if field == "account_groups" {
					id = group["group_id"]
				}
			}
			actual = append(actual, stringValue(id))
		}
		slices.Sort(actual)
		if !slices.Equal(actual, wanted) {
			return false
		}
	}
	return found
}

func (s *Service) enqueue(ctx context.Context, operation, message string, run func(context.Context, func([]ResultItem) error) ([]ResultItem, error)) (taskstore.Task, error) {
	id, err := randomID()
	if err != nil {
		return taskstore.Task{}, err
	}
	return s.enqueueWithID(ctx, id, operation, message, run)
}

func (s *Service) enqueueWithID(ctx context.Context, id, operation, message string, run func(context.Context, func([]ResultItem) error) ([]ResultItem, error), completions ...chan taskstore.Task) (taskstore.Task, error) {
	s.cleanupMu.RLock()
	defer s.cleanupMu.RUnlock()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: id, Skill: Skill, Operation: operation, Status: "queued", Message: message, CreatedAt: now, UpdatedAt: now, Result: map[string]any{"request_id": id, "phase": "queued", "items": []ResultItem{}}}
	if err := s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	queued := task
	err := taskrunner.GoTask(s.runner, id, func(parent context.Context) {
		defer func() {
			for _, completion := range completions {
				if completion != nil {
					completion <- task
				}
			}
		}()
		runCtx, cancel := context.WithTimeout(parent, 2*time.Hour)
		defer cancel()
		update := func(rows []ResultItem) error {
			completed := 0
			for _, row := range rows {
				if row.Status != "queued" && row.Status != "running" && row.Status != "waiting_input" {
					completed++
				}
			}
			task.Status, task.Message = "running", "正在处理账号"
			for _, row := range rows {
				if row.Status == "waiting_input" {
					task.Status, task.Message = "waiting_input", "等待完成自动重新授权中的官方验证"
					break
				}
			}
			if len(rows) > 0 {
				task.Progress = completed * 100 / len(rows)
			}
			task.Result = map[string]any{"request_id": id, "phase": "processing", "items": rows}
			task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			return s.tasks.Save(runCtx, task)
		}
		rows, runErr := run(runCtx, update)
		task.Status, task.Progress, task.Message = "succeeded", 100, "账号处理完成"
		for _, row := range rows {
			if row.Status != "succeeded" && row.Status != "skipped" {
				task.Status, task.Message = "partial", "部分账号需要复核，请查看处理明细"
			}
		}
		if runErr != nil {
			task.Status, task.Message = "failed", publicError(runErr).Error()
		}
		if parent.Err() != nil {
			task.Status, task.Message = "cancelled", "任务已取消，请核对已处理账号"
		}
		task.Result = map[string]any{"request_id": id, "phase": "complete", "items": rows}
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		persist, done := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
		defer done()
		if err := s.tasks.Save(persist, task); err != nil {
			task.Status, task.Message = "failed", "账号操作结果保存失败，请核对线上账号和后台任务"
		}
	})
	if err != nil {
		task.Status, task.Message = "failed", "任务队列繁忙，请重新解析后重试"
		_ = s.tasks.Save(context.WithoutCancel(ctx), task)
		return taskstore.Task{}, err
	}
	return queued, nil
}
