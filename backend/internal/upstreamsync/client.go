package upstreamsync

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
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/redact"
)

const (
	maximumResponseBytes = 1 << 20
	pageSize             = 1000
	maximumPages         = 10000
)

type StatusError struct {
	StatusCode int
	Path       string
	Detail     string
}

func (e *StatusError) Error() string {
	message := fmt.Sprintf("上游请求失败（HTTP %d，%s）", e.StatusCode, e.Path)
	if e.Detail != "" {
		message += "：" + e.Detail
	}
	return message
}

func IsAuthenticationError(err error) bool {
	var status *StatusError
	return errors.As(err, &status) && (status.StatusCode == http.StatusUnauthorized || status.StatusCode == http.StatusForbidden)
}

type Reader struct {
	http      *http.Client
	pathMu    sync.RWMutex
	pathCache map[string]string
}

type CreatedKey struct {
	KeyID   string
	Name    string
	GroupID string
	Secret  string
}

func NewReader(client *http.Client) *Reader {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	copy := *client
	if copy.Timeout == 0 {
		copy.Timeout = 20 * time.Second
	}
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Reader{http: &copy, pathCache: map[string]string{}}
}

func (r *Reader) ReadCatalog(ctx context.Context, record configstore.AuthRecord) (business.UpstreamCatalogSnapshot, error) {
	groupPaths := []string{"/api/v1/groups/available", "/api/v1/admin/groups"}
	if isNewAPI(record.UpstreamType) {
		groupPaths = []string{"/api/user/self/groups", "/api/user/groups"}
	}
	payload, err := r.getFallback(ctx, record, groupPaths, nil)
	if err != nil {
		return business.UpstreamCatalogSnapshot{}, err
	}
	rows, err := catalogItems(payload)
	if err != nil {
		return business.UpstreamCatalogSnapshot{}, err
	}
	if len(rows) == 0 {
		return business.UpstreamCatalogSnapshot{}, errors.New("上游分组目录返回为空或格式不可读")
	}
	groups := make([]business.UpstreamCatalogGroup, 0, len(rows))
	for _, row := range rows {
		if isNewAPI(record.UpstreamType) && virtualAutoGroup(row) {
			continue
		}
		id := textValue(firstPresent(row, "group_id", "groupId", "id"))
		name := textValue(firstPresent(row, "name", "group", "group_name", "groupName"))
		if name == "" {
			name = id
		}
		status := statusText(row)
		// NewAPI's ratio is already resolved for the authenticated user: a
		// user-group override wins over scheduled and base ratios. base_ratio is
		// intentionally not a fallback because it can silently bypass that override.
		rate, err := optionalDecimal(row, "effective_rate", "actual_rate", "custom_rate", "rate_multiplier", "group_ratio", "groupRatio", "ratio", "rate", "multiplier")
		if err != nil {
			return business.UpstreamCatalogSnapshot{}, fmt.Errorf("上游分组 %s 倍率不可读：%w", id, err)
		}
		groups = append(groups, business.UpstreamCatalogGroup{
			GroupID: id, Name: name, Description: optionalText(firstPresent(row, "description", "desc", "介绍", "group_description")),
			Platform: optionalText(firstPresent(row, "platform", "provider", "type")), Status: status, RawRate: rate,
		})
	}
	keys, err := r.readKeys(ctx, record)
	if err != nil {
		return business.UpstreamCatalogSnapshot{}, err
	}
	return business.UpstreamCatalogSnapshot{Groups: groups, Keys: keys}, nil
}

