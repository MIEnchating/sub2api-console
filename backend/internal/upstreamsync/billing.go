package upstreamsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

const newAPIBillingPageSize = 100

type KeyUsageObservation struct {
	Cost   float64
	Source string
}

type NewAPIUsageObservations struct {
	Keys         map[string]KeyUsageObservation
	QuotaPerUnit float64
}

func (r *Reader) ReadSub2APIKeyUsage(
	ctx context.Context,
	record configstore.AuthRecord,
	keyID, date, timezone string,
) (KeyUsageObservation, error) {
	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return KeyUsageObservation{}, errors.New("绑定缺少稳定 Key ID")
	}
	payload, _, err := r.request(ctx, record, "/api/v1/usage/stats", url.Values{
		"api_key_id": {keyID}, "start_date": {date}, "end_date": {date}, "timezone": {timezone},
	}, true)
	if err != nil {
		return KeyUsageObservation{}, err
	}
	data, err := payloadObject(payload, "Sub2API Key 计费")
	if err != nil {
		return KeyUsageObservation{}, err
	}
	raw, present := data["total_actual_cost"]
	if !present {
		raw, present = data["total_cost"]
	}
	if !present {
		return KeyUsageObservation{}, errors.New("Sub2API Key 计费未返回 total_actual_cost")
	}
	cost, err := finiteNumber(raw)
	if err != nil || cost < 0 {
		return KeyUsageObservation{}, errors.New("Sub2API Key 计费金额无效")
	}
	return KeyUsageObservation{Cost: cost, Source: "sub2api-key-stats"}, nil
}

// ReadNewAPIKeyUsage reads the full positive-consume and refund log windows
// twice. It returns only stable Token-ID totals and fails closed when pagination
// or attribution cannot be proven complete.
func (r *Reader) ReadNewAPIKeyUsage(
	ctx context.Context,
	record configstore.AuthRecord,
	start, end time.Time,
) (NewAPIUsageObservations, error) {
	if !end.After(start) {
		return NewAPIUsageObservations{}, errors.New("NewAPI 计费时间窗口无效")
	}
	public, err := r.getPublic(ctx, record.BaseURL, "/api/status")
	if err != nil {
		return NewAPIUsageObservations{}, fmt.Errorf("NewAPI quota_per_unit 读取失败：%w", err)
	}
	status, err := payloadObject(public, "NewAPI 状态")
	if err != nil {
		return NewAPIUsageObservations{}, err
	}
	quotaPerUnit, err := finiteNumber(status["quota_per_unit"])
	if err != nil || quotaPerUnit <= 0 {
		return NewAPIUsageObservations{}, errors.New("NewAPI quota_per_unit 缺失或无效")
	}

	totals := map[string]float64{}
	for _, logType := range []int{2, 6} {
		first, firstFingerprint, err := r.readNewAPILogPass(ctx, record, logType, start, end)
		if err != nil {
			return NewAPIUsageObservations{}, err
		}
		second, secondFingerprint, err := r.readNewAPILogPass(ctx, record, logType, start, end)
		if err != nil {
			return NewAPIUsageObservations{}, err
		}
		if firstFingerprint != secondFingerprint {
			return NewAPIUsageObservations{}, errors.New("NewAPI 计费日志在双次分页读取期间发生变化，无法精确核对")
		}
		for tokenID, quota := range first {
			if logType == 6 {
				totals[tokenID] -= quota
			} else {
				totals[tokenID] += quota
			}
		}
		_ = second
	}
	result := make(map[string]KeyUsageObservation, len(totals))
	for tokenID, quota := range totals {
		result[tokenID] = KeyUsageObservation{Cost: quota / quotaPerUnit, Source: "newapi-token-logs"}
	}
	return NewAPIUsageObservations{Keys: result, QuotaPerUnit: quotaPerUnit}, nil
}

