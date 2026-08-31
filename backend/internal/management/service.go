package management

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type TargetStore interface {
	TargetSettings(context.Context) (configstore.TargetSettings, error)
	AccountDefaults(context.Context) (configstore.AccountDefaultsSettings, error)
}

type Repository interface {
	SyncManagementSnapshot(context.Context, []map[string]any, []map[string]any, string) (business.ManagementSyncResult, error)
	CommitAccountBaseURLObservations(context.Context, []business.AccountBaseURLObservation) error
	BoundAccountsForMaintenance(context.Context, []string) ([]business.BoundAccountMaintenance, error)
	CommitAccountRateObservations(context.Context, []business.AccountRateObservation) error
	CommitBindingVerification(context.Context, []business.BindingVerification) error
	CommitAccountNameRepairs(context.Context, []business.AccountNameRepairCommit) error
	CommitAccountDefaultsRepairs(context.Context, []business.AccountDefaultsRepairCommit, string) error
	RepairAccountUpstreamHosts(context.Context, []string, string) (business.AccountUpstreamHostRepairResult, error)
	CleanupMissingBindings(context.Context, []string, string) (business.MissingBindingCleanupResult, error)
	ManualPriorityControls(context.Context, []string) (map[string]business.ManualPriorityControl, error)
}

type TaskStore interface {
	Save(context.Context, taskstore.Task) error
}

type AccountRateWriter interface {
	SyncAccountRate(context.Context, string, string, string, string) (map[string]any, error)
	SyncAccountMultiplier(context.Context, string, string, string) (map[string]any, error)
}

type UpstreamCatalogReader interface {
	ReadCatalog(context.Context, configstore.AuthRecord) (business.UpstreamCatalogSnapshot, error)
}

type upstreamAuthStore interface {
	AuthRecord(context.Context, string) (*configstore.AuthRecord, error)
}

type accountNameRepository interface {
	AccountNamesForMaintenance(context.Context, []string) (map[string]string, error)
}

type UpstreamAuthResolver interface {
	ResolveAuth(context.Context, string, string) (*configstore.AuthRecord, error)
}

type Service struct {
	targets    TargetStore
	repository Repository
	tasks      TaskStore
	rateWriter AccountRateWriter
	upstreams  UpstreamCatalogReader
	resolver   UpstreamAuthResolver
	timeout    time.Duration
}

func (s *Service) UseUpstreamCatalogReader(reader UpstreamCatalogReader) {
	s.upstreams = reader
}

func (s *Service) UseUpstreamAuthResolver(resolver UpstreamAuthResolver) {
	s.resolver = resolver
}

func New(targets TargetStore, repository Repository, tasks TaskStore, rateWriters ...AccountRateWriter) *Service {
	service := &Service{targets: targets, repository: repository, tasks: tasks, timeout: 10 * time.Minute}
	if len(rateWriters) > 0 {
		service.rateWriter = rateWriters[0]
	}
	return service
}

