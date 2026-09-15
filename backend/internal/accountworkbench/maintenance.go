package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskcontext"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type maintenanceStore interface {
	WorkbenchMaintenance(context.Context, string) (configstore.WorkbenchMaintenance, error)
	SaveWorkbenchMaintenance(context.Context, string, configstore.WorkbenchMaintenance) error
	WorkbenchMaintenanceRuntime(context.Context, string) (configstore.WorkbenchMaintenanceRuntime, error)
	SaveWorkbenchMaintenanceRuntime(context.Context, string, configstore.WorkbenchMaintenanceRuntime) error
}

type MaintenanceView struct {
	configstore.WorkbenchMaintenance
	PendingUploads          []MaintenancePendingUploadView `json:"pending_uploads"`
	ReauthorizationAttached bool                           `json:"reauthorization_attached"`
	LastRunAt               string                         `json:"last_run_at,omitempty"`
	LastTaskID              string                         `json:"last_task_id,omitempty"`
}

func (s *Service) Maintenance(ctx context.Context) (MaintenanceView, error) {
	var result MaintenanceView
	target, err := s.private.TargetSettings(ctx)
	if err != nil {
		return result, err
	}
	store, ok := s.private.(maintenanceStore)
	if !ok {
		return result, errors.New("自动维护存储尚未就绪")
	}
	result.WorkbenchMaintenance, err = store.WorkbenchMaintenance(ctx, target.BaseURL)
	if err != nil {
		return result, err
	}
	runtime, err := store.WorkbenchMaintenanceRuntime(ctx, target.BaseURL)
	result.LastRunAt, result.LastTaskID = runtime.LastRunAt, runtime.LastTaskID
	if result.Enabled && result.ReauthorizeWithProfiles {
		_, ownerErr := s.currentMaintenanceOwner(ctx, target, result.Revision)
		result.ReauthorizationAttached = ownerErr == nil
	}
	if err == nil {
		result.PendingUploads, err = s.pendingUploadViews(ctx, target, result.ReauthorizationAttached)
	}
	return result, err
}

func (s *Service) SaveMaintenance(ctx context.Context, input configstore.WorkbenchMaintenance) (MaintenanceView, error) {
	if input.IntervalMinutes < 1 || input.IntervalMinutes > 1440 || input.CooldownMinutes < 0 || input.CooldownMinutes > 1440 || input.Revision < 0 {
		return MaintenanceView{}, errors.New("维护间隔必须为 1～1440 分钟，冷却必须为 0～1440 分钟")
	}
	input.Model = strings.TrimSpace(input.Model)
	if input.Model == "" || len(input.Model) > 200 || len(input.GroupIDs) > 500 {
		return MaintenanceView{}, errors.New("维护模型或分组无效")
	}
	seen := map[string]bool{}
	for _, id := range input.GroupIDs {
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil || n < 1 || seen[id] {
			return MaintenanceView{}, errors.New("维护分组必须为不重复的稳定 ID")
		}
		seen[id] = true
	}
	if input.GroupIDs == nil {
		input.GroupIDs = []string{}
	}
	_, target, err := s.client(ctx)
	if err != nil {
		return MaintenanceView{}, err
	}
	store, ok := s.private.(maintenanceStore)
	if !ok {
		return MaintenanceView{}, errors.New("自动维护存储尚未就绪")
	}
	if err := store.SaveWorkbenchMaintenance(ctx, target.BaseURL, input); err != nil {
		return MaintenanceView{}, err
	}
	s.detachMaintenanceOwner()
	return s.Maintenance(ctx)
}

