package management

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
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
	CommitAccountRateObservations(context.Context, []business.AccountRateObservation) error
	CommitBindingVerification(context.Context, []business.BindingVerification) error
	CommitAccountNameRepairs(context.Context, []business.AccountNameRepairCommit) error
	CleanupMissingBindings(context.Context, []string, string) (business.MissingBindingCleanupResult, error)
}

type TaskStore interface {
	Save(context.Context, taskstore.Task) error
}

type AccountRateWriter interface {
	SyncAccountRate(context.Context, string, string, string, string) (map[string]any, error)
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

func (s *Service) EnqueueAccountRateSync(ctx context.Context, accountIDs []string, actor string) (taskstore.Task, error) {
	return s.enqueueMaintenance(ctx, "account-rate-sync", "账号倍率同步已排队", accountIDs, actor)
}

func (s *Service) SyncAllAccountRates(ctx context.Context, actor string) (map[string]any, error) {
	return s.syncAccountRates(ctx, nil, actor)
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
	runningMessage := "读取管理平台账号目录"
	if operation == "account-rate-sync" {
		runningMessage = "正在从上游探测账号有效倍率并写回管理平台"
	}
	task.Status, task.Progress, task.Message, task.UpdatedAt = "running", 10, runningMessage, time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.tasks.Save(ctx, task); err != nil {
		return
	}
	var result map[string]any
	var err error
	if operation == "account-rate-sync" {
		result, err = s.syncAccountRates(ctx, accountIDs, actor)
	} else if operation == "account-name-repair" {
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
		if operation == "account-rate-sync" {
			task.Message = fmt.Sprintf("账号倍率同步完成：更新 %v 个，未变 %v 个，缺失 %v 个，失败 %v 个", result["updated"], result["unchanged"], result["missing"], result["failed"])
			if failed, _ := result["failed"].(int); failed > 0 {
				task.Status = "failed"
				task.Message = fmt.Sprintf("账号倍率同步部分失败：更新 %v 个，未变 %v 个，缺失 %v 个，失败 %v 个", result["updated"], result["unchanged"], result["missing"], result["failed"])
			}
		} else if operation == "account-name-repair" {
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
			"updated": 0, "unchanged": 0, "missing": 0, "failed": 0,
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
		account    business.BoundAccountMaintenance
		multiplier string
		err        error
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
			if upstreamRates[index].err != nil || isNewAPIType(upstreamRates[index].account.UpstreamType) {
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
		if probe.err == nil {
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
				item["name_before"], item["name_after"] = remoteName, expectedName
				item["upstream_multiplier"] = probe.multiplier
				if remoteMultiplier == probe.multiplier && remoteName == expectedName &&
					sameRate(account.CurrentMultiplier, probe.multiplier) && account.AccountName == expectedName {
					item["status"] = "已确认一致"
					results[index] = rateResult{item: item, unchanged: true}
					continue
				}
				writeResult, writeErr := s.rateWriter.SyncAccountRate(ctx, accountID, expectedName, probe.multiplier, actor)
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
	updated, unchanged, missing, failed, written := 0, 0, 0, 0, 0
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
		if result.failed {
			failed++
		}
		if result.written {
			written++
		}
	}
	return map[string]any{
		"operation": "account.rate.sync", "source": "upstream_live", "requested": len(accountIDs),
		"updated": updated, "unchanged": unchanged, "missing": missing, "failed": failed,
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
	raw, ok := new(big.Rat).SetString(strings.TrimSpace(*rawRate))
	if !ok || raw.Sign() <= 0 {
		return "", errors.New("NewAPI 上游返回非法倍率")
	}
	recharge, ok := new(big.Rat).SetString(strings.TrimSpace(account.RechargeRate))
	if !ok || recharge.Sign() <= 0 {
		return "", errors.New("NewAPI 上游充值倍率无效")
	}
	raw.Quo(raw, recharge)
	text := raw.FloatString(28)
	text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	if text == "" || text == "0" {
		return "", errors.New("NewAPI 上游折算倍率无效")
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
