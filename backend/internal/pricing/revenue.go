package pricing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

const revenueTimezone = "Asia/Shanghai"

type RevenueRequest struct {
	Date string `json:"date"`
}

type RevenueRow struct {
	AccountID        string   `json:"account_id"`
	AccountName      string   `json:"account_name"`
	LocalGroup       string   `json:"local_group"`
	UpstreamHost     string   `json:"upstream_host"`
	UpstreamKeyName  string   `json:"upstream_key_name"`
	AccountCost      *float64 `json:"account_cost"`
	ActualCost       *float64 `json:"actual_cost"`
	UpstreamRawCost  *float64 `json:"upstream_raw_cost"`
	RechargeRate     *float64 `json:"recharge_rate"`
	UpstreamCost     *float64 `json:"upstream_cost"`
	Difference       *float64 `json:"difference"`
	Revenue          *float64 `json:"revenue"`
	Category         string   `json:"category"`
	Note             string   `json:"note"`
	AttributionLevel string   `json:"attribution_level"`
}

type RevenueSummary struct {
	Group        string  `json:"group"`
	Accounts     int     `json:"accounts"`
	AccountCost  float64 `json:"account_cost"`
	ActualCost   float64 `json:"actual_cost"`
	UpstreamRaw  float64 `json:"upstream_raw_cost"`
	UpstreamCost float64 `json:"upstream_cost"`
	Difference   float64 `json:"difference"`
	Revenue      float64 `json:"revenue"`
}

type RevenueIssue struct {
	Host   string `json:"host"`
	Reason string `json:"reason"`
}

type RevenueReport struct {
	ReportDate  string           `json:"report_date"`
	Timezone    string           `json:"timezone"`
	Tolerance   float64          `json:"tolerance"`
	Rows        []RevenueRow     `json:"rows"`
	Summaries   []RevenueSummary `json:"summaries"`
	Issues      []RevenueIssue   `json:"issues"`
	Comparable  int              `json:"comparable"`
	Unavailable int              `json:"unavailable"`
	Abnormal    int              `json:"abnormal"`
	GeneratedAt string           `json:"generated_at"`
}

type localUsageResult struct {
	totals adminclient.UsageTotals
	err    error
}

type upstreamUsageResult struct {
	host   string
	values map[string]upstreamsync.KeyUsageObservation
	issue  string
}