func (r *Reader) ReadBalance(ctx context.Context, record configstore.AuthRecord) (business.UpstreamBalanceObservation, error) {
	paths := []string{"/api/v1/user/profile", "/api/user/self"}
	publicPath := "/api/v1/settings/public"
	if isNewAPI(record.UpstreamType) {
		paths, publicPath = []string{"/api/user/self", "/api/v1/user/profile"}, "/api/status"
	}
	payload, err := r.getFallback(ctx, record, paths, nil)
	if err != nil {
		return business.UpstreamBalanceObservation{}, err
	}
	data, err := payloadObject(payload, "上游余额")
	if err != nil {
		return business.UpstreamBalanceObservation{}, err
	}
	raw, present := presentValue(data, "balance", "remain_quota", "quota", "remaining")
	var balance *string
	if present && raw != nil && textValue(raw) != "" {
		value, err := decimalText(raw)
		if err != nil {
			return business.UpstreamBalanceObservation{}, errors.New("上游返回的余额不是有限数值")
		}
		balance = &value
	}
	var siteName, quotaPerUnit, balanceUnit *string
	if public, publicErr := r.getPublic(ctx, record.BaseURL, publicPath); publicErr == nil {
		if publicData, objectErr := payloadObject(public, "上游公开配置"); objectErr == nil {
			siteName = optionalText(firstPresent(publicData, "system_name", "site_name", "name"))
			if isNewAPI(record.UpstreamType) {
				if quota, quotaErr := optionalDecimal(publicData, "quota_per_unit"); quotaErr == nil {
					quotaPerUnit = quota
				}
			}
		}
	}
	if isNewAPI(record.UpstreamType) && balance != nil {
		divisor := "500000"
		if quotaPerUnit != nil {
			if positiveDecimal(*quotaPerUnit) {
				divisor = *quotaPerUnit
			}
		}
		converted, err := divideDecimal(*balance, divisor)
		if err != nil {
			return business.UpstreamBalanceObservation{}, errors.New("New API 余额换算失败")
		}
		balance, quotaPerUnit = &converted, &divisor
		unit := "usd"
		balanceUnit = &unit
	}
	hardRaw, hardPresent := presentValue(data, "balance_hard_closed", "hard_closed", "closed")
	var hardClosed *bool
	if hardPresent && hardRaw != nil {
		parsed, ok := strictBool(hardRaw)
		if !ok {
			return business.UpstreamBalanceObservation{}, errors.New("上游余额关闭状态不可读")
		}
		hardClosed = &parsed
	}
	status := "未返回余额"
	if balance != nil {
		status = "已读取"
	}
	return business.UpstreamBalanceObservation{
		RawBalance: balance, Status: status, HardClosed: hardClosed, HardClosedPresent: hardPresent,
		SiteName: siteName, QuotaPerUnit: quotaPerUnit, BalanceUnit: balanceUnit,
	}, nil
}

func (r *Reader) ReadSiteName(ctx context.Context, record configstore.AuthRecord) (*string, error) {
	publicPath := "/api/v1/settings/public"
	if isNewAPI(record.UpstreamType) {
		publicPath = "/api/status"
	}
	payload, err := r.getPublic(ctx, record.BaseURL, publicPath)
	if err != nil {
		return nil, err
	}
	data, err := payloadObject(payload, "上游公开配置")
	if err != nil {
		return nil, err
	}
	return optionalText(firstPresent(data, "system_name", "site_name", "name")), nil
}

func (r *Reader) CreateKey(ctx context.Context, record configstore.AuthRecord, name, groupID string) (CreatedKey, error) {
	return r.CreateKeyWithVerification(ctx, record, name, groupID, false)
}