func (s *Service) EnqueueSync(ctx context.Context, actor string) (taskstore.Task, error) {
	id, err := managementTaskID()
	if err != nil {
		return taskstore.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{
		ID: id, Skill: "sub2api-operations", Operation: "management-snapshot-sync", Status: "queued", Progress: 0,
		Message: "管理快照同步已排队", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	go s.execute(task, actor)
	return task, nil
}

func (s *Service) EnqueueAccountRevalidation(ctx context.Context, accountIDs []string, actor string) (taskstore.Task, error) {
	return s.enqueueMaintenance(ctx, "account-binding-revalidation", "账号批量复验已排队", accountIDs, actor)
}

func (s *Service) EnqueueAccountBaseURLValidation(ctx context.Context, accountIDs []string, actor string) (taskstore.Task, error) {
	return s.enqueueMaintenance(ctx, "account-base-url-validation", "账号 Base URL 校验已排队", accountIDs, actor)
}

func (s *Service) EnqueueAccountConfigurationCheck(ctx context.Context, accountIDs []string, actor string) (taskstore.Task, error) {
	return s.enqueueMaintenance(ctx, "account-configuration-check", "账号配置校验与修复已排队", accountIDs, actor)
}

func (s *Service) EnqueueAccountBaseURLRepair(ctx context.Context, accountIDs []string, actor string) (taskstore.Task, error) {
	return s.enqueueMaintenance(ctx, "account-base-url-repair", "账号配置与状态修复已排队", accountIDs, actor)
}

func (s *Service) EnqueueAccountUpstreamHostRepair(ctx context.Context, accountIDs []string, actor string) (taskstore.Task, error) {
	return s.enqueueMaintenance(ctx, "account-upstream-host-repair", "账号归属 Host 修复已排队", accountIDs, actor)
}

func (s *Service) EnqueueAccountRateSync(ctx context.Context, accountIDs []string, actor string) (taskstore.Task, error) {
	return s.enqueueMaintenance(ctx, "account-rate-sync", "账号倍率同步已排队", accountIDs, actor)
}

func (s *Service) SyncAllAccountRates(ctx context.Context, actor string) (map[string]any, error) {
	return s.syncAccountRates(ctx, nil, actor)
}

func (s *Service) EnqueueAccountNameRepair(ctx context.Context, accountIDs []string, actor string) (taskstore.Task, error) {
	return s.enqueueMaintenance(ctx, "account-name-repair", "账号命名修复已排队", accountIDs, actor)
}

func (s *Service) EnqueueAccountDefaultsRepair(ctx context.Context, accountIDs []string, actor string) (taskstore.Task, error) {
	return s.enqueueMaintenance(ctx, "account-defaults-repair", "账号默认参数修复已排队", accountIDs, actor)
}

func (s *Service) EnqueueMissingBindingCleanup(ctx context.Context, accountIDs []string, actor string) (taskstore.Task, error) {
	return s.enqueueMaintenance(ctx, "account-missing-binding-cleanup", "失效绑定修复已排队", accountIDs, actor)
}

func (s *Service) enqueueMaintenance(ctx context.Context, operation, message string, accountIDs []string, actor string) (taskstore.Task, error) {
	accountIDs, err := normalizeAccountIDs(accountIDs)
	if err != nil {
		return taskstore.Task{}, err
	}
	id, err := managementTaskID()
	if err != nil {
		return taskstore.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: id, Skill: "sub2api-operations", Operation: operation, Status: "queued", Progress: 0,
		Message: message, Result: map[string]any{"requested": len(accountIDs)}, CreatedAt: now, UpdatedAt: now}
	if err := s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	go s.executeMaintenance(task, operation, accountIDs, actor)
	return task, nil
}

func (s *Service) executeMaintenance(task taskstore.Task, operation string, accountIDs []string, actor string) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	runningMessage := "读取管理平台账号目录"
	if operation == "account-rate-sync" {
		runningMessage = "正在从上游探测账号有效倍率并写回管理平台"
	} else if operation == "account-base-url-validation" {
		runningMessage = "正在读取管理平台账号详情中的 Base URL"
	} else if operation == "account-configuration-check" {
		runningMessage = "正在校验 Base URL 并修复错误开户参数"
	} else if operation == "account-base-url-repair" {
		runningMessage = "正在修复 Base URL、恢复账号状态并开启调度"
	} else if operation == "account-upstream-host-repair" {
		runningMessage = "正在根据账号绑定修复归属 Host"
	} else if operation == "account-defaults-repair" {
		runningMessage = "正在核对并修复控制台开户账号的默认参数"
	}
	task.Status, task.Progress, task.Message, task.UpdatedAt = "running", 10, runningMessage, time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.tasks.Save(ctx, task); err != nil {
		return
	}
	var result map[string]any
	var err error
	requested := len(accountIDs)
	protected := []map[string]any{}
	if operation != "account-rate-sync" {
		accountIDs, protected, err = s.excludeManualPriorityAccounts(ctx, accountIDs)
	}
	if err == nil {
		if len(accountIDs) == 0 && len(protected) > 0 {
			result = manualPriorityOnlyMaintenanceResult(operation, requested, protected, actor)
		} else {
			result, err = s.runMaintenance(ctx, operation, accountIDs, actor)
			if result != nil && len(protected) > 0 {
				mergeManualPrioritySkips(result, requested, protected)
			}
		}
	}
	task.Progress, task.UpdatedAt = 100, time.Now().UTC().Format(time.RFC3339Nano)
	if err != nil {
		task.Status, task.Message = "failed", err.Error()
		if result == nil {
			result = map[string]any{}
		}
		result["error"] = err.Error()
	} else {
		task.Status = "succeeded"
		if operation == "account-rate-sync" {
			task.Message = fmt.Sprintf("账号倍率同步完成：更新 %v 个，未变 %v 个，缺失 %v 个，失败 %v 个", result["updated"], result["unchanged"], result["missing"], result["failed"])
			if failed, _ := result["failed"].(int); failed > 0 {
				task.Status = "failed"
				task.Message = fmt.Sprintf("账号倍率同步部分失败：更新 %v 个，未变 %v 个，缺失 %v 个，失败 %v 个", result["updated"], result["unchanged"], result["missing"], result["failed"])
			}
		} else if operation == "account-base-url-validation" {
			task.Message = fmt.Sprintf("Base URL 校验完成：已读取 %v 个，未返回 %v 个，失败 %v 个", result["resolved"], result["unavailable"], result["failed"])
		} else if operation == "account-configuration-check" {
			task.Message = fmt.Sprintf("配置校验完成：Base URL 已读取 %v 个；参数已修复 %v 个，无需修复 %v 个，跳过 %v 个，失败 %v 个",
				result["base_url_resolved"], result["parameters_repaired"], result["parameters_unchanged"], result["parameters_skipped"], result["failed"])
			if failed, _ := result["failed"].(int); failed > 0 {
				task.Status = "failed"
			}
		} else if operation == "account-base-url-repair" {
			task.Message = fmt.Sprintf("账号配置与状态修复完成：已修复 %v 个，未变 %v 个，跳过 %v 个，失败 %v 个", result["repaired"], result["unchanged"], result["skipped"], result["failed"])
			if failed, _ := result["failed"].(int); failed > 0 {
				task.Status = "failed"
			}
		} else if operation == "account-upstream-host-repair" {
			task.Message = fmt.Sprintf("归属 Host 修复完成：已修复 %v 个，无需修复 %v 个，跳过 %v 个", result["repaired"], result["unchanged"], result["skipped"])
		} else if operation == "account-name-repair" {
			task.Message = fmt.Sprintf("命名修复完成：已修复 %v 个", result["renamed"])
		} else if operation == "account-defaults-repair" {
			task.Message = fmt.Sprintf("默认参数修复完成：已修复 %v 个，无需修复 %v 个，跳过 %v 个，失败 %v 个", result["repaired"], result["unchanged"], result["skipped"], result["failed"])
			if failed, _ := result["failed"].(int); failed > 0 {
				task.Status = "failed"
			}
		} else if operation == "account-missing-binding-cleanup" {
			task.Message = fmt.Sprintf("失效绑定修复完成：已清理 %v 个", result["cleaned"])
		} else {
			task.Message = fmt.Sprintf("批量复验完成：存在 %v 个，缺失 %v 个", result["verified"], result["missing"])
		}
	}
	task.Result = result
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) runMaintenance(ctx context.Context, operation string, accountIDs []string, actor string) (map[string]any, error) {
	switch operation {
	case "account-rate-sync":
		return s.syncAccountRates(ctx, accountIDs, actor)
	case "account-base-url-validation":
		return s.validateAccountBaseURLs(ctx, accountIDs, actor)
	case "account-configuration-check":
		return s.checkAccountConfiguration(ctx, accountIDs, actor)
	case "account-base-url-repair":
		return s.repairAccountBaseURLs(ctx, accountIDs, actor)
	case "account-upstream-host-repair":
		return s.repairAccountUpstreamHosts(ctx, accountIDs, actor)
	case "account-name-repair":
		return s.repairAccountNames(ctx, accountIDs, actor)
	case "account-defaults-repair":
		return s.repairAccountDefaults(ctx, accountIDs, actor)
	case "account-missing-binding-cleanup":
		return s.cleanupMissingBindings(ctx, accountIDs, actor)
	default:
		return s.revalidateAccounts(ctx, accountIDs, actor)
	}
}

func (s *Service) excludeManualPriorityAccounts(ctx context.Context, accountIDs []string) ([]string, []map[string]any, error) {
	controls, err := s.repository.ManualPriorityControls(ctx, accountIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("人工优先位保护状态读取失败：%w", err)
	}
	eligible := make([]string, 0, len(accountIDs))
	protected := make([]map[string]any, 0, len(controls))
	for _, accountID := range accountIDs {
		if _, found := controls[accountID]; !found {
			eligible = append(eligible, accountID)
			continue
		}
		protected = append(protected, map[string]any{
			"account_id":   accountID,
			"status":       "人工优先位，已跳过",
			"reason":       "人工控制账号仅允许按设置同步余额与倍率",
			"remote_write": false,
		})
	}
	return eligible, protected, nil
}

func manualPriorityOnlyMaintenanceResult(operation string, requested int, protected []map[string]any, actor string) map[string]any {
	result := map[string]any{
		"operation": operation, "requested": requested, "skipped": len(protected), "items": protected,
		"actor": actor, "remote_write": false,
	}
	switch operation {
	case "account-base-url-validation":
		result["resolved"], result["unavailable"], result["failed"] = 0, 0, 0
		result["read_only"] = true
	case "account-configuration-check":
		result["base_url_resolved"], result["base_url_unavailable"], result["base_url_failed"] = 0, 0, 0
		result["parameters_repaired"], result["parameters_unchanged"], result["parameters_skipped"] = 0, 0, len(protected)
		result["parameters_failed"], result["failed"] = 0, 0
	case "account-base-url-repair", "account-defaults-repair":
		result["repaired"], result["unchanged"], result["failed"] = 0, 0, 0
	case "account-upstream-host-repair":
		result["repaired"], result["unchanged"] = 0, 0
		items := make([]business.AccountUpstreamHostRepairItem, 0, len(protected))
		for _, item := range protected {
			reason, _ := item["reason"].(string)
			items = append(items, business.AccountUpstreamHostRepairItem{
				AccountID: item["account_id"].(string), Status: item["status"].(string), Reason: &reason,
			})
		}
		result["items"] = items
	case "account-name-repair":
		result["renamed"], result["unchanged"], result["missing"], result["failed"] = 0, 0, 0, 0
	case "account-missing-binding-cleanup":
		result["bound"], result["cleaned"] = 0, 0
	default:
		result["bound"], result["verified"], result["missing"] = 0, 0, 0
	}
	return result
}

func mergeManualPrioritySkips(result map[string]any, requested int, protected []map[string]any) {
	result["requested"] = requested
	skipped := resultInteger(result, "skipped") + len(protected)
	result["skipped"] = skipped
	if _, present := result["parameters_skipped"]; present {
		result["parameters_skipped"] = resultInteger(result, "parameters_skipped") + len(protected)
	}
	switch items := result["items"].(type) {
	case []map[string]any:
		result["items"] = append(items, protected...)
	case []business.AccountUpstreamHostRepairItem:
		for _, item := range protected {
			reason, _ := item["reason"].(string)
			items = append(items, business.AccountUpstreamHostRepairItem{
				AccountID: item["account_id"].(string), Status: item["status"].(string), Reason: &reason,
			})
		}
		result["items"] = items
	}
}

func (s *Service) repairAccountUpstreamHosts(ctx context.Context, accountIDs []string, actor string) (map[string]any, error) {
	result, err := s.repository.RepairAccountUpstreamHosts(ctx, accountIDs, actor)
	if err != nil {
		return nil, fmt.Errorf("账号归属 Host 修复失败：%w", err)
	}
	return map[string]any{
		"operation": "account.upstream_host.repair", "requested": result.Requested,
		"repaired": result.Repaired, "unchanged": result.Unchanged, "skipped": result.Skipped,
		"items": result.Items, "event_id": result.EventID, "actor": actor, "remote_write": false,
	}, nil
}

func (s *Service) validateAccountBaseURLs(ctx context.Context, accountIDs []string, actor string) (map[string]any, error) {
	client, err := s.maintenanceClient(ctx)
	if err != nil {
		return nil, err
	}
	type validationResult struct {
		row       map[string]any
		item      map[string]any
		resolved  bool
		available bool
		source    string
	}
	results := make([]validationResult, len(accountIDs))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(8, len(accountIDs)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				accountID := accountIDs[index]
				item := map[string]any{"account_id": accountID, "status": "详情读取失败"}
				row, readErr := client.Account(ctx, accountID)
				if readErr != nil {
					item["error"] = readErr.Error()
					results[index].item = item
					continue
				}
				item["account_name"] = strings.TrimSpace(fmt.Sprint(firstValue(row, "name")))
				baseURL, source, available := managementRowBaseURL(row)
				item["status"] = "详情未返回 Base URL"
				if available {
					item["status"], item["base_url"], item["source"] = "已读取", baseURL, source
				}
				results[index] = validationResult{row: row, item: item, resolved: true, available: available, source: source}
			}
		}()
	}
	for index := range accountIDs {
		jobs <- index
	}
	close(jobs)
	workers.Wait()

	observations := make([]business.AccountBaseURLObservation, 0, len(results))
	items := make([]map[string]any, 0, len(results))
	resolved, unavailable, failed := 0, 0, 0
	for _, result := range results {
		items = append(items, result.item)
		if !result.resolved {
			failed++
			continue
		}
		observation := business.AccountBaseURLObservation{AccountID: strings.TrimSpace(fmt.Sprint(firstValue(result.row, "id", "account_id")))}
		if result.available {
			resolved++
			baseURL, _, _ := managementRowBaseURL(result.row)
			observation.BaseURL = &baseURL
			observation.Source = result.source
		} else {
			unavailable++
		}
		observations = append(observations, observation)
	}
	if len(observations) > 0 {
		if err := s.repository.CommitAccountBaseURLObservations(ctx, observations); err != nil {
			return nil, fmt.Errorf("Base URL 校验结果保存失败：%w", err)
		}
	}
	return map[string]any{
		"operation": "account.base_url.validation", "requested": len(accountIDs),
		"resolved": resolved, "unavailable": unavailable, "failed": failed,
		"items": items, "actor": actor, "remote_write": false, "read_only": true,
	}, nil
}

