package adminclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/redact"
)

const maximumResponseBytes = 4 << 20

type Error struct{ Message string }

func (e *Error) Error() string { return e.Message }

// CommitUnknownError means a non-idempotent request may have reached the
// server, but its response was not trustworthy enough to determine the result.
type CommitUnknownError struct {
	Marker string
	Cause  error
}

func (e *CommitUnknownError) Error() string {
	message := "管理 API 写入的提交结果不确定"
	if e.Marker != "" {
		message += "（marker " + e.Marker + "）"
	}
	if e.Cause != nil {
		message += "：" + e.Cause.Error()
	}
	return message
}

func (e *CommitUnknownError) Unwrap() error { return e.Cause }

type responseOutcomeUnknown struct{ cause error }

func (e *responseOutcomeUnknown) Error() string { return e.cause.Error() }
func (e *responseOutcomeUnknown) Unwrap() error { return e.cause }

type AccountStillReadableError struct {
	AccountID string
	DeleteErr error
}

func (e *AccountStillReadableError) Error() string {
	message := fmt.Sprintf("账号 %s 删除后仍可读，已停止本地清理", e.AccountID)
	if e.DeleteErr != nil {
		return fmt.Sprintf("管理平台账号删除请求返回错误：%v；%s", e.DeleteErr, message)
	}
	return message
}

func (e *AccountStillReadableError) Unwrap() error { return e.DeleteErr }

type AccountReadbackUnknownError struct {
	AccountID   string
	DeleteErr   error
	ReadbackErr error
}

func (e *AccountReadbackUnknownError) Error() string {
	if e.DeleteErr != nil {
		return fmt.Sprintf(
			"管理平台账号删除请求返回错误：%v；账号 %s 删除后读回失败，删除结果未知：%v",
			e.DeleteErr,
			e.AccountID,
			e.ReadbackErr,
		)
	}
	return fmt.Sprintf("账号 %s 删除后读回失败，删除结果未知：%v", e.AccountID, e.ReadbackErr)
}

func (e *AccountReadbackUnknownError) Unwrap() []error {
	causes := make([]error, 0, 2)
	if e.DeleteErr != nil {
		causes = append(causes, e.DeleteErr)
	}
	if e.ReadbackErr != nil {
		causes = append(causes, e.ReadbackErr)
	}
	return causes
}

type HTTPError struct {
	StatusCode int
	Detail     string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("管理 API 返回 HTTP %d：%s", e.StatusCode, e.Detail)
}

type MonitoringDisabled struct{}

func (*MonitoringDisabled) Error() string { return "Sub2API 运维监控未开启" }

type Config struct {
	BaseURL  string
	AdminKey string
	Timeout  time.Duration
	Attempts int
}

type Client struct {
	baseURL  string
	adminKey string
	attempts int
	http     *http.Client
}

type EvidencePage struct {
	Items    []map[string]any
	Total    int
	Page     int
	PageSize int
}

type AccountUpstreamMultiplierResult struct {
	Multiplier string
	Err        error
}

type UsageTotals struct {
	AccountCost string
	ActualCost  string
}

