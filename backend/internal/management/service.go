package management

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
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
}

type Repository interface {
	SyncManagementSnapshot(context.Context, []map[string]any, []map[string]any, string) (business.ManagementSyncResult, error)
	BoundAccountsForMaintenance(context.Context, []string) ([]business.BoundAccountMaintenance, error)
	CommitBindingVerification(context.Context, []business.BindingVerification) error
	CommitAccountNameRepairs(context.Context, []business.AccountNameRepairCommit) error
	CleanupMissingBindings(context.Context, []string, string) (business.MissingBindingCleanupResult, error)
}

type TaskStore interface {
	Save(context.Context, taskstore.Task) error
}

type Service struct {
	targets    TargetStore
	repository Repository
	tasks      TaskStore
	timeout    time.Duration
}

func New(targets TargetStore, repository Repository, tasks TaskStore) *Service {
	return &Service{targets: targets, repository: repository, tasks: tasks, timeout: 10 * time.Minute}
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

func (s *Service) EnqueueAccountNameRepair(ctx context.Context, accountIDs []string, actor string) (taskstore.Task, error) {
	return s.enqueueMaintenance(ctx, "account-name-repair", "账号命名修复已排队", accountIDs, actor)
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
	task.Status, task.Progress, task.Message, task.UpdatedAt = "running", 10, "读取管理平台账号目录", time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.tasks.Save(ctx, task); err != nil {
		return
	}
	var result map[string]any
	var err error
	if operation == "account-name-repair" {
		result, err = s.repairAccountNames(ctx, accountIDs, actor)
	} else if operation == "account-missing-binding-cleanup" {
		result, err = s.cleanupMissingBindings(ctx, accountIDs, actor)
	} else {
		result, err = s.revalidateAccounts(ctx, accountIDs, actor)
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
		if operation == "account-name-repair" {
			task.Message = fmt.Sprintf("命名修复完成：已修复 %v 个", result["renamed"])
		} else if operation == "account-missing-binding-cleanup" {
			task.Message = fmt.Sprintf("失效绑定修复完成：已清理 %v 个", result["cleaned"])
		} else {
			task.Message = fmt.Sprintf("批量复验完成：存在 %v 个，缺失 %v 个", result["verified"], result["missing"])
		}
	}
	task.Result = result
	taskstore.PersistFinal(s.tasks, task)
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
		item   map[string]any
		commit *business.AccountNameRepairCommit
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
					} else {
						item["status"] = "已修复"
						item["before"] = before
						results[index].commit = &business.AccountNameRepairCommit{AccountID: account.AccountID, Name: account.ExpectedName}
					}
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
	renamed, unchanged, missing, failed := 0, 0, 0, 0
	for index, result := range results {
		items = append(items, result.item)
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
		"renamed": renamed, "unchanged": unchanged, "missing": missing, "failed": failed, "items": items, "actor": actor, "remote_write": renamed > 0}
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