func (s *Service) checkAccountConfiguration(ctx context.Context, accountIDs []string, actor string) (map[string]any, error) {
	baseURLResult, baseURLErr := s.validateAccountBaseURLs(ctx, accountIDs, actor)
	defaultsResult, defaultsErr := s.repairAccountDefaults(ctx, accountIDs, actor)
	result := map[string]any{
		"operation": "account.configuration.check", "requested": len(accountIDs), "actor": actor,
		"base_url": baseURLResult, "parameters": defaultsResult,
	}
	if baseURLResult != nil {
		result["base_url_resolved"] = resultInteger(baseURLResult, "resolved")
		result["base_url_unavailable"] = resultInteger(baseURLResult, "unavailable")
		result["base_url_failed"] = resultInteger(baseURLResult, "failed")
	}
	if defaultsResult != nil {
		result["parameters_repaired"] = resultInteger(defaultsResult, "repaired")
		result["parameters_unchanged"] = resultInteger(defaultsResult, "unchanged")
		result["parameters_skipped"] = resultInteger(defaultsResult, "skipped")
		result["parameters_failed"] = resultInteger(defaultsResult, "failed")
		result["remote_write"] = defaultsResult["remote_write"]
	}
	failed := resultInteger(result, "base_url_failed") + resultInteger(result, "parameters_failed")
	if baseURLErr != nil {
		failed++
		result["base_url_error"] = baseURLErr.Error()
	}
	if defaultsErr != nil && resultInteger(result, "parameters_failed") == 0 {
		failed++
		result["parameters_error"] = defaultsErr.Error()
	}
	result["failed"] = failed
	if baseURLErr != nil || defaultsErr != nil {
		return result, errors.Join(baseURLErr, defaultsErr)
	}
	return result, nil
}

func resultInteger(result map[string]any, key string) int {
	if result == nil {
		return 0
	}
	value, _ := result[key].(int)
	return value
}