func New(config Config, transport http.RoundTripper) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("管理地址必须是完整的 http 或 https URL")
	}
	if strings.TrimSpace(config.AdminKey) == "" {
		return nil, errors.New("管理密钥不能为空")
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.Timeout < time.Second || config.Timeout > 120*time.Second {
		return nil, errors.New("timeout 必须在 1 到 120 秒之间")
	}
	if config.Attempts == 0 {
		config.Attempts = 3
	}
	if config.Attempts < 1 || config.Attempts > 5 {
		return nil, errors.New("attempts 必须在 1 到 5 之间")
	}
	client := &http.Client{Timeout: config.Timeout, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if transport == nil {
		client.Transport = http.DefaultTransport
	}
	return &Client{baseURL: baseURL, adminKey: strings.TrimSpace(config.AdminKey), attempts: config.Attempts, http: client}, nil
}

func (c *Client) Accounts(ctx context.Context) ([]map[string]any, error) {
	return c.fetchPaged(ctx, "/admin/accounts", "账号目录")
}

func (c *Client) Groups(ctx context.Context) ([]map[string]any, error) {
	groupItems, err := c.fetchPaged(ctx, "/admin/groups", "分组目录")
	if err == nil {
		return groupItems, nil
	}
	var httpError *HTTPError
	var contractError *Error
	missingRoute := errors.As(err, &httpError) && httpError.StatusCode == http.StatusNotFound
	shapeError := errors.As(err, &contractError) && (strings.Contains(contractError.Message, "缺少 items") ||
		strings.Contains(contractError.Message, "无有效 items") ||
		strings.Contains(contractError.Message, "格式不可读"))
	if !missingRoute && !shapeError {
		return nil, err
	}
	payload, err := c.request(ctx, http.MethodGet, "/admin/groups/all", nil, nil)
	if err != nil {
		return nil, err
	}
	groupItems, _, err = items(payload, "分组目录")
	if err != nil {
		return nil, err
	}
	return uniqueStableItems(groupItems, "分组目录")
}

func (c *Client) Account(ctx context.Context, accountID string) (map[string]any, error) {
	if !stableID(accountID) {
		return nil, errors.New("账号 ID 必须是稳定数字 ID")
	}
	return c.resourceDetail(ctx, "/admin/accounts/"+accountID, accountID, "账号")
}

func (c *Client) AccountUsageTotals(ctx context.Context, accountID, date, timezone string) (UsageTotals, error) {
	if !stableID(accountID) {
		return UsageTotals{}, errors.New("账号 ID 必须是稳定数字 ID")
	}
	payload, err := c.request(ctx, http.MethodGet, "/admin/usage/stats", nil, map[string]string{
		"account_id": accountID, "start_date": date, "end_date": date,
		"timezone": timezone, "nocache": "true",
	})
	if err != nil {
		return UsageTotals{}, err
	}
	data, err := responseObject(payload, "账号计费统计")
	if err != nil {
		return UsageTotals{}, err
	}
	accountCost, err := exactJSONDecimal(data["total_account_cost"])
	if err != nil {
		return UsageTotals{}, errors.New("账号计费统计未返回有效 total_account_cost")
	}
	actualCost, err := exactJSONDecimal(data["total_actual_cost"])
	if err != nil {
		return UsageTotals{}, errors.New("账号计费统计未返回有效 total_actual_cost")
	}
	return UsageTotals{AccountCost: accountCost, ActualCost: actualCost}, nil
}

func exactJSONDecimal(value any) (string, error) {
	var text string
	switch raw := value.(type) {
	case json.Number:
		text = raw.String()
	case float64:
		text = strconv.FormatFloat(raw, 'g', -1, 64)
	case string:
		text = strings.TrimSpace(raw)
	default:
		return "", errors.New("数值字段类型无效")
	}
	parsed, ok := new(big.Rat).SetString(text)
	if !ok {
		return "", errors.New("数值字段不是有限十进制数")
	}
	return normalizedDecimal(parsed), nil
}

// UpdateAccount returns the complete account object supplied by Sub2API's
// update response so callers can confirm changed fields without another GET.
func (c *Client) UpdateAccount(ctx context.Context, accountID string, body map[string]any) (map[string]any, error) {
	if !stableID(accountID) {
		return nil, errors.New("账号 ID 必须是稳定数字 ID")
	}
	payload, err := c.Mutate(ctx, http.MethodPut, "/admin/accounts/"+accountID, body)
	if err != nil {
		return nil, err
	}
	return responseObject(payload, "账号更新")
}

func (c *Client) RecoverAccountState(ctx context.Context, accountID string) (map[string]any, error) {
	if !stableID(accountID) {
		return nil, errors.New("账号 ID 必须是稳定数字 ID")
	}
	payload, err := c.Mutate(ctx, http.MethodPost, "/admin/accounts/"+accountID+"/recover-state", nil)
	if err != nil {
		return nil, err
	}
	return responseObject(payload, "账号状态恢复")
}

func (c *Client) SetAccountSchedulable(ctx context.Context, accountID string, schedulable bool) (map[string]any, error) {
	if !stableID(accountID) {
		return nil, errors.New("账号 ID 必须是稳定数字 ID")
	}
	payload, err := c.Mutate(ctx, http.MethodPost, "/admin/accounts/"+accountID+"/schedulable", map[string]any{
		"schedulable": schedulable,
	})
	if err != nil {
		return nil, err
	}
	return responseObject(payload, "账号调度状态更新")
}

func (c *Client) UpdateAccountGroups(ctx context.Context, accountID string, groupIDs []int64) (map[string]any, error) {
	return c.UpdateAccountGroupsAndBaseURL(ctx, accountID, groupIDs, nil)
}

func (c *Client) UpdateAccountGroupsAndBaseURL(ctx context.Context, accountID string, groupIDs []int64, baseURL *string) (map[string]any, error) {
	if !stableID(accountID) {
		return nil, errors.New("账号 ID 必须是稳定数字 ID")
	}
	expected, ok := stableIDValues(groupIDs)
	if !ok {
		return nil, errors.New("账号分组必须使用稳定数字 ID")
	}
	body := map[string]any{"group_ids": groupIDs}
	if baseURL != nil {
		normalized := strings.TrimRight(strings.TrimSpace(*baseURL), "/")
		if normalized == "" {
			return nil, errors.New("账号 Base URL 不能为空")
		}
		body["credentials"] = map[string]any{"base_url": normalized}
		baseURL = &normalized
	}
	if _, err := c.Mutate(ctx, http.MethodPut, "/admin/accounts/"+accountID, body); err != nil {
		return nil, err
	}
	account, err := c.Account(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("账号分组写后确认失败：%w", err)
	}
	actual, ok := stableIDValues(account["group_ids"])
	if !ok || strings.Join(expected, ",") != strings.Join(actual, ",") {
		return nil, errors.New("账号分组写后确认不一致")
	}
	if baseURL != nil && accountBaseURL(account) != *baseURL {
		return nil, errors.New("账号 Base URL 写后确认不一致")
	}
	return account, nil
}

func accountBaseURL(account map[string]any) string {
	raw := account["base_url"]
	if credentials, ok := account["credentials"].(map[string]any); ok {
		if value, present := credentials["base_url"]; present {
			raw = value
		}
	}
	if raw == nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(fmt.Sprint(raw)), "/")
}

// AccountUpstreamMultiplier asks Sub2API to authenticate with the selected
// account and probe the multiplier declared by that account's upstream.
// This is deliberately different from reading the account's stored
// rate_multiplier field from the management catalog.
func (c *Client) AccountUpstreamMultiplier(ctx context.Context, accountID string) (string, error) {
	if !stableID(accountID) {
		return "", errors.New("账号 ID 必须是稳定数字 ID")
	}
	payload, err := c.Mutate(ctx, http.MethodPost, "/admin/accounts/"+accountID+"/upstream-billing-probe", nil)
	if err != nil {
		return "", fmt.Errorf("上游倍率探测失败：%w", err)
	}
	result, err := responseObject(payload, "上游倍率探测")
	if err != nil {
		return "", err
	}
	return upstreamMultiplierFromProbeResult(result)
}