func (r *Reader) CreateKeyWithVerification(ctx context.Context, record configstore.AuthRecord, name, groupID string, verification bool) (CreatedKey, error) {
	name, groupID = strings.TrimSpace(name), strings.TrimSpace(groupID)
	if name == "" || groupID == "" {
		return CreatedKey{}, errors.New("上游 Key 创建必须包含名称和稳定分组 ID")
	}
	beforeIDs := map[string]struct{}{}
	if verification {
		before, err := r.readKeys(ctx, record)
		if err != nil {
			return CreatedKey{}, err
		}
		beforeIDs = make(map[string]struct{}, len(before))
		for _, key := range before {
			beforeIDs[key.KeyID] = struct{}{}
		}
	}
	paths := []string{"/api/v1/keys"}
	body := map[string]any{"name": name}
	if isNewAPI(record.UpstreamType) {
		paths = []string{"/api/token/"}
		body = map[string]any{
			"name": name, "group": groupID, "expired_time": -1, "remain_quota": 0,
			"unlimited_quota": true, "model_limits_enabled": false, "model_limits": "", "cross_group_retry": false,
		}
	} else {
		numericGroupID, parseErr := strconv.ParseInt(groupID, 10, 64)
		if parseErr != nil || numericGroupID <= 0 {
			return CreatedKey{}, errors.New("Sub2API 上游分组必须使用数字稳定 ID")
		}
		body["group_id"] = numericGroupID
	}
	payload, err := r.postFallback(ctx, record, paths, body)
	if err != nil {
		return CreatedKey{}, err
	}
	created, err := payloadObject(payload, "上游 Key 创建")
	if err != nil {
		return CreatedKey{}, err
	}
	keyID := textValue(firstPresent(created, "id", "key_id", "token_id"))
	verifiedName := name
	if returnedGroup := textValue(firstPresent(created, "group_id", "groupId", "group")); returnedGroup != "" && returnedGroup != groupID {
		return CreatedKey{}, errors.New("上游 Key 创建响应的分组与请求不一致")
	}
	if returnedName := strings.TrimSpace(textValue(firstPresent(created, "name", "key_name"))); returnedName != "" {
		verifiedName = returnedName
	}
	lookupMissingID := keyID == ""
	if verification || lookupMissingID {
		after, err := r.readKeys(ctx, record)
		if err != nil {
			if lookupMissingID && !verification {
				return CreatedKey{}, fmt.Errorf("上游 Key 已创建但响应未返回稳定 ID，目录补读失败：%w", err)
			}
			return CreatedKey{}, err
		}
		if lookupMissingID {
			matches := make([]business.UpstreamCatalogKey, 0, 1)
			for _, key := range after {
				_, existedBeforeWrite := beforeIDs[key.KeyID]
				if (!verification || !existedBeforeWrite) && key.Name == name && key.UpstreamGroup != nil && *key.UpstreamGroup == groupID {
					matches = append(matches, key)
				}
			}
			if len(matches) != 1 {
				if verification {
					return CreatedKey{}, errors.New("上游 Key 创建结果缺少稳定 ID，读回无法唯一确认")
				}
				return CreatedKey{}, errors.New("上游 Key 已创建但响应未返回稳定 ID，目录补读无法唯一定位新 Key")
			}
			keyID = matches[0].KeyID
			if strings.TrimSpace(matches[0].Name) != "" {
				verifiedName = strings.TrimSpace(matches[0].Name)
			}
		}
		if verification {
			var verified *business.UpstreamCatalogKey
			for index := range after {
				if after[index].KeyID == keyID {
					verified = &after[index]
					break
				}
			}
			if verified == nil || verified.UpstreamGroup == nil || *verified.UpstreamGroup != groupID {
				return CreatedKey{}, errors.New("上游 Key 创建后稳定 ID 或分组读回不一致")
			}
			if strings.TrimSpace(verified.Name) != "" {
				verifiedName = strings.TrimSpace(verified.Name)
			}
		}
	}
	secret := textValue(firstPresent(created, "key", "api_key", "token"))
	if isNewAPI(record.UpstreamType) {
		revealed, revealErr := r.postFallback(ctx, record, []string{"/api/token/" + url.PathEscape(keyID) + "/key"}, map[string]any{})
		if revealErr != nil {
			return CreatedKey{}, revealErr
		}
		data, dataErr := payloadObject(revealed, "NewAPI Key 密钥")
		if dataErr != nil {
			return CreatedKey{}, dataErr
		}
		secret = textValue(firstPresent(data, "key", "token"))
	}
	if secret == "" {
		return CreatedKey{}, errors.New("上游 Key 创建成功但一次性密钥读取为空")
	}
	return CreatedKey{KeyID: keyID, Name: verifiedName, GroupID: groupID, Secret: secret}, nil
}