func (s *Service) repairAccountBaseURLs(ctx context.Context, accountIDs []string, actor string) (map[string]any, error) {
	bound, err := s.repository.BoundAccountsForMaintenance(ctx, accountIDs)
	if err != nil {
		return nil, err
	}
	client, err := s.maintenanceClient(ctx)
	if err != nil {
		return nil, err
	}
	boundByID := make(map[string][]business.BoundAccountMaintenance, len(bound))
	for _, account := range bound {
		boundByID[account.AccountID] = append(boundByID[account.AccountID], account)
	}
	type repairResult struct {
		item        map[string]any
		kind        string
		observation *business.AccountBaseURLObservation
		written     bool
	}
	results := make([]repairResult, len(accountIDs))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(4, len(accountIDs)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				accountID := accountIDs[index]
				bindings := boundByID[accountID]
				item := map[string]any{"account_id": accountID, "remote_write": false, "readback_confirmed": false}
				targets, hosts := map[string]string{}, map[string]struct{}{}
				for _, account := range bindings {
					item["account_name"] = account.AccountName
					host := strings.TrimSpace(account.UpstreamHost)
					if host != "" {
						hosts[strings.ToLower(host)] = struct{}{}
						item["upstream_host"] = host
					}
					if target, ok := validAccountRepairBaseURL(account.NamingBaseURL); ok {
						targets[comparableBaseURL(target)] = target
					}
				}
				switch {
				case len(bindings) == 0:
					item["status"] = "没有绑定，无法修复"
					results[index] = repairResult{item: item, kind: "skipped"}
					continue
				case len(hosts) != 1 || len(targets) != 1:
					item["status"] = "归属上游不唯一，无法自动修复"
					results[index] = repairResult{item: item, kind: "skipped"}
					continue
				}
				var target string
				for _, value := range targets {
					target = value
				}
				item["after"] = target
				row, readErr := client.Account(ctx, accountID)
				if readErr != nil {
					item["status"], item["error"] = "账号详情读取失败", readErr.Error()
					results[index] = repairResult{item: item, kind: "failed"}
					continue
				}
				current, source, available := managementRowBaseURL(row)
				if available {
					item["before"] = current
				}
				explicitTarget := source == "explicit" && sameBaseURL(current, target)
				if source == "explicit" && !explicitTarget {
					item["status"] = "已有显式 Base URL，未覆盖"
					results[index] = repairResult{item: item, kind: "skipped",
						observation: &business.AccountBaseURLObservation{AccountID: accountID, BaseURL: &current, Source: source}}
					continue
				}
				if !explicitTarget && (!available || source != "platform_default") {
					item["status"] = "账号类型未提供可修复的默认 Base URL"
					results[index] = repairResult{item: item, kind: "skipped"}
					continue
				}
				remoteWritten := false
				if !explicitTarget {
					if _, updateErr := client.UpdateAccount(ctx, accountID, map[string]any{
						"credentials": map[string]any{"base_url": target},
					}); updateErr != nil {
						item["status"], item["error"] = "Base URL 修复失败", updateErr.Error()
						results[index] = repairResult{item: item, kind: "failed"}
						continue
					}
					remoteWritten = true
				}
				recoveredRow, recoverErr := client.RecoverAccountState(ctx, accountID)
				if recoverErr != nil {
					item["status"], item["error"] = "账号状态恢复失败", recoverErr.Error()
					results[index] = repairResult{item: item, kind: "failed", written: remoteWritten}
					continue
				}
				remoteWritten = true
				if !strings.EqualFold(strings.TrimSpace(fmt.Sprint(firstValue(recoveredRow, "status"))), "active") {
					if _, statusErr := client.UpdateAccount(ctx, accountID, map[string]any{"status": "active"}); statusErr != nil {
						item["status"], item["error"] = "账号状态恢复失败", statusErr.Error()
						results[index] = repairResult{item: item, kind: "failed", written: true}
						continue
					}
				}
				if _, schedulingErr := client.SetAccountSchedulable(ctx, accountID, true); schedulingErr != nil {
					item["status"], item["error"] = "调度开启失败", schedulingErr.Error()
					results[index] = repairResult{item: item, kind: "failed", written: true}
					continue
				}
				item["remote_write"] = true
				confirmedRow, confirmErr := client.Account(ctx, accountID)
				if confirmErr != nil {
					item["status"], item["error"] = "写后确认失败", confirmErr.Error()
					results[index] = repairResult{item: item, kind: "failed", written: true}
					continue
				}
				confirmed, confirmedSource, confirmedAvailable := managementRowBaseURL(confirmedRow)
				confirmedStatus := strings.ToLower(strings.TrimSpace(fmt.Sprint(firstValue(confirmedRow, "status"))))
				confirmedSchedulable, schedulableOK := confirmedRow["schedulable"].(bool)
				if !confirmedAvailable || confirmedSource != "explicit" || !sameBaseURL(confirmed, target) ||
					confirmedStatus != "active" || !schedulableOK || !confirmedSchedulable {
					item["status"], item["error"] = "写后确认失败", "Base URL、账号状态或调度状态未全部恢复"
					results[index] = repairResult{item: item, kind: "failed", written: true}
					continue
				}
				item["status"], item["readback_confirmed"] = "已修复并恢复调度", true
				results[index] = repairResult{item: item, kind: "repaired", written: true,
					observation: &business.AccountBaseURLObservation{AccountID: accountID, BaseURL: &confirmed, Source: confirmedSource}}
			}
		}()
	}
	for index := range accountIDs {
		jobs <- index
	}
	close(jobs)
	workers.Wait()

	items := make([]map[string]any, 0, len(results))
	observations := make([]business.AccountBaseURLObservation, 0, len(results))
	repaired, unchanged, skipped, failed, written := 0, 0, 0, 0, 0
	for _, result := range results {
		items = append(items, result.item)
		if result.observation != nil {
			observations = append(observations, *result.observation)
		}
		if result.written {
			written++
		}
		switch result.kind {
		case "repaired":
			repaired++
		case "unchanged":
			unchanged++
		case "skipped":
			skipped++
		default:
			failed++
		}
	}
	result := map[string]any{
		"operation": "account.base_url.repair", "requested": len(accountIDs), "repaired": repaired,
		"unchanged": unchanged, "skipped": skipped, "failed": failed, "items": items, "actor": actor,
		"remote_write": written > 0,
	}
	if len(observations) > 0 {
		if err := s.repository.CommitAccountBaseURLObservations(ctx, observations); err != nil {
			return result, fmt.Errorf("Base URL 修复结果保存失败：%w", err)
		}
	}
	if failed > 0 {
		return result, errors.New("部分账号 Base URL 修复失败，请查看明细")
	}
	return result, nil
}