// AccountUpstreamMultipliers uses Sub2API's native batch probe. Each returned
// item is kept independent so one upstream failure does not hide other rates.
func (c *Client) AccountUpstreamMultipliers(ctx context.Context, accountIDs []string) (map[string]AccountUpstreamMultiplierResult, error) {
	result := make(map[string]AccountUpstreamMultiplierResult, len(accountIDs))
	unique := make([]string, 0, len(accountIDs))
	for _, accountID := range accountIDs {
		if !stableID(accountID) {
			return nil, errors.New("批量上游倍率探测包含无效账号 ID")
		}
		if _, exists := result[accountID]; exists {
			continue
		}
		result[accountID] = AccountUpstreamMultiplierResult{}
		unique = append(unique, accountID)
	}
	for offset := 0; offset < len(unique); offset += 20 {
		end := min(offset+20, len(unique))
		chunk := unique[offset:end]
		numericIDs := make([]int64, 0, len(chunk))
		for _, accountID := range chunk {
			parsed, _ := strconv.ParseInt(accountID, 10, 64)
			numericIDs = append(numericIDs, parsed)
		}
		payload, err := c.Mutate(ctx, http.MethodPost, "/admin/accounts/upstream-billing-probe/batch", map[string]any{"account_ids": numericIDs})
		if err != nil {
			return nil, fmt.Errorf("批量上游倍率探测失败：%w", err)
		}
		data, err := responseObject(payload, "批量上游倍率探测")
		if err != nil {
			return nil, err
		}
		rawResults, ok := data["results"].([]any)
		if !ok {
			return nil, errors.New("批量上游倍率探测未返回 results")
		}
		requested := make(map[string]struct{}, len(chunk))
		for _, accountID := range chunk {
			requested[accountID] = struct{}{}
		}
		seen := make(map[string]struct{}, len(chunk))
		for _, raw := range rawResults {
			item, ok := raw.(map[string]any)
			if !ok {
				return nil, errors.New("批量上游倍率探测返回无效项目")
			}
			accountID := strings.TrimSpace(fmt.Sprint(item["account_id"]))
			if _, wanted := requested[accountID]; !wanted {
				return nil, fmt.Errorf("批量上游倍率探测返回未请求账号 %q", accountID)
			}
			if _, duplicate := seen[accountID]; duplicate {
				return nil, fmt.Errorf("批量上游倍率探测重复返回账号 %s", accountID)
			}
			seen[accountID] = struct{}{}
			multiplier, itemErr := upstreamMultiplierFromProbeResult(item)
			result[accountID] = AccountUpstreamMultiplierResult{Multiplier: multiplier, Err: itemErr}
		}
		for _, accountID := range chunk {
			if _, found := seen[accountID]; !found {
				result[accountID] = AccountUpstreamMultiplierResult{Err: errors.New("批量上游倍率探测未返回该账号结果")}
			}
		}
	}
	return result, nil
}

func upstreamMultiplierFromProbeResult(result map[string]any) (string, error) {
	if detail := strings.TrimSpace(fmt.Sprint(result["error"])); detail != "" && detail != "<nil>" {
		return "", errors.New("上游倍率探测失败：" + detail)
	}
	snapshot, ok := result["snapshot"].(map[string]any)
	if !ok {
		return "", errors.New("上游倍率探测未返回倍率快照")
	}
	if !strings.EqualFold(strings.TrimSpace(fmt.Sprint(snapshot["status"])), "ok") {
		detail := strings.TrimSpace(fmt.Sprint(snapshot["last_error"]))
		if detail == "" || detail == "<nil>" {
			detail = strings.TrimSpace(fmt.Sprint(snapshot["error"]))
		}
		if detail == "" || detail == "<nil>" {
			detail = "未返回成功快照"
		}
		return "", errors.New("上游倍率探测失败：" + detail)
	}
	data, ok := snapshot["data"].(map[string]any)
	if !ok {
		return "", errors.New("上游倍率探测未返回有效数据")
	}
	if scope := strings.ToLower(strings.TrimSpace(fmt.Sprint(data["billing_scope"]))); scope != "" && scope != "<nil>" && scope != "token" {
		return "", fmt.Errorf("上游倍率探测返回不支持的计费范围 %q", scope)
	}
	// resolved_rate_multiplier includes the upstream's per-user override. The
	// effective value may additionally contain a temporary peak coefficient and
	// must not be frozen into the management account's static multiplier.
	for _, field := range []string{"resolved_rate_multiplier", "effective_rate_multiplier"} {
		raw, present := data[field]
		if !present || raw == nil {
			continue
		}
		value, ok := new(big.Rat).SetString(strings.TrimSpace(fmt.Sprint(raw)))
		if !ok || value.Sign() <= 0 {
			return "", errors.New("上游倍率探测返回非法倍率")
		}
		return normalizedDecimal(value), nil
	}
	return "", errors.New("上游倍率探测未返回有效倍率")
}

func responseObject(payload map[string]any, label string) (map[string]any, error) {
	if raw, present := payload["data"]; present {
		value, ok := raw.(map[string]any)
		if !ok {
			return nil, &Error{label + "返回格式不可读"}
		}
		return value, nil
	}
	return payload, nil
}

func normalizedDecimal(value *big.Rat) string {
	text := value.FloatString(28)
	text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	if text == "" || text == "-0" {
		return "0"
	}
	return text
}

func (c *Client) AccountModels(ctx context.Context, accountID string) ([]string, error) {
	return c.accountModelIDs(ctx, http.MethodGet, accountID, "/models")
}

func (c *Client) SyncAccountModels(ctx context.Context, accountID string) ([]string, error) {
	return c.accountModelIDs(ctx, http.MethodPost, accountID, "/models/sync-upstream")
}