func (r *Reader) readNewAPILogPass(
	ctx context.Context,
	record configstore.AuthRecord,
	logType int,
	start, end time.Time,
) (map[string]float64, string, error) {
	totals := map[string]float64{}
	fingerprints := make([]string, 0)
	seen := map[string]struct{}{}
	expectedTotal := -1
	fetched := 0
	for page := 1; page <= maximumPages; page++ {
		payload, _, err := r.request(ctx, record, "/api/log/self", url.Values{
			"p": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(newAPIBillingPageSize)},
			"type": {strconv.Itoa(logType)}, "start_timestamp": {strconv.FormatInt(start.Unix(), 10)},
			"end_timestamp": {strconv.FormatInt(end.Unix()-1, 10)},
		}, true)
		if err != nil {
			return nil, "", err
		}
		pageData, err := payloadObject(payload, "NewAPI 计费日志")
		if err != nil {
			return nil, "", err
		}
		pageNumber, err := strictInteger(pageData["page"])
		if err != nil || pageNumber != page {
			return nil, "", errors.New("NewAPI 计费日志返回无效页码")
		}
		pageSize, err := strictInteger(pageData["page_size"])
		if err != nil || pageSize <= 0 || pageSize > newAPIBillingPageSize {
			return nil, "", errors.New("NewAPI 计费日志返回无效分页大小")
		}
		total, err := strictInteger(pageData["total"])
		if err != nil || total < 0 || total >= 10000 {
			return nil, "", errors.New("NewAPI 计费日志总数无效或达到查询上限")
		}
		if expectedTotal == -1 {
			expectedTotal = total
		} else if expectedTotal != total {
			return nil, "", errors.New("NewAPI 计费日志分页总数发生变化")
		}
		rawItems, ok := pageData["items"].([]any)
		if !ok {
			return nil, "", errors.New("NewAPI 计费日志未返回 items")
		}
		if len(rawItems) == 0 && fetched < expectedTotal {
			return nil, "", errors.New("NewAPI 计费日志分页提前结束")
		}
		for _, raw := range rawItems {
			item, ok := raw.(map[string]any)
			if !ok {
				return nil, "", errors.New("NewAPI 计费日志包含无效项目")
			}
			rowType, err := strictInteger(item["type"])
			if err != nil || rowType != logType {
				return nil, "", errors.New("NewAPI 计费日志类型与查询条件不一致")
			}
			quota, err := finiteNumber(item["quota"])
			if err != nil || quota < 0 {
				return nil, "", errors.New("NewAPI 计费日志包含无效 quota")
			}
			tokenID, err := strictInteger(item["token_id"])
			if err != nil || (quota != 0 && tokenID <= 0) {
				return nil, "", errors.New("NewAPI 非零计费日志缺少稳定 Token ID")
			}
			fingerprint, err := newAPILogFingerprint(item)
			if err != nil {
				return nil, "", err
			}
			if _, duplicate := seen[fingerprint]; duplicate {
				return nil, "", errors.New("NewAPI 计费日志分页包含重复项目")
			}
			seen[fingerprint] = struct{}{}
			fingerprints = append(fingerprints, fingerprint)
			if quota != 0 {
				totals[strconv.Itoa(tokenID)] += quota
			}
		}
		fetched += len(rawItems)
		if fetched == expectedTotal {
			break
		}
		if fetched > expectedTotal {
			return nil, "", errors.New("NewAPI 计费日志分页数量超过 total")
		}
	}
	if fetched != expectedTotal {
		return nil, "", errors.New("NewAPI 计费日志分页未完整读取")
	}
	sort.Strings(fingerprints)
	return totals, strings.Join(fingerprints, "\n"), nil
}

func newAPILogFingerprint(item map[string]any) (string, error) {
	copy := make(map[string]any, len(item))
	for key, value := range item {
		if key == "id" {
			continue
		}
		copy[key] = value
	}
	encoded, err := json.Marshal(copy)
	if err != nil {
		return "", errors.New("NewAPI 计费日志指纹生成失败")
	}
	return string(encoded), nil
}

func finiteNumber(value any) (float64, error) {
	var parsed float64
	var err error
	switch raw := value.(type) {
	case json.Number:
		parsed, err = raw.Float64()
	case float64:
		parsed = raw
	case float32:
		parsed = float64(raw)
	case int:
		parsed = float64(raw)
	case int64:
		parsed = float64(raw)
	default:
		return 0, errors.New("数值字段类型无效")
	}
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, errors.New("数值字段不是有限数值")
	}
	return parsed, nil
}

func strictInteger(value any) (int, error) {
	parsed, err := finiteNumber(value)
	if err != nil || parsed != math.Trunc(parsed) || parsed < math.MinInt || parsed > math.MaxInt {
		return 0, errors.New("整数字段无效")
	}
	return int(parsed), nil
}
