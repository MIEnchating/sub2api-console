package adminclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/redact"
)

type Error struct{ Message string }

func (e *Error) Error() string { return e.Message }

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

func (c *Client) AccountModels(ctx context.Context, accountID string) ([]string, error) {
	return c.accountModelIDs(ctx, http.MethodGet, accountID, "/models")
}

func (c *Client) SyncAccountModels(ctx context.Context, accountID string) ([]string, error) {
	return c.accountModelIDs(ctx, http.MethodPost, accountID, "/models/sync-upstream")
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
	return c.CreateAccountWithVerification(ctx, body, false)
}

func (c *Client) CreateAccountWithVerification(ctx context.Context, body map[string]any, verification bool) (map[string]any, error) {
	var beforeIDs map[string]struct{}
	var baselineErr error
	if verification {
		beforeIDs, baselineErr = c.accountIDs(ctx)
	}
	payload, err := c.Mutate(ctx, http.MethodPost, "/admin/accounts", body)
	if err != nil {
		return nil, err
	}
	data, present, err := createdAccountObject(payload)
	if err != nil {
		return nil, err
	}
	if present {
		return data, nil
	}
	if !verification {
		return nil, &Error{"账号创建成功但响应未返回稳定 ID；写后确认关闭，无法安全定位新账号"}
	}
	if baselineErr != nil {
		return nil, &Error{"账号创建成功但响应未返回稳定 ID，且创建前目录读取失败：" + baselineErr.Error()}
	}
	after, err := c.Accounts(ctx)
	if err != nil {
		return nil, &Error{"账号创建成功但响应未返回稳定 ID，目录补读失败：" + err.Error()}
	}
	matches := make([]map[string]any, 0, 1)
	for _, account := range after {
		accountID := strings.TrimSpace(fmt.Sprint(account["id"]))
		if !stableID(accountID) {
			continue
		}
		if _, existed := beforeIDs[accountID]; existed || !matchesCreatedAccount(account, body) {
			continue
		}
		matches = append(matches, account)
	}
	if len(matches) != 1 {
		return nil, &Error{"账号创建结果缺少稳定 ID，目录补读无法唯一确认新账号"}
	}
	return matches[0], nil
}

func (c *Client) accountIDs(ctx context.Context) (map[string]struct{}, error) {
	accounts, err := c.Accounts(ctx)
	if err != nil {
		return nil, err
	}
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
	sort.Strings(values)
	return values, true
}

func (c *Client) DeleteAccount(ctx context.Context, accountID string) (map[string]any, error) {
	return c.DeleteAccountWithVerification(ctx, accountID, true)
}

func (c *Client) DeleteAccountWithVerification(ctx context.Context, accountID string, verification bool) (map[string]any, error) {
	if !stableID(accountID) {
		return nil, errors.New("账号 ID 必须是稳定正整数")
	}
	payload, err := c.Mutate(ctx, http.MethodDelete, "/admin/accounts/"+accountID, nil)
	var httpError *HTTPError
	if err != nil && (!errors.As(err, &httpError) || httpError.StatusCode != http.StatusNotFound) {
		return nil, err
	}
	if !verification {
		if payload == nil {
			payload = map[string]any{}
		}
		payload["account_id"] = accountID
		payload["deleted"] = true
		payload["confirmed_absent"] = false
		return payload, nil
	}
	if err == nil {
		_, err = c.Account(ctx, accountID)
	}
	if errors.As(err, &httpError) && httpError.StatusCode == http.StatusNotFound {
		return map[string]any{"account_id": accountID, "deleted": true, "confirmed_absent": true}, nil
	}
	if err != nil {
		return nil, err
	}
	return nil, &Error{fmt.Sprintf("账号 %s 删除后仍可读，已停止本地清理", accountID)}
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
		if present && len(result) >= total {
			return result, nil
		}
		if len(pageItems) == 0 {
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
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	var last error
	for attempt := 0; attempt < c.attempts; attempt++ {
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
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			last = err
		} else {
			payload, responseErr, retry := decodeResponse(response)
			if responseErr == nil {
				return payload, nil
			}
			last = responseErr
			if !retry {
				return nil, responseErr
			}
		}
		if attempt+1 < c.attempts {
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
	raw, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return nil, err, true
	}
	detail := errorDetail(raw)
	if (response.StatusCode == 404 || response.StatusCode == 503) && containsMonitoringDisabled(detail) {
		return nil, &MonitoringDisabled{}, false
	}
	if response.StatusCode >= 400 {
		httpError := &HTTPError{response.StatusCode, detail}
		retry := response.StatusCode == 408 || response.StatusCode == 425 || response.StatusCode == 429 || response.StatusCode >= 500
		return nil, httpError, retry
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return nil, &Error{"管理 API 返回不是 JSON"}, false
	}
	if payload == nil {
		return nil, &Error{"管理 API 返回格式不可读"}, false
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
		metadata = map[string]any{"items": list, "total": int64(len(list))}
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