func (s *Service) EnqueueRevenue(ctx context.Context, request RevenueRequest, actor string) (taskstore.Task, error) {
	if s.tasks == nil {
		return taskstore.Task{}, errors.New("收入核算任务服务尚未就绪")
	}
	date, _, _, err := revenueWindow(request.Date, time.Now())
	if err != nil {
		return taskstore.Task{}, err
	}
	id, err := randomID()
	if err != nil {
		return taskstore.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{
		ID: id, Skill: "sub2api-billing-reconciliation", Operation: "revenue-calculation",
		Status: "queued", Progress: 0, Message: "收入核算已排队", Result: map[string]any{"report_date": date},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	go s.executeRevenue(task, RevenueRequest{Date: date}, actor)
	return task, nil
}

func (s *Service) executeRevenue(task taskstore.Task, request RevenueRequest, actor string) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	task.Status, task.Progress, task.Message, task.UpdatedAt = "running", 5, "正在读取本地计费和上游消费", time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.tasks.Save(ctx, task); err != nil {
		return
	}
	report, err := s.CalculateRevenue(ctx, request, actor)
	task.Progress, task.UpdatedAt = 100, time.Now().UTC().Format(time.RFC3339Nano)
	if err != nil {
		task.Status, task.Message = "failed", "收入核算失败："+err.Error()
		task.Result = map[string]any{"error": err.Error(), "report_date": request.Date}
	} else {
		task.Status = "succeeded"
		task.Message = fmt.Sprintf("收入核算完成：精确核对 %d，无法核对 %d，计费异常 %d", report.Comparable, report.Unavailable, report.Abnormal)
		encoded, _ := json.Marshal(report)
		_ = json.Unmarshal(encoded, &task.Result)
	}
	taskstore.PersistFinal(s.tasks, task)
}

func (s *Service) CalculateRevenue(ctx context.Context, request RevenueRequest, actor string) (RevenueReport, error) {
	date, start, end, err := revenueWindow(request.Date, time.Now())
	if err != nil {
		return RevenueReport{}, err
	}
	catalog, err := s.repository.RevenueCatalog(ctx)
	if err != nil {
		return RevenueReport{}, fmt.Errorf("Console 本地账号绑定读取失败：%w", err)
	}
	settings, err := s.targets.TargetSettings(ctx)
	if err != nil {
		return RevenueReport{}, err
	}
	management, err := adminclient.New(adminclient.Config{
		BaseURL: settings.BaseURL, AdminKey: settings.AdminKey,
		Timeout: time.Duration(settings.TimeoutSeconds) * time.Second, Attempts: 2,
	}, nil)
	if err != nil {
		return RevenueReport{}, err
	}
	authStore, ok := s.targets.(AuthStore)
	if !ok {
		return RevenueReport{}, errors.New("收入核算私有授权存储尚未就绪")
	}

	local := fetchLocalRevenueUsage(ctx, management, catalog.Accounts, date)
	upstream, issues := s.fetchUpstreamRevenueUsage(ctx, authStore, catalog.Accounts, date, start, end, actor)
	shared := sharedRevenueKeys(catalog.Accounts)
	report := RevenueReport{
		ReportDate: date, Timezone: revenueTimezone, Tolerance: 2,
		Rows: []RevenueRow{}, Summaries: []RevenueSummary{}, Issues: issues,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	for _, account := range catalog.Accounts {
		row := buildRevenueRow(account, local[account.ID], upstream, shared)
		switch row.Category {
		case "无法核对":
			report.Unavailable++
		case "计费异常":
			report.Comparable++
			report.Abnormal++
		default:
			report.Comparable++
		}
		report.Rows = append(report.Rows, row)
	}
	sort.SliceStable(report.Rows, func(left, right int) bool {
		leftRank, rightRank := revenueCategoryRank(report.Rows[left].Category), revenueCategoryRank(report.Rows[right].Category)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if report.Rows[left].LocalGroup != report.Rows[right].LocalGroup {
			return report.Rows[left].LocalGroup < report.Rows[right].LocalGroup
		}
		return numericLess(report.Rows[left].AccountID, report.Rows[right].AccountID)
	})
	report.Summaries = summarizeRevenue(report.Rows)
	return report, nil
}

func revenueWindow(raw string, now time.Time) (string, time.Time, time.Time, error) {
	location, err := time.LoadLocation(revenueTimezone)
	if err != nil {
		return "", time.Time{}, time.Time{}, errors.New("收入核算时区不可用")
	}
	date := strings.TrimSpace(raw)
	if date == "" {
		date = now.In(location).AddDate(0, 0, -1).Format("2006-01-02")
	}
	parsed, err := time.ParseInLocation("2006-01-02", date, location)
	if err != nil || parsed.Format("2006-01-02") != date {
		return "", time.Time{}, time.Time{}, errors.New("核算日期必须是 YYYY-MM-DD")
	}
	today := time.Date(now.In(location).Year(), now.In(location).Month(), now.In(location).Day(), 0, 0, 0, 0, location)
	if !parsed.Before(today) {
		return "", time.Time{}, time.Time{}, errors.New("收入只能核算已结束的完整自然日")
	}
	return date, parsed.UTC(), parsed.AddDate(0, 0, 1).UTC(), nil
}

func fetchLocalRevenueUsage(ctx context.Context, client *adminclient.Client, accounts []business.RevenueAccount, date string) map[string]localUsageResult {
	result := make(map[string]localUsageResult, len(accounts))
	jobs := make(chan business.RevenueAccount)
	values := make(chan struct {
		id string
		localUsageResult
	}, len(accounts))
	workers := 8
	if len(accounts) < workers {
		workers = len(accounts)
	}
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for account := range jobs {
				totals, err := client.AccountUsageTotals(ctx, account.ID, date, revenueTimezone)
				values <- struct {
					id string
					localUsageResult
				}{account.ID, localUsageResult{totals: totals, err: err}}
			}
		}()
	}
	go func() {
		for _, account := range accounts {
			jobs <- account
		}
		close(jobs)
		wait.Wait()
		close(values)
	}()
	for value := range values {
		result[value.id] = value.localUsageResult
	}
	return result
}

func (s *Service) fetchUpstreamRevenueUsage(
	ctx context.Context,
	authStore AuthStore,
	accounts []business.RevenueAccount,
	date string,
	start, end time.Time,
	actor string,
) (map[string]upstreamsync.KeyUsageObservation, []RevenueIssue) {
	bindingsByHost := map[string][]business.RevenueBinding{}
	for _, account := range accounts {
		for _, binding := range account.Bindings {
			host := configstore.CanonicalHost(binding.AuthHost)
			if host != "" {
				bindingsByHost[host] = append(bindingsByHost[host], binding)
			}
		}
	}
	hosts := make([]string, 0, len(bindingsByHost))
	for host := range bindingsByHost {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	jobs := make(chan string)
	results := make(chan upstreamUsageResult, len(hosts))
	workers := 4
	if len(hosts) < workers {
		workers = len(hosts)
	}
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for host := range jobs {
				results <- s.fetchRevenueHost(ctx, authStore, host, bindingsByHost[host], date, start, end, actor)
			}
		}()
	}
	go func() {
		for _, host := range hosts {
			jobs <- host
		}
		close(jobs)
		wait.Wait()
		close(results)
	}()
	values := map[string]upstreamsync.KeyUsageObservation{}
	issues := []RevenueIssue{}
	for result := range results {
		if result.issue != "" {
			issues = append(issues, RevenueIssue{Host: result.host, Reason: result.issue})
		}
		for keyID, value := range result.values {
			values[revenueKey(result.host, keyID)] = value
		}
	}
	sort.Slice(issues, func(left, right int) bool { return issues[left].Host < issues[right].Host })
	return values, issues
}