func validAccountRepairBaseURL(value string) (string, bool) {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	validScheme := parsed != nil && (strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https"))
	if err != nil || !validScheme || parsed.Host == "" || parsed.User != nil {
		return "", false
	}
	return value, true
}

func comparableBaseURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed == nil {
		return strings.TrimRight(strings.TrimSpace(value), "/")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.Fragment = ""
	return parsed.String()
}

func sameBaseURL(left, right string) bool {
	return comparableBaseURL(left) == comparableBaseURL(right)
}

func managementRowBaseURL(row map[string]any) (string, string, bool) {
	raw, present := row["base_url"]
	if !present {
		credentials, ok := row["credentials"].(map[string]any)
		if !ok {
			return managementDefaultBaseURL(row)
		}
		raw, present = credentials["base_url"]
	}
	value, ok := raw.(string)
	value = strings.TrimSpace(value)
	if present && ok && value != "" {
		return value, "explicit", true
	}
	return managementDefaultBaseURL(row)
}

func managementDefaultBaseURL(row map[string]any) (string, string, bool) {
	platform := strings.ToLower(strings.TrimSpace(fmt.Sprint(firstValue(row, "platform"))))
	accountType := strings.ToLower(strings.TrimSpace(fmt.Sprint(firstValue(row, "type", "account_type"))))
	if accountType != "apikey" && accountType != "upstream" && accountType != "oauth" {
		return "", "", false
	}
	var value string
	switch platform {
	case "anthropic":
		if accountType == "apikey" {
			value = "https://api.anthropic.com"
		}
	case "openai":
		if accountType == "apikey" || accountType == "upstream" {
			value = "https://api.openai.com"
		}
	case "grok":
		if accountType == "oauth" {
			value = "https://cli-chat-proxy.grok.com/v1"
		} else {
			value = "https://api.x.ai/v1"
		}
	case "gemini":
		if accountType == "apikey" {
			value = "https://generativelanguage.googleapis.com"
		}
	}
	return value, "platform_default", value != ""
}

func (s *Service) syncAccountRates(ctx context.Context, accountIDs []string, actor string) (map[string]any, error) {
	if s.rateWriter == nil {
		return nil, errors.New("账号倍率写回服务尚未就绪")
	}
	bound, err := s.repository.BoundAccountsForMaintenance(ctx, accountIDs)
	if err != nil {
		return nil, fmt.Errorf("账号绑定读取失败：%w", err)
	}
	if len(accountIDs) == 0 {
		seen := make(map[string]struct{}, len(bound))
		for _, account := range bound {
			if _, found := seen[account.AccountID]; found {
				continue
			}
			seen[account.AccountID] = struct{}{}
			accountIDs = append(accountIDs, account.AccountID)
		}
	}
	if len(accountIDs) == 0 {
		return map[string]any{
			"operation": "account.rate.sync", "source": "upstream_live", "requested": 0,
			"updated": 0, "unchanged": 0, "skipped": 0, "missing": 0, "failed": 0,
			"items": []map[string]any{}, "read_only": false, "remote_write": false,
		}, nil
	}
	localNames := make(map[string]string, len(accountIDs))
	if names, ok := s.repository.(accountNameRepository); ok {
		localNames, err = names.AccountNamesForMaintenance(ctx, accountIDs)
		if err != nil {
			return nil, fmt.Errorf("账号名称读取失败：%w", err)
		}
	}
	byID := make(map[string][]business.BoundAccountMaintenance, len(bound))
	for _, account := range bound {
		if strings.TrimSpace(account.AccountName) == "" {
			account.AccountName = localNames[account.AccountID]
		}
		byID[account.AccountID] = append(byID[account.AccountID], account)
	}
	type catalogLoad struct {
		ready    chan struct{}
		snapshot business.UpstreamCatalogSnapshot
		err      error
	}
	loads := map[string]*catalogLoad{}
	var loadsMu sync.Mutex
	loadCatalog := func(run context.Context, host string) (business.UpstreamCatalogSnapshot, error) {
		loadsMu.Lock()
		if existing := loads[host]; existing != nil {
			loadsMu.Unlock()
			select {
			case <-existing.ready:
				return existing.snapshot, existing.err
			case <-run.Done():
				return business.UpstreamCatalogSnapshot{}, run.Err()
			}
		}
		load := &catalogLoad{ready: make(chan struct{})}
		loads[host] = load
		loadsMu.Unlock()
		defer close(load.ready)
		if s.upstreams == nil {
			load.err = errors.New("NewAPI 上游目录读取服务尚未就绪")
			return load.snapshot, load.err
		}
		auths, ok := s.targets.(upstreamAuthStore)
		if !ok {
			load.err = errors.New("NewAPI 私有授权读取服务尚未就绪")
			return load.snapshot, load.err
		}
		record, authErr := auths.AuthRecord(run, host)
		if authErr != nil {
			load.err = authErr
			return load.snapshot, load.err
		}
		if record == nil {
			if s.resolver != nil {
				record, authErr = s.resolver.ResolveAuth(run, host, actor)
				if authErr != nil {
					load.err = fmt.Errorf("Host %q 的私有授权恢复失败：%w", host, authErr)
					return load.snapshot, load.err
				}
			}
			if record == nil {
				load.err = fmt.Errorf("未找到 Host %q 的私有授权记录", host)
				return load.snapshot, load.err
			}
		}
		load.snapshot, load.err = s.upstreams.ReadCatalog(run, *record)
		return load.snapshot, load.err
	}

	client, err := s.maintenanceClient(ctx)
	if err != nil {
		return nil, err
	}
	type upstreamRate struct {
		account              business.BoundAccountMaintenance
		multiplier           string
		manualMultiplierOnly bool
		skippedReason        string
		err                  error
	}
	upstreamRates := make([]upstreamRate, len(accountIDs))
	sub2APIIDs := make([]string, 0, len(accountIDs))
	newAPIIndexes := make([]int, 0, len(accountIDs))
	for index, accountID := range accountIDs {
		upstreamRates[index].account = business.BoundAccountMaintenance{
			AccountID: accountID, AccountName: localNames[accountID],
		}
		bindings := byID[accountID]
		switch len(bindings) {
		case 0:
			upstreamRates[index].err = errors.New("未找到该账号的有效上游绑定")
		case 1:
			upstreamRates[index].account = bindings[0]
			if bindings[0].ManualPriority && !bindings[0].SyncBalanceMultiplier {
				upstreamRates[index].skippedReason = "人工控制账号未开启余额与倍率同步"
				continue
			}
			upstreamRates[index].manualMultiplierOnly = bindings[0].ManualPriority
			if isNewAPIType(bindings[0].UpstreamType) {
				newAPIIndexes = append(newAPIIndexes, index)
			} else {
				sub2APIIDs = append(sub2APIIDs, accountID)
			}
		default:
			upstreamRates[index].err = errors.New("账号存在多个上游绑定，无法唯一判定倍率")
		}
	}
	if len(sub2APIIDs) > 0 {
		batch, batchErr := client.AccountUpstreamMultipliers(ctx, sub2APIIDs)
		for index, accountID := range accountIDs {
			if upstreamRates[index].err != nil || upstreamRates[index].skippedReason != "" || isNewAPIType(upstreamRates[index].account.UpstreamType) {
				continue
			}
			if batchErr != nil {
				upstreamRates[index].err = batchErr
				continue
			}
			item, found := batch[accountID]
			if !found {
				upstreamRates[index].err = errors.New("批量上游倍率探测未返回该账号结果")
				continue
			}
			upstreamRates[index].multiplier, upstreamRates[index].err = item.Multiplier, item.Err
		}
	}
	newAPIJobs := make(chan int)
	var probeWorkers sync.WaitGroup
	for range min(4, len(newAPIIndexes)) {
		probeWorkers.Add(1)
		go func() {
			defer probeWorkers.Done()
			for index := range newAPIJobs {
				account := upstreamRates[index].account
				catalog, catalogErr := loadCatalog(ctx, account.UpstreamHost)
				if catalogErr != nil {
					upstreamRates[index].err = catalogErr
					continue
				}
				upstreamRates[index].multiplier, upstreamRates[index].err = newAPIAccountMultiplier(account, catalog)
			}
		}()
	}
	for _, index := range newAPIIndexes {
		newAPIJobs <- index
	}
	close(newAPIJobs)
	probeWorkers.Wait()
	observations := make([]business.AccountRateObservation, 0, len(upstreamRates))
	for _, probe := range upstreamRates {
		if probe.err == nil && probe.skippedReason == "" {
			observations = append(observations, business.AccountRateObservation{AccountID: probe.account.AccountID, Rate: probe.multiplier})
		}
	}
	if err := s.repository.CommitAccountRateObservations(ctx, observations); err != nil {
		return nil, fmt.Errorf("上游倍率观测保存失败：%w", err)
	}

	// Upstream collection is complete before this point. Read the management
	// catalog once, compare stable IDs, and only write changed multipliers.
	remoteRows, err := client.Accounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("管理平台账号目录读取失败：%w", err)
	}
	remoteByID := make(map[string]map[string]any, len(remoteRows))
	for _, row := range remoteRows {
		accountID := strings.TrimSpace(fmt.Sprint(firstValue(row, "id", "account_id")))
		if accountID == "" {
			continue
		}
		if _, duplicate := remoteByID[accountID]; duplicate {
			return nil, fmt.Errorf("管理平台返回重复账号 ID：%s", accountID)
		}
		remoteByID[accountID] = row
	}
	type rateResult struct {
		item      map[string]any
		updated   bool
		unchanged bool
		missing   bool
		failed    bool
		skipped   bool
		written   bool
	}
	results := make([]rateResult, len(accountIDs))
	writeJobs := make(chan int)
	var writeWorkers sync.WaitGroup
	for range min(4, len(accountIDs)) {
		writeWorkers.Add(1)
		go func() {
			defer writeWorkers.Done()
			for index := range writeJobs {
				accountID := accountIDs[index]
				probe := upstreamRates[index]
				account := probe.account
				item := map[string]any{"account_id": accountID, "remote_write": false, "readback_confirmed": false}
				item["account_name"], item["upstream_host"] = account.AccountName, account.UpstreamHost
				if probe.skippedReason != "" {
					item["status"], item["reason"] = "人工控制，已跳过", probe.skippedReason
					results[index] = rateResult{item: item, skipped: true}
					continue
				}
				if probe.err != nil {
					item["status"], item["error"] = "上游探测失败", probe.err.Error()
					var httpError *adminclient.HTTPError
					missing := errors.As(probe.err, &httpError) && httpError.StatusCode == http.StatusNotFound
					if missing {
						item["status"] = "管理平台不存在"
					}
					results[index] = rateResult{item: item, missing: missing, failed: !missing}
					continue
				}
				remote, exists := remoteByID[accountID]
				if !exists {
					item["status"] = "管理平台不存在"
					results[index] = rateResult{item: item, missing: true}
					continue
				}
				remoteMultiplier, rateErr := managementAccountMultiplier(remote)
				if rateErr != nil {
					item["status"], item["error"] = "同步失败", rateErr.Error()
					results[index] = rateResult{item: item, failed: true}
					continue
				}
				remoteName := strings.TrimSpace(fmt.Sprint(firstValue(remote, "name")))
				expectedName := account.NameForMultiplier(probe.multiplier)
				item["account_name"], item["before"], item["after"] = remoteName, remoteMultiplier, probe.multiplier
				item["name_before"] = remoteName
				if !probe.manualMultiplierOnly {
					item["name_after"] = expectedName
				}
				item["upstream_multiplier"] = probe.multiplier
				nameMatches := probe.manualMultiplierOnly || (remoteName == expectedName && account.AccountName == expectedName)
				if remoteMultiplier == probe.multiplier && sameRate(account.CurrentMultiplier, probe.multiplier) && nameMatches {
					item["status"] = "已确认一致"
					results[index] = rateResult{item: item, unchanged: true}
					continue
				}
				var writeResult map[string]any
				var writeErr error
				if probe.manualMultiplierOnly {
					writeResult, writeErr = s.rateWriter.SyncAccountMultiplier(ctx, accountID, probe.multiplier, actor)
				} else {
					writeResult, writeErr = s.rateWriter.SyncAccountRate(ctx, accountID, expectedName, probe.multiplier, actor)
				}
				if writeErr != nil {
					item["status"], item["error"] = "写回失败", writeErr.Error()
					var state interface{ RemoteWriteSucceeded() bool }
					if errors.As(writeErr, &state) {
						item["remote_write"] = state.RemoteWriteSucceeded()
					}
					results[index] = rateResult{item: item, failed: true, written: item["remote_write"] == true}
					continue
				}
				item["remote_write"] = writeResult["remote_write"]
				item["readback_confirmed"] = writeResult["readback_confirmed"]
				item["status"] = "已同步"
				results[index] = rateResult{item: item, updated: true, written: true}
			}
		}()
	}
	for index := range accountIDs {
		writeJobs <- index
	}
	close(writeJobs)
	writeWorkers.Wait()

	items := make([]map[string]any, 0, len(results))
	updated, unchanged, skipped, missing, failed, written := 0, 0, 0, 0, 0, 0
	for _, result := range results {
		items = append(items, result.item)
		if result.updated {
			updated++
		}
		if result.unchanged {
			unchanged++
		}
		if result.missing {
			missing++
		}
		if result.skipped {
			skipped++
		}
		if result.failed {
			failed++
		}
		if result.written {
			written++
		}
	}
	return map[string]any{
		"operation": "account.rate.sync", "source": "upstream_live", "requested": len(accountIDs),
		"updated": updated, "unchanged": unchanged, "skipped": skipped, "missing": missing, "failed": failed,
		"items": items, "read_only": false, "remote_write": written > 0,
	}, nil
}