func (r *Reader) RevealKey(ctx context.Context, record configstore.AuthRecord, keyID, groupID string) (CreatedKey, error) {
	keyID, groupID = strings.TrimSpace(keyID), strings.TrimSpace(groupID)
	if keyID == "" || groupID == "" {
		return CreatedKey{}, errors.New("待续开户记录缺少稳定 Key/分组 ID")
	}
	rows, err := r.readRawKeys(ctx, record)
	if err != nil {
		return CreatedKey{}, err
	}
	matches := make([]map[string]any, 0, 1)
	for _, row := range rows {
		if textValue(firstPresent(row, "id", "key_id", "token_id")) == keyID {
			matches = append(matches, row)
		}
	}
	if len(matches) != 1 {
		return CreatedKey{}, errors.New("待续开户的上游 Key 无法唯一读回")
	}
	row := matches[0]
	returnedGroup := textValue(firstPresent(row, "group_id", "groupId", "upstream_group", "group"))
	if returnedGroup != groupID {
		return CreatedKey{}, errors.New("待续开户的上游 Key 分组读回不一致")
	}
	secret := textValue(firstPresent(row, "key", "api_key"))
	if isNewAPI(record.UpstreamType) {
		payload, revealErr := r.postFallback(ctx, record, []string{"/api/token/" + url.PathEscape(keyID) + "/key"}, map[string]any{})
		if revealErr != nil {
			return CreatedKey{}, revealErr
		}
		data, dataErr := payloadObject(payload, "NewAPI Key 密钥")
		if dataErr != nil {
			return CreatedKey{}, dataErr
		}
		secret = textValue(firstPresent(data, "key", "token"))
	}
	if secret == "" {
		return CreatedKey{}, errors.New("待续开户的上游 Key 密钥不可读")
	}
	name := textValue(firstPresent(row, "name", "key_name"))
	if name == "" {
		name = "key-" + keyID
	}
	return CreatedKey{KeyID: keyID, Name: name, GroupID: groupID, Secret: secret}, nil
}

func (r *Reader) DeleteKey(ctx context.Context, record configstore.AuthRecord, keyID string) error {
	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return errors.New("上游 Key 删除必须包含稳定 ID")
	}
	paths := []string{"/api/v1/keys/" + url.PathEscape(keyID), "/api/token/" + url.PathEscape(keyID)}
	if isNewAPI(record.UpstreamType) {
		paths[0], paths[1] = paths[1], paths[0]
	}
	return r.deleteFallback(ctx, record, paths)
}

func (r *Reader) readKeys(ctx context.Context, record configstore.AuthRecord) ([]business.UpstreamCatalogKey, error) {
	rows, err := r.readRawKeys(ctx, record)
	if err != nil {
		return nil, err
	}
	result := make([]business.UpstreamCatalogKey, 0, len(rows))
	for _, row := range rows {
		id := textValue(firstPresent(row, "id", "key_id", "token_id"))
		group := firstPresent(row, "group_id", "groupId", "upstream_group", "group")
		if object, ok := group.(map[string]any); ok {
			group = firstPresent(object, "id", "group_id", "groupId", "name")
		}
		rate, err := optionalDecimal(row, "effective_rate", "actual_rate", "custom_rate", "override_rate", "key_rate", "raw_rate", "rate", "group_rate", "multiplier")
		if err != nil {
			return nil, fmt.Errorf("上游 Key %s 倍率不可读：%w", id, err)
		}
		name := textValue(firstPresent(row, "name", "key_name"))
		if name == "" {
			name = id
		}
		result = append(result, business.UpstreamCatalogKey{
			KeyID: id, Name: name, UpstreamGroup: optionalText(group), RateAmbiguous: newAPIMultiGroupToken(record.UpstreamType, row),
			Status: statusText(row), Rate: rate,
		})
	}
	return result, nil
}

func newAPIMultiGroupToken(upstreamType string, row map[string]any) bool {
	if !isNewAPI(upstreamType) {
		return false
	}
	value := firstPresent(row, "group_route_config", "groupRouteConfig")
	switch raw := value.(type) {
	case nil:
		return false
	case []any:
		return len(raw) > 0
	case map[string]any:
		return len(raw) > 0
	default:
		text := strings.TrimSpace(textValue(raw))
		return text != "" && text != "[]" && text != "{}" && text != "null"
	}
}