func (c *Client) PreviewAccountModels(ctx context.Context, platform, accountType, baseURL, apiKey string) ([]string, error) {
	platform = strings.TrimSpace(platform)
	accountType = strings.TrimSpace(accountType)
	baseURL = strings.TrimSpace(baseURL)
	apiKey = strings.TrimSpace(apiKey)
	if platform == "" || accountType == "" || baseURL == "" || apiKey == "" {
		return nil, errors.New("开户模型同步参数不完整")
	}
	payload, err := c.request(ctx, http.MethodPost, "/admin/accounts/models/sync-upstream-preview", map[string]any{
		"platform": platform, "type": accountType, "base_url": baseURL, "api_key": apiKey,
	}, nil)
	if err != nil {
		return nil, err
	}
	models := collectModelIDs(payload)
	if len(models) == 0 {
		return nil, errors.New("上游模型同步未返回可用模型")
	}
	return models, nil
}

func (c *Client) accountModelIDs(ctx context.Context, method, accountID, suffix string) ([]string, error) {
	if !stableID(accountID) {
		return nil, errors.New("账号 ID 必须是稳定数字 ID")
	}
	payload, err := c.request(ctx, method, "/admin/accounts/"+accountID+suffix, nil, nil)
	if err != nil {
		return nil, err
	}
	return collectModelIDs(payload), nil
}

func collectModelIDs(raw any) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	var walk func(any)
	walk = func(value any) {
		switch item := value.(type) {
		case map[string]any:
			for _, key := range []string{"id", "model_id", "name"} {
				if text, ok := item[key].(string); ok {
					add(text)
					break
				}
			}
			for _, key := range []string{"data", "models", "items"} {
				if nested, present := item[key]; present {
					walk(nested)
				}
			}
		case []any:
			for _, nested := range item {
				walk(nested)
			}
		case string:
			add(item)
		}
	}
	walk(raw)
	sort.Strings(result)
	return result
}

func (c *Client) Group(ctx context.Context, groupID string) (map[string]any, error) {
	if !stableID(groupID) {
		return nil, errors.New("分组 ID 必须是稳定正整数")
	}
	return c.resourceDetail(ctx, "/admin/groups/"+groupID, groupID, "分组")
}