func sameRate(left, right string) bool {
	leftRate, leftOK := new(big.Rat).SetString(strings.TrimSpace(left))
	rightRate, rightOK := new(big.Rat).SetString(strings.TrimSpace(right))
	return leftOK && rightOK && leftRate.Cmp(rightRate) == 0
}

func isNewAPIType(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "newapi" || value == "oneapi"
}

func managementAccountMultiplier(row map[string]any) (string, error) {
	raw := firstValue(row, "rate_multiplier", "multiplier")
	text := strings.TrimSpace(fmt.Sprint(raw))
	value, ok := new(big.Rat).SetString(text)
	if !ok || value.Sign() <= 0 {
		return "", errors.New("管理平台账号倍率必须是大于 0 的有效数字")
	}
	normalized := value.FloatString(28)
	normalized = strings.TrimRight(strings.TrimRight(normalized, "0"), ".")
	if normalized == "" || normalized == "0" {
		return "", errors.New("管理平台账号倍率必须是大于 0 的有效数字")
	}
	return normalized, nil
}

func newAPIAccountMultiplier(account business.BoundAccountMaintenance, catalog business.UpstreamCatalogSnapshot) (string, error) {
	var matched *business.UpstreamCatalogKey
	for index := range catalog.Keys {
		if strings.TrimSpace(catalog.Keys[index].KeyID) != strings.TrimSpace(account.UpstreamKeyID) {
			continue
		}
		if matched != nil {
			return "", errors.New("NewAPI 上游返回重复的稳定 Token ID")
		}
		matched = &catalog.Keys[index]
	}
	if matched == nil {
		return "", errors.New("NewAPI 上游未找到绑定的稳定 Token ID")
	}
	if matched.RateAmbiguous {
		return "", errors.New("NewAPI Token 使用多分组路由，无法判定单一倍率")
	}
	groupID := strings.TrimSpace(account.UpstreamGroupID)
	if matched.UpstreamGroup != nil && strings.TrimSpace(*matched.UpstreamGroup) != "" {
		groupID = strings.TrimSpace(*matched.UpstreamGroup)
	}
	if groupID == "" || strings.EqualFold(groupID, "auto") {
		return "", errors.New("NewAPI Token 未绑定唯一固定分组，无法判定单一倍率")
	}
	var rawRate *string
	for _, group := range catalog.Groups {
		if strings.TrimSpace(group.GroupID) == groupID || strings.TrimSpace(group.Name) == groupID {
			if rawRate != nil {
				return "", fmt.Errorf("NewAPI 上游分组 %q 不唯一", groupID)
			}
			rawRate = group.RawRate
		}
	}
	if rawRate == nil || strings.TrimSpace(*rawRate) == "" {
		return "", fmt.Errorf("NewAPI 上游分组 %q 未返回有效倍率", groupID)
	}
	text, err := business.ConvertMultiplier(*rawRate, account.RechargeRate)
	if err != nil {
		return "", fmt.Errorf("NewAPI 上游折算倍率无效: %w", err)
	}
	return text, nil
}

func (s *Service) maintenanceClient(ctx context.Context) (*adminclient.Client, error) {
	target, err := s.targets.TargetSettings(ctx)
	if err != nil {
		return nil, err
	}
	return adminclient.New(adminclient.Config{BaseURL: target.BaseURL, AdminKey: target.AdminKey,
		Timeout: time.Duration(target.TimeoutSeconds) * time.Second, Attempts: 3}, nil)
}

func (s *Service) maintenanceCatalog(ctx context.Context, accountIDs []string) ([]business.BoundAccountMaintenance, map[string]map[string]any, error) {
	bound, err := s.repository.BoundAccountsForMaintenance(ctx, accountIDs)
	if err != nil {
		return nil, nil, err
	}
	client, err := s.maintenanceClient(ctx)
	if err != nil {
		return nil, nil, err
	}
	remote, err := client.Accounts(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("管理平台账号目录读取失败：%w", err)
	}
	byID := make(map[string]map[string]any, len(remote))
	for _, row := range remote {
		id := strings.TrimSpace(fmt.Sprint(firstValue(row, "id", "account_id")))
		if id != "" {
			byID[id] = row
		}
	}
	return bound, byID, nil
}

func (s *Service) revalidateAccounts(ctx context.Context, accountIDs []string, actor string) (map[string]any, error) {
	bound, remote, err := s.maintenanceCatalog(ctx, accountIDs)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(bound))
	commits := make([]business.BindingVerification, 0, len(bound))
	verified, missing := 0, 0
	for _, account := range bound {
		_, exists := remote[account.AccountID]
		if exists {
			verified++
		} else {
			missing++
		}
		commits = append(commits, business.BindingVerification{AccountID: account.AccountID, Exists: exists})
		items = append(items, map[string]any{"account_id": account.AccountID, "account_name": account.AccountName,
			"upstream_host": account.UpstreamHost, "status": map[bool]string{true: "已确认存在", false: "管理平台不存在"}[exists]})
	}
	if err := s.repository.CommitBindingVerification(ctx, commits); err != nil {
		return nil, fmt.Errorf("复验结果保存失败：%w", err)
	}
	return map[string]any{"operation": "account.binding.revalidation", "requested": len(accountIDs), "bound": len(bound),
		"verified": verified, "missing": missing, "items": items, "actor": actor, "remote_write": false}, nil
}