func (r *Reader) readRawKeys(ctx context.Context, record configstore.AuthRecord) ([]map[string]any, error) {
	paths := []string{"/api/v1/keys", "/api/token/"}
	if isNewAPI(record.UpstreamType) {
		paths = []string{"/api/token/", "/api/v1/keys"}
	}
	result := []map[string]any{}
	seen := map[string]struct{}{}
	for page := 1; page <= maximumPages; page++ {
		query := url.Values{"page": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(pageSize)}}
		payload, err := r.getFallback(ctx, record, paths, query)
		if err != nil {
			var status *StatusError
			if errors.As(err, &status) && status.StatusCode == http.StatusNotFound {
				return nil, fmt.Errorf("未找到上游 Key 列表接口，请检查平台类型或上游版本（HTTP %d，%s）", status.StatusCode, status.Path)
			}
			return nil, err
		}
		rows, err := keyItems(payload)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			break
		}
		newIDs := 0
		for _, row := range rows {
			id := textValue(firstPresent(row, "id", "key_id", "token_id"))
			if id == "" {
				return nil, errors.New("上游 Key 目录包含缺少稳定 ID 的项目")
			}
			if _, duplicate := seen[id]; duplicate {
				continue
			}
			seen[id] = struct{}{}
			newIDs++
			result = append(result, cloneObject(row))
		}
		if page > 1 && newIDs == 0 {
			break
		}
		total, totalPresent, err := pageTotal(payload)
		if err != nil {
			return nil, err
		}
		if totalPresent && page*pageSize >= total {
			break
		}
	}
	return result, nil
}

func (r *Reader) getFallback(ctx context.Context, record configstore.AuthRecord, paths []string, query url.Values) (any, error) {
	var last error
	cacheKey := r.pathCacheKey(http.MethodGet, record, paths)
	ordered := r.cachedPaths(cacheKey, paths)
	for index, path := range ordered {
		payload, status, err := r.request(ctx, record, path, query, true)
		if err == nil {
			r.rememberPath(cacheKey, path)
			return payload, nil
		}
		last = err
		if status == http.StatusNotFound {
			r.forgetPath(cacheKey, path)
		}
		if status == http.StatusNotFound && index < len(ordered)-1 {
			continue
		}
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			return nil, err
		}
	}
	if last == nil {
		last = errors.New("没有可用的上游请求地址")
	}
	return nil, last
}

func (r *Reader) getPublic(ctx context.Context, baseURL, path string) (any, error) {
	record := configstore.AuthRecord{BaseURL: baseURL, Headers: map[string]string{}, Cookies: map[string]string{}}
	payload, _, err := r.request(ctx, record, path, nil, false)
	return payload, err
}

func (r *Reader) postFallback(ctx context.Context, record configstore.AuthRecord, paths []string, body map[string]any) (any, error) {
	var last error
	cacheKey := r.pathCacheKey(http.MethodPost, record, paths)
	ordered := r.cachedPaths(cacheKey, paths)
	for index, path := range ordered {
		payload, status, err := r.requestJSON(ctx, record, http.MethodPost, path, nil, body, true)
		if err == nil {
			r.rememberPath(cacheKey, path)
			return payload, nil
		}
		last = err
		if status == http.StatusNotFound {
			r.forgetPath(cacheKey, path)
		}
		if status == http.StatusNotFound && index < len(ordered)-1 {
			continue
		}
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			return nil, err
		}
	}
	if last == nil {
		last = errors.New("没有可用的上游写入地址")
	}
	return nil, last
}

func (r *Reader) deleteFallback(ctx context.Context, record configstore.AuthRecord, paths []string) error {
	var last error
	cacheKey := r.pathCacheKey(http.MethodDelete, record, paths)
	ordered := r.cachedPaths(cacheKey, paths)
	for index, path := range ordered {
		_, status, err := r.requestJSON(ctx, record, http.MethodDelete, path, nil, nil, true)
		if err == nil {
			r.rememberPath(cacheKey, path)
			return nil
		}
		last = err
		if status == http.StatusNotFound {
			r.forgetPath(cacheKey, path)
		}
		if status == http.StatusNotFound && index < len(ordered)-1 {
			continue
		}
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			return err
		}
	}
	if last == nil {
		last = errors.New("没有可用的上游删除地址")
	}
	return last
}

func (r *Reader) pathCacheKey(method string, record configstore.AuthRecord, paths []string) string {
	if len(paths) < 2 {
		return ""
	}
	return method + "\x00" + strings.TrimRight(strings.ToLower(strings.TrimSpace(record.BaseURL)), "/") + "\x00" + strings.Join(paths, "\x00")
}