func (s *Service) fetchRevenueHost(
	ctx context.Context,
	authStore AuthStore,
	host string,
	bindings []business.RevenueBinding,
	date string,
	start, end time.Time,
	actor string,
) upstreamUsageResult {
	result := upstreamUsageResult{host: host, values: map[string]upstreamsync.KeyUsageObservation{}}
	record, err := authStore.AuthRecord(ctx, host)
	if err != nil || record == nil {
		if err == nil {
			err = errors.New("未找到私有授权记录")
		}
		result.issue = err.Error()
		return result
	}
	read := func(current configstore.AuthRecord) error {
		kind := strings.ToLower(strings.TrimSpace(current.UpstreamType))
		if strings.Contains(kind, "newapi") || strings.Contains(kind, "oneapi") {
			observation, err := s.usage.ReadNewAPIKeyUsage(ctx, current, start, end)
			if err != nil {
				return err
			}
			if err := s.repository.ValidateNewAPIQuotaUnit(ctx, host, observation.QuotaPerUnit, start, end); err != nil {
				return err
			}
			for keyID, value := range observation.Keys {
				result.values[keyID] = value
			}
			// A complete host read proves an absent stable Token had exact zero usage.
			for _, binding := range bindings {
				keyID := strings.TrimSpace(binding.UpstreamKeyID)
				if keyID != "" {
					if _, found := result.values[keyID]; !found {
						result.values[keyID] = upstreamsync.KeyUsageObservation{Cost: 0, Source: "newapi-token-logs"}
					}
				}
			}
			return nil
		}
		if !strings.Contains(kind, "sub2api") {
			return errors.New("未知上游类型，无法读取稳定 Key 消费")
		}
		seen := map[string]struct{}{}
		failures := make([]string, 0)
		for _, binding := range bindings {
			keyID := strings.TrimSpace(binding.UpstreamKeyID)
			if keyID == "" {
				continue
			}
			if _, duplicate := seen[keyID]; duplicate {
				continue
			}
			seen[keyID] = struct{}{}
			value, err := s.usage.ReadSub2APIKeyUsage(ctx, current, keyID, date, revenueTimezone)
			if err != nil {
				if upstreamsync.IsAuthenticationError(err) {
					return fmt.Errorf("Key %s：%w", keyID, err)
				}
				failures = append(failures, fmt.Sprintf("Key %s：%s", keyID, err.Error()))
				continue
			}
			result.values[keyID] = value
		}
		if len(failures) > 0 {
			return errors.New(strings.Join(failures, "；"))
		}
		return nil
	}
	err = read(*record)
	if err != nil && upstreamsync.IsAuthenticationError(err) && s.resolver != nil {
		if recovered, recoveryErr := s.resolver.ResolveAuth(ctx, host, actor); recoveryErr == nil && recovered != nil {
			result.values = map[string]upstreamsync.KeyUsageObservation{}
			err = read(*recovered)
		}
	}
	if err != nil {
		if len(result.values) == 0 {
			result.values = map[string]upstreamsync.KeyUsageObservation{}
		}
		result.issue = err.Error()
	}
	return result
}

