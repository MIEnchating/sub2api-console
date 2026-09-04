package upstreamsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type KeyUsageObservation struct {
	Cost   string
	Source string
}

type NewAPIUsageObservations struct {
	Keys         map[string]KeyUsageObservation
	QuotaPerUnit string
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
	cost, err := decimalText(raw)
	parsedCost, parsed := new(big.Rat).SetString(cost)
	if err != nil || !parsed || parsedCost.Sign() < 0 {
		return KeyUsageObservation{}, errors.New("Sub2API Key 计费金额无效")
	}
	return KeyUsageObservation{Cost: cost, Source: "sub2api-key-stats"}, nil
}

// ReadNewAPIKeyUsage uses NewAPI's server-side flow aggregation. It never scans
// request logs or allocates host totals across Tokens.
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
	quotaPerUnitText, err := decimalText(status["quota_per_unit"])
	quotaPerUnitRat, parsed := new(big.Rat).SetString(quotaPerUnitText)
	if err != nil || !parsed || quotaPerUnitRat.Sign() <= 0 {
		return NewAPIUsageObservations{}, errors.New("NewAPI quota_per_unit 缺失或无效")
	}
	if enabled, present := strictBool(status["enable_data_export"]); present && !enabled {
		return NewAPIUsageObservations{}, errors.New("NewAPI 未开启数据看板，无法按稳定 Token ID 核对消费")
	}
	payload, responseStatus, err := r.request(ctx, record, "/api/data/flow/self", url.Values{
		"start_timestamp": {strconv.FormatInt(start.Unix(), 10)},
		"end_timestamp":   {strconv.FormatInt(end.Unix()-1, 10)},
	}, true)
	if err != nil {
		if responseStatus == 404 || responseStatus == 405 {
			return NewAPIUsageObservations{}, errors.New("NewAPI 版本不支持稳定 Token ID 消费聚合，无法精确核对")
		}
		return NewAPIUsageObservations{}, fmt.Errorf("NewAPI Token 消费聚合读取失败：%w", err)
	}
	rows, err := newAPIFlowRows(payload)
	if err != nil {
		return NewAPIUsageObservations{}, err
	}
	totals := make(map[string]*big.Rat)
	for _, row := range rows {
		quotaText, quotaErr := decimalText(row["quota"])
		quota, quotaOK := new(big.Rat).SetString(quotaText)
		tokenID, tokenErr := strictInteger(row["token_id"])
		if quotaErr != nil || !quotaOK || quota.Sign() < 0 {
			return NewAPIUsageObservations{}, errors.New("NewAPI Token 消费聚合包含无效 quota")
		}
		if tokenErr != nil || tokenID <= 0 {
			return NewAPIUsageObservations{}, errors.New("NewAPI Token 消费聚合缺少稳定 Token ID")
		}
		key := strconv.Itoa(tokenID)
		if totals[key] == nil {
			totals[key] = new(big.Rat)
		}
		totals[key].Add(totals[key], quota)
	}
	result := make(map[string]KeyUsageObservation, len(totals))
	for tokenID, quota := range totals {
		result[tokenID] = KeyUsageObservation{Cost: ratText(new(big.Rat).Quo(quota, quotaPerUnitRat)), Source: "newapi-token-flow"}
	}
	return NewAPIUsageObservations{Keys: result, QuotaPerUnit: quotaPerUnitText}, nil
}

func newAPIFlowRows(payload any) ([]map[string]any, error) {
	object, ok := payload.(map[string]any)
	if !ok {
		return nil, errors.New("NewAPI Token 消费聚合返回格式不可读")
	}
	rawRows, ok := object["data"].([]any)
	if !ok {
		return nil, errors.New("NewAPI Token 消费聚合未返回数据列表")
	}
	rows, err := objectRows(rawRows, "NewAPI Token 消费聚合")
	if err != nil {
		return nil, err
	}
	return rows, nil
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
