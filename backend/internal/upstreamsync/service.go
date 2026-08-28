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

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/redact"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type Repository interface {
	Upstreams(context.Context) (business.UpstreamSummary, error)
	ApplyUpstreamSync(context.Context, business.UpstreamSyncWrite) (business.UpstreamSyncWriteResult, error)
	RecordUpstreamSyncFailure(context.Context, string, string, string, bool) error
	RecordRuntimeEvent(context.Context, string, string, string, map[string]any) (int64, error)
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

type TaskStore interface {
	Save(context.Context, taskstore.Task) error
}

type Scope struct {
	Catalog bool
	Balance bool
	Name    bool
	KeyID   *string
}

type siteNameReader interface {
	ReadSiteName(context.Context, configstore.AuthRecord) (*string, error)
}

type HostResult struct {
	Host                 string  `json:"host"`
	Status               string  `json:"status"`
	AuthStatus           string  `json:"auth_status"`
	BalanceStatus        string  `json:"balance_status"`
	Balance              *string `json:"balance,omitempty"`
	GroupCount           int     `json:"group_count"`
	KeyCount             int     `json:"key_count"`
	AccountTotal         int     `json:"account_total"`
	AccountRateSucceeded int     `json:"account_rate_succeeded"`
	AccountRateFailed    int     `json:"account_rate_failed"`
	AuthRecovered        bool    `json:"auth_recovered"`
	Reason               *string `json:"reason,omitempty"`
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
	tasks      TaskStore
	timeout    time.Duration
	workers    int
}

func New(repository Repository, private PrivateStore, reader CatalogReader, refresher Refresher, tasks TaskStore) *Service {
	return &Service{repository: repository, private: private, reader: reader, refresher: refresher, tasks: tasks, timeout: 30 * time.Minute, workers: 4}
}

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
	record, err := s.private.AuthRecord(ctx, host)
	if err != nil {
		return taskstore.Task{}, err
	}
	if record == nil {
		return taskstore.Task{}, errors.New("未找到该 Host 的私有授权记录")
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
	go s.execute(task, hosts, scope, actor, summary)
	return task, nil
}

func (s *Service) execute(task taskstore.Task, hosts []string, scope Scope, actor string, summary business.UpstreamSummary) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	task.Status, task.Progress, task.Message, task.UpdatedAt = "running", 5, scopeMessage(scope), time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.tasks.Save(ctx, task); err != nil {
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
	task.Progress, task.UpdatedAt = 100, time.Now().UTC().Format(time.RFC3339Nano)
	task.Result = map[string]any{
		"total": result.Total, "succeeded": result.Succeeded, "auth_failed": result.AuthFailed,
		"failed": result.Failed, "account_total": result.AccountTotal,
		"account_rate_succeeded": result.AccountRateSucceeded, "account_rate_failed": result.AccountRateFailed,
		"hosts": result.Hosts, "remote_write": false, "credentials_exposed": false,
	}
	if result.AuthFailed > 0 || result.Failed > 0 {
		task.Status = "failed"
		task.Message = fmt.Sprintf("上游同步完成：成功 %d，鉴权失败 %d，其他失败 %d", result.Succeeded, result.AuthFailed, result.Failed)
	} else {
		task.Status = "succeeded"
		task.Message = fmt.Sprintf("上游同步完成：成功 %d", result.Succeeded)
	}
	taskstore.PersistFinal(s.tasks, task)
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
			result.Hosts[index] = failedHost(hosts[index], "failed", "未确认", reason)
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
	record, err := s.private.AuthRecord(ctx, host)
	if err != nil {
		return s.failed(ctx, host, scope, false, "私有授权记录读取失败："+err.Error())
	}
	if record == nil {
		return s.failed(ctx, host, scope, true, "未配置私有授权记录")
	}
	catalog, balance, err := s.read(ctx, *record, scope)
	recovered := false
	if err != nil && IsAuthenticationError(err) {
		rotated, refreshErr := s.refresher.Refresh(ctx, *record)
		if refreshErr != nil {
			return s.failed(ctx, host, scope, true, "refresh_token 续签失败："+refreshErr.Error())
		}
		if err := s.private.SaveAuthRecord(ctx, rotated, allAuthFields()); err != nil {
			return s.failed(ctx, host, scope, true, "新鉴权信息保存失败："+err.Error())
		}
		recovered = true
		catalog, balance, err = s.read(ctx, rotated, scope)
	}
	if err != nil {
		return s.failed(ctx, host, scope, IsAuthenticationError(err), err.Error())
	}
	write := business.UpstreamSyncWrite{Host: host, Catalog: catalog, Balance: balance, NameOnly: scope.Name, KeyID: scope.KeyID, AuthRecovered: recovered, AuthenticationOK: !scope.Name}
	persisted, err := s.repository.ApplyUpstreamSync(ctx, write)
	if err != nil {
		return s.failed(ctx, host, scope, false, "本地同步提交失败："+err.Error())
	}
	status := "已鉴权"
	if scope.Name {
		status = "未变更"
	} else if recovered {
		status = "已恢复"
	}
	if _, eventErr := s.repository.RecordRuntimeEvent(ctx, "upstream.sync", "succeeded", "上游同步完成："+host, map[string]any{
		"actor": actorOrConsole(actor), "host": host, "catalog": scope.Catalog, "balance": scope.Balance, "name": scope.Name,
		"key_id": scope.KeyID, "auth_recovered": recovered, "group_count": persisted.GroupCount, "key_count": persisted.KeyCount,
	}); eventErr != nil {
		slog.Error("上游同步成功事件保存失败", "host", host, "error", eventErr)
	}
	return HostResult{
		Host: host, Status: "succeeded", AuthStatus: status, BalanceStatus: persisted.BalanceStatus,
		Balance: persisted.Balance, GroupCount: persisted.GroupCount, KeyCount: persisted.KeyCount,
		AccountTotal: persisted.AccountTotal, AccountRateSucceeded: persisted.AccountRateSucceeded,
		AccountRateFailed: persisted.AccountRateFailed, AuthRecovered: recovered,
	}
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
		if catalogRead.err != nil {
			return nil, nil, catalogRead.err
		}
		if balanceRead.err != nil {
			return nil, nil, balanceRead.err
		}
		catalog, balance = &catalogRead.value, &balanceRead.value
		return catalog, balance, nil
	}
	if scope.Catalog {
		value, err := s.reader.ReadCatalog(ctx, record)
		if err != nil {
			return nil, nil, err
		}
		catalog = &value
	}
	if scope.Balance {
		value, err := s.reader.ReadBalance(ctx, record)
		if err != nil {
			return nil, nil, err
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
	status, authStatus := "failed", "未确认"
	if authenticationFailure {
		status, authStatus = "auth_failed", "鉴权失效"
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
	if scope.Catalog && !scope.Balance {
		return "catalog"
	}
	if scope.Balance && !scope.Catalog {
		return "balance"
	}
	return "all"
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
	value = redact.Secrets(value)
	if len(value) > 500 {
		value = value[:500]
	}
	return value
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