func (c *Client) resourceDetail(ctx context.Context, path, expectedID, label string) (map[string]any, error) {
	payload, err := c.request(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	data := payload
	if raw, present := payload["data"]; present {
		var ok bool
		data, ok = raw.(map[string]any)
		if !ok {
			return nil, &Error{label + "详情返回格式不可读"}
		}
	}
	if strings.TrimSpace(fmt.Sprint(data["id"])) != expectedID {
		return nil, &Error{label + "详情 ID 与请求不一致"}
	}
	return data, nil
}

func (c *Client) RequestDetails(ctx context.Context, accountID string, lookbackMinutes, maxSamples int) ([]map[string]any, error) {
	if !stableID(accountID) {
		return nil, errors.New("账号 ID 必须是稳定正整数")
	}
	if lookbackMinutes < 1 || maxSamples < 1 {
		return nil, errors.New("流量回溯和样本上限必须是正整数")
	}
	end := time.Now().UTC()
	start := end.Add(-time.Duration(lookbackMinutes) * time.Minute)
	return c.fetchEvidence(ctx, "/admin/ops/requests", "运维请求记录", map[string]string{
		"account_id": accountID, "kind": "all", "start_time": start.Format(time.RFC3339Nano), "end_time": end.Format(time.RFC3339Nano),
	}, maxSamples, "request_id", 100)
}

func (c *Client) RequestTrace(ctx context.Context, requestID string, lookbackMinutes, maxSamples int) ([]map[string]any, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || len(requestID) > 256 {
		return nil, errors.New("request_id 无效")
	}
	if lookbackMinutes < 1 || maxSamples < 1 {
		return nil, errors.New("请求回溯或样本上限必须是正整数")
	}
	end := time.Now().UTC()
	start := end.Add(-time.Duration(lookbackMinutes) * time.Minute)
	return c.fetchEvidence(ctx, "/admin/ops/requests", "运维请求追踪", map[string]string{
		"request_id": requestID, "kind": "all", "start_time": start.Format(time.RFC3339Nano), "end_time": end.Format(time.RFC3339Nano),
	}, maxSamples, "request_id", 100)
}

func (c *Client) SystemLogsByRequestID(ctx context.Context, requestID string, lookbackMinutes, maxSamples int) ([]map[string]any, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || len(requestID) > 256 {
		return nil, errors.New("request_id 无效")
	}
	if lookbackMinutes < 1 || maxSamples < 1 {
		return nil, errors.New("请求回溯或样本上限必须是正整数")
	}
	end := time.Now().UTC()
	start := end.Add(-time.Duration(lookbackMinutes) * time.Minute)
	return c.fetchEvidence(ctx, "/admin/ops/system-logs", "运维系统日志", map[string]string{
		"request_id": requestID, "start_time": start.Format(time.RFC3339Nano), "end_time": end.Format(time.RFC3339Nano),
	}, maxSamples, "id", 100)
}

func (c *Client) SystemLogs(ctx context.Context, filters map[string]string, page, pageSize int) (EvidencePage, error) {
	if page < 1 || pageSize < 1 || pageSize > 100 {
		return EvidencePage{}, errors.New("系统日志分页参数无效")
	}
	query := make(map[string]string, len(filters)+2)
	for key, value := range filters {
		if normalized := strings.TrimSpace(value); normalized != "" {
			query[key] = normalized
		}
	}
	query["page"] = strconv.Itoa(page)
	query["page_size"] = strconv.Itoa(pageSize)
	payload, err := c.request(ctx, http.MethodGet, "/admin/ops/system-logs", nil, query)
	if err != nil {
		return EvidencePage{}, err
	}
	rows, metadata, err := items(payload, "运维系统日志")
	if err != nil {
		return EvidencePage{}, err
	}
	total, present, err := pageTotal(metadata, "运维系统日志")
	if err != nil {
		return EvidencePage{}, err
	}
	if !present {
		total = len(rows)
	}
	return EvidencePage{Items: rows, Total: total, Page: page, PageSize: pageSize}, nil
}

func (c *Client) RequestErrors(ctx context.Context, accountID string, lookbackMinutes, maxSamples int) ([]map[string]any, error) {
	if !stableID(accountID) || lookbackMinutes < 1 || maxSamples < 1 {
		return nil, errors.New("账号 ID、请求回溯或样本上限无效")
	}
	end := time.Now().UTC()
	start := end.Add(-time.Duration(lookbackMinutes) * time.Minute)
	return c.fetchEvidence(ctx, "/admin/ops/requests", "运维错误请求", map[string]string{
		"account_id": accountID, "kind": "error", "start_time": start.Format(time.RFC3339Nano), "end_time": end.Format(time.RFC3339Nano),
	}, maxSamples, "request_id", 100)
}

func (c *Client) Mutate(ctx context.Context, method, path string, body map[string]any) (map[string]any, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method != http.MethodPost && method != http.MethodPut && method != http.MethodPatch && method != http.MethodDelete {
		return nil, errors.New("管理写操作方法不受支持")
	}
	return c.request(ctx, method, path, body, nil)
}

func (c *Client) CreateAccount(ctx context.Context, body map[string]any) (map[string]any, error) {
	return c.createAccount(ctx, body, "")
}

func (c *Client) CreateAccountWithMarker(ctx context.Context, body map[string]any, marker string) (map[string]any, error) {
	marker = strings.TrimSpace(marker)
	if marker == "" || len(marker) > 256 || strings.ContainsAny(marker, "\r\n") {
		return nil, errors.New("账号创建 marker 无效")
	}
	if !markerLinePresent(fmt.Sprint(body["notes"]), marker) {
		return nil, errors.New("账号创建备注缺少不可变 marker")
	}
	return c.createAccount(ctx, body, marker)
}

func (c *Client) createAccount(ctx context.Context, body map[string]any, marker string) (map[string]any, error) {
	var beforeIDs map[string]struct{}
	var baselineErr error
	if marker == "" {
		before, err := c.Accounts(ctx)
		beforeIDs, baselineErr = accountIDSet(before)
		if err != nil {
			baselineErr = err
		}
	}
	payload, err := c.requestWithSemantics(ctx, http.MethodPost, "/admin/accounts", body, nil, true)
	if err != nil {
		var unknown *CommitUnknownError
		if !errors.As(err, &unknown) {
			return nil, err
		}
		if marker == "" && baselineErr != nil {
			return nil, &CommitUnknownError{Cause: fmt.Errorf("%w；创建前目录基线不可用：%v", unknown.Cause, baselineErr)}
		}
		return c.reconcileCreatedAccount(ctx, beforeIDs, body, marker, &CommitUnknownError{Marker: marker, Cause: unknown.Cause})
	}
	data, present, err := createdAccountObject(payload)
	if err == nil && present {
		return data, nil
	}
	cause := err
	if cause == nil {
		cause = errors.New("账号创建成功但响应未返回稳定 ID")
	}
	if baselineErr != nil {
		return nil, &CommitUnknownError{Marker: marker, Cause: fmt.Errorf("%w；创建前目录基线不可用：%v", cause, baselineErr)}
	}
	return c.reconcileCreatedAccount(ctx, beforeIDs, body, marker, &CommitUnknownError{Marker: marker, Cause: cause})
}

// ReconcileAccountWithMarker is deliberately read-only. Callers use it after
// persisting an uncertain create result so a retry cannot issue a second POST.
func (c *Client) ReconcileAccountWithMarker(ctx context.Context, marker string) (map[string]any, bool, error) {
	marker = strings.TrimSpace(marker)
	if marker == "" || len(marker) > 256 || strings.ContainsAny(marker, "\r\n") {
		return nil, false, errors.New("账号创建 marker 无效")
	}
	accounts, err := c.Accounts(ctx)
	if err != nil {
		return nil, false, err
	}
	account, err := accountByMarker(accounts, marker)
	return account, account != nil, err
}

func (c *Client) reconcileCreatedAccount(ctx context.Context, beforeIDs map[string]struct{}, body map[string]any, marker string, unknown *CommitUnknownError) (map[string]any, error) {
	after, err := c.Accounts(ctx)
	if err != nil {
		unknown.Cause = fmt.Errorf("%w；marker 对账失败：%v", unknown.Cause, err)
		return nil, unknown
	}
	if marker != "" {
		account, markerErr := accountByMarker(after, marker)
		if markerErr != nil {
			unknown.Cause = fmt.Errorf("%w；%v", unknown.Cause, markerErr)
			return nil, unknown
		}
		if account != nil {
			return account, nil
		}
		unknown.Cause = fmt.Errorf("%w；账号 marker 尚未在管理目录中可见", unknown.Cause)
		return nil, unknown
	}
	matches := make([]map[string]any, 0, 1)
	for _, account := range after {
		accountID := strings.TrimSpace(fmt.Sprint(account["id"]))
		if !stableID(accountID) {
			continue
		}
		if _, existed := beforeIDs[accountID]; !existed && matchesCreatedAccount(account, body) {
			matches = append(matches, account)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		unknown.Cause = fmt.Errorf("%w；账号创建结果缺少稳定 ID，目录补读无法唯一确认新账号：返回多个候选账号", unknown.Cause)
	} else {
		unknown.Cause = fmt.Errorf("%w；账号创建结果缺少稳定 ID，目录补读无法唯一确认新账号", unknown.Cause)
	}
	return nil, unknown
}

func accountIDSet(accounts []map[string]any) (map[string]struct{}, error) {
	result := make(map[string]struct{}, len(accounts))
	for _, account := range accounts {
		accountID := strings.TrimSpace(fmt.Sprint(account["id"]))
		if !stableID(accountID) {
			return nil, &Error{"账号目录项目缺少稳定 ID"}
		}
		result[accountID] = struct{}{}
	}
	return result, nil
}

func accountByMarker(accounts []map[string]any, marker string) (map[string]any, error) {
	matches := make([]map[string]any, 0, 1)
	for _, account := range accounts {
		if markerLinePresent(fmt.Sprint(account["notes"]), marker) {
			if !stableID(strings.TrimSpace(fmt.Sprint(account["id"]))) {
				return nil, errors.New("账号创建 marker 对账结果缺少稳定 ID")
			}
			matches = append(matches, account)
		}
	}
	if len(matches) > 1 {
		return nil, errors.New("账号创建 marker 对账返回多个账号")
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return nil, nil
}

func markerLinePresent(notes, marker string) bool {
	notes = strings.ReplaceAll(notes, "\r\n", "\n")
	for _, line := range strings.Split(notes, "\n") {
		if strings.TrimSpace(line) == marker {
			return true
		}
	}
	return false
}

func createdAccountObject(payload map[string]any) (map[string]any, bool, error) {
	type candidate struct {
		value map[string]any
		depth int
	}
	queue := []candidate{{value: payload}}
	matches := map[string]map[string]any{}
	invalidID := false
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, key := range []string{"id", "account_id", "accountId"} {
			rawID, present := current.value[key]
			if !present {
				continue
			}
			accountID := strings.TrimSpace(fmt.Sprint(rawID))
			if stableID(accountID) {
				if key == "accountId" {
					current.value["id"] = rawID
				}
				matches[accountID] = current.value
			} else {
				invalidID = true
			}
			break
		}
		if current.depth >= 4 {
			continue
		}
		for _, key := range []string{"data", "account", "result", "item", "record", "created_account"} {
			rawNested, present := current.value[key]
			if !present {
				continue
			}
			if nested, ok := rawNested.(map[string]any); ok {
				queue = append(queue, candidate{value: nested, depth: current.depth + 1})
				continue
			}
			accountID := strings.TrimSpace(fmt.Sprint(rawNested))
			if stableID(accountID) {
				matches[accountID] = map[string]any{"id": rawNested}
			}
		}
	}
	if len(matches) > 1 {
		return nil, false, &Error{"账号创建返回包含多个不同的稳定 ID"}
	}
	for _, match := range matches {
		return match, true, nil
	}
	if invalidID {
		return nil, false, &Error{"账号创建返回包含无效稳定 ID"}
	}
	return nil, false, nil
}

func matchesCreatedAccount(account, requested map[string]any) bool {
	for _, field := range []string{"name", "platform", "type"} {
		expected := strings.TrimSpace(fmt.Sprint(requested[field]))
		actual := strings.TrimSpace(fmt.Sprint(account[field]))
		if expected == "" || actual != expected {
			return false
		}
	}
	expectedGroups, ok := stableIDValues(requested["group_ids"])
	if !ok {
		return false
	}
	actualGroups, ok := stableIDValues(account["group_ids"])
	if !ok || strings.Join(expectedGroups, ",") != strings.Join(actualGroups, ",") {
		return false
	}
	for _, field := range []string{"rate_multiplier", "notes"} {
		expected, requestedField := requested[field]
		actual, returnedField := account[field]
		if requestedField && returnedField && strings.TrimSpace(fmt.Sprint(actual)) != strings.TrimSpace(fmt.Sprint(expected)) {
			return false
		}
	}
	return true
}

func stableIDValues(raw any) ([]string, bool) {
	values := []string{}
	switch typed := raw.(type) {
	case []any:
		for _, value := range typed {
			values = append(values, strings.TrimSpace(fmt.Sprint(value)))
		}
	case []int64:
		for _, value := range typed {
			values = append(values, strconv.FormatInt(value, 10))
		}
	case []int:
		for _, value := range typed {
			values = append(values, strconv.Itoa(value))
		}
	default:
		return nil, false
	}
	for _, value := range values {
		if !stableID(value) {
			return nil, false
		}
	}
	sort.Slice(values, func(left, right int) bool {
		leftID, _ := strconv.ParseInt(values[left], 10, 64)
		rightID, _ := strconv.ParseInt(values[right], 10, 64)
		return leftID < rightID
	})
	return values, true
}

func (c *Client) DeleteAccount(ctx context.Context, accountID string) (map[string]any, error) {
	return c.DeleteAccountWithVerification(ctx, accountID, true)
}

func (c *Client) DeleteAccountWithVerification(ctx context.Context, accountID string, verification bool) (map[string]any, error) {
	if !stableID(accountID) {
		return nil, errors.New("账号 ID 必须是稳定正整数")
	}
	payload, deleteErr := c.Mutate(ctx, http.MethodDelete, "/admin/accounts/"+accountID, nil)
	if !verification {
		var deleteHTTPError *HTTPError
		if deleteErr != nil && (!errors.As(deleteErr, &deleteHTTPError) || deleteHTTPError.StatusCode != http.StatusNotFound) {
			return nil, deleteErr
		}
		if payload == nil {
			payload = map[string]any{}
		}
		payload["account_id"] = accountID
		payload["deleted"] = true
		payload["confirmed_absent"] = false
		payload["delete_response_confirmed"] = deleteErr == nil
		return payload, nil
	}
	_, readbackErr := c.Account(ctx, accountID)
	var readbackHTTPError *HTTPError
	if errors.As(readbackErr, &readbackHTTPError) && readbackHTTPError.StatusCode == http.StatusNotFound {
		return map[string]any{
			"account_id": accountID, "deleted": true, "confirmed_absent": true,
			"delete_response_confirmed": deleteErr == nil,
		}, nil
	}
	if readbackErr != nil {
		return nil, &AccountReadbackUnknownError{
			AccountID: accountID, DeleteErr: deleteErr, ReadbackErr: readbackErr,
		}
	}
	return nil, &AccountStillReadableError{AccountID: accountID, DeleteErr: deleteErr}
}

func (c *Client) OpenAccountTest(ctx context.Context, accountID string, body map[string]any) (*http.Response, error) {
	if !stableID(accountID) {
		return nil, errors.New("账号 ID 必须是稳定数字 ID")
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/admin/accounts/"+accountID+"/test", bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	c.headers(request)
	return c.http.Do(request)
}

func (c *Client) fetchPaged(ctx context.Context, path, label string) ([]map[string]any, error) {
	result := []map[string]any{}
	seen := map[string]struct{}{}
	var declaredTotal *int
	for page := 1; page <= 10000; page++ {
		payload, err := c.request(ctx, http.MethodGet, path, nil, map[string]string{"page": strconv.Itoa(page), "page_size": "1000"})
		if err != nil {
			return nil, err
		}
		pageItems, metadata, err := items(payload, label)
		if err != nil {
			return nil, err
		}
		for _, item := range pageItems {
			id := strings.TrimSpace(fmt.Sprint(item["id"]))
			if id == "" {
				return nil, &Error{label + "项目缺少稳定 ID"}
			}
			if _, ok := seen[id]; ok {
				return nil, &Error{label + "返回重复 ID"}
			}
			seen[id] = struct{}{}
			result = append(result, item)
		}
		total, present, err := pageTotal(metadata, label)
		if err != nil {
			return nil, err
		}
		if present {
			if declaredTotal != nil && *declaredTotal != total {
				return nil, &Error{fmt.Sprintf("%s 分页总数在读取期间发生变化", label)}
			}
			declaredTotal = &total
		}
		if declaredTotal != nil {
			if len(result) == *declaredTotal {
				return result, nil
			}
			if len(result) > *declaredTotal {
				return nil, &Error{fmt.Sprintf("%s 分页返回项目数超过声明总数", label)}
			}
		}
		if len(pageItems) == 0 {
			if declaredTotal != nil {
				return nil, &Error{fmt.Sprintf("%s 分页未完整：已读取 %d/%d 项", label, len(result), *declaredTotal)}
			}
			return result, nil
		}
	}
	return nil, &Error{label + "分页超过安全上限"}
}

func (c *Client) fetchEvidence(ctx context.Context, path, label string, params map[string]string, limit int, idField string, pageSize int) ([]map[string]any, error) {
	result := []map[string]any{}
	seen := map[string]struct{}{}
	for page := 1; page <= 10000; page++ {
		query := map[string]string{}
		for k, v := range params {
			query[k] = v
		}
		query["page"] = strconv.Itoa(page)
		size := limit
		if size > pageSize {
			size = pageSize
		}
		query["page_size"] = strconv.Itoa(size)
		payload, err := c.request(ctx, http.MethodGet, path, nil, query)
		if err != nil {
			return nil, err
		}
		pageItems, metadata, err := items(payload, label)
		if err != nil {
			return nil, err
		}
		for _, item := range pageItems {
			id := strings.TrimSpace(fmt.Sprint(item[idField]))
			if idField == "request_id" {
				result = append(result, item)
			} else {
				if id == "" {
					return nil, &Error{label + "项目缺少稳定 ID"}
				}
				if _, ok := seen[id]; ok {
					return nil, &Error{label + "返回重复 " + idField}
				}
				seen[id] = struct{}{}
				result = append(result, item)
			}
			if len(result) >= limit {
				return result, nil
			}
		}
		total, present, err := pageTotal(metadata, label)
		if err != nil {
			return nil, err
		}
		if present && len(result) >= total {
			return result, nil
		}
		if len(pageItems) == 0 {
			return result, nil
		}
	}
	return nil, &Error{label + "分页超过安全上限"}
}

func (c *Client) request(ctx context.Context, method, path string, body map[string]any, query map[string]string) (map[string]any, error) {
	return c.requestWithSemantics(ctx, method, path, body, query, false)
}

func (c *Client) requestWithSemantics(ctx context.Context, method, path string, body map[string]any, query map[string]string, nonIdempotentCreate bool) (map[string]any, error) {
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	attempts := c.attempts
	if nonIdempotentCreate {
		attempts = 1
	}
	var last error
	for attempt := 0; attempt < attempts; attempt++ {
		endpoint, err := url.Parse(c.baseURL + "/api/v1" + path)
		if err != nil {
			return nil, err
		}
		values := endpoint.Query()
		for key, value := range query {
			values.Set(key, value)
		}
		endpoint.RawQuery = values.Encode()
		request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bytes.NewReader(encoded))
		if err != nil {
			return nil, err
		}
		c.headers(request)
		response, err := c.http.Do(request)
		if err != nil {
			if nonIdempotentCreate {
				cause := fmt.Errorf("管理 API 请求传输中断：%T", err)
				if ctx.Err() != nil {
					cause = fmt.Errorf("管理 API 请求发送后上下文结束：%w", ctx.Err())
				}
				return nil, &CommitUnknownError{Cause: cause}
			}
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			last = err
		} else {
			payload, responseErr, retry := decodeResponse(response)
			if responseErr == nil {
				return payload, nil
			}
			var unknown *responseOutcomeUnknown
			if nonIdempotentCreate && errors.As(responseErr, &unknown) {
				return nil, &CommitUnknownError{Cause: unknown.cause}
			}
			var httpError *HTTPError
			if nonIdempotentCreate && errors.As(responseErr, &httpError) && (httpError.StatusCode == http.StatusRequestTimeout || httpError.StatusCode >= 500) {
				return nil, &CommitUnknownError{Cause: responseErr}
			}
			last = responseErr
			if !retry {
				return nil, responseErr
			}
		}
		if attempt+1 < attempts {
			timer := time.NewTimer(time.Duration(1<<attempt) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return nil, &Error{"管理 API 请求失败：" + path + "：" + last.Error()}
}

func (c *Client) headers(request *http.Request) {
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-API-Key", c.adminKey)
	request.Header.Set("User-Agent", "Sub2API-Console/1.0")
}

func decodeResponse(response *http.Response) (map[string]any, error, bool) {
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil {
		return nil, &responseOutcomeUnknown{cause: err}, true
	}
	if len(raw) > maximumResponseBytes {
		return nil, &responseOutcomeUnknown{cause: &Error{"管理 API 响应超过 4 MiB 安全上限"}}, false
	}
	detail := errorDetail(raw)
	if (response.StatusCode == 404 || response.StatusCode == 503) && containsMonitoringDisabled(detail) {
		return nil, &MonitoringDisabled{}, false
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		httpError := &HTTPError{response.StatusCode, detail}
		retry := response.StatusCode == 408 || response.StatusCode == 425 || response.StatusCode == 429 || response.StatusCode >= 500
		return nil, httpError, retry
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return nil, &responseOutcomeUnknown{cause: &Error{"管理 API 返回不是 JSON"}}, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, &responseOutcomeUnknown{cause: &Error{"管理 API 返回包含 JSON 尾随数据"}}, false
	}
	if payload == nil {
		return nil, &responseOutcomeUnknown{cause: &Error{"管理 API 返回格式不可读"}}, false
	}
	if !businessSuccess(payload) {
		return nil, &Error{"管理 API 返回业务失败：" + payloadError(payload)}, false
	}
	return payload, nil, false
}

func items(payload map[string]any, label string) ([]map[string]any, map[string]any, error) {
	rawData, present := payload["data"]
	var rawItems any
	var metadata map[string]any
	if data, ok := rawData.(map[string]any); ok {
		var itemsPresent bool
		rawItems, itemsPresent = data["items"]
		if _, valid := rawItems.([]any); !itemsPresent || !valid {
			return nil, nil, &Error{label + " data 字段无有效 items（缺少 items）"}
		}
		metadata = data
	} else if list, ok := rawData.([]any); ok {
		rawItems = list
		metadata = map[string]any{"items": list}
	} else if !present {
		rawItems = payload["items"]
		metadata = payload
	} else {
		return nil, nil, &Error{label + " data 字段无有效 items（缺少 items）"}
	}
	list, ok := rawItems.([]any)
	if !ok {
		return nil, nil, &Error{label + " 返回缺少 items"}
	}
	result := make([]map[string]any, 0, len(list))
	for _, raw := range list {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, nil, &Error{label + " 返回包含无效项目"}
		}
		result = append(result, item)
	}
	return result, metadata, nil
}

func uniqueStableItems(values []map[string]any, label string) ([]map[string]any, error) {
	seen := map[string]struct{}{}
	for _, item := range values {
		id := strings.TrimSpace(fmt.Sprint(item["id"]))
		if id == "" {
			return nil, &Error{label + "项目缺少稳定 ID"}
		}
		if _, ok := seen[id]; ok {
			return nil, &Error{label + "返回重复 ID"}
		}
		seen[id] = struct{}{}
	}
	return values, nil
}

func pageTotal(metadata map[string]any, label string) (int, bool, error) {
	raw, present := metadata["total"]
	if !present {
		return 0, false, nil
	}
	if raw == nil {
		return 0, true, &Error{label + " total 必须是非负整数"}
	}
	text := strings.TrimSpace(fmt.Sprint(raw))
	parsed, err := strconv.Atoi(text)
	if err != nil || parsed < 0 || strconv.Itoa(parsed) != text {
		return 0, true, &Error{label + " total 必须是非负整数"}
	}
	return parsed, true, nil
}

func businessSuccess(payload map[string]any) bool {
	if raw, present := payload["code"]; present {
		if raw != true && fmt.Sprint(raw) != "0" && fmt.Sprint(raw) != "200" {
			return false
		}
	}
	for _, key := range []string{"success", "ok"} {
		if raw, present := payload[key]; present {
			normalized := strings.ToLower(strings.TrimSpace(fmt.Sprint(raw)))
			if raw != true && normalized != "1" && normalized != "200" && normalized != "true" && normalized != "success" && normalized != "succeeded" && normalized != "ok" {
				return false
			}
		}
	}
	for _, key := range []string{"error", "errors"} {
		if raw, present := payload[key]; present && raw != nil && fmt.Sprint(raw) != "" && fmt.Sprint(raw) != "[]" && fmt.Sprint(raw) != "map[]" {
			return false
		}
	}
	status := strings.ToLower(strings.TrimSpace(fmt.Sprint(payload["status"])))
	return status != "error" && status != "failed" && status != "failure"
}

func errorDetail(raw []byte) string {
	var payload map[string]any
	if json.Unmarshal(raw, &payload) == nil {
		for _, key := range []string{"message", "error", "detail"} {
			if value, present := payload[key]; present {
				if value == nil {
					return "空值"
				}
				return truncate(redact.Secrets(fmt.Sprint(value)), 300)
			}
		}
	}
	text := strings.ReplaceAll(strings.TrimSpace(string(raw)), "\n", " ")
	if text == "" {
		return "未提供错误详情"
	}
	return truncate(redact.Secrets(text), 300)
}
func payloadError(payload map[string]any) string {
	for _, key := range []string{"message", "error"} {
		if value, present := payload[key]; present {
			if value == nil {
				return "空值"
			}
			return truncate(redact.Secrets(fmt.Sprint(value)), 300)
		}
	}
	return "未提供错误详情"
}
func containsMonitoringDisabled(value string) bool {
	value = strings.ToLower(value)
	for _, marker := range []string{"ops_disabled", "monitoring is disabled", "ops service not available", "未开启"} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}
func stableID(value string) bool {
	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	return err == nil && parsed > 0 && strings.TrimSpace(value) == value
}
func truncate(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}