func buildRevenueRow(
	account business.RevenueAccount,
	local localUsageResult,
	upstream map[string]upstreamsync.KeyUsageObservation,
	shared map[string]struct{},
) RevenueRow {
	row := RevenueRow{
		AccountID: account.ID, AccountName: account.Name, LocalGroup: strings.Join(account.Groups, ", "),
		Category: "无法核对", AttributionLevel: "unavailable",
	}
	if row.LocalGroup == "" {
		row.LocalGroup = "未分组"
	}
	if local.err == nil {
		row.AccountCost = revenueFloat(local.totals.AccountCost)
		row.ActualCost = revenueFloat(local.totals.ActualCost)
	}
	if len(account.Bindings) == 0 {
		row.Note = localRevenueNote(local.err, "缺少上游绑定")
		return row
	}
	if len(account.Bindings) != 1 {
		row.Note = localRevenueNote(local.err, "账号存在多个上游绑定，无法唯一归因")
		return row
	}
	binding := account.Bindings[0]
	host := configstore.CanonicalHost(binding.AuthHost)
	keyID := strings.TrimSpace(binding.UpstreamKeyID)
	row.UpstreamHost, row.UpstreamKeyName = host, binding.UpstreamKeyName
	if local.err != nil {
		row.Note = "本地计费读取失败：" + local.err.Error()
		return row
	}
	if keyID == "" {
		row.Note = "绑定缺少稳定 Key/Token ID"
		return row
	}
	if _, duplicate := shared[revenueKey(host, keyID)]; duplicate {
		row.Note = "多个本地账号共享同一稳定 Key/Token，未重复计算消费"
		return row
	}
	observation, found := upstream[revenueKey(host, keyID)]
	if !found {
		row.Note = "未取到该稳定 Key/Token 的精确消费"
		return row
	}
	recharge, err := strconv.ParseFloat(strings.TrimSpace(binding.RechargeRate), 64)
	if err != nil || math.IsNaN(recharge) || math.IsInf(recharge, 0) || recharge <= 0 {
		row.Note = "充值倍率缺失或无效"
		return row
	}
	converted := observation.Cost / recharge
	difference := local.totals.AccountCost - converted
	revenue := local.totals.ActualCost - converted
	row.UpstreamRawCost, row.RechargeRate, row.UpstreamCost = revenueFloat(observation.Cost), revenueFloat(recharge), revenueFloat(converted)
	row.Difference, row.Revenue = revenueFloat(difference), revenueFloat(revenue)
	row.AttributionLevel = "key"
	if math.Abs(difference) <= 2 {
		row.Category, row.Note = "正常", "-"
	} else {
		row.Category = "计费异常"
		if difference < 0 {
			row.Note = fmt.Sprintf("差额亏损 %.2f", math.Abs(difference))
		} else {
			row.Note = fmt.Sprintf("差额盈余 %.2f", difference)
		}
	}
	return row
}

func summarizeRevenue(rows []RevenueRow) []RevenueSummary {
	grouped := map[string]*RevenueSummary{}
	for _, row := range rows {
		if row.AttributionLevel != "key" || row.AccountCost == nil || row.ActualCost == nil || row.UpstreamRawCost == nil || row.UpstreamCost == nil {
			continue
		}
		summary := grouped[row.LocalGroup]
		if summary == nil {
			summary = &RevenueSummary{Group: row.LocalGroup}
			grouped[row.LocalGroup] = summary
		}
		summary.Accounts++
		summary.AccountCost += *row.AccountCost
		summary.ActualCost += *row.ActualCost
		summary.UpstreamRaw += *row.UpstreamRawCost
		summary.UpstreamCost += *row.UpstreamCost
		summary.Difference += *row.Difference
		summary.Revenue += *row.Revenue
	}
	groups := make([]string, 0, len(grouped))
	for group := range grouped {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	result := make([]RevenueSummary, 0, len(groups)+1)
	total := RevenueSummary{Group: "合计"}
	for _, group := range groups {
		value := *grouped[group]
		result = append(result, value)
		total.Accounts += value.Accounts
		total.AccountCost += value.AccountCost
		total.ActualCost += value.ActualCost
		total.UpstreamRaw += value.UpstreamRaw
		total.UpstreamCost += value.UpstreamCost
		total.Difference += value.Difference
		total.Revenue += value.Revenue
	}
	result = append(result, total)
	return result
}

func sharedRevenueKeys(accounts []business.RevenueAccount) map[string]struct{} {
	counts := map[string]int{}
	for _, account := range accounts {
		for _, binding := range account.Bindings {
			host, keyID := configstore.CanonicalHost(binding.AuthHost), strings.TrimSpace(binding.UpstreamKeyID)
			if host != "" && keyID != "" {
				counts[revenueKey(host, keyID)]++
			}
		}
	}
	result := map[string]struct{}{}
	for key, count := range counts {
		if count > 1 {
			result[key] = struct{}{}
		}
	}
	return result
}

func revenueKey(host, keyID string) string {
	return configstore.CanonicalHost(host) + "\x00" + strings.TrimSpace(keyID)
}

func revenueFloat(value float64) *float64 {
	value = math.Round(value*1_000_000) / 1_000_000
	return &value
}

func localRevenueNote(err error, fallback string) string {
	if err != nil {
		return fallback + "；本地计费读取失败：" + err.Error()
	}
	return fallback
}

func revenueCategoryRank(category string) int {
	switch category {
	case "计费异常":
		return 0
	case "正常":
		return 1
	default:
		return 2
	}
}