func (s *Service) repairAccountNames(ctx context.Context, accountIDs []string, actor string) (map[string]any, error) {
	bound, remote, err := s.maintenanceCatalog(ctx, accountIDs)
	if err != nil {
		return nil, err
	}
	client, err := s.maintenanceClient(ctx)
	if err != nil {
		return nil, err
	}
	type repairResult struct {
		item          map[string]any
		commit        *business.AccountNameRepairCommit
		remoteWritten bool
	}
	results := make([]repairResult, len(bound))
	jobs := make(chan int)
	var workers sync.WaitGroup
	workerCount := min(4, len(bound))
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				account := bound[index]
				row, exists := remote[account.AccountID]
				before := strings.TrimSpace(fmt.Sprint(firstValue(row, "name")))
				item := map[string]any{"account_id": account.AccountID, "account_name": account.AccountName, "before": before, "after": account.ExpectedName,
					"upstream_host": account.UpstreamHost}
				switch {
				case !exists:
					item["status"] = "管理平台不存在"
				case before == account.ExpectedName:
					item["status"] = "无需修复"
				default:
					_, writeErr := client.Mutate(ctx, http.MethodPut, "/admin/accounts/"+account.AccountID, map[string]any{"name": account.ExpectedName})
					if writeErr != nil {
						item["status"], item["error"] = "修复失败", writeErr.Error()
						break
					}
					results[index].remoteWritten = true
					item["remote_write"] = true
					readback, readErr := client.Account(ctx, account.AccountID)
					if readErr != nil {
						item["status"], item["error"] = "修复失败", "管理平台名称写入成功，但账号读回失败："+readErr.Error()
						break
					}
					confirmed := strings.TrimSpace(fmt.Sprint(firstValue(readback, "name")))
					if confirmed != account.ExpectedName {
						item["status"], item["error"] = "修复失败", "管理平台名称写入成功，但账号名称读回不一致"
						break
					}
					item["status"] = "已修复"
					item["before"] = before
					item["readback_confirmed"] = true
					results[index].commit = &business.AccountNameRepairCommit{AccountID: account.AccountID, Name: confirmed}
				}
				results[index].item = item
			}
		}()
	}
	for index := range bound {
		jobs <- index
	}
	close(jobs)
	workers.Wait()
	items := make([]map[string]any, 0, len(results))
	verification := make([]business.BindingVerification, 0, len(results))
	commits := make([]business.AccountNameRepairCommit, 0, len(results))
	renamed, unchanged, missing, failed, written := 0, 0, 0, 0, 0
	for index, result := range results {
		items = append(items, result.item)
		if result.remoteWritten {
			written++
		}
		_, exists := remote[bound[index].AccountID]
		verification = append(verification, business.BindingVerification{AccountID: bound[index].AccountID, Exists: exists})
		switch result.item["status"] {
		case "已修复":
			renamed++
			commits = append(commits, *result.commit)
		case "无需修复":
			unchanged++
		case "管理平台不存在":
			missing++
		default:
			failed++
		}
	}
	result := map[string]any{"operation": "account.name.repair", "requested": len(accountIDs), "bound": len(bound),
		"renamed": renamed, "unchanged": unchanged, "missing": missing, "failed": failed, "items": items, "actor": actor, "remote_write": written > 0}
	if err := s.repository.CommitBindingVerification(ctx, verification); err != nil {
		return result, fmt.Errorf("绑定状态保存失败：%w", err)
	}
	if err := s.repository.CommitAccountNameRepairs(ctx, commits); err != nil {
		return result, fmt.Errorf("名称修复结果保存失败：%w", err)
	}
	if failed > 0 {
		return result, errors.New("部分账号名称修复失败，请查看明细")
	}
	return result, nil
}

func (s *Service) repairAccountDefaults(ctx context.Context, accountIDs []string, actor string) (map[string]any, error) {
	defaults, err := s.targets.AccountDefaults(ctx)
	if err != nil {
		return nil, fmt.Errorf("账号默认参数读取失败：%w", err)
	}
	bound, remote, err := s.maintenanceCatalog(ctx, accountIDs)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]business.BoundAccountMaintenance, len(bound))
	for _, account := range bound {
		if _, exists := byID[account.AccountID]; !exists {
			byID[account.AccountID] = account
		}
	}
	client, err := s.maintenanceClient(ctx)
	if err != nil {
		return nil, err
	}
	type repairResult struct {
		item    map[string]any
		commit  *business.AccountDefaultsRepairCommit
		written bool
		kind    string
	}
	results := make([]repairResult, len(accountIDs))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(4, len(accountIDs)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				accountID := accountIDs[index]
				account, boundExists := byID[accountID]
				item := map[string]any{"account_id": accountID, "remote_write": false, "readback_confirmed": false}
				if boundExists {
					item["account_name"], item["upstream_host"] = account.AccountName, account.UpstreamHost
				}
				row, remoteExists := remote[accountID]
				switch {
				case !boundExists:
					item["status"] = "没有绑定，已跳过"
					results[index] = repairResult{item: item, kind: "skipped"}
					continue
				case !account.ConsoleOnboarded:
					item["status"] = "非本控制台添加，未修改"
					results[index] = repairResult{item: item, kind: "skipped"}
					continue
				case !remoteExists:
					item["status"] = "管理平台不存在"
					results[index] = repairResult{item: item, kind: "failed"}
					continue
				}
				item["account_name"] = strings.TrimSpace(fmt.Sprint(firstValue(row, "name")))
				concurrency, concurrencyErr := managementInteger(row, "concurrency")
				priority, priorityErr := managementInteger(row, "priority")
				loadFactor, loadFactorErr := managementOptionalInteger(row, "load_factor")
				if concurrencyErr != nil || priorityErr != nil || loadFactorErr != nil {
					item["status"] = "参数读取失败"
					item["error"] = errors.Join(concurrencyErr, priorityErr, loadFactorErr).Error()
					results[index] = repairResult{item: item, kind: "failed"}
					continue
				}
				body := map[string]any{}
				priorityValue, concurrencyValue := priority, concurrency
				commit := business.AccountDefaultsRepairCommit{
					AccountID: accountID, Priority: &priorityValue, Concurrency: &concurrencyValue, LoadFactorPresent: true,
				}
				if loadFactor != nil && *loadFactor > 0 {
					value := strconv.FormatInt(*loadFactor, 10)
					commit.LoadFactor = &value
				}
				afterConcurrency, afterPriority := concurrency, priority
				if concurrency <= 0 {
					afterConcurrency = defaults.Concurrency
					body["concurrency"] = afterConcurrency
					*commit.Concurrency = afterConcurrency
				}
				if priority <= 0 {
					afterPriority = defaults.Priority
					body["priority"] = afterPriority
					*commit.Priority = afterPriority
				}
				if loadFactor != nil && *loadFactor <= 0 {
					body["load_factor"] = int64(0)
					commit.LoadFactor = nil
				}
				item["before"] = accountDefaultsSummary(concurrency, priority, loadFactor)
				item["after"] = accountDefaultsSummary(afterConcurrency, afterPriority, positiveLoadFactor(loadFactor))
				if len(body) == 0 {
					item["status"] = "无需修复"
					results[index] = repairResult{item: item, commit: &commit, kind: "unchanged"}
					continue
				}
				updated, updateErr := client.UpdateAccount(ctx, accountID, body)
				if updateErr != nil {
					item["status"], item["error"] = "修复失败", updateErr.Error()
					results[index] = repairResult{item: item, kind: "failed"}
					continue
				}
				item["remote_write"] = true
				confirmedConcurrency, confirmedConcurrencyErr := managementInteger(updated, "concurrency")
				confirmedPriority, confirmedPriorityErr := managementInteger(updated, "priority")
				confirmedLoadFactor, confirmedLoadFactorErr := managementOptionalInteger(updated, "load_factor")
				confirmed := confirmedConcurrencyErr == nil && confirmedPriorityErr == nil && confirmedLoadFactorErr == nil &&
					confirmedConcurrency == afterConcurrency && confirmedPriority == afterPriority &&
					(body["load_factor"] == nil || confirmedLoadFactor == nil)
				if !confirmed {
					item["status"], item["error"] = "修复失败", "管理平台更新响应中的账号参数与预期不一致"
					results[index] = repairResult{item: item, written: true, kind: "failed"}
					continue
				}
				item["status"], item["readback_confirmed"] = "已修复", true
				commit.RemoteRepaired = true
				results[index] = repairResult{item: item, commit: &commit, written: true, kind: "repaired"}
			}
		}()
	}
	for index := range accountIDs {
		jobs <- index
	}
	close(jobs)
	workers.Wait()

	items := make([]map[string]any, 0, len(results))
	commits := make([]business.AccountDefaultsRepairCommit, 0, len(results))
	repaired, unchanged, skipped, failed, written := 0, 0, 0, 0, 0
	for _, result := range results {
		items = append(items, result.item)
		if result.written {
			written++
		}
		switch result.kind {
		case "repaired":
			repaired++
			commits = append(commits, *result.commit)
		case "unchanged":
			unchanged++
			commits = append(commits, *result.commit)
		case "skipped":
			skipped++
		default:
			failed++
		}
	}
	result := map[string]any{"operation": "account.defaults.repair", "requested": len(accountIDs), "bound": len(byID),
		"repaired": repaired, "unchanged": unchanged, "skipped": skipped, "failed": failed, "items": items,
		"actor": actor, "remote_write": written > 0, "default_concurrency": defaults.Concurrency, "default_priority": defaults.Priority}
	if err := s.repository.CommitAccountDefaultsRepairs(ctx, commits, actor); err != nil {
		return result, fmt.Errorf("默认参数修复结果保存失败：%w", err)
	}
	if failed > 0 {
		return result, errors.New("部分账号默认参数修复失败，请查看明细")
	}
	return result, nil
}

