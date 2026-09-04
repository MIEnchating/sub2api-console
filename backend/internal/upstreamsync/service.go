package upstreamsync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/redact"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type Repository interface {
	Upstreams(context.Context) (business.UpstreamSummary, error)
	UpstreamMutationAccountIDs(context.Context, string) ([]string, error)
	ApplyUpstreamSync(context.Context, business.UpstreamSyncWrite) (business.UpstreamSyncWriteResult, error)
	RecordUpstreamSyncFailure(context.Context, string, string, string, bool) error
	RecordRuntimeEvent(context.Context, string, string, string, map[string]any) (int64, error)
}

type balanceSyncPolicy interface {
	HostBalanceSyncAllowed(context.Context, string) (bool, error)
}

type PrivateStore interface {
	AuthRecord(context.Context, string) (*configstore.AuthRecord, error)
	SaveAuthRecord(context.Context, configstore.AuthRecord, map[string]bool) error
}

type CatalogReader interface {
	ReadCatalog(context.Context, configstore.AuthRecord) (business.UpstreamCatalogSnapshot, error)
	ReadBalance(context.Context, configstore.AuthRecord) (business.UpstreamBalanceObservation, error)
}

type Refresher interface {
	Refresh(context.Context, configstore.AuthRecord) (configstore.AuthRecord, error)
}

type AuthResolver interface {
	ResolveAuth(context.Context, string, string) (*configstore.AuthRecord, error)
}

type TaskStore interface {
	Save(context.Context, taskstore.Task) error
}

type AccountRateSyncScheduler interface {
	EnqueueHostAccountRateSync(context.Context, string, string) (string, error)
	EnqueueAllAccountRateSync(context.Context, string) (string, error)
}

type Scope struct {
	Catalog bool
	Balance bool
	Name    bool
	KeyID   *string
}

type readScopeError struct {
	catalog error
	balance error
}

func (e *readScopeError) Error() string {
	switch {
	case e.catalog != nil && e.balance != nil:
		return "分组目录读取失败：" + e.catalog.Error() + "；余额读取失败：" + e.balance.Error()
	case e.catalog != nil:
		return e.catalog.Error()
	case e.balance != nil:
		return e.balance.Error()
	default:
		return "上游读取失败"
	}
}

func (e *readScopeError) Unwrap() []error {
	causes := make([]error, 0, 2)
	if e.catalog != nil {
		causes = append(causes, e.catalog)
	}
	if e.balance != nil {
		causes = append(causes, e.balance)
	}
	return causes
}

type siteNameReader interface {
	ReadSiteName(context.Context, configstore.AuthRecord) (*string, error)
}

type HostResult struct {
	Host                 string                            `json:"host"`
	Status               string                            `json:"status"`
	AuthStatus           string                            `json:"auth_status"`
	BalanceStatus        string                            `json:"balance_status"`
	Balance              *string                           `json:"balance,omitempty"`
	DisplayBalance       *string                           `json:"display_balance,omitempty"`
	BalanceUnit          *string                           `json:"balance_unit,omitempty"`
	GroupCount           int                               `json:"group_count"`
	KeyCount             int                               `json:"key_count"`
	AccountTotal         int                               `json:"account_total"`
	AccountRateSucceeded int                               `json:"account_rate_succeeded"`
	AccountRateFailed    int                               `json:"account_rate_failed"`
	AuthRecovered        bool                              `json:"auth_recovered"`
	Reason               *string                           `json:"reason,omitempty"`
	Catalog              *business.UpstreamCatalogSnapshot `json:"-"`
}

type BatchResult struct {
	Total                int          `json:"total"`
	Succeeded            int          `json:"succeeded"`
	AuthFailed           int          `json:"auth_failed"`
	Failed               int          `json:"failed"`
	AccountTotal         int          `json:"account_total"`
	AccountRateSucceeded int          `json:"account_rate_succeeded"`
	AccountRateFailed    int          `json:"account_rate_failed"`
	Hosts                []HostResult `json:"hosts"`
}

type Service struct {
	repository Repository
	private    PrivateStore
	reader     CatalogReader
	refresher  Refresher
	resolver   AuthResolver
	tasks      TaskStore
	taskRunner taskrunner.Runner
	rateSync   AccountRateSyncScheduler
	timeout    time.Duration
	workers    int
}

