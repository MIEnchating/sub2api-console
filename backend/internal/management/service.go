package management

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
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
	AccountDefaults(context.Context) (configstore.AccountDefaultsSettings, error)
}

type Repository interface {
	ManagementAccountIDs(context.Context) ([]string, error)
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

type mutationProtectionRepository interface {
	AccountMutationProtection(context.Context, string) (business.AccountMutationProtection, error)
}

type TaskStore interface {
	Save(context.Context, taskstore.Task) error
}

type AccountRateWriter interface {
	SyncAccountRateIfCurrent(context.Context, string, string, string, string, string, func(context.Context) error) (map[string]any, error)
	SyncAccountMultiplierIfCurrent(context.Context, string, string, string, string, func(context.Context) error) (map[string]any, error)
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

type accountRateProbe struct {
	account              business.BoundAccountMaintenance
	observedMultiplier   string
	multiplier           string
	manualMultiplierOnly bool
	skippedReason        string
	fallbackEligible     bool
	fallback             bool
	fallbackSource       string
	err                  error
}

var errAccountRateBindingMissing = errors.New("未找到该账号的有效上游绑定")

type accountRateWriteSkippedError struct {
	reason string
}

func (err *accountRateWriteSkippedError) Error() string { return err.reason }

type Service struct {
	targets         TargetStore
	repository      Repository
	tasks           TaskStore
	taskRunner      taskrunner.Runner
	rateWriter      AccountRateWriter
	upstreams       UpstreamCatalogReader
	resolver        UpstreamAuthResolver
	timeout         time.Duration
	rateBatchMu     sync.Mutex
	rateBatchCursor int
}

func (s *Service) UseUpstreamCatalogReader(reader UpstreamCatalogReader) {
	s.upstreams = reader
}

func (s *Service) UseUpstreamAuthResolver(resolver UpstreamAuthResolver) {
	s.resolver = resolver
}

func (s *Service) UseTaskRunner(runner taskrunner.Runner) { s.taskRunner = runner }

func New(targets TargetStore, repository Repository, tasks TaskStore, rateWriters ...AccountRateWriter) *Service {
	service := &Service{targets: targets, repository: repository, tasks: tasks, timeout: 10 * time.Minute}
	if len(rateWriters) > 0 {
		service.rateWriter = rateWriters[0]
	}
	return service
}

func (s *Service) acquireAccountMutation(ctx context.Context, accountID string) (context.Context, func(), *string, error) {
	guarded, release, err := s.acquireAccountMutations(ctx, []string{accountID}, false)
	if err != nil {
		return nil, nil, nil, err
	}
	reason, err := s.accountMutationProtectionReason(guarded, accountID, false)
	if err != nil {
		release()
		return nil, nil, nil, err
	}
	if reason != nil {
		release()
		return nil, nil, reason, nil
	}
	return guarded, release, nil, nil
}

func (s *Service) acquireAccountMutations(ctx context.Context, accountIDs []string, includeCatalog bool) (context.Context, func(), error) {
	return s.acquireAccountMutationResources(
		ctx,
		accountIDs,
		accountMutationResources(accountIDs, includeCatalog),
	)
}

func (s *Service) acquireAccountMutationResources(
	ctx context.Context,
	accountIDs []string,
	resources []string,
) (context.Context, func(), error) {
	ctx, err := targetguard.Capture(ctx, s.targets)
	if err != nil {
		return nil, nil, err
	}
	guarded, release, err := targetguard.Acquire(ctx, s.repository, resources...)
	if err != nil {
		return nil, nil, err
	}
	guarded, err = targetguard.Bind(guarded, s.targets)
	if err != nil {
		_ = release()
		return nil, nil, err
	}
	return guarded, s.logAccountMutationRelease(release, accountIDs), nil
}

func (s *Service) acquireLocalAccountMutations(ctx context.Context, accountIDs []string, includeCatalog bool) (context.Context, func(), error) {
	resources := accountMutationResources(accountIDs, includeCatalog)
	guarded, release, err := mutationguard.Acquire(ctx, s.repository, resources...)
	if err != nil {
		return nil, nil, err
	}
	return guarded, s.logAccountMutationRelease(release, accountIDs), nil
}

func accountMutationResources(accountIDs []string, includeCatalog bool) []string {
	resources := make([]string, 0, len(accountIDs)+1)
	if includeCatalog {
		resources = append(resources, mutationguard.AccountCatalog())
	}
	for _, accountID := range accountIDs {
		resources = append(resources, mutationguard.Account(accountID))
	}
	return resources
}

func (s *Service) logAccountMutationRelease(release func() error, accountIDs []string) func() {
	return func() {
		if err := release(); err != nil {
			slog.Error("账号维护租约释放失败", "account_ids", accountIDs, "error", err)
		}
	}
}

func (s *Service) accountMutationProtectionReason(
	ctx context.Context,
	accountID string,
	allowManualPriority bool,
) (*string, error) {
	repository, ok := s.repository.(mutationProtectionRepository)
	if !ok {
		return nil, nil
	}
	protection, err := repository.AccountMutationProtection(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("人工保护状态复核失败：%w", err)
	}
	if allowManualPriority {
		protection.ManualPriority = false
	}
	if !protection.Protected() {
		return nil, nil
	}
	reason := "账号已启用" + strings.Join(protection.Reasons(), "、") + "，账号维护未执行自动变更"
	return &reason, nil
}

func (s *Service) EnqueueSync(ctx context.Context, actor string) (taskstore.Task, error) {
	expectedTarget, targetErr := targetguard.Expected(ctx, s.targets)
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
	if err := taskrunner.Go(s.taskRunner, func(parent context.Context) {
		if targetErr == nil {
			parent = targetguard.Expect(parent, expectedTarget)
		}
		s.execute(parent, task, actor)
	}); err != nil {
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
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

func (s *Service) EnqueueHostAccountRateSync(ctx context.Context, host, actor string) (string, error) {
	allowed, err := s.automaticRateSyncAllowed(ctx)
	if err != nil || !allowed {
		return "", err
	}
	bound, err := s.repository.BoundAccountsForMaintenance(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("读取上游绑定账号失败：%w", err)
	}
	accountIDs := accountIDsForHost(bound, host)
	if len(accountIDs) == 0 {
		return "", nil
	}
	task, err := s.EnqueueAccountRateSync(ctx, accountIDs, actor)
	if err != nil {
		return "", err
	}
	return task.ID, nil
}

func (s *Service) EnqueueHostAccountBaseURLSync(ctx context.Context, host, actor string) (string, error) {
	allowed, err := s.automaticRateSyncAllowed(ctx)
	if err != nil || !allowed {
		return "", err
	}
	bound, err := s.repository.BoundAccountsForMaintenance(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("读取上游绑定账号失败：%w", err)
	}
	accountIDs := accountIDsForHost(bound, host)
	if len(accountIDs) == 0 {
		return "", nil
	}
	task, err := s.enqueueMaintenance(ctx, "account-base-url-sync", "上游账号 Base URL 同步已排队", accountIDs, actor)
	if err != nil {
		return "", err
	}
	return task.ID, nil
}

func (s *Service) EnqueueAllAccountRateSync(ctx context.Context, actor string) (string, error) {
	allowed, err := s.automaticRateSyncAllowed(ctx)
	if err != nil || !allowed {
		return "", err
	}
	bound, err := s.repository.BoundAccountsForMaintenance(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("读取上游绑定账号失败：%w", err)
	}
	accountIDs := uniqueMaintenanceAccountIDs(bound)
	if len(accountIDs) == 0 {
		return "", nil
	}
	task, err := s.EnqueueAccountRateSync(ctx, accountIDs, actor)
	if err != nil {
		return "", err
	}
	return task.ID, nil
}

// EnqueueAccountRateSyncBatch queues a bounded slice for scheduled inspection.
// A rotating cursor prevents the same leading accounts from being selected on
// every heartbeat.
func (s *Service) EnqueueAccountRateSyncBatch(ctx context.Context, batchSize, batchPercent int, actor string) (string, error) {
	allowed, err := s.automaticRateSyncAllowed(ctx)
	if err != nil || !allowed {
		return "", err
	}
	bound, err := s.repository.BoundAccountsForMaintenance(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("读取上游绑定账号失败：%w", err)
	}
	accountIDs := uniqueMaintenanceAccountIDs(bound)
	if len(accountIDs) == 0 {
		return "", nil
	}
	s.rateBatchMu.Lock()
	selected, nextCursor := selectAccountRateBatch(accountIDs, batchSize, batchPercent, s.rateBatchCursor)
	s.rateBatchCursor = nextCursor
	s.rateBatchMu.Unlock()
	task, err := s.EnqueueAccountRateSync(ctx, selected, actor)
	if err != nil {
		return "", err
	}
	return task.ID, nil
}

func selectAccountRateBatch(accountIDs []string, batchSize, batchPercent, cursor int) ([]string, int) {
	if len(accountIDs) == 0 {
		return nil, 0
	}
	if batchSize < 1 {
		if batchPercent > 0 {
			batchSize = (len(accountIDs)*batchPercent + 99) / 100
		} else {
			batchSize = len(accountIDs)
		}
	}
	if batchSize > len(accountIDs) {
		batchSize = len(accountIDs)
	}
	start := cursor % len(accountIDs)
	if start < 0 {
		start = 0
	}
	selected := make([]string, 0, batchSize)
	for index := 0; index < batchSize; index++ {
		selected = append(selected, accountIDs[(start+index)%len(accountIDs)])
	}
	return selected, (start + batchSize) % len(accountIDs)
}

func (s *Service) automaticRateSyncAllowed(ctx context.Context) (bool, error) {
	reader, ok := s.repository.(interface {
		Mode(context.Context) (string, error)
	})
	if !ok {
		return true, nil
	}
	mode, err := reader.Mode(ctx)
	if err != nil {
		return false, err
	}
	return mode == runtimepolicy.Full, nil
}

func accountIDsForHost(bound []business.BoundAccountMaintenance, host string) []string {
	host = configstore.CanonicalHost(host)
	if host == "" {
		return nil
	}
	matched := make([]business.BoundAccountMaintenance, 0, len(bound))
	for _, account := range bound {
		if configstore.CanonicalHost(account.UpstreamHost) != host && configstore.CanonicalHost(account.SourceAuthHost) != host {
			continue
		}
		matched = append(matched, account)
	}
	return uniqueMaintenanceAccountIDs(matched)
}

func uniqueMaintenanceAccountIDs(bound []business.BoundAccountMaintenance) []string {
	seen := make(map[string]struct{}, len(bound))
	result := make([]string, 0, len(bound))
	for _, account := range bound {
		accountID := strings.TrimSpace(account.AccountID)
		if accountID == "" {
			continue
		}
		if _, found := seen[accountID]; found {
			continue
		}
		seen[accountID] = struct{}{}
		result = append(result, accountID)
	}
	return result
}

func (s *Service) SyncAllAccountRates(ctx context.Context, actor string) (map[string]any, error) {
	return s.syncAccountRatesWithCatalog(ctx, nil, actor, nil)
}

// SyncAllAccountRatesWithCatalog reuses catalog snapshots collected earlier in
// the same inspection round. Hosts missing from snapshots are read live.
func (s *Service) SyncAllAccountRatesWithCatalog(ctx context.Context, actor string, catalogs map[string]business.UpstreamCatalogSnapshot) (map[string]any, error) {
	return s.syncAccountRatesWithCatalog(ctx, nil, actor, catalogs)
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
	automatic := mutationguard.IsAutomaticInspection(ctx)
	var expectedTarget configstore.TargetSettings
	var targetErr error
	if operation != "account-upstream-host-repair" {
		expectedTarget, targetErr = targetguard.Expected(ctx, s.targets)
	}
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
	if err := taskrunner.Go(s.taskRunner, func(parent context.Context) {
		// Task runners use a process-level context, so explicitly preserve the
		// lower-priority marker when an automatic inspection queues maintenance.
		parent = preserveAutomaticInspection(parent, automatic)
		if operation != "account-upstream-host-repair" && targetErr == nil {
			parent = targetguard.Expect(parent, expectedTarget)
		}
		s.executeMaintenanceContext(parent, task, operation, accountIDs, actor)
	}); err != nil {
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
	return task, nil
}

func preserveAutomaticInspection(ctx context.Context, automatic bool) context.Context {
	if automatic {
		return mutationguard.WithAutomaticInspection(ctx)
	}
	return ctx
}

func (s *Service) executeMaintenanceContext(parent context.Context, task taskstore.Task, operation string, accountIDs []string, actor string) {
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()
	runningMessage := "读取管理平台账号目录"
	if operation == "account-rate-sync" {
		runningMessage = "正在从上游探测账号有效倍率并写回管理平台"
	} else if operation == "account-base-url-sync" {
		runningMessage = "正在将上游 Base URL 写入全部绑定账号"
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
	if !taskstore.SaveRunning(ctx, s.tasks, task) {
		return
	}
	var result map[string]any
	var err error
	requested := len(accountIDs)
	protected := []map[string]any{}
	if operation != "account-rate-sync" && operation != "account-base-url-sync" {
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
		if _, present := result["remote_write"]; !present {
			result["remote_write"] = false
		}
	} else {
		task.Status = "succeeded"
		if operation == "account-rate-sync" {
			task.Message = fmt.Sprintf("账号倍率同步完成：更新 %v 个，未变 %v 个，跳过 %v 个（其中只读降级 %v 个），缺失 %v 个，失败 %v 个",
				result["updated"], result["unchanged"], result["skipped"], result["fallback"], result["missing"], result["failed"])
			if failed, _ := result["failed"].(int); failed > 0 {
				task.Status = "failed"
				task.Message = fmt.Sprintf("账号倍率同步部分失败：更新 %v 个，未变 %v 个，跳过 %v 个（其中只读降级 %v 个），缺失 %v 个，失败 %v 个",
					result["updated"], result["unchanged"], result["skipped"], result["fallback"], result["missing"], result["failed"])
			}
		} else if operation == "account-base-url-sync" {
			task.Message = fmt.Sprintf("上游账号 Base URL 同步完成：更新 %v 个，未变 %v 个，失败 %v 个", result["updated"], result["unchanged"], result["failed"])
			if failed, _ := result["failed"].(int); failed > 0 {
				task.Status = "failed"
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
	taskstore.MarkCancelled(ctx, &task, "账号维护任务已取消")
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) runMaintenance(ctx context.Context, operation string, accountIDs []string, actor string) (map[string]any, error) {
	switch operation {
	case "account-rate-sync":
		return s.syncAccountRates(ctx, accountIDs, actor)
	case "account-base-url-sync":
		return s.syncAccountBaseURLs(ctx, accountIDs, actor)
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
	if len(accountIDs) == 0 {
		return map[string]any{
			"operation": "account.upstream_host.repair", "requested": 0, "repaired": 0,
			"unchanged": 0, "skipped": 0, "items": []business.AccountUpstreamHostRepairItem{},
			"event_id": int64(0), "actor": actor, "remote_write": false,
		}, nil
	}
	accountIDs, err := normalizeAccountIDs(accountIDs)
	if err != nil {
		return nil, err
	}
	guarded, release, err := s.acquireLocalAccountMutations(ctx, accountIDs, true)
	if err != nil {
		return nil, fmt.Errorf("账号归属 Host 修复锁获取失败：%w", err)
	}
	defer release()

	eligible := make([]string, 0, len(accountIDs))
	protected := make(map[string]business.AccountUpstreamHostRepairItem)
	for _, accountID := range accountIDs {
		reason, protectionErr := s.accountMutationProtectionReason(guarded, accountID, false)
		if protectionErr != nil {
			return nil, fmt.Errorf("账号 %s 归属 Host 修复保护状态复核失败：%w", accountID, protectionErr)
		}
		if reason == nil {
			eligible = append(eligible, accountID)
			continue
		}
		protected[accountID] = business.AccountUpstreamHostRepairItem{
			AccountID: accountID, Status: "人工保护，已跳过", Reason: reason,
		}
	}

	repaired := business.AccountUpstreamHostRepairResult{}
	if len(eligible) > 0 {
		repaired, err = s.repository.RepairAccountUpstreamHosts(guarded, eligible, actor)
		if err != nil {
			return nil, fmt.Errorf("账号归属 Host 修复失败：%w", err)
		}
	}
	itemsByID := make(map[string]business.AccountUpstreamHostRepairItem, len(repaired.Items)+len(protected))
	for _, item := range repaired.Items {
		itemsByID[item.AccountID] = item
	}
	for accountID, item := range protected {
		itemsByID[accountID] = item
	}
	items := make([]business.AccountUpstreamHostRepairItem, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		if item, found := itemsByID[accountID]; found {
			items = append(items, item)
		}
	}
	return map[string]any{
		"operation": "account.upstream_host.repair", "requested": len(accountIDs),
		"repaired": repaired.Repaired, "unchanged": repaired.Unchanged, "skipped": repaired.Skipped + len(protected),
		"items": items, "event_id": repaired.EventID, "actor": actor, "remote_write": false,
	}, nil
}

func (s *Service) validateAccountBaseURLs(ctx context.Context, accountIDs []string, actor string) (map[string]any, error) {
	guarded, releaseAll, err := s.acquireAccountMutations(ctx, accountIDs, false)
	if err != nil {
		return nil, err
	}
	defer releaseAll()
	ctx = guarded
	client, err := s.maintenanceClient(ctx)
	if err != nil {
		return nil, err
	}
	type validationResult struct {
		item      map[string]any
		resolved  bool
		available bool
		skipped   bool
		fatal     bool
		err       error
	}
	results := make([]validationResult, len(accountIDs))
	var commitMu sync.Mutex
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(8, len(accountIDs)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				accountID := accountIDs[index]
				item := map[string]any{"account_id": accountID, "status": "详情读取失败"}
				accountCtx, release, protectedReason, guardErr := s.acquireAccountMutation(ctx, accountID)
				if guardErr != nil {
					item["error"] = guardErr.Error()
					results[index] = validationResult{item: item, fatal: true, err: guardErr}
					continue
				}
				if protectedReason != nil {
					item["status"], item["reason"] = "人工保护，已跳过", *protectedReason
					results[index] = validationResult{item: item, skipped: true}
					continue
				}
				func() {
					defer release()
					row, readErr := client.Account(accountCtx, accountID)
					if readErr != nil {
						item["error"] = readErr.Error()
						results[index] = validationResult{item: item, err: readErr}
						return
					}
					returnedID := strings.TrimSpace(fmt.Sprint(firstValue(row, "id", "account_id")))
					if returnedID != accountID {
						readErr = fmt.Errorf("账号详情返回的稳定 ID 为 %q，与请求账号 %s 不一致", returnedID, accountID)
						item["error"] = readErr.Error()
						results[index] = validationResult{item: item, err: readErr}
						return
					}
					item["account_name"] = strings.TrimSpace(fmt.Sprint(firstValue(row, "name")))
					baseURL, source, available := managementRowBaseURL(row)
					item["status"] = "详情未返回 Base URL"
					observation := business.AccountBaseURLObservation{AccountID: accountID}
					if available {
						item["status"], item["base_url"], item["source"] = "已读取", baseURL, source
						observation.BaseURL, observation.Source = &baseURL, source
					}
					commitMu.Lock()
					commitErr := s.repository.CommitAccountBaseURLObservations(accountCtx, []business.AccountBaseURLObservation{observation})
					commitMu.Unlock()
					if commitErr != nil {
						item["status"], item["error"] = "校验结果保存失败", commitErr.Error()
						results[index] = validationResult{item: item, fatal: true, err: commitErr}
						return
					}
					results[index] = validationResult{item: item, resolved: true, available: available}
				}()
			}
		}()
	}
	for index := range accountIDs {
		jobs <- index
	}
	close(jobs)
	workers.Wait()

	items := make([]map[string]any, 0, len(results))
	resolved, unavailable, skipped, failed := 0, 0, 0, 0
	fatalErrors := make([]error, 0)
	for _, result := range results {
		items = append(items, result.item)
		if result.skipped {
			skipped++
			continue
		}
		if !result.resolved {
			failed++
			if result.fatal && result.err != nil {
				fatalErrors = append(fatalErrors, result.err)
			}
			continue
		}
		if result.available {
			resolved++
		} else {
			unavailable++
		}
	}
	response := map[string]any{
		"operation": "account.base_url.validation", "requested": len(accountIDs),
		"resolved": resolved, "unavailable": unavailable, "skipped": skipped, "failed": failed,
		"items": items, "actor": actor, "remote_write": false, "read_only": true,
	}
	if err := ctx.Err(); err != nil {
		return response, err
	}
	if len(fatalErrors) > 0 {
		return response, fmt.Errorf("Base URL 校验未完整保存：%w", errors.Join(fatalErrors...))
	}
	return response, nil
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
	guarded, releaseAll, err := s.acquireAccountMutations(ctx, accountIDs, false)
	if err != nil {
		return nil, err
	}
	defer releaseAll()
	ctx = guarded
	client, err := s.maintenanceClient(ctx)
	if err != nil {
		return nil, err
	}
	type repairResult struct {
		item    map[string]any
		kind    string
		written bool
	}
	results := make([]repairResult, len(accountIDs))
	var commitMu sync.Mutex
	process := func(index int) {
		accountID := accountIDs[index]
		item := map[string]any{"account_id": accountID, "remote_write": false, "readback_confirmed": false}
		accountCtx, release, protectedReason, guardErr := s.acquireAccountMutation(ctx, accountID)
		if guardErr != nil {
			item["status"], item["error"] = "账号变更锁获取失败", guardErr.Error()
			results[index] = repairResult{item: item, kind: "failed"}
			return
		}
		if protectedReason != nil {
			item["status"], item["reason"] = "人工保护，已跳过", *protectedReason
			results[index] = repairResult{item: item, kind: "skipped"}
			return
		}
		defer release()
		bindings, baselineErr := s.repository.BoundAccountsForMaintenance(accountCtx, []string{accountID})
		if baselineErr != nil {
			item["status"], item["error"] = "账号绑定复读失败", baselineErr.Error()
			results[index] = repairResult{item: item, kind: "failed"}
			return
		}
		targets, hosts := map[string]string{}, map[string]struct{}{}
		for _, account := range bindings {
			if account.AccountID != accountID {
				continue
			}
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
			return
		case len(hosts) != 1 || len(targets) != 1:
			item["status"] = "归属上游不唯一，无法自动修复"
			results[index] = repairResult{item: item, kind: "skipped"}
			return
		}
		var target string
		for _, value := range targets {
			target = value
		}
		item["after"] = target
		row, readErr := client.Account(accountCtx, accountID)
		if readErr != nil {
			item["status"], item["error"] = "账号详情读取失败", readErr.Error()
			results[index] = repairResult{item: item, kind: "failed"}
			return
		}
		returnedID := strings.TrimSpace(fmt.Sprint(firstValue(row, "id", "account_id")))
		if returnedID != accountID {
			item["status"] = "账号详情读取失败"
			item["error"] = fmt.Sprintf("账号详情返回的稳定 ID 为 %q，与请求账号 %s 不一致", returnedID, accountID)
			results[index] = repairResult{item: item, kind: "failed"}
			return
		}
		current, source, available := managementRowBaseURL(row)
		if available {
			item["before"] = current
		}
		explicitTarget := source == "explicit" && sameBaseURL(current, target)
		if source == "explicit" && !explicitTarget {
			item["status"] = "已有显式 Base URL，未覆盖"
			commitMu.Lock()
			commitErr := s.repository.CommitAccountBaseURLObservations(accountCtx, []business.AccountBaseURLObservation{{
				AccountID: accountID, BaseURL: &current, Source: source,
			}})
			commitMu.Unlock()
			if commitErr != nil {
				item["status"], item["error"] = "Base URL 结果保存失败", commitErr.Error()
				results[index] = repairResult{item: item, kind: "failed"}
				return
			}
			results[index] = repairResult{item: item, kind: "skipped"}
			return
		}
		if !explicitTarget && (!available || source != "platform_default") {
			item["status"] = "账号类型未提供可修复的默认 Base URL"
			results[index] = repairResult{item: item, kind: "skipped"}
			return
		}
		remoteWritten := false
		if !explicitTarget {
			if _, updateErr := client.UpdateAccount(accountCtx, accountID, map[string]any{
				"credentials": map[string]any{"base_url": target},
			}); updateErr != nil {
				item["status"], item["error"] = "Base URL 修复失败", updateErr.Error()
				results[index] = repairResult{item: item, kind: "failed"}
				return
			}
			remoteWritten = true
		}
		recoveredRow, recoverErr := client.RecoverAccountState(accountCtx, accountID)
		if recoverErr != nil {
			item["status"], item["error"] = "账号状态恢复失败", recoverErr.Error()
			results[index] = repairResult{item: item, kind: "failed", written: remoteWritten}
			return
		}
		remoteWritten = true
		if !strings.EqualFold(strings.TrimSpace(fmt.Sprint(firstValue(recoveredRow, "status"))), "active") {
			if _, statusErr := client.UpdateAccount(accountCtx, accountID, map[string]any{"status": "active"}); statusErr != nil {
				item["status"], item["error"] = "账号状态恢复失败", statusErr.Error()
				results[index] = repairResult{item: item, kind: "failed", written: true}
				return
			}
		}
		if _, schedulingErr := client.SetAccountSchedulable(accountCtx, accountID, true); schedulingErr != nil {
			item["status"], item["error"] = "调度开启失败", schedulingErr.Error()
			results[index] = repairResult{item: item, kind: "failed", written: true}
			return
		}
		item["remote_write"] = true
		confirmedRow, confirmErr := client.Account(accountCtx, accountID)
		if confirmErr != nil {
			item["status"], item["error"] = "写后确认失败", confirmErr.Error()
			results[index] = repairResult{item: item, kind: "failed", written: true}
			return
		}
		confirmedID := strings.TrimSpace(fmt.Sprint(firstValue(confirmedRow, "id", "account_id")))
		if confirmedID != accountID {
			item["status"] = "写后确认失败"
			item["error"] = fmt.Sprintf("账号读回的稳定 ID 为 %q，与请求账号 %s 不一致", confirmedID, accountID)
			results[index] = repairResult{item: item, kind: "failed", written: true}
			return
		}
		confirmed, confirmedSource, confirmedAvailable := managementRowBaseURL(confirmedRow)
		confirmedStatus := strings.ToLower(strings.TrimSpace(fmt.Sprint(firstValue(confirmedRow, "status"))))
		confirmedSchedulable, schedulableOK := confirmedRow["schedulable"].(bool)
		if !confirmedAvailable || confirmedSource != "explicit" || !sameBaseURL(confirmed, target) ||
			confirmedStatus != "active" || !schedulableOK || !confirmedSchedulable {
			item["status"], item["error"] = "写后确认失败", "Base URL、账号状态或调度状态未全部恢复"
			results[index] = repairResult{item: item, kind: "failed", written: true}
			return
		}
		commitMu.Lock()
		commitErr := s.repository.CommitAccountBaseURLObservations(accountCtx, []business.AccountBaseURLObservation{{
			AccountID: accountID, BaseURL: &confirmed, Source: confirmedSource,
		}})
		commitMu.Unlock()
		if commitErr != nil {
			item["status"], item["error"] = "Base URL 修复结果保存失败", commitErr.Error()
			results[index] = repairResult{item: item, kind: "failed", written: true}
			return
		}
		item["status"], item["readback_confirmed"] = "已修复并恢复调度", true
		results[index] = repairResult{item: item, kind: "repaired", written: true}
	}
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(4, len(accountIDs)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				process(index)
			}
		}()
	}
	for index := range accountIDs {
		jobs <- index
	}
	close(jobs)
	workers.Wait()

	items := make([]map[string]any, 0, len(results))
	repaired, unchanged, skipped, failed, written := 0, 0, 0, 0, 0
	for _, result := range results {
		items = append(items, result.item)
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
	if failed > 0 {
		return result, errors.New("部分账号 Base URL 修复失败，请查看明细")
	}
	return result, nil
}

func (s *Service) syncAccountBaseURLs(ctx context.Context, accountIDs []string, actor string) (map[string]any, error) {
	if len(accountIDs) == 0 {
		return map[string]any{
			"operation": "account.base_url.sync", "requested": 0, "updated": 0,
			"unchanged": 0, "failed": 0, "items": []map[string]any{}, "actor": actor, "remote_write": false,
		}, nil
	}
	guarded, release, err := s.acquireAccountMutations(ctx, accountIDs, false)
	if err != nil {
		return nil, err
	}
	defer release()
	ctx = guarded
	bound, err := s.repository.BoundAccountsForMaintenance(ctx, accountIDs)
	if err != nil {
		return nil, fmt.Errorf("账号绑定读取失败：%w", err)
	}
	client, err := s.maintenanceClient(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string][]business.BoundAccountMaintenance, len(accountIDs))
	for _, account := range bound {
		byID[account.AccountID] = append(byID[account.AccountID], account)
	}
	items := make([]map[string]any, 0, len(accountIDs))
	updated, unchanged, failed := 0, 0, 0
	for _, accountID := range accountIDs {
		item := map[string]any{"account_id": accountID}
		targets := map[string]string{}
		for _, account := range byID[accountID] {
			item["account_name"] = account.AccountName
			if target, ok := validAccountRepairBaseURL(account.NamingBaseURL); ok {
				targets[comparableBaseURL(target)] = target
			}
		}
		if len(targets) != 1 {
			item["status"] = "上游 Base URL 不唯一或无效"
			failed++
			items = append(items, item)
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
			failed++
			items = append(items, item)
			continue
		}
		returnedID := strings.TrimSpace(fmt.Sprint(firstValue(row, "id", "account_id")))
		if returnedID != accountID {
			item["status"], item["error"] = "账号详情读取失败", fmt.Sprintf("账号详情返回的稳定 ID 为 %q", returnedID)
			failed++
			items = append(items, item)
			continue
		}
		current, _, available := managementRowBaseURL(row)
		if available {
			item["before"] = current
		}
		if available && sameBaseURL(current, target) {
			if commitErr := s.repository.CommitAccountBaseURLObservations(ctx, []business.AccountBaseURLObservation{{
				AccountID: accountID, BaseURL: &target, Source: "explicit",
			}}); commitErr != nil {
				item["status"], item["error"] = "Base URL 结果保存失败", commitErr.Error()
				failed++
			} else {
				item["status"] = "无需更新"
				unchanged++
			}
			items = append(items, item)
			continue
		}
		if _, updateErr := client.UpdateAccount(ctx, accountID, map[string]any{
			"credentials": map[string]any{"base_url": target},
		}); updateErr != nil {
			item["status"], item["error"] = "Base URL 写入失败", updateErr.Error()
			failed++
			items = append(items, item)
			continue
		}
		confirmedRow, confirmErr := client.Account(ctx, accountID)
		if confirmErr != nil {
			item["status"], item["error"] = "Base URL 写后读取失败", confirmErr.Error()
			item["remote_write"] = true
			failed++
			items = append(items, item)
			continue
		}
		confirmedID := strings.TrimSpace(fmt.Sprint(firstValue(confirmedRow, "id", "account_id")))
		confirmed, confirmedSource, confirmedAvailable := managementRowBaseURL(confirmedRow)
		if confirmedID != accountID || !confirmedAvailable || confirmedSource != "explicit" || !sameBaseURL(confirmed, target) {
			item["status"], item["error"] = "Base URL 写后确认失败", "管理平台读回值与上游配置不一致"
			item["remote_write"] = true
			failed++
			items = append(items, item)
			continue
		}
		if commitErr := s.repository.CommitAccountBaseURLObservations(ctx, []business.AccountBaseURLObservation{{
			AccountID: accountID, BaseURL: &confirmed, Source: confirmedSource,
		}}); commitErr != nil {
			item["status"], item["error"] = "Base URL 同步结果保存失败", commitErr.Error()
			item["remote_write"] = true
			failed++
			items = append(items, item)
			continue
		}
		item["status"], item["remote_write"], item["readback_confirmed"] = "已更新", true, true
		updated++
		items = append(items, item)
	}
	result := map[string]any{
		"operation": "account.base_url.sync", "requested": len(accountIDs), "updated": updated,
		"unchanged": unchanged, "failed": failed, "items": items, "actor": actor, "remote_write": updated > 0,
	}
	if failed > 0 {
		return result, errors.New("部分账号 Base URL 同步失败，请查看明细")
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
	return s.syncAccountRatesWithCatalog(ctx, accountIDs, actor, nil)
}

func (s *Service) syncAccountRatesWithCatalog(ctx context.Context, accountIDs []string, actor string, catalogSnapshots map[string]business.UpstreamCatalogSnapshot) (map[string]any, error) {
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
			"updated": 0, "unchanged": 0, "skipped": 0, "fallback": 0, "missing": 0, "failed": 0,
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
		ready            chan struct{}
		snapshot         business.UpstreamCatalogSnapshot
		fallbackEligible bool
		err              error
	}
	loads := map[string]*catalogLoad{}
	var loadsMu sync.Mutex
	loadCatalog := func(run context.Context, host string) (business.UpstreamCatalogSnapshot, bool, error) {
		host = configstore.CanonicalHost(host)
		if snapshot, found := catalogSnapshots[host]; found {
			return snapshot, false, nil
		}
		loadsMu.Lock()
		if existing := loads[host]; existing != nil {
			loadsMu.Unlock()
			select {
			case <-existing.ready:
				return existing.snapshot, existing.fallbackEligible, existing.err
			case <-run.Done():
				return business.UpstreamCatalogSnapshot{}, false, run.Err()
			}
		}
		load := &catalogLoad{ready: make(chan struct{})}
		loads[host] = load
		loadsMu.Unlock()
		defer close(load.ready)
		if s.upstreams == nil {
			load.err = errors.New("NewAPI 上游目录读取服务尚未就绪")
			return load.snapshot, false, load.err
		}
		auths, ok := s.targets.(upstreamAuthStore)
		if !ok {
			load.err = errors.New("NewAPI 私有授权读取服务尚未就绪")
			return load.snapshot, false, load.err
		}
		record, authErr := auths.AuthRecord(run, host)
		if authErr != nil {
			load.err = authErr
			return load.snapshot, false, load.err
		}
		if record == nil {
			if s.resolver != nil {
				record, authErr = s.resolver.ResolveAuth(run, host, actor)
				if authErr != nil {
					load.err = fmt.Errorf("Host %q 的私有授权恢复失败：%w", host, authErr)
					return load.snapshot, false, load.err
				}
			}
			if record == nil {
				load.err = fmt.Errorf("未找到 Host %q 的私有授权记录", host)
				return load.snapshot, false, load.err
			}
		}
		load.snapshot, load.err = s.upstreams.ReadCatalog(run, *record)
		load.fallbackEligible = fallbackEligibleRateReadError(load.err)
		return load.snapshot, load.fallbackEligible, load.err
	}

	upstreamRates := make([]accountRateProbe, len(accountIDs))
	sub2APIIDs := make([]string, 0, len(accountIDs))
	newAPIIndexes := make([]int, 0, len(accountIDs))
	for index, accountID := range accountIDs {
		upstreamRates[index].account = business.BoundAccountMaintenance{
			AccountID: accountID, AccountName: localNames[accountID],
		}
		bindings := byID[accountID]
		switch len(bindings) {
		case 0:
			upstreamRates[index].err = errAccountRateBindingMissing
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
		targetCtx, captureErr := targetguard.Capture(ctx, s.targets)
		if captureErr != nil {
			return nil, captureErr
		}
		readCtx, releaseRead, acquireErr := targetguard.Acquire(targetCtx, s.repository)
		if acquireErr != nil {
			return nil, acquireErr
		}
		readCtx, err = targetguard.Bind(readCtx, s.targets)
		if err != nil {
			_ = releaseRead()
			return nil, err
		}
		readClient, clientErr := s.maintenanceClient(readCtx)
		if clientErr != nil {
			_ = releaseRead()
			return nil, clientErr
		}
		batch, batchErr := readClient.AccountUpstreamMultipliers(readCtx, sub2APIIDs)
		if releaseErr := releaseRead(); releaseErr != nil {
			return nil, fmt.Errorf("管理倍率探测目标租约释放失败：%w", releaseErr)
		}
		ctx = targetCtx
		for index, accountID := range accountIDs {
			if upstreamRates[index].err != nil || upstreamRates[index].skippedReason != "" || isNewAPIType(upstreamRates[index].account.UpstreamType) {
				continue
			}
			if batchErr != nil {
				upstreamRates[index].err = batchErr
				upstreamRates[index].fallbackEligible = fallbackEligibleRateReadError(batchErr)
				continue
			}
			item, found := batch[accountID]
			if !found {
				upstreamRates[index].err = errors.New("批量上游倍率探测未返回该账号结果")
				upstreamRates[index].fallbackEligible = true
				continue
			}
			if item.Err != nil {
				upstreamRates[index].err = item.Err
				upstreamRates[index].fallbackEligible = fallbackEligibleRateReadError(item.Err)
				continue
			}
			upstreamRates[index].observedMultiplier = item.Multiplier
			rechargeRate := strings.TrimSpace(upstreamRates[index].account.RechargeRate)
			if rechargeRate == "" {
				rechargeRate = "1"
			}
			upstreamRates[index].multiplier, upstreamRates[index].err = business.ConvertMultiplier(item.Multiplier, rechargeRate)
			if upstreamRates[index].err != nil {
				upstreamRates[index].err = fmt.Errorf("Sub2API 上游折算倍率无效: %w", upstreamRates[index].err)
			}
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
				catalog, fallbackEligible, catalogErr := loadCatalog(ctx, account.RateSourceHost())
				if catalogErr != nil {
					upstreamRates[index].err = catalogErr
					upstreamRates[index].fallbackEligible = fallbackEligible
					continue
				}
				upstreamRates[index].observedMultiplier, upstreamRates[index].multiplier, upstreamRates[index].err = newAPIAccountRates(account, catalog)
			}
		}()
	}
	for _, index := range newAPIIndexes {
		newAPIJobs <- index
	}
	close(newAPIJobs)
	probeWorkers.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for index := range upstreamRates {
		applyStoredRateFallback(&upstreamRates[index])
	}
	liveIndexes := make([]int, 0, len(upstreamRates))
	liveAccountIDs := make([]string, 0, len(upstreamRates))
	for index, probe := range upstreamRates {
		if probe.err == nil && probe.skippedReason == "" && !probe.fallback {
			liveIndexes = append(liveIndexes, index)
			liveAccountIDs = append(liveAccountIDs, probe.account.AccountID)
		}
	}
	var client *adminclient.Client
	writeCtx := ctx
	var releaseManagement func()
	managementGuarded := false
	releaseManagementGuard := func() {
		if releaseManagement == nil {
			return
		}
		releaseManagement()
		releaseManagement = nil
	}
	ensureManagementGuard := func() error {
		if managementGuarded {
			return nil
		}
		resources := accountMutationResources(liveAccountIDs, false)
		for _, index := range liveIndexes {
			rateSourceResource := mutationguard.Upstream(upstreamRates[index].account.RateSourceHost())
			if rateSourceResource == "" {
				return fmt.Errorf("账号 %s 倍率同步来源 Host 无效", upstreamRates[index].account.AccountID)
			}
			resources = append(resources, rateSourceResource)
		}
		guarded, release, guardErr := s.acquireAccountMutationResources(ctx, liveAccountIDs, resources)
		if guardErr != nil {
			return guardErr
		}
		ctx = guarded
		releaseManagement = release
		managementGuarded = true
		return nil
	}
	defer releaseManagementGuard()
	loadManagementClient := func() (*adminclient.Client, error) {
		if client != nil {
			return client, nil
		}
		loaded, loadErr := s.maintenanceClient(ctx)
		if loadErr != nil {
			return nil, loadErr
		}
		client = loaded
		return client, nil
	}
	managementObservation := false
	for _, index := range liveIndexes {
		if !isNewAPIType(upstreamRates[index].account.UpstreamType) {
			managementObservation = true
			break
		}
	}
	if managementObservation {
		if err := ensureManagementGuard(); err != nil {
			return nil, err
		}
	}
	if len(liveIndexes) > 0 {
		guarded, release, guardErr := s.acquireLocalAccountMutations(ctx, liveAccountIDs, false)
		if guardErr != nil {
			return nil, fmt.Errorf("上游倍率观测账号锁获取失败：%w", guardErr)
		}
		commitErr := func() error {
			defer release()
			currentBindings, baselineErr := s.repository.BoundAccountsForMaintenance(guarded, liveAccountIDs)
			if baselineErr != nil {
				return fmt.Errorf("倍率观测保存前绑定复读失败：%w", baselineErr)
			}
			currentByID := make(map[string][]business.BoundAccountMaintenance, len(currentBindings))
			for _, binding := range currentBindings {
				currentByID[binding.AccountID] = append(currentByID[binding.AccountID], binding)
			}
			observations := make([]business.AccountRateObservation, 0, len(liveIndexes))
			for _, index := range liveIndexes {
				probe := &upstreamRates[index]
				bindings := currentByID[probe.account.AccountID]
				if len(bindings) != 1 {
					probe.err = errors.New("账号绑定在上游探测后发生变化，无法安全保存倍率观测")
					continue
				}
				current := bindings[0]
				if !sameRateObservationBaseline(probe.account, current) {
					probe.err = errors.New("账号绑定在上游探测后发生变化，请重试倍率同步")
					continue
				}
				probe.account = current
				probe.manualMultiplierOnly = current.ManualPriority
				if current.ManualPriority && !current.SyncBalanceMultiplier {
					probe.skippedReason = "人工控制账号未开启余额与倍率同步"
					continue
				}
				protectedReason, protectionErr := s.accountMutationProtectionReason(
					guarded,
					current.AccountID,
					current.ManualPriority && current.SyncBalanceMultiplier,
				)
				if protectionErr != nil {
					return fmt.Errorf("账号 %s 倍率观测保护状态复核失败：%w", current.AccountID, protectionErr)
				}
				if protectedReason != nil {
					probe.skippedReason = *protectedReason
					continue
				}
				observations = append(observations, business.AccountRateObservation{
					AccountID: current.AccountID, Rate: probe.observedMultiplier,
				})
			}
			if len(observations) == 0 {
				return nil
			}
			return s.repository.CommitAccountRateObservations(guarded, observations)
		}()
		if commitErr != nil {
			return nil, fmt.Errorf("上游倍率观测保存失败：%w", commitErr)
		}
	}

	// Read the management catalog only when at least one live observation can
	// actually be compared and written. Manual skips, read-only fallbacks, and
	// permanent probe failures do not need it.
	needsRemoteCatalog := false
	for _, probe := range upstreamRates {
		if probe.err == nil && probe.skippedReason == "" && !probe.fallback {
			needsRemoteCatalog = true
			break
		}
	}
	remoteByID := map[string]map[string]any{}
	if needsRemoteCatalog {
		if err := ensureManagementGuard(); err != nil {
			return nil, err
		}
		client, err = loadManagementClient()
		if err != nil {
			return nil, err
		}
		remoteRows, accountsErr := client.Accounts(ctx)
		if accountsErr != nil {
			return nil, fmt.Errorf("管理平台账号目录读取失败：%w", accountsErr)
		}
		remoteByID = make(map[string]map[string]any, len(remoteRows))
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
	}
	// The catalog snapshot and local observations are complete. Per-account
	// writers acquire their own account and rate-source leases below.
	releaseManagementGuard()
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
					item["status"], item["reason"] = "人工保护，已跳过", probe.skippedReason
					results[index] = rateResult{item: item, skipped: true}
					continue
				}
				if probe.fallback {
					item["observation_source"] = probe.fallbackSource
					item["probe_error"] = probe.err.Error()
					item["upstream_raw_multiplier"] = probe.observedMultiplier
					item["recharge_rate"] = account.RechargeRate
					item["account_multiplier"] = probe.multiplier
					item["read_only"] = true
					item["status"] = "只读降级，已跳过写回"
					results[index] = rateResult{item: item, skipped: true}
					continue
				} else {
					item["observation_source"] = "live"
				}
				if errors.Is(probe.err, errAccountRateBindingMissing) {
					item["status"] = "未绑定"
					results[index] = rateResult{item: item, missing: true}
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
				item["upstream_raw_multiplier"] = probe.observedMultiplier
				item["recharge_rate"] = account.RechargeRate
				item["account_multiplier"] = probe.multiplier
				nameMatches := probe.manualMultiplierOnly || (remoteName == expectedName && account.AccountName == expectedName)
				if remoteMultiplier == probe.multiplier && sameRate(account.CurrentMultiplier, probe.multiplier) && nameMatches {
					item["status"] = "已确认一致"
					results[index] = rateResult{item: item, unchanged: true}
					continue
				}
				checkCurrent := func(guarded context.Context) error {
					return s.validateAccountRateWriteCurrent(
						guarded,
						account,
						probe.observedMultiplier,
						probe.manualMultiplierOnly,
					)
				}
				var writeResult map[string]any
				var writeErr error
				rateSourceHost := account.RateSourceHost()
				if probe.manualMultiplierOnly {
					writeResult, writeErr = s.rateWriter.SyncAccountMultiplierIfCurrent(writeCtx, accountID, probe.multiplier, actor, rateSourceHost, checkCurrent)
				} else {
					writeResult, writeErr = s.rateWriter.SyncAccountRateIfCurrent(writeCtx, accountID, expectedName, probe.multiplier, actor, rateSourceHost, checkCurrent)
				}
				if writeErr != nil {
					var skipped *accountRateWriteSkippedError
					if errors.As(writeErr, &skipped) {
						item["status"], item["reason"] = "状态已变化，已跳过", skipped.reason
						results[index] = rateResult{item: item, skipped: true}
						continue
					}
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
	updated, unchanged, skipped, missing, failed, written, fallback := 0, 0, 0, 0, 0, 0, 0
	for _, result := range results {
		items = append(items, result.item)
		if source := result.item["observation_source"]; source == "last_successful" || source == "group_catalog" {
			fallback++
		}
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
		"fallback": fallback, "items": items, "read_only": false, "remote_write": written > 0,
	}, nil
}

func applyStoredRateFallback(probe *accountRateProbe) {
	if probe == nil || probe.err == nil || !probe.fallbackEligible {
		return
	}
	raw := strings.TrimSpace(probe.account.KnownRawRate)
	if raw == "" {
		return
	}
	rechargeRate := strings.TrimSpace(probe.account.RechargeRate)
	if rechargeRate == "" {
		rechargeRate = "1"
	}
	converted, err := business.ConvertMultiplier(raw, rechargeRate)
	if err != nil {
		return
	}
	probe.observedMultiplier = raw
	probe.multiplier = converted
	probe.fallback = true
	probe.fallbackSource = "last_successful"
	if probe.account.KnownRawRateSource == "group_catalog" {
		probe.fallbackSource = "group_catalog"
	}
}

func fallbackEligibleRateReadError(err error) bool {
	return err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

func sameRate(left, right string) bool {
	leftRate, leftOK := new(big.Rat).SetString(strings.TrimSpace(left))
	rightRate, rightOK := new(big.Rat).SetString(strings.TrimSpace(right))
	return leftOK && rightOK && leftRate.Cmp(rightRate) == 0
}

func sameRateObservationBaseline(left, right business.BoundAccountMaintenance) bool {
	return strings.TrimSpace(left.AccountID) == strings.TrimSpace(right.AccountID) &&
		strings.EqualFold(strings.TrimSpace(left.UpstreamHost), strings.TrimSpace(right.UpstreamHost)) &&
		strings.EqualFold(strings.TrimSpace(left.SourceAuthHost), strings.TrimSpace(right.SourceAuthHost)) &&
		strings.EqualFold(strings.TrimSpace(left.UpstreamType), strings.TrimSpace(right.UpstreamType)) &&
		strings.TrimSpace(left.UpstreamKeyID) == strings.TrimSpace(right.UpstreamKeyID) &&
		strings.TrimSpace(left.UpstreamGroupID) == strings.TrimSpace(right.UpstreamGroupID) &&
		sameRateOrText(left.RechargeRate, right.RechargeRate)
}

func sameRateOrText(left, right string) bool {
	return sameRate(left, right) || strings.TrimSpace(left) == strings.TrimSpace(right)
}

func sameRateWriteBaseline(left, right business.BoundAccountMaintenance, multiplierOnly bool) bool {
	if !sameRateObservationBaseline(left, right) ||
		!sameRateOrText(left.CurrentMultiplier, right.CurrentMultiplier) ||
		left.ManualPriority != right.ManualPriority ||
		left.SyncBalanceMultiplier != right.SyncBalanceMultiplier {
		return false
	}
	if multiplierOnly {
		return true
	}
	return strings.TrimSpace(left.AccountName) == strings.TrimSpace(right.AccountName) &&
		strings.TrimSpace(left.NamingSiteName) == strings.TrimSpace(right.NamingSiteName) &&
		strings.TrimSpace(left.NamingBaseURL) == strings.TrimSpace(right.NamingBaseURL)
}

func (s *Service) validateAccountRateWriteCurrent(
	ctx context.Context,
	expected business.BoundAccountMaintenance,
	observedMultiplier string,
	multiplierOnly bool,
) error {
	currentBindings, err := s.repository.BoundAccountsForMaintenance(ctx, []string{expected.AccountID})
	if err != nil {
		return fmt.Errorf("倍率写回前绑定复读失败：%w", err)
	}
	if len(currentBindings) != 1 {
		return &accountRateWriteSkippedError{reason: "账号绑定在上游探测后发生变化，请重新同步倍率"}
	}
	current := currentBindings[0]
	if !sameRateWriteBaseline(expected, current, multiplierOnly) {
		return &accountRateWriteSkippedError{reason: "账号倍率来源或目标字段在上游探测后发生变化，请重新同步倍率"}
	}
	if !strings.EqualFold(strings.TrimSpace(current.KnownRawRateSource), "account_observation") ||
		!sameRate(current.KnownRawRate, observedMultiplier) {
		return &accountRateWriteSkippedError{reason: "账号上游倍率观测已被更新，请重新同步倍率"}
	}
	protectedReason, err := s.accountMutationProtectionReason(
		ctx,
		current.AccountID,
		current.ManualPriority && current.SyncBalanceMultiplier,
	)
	if err != nil {
		return fmt.Errorf("倍率写回前保护状态复核失败：%w", err)
	}
	if protectedReason != nil {
		return &accountRateWriteSkippedError{reason: *protectedReason}
	}
	return nil
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

func newAPIAccountRates(account business.BoundAccountMaintenance, catalog business.UpstreamCatalogSnapshot) (string, string, error) {
	var matched *business.UpstreamCatalogKey
	for index := range catalog.Keys {
		if strings.TrimSpace(catalog.Keys[index].KeyID) != strings.TrimSpace(account.UpstreamKeyID) {
			continue
		}
		if matched != nil {
			return "", "", errors.New("NewAPI 上游返回重复的稳定 Token ID")
		}
		matched = &catalog.Keys[index]
	}
	if matched == nil {
		return "", "", errors.New("NewAPI 上游未找到绑定的稳定 Token ID")
	}
	if matched.RateAmbiguous {
		return "", "", errors.New("NewAPI Token 使用多分组路由，无法判定单一倍率")
	}
	groupID := strings.TrimSpace(account.UpstreamGroupID)
	if matched.UpstreamGroup != nil && strings.TrimSpace(*matched.UpstreamGroup) != "" {
		groupID = strings.TrimSpace(*matched.UpstreamGroup)
	}
	if groupID == "" || strings.EqualFold(groupID, "auto") {
		return "", "", errors.New("NewAPI Token 未绑定唯一固定分组，无法判定单一倍率")
	}
	var rawRate *string
	for _, group := range catalog.Groups {
		if strings.TrimSpace(group.GroupID) == groupID || strings.TrimSpace(group.Name) == groupID {
			if rawRate != nil {
				return "", "", fmt.Errorf("NewAPI 上游分组 %q 不唯一", groupID)
			}
			rawRate = group.RawRate
		}
	}
	if rawRate == nil || strings.TrimSpace(*rawRate) == "" {
		return "", "", fmt.Errorf("NewAPI 上游分组 %q 未返回有效倍率", groupID)
	}
	text, err := business.ConvertMultiplier(*rawRate, account.RechargeRate)
	if err != nil {
		return "", "", fmt.Errorf("NewAPI 上游折算倍率无效: %w", err)
	}
	return *rawRate, text, nil
}

func (s *Service) maintenanceClient(ctx context.Context) (*adminclient.Client, error) {
	target, err := targetguard.Settings(ctx, s.targets)
	if err != nil {
		return nil, err
	}
	return adminclient.New(adminclient.Config{BaseURL: target.BaseURL, AdminKey: target.AdminKey,
		Timeout: time.Duration(target.TimeoutSeconds) * time.Second, Attempts: 3}, nil)
}

func (s *Service) revalidateAccounts(ctx context.Context, accountIDs []string, actor string) (map[string]any, error) {
	guarded, release, err := s.acquireAccountMutations(ctx, accountIDs, true)
	if err != nil {
		return nil, err
	}
	defer release()
	bound, err := s.repository.BoundAccountsForMaintenance(guarded, accountIDs)
	if err != nil {
		return nil, fmt.Errorf("获取账号租约后绑定目录复读失败：%w", err)
	}
	bound = uniqueBoundAccounts(bound)
	client, err := s.maintenanceClient(guarded)
	if err != nil {
		return nil, err
	}
	remoteRows, err := client.Accounts(guarded)
	if err != nil {
		return nil, fmt.Errorf("管理平台账号目录读取失败：%w", err)
	}
	remote := make(map[string]map[string]any, len(remoteRows))
	for _, row := range remoteRows {
		accountID := strings.TrimSpace(fmt.Sprint(firstValue(row, "id", "account_id")))
		if accountID == "" {
			return nil, errors.New("管理平台账号目录包含缺少稳定 ID 的记录")
		}
		if _, duplicate := remote[accountID]; duplicate {
			return nil, fmt.Errorf("管理平台返回重复账号 ID：%s", accountID)
		}
		remote[accountID] = row
	}
	items := make([]map[string]any, 0, len(bound))
	commits := make([]business.BindingVerification, 0, len(bound))
	verified, missing, skipped := 0, 0, 0
	for _, account := range bound {
		protectedReason, protectionErr := s.accountMutationProtectionReason(guarded, account.AccountID, false)
		if protectionErr != nil {
			return nil, fmt.Errorf("账号 %s 复验保护状态复核失败：%w", account.AccountID, protectionErr)
		}
		if protectedReason != nil {
			skipped++
			items = append(items, map[string]any{
				"account_id": account.AccountID, "account_name": account.AccountName,
				"upstream_host": account.UpstreamHost, "status": "人工保护，已跳过", "reason": *protectedReason,
			})
			continue
		}
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
	if len(commits) > 0 {
		if err := s.repository.CommitBindingVerification(guarded, commits); err != nil {
			return nil, fmt.Errorf("复验结果保存失败：%w", err)
		}
	}
	return map[string]any{"operation": "account.binding.revalidation", "requested": len(accountIDs), "bound": len(bound),
		"verified": verified, "missing": missing, "skipped": skipped, "items": items, "actor": actor, "remote_write": false}, nil
}

func (s *Service) repairAccountNames(ctx context.Context, accountIDs []string, actor string) (map[string]any, error) {
	bound, err := s.repository.BoundAccountsForMaintenance(ctx, accountIDs)
	if err != nil {
		return nil, err
	}
	bound = uniqueBoundAccounts(bound)
	guarded, releaseAll, err := s.acquireAccountMutations(ctx, accountIDs, false)
	if err != nil {
		return nil, err
	}
	defer releaseAll()
	ctx = guarded
	client, err := s.maintenanceClient(ctx)
	if err != nil {
		return nil, err
	}
	type repairResult struct {
		item          map[string]any
		kind          string
		remoteWritten bool
	}
	results := make([]repairResult, len(bound))
	var commitMu sync.Mutex
	jobs := make(chan int)
	var workers sync.WaitGroup
	workerCount := min(4, len(bound))
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				account := bound[index]
				item := map[string]any{
					"account_id": account.AccountID, "account_name": account.AccountName,
					"upstream_host": account.UpstreamHost, "remote_write": false, "readback_confirmed": false,
				}
				accountCtx, release, protectedReason, guardErr := s.acquireAccountMutation(ctx, account.AccountID)
				if guardErr != nil {
					item["status"], item["error"] = "修复失败", guardErr.Error()
					results[index] = repairResult{item: item, kind: "failed"}
					continue
				}
				if protectedReason != nil {
					item["status"], item["reason"] = "人工保护，已跳过", *protectedReason
					results[index] = repairResult{item: item, kind: "skipped"}
					continue
				}
				func() {
					defer release()
					currentBindings, baselineErr := s.repository.BoundAccountsForMaintenance(accountCtx, []string{account.AccountID})
					if baselineErr != nil {
						item["status"], item["error"] = "修复失败", "账号命名依据复读失败："+baselineErr.Error()
						results[index] = repairResult{item: item, kind: "failed"}
						return
					}
					expectedNames := map[string]struct{}{}
					for _, currentBinding := range currentBindings {
						if currentBinding.AccountID != account.AccountID {
							continue
						}
						item["account_name"], item["upstream_host"] = currentBinding.AccountName, currentBinding.UpstreamHost
						if expected := strings.TrimSpace(currentBinding.ExpectedName); expected != "" {
							expectedNames[expected] = struct{}{}
						}
					}
					if len(currentBindings) == 0 {
						item["status"] = "没有绑定，已跳过"
						results[index] = repairResult{item: item, kind: "skipped"}
						return
					}
					if len(expectedNames) != 1 {
						item["status"], item["error"] = "修复失败", "账号当前绑定无法确定唯一命名依据"
						results[index] = repairResult{item: item, kind: "failed"}
						return
					}
					expectedName := ""
					for value := range expectedNames {
						expectedName = value
					}
					item["after"] = expectedName
					current, readErr := client.Account(accountCtx, account.AccountID)
					if readErr != nil {
						var httpError *adminclient.HTTPError
						if errors.As(readErr, &httpError) && httpError.StatusCode == http.StatusNotFound {
							commitMu.Lock()
							commitErr := s.repository.CommitBindingVerification(accountCtx, []business.BindingVerification{{AccountID: account.AccountID, Exists: false}})
							commitMu.Unlock()
							if commitErr != nil {
								item["status"], item["error"] = "修复失败", "缺失状态保存失败："+commitErr.Error()
								results[index] = repairResult{item: item, kind: "failed"}
								return
							}
							item["status"] = "管理平台不存在"
							results[index] = repairResult{item: item, kind: "missing"}
							return
						}
						item["status"], item["error"] = "修复失败", "账号锁定后读回失败："+readErr.Error()
						results[index] = repairResult{item: item, kind: "failed"}
						return
					}
					returnedID := strings.TrimSpace(fmt.Sprint(firstValue(current, "id", "account_id")))
					if returnedID != account.AccountID {
						item["status"] = "修复失败"
						item["error"] = fmt.Sprintf("账号详情返回的稳定 ID 为 %q，与请求账号 %s 不一致", returnedID, account.AccountID)
						results[index] = repairResult{item: item, kind: "failed"}
						return
					}
					before := strings.TrimSpace(fmt.Sprint(firstValue(current, "name")))
					item["before"] = before
					if before == expectedName {
						commitMu.Lock()
						commitErr := s.repository.CommitBindingVerification(accountCtx, []business.BindingVerification{{AccountID: account.AccountID, Exists: true}})
						commitMu.Unlock()
						if commitErr != nil {
							item["status"], item["error"] = "修复失败", "绑定状态保存失败："+commitErr.Error()
							results[index] = repairResult{item: item, kind: "failed"}
							return
						}
						item["status"] = "无需修复"
						results[index] = repairResult{item: item, kind: "unchanged"}
						return
					}
					_, writeErr := client.Mutate(accountCtx, http.MethodPut, "/admin/accounts/"+account.AccountID, map[string]any{"name": expectedName})
					if writeErr != nil {
						item["status"], item["error"] = "修复失败", writeErr.Error()
						results[index] = repairResult{item: item, kind: "failed"}
						return
					}
					item["remote_write"] = true
					readback, readErr := client.Account(accountCtx, account.AccountID)
					if readErr != nil {
						item["status"], item["error"] = "修复失败", "管理平台名称写入成功，但账号读回失败："+readErr.Error()
						results[index] = repairResult{item: item, kind: "failed", remoteWritten: true}
						return
					}
					confirmedID := strings.TrimSpace(fmt.Sprint(firstValue(readback, "id", "account_id")))
					if confirmedID != account.AccountID {
						item["status"] = "修复失败"
						item["error"] = fmt.Sprintf("管理平台名称写入成功，但账号读回的稳定 ID 为 %q", confirmedID)
						results[index] = repairResult{item: item, kind: "failed", remoteWritten: true}
						return
					}
					confirmed := strings.TrimSpace(fmt.Sprint(firstValue(readback, "name")))
					if confirmed != expectedName {
						item["status"], item["error"] = "修复失败", "管理平台名称写入成功，但账号名称读回不一致"
						results[index] = repairResult{item: item, kind: "failed", remoteWritten: true}
						return
					}
					commitMu.Lock()
					commitErr := s.repository.CommitAccountNameRepairs(accountCtx, []business.AccountNameRepairCommit{{
						AccountID: account.AccountID, Name: confirmed,
					}})
					commitMu.Unlock()
					if commitErr != nil {
						item["status"], item["error"] = "修复失败", "名称修复结果保存失败："+commitErr.Error()
						results[index] = repairResult{item: item, kind: "failed", remoteWritten: true}
						return
					}
					item["status"], item["readback_confirmed"] = "已修复", true
					results[index] = repairResult{item: item, kind: "repaired", remoteWritten: true}
				}()
			}
		}()
	}
	for index := range bound {
		jobs <- index
	}
	close(jobs)
	workers.Wait()
	items := make([]map[string]any, 0, len(results))
	renamed, unchanged, missing, skipped, failed, written := 0, 0, 0, 0, 0, 0
	for _, result := range results {
		items = append(items, result.item)
		if result.remoteWritten {
			written++
		}
		switch result.kind {
		case "repaired":
			renamed++
		case "unchanged":
			unchanged++
		case "missing":
			missing++
		case "skipped":
			skipped++
		default:
			failed++
		}
	}
	result := map[string]any{"operation": "account.name.repair", "requested": len(accountIDs), "bound": len(bound),
		"renamed": renamed, "unchanged": unchanged, "missing": missing, "skipped": skipped, "failed": failed, "items": items, "actor": actor, "remote_write": written > 0}
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
	bound, err := s.repository.BoundAccountsForMaintenance(ctx, accountIDs)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]business.BoundAccountMaintenance, len(bound))
	for _, account := range bound {
		if _, exists := byID[account.AccountID]; !exists {
			byID[account.AccountID] = account
		}
	}
	guarded, releaseAll, err := s.acquireAccountMutations(ctx, accountIDs, false)
	if err != nil {
		return nil, err
	}
	defer releaseAll()
	ctx = guarded
	client, err := s.maintenanceClient(ctx)
	if err != nil {
		return nil, err
	}
	type repairResult struct {
		item    map[string]any
		written bool
		kind    string
	}
	results := make([]repairResult, len(accountIDs))
	var commitMu sync.Mutex
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
				if !boundExists {
					item["status"] = "没有绑定，已跳过"
					results[index] = repairResult{item: item, kind: "skipped"}
					continue
				}
				accountCtx, release, protectedReason, guardErr := s.acquireAccountMutation(ctx, accountID)
				if guardErr != nil {
					item["status"], item["error"] = "账号变更锁获取失败", guardErr.Error()
					results[index] = repairResult{item: item, kind: "failed"}
					continue
				}
				if protectedReason != nil {
					item["status"], item["reason"] = "人工保护，已跳过", *protectedReason
					results[index] = repairResult{item: item, kind: "skipped"}
					continue
				}
				func() {
					defer release()
					currentBindings, baselineErr := s.repository.BoundAccountsForMaintenance(accountCtx, []string{accountID})
					if baselineErr != nil {
						item["status"], item["error"] = "账号绑定复读失败", baselineErr.Error()
						results[index] = repairResult{item: item, kind: "failed"}
						return
					}
					var currentAccount *business.BoundAccountMaintenance
					for bindingIndex := range currentBindings {
						if currentBindings[bindingIndex].AccountID == accountID {
							currentAccount = &currentBindings[bindingIndex]
							break
						}
					}
					if currentAccount == nil {
						item["status"] = "没有绑定，已跳过"
						results[index] = repairResult{item: item, kind: "skipped"}
						return
					}
					item["account_name"], item["upstream_host"] = currentAccount.AccountName, currentAccount.UpstreamHost
					if !currentAccount.ConsoleOnboarded {
						item["status"] = "非本控制台添加，未修改"
						results[index] = repairResult{item: item, kind: "skipped"}
						return
					}
					row, readErr := client.Account(accountCtx, accountID)
					if readErr != nil {
						item["status"], item["error"] = "参数读取失败", readErr.Error()
						results[index] = repairResult{item: item, kind: "failed"}
						return
					}
					returnedID := strings.TrimSpace(fmt.Sprint(firstValue(row, "id", "account_id")))
					if returnedID != accountID {
						item["status"] = "参数读取失败"
						item["error"] = fmt.Sprintf("账号详情返回的稳定 ID 为 %q，与请求账号 %s 不一致", returnedID, accountID)
						results[index] = repairResult{item: item, kind: "failed"}
						return
					}
					item["account_name"] = strings.TrimSpace(fmt.Sprint(firstValue(row, "name")))
					concurrency, concurrencyErr := managementInteger(row, "concurrency")
					priority, priorityErr := managementInteger(row, "priority")
					loadFactor, loadFactorErr := managementOptionalInteger(row, "load_factor")
					if concurrencyErr != nil || priorityErr != nil || loadFactorErr != nil {
						item["status"] = "参数读取失败"
						item["error"] = errors.Join(concurrencyErr, priorityErr, loadFactorErr).Error()
						results[index] = repairResult{item: item, kind: "failed"}
						return
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
						commitMu.Lock()
						commitErr := s.repository.CommitAccountDefaultsRepairs(accountCtx, []business.AccountDefaultsRepairCommit{commit}, actor)
						commitMu.Unlock()
						if commitErr != nil {
							item["status"], item["error"] = "默认参数结果保存失败", commitErr.Error()
							results[index] = repairResult{item: item, kind: "failed"}
							return
						}
						item["status"] = "无需修复"
						results[index] = repairResult{item: item, kind: "unchanged"}
						return
					}
					_, updateErr := client.UpdateAccount(accountCtx, accountID, body)
					if updateErr != nil {
						item["status"], item["error"] = "修复失败", updateErr.Error()
						results[index] = repairResult{item: item, kind: "failed"}
						return
					}
					item["remote_write"] = true
					readback, readbackErr := client.Account(accountCtx, accountID)
					if readbackErr != nil {
						item["status"], item["error"] = "修复失败", "管理平台参数写入成功，但账号读回失败："+readbackErr.Error()
						results[index] = repairResult{item: item, written: true, kind: "failed"}
						return
					}
					confirmedID := strings.TrimSpace(fmt.Sprint(firstValue(readback, "id", "account_id")))
					if confirmedID != accountID {
						item["status"] = "修复失败"
						item["error"] = fmt.Sprintf("管理平台参数写入成功，但账号读回的稳定 ID 为 %q", confirmedID)
						results[index] = repairResult{item: item, written: true, kind: "failed"}
						return
					}
					confirmedConcurrency, confirmedConcurrencyErr := managementInteger(readback, "concurrency")
					confirmedPriority, confirmedPriorityErr := managementInteger(readback, "priority")
					confirmedLoadFactor, confirmedLoadFactorErr := managementOptionalInteger(readback, "load_factor")
					confirmed := confirmedConcurrencyErr == nil && confirmedPriorityErr == nil && confirmedLoadFactorErr == nil &&
						confirmedConcurrency == afterConcurrency && confirmedPriority == afterPriority &&
						(body["load_factor"] == nil || confirmedLoadFactor == nil)
					if !confirmed {
						item["status"], item["error"] = "修复失败", "管理平台账号参数读回与预期不一致"
						results[index] = repairResult{item: item, written: true, kind: "failed"}
						return
					}
					commit.RemoteRepaired = true
					commitMu.Lock()
					commitErr := s.repository.CommitAccountDefaultsRepairs(accountCtx, []business.AccountDefaultsRepairCommit{commit}, actor)
					commitMu.Unlock()
					if commitErr != nil {
						item["status"], item["error"] = "默认参数修复结果保存失败", commitErr.Error()
						results[index] = repairResult{item: item, written: true, kind: "failed"}
						return
					}
					item["status"], item["readback_confirmed"] = "已修复", true
					results[index] = repairResult{item: item, written: true, kind: "repaired"}
				}()
			}
		}()
	}
	for index := range accountIDs {
		jobs <- index
	}
	close(jobs)
	workers.Wait()

	items := make([]map[string]any, 0, len(results))
	repaired, unchanged, skipped, failed, written := 0, 0, 0, 0, 0
	for _, result := range results {
		items = append(items, result.item)
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
	result := map[string]any{"operation": "account.defaults.repair", "requested": len(accountIDs), "bound": len(byID),
		"repaired": repaired, "unchanged": unchanged, "skipped": skipped, "failed": failed, "items": items,
		"actor": actor, "remote_write": written > 0, "default_concurrency": defaults.Concurrency, "default_priority": defaults.Priority}
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
	bound, err := s.repository.BoundAccountsForMaintenance(ctx, accountIDs)
	if err != nil {
		return nil, err
	}
	bound = uniqueBoundAccounts(bound)
	result := map[string]any{"operation": "account.missing-binding.cleanup", "requested": len(accountIDs),
		"bound": len(bound), "cleaned": 0, "skipped": len(bound), "items": []map[string]any{}, "actor": actor, "remote_write": false}
	if len(bound) == 0 {
		return result, nil
	}
	boundIDs := make([]string, 0, len(bound))
	lockedIDs := make(map[string]struct{}, len(bound))
	for _, account := range bound {
		boundIDs = append(boundIDs, account.AccountID)
		lockedIDs[account.AccountID] = struct{}{}
	}
	guardedCtx, release, err := s.acquireAccountMutations(ctx, boundIDs, true)
	if err != nil {
		return result, err
	}
	defer release()
	bound, err = s.repository.BoundAccountsForMaintenance(guardedCtx, accountIDs)
	if err != nil {
		return result, fmt.Errorf("获取账号租约后绑定状态复读失败：%w", err)
	}
	bound = uniqueBoundAccounts(bound)
	for _, account := range bound {
		if _, locked := lockedIDs[account.AccountID]; !locked {
			return result, errors.New("获取账号租约后绑定账号集合已变化，请重试清理")
		}
	}
	client, err := s.maintenanceClient(guardedCtx)
	if err != nil {
		return result, err
	}
	remoteRows, err := client.Accounts(guardedCtx)
	if err != nil {
		return result, fmt.Errorf("获取账号租约后管理平台账号目录复读失败：%w", err)
	}
	remote := make(map[string]map[string]any, len(remoteRows))
	for _, row := range remoteRows {
		accountID := strings.TrimSpace(fmt.Sprint(firstValue(row, "id", "account_id")))
		if accountID == "" {
			return result, errors.New("管理平台账号目录包含缺少稳定 ID 的记录")
		}
		if _, duplicate := remote[accountID]; duplicate {
			return result, fmt.Errorf("管理平台返回重复账号 ID：%s", accountID)
		}
		remote[accountID] = row
	}
	verification := make([]business.BindingVerification, 0, len(bound))
	missingIDs := make([]string, 0, len(bound))
	items := make([]map[string]any, 0, len(bound))
	for _, account := range bound {
		_, exists := remote[account.AccountID]
		status := "账号仍然存在，未清理"
		if protectionRepository, ok := s.repository.(mutationProtectionRepository); ok {
			protection, protectionErr := protectionRepository.AccountMutationProtection(guardedCtx, account.AccountID)
			if protectionErr != nil {
				return result, fmt.Errorf("账号 %s 人工保护状态复核失败：%w", account.AccountID, protectionErr)
			}
			if protection.Protected() {
				status = "账号已启用" + strings.Join(protection.Reasons(), "、") + "，未清理"
				items = append(items, map[string]any{"account_id": account.AccountID, "account_name": account.AccountName,
					"upstream_host": account.UpstreamHost, "status": status})
				continue
			}
		}
		verification = append(verification, business.BindingVerification{AccountID: account.AccountID, Exists: exists})
		if !exists {
			status = "待清理"
			missingIDs = append(missingIDs, account.AccountID)
		}
		items = append(items, map[string]any{"account_id": account.AccountID, "account_name": account.AccountName,
			"upstream_host": account.UpstreamHost, "status": status})
	}
	result["bound"], result["skipped"], result["items"] = len(bound), len(bound)-len(missingIDs), items
	if len(verification) > 0 {
		if err := s.repository.CommitBindingVerification(guardedCtx, verification); err != nil {
			return result, fmt.Errorf("最新复验结果保存失败：%w", err)
		}
	}
	if len(missingIDs) == 0 {
		return result, nil
	}
	cleanup, err := s.repository.CleanupMissingBindings(guardedCtx, missingIDs, actor)
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

func uniqueBoundAccounts(values []business.BoundAccountMaintenance) []business.BoundAccountMaintenance {
	seen := make(map[string]struct{}, len(values))
	result := make([]business.BoundAccountMaintenance, 0, len(values))
	for _, value := range values {
		accountID := strings.TrimSpace(value.AccountID)
		if accountID == "" {
			continue
		}
		if _, duplicate := seen[accountID]; duplicate {
			continue
		}
		seen[accountID] = struct{}{}
		result = append(result, value)
	}
	return result
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

func (s *Service) execute(parent context.Context, task taskstore.Task, actor string) {
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()
	task.Status, task.Progress, task.Message, task.UpdatedAt = "running", 20, "读取 Admin API 账号与分组目录", time.Now().UTC().Format(time.RFC3339Nano)
	if !taskstore.SaveRunning(ctx, s.tasks, task) {
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
			"deleted_groups": result.DeletedGroups,
			"event_id":       result.EventID, "remote_write": result.RemoteWrite, "read_only": result.ReadOnly,
		}
	}
	taskstore.MarkCancelled(ctx, &task, "管理快照同步已取消")
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) Sync(ctx context.Context, actor string) (business.ManagementSyncResult, error) {
	ctx, err := targetguard.Capture(ctx, s.targets)
	if err != nil {
		return business.ManagementSyncResult{}, err
	}
	var groupIdentities []business.ManagementSnapshotGroupIdentity
	var accountIDs []string
	discoveryErr := func() (resultErr error) {
		discoveryCtx, release, acquireErr := targetguard.Acquire(ctx, s.repository, mutationguard.AccountCatalog())
		if acquireErr != nil {
			return acquireErr
		}
		defer func() {
			if releaseErr := release(); releaseErr != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("管理快照发现租约释放失败：%w", releaseErr))
			}
		}()
		discoveryCtx, resultErr = targetguard.Bind(discoveryCtx, s.targets)
		if resultErr != nil {
			return resultErr
		}
		client, clientErr := s.maintenanceClient(discoveryCtx)
		if clientErr != nil {
			return clientErr
		}
		groups, groupsErr := client.Groups(discoveryCtx)
		if groupsErr != nil {
			return fmt.Errorf("分组目录读取失败：%w", groupsErr)
		}
		groupIdentities, resultErr = business.ManagementSnapshotGroupIdentities(groups)
		if resultErr != nil {
			return resultErr
		}
		accounts, accountsErr := client.Accounts(discoveryCtx)
		if accountsErr != nil {
			return fmt.Errorf("账号目录读取失败：%w", accountsErr)
		}
		remoteAccountIDs, idsErr := business.ManagementSnapshotAccountIDs(accounts)
		if idsErr != nil {
			return idsErr
		}
		localAccountIDs, localErr := s.repository.ManagementAccountIDs(discoveryCtx)
		if localErr != nil {
			return fmt.Errorf("本地账号 ID 读取失败：%w", localErr)
		}
		accountIDs = unionAccountIDs(remoteAccountIDs, localAccountIDs)
		return nil
	}()
	if discoveryErr != nil {
		return business.ManagementSyncResult{}, discoveryErr
	}
	resources := accountMutationResources(accountIDs, true)
	accountCtx, release, err := targetguard.Acquire(ctx, s.repository, resources...)
	if err != nil {
		return business.ManagementSyncResult{}, err
	}
	defer func() {
		if err := release(); err != nil {
			slog.Error("管理快照变更租约释放失败", "account_ids", accountIDs, "error", err)
		}
	}()
	accountCtx, err = targetguard.Bind(accountCtx, s.targets)
	if err != nil {
		return business.ManagementSyncResult{}, err
	}
	client, err := s.maintenanceClient(accountCtx)
	if err != nil {
		return business.ManagementSyncResult{}, err
	}
	groups, err := client.Groups(accountCtx)
	if err != nil {
		return business.ManagementSyncResult{}, fmt.Errorf("锁内分组目录复读失败：%w", err)
	}
	confirmedGroupIdentities, err := business.ManagementSnapshotGroupIdentities(groups)
	if err != nil {
		return business.ManagementSyncResult{}, err
	}
	if !managementGroupIdentitiesEqual(groupIdentities, confirmedGroupIdentities) {
		return business.ManagementSyncResult{}, errors.New("获取账号租约期间管理分组目录发生变化，请重试快照同步")
	}
	accounts, err := client.Accounts(accountCtx)
	if err != nil {
		return business.ManagementSyncResult{}, fmt.Errorf("锁内账号目录复读失败：%w", err)
	}
	confirmedIDs, err := business.ManagementSnapshotAccountIDs(accounts)
	if err != nil {
		return business.ManagementSyncResult{}, err
	}
	confirmedLocalIDs, err := s.repository.ManagementAccountIDs(accountCtx)
	if err != nil {
		return business.ManagementSyncResult{}, fmt.Errorf("锁内本地账号 ID 复读失败：%w", err)
	}
	if !accountIDsCover(accountIDs, unionAccountIDs(confirmedIDs, confirmedLocalIDs)) {
		return business.ManagementSyncResult{}, errors.New("获取账号租约后管理目录出现新的账号，请重试快照同步")
	}
	accounts = authoritativeManagementAccountGroups(accounts)
	return s.repository.SyncManagementSnapshot(accountCtx, accounts, groups, actor)
}

func authoritativeManagementAccountGroups(accounts []map[string]any) []map[string]any {
	result := make([]map[string]any, len(accounts))
	for index, account := range accounts {
		copy := make(map[string]any, len(account)+1)
		for key, value := range account {
			copy[key] = value
		}
		if _, present := firstPresentValue(copy, "groups", "account_groups", "group_ids"); !present {
			copy["group_ids"] = []any{}
		}
		result[index] = copy
	}
	return result
}

func firstPresentValue(row map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if value, found := row[key]; found {
			return value, true
		}
	}
	return nil, false
}

func unionAccountIDs(groups ...[]string) []string {
	seen := map[string]struct{}{}
	for _, values := range groups {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value != "" {
				seen[value] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func accountIDsCover(locked, current []string) bool {
	lockedSet := make(map[string]struct{}, len(locked))
	for _, accountID := range locked {
		lockedSet[accountID] = struct{}{}
	}
	for _, accountID := range current {
		if _, found := lockedSet[accountID]; !found {
			return false
		}
	}
	return true
}

func managementGroupIdentitiesEqual(
	left []business.ManagementSnapshotGroupIdentity,
	right []business.ManagementSnapshotGroupIdentity,
) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func managementTaskID() (string, error) {
	value := make([]byte, 6)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