func (r *Reader) cachedPaths(key string, paths []string) []string {
	if key == "" {
		return append([]string{}, paths...)
	}
	r.pathMu.RLock()
	cached := r.pathCache[key]
	r.pathMu.RUnlock()
	result := make([]string, 0, len(paths))
	if cached != "" {
		for _, path := range paths {
			if path == cached {
				result = append(result, path)
				break
			}
		}
	}
	for _, path := range paths {
		if path != cached {
			result = append(result, path)
		}
	}
	return result
}

func (r *Reader) rememberPath(key, path string) {
	if key == "" {
		return
	}
	r.pathMu.Lock()
	r.pathCache[key] = path
	r.pathMu.Unlock()
}

func (r *Reader) forgetPath(key, path string) {
	if key == "" {
		return
	}
	r.pathMu.Lock()
	if r.pathCache[key] == path {
		delete(r.pathCache, key)
	}
	r.pathMu.Unlock()
}

func (r *Reader) request(ctx context.Context, record configstore.AuthRecord, path string, query url.Values, authenticated bool) (any, int, error) {
	return r.requestJSON(ctx, record, http.MethodGet, path, query, nil, authenticated)
}

func (r *Reader) requestJSON(ctx context.Context, record configstore.AuthRecord, method, path string, query url.Values, payloadBody map[string]any, authenticated bool) (any, int, error) {
	baseURL, err := configstore.ValidateBaseURL(record.BaseURL)
	if err != nil {
		return nil, 0, err
	}
	var reader io.Reader
	if payloadBody != nil {
		encoded, encodeErr := json.Marshal(payloadBody)
		if encodeErr != nil {
			return nil, 0, errors.New("上游请求参数编码失败")
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(baseURL, "/")+path, reader)
	if err != nil {
		return nil, 0, errors.New("上游请求创建失败")
	}
	request.URL.RawQuery = query.Encode()
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Sub2API-Console/0.1")
	if payloadBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if authenticated {
		applyAuthentication(request, record)
	}
	response, err := r.http.Do(request)
	if err != nil {
		return nil, 0, fmt.Errorf("上游网络请求失败：%T", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil || len(body) > maximumResponseBytes {
		return nil, response.StatusCode, errors.New("上游响应过大或读取失败")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, response.StatusCode, &StatusError{
			StatusCode: response.StatusCode,
			Path:       path,
			Detail:     upstreamErrorDetail(body),
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return nil, response.StatusCode, errors.New("上游返回不是有效 JSON")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, response.StatusCode, errors.New("上游响应包含尾随数据")
	}
	if object, ok := payload.(map[string]any); ok && !businessSuccess(object) {
		return nil, response.StatusCode, errors.New("上游业务读取失败")
	}
	if _, object := payload.(map[string]any); !object {
		if _, array := payload.([]any); !array {
			return nil, response.StatusCode, errors.New("上游返回格式不可读")
		}
	}
	return payload, response.StatusCode, nil
}

func upstreamErrorDetail(body []byte) string {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	values := make([]string, 0, 2)
	for _, key := range []string{"message", "detail", "error", "reason"} {
		value, ok := payload[key].(string)
		if !ok {
			continue
		}
		value = strings.TrimSpace(redact.Secrets(value))
		if value == "" || slices.Contains(values, value) {
			continue
		}
		values = append(values, value)
		if len(values) == 2 {
			break
		}
	}
	detail := strings.Join(values, "；")
	characters := []rune(detail)
	if len(characters) > 300 {
		detail = string(characters[:300]) + "…"
	}
	return detail
}

func applyAuthentication(request *http.Request, record configstore.AuthRecord) {
	for key, value := range record.Headers {
		if !strings.EqualFold(key, "cookie") && !strings.ContainsAny(value, "\r\n") {
			request.Header.Set(key, value)
		}
	}
	if request.Header.Get("Authorization") == "" {
		var token *string
		switch record.AuthMode {
		case "newapi_admin_key":
			token = record.AdminKey
		case "custom_headers", "cookie":
		default:
			token = record.AccessToken
			if token == nil {
				token = record.AdminKey
			}
		}
		if token != nil && strings.TrimSpace(*token) != "" {
			request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(*token))
		}
	}
	if record.AuthMode != "custom_headers" && record.AuthMode != "cookie" && record.UserID != nil && strings.TrimSpace(*record.UserID) != "" {
		request.Header.Set("New-Api-User", strings.TrimSpace(*record.UserID))
	}
	for key, value := range record.Cookies {
		if strings.TrimSpace(key) != "" && !strings.ContainsAny(value, "\r\n") {
			request.AddCookie(&http.Cookie{Name: key, Value: value})
		}
	}
}

func catalogItems(payload any) ([]map[string]any, error) {
	if array, ok := payload.([]any); ok {
		return objectRows(array, "上游分组目录")
	}
	object, ok := payload.(map[string]any)
	if !ok {
		return nil, errors.New("上游分组目录返回格式不可读")
	}
	data := any(object)
	if raw, present := object["data"]; present {
		if raw == nil {
			return nil, errors.New("上游分组目录返回缺少对象")
		}
		data = raw
	}
	if array, ok := data.([]any); ok {
		return objectRows(array, "上游分组目录")
	}
	mapped, ok := data.(map[string]any)
	if !ok {
		return nil, errors.New("上游分组目录返回格式不可读")
	}
	for _, key := range []string{"items", "groups", "group_list"} {
		if raw, present := mapped[key]; present {
			array, ok := raw.([]any)
			if !ok {
				return nil, errors.New("上游分组目录包含无效项目")
			}
			return objectRows(array, "上游分组目录")
		}
	}
	rows := make([]map[string]any, 0, len(mapped))
	for name, raw := range mapped {
		row, ok := raw.(map[string]any)
		if !ok {
			return nil, errors.New("上游分组目录包含无效项目")
		}
		copy := cloneObject(row)
		if _, present := copy["id"]; !present {
			copy["id"] = name
		}
		if _, present := copy["group_id"]; !present {
			copy["group_id"] = name
		}
		if _, present := copy["name"]; !present {
			copy["name"] = name
		}
		rows = append(rows, copy)
	}
	return rows, nil
}

func keyItems(payload any) ([]map[string]any, error) {
	if array, ok := payload.([]any); ok {
		return objectRows(array, "上游 Key 目录")
	}
	object, ok := payload.(map[string]any)
	if !ok {
		return nil, errors.New("上游 Key 目录返回格式不可读")
	}
	data, present := object["data"]
	if present && data == nil {
		return nil, errors.New("上游 Key 目录返回缺少对象")
	}
	if present {
		switch item := data.(type) {
		case []any:
			return objectRows(item, "上游 Key 目录")
		case map[string]any:
			for _, key := range []string{"items", "keys", "tokens"} {
				if raw, found := item[key]; found {
					array, ok := raw.([]any)
					if !ok {
						return nil, errors.New("上游 Key 目录包含无效项目")
					}
					return objectRows(array, "上游 Key 目录")
				}
			}
			return nil, errors.New("上游 Key 目录返回对象缺少 items")
		}
	}
	if raw, found := object["items"]; found {
		array, ok := raw.([]any)
		if !ok {
			return nil, errors.New("上游 Key 目录包含无效项目")
		}
		return objectRows(array, "上游 Key 目录")
	}
	return []map[string]any{}, nil
}

func objectRows(values []any, label string) ([]map[string]any, error) {
	result := make([]map[string]any, len(values))
	for index, raw := range values {
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s第 %d 项不是对象", label, index+1)
		}
		result[index] = item
	}
	return result, nil
}

func payloadObject(payload any, label string) (map[string]any, error) {
	object, ok := payload.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s返回缺少对象", label)
	}
	if raw, present := object["data"]; present {
		data, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s返回缺少对象", label)
		}
		return data, nil
	}
	return object, nil
}