func managementInteger(row map[string]any, key string) (int64, error) {
	value, present := row[key]
	if !present || value == nil {
		return 0, fmt.Errorf("管理平台账号未返回 %s", key)
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(value)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("管理平台账号 %s 不是有效整数", key)
	}
	return parsed, nil
}

func managementOptionalInteger(row map[string]any, key string) (*int64, error) {
	value, present := row[key]
	if !present || value == nil || strings.TrimSpace(fmt.Sprint(value)) == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(value)), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("管理平台账号 %s 不是有效整数", key)
	}
	return &parsed, nil
}

func positiveLoadFactor(value *int64) *int64 {
	if value == nil || *value <= 0 {
		return nil
	}
	return value
}

func accountDefaultsSummary(concurrency, priority int64, loadFactor *int64) string {
	effectiveLoad := concurrency
	loadLabel := "跟随并发"
	if loadFactor != nil && *loadFactor > 0 {
		effectiveLoad, loadLabel = *loadFactor, strconv.FormatInt(*loadFactor, 10)
	}
	return fmt.Sprintf("并发 %d · 负载 %s（有效 %d）· 优先级 %d", concurrency, loadLabel, effectiveLoad, priority)
}

func (s *Service) cleanupMissingBindings(ctx context.Context, accountIDs []string, actor string) (map[string]any, error) {
	bound, remote, err := s.maintenanceCatalog(ctx, accountIDs)
	if err != nil {
		return nil, err
	}
	verification := make([]business.BindingVerification, 0, len(bound))
	missingIDs := make([]string, 0, len(bound))
	items := make([]map[string]any, 0, len(bound))
	for _, account := range bound {
		_, exists := remote[account.AccountID]
		verification = append(verification, business.BindingVerification{AccountID: account.AccountID, Exists: exists})
		status := "账号仍然存在，未清理"
		if !exists {
			status = "待清理"
			missingIDs = append(missingIDs, account.AccountID)
		}
		items = append(items, map[string]any{"account_id": account.AccountID, "account_name": account.AccountName,
			"upstream_host": account.UpstreamHost, "status": status})
	}
	result := map[string]any{"operation": "account.missing-binding.cleanup", "requested": len(accountIDs),
		"bound": len(bound), "cleaned": 0, "skipped": len(bound) - len(missingIDs), "items": items, "actor": actor, "remote_write": false}
	if err := s.repository.CommitBindingVerification(ctx, verification); err != nil {
		return result, fmt.Errorf("最新复验结果保存失败：%w", err)
	}
	if len(missingIDs) == 0 {
		return result, nil
	}
	cleanup, err := s.repository.CleanupMissingBindings(ctx, missingIDs, actor)
	if err != nil {
		return result, fmt.Errorf("失效绑定清理失败：%w", err)
	}
	cleanedIDs := make(map[string]struct{}, len(cleanup.IDs))
	for _, accountID := range cleanup.IDs {
		cleanedIDs[accountID] = struct{}{}
	}
	for _, item := range items {
		accountID, _ := item["account_id"].(string)
		if _, cleaned := cleanedIDs[accountID]; cleaned {
			item["status"] = "已清理失效绑定"
		}
	}
	result["cleaned"], result["event_id"] = cleanup.Cleaned, cleanup.EventID
	return result, nil
}

func normalizeAccountIDs(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, errors.New("当前筛选结果中没有可处理账号")
	}
	if len(values) > 1000 {
		return nil, errors.New("单次最多处理 1000 个账号")
	}
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed <= 0 || strconv.FormatInt(parsed, 10) != value {
			return nil, errors.New("账号必须使用稳定正整数 ID")
		}
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func firstValue(row map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, found := row[key]; found {
			return value
		}
	}
	return ""
}

func (s *Service) execute(task taskstore.Task, actor string) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	task.Status, task.Progress, task.Message, task.UpdatedAt = "running", 20, "读取 Admin API 账号与分组目录", time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.tasks.Save(ctx, task); err != nil {
		return
	}
	result, err := s.Sync(ctx, actor)
	task.Progress, task.UpdatedAt = 100, time.Now().UTC().Format(time.RFC3339Nano)
	if err != nil {
		task.Status, task.Message = "failed", "管理快照同步失败："+err.Error()
		task.Result = map[string]any{"error": err.Error(), "remote_write": false}
	} else {
		task.Status, task.Message = "succeeded", "管理快照已同步到 Console 业务库"
		task.Result = map[string]any{
			"accounts": result.Accounts, "group_links": result.GroupLinks, "groups": result.Groups,
			"event_id": result.EventID, "remote_write": result.RemoteWrite, "read_only": result.ReadOnly,
		}
	}
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) Sync(ctx context.Context, actor string) (business.ManagementSyncResult, error) {
	target, err := s.targets.TargetSettings(ctx)
	if err != nil {
		return business.ManagementSyncResult{}, err
	}
	client, err := adminclient.New(adminclient.Config{
		BaseURL: target.BaseURL, AdminKey: target.AdminKey,
		Timeout: time.Duration(target.TimeoutSeconds) * time.Second, Attempts: 3,
	}, nil)
	if err != nil {
		return business.ManagementSyncResult{}, err
	}
	groups, err := client.Groups(ctx)
	if err != nil {
		return business.ManagementSyncResult{}, fmt.Errorf("分组目录读取失败：%w", err)
	}
	accounts, err := client.Accounts(ctx)
	if err != nil {
		return business.ManagementSyncResult{}, fmt.Errorf("账号目录读取失败：%w", err)
	}
	return s.repository.SyncManagementSnapshot(ctx, accounts, groups, actor)
}

func managementTaskID() (string, error) {
	value := make([]byte, 6)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