func (s *Service) CheckMaintenance(ctx context.Context, revision int64, confirmed bool) (taskstore.Task, error) {
	if !confirmed {
		return taskstore.Task{}, errors.New("请先确认维护影响范围")
	}
	_, target, err := s.client(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	store, ok := s.private.(maintenanceStore)
	if !ok {
		return taskstore.Task{}, errors.New("自动维护存储尚未就绪")
	}
	config, err := store.WorkbenchMaintenance(ctx, target.BaseURL)
	if err != nil {
		return taskstore.Task{}, err
	}
	if config.Revision != revision {
		return taskstore.Task{}, errors.New("自动维护配置已变化，请刷新后重试")
	}
	s.maintenanceMu.Lock()
	if s.maintenanceRunning {
		s.maintenanceMu.Unlock()
		return taskstore.Task{}, errors.New("账号维护正在运行，请等待当前任务完成")
	}
	s.maintenanceRunning = true
	s.maintenanceMu.Unlock()
	release := func() { s.maintenanceMu.Lock(); s.maintenanceRunning = false; s.maintenanceMu.Unlock() }
	task, err := s.enqueue(ctx, "account-workbench-maintenance", "等待维护 OAuth 账号", func(run context.Context, update func([]ResultItem) error) ([]ResultItem, error) {
		defer release()
		run = targetguard.Expect(run, target)
		if _, err := targetguard.Pin(run, s.private); err != nil {
			return nil, err
		}
		currentConfig, err := store.WorkbenchMaintenance(run, target.BaseURL)
		if err != nil || currentConfig.Revision != config.Revision {
			return nil, errors.New("自动维护配置已变化，已停止旧任务")
		}
		client, err := s.clientFor(target)
		if err != nil {
			return nil, err
		}
		rows, pendingAccounts, err := s.processMaintenanceUploads(run, target, config, update)
		if err != nil {
			return rows, err
		}
		accounts, err := client.Accounts(run)
		if err != nil {
			return nil, err
		}
		runtime, err := store.WorkbenchMaintenanceRuntime(run, target.BaseURL)
		if err != nil {
			return nil, err
		}
		runtime.LastTaskID = taskcontext.ID(run)
		if err := store.SaveWorkbenchMaintenanceRuntime(run, target.BaseURL, runtime); err != nil {
			return nil, err
		}
		for _, account := range accounts {
			if run.Err() != nil {
				return rows, run.Err()
			}
			if err := s.validateMaintenanceWrite(run, target.BaseURL, config.Revision, ""); err != nil {
				return rows, err
			}
			if !maintenanceAccountSelected(account, config.GroupIDs) {
				continue
			}
			id := stringValue(account["id"])
			if pendingAccounts[id] {
				continue
			}
			row := ResultItem{Index: len(rows), Name: exportAccountName(account, target.AdminKey), AccountID: id, Status: "skipped", Message: "账号无需修复"}
			now := time.Now()
			until, _ := time.Parse(time.RFC3339, runtime.Cooldowns[id])
			if now.Before(until) {
				row.Message = "账号处于冷却期，本轮不重复修复"
			} else {
				delete(runtime.Cooldowns, id)
				action := maintenanceAction(account, now, time.Duration(config.IntervalMinutes)*time.Minute)
				switch action {
				case "manual":
					row.Status, row.Message = "review", "账号需要人工重新授权或权限处理"
				case "cooldown":
					row.Message = "上游限流或临时异常，等待冷却后复核"
					runtime.Cooldowns[id] = now.Add(time.Duration(config.CooldownMinutes) * time.Minute).UTC().Format(time.RFC3339)
				case "refresh":
					row.Status, row.Message = "running", "正在刷新授权"
					if err := update(append(rows, row)); err != nil {
						return rows, err
					}
					row = s.repairAccount(run, account, config, row)
					if row.Status != "succeeded" {
						runtime.Cooldowns[id] = now.Add(time.Duration(config.CooldownMinutes) * time.Minute).UTC().Format(time.RFC3339)
					}
				}
			}
			rows = append(rows, row)
			if err := update(rows); err != nil {
				return rows, err
			}
			if err := store.SaveWorkbenchMaintenanceRuntime(run, target.BaseURL, runtime); err != nil {
				return rows, err
			}
		}
		if config.Enabled && config.ReauthorizeWithProfiles {
			rows, err = s.maintainReauthorization(run, target, config, rows, pendingAccounts, update)
			if err != nil {
				return rows, err
			}
		}
		runtime.LastRunAt = time.Now().UTC().Format(time.RFC3339)
		if err := store.SaveWorkbenchMaintenanceRuntime(run, target.BaseURL, runtime); err != nil {
			return rows, err
		}
		if s.syncAccounts != nil {
			if _, err := s.syncAccounts(run, "account-workbench"); err != nil {
				return rows, err
			}
		}
		return rows, nil
	})
	if err != nil {
		release()
		return task, err
	}
	return task, nil
}

func (s *Service) repairAccount(ctx context.Context, original map[string]any, config configstore.WorkbenchMaintenance, row ResultItem) ResultItem {
	row.Status = "review"
	guarded, release, err := targetguard.Acquire(ctx, s.repository, mutationguard.Account(row.AccountID))
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
	target, err := targetguard.Settings(guarded, s.private)
	if err != nil {
		row.Message = publicError(err).Error()
		return row
	}
	validate := func() error {
		return s.validateMaintenanceWrite(guarded, target.BaseURL, config.Revision, row.AccountID)
	}
	if err := validate(); err != nil {
		row.Message = err.Error()
		return row
	}
	client, err := s.clientFor(target)
	if err != nil {
		row.Message = publicError(err).Error()
		return row
	}
	before, err := client.Account(guarded, row.AccountID)
	if err != nil || !sameAccountConfiguration(original, before) {
		row.Message = "账号在排队期间已变化，本轮跳过"
		return row
	}
	if err := s.audit(guarded, row.AccountID, "workbench.refresh", "started", false); err != nil {
		row.Message = "审计保存失败，未刷新授权"
		return row
	}
	if config.CheckAfterRepair {
		if err := validate(); err != nil {
			row.Message = err.Error()
			return row
		}
		if _, err := client.SetAccountSchedulable(guarded, row.AccountID, false); err != nil {
			row.Message = publicError(err).Error()
			return row
		}
		paused, err := client.Account(guarded, row.AccountID)
		if err != nil || paused["schedulable"] != false {
			row.Message = "暂停调度未读回确认，未刷新授权"
			return row
		}
		expected := copyObject(before)
		expected["schedulable"] = false
		if !sameAccountConfiguration(expected, paused) {
			row.Message = "暂停期间账号配置发生变化，未刷新授权"
			return row
		}
	}
	if err := validate(); err != nil {
		row.Message = err.Error()
		return row
	}
	_, err = client.Mutate(guarded, http.MethodPost, "/admin/openai/accounts/"+row.AccountID+"/refresh", nil)
	if err != nil {
		row.Message = "授权刷新失败，请重新授权后再维护"
		return row
	}
	after, err := client.Account(guarded, row.AccountID)
	if err != nil {
		row.Message = "授权刷新后读回失败，请核对账号"
		return row
	}
	credentials, _ := after["credentials"].(map[string]any)
	if config.CheckAfterRepair && after["schedulable"] != false {
		row.Message = "刷新后的暂停状态未保持，请立即核对线上调度设置"
		return row
	}
	if stringValue(credentials["access_token"]) == "" {
		row.Message = "刷新后未获得有效授权，请重新授权"
		return row
	}
	beforeCredentials, _ := before["credentials"].(map[string]any)
	if !sameCredentialIdentity(beforeCredentials, credentials) {
		row.Message = "刷新后的账号身份发生变化，已停止自动恢复"
		return row
	}
	if err := s.audit(guarded, row.AccountID, "workbench.refresh", "succeeded", true); err != nil {
		row.Message = "授权已刷新，但审计保存失败，未恢复调度"
		return row
	}
	_ = release()
	release = nil
	if !config.CheckAfterRepair {
		row.Status, row.Message = "succeeded", "授权已刷新，保留原有调度状态"
		return row
	}
	if s.checker == nil {
		row.Message = "授权已刷新，行为检测服务尚未就绪"
		return row
	}
	report, err := s.checker.CheckOAuth(ctx, row.AccountID, row.Name, credentials, config.Model, 60)
	row.Report = publicReport(report)
	for _, key := range []string{"access_token", "refresh_token", "id_token"} {
		if secret := stringValue(credentials[key]); secret != "" {
			scrubReportSecret(row.Report, secret)
		}
	}
	if err != nil || report["verdict"] != "SOL_CONSISTENT" {
		row.Message = "授权已刷新，行为检测需要复核，未自动恢复调度"
		return row
	}
	guarded, release, err = targetguard.Acquire(ctx, s.repository, mutationguard.Account(row.AccountID))
	if err == nil {
		guarded, err = targetguard.Bind(guarded, s.private)
	}
	if err != nil {
		row.Message = "检测后管理目标发生变化，未自动恢复"
		return row
	}
	if err := validate(); err != nil {
		row.Message = err.Error()
		return row
	}
	current, err := client.Account(guarded, row.AccountID)
	if err != nil || !sameAccountConfiguration(after, current) {
		row.Message = "检测期间账号配置发生变化，未自动恢复"
		return row
	}
	if err := s.audit(guarded, row.AccountID, "workbench.recover", "started", false); err != nil {
		row.Message = "审计保存失败，未自动恢复调度"
		return row
	}
	if err := validate(); err != nil {
		row.Message = err.Error()
		return row
	}
	if _, err := client.Mutate(guarded, http.MethodPost, "/admin/accounts/"+row.AccountID+"/clear-error", nil); err != nil {
		row.Message = publicError(err).Error()
		return row
	}
	if err := validate(); err != nil {
		row.Message = err.Error()
		return row
	}
	if _, err := client.RecoverAccountState(guarded, row.AccountID); err != nil {
		row.Message = publicError(err).Error()
		return row
	}
	if err := validate(); err != nil {
		row.Message = err.Error()
		return row
	}
	if _, err := client.UpdateAccount(guarded, row.AccountID, map[string]any{"status": "active"}); err != nil {
		row.Message = publicError(err).Error()
		return row
	}
	ready, err := client.Account(guarded, row.AccountID)
	if err != nil || ready["status"] != "active" || ready["schedulable"] != false || !accountRuntimeReady(ready, time.Now()) || !sameAccountStaticConfiguration(after, ready) {
		row.Message = "恢复前账号状态未读回确认，保持不可调度"
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
	if err != nil || confirmed["schedulable"] != true || confirmed["status"] != "active" || !accountRuntimeReady(confirmed, time.Now()) || !sameAccountStaticConfiguration(after, confirmed) {
		row.Message = "恢复状态尚未确认，请核对线上账号"
		return row
	}
	if err := s.audit(guarded, row.AccountID, "workbench.recover", "succeeded", true); err != nil {
		row.Message = "已恢复调度，但审计保存失败"
		return row
	}
	row.Status, row.Message = "succeeded", "授权已刷新，行为检测通过并确认恢复调度"
	return row
}

func sameCredentialIdentity(before, after map[string]any) bool {
	previous, current := inputIdentity(before), inputIdentity(after)
	if previous.workspace != "" && previous.workspace != current.workspace {
		return false
	}
	if previous.user != "" && previous.user != current.user {
		return false
	}
	return previous.workspace != "" || previous.user != ""
}

func (s *Service) validateMaintenanceWrite(ctx context.Context, target string, revision int64, accountID string) error {
	if _, err := targetguard.Pin(ctx, s.private); err != nil {
		return err
	}
	store, ok := s.private.(maintenanceStore)
	if !ok {
		return errors.New("自动维护存储尚未就绪")
	}
	current, err := store.WorkbenchMaintenance(ctx, target)
	if err != nil || current.Revision != revision {
		return errors.New("自动维护配置已变化，已停止旧任务")
	}
	if accountID != "" {
		return s.checkProtection(ctx, accountID)
	}
	return ctx.Err()
}

var transientFailure = regexp.MustCompile(`(?i)\b429\b|rate.?limit|too many requests|quota|overload|timeout|timed out|network|connection|\b50[234]\b`)
var permanentFailure = regexp.MustCompile(`(?i)invalid_grant|refresh[_ -]?token.*(?:invalid|revoked|expired)|deactivated|suspended|forbidden|\b403\b`)
var authFailure = regexp.MustCompile(`(?i)\b401\b|unauthori[sz]ed|access[_ -]?token.*(?:invalid|expired)|token expired|authentication`)

func maintenanceAction(account map[string]any, now time.Time, window time.Duration) string {
	text := strings.Join([]string{stringValue(account["error_message"]), stringValue(account["error"]), stringValue(account["temp_unschedulable_reason"])}, " ")
	if (account["status"] == "inactive" || account["schedulable"] == false) && strings.TrimSpace(text) == "" {
		return "none"
	}
	if permanentFailure.MatchString(text) {
		return "manual"
	}
	if transientFailure.MatchString(text) {
		return "cooldown"
	}
	if authFailure.MatchString(text) {
		return "refresh"
	}
	credentials, _ := account["credentials"].(map[string]any)
	expiry := credentialExpiry(credentials["expires_at"])
	if !expiry.IsZero() && !expiry.After(now.Add(window)) {
		return "refresh"
	}
	if strings.TrimSpace(text) != "" || account["status"] == "error" {
		return "manual"
	}
	return "none"
}

func credentialExpiry(value any) time.Time {
	text := stringValue(value)
	if expiry, err := time.Parse(time.RFC3339, text); err == nil {
		return expiry
	}
	if seconds, err := strconv.ParseInt(text, 10, 64); err == nil && seconds > 0 {
		return time.Unix(seconds, 0)
	}
	return time.Time{}
}

func maintenanceAccountSelected(account map[string]any, groups []string) bool {
	if account["platform"] != "openai" || account["type"] != "oauth" {
		return false
	}
	if len(groups) == 0 {
		return true
	}
	if raw, err := json.Marshal(account["group_ids"]); err == nil {
		for _, id := range configIDs(raw) {
			if slices.Contains(groups, id) {
				return true
			}
		}
	}
	if values, ok := account["groups"].([]any); ok {
		for _, value := range values {
			if group, ok := value.(map[string]any); ok && slices.Contains(groups, stringValue(group["id"])) {
				return true
			}
		}
	}
	return false
}

// RunScheduler uses the server lifecycle context; restart waits a complete interval.
func (s *Service) RunScheduler(ctx context.Context) {
	defer s.detachMaintenanceOwner()
	ticks := s.maintenanceTicks
	if ticks == nil {
		ticker := time.NewTicker(time.Second * 30)
		defer ticker.Stop()
		ticks = ticker.C
	}
	var scheduledKey string
	next := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case now, open := <-ticks:
			if !open {
				return
			}
			if cleaner, ok := s.private.(interface {
				PurgeExpiredWorkbenchQueues(context.Context, time.Time) error
			}); ok {
				if err := cleaner.PurgeExpiredWorkbenchQueues(ctx, now); err != nil && ctx.Err() == nil {
					slog.Error("账号工作台私有批次恢复资料清理失败")
				}
			}
			if cleaner, ok := s.private.(interface {
				PurgeExpiredWorkbenchOAuthCheckpoints(context.Context, time.Time) error
			}); ok {
				if err := cleaner.PurgeExpiredWorkbenchOAuthCheckpoints(ctx, now); err != nil && ctx.Err() == nil {
					slog.Error("账号工作台授权检查点清理失败")
				}
			}
			if cleaner, ok := s.private.(interface{ PurgeExpiredWorkbenchExecutions(context.Context) error }); ok {
				if err := cleaner.PurgeExpiredWorkbenchExecutions(ctx); err != nil && ctx.Err() == nil {
					slog.Error("账号工作台私有执行记录清理失败")
				}
			}
			config, err := s.Maintenance(ctx)
			if err != nil || !config.Enabled {
				scheduledKey = ""
				continue
			}
			target, err := s.private.TargetSettings(ctx)
			if err != nil {
				continue
			}
			key := fmt.Sprintf("%s:%d", target.BaseURL, config.Revision)
			if key != scheduledKey {
				scheduledKey = key
				next = now.Add(time.Duration(config.IntervalMinutes) * time.Minute)
				continue
			}
			if now.Before(next) {
				continue
			}
			if _, err := s.CheckMaintenance(ctx, config.Revision, true); err == nil {
				next = now.Add(time.Duration(config.IntervalMinutes) * time.Minute)
			}
		}
	}
}

// UseMaintenanceTicks replaces the scheduler clock in isolated tests.
func (s *Service) UseMaintenanceTicks(ticks <-chan time.Time) { s.maintenanceTicks = ticks }