func pageTotal(payload any) (int, bool, error) {
	object, ok := payload.(map[string]any)
	if !ok {
		return 0, false, nil
	}
	var value any
	var present bool
	if data, ok := object["data"].(map[string]any); ok {
		value, present = data["total"]
	}
	if !present {
		value, present = object["total"]
	}
	if !present || value == nil {
		return 0, false, nil
	}
	text := textValue(value)
	parsed, err := strconv.Atoi(text)
	if err != nil || parsed < 0 || strconv.Itoa(parsed) != text {
		return 0, false, errors.New("上游 Key 目录 total 必须是非负整数")
	}
	return parsed, true, nil
}

func statusText(row map[string]any) *string {
	value, present := presentValue(row, "status", "state", "enabled")
	if !present {
		active := "active"
		return &active
	}
	if value == nil {
		return nil
	}
	if boolean, ok := value.(bool); ok {
		status := "disabled"
		if boolean {
			status = "active"
		}
		return &status
	}
	return optionalText(value)
}

func optionalDecimal(row map[string]any, names ...string) (*string, error) {
	value, present := presentValue(row, names...)
	if !present || value == nil || textValue(value) == "" {
		return nil, nil
	}
	text, err := decimalText(value)
	if err != nil {
		return nil, err
	}
	return &text, nil
}