func New(repository Repository, private PrivateStore, reader CatalogReader, refresher Refresher, tasks TaskStore, schedulers ...AccountRateSyncScheduler) *Service {
	service := &Service{repository: repository, private: private, reader: reader, refresher: refresher, tasks: tasks, timeout: 30 * time.Minute, workers: 4}
	if len(schedulers) > 0 {
		service.rateSync = schedulers[0]
	}
	return service
}

func (s *Service) SetAuthResolver(resolver AuthResolver) {
	s.resolver = resolver
}

func (s *Service) UseTaskRunner(runner taskrunner.Runner) { s.taskRunner = runner }

func (s *Service) EnqueueAll(ctx context.Context, scope Scope, actor, operation string) (taskstore.Task, error) {
	var err error
	scope, err = normalizeScope(scope)
	if err != nil {
		return taskstore.Task{}, err
	}
	if scope.KeyID != nil {
		return taskstore.Task{}, errors.New("批量 Host 同步不能指定 Key ID")
	}
	summary, err := s.repository.Upstreams(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	hosts := make([]string, len(summary.Hosts))
	for index := range summary.Hosts {
		hosts[index] = summary.Hosts[index].Host
	}
	return s.enqueue(ctx, hosts, scope, actor, operation, summary)
}

func (s *Service) EnqueueHost(ctx context.Context, host string, scope Scope, actor, operation string) (taskstore.Task, error) {
	var err error
	scope, err = normalizeScope(scope)
	if err != nil {
		return taskstore.Task{}, err
	}
	host = configstore.CanonicalHost(host)
	if host == "" {
		return taskstore.Task{}, errors.New("上游 Host 不能为空")
	}
	record, _, err := s.authRecord(ctx, host, actor)
	if err != nil {
		return taskstore.Task{}, fmt.Errorf("Host %q 的私有授权恢复失败：%w", host, err)
	}
	if record == nil {
		return taskstore.Task{}, fmt.Errorf("未找到 Host %q 的私有授权记录，请检查账号归属 Host 是否完全一致（含端口）；Base URL 可以不同", host)
	}
	summary, err := s.repository.Upstreams(ctx)
	if err != nil {
		return taskstore.Task{}, err
	}
	return s.enqueue(ctx, []string{host}, scope, actor, operation, summary)
}

func (s *Service) SyncHost(ctx context.Context, host string, scope Scope, actor string) (HostResult, error) {
	var err error
	scope, err = normalizeScope(scope)
	if err != nil {
		return HostResult{}, err
	}
	host = configstore.CanonicalHost(host)
	if host == "" {
		return HostResult{}, errors.New("上游 Host 不能为空")
	}
	return s.syncHost(ctx, host, scope, actor), nil
}

func (s *Service) SyncAllNow(ctx context.Context, scope Scope, actor string) (BatchResult, error) {
	var err error
	scope, err = normalizeScope(scope)
	if err != nil {
		return BatchResult{}, err
	}
	if scope.KeyID != nil {
		return BatchResult{}, errors.New("批量 Host 同步不能指定 Key ID")
	}
	summary, err := s.repository.Upstreams(ctx)
	if err != nil {
		return BatchResult{}, err
	}
	hosts := make([]string, len(summary.Hosts))
	for index := range summary.Hosts {
		hosts[index] = summary.Hosts[index].Host
	}
	result := s.syncHosts(ctx, hosts, scope, actor, nil)
	applyBatchAccountCounts(&result, summary, scope)
	return result, nil
}

func (s *Service) enqueue(
	ctx context.Context,
	hosts []string,
	scope Scope,
	actor string,
	operation string,
	summary business.UpstreamSummary,
) (taskstore.Task, error) {
	id, err := taskID()
	if err != nil {
		return taskstore.Task{}, err
	}
	if strings.TrimSpace(operation) == "" {
		operation = "upstream-sync"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{
		ID: id, Skill: "sub2api-upstream-info", Operation: operation, Status: "queued", Progress: 0,
		Message: "上游同步已排队", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	if err := taskrunner.Go(s.taskRunner, func(parent context.Context) {
		s.execute(parent, task, hosts, scope, actor, summary)
	}); err != nil {
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
	return task, nil
}

func (s *Service) execute(parent context.Context, task taskstore.Task, hosts []string, scope Scope, actor string, summary business.UpstreamSummary) {
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()
	task.Status, task.Progress, task.Message, task.UpdatedAt = "running", 5, scopeMessage(scope), time.Now().UTC().Format(time.RFC3339Nano)
	if !taskstore.SaveRunning(ctx, s.tasks, task) {
		return
	}
	result := s.syncHosts(ctx, hosts, scope, actor, func(completed, total int) {
		progress := 95
		if total > 0 {
			progress = 5 + completed*90/total
		}
		task.Progress, task.Message, task.UpdatedAt = progress, fmt.Sprintf("上游同步进度：%d/%d", completed, total), time.Now().UTC().Format(time.RFC3339Nano)
		taskstore.PersistProgress(s.tasks, task)
	})
	applyBatchAccountCounts(&result, summary, scope)
	accountRateTaskID, accountRateErr := s.enqueueAccountRateSync(ctx, hosts, scope, actor, result)
	task.Progress, task.UpdatedAt = 100, time.Now().UTC().Format(time.RFC3339Nano)
	task.Result = map[string]any{
		"total": result.Total, "succeeded": result.Succeeded, "auth_failed": result.AuthFailed,
		"failed": result.Failed, "account_total": result.AccountTotal,
		"account_rate_succeeded": result.AccountRateSucceeded, "account_rate_failed": result.AccountRateFailed,
		"hosts": result.Hosts, "remote_write": false, "credentials_exposed": false,
	}
	if accountRateTaskID != "" {
		task.Result["account_rate_sync_task_id"] = accountRateTaskID
	}
	if accountRateErr != nil {
		task.Result["account_rate_sync_error"] = accountRateErr.Error()
	}
	if result.AuthFailed > 0 || result.Failed > 0 || accountRateErr != nil {
		task.Status = "failed"
		task.Message = fmt.Sprintf("上游同步完成：成功 %d，鉴权失败 %d，其他失败 %d", result.Succeeded, result.AuthFailed, result.Failed)
		if accountRateErr != nil {
			task.Message += "；账号成本同步排队失败：" + accountRateErr.Error()
		}
	} else {
		task.Status = "succeeded"
		task.Message = fmt.Sprintf("上游同步完成：成功 %d", result.Succeeded)
		if accountRateTaskID != "" {
			task.Message += "；相关账号成本与名称同步已排队"
		}
	}
	taskstore.MarkCancelled(ctx, &task, "上游同步已取消")
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) enqueueAccountRateSync(ctx context.Context, hosts []string, scope Scope, actor string, result BatchResult) (string, error) {
	if s.rateSync == nil || !scope.Catalog || result.Succeeded == 0 {
		return "", nil
	}
	hosts = canonicalHosts(hosts)
	if len(hosts) == 1 {
		return s.rateSync.EnqueueHostAccountRateSync(ctx, hosts[0], actor)
	}
	return s.rateSync.EnqueueAllAccountRateSync(ctx, actor)
}

func (s *Service) syncHosts(ctx context.Context, hosts []string, scope Scope, actor string, progress func(int, int)) BatchResult {
	hosts = canonicalHosts(hosts)
	result := BatchResult{Total: len(hosts), Hosts: make([]HostResult, len(hosts))}
	if len(hosts) == 0 {
		return result
	}
	type job struct{ index int }
	type outcome struct {
		index int
		value HostResult
	}
	jobs := make(chan job)
	outcomes := make(chan outcome, len(hosts))
	workers := s.workers
	if workers < 1 {
		workers = 1
	}
	if workers > 4 {
		workers = 4
	}
	if workers > len(hosts) {
		workers = len(hosts)
	}
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for item := range jobs {
				outcomes <- outcome{index: item.index, value: s.syncHost(ctx, hosts[item.index], scope, actor)}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for index := range hosts {
			select {
			case jobs <- job{index: index}:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wait.Wait(); close(outcomes) }()
	completed := 0
	for item := range outcomes {
		result.Hosts[item.index] = item.value
		completed++
		if progress != nil {
			progress(completed, len(hosts))
		}
	}
	for index := range result.Hosts {
		if result.Hosts[index].Host == "" {
			reason := "上游同步超时或已取消"
			result.Hosts[index] = failedHost(hosts[index], "failed", business.UpstreamAuthStatusUnconfirmed, reason)
		}
		switch result.Hosts[index].Status {
		case "succeeded":
			result.Succeeded++
		case "auth_failed":
			result.AuthFailed++
		default:
			result.Failed++
		}
	}
	return result
}

func applyBatchAccountCounts(result *BatchResult, summary business.UpstreamSummary, scope Scope) {
	if !scope.Catalog {
		return
	}
	knownAccounts := make(map[string]int, len(summary.Hosts))
	for _, host := range summary.Hosts {
		knownAccounts[configstore.CanonicalHost(host.Host)] = int(host.AccountCount)
	}
	result.AccountTotal, result.AccountRateSucceeded, result.AccountRateFailed = 0, 0, 0
	for index := range result.Hosts {
		host := &result.Hosts[index]
		if host.Status != "succeeded" && host.AccountTotal == 0 {
			host.AccountTotal = knownAccounts[configstore.CanonicalHost(host.Host)]
			host.AccountRateFailed = host.AccountTotal
		}
		result.AccountTotal += host.AccountTotal
		result.AccountRateSucceeded += host.AccountRateSucceeded
		result.AccountRateFailed += host.AccountRateFailed
	}
}

func (s *Service) syncHost(ctx context.Context, host string, scope Scope, actor string) HostResult {
	if scope.Balance {
		if policy, ok := s.repository.(balanceSyncPolicy); ok {
			allowed, policyErr := policy.HostBalanceSyncAllowed(ctx, host)
			if policyErr != nil {
				return s.failed(ctx, host, Scope{Balance: true}, false, "人工优先位余额同步策略读取失败："+policyErr.Error())
			}
			if !allowed {
				if !scope.Catalog {
					reason := "该 Host 下的人工优先位账号均关闭了上游余额同步"
					return HostResult{Host: host, Status: "succeeded", AuthStatus: "未变更", BalanceStatus: reason, Reason: &reason}
				}
				scope.Balance = false
			}
		}
	}
	record, recovered, err := s.authRecord(ctx, host, actor)
	if err != nil {
		return s.failed(ctx, host, scope, true, "私有授权恢复失败："+err.Error())
	}
	if record == nil {
		return s.failed(ctx, host, scope, true, "未配置私有授权记录")
	}
	accountIDs, err := s.repository.UpstreamMutationAccountIDs(ctx, host)
	if err != nil {
		return s.failed(ctx, host, scope, false, "上游关联账号读取失败："+err.Error())
	}
	resources := make([]string, 0, len(accountIDs)+1)
	resources = append(resources, mutationguard.Upstream(host))
	for _, accountID := range accountIDs {
		resources = append(resources, mutationguard.Account(accountID))
	}
	guardedCtx, release, err := mutationguard.Acquire(ctx, s.repository, resources...)
	if err != nil {
		return failedHost(host, "failed", business.UpstreamAuthStatusUnconfirmed, "上游同步租约获取失败："+err.Error())
	}
	defer func() {
		if err := release(); err != nil {
			slog.Error("上游同步租约释放失败", "host", host, "error", err)
		}
	}()
	ctx = guardedCtx
	record, err = s.private.AuthRecord(ctx, host)
	if err != nil {
		return s.failed(ctx, host, scope, true, "获取同步租约后私有授权复读失败："+err.Error())
	}
	if record == nil {
		return s.failed(ctx, host, scope, true, "获取同步租约后私有授权已被删除")
	}
	confirmedAccountIDs, confirmErr := s.repository.UpstreamMutationAccountIDs(ctx, host)
	if confirmErr != nil {
		return s.failed(ctx, host, scope, false, "获取同步租约后关联账号复读失败："+confirmErr.Error())
	}
	if !stringIDsCover(accountIDs, confirmedAccountIDs) {
		return s.failed(ctx, host, scope, false, "获取同步租约后关联账号集合已变化，请重试同步")
	}
	catalog, balance, err := s.read(ctx, *record, scope)
	failureScope := readFailureScope(scope, err)
	if err != nil && IsAuthenticationError(err) {
		rotated, refreshErr := s.refresher.Refresh(ctx, *record)
		if refreshErr != nil {
			return s.failed(ctx, host, failureScope, true, "refresh_token 续签失败："+refreshErr.Error())
		}
		if saveErr := s.private.SaveAuthRecord(ctx, rotated, allAuthFields()); saveErr != nil {
			return s.failed(ctx, host, failureScope, true, "新鉴权信息保存失败："+saveErr.Error())
		}
		recovered = true
		catalog, balance, err = s.read(ctx, rotated, scope)
		failureScope = readFailureScope(scope, err)
	}
	if err != nil {
		return s.failed(ctx, host, failureScope, IsAuthenticationError(err), err.Error())
	}
	write := business.UpstreamSyncWrite{
		Host: host, Catalog: catalog, Balance: balance, NameOnly: scope.Name, KeyID: scope.KeyID,
		AuthRecovered: recovered, AuthenticationOK: !scope.Name, AuthMethod: record.AuthMode,
	}
	persisted, err := s.repository.ApplyUpstreamSync(ctx, write)
	if err != nil {
		return s.failed(ctx, host, scope, false, "本地同步提交失败："+err.Error())
	}
	status := business.UpstreamAuthStatusAuthenticated
	if scope.Name {
		status = "未变更"
	} else if recovered {
		status = business.UpstreamAuthStatusRecovered
	}
	if _, eventErr := s.repository.RecordRuntimeEvent(ctx, "upstream.sync", "succeeded", "上游同步完成："+host, map[string]any{
		"actor": actorOrConsole(actor), "host": host, "catalog": scope.Catalog, "balance": scope.Balance, "name": scope.Name,
		"key_id": scope.KeyID, "auth_recovered": recovered, "group_count": persisted.GroupCount, "key_count": persisted.KeyCount,
	}); eventErr != nil {
		slog.Error("上游同步成功事件保存失败", "host", host, "error", eventErr)
	}
	return HostResult{
		Host: host, Status: "succeeded", AuthStatus: status, BalanceStatus: persisted.BalanceStatus,
		Balance: persisted.Balance, DisplayBalance: persisted.DisplayBalance, BalanceUnit: persisted.BalanceUnit,
		GroupCount: persisted.GroupCount, KeyCount: persisted.KeyCount,
		AccountTotal: persisted.AccountTotal, AccountRateSucceeded: persisted.AccountRateSucceeded,
		AccountRateFailed: persisted.AccountRateFailed, AuthRecovered: recovered,
		Catalog: catalog,
	}
}

func stringIDsCover(locked, current []string) bool {
	values := make(map[string]struct{}, len(locked))
	for _, value := range locked {
		values[strings.TrimSpace(value)] = struct{}{}
	}
	for _, value := range current {
		if _, found := values[strings.TrimSpace(value)]; !found {
			return false
		}
	}
	return true
}

func (s *Service) authRecord(ctx context.Context, host, actor string) (*configstore.AuthRecord, bool, error) {
	record, err := s.private.AuthRecord(ctx, host)
	if err != nil || record != nil || s.resolver == nil {
		return record, false, err
	}
	record, err = s.resolver.ResolveAuth(ctx, host, actor)
	if err != nil {
		return nil, false, err
	}
	return record, record != nil, nil
}

func (s *Service) read(ctx context.Context, record configstore.AuthRecord, scope Scope) (*business.UpstreamCatalogSnapshot, *business.UpstreamBalanceObservation, error) {
	var catalog *business.UpstreamCatalogSnapshot
	var balance *business.UpstreamBalanceObservation
	if scope.Catalog && scope.Balance {
		type catalogOutcome struct {
			value business.UpstreamCatalogSnapshot
			err   error
		}
		type balanceOutcome struct {
			value business.UpstreamBalanceObservation
			err   error
		}
		catalogResult := make(chan catalogOutcome, 1)
		balanceResult := make(chan balanceOutcome, 1)
		go func() {
			value, err := s.reader.ReadCatalog(ctx, record)
			catalogResult <- catalogOutcome{value: value, err: err}
		}()
		go func() {
			value, err := s.reader.ReadBalance(ctx, record)
			balanceResult <- balanceOutcome{value: value, err: err}
		}()
		catalogRead, balanceRead := <-catalogResult, <-balanceResult
		if catalogRead.err != nil || balanceRead.err != nil {
			return nil, nil, &readScopeError{catalog: catalogRead.err, balance: balanceRead.err}
		}
		catalog, balance = &catalogRead.value, &balanceRead.value
		return catalog, balance, nil
	}
	if scope.Catalog {
		value, err := s.reader.ReadCatalog(ctx, record)
		if err != nil {
			return nil, nil, &readScopeError{catalog: err}
		}
		catalog = &value
	}
	if scope.Balance {
		value, err := s.reader.ReadBalance(ctx, record)
		if err != nil {
			return nil, nil, &readScopeError{balance: err}
		}
		balance = &value
	}
	if scope.Name {
		reader, ok := s.reader.(siteNameReader)
		if !ok {
			return nil, nil, errors.New("上游名称读取能力不可用")
		}
		value, err := reader.ReadSiteName(ctx, record)
		if err != nil {
			return nil, nil, err
		}
		balance = &business.UpstreamBalanceObservation{SiteName: value, Status: "未读取"}
	}
	return catalog, balance, nil
}

func (s *Service) failed(ctx context.Context, host string, scope Scope, authenticationFailure bool, reason string) HostResult {
	reason = safeReason(reason)
	if persistenceErr := s.repository.RecordUpstreamSyncFailure(ctx, host, scopeName(scope), reason, authenticationFailure); persistenceErr != nil {
		reason += "；失败状态保存失败：" + safeReason(persistenceErr.Error())
	}
	status, authStatus := "failed", "未变更"
	if authenticationFailure {
		status, authStatus = "auth_failed", business.UpstreamAuthStatusInvalid
	}
	if _, eventErr := s.repository.RecordRuntimeEvent(ctx, "upstream.sync", "failed", "上游同步失败："+host, map[string]any{
		"host": host, "reason": reason, "authentication_failure": authenticationFailure,
	}); eventErr != nil {
		slog.Error("上游同步失败事件保存失败", "host", host, "error", eventErr)
	}
	return failedHost(host, status, authStatus, reason)
}

func failedHost(host, status, authStatus, reason string) HostResult {
	return HostResult{Host: host, Status: status, AuthStatus: authStatus, BalanceStatus: "未读取", Reason: &reason}
}

func normalizeScope(scope Scope) (Scope, error) {
	if !scope.Catalog && !scope.Balance && !scope.Name {
		return Scope{}, errors.New("上游同步至少需要包含名称、余额或分组目录")
	}
	if scope.Name && (scope.Catalog || scope.Balance || scope.KeyID != nil) {
		return Scope{}, errors.New("上游名称修复必须独立执行")
	}
	if scope.KeyID != nil {
		keyID := strings.TrimSpace(*scope.KeyID)
		if !scope.Catalog || keyID == "" || len(keyID) > 255 {
			return Scope{}, errors.New("按 Key 同步必须包含有效稳定 Key ID 和分组目录")
		}
		scope.KeyID = &keyID
	}
	return scope, nil
}

func scopeName(scope Scope) string {
	if scope.Name {
		return "name"
	}
	if scope.KeyID != nil {
		if scope.Balance {
			return "key_balance"
		}
		return "key"
	}
	if scope.Catalog && !scope.Balance {
		return "catalog"
	}
	if scope.Balance && !scope.Catalog {
		return "balance"
	}
	return "all"
}

func readFailureScope(requested Scope, err error) Scope {
	var scoped *readScopeError
	if err == nil || !errors.As(err, &scoped) {
		return requested
	}
	result := Scope{Catalog: scoped.catalog != nil, Balance: scoped.balance != nil}
	if result.Catalog {
		result.KeyID = requested.KeyID
	}
	return result
}

func scopeMessage(scope Scope) string {
	switch scopeName(scope) {
	case "name":
		return "正在读取上游公开站点名称"
	case "catalog":
		return "正在同步上游分组和 Key 目录"
	case "balance":
		return "正在同步上游余额"
	default:
		return "正在同步上游分组、倍率、余额与鉴权状态"
	}
}

func canonicalHosts(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, raw := range values {
		host := configstore.CanonicalHost(raw)
		if host == "" {
			continue
		}
		if _, exists := seen[host]; exists {
			continue
		}
		seen[host] = struct{}{}
		result = append(result, host)
	}
	sort.Strings(result)
	return result
}

func safeReason(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "上游同步失败"
	}
	return truncateUTF8Bytes(strings.ToValidUTF8(redact.Secrets(value), "\uFFFD"), 500)
}

func truncateUTF8Bytes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return value[:limit]
}

func allAuthFields() map[string]bool {
	return map[string]bool{
		"base_url": true, "upstream_type": true, "auth_mode": true, "access_token": true, "refresh_token": true,
		"admin_key": true, "user_id": true, "headers": true, "cookies": true,
	}
}

func actorOrConsole(value string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return "console"
}

func taskID() (string, error) {
	value := make([]byte, 6)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