func decimalText(value any) (string, error) {
	text := textValue(value)
	rational, ok := new(big.Rat).SetString(text)
	if !ok {
		return "", errors.New("不是有限十进制数")
	}
	return ratText(rational), nil
}

func divideDecimal(numerator, denominator string) (string, error) {
	left, leftOK := new(big.Rat).SetString(numerator)
	right, rightOK := new(big.Rat).SetString(denominator)
	if !leftOK || !rightOK || right.Sign() <= 0 {
		return "", errors.New("十进制除法参数无效")
	}
	return ratText(new(big.Rat).Quo(left, right)), nil
}

func ratText(value *big.Rat) string {
	text := strings.TrimRight(strings.TrimRight(value.FloatString(28), "0"), ".")
	if text == "" || text == "-0" {
		return "0"
	}
	return text
}

func positiveDecimal(value string) bool {
	parsed, ok := new(big.Rat).SetString(value)
	return ok && parsed.Sign() > 0
}

func strictBool(value any) (bool, bool) {
	switch item := value.(type) {
	case bool:
		return item, true
	case json.Number:
		if item.String() == "0" || item.String() == "1" {
			return item.String() == "1", true
		}
	case string:
		switch strings.ToLower(strings.TrimSpace(item)) {
		case "true", "1", "yes", "on":
			return true, true
		case "false", "0", "no", "off":
			return false, true
		}
	}
	return false, false
}

func virtualAutoGroup(row map[string]any) bool {
	for _, name := range []string{"group_id", "groupId", "id"} {
		if value, present := row[name]; present && strings.EqualFold(strings.TrimSpace(textValue(value)), "auto") {
			return true
		}
	}
	return false
}

func businessSuccess(payload map[string]any) bool {
	if value, present := payload["code"]; present && !successValue(value, true) {
		return false
	}
	for _, key := range []string{"success", "ok"} {
		if value, present := payload[key]; present && !successValue(value, false) {
			return false
		}
	}
	for _, key := range []string{"error", "errors"} {
		if value, present := payload[key]; present && value != nil && textValue(value) != "" {
			return false
		}
	}
	if status, ok := payload["status"].(string); ok {
		switch strings.ToLower(strings.TrimSpace(status)) {
		case "error", "failed", "failure":
			return false
		}
	}
	return true
}

func successValue(value any, code bool) bool {
	if value == true {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(textValue(value)))
	if code && (text == "0" || text == "200") {
		return true
	}
	return text == "1" || text == "200" || text == "true" || text == "success" || text == "succeeded" || text == "ok"
}

func presentValue(row map[string]any, names ...string) (any, bool) {
	for _, name := range names {
		if value, present := row[name]; present {
			return value, true
		}
	}
	return nil, false
}

func firstPresent(row map[string]any, names ...string) any {
	value, _ := presentValue(row, names...)
	return value
}

func textValue(value any) string {
	if value == nil {
		return ""
	}
	if number, ok := value.(json.Number); ok {
		return number.String()
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func optionalText(value any) *string {
	text := textValue(value)
	if text == "" {
		return nil
	}
	return &text
}

func cloneObject(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func isNewAPI(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "newapi" || value == "oneapi"
}
