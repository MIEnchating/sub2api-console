package workbenchprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/decimalutil"
)

const bowerEndpoint = "https://smsbower.page/stubs/handler_api.php"

func (s *SMS) bowerRequest(ctx context.Context, action string, params url.Values, mutation bool) (string, error) {
	if params == nil {
		params = make(url.Values)
	}
	params.Set("api_key", s.config.APIKey)
	params.Set("action", action)
	body, err := s.http.Do(ctx, Request{Method: http.MethodGet, URL: bowerEndpoint + "?" + params.Encode(), NonReplayable: mutation})
	if err != nil {
		if mutation {
			return "", uncertainSMS(err)
		}
		return "", err
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		err := smsFailure("sms_response_invalid", "SMSBower 返回空响应，请核对供应商状态", false)
		err.Uncertain = mutation
		return "", err
	}
	return text, nil
}

func (s *SMS) bowerAcquire(ctx context.Context) (SMSNumber, error) {
	if !smsCountry.MatchString(s.config.Country) {
		return SMSNumber{}, smsFailure("sms_country_required", "请先选择 SMSBower 国家与价格", true)
	}
	params := url.Values{"service": {s.config.Service}, "country": {s.config.Country}}
	if s.config.MaxPrice != "" {
		params.Set("maxPrice", s.config.MaxPrice)
	}
	text, err := s.bowerRequest(ctx, "getNumber", params, true)
	if err != nil {
		return SMSNumber{}, err
	}
	parts := strings.Split(text, ":")
	if len(parts) != 3 || parts[0] != "ACCESS_NUMBER" || !smsRequestID.MatchString(parts[1]) {
		return SMSNumber{}, bowerFailure(text, true)
	}
	phone, err := normalizeSMSPhone(parts[2])
	if err != nil {
		return SMSNumber{RequestID: parts[1]}, err
	}
	return SMSNumber{RequestID: parts[1], Phone: phone}, nil
}

func (s *SMS) bowerPoll(ctx context.Context, requestID string) (SMSMessage, error) {
	text, err := s.bowerRequest(ctx, "getStatus", url.Values{"id": {requestID}}, false)
	if err != nil {
		return SMSMessage{}, err
	}
	if text == "STATUS_WAIT_CODE" || text == "STATUS_WAIT_RESEND" || strings.HasPrefix(text, "STATUS_WAIT_RETRY") {
		return SMSMessage{Pending: true}, nil
	}
	if strings.HasPrefix(text, "STATUS_OK:") {
		code := independentSMSCode(strings.TrimPrefix(text, "STATUS_OK:"))
		if code == "" {
			return SMSMessage{}, smsFailure("sms_code_invalid", "供应商短信不含有效的独立六位验证码，请核对短信", true)
		}
		return SMSMessage{Code: code}, nil
	}
	return SMSMessage{}, bowerFailure(text, false)
}

func (s *SMS) bowerStatus(ctx context.Context, requestID, action string) error {
	status, expected := "1", "ACCESS_READY"
	switch action {
	case "complete":
		status, expected = "6", "ACCESS_ACTIVATION"
	case "release":
		status, expected = "8", "ACCESS_CANCEL"
	}
	text, err := s.bowerRequest(ctx, "setStatus", url.Values{"id": {requestID}, "status": {status}}, true)
	if err != nil {
		return err
	}
	if text == expected || (action == "ready" && text == "ACCESS_RETRY_GET") {
		return nil
	}
	return bowerFailure(text, true)
}

func bowerFailure(text string, mutation bool) *SMSError {
	messages := map[string]string{
		"BAD_KEY": "SMSBower API Key 无效，请检查配置", "BAD_ACTION": "SMSBower 不支持该接口操作，请核对供应商版本",
		"BAD_SERVICE": "SMSBower 服务代码无效，请检查配置", "NO_NUMBERS": "所选国家当前无可用号码，请重新查询库存",
		"NO_BALANCE": "SMSBower 余额不足，请补充余额后重新申请", "NO_ACTIVATION": "SMSBower 订单不存在，请核对订单",
		"BAD_STATUS": "SMSBower 订单状态不允许该操作，请核对订单", "EARLY_CANCEL_DENIED": "SMSBower 暂不允许取消该订单，请稍后在供应商核对",
		"STATUS_CANCEL": "SMSBower 已取消该号码订单，请重新授权", "BANNED": "SMSBower 账号已受限制，请联系供应商",
	}
	if message, known := messages[text]; known {
		return smsFailure("smsbower_"+strings.ToLower(text), message, text != "NO_NUMBERS" && text != "EARLY_CANCEL_DENIED")
	}
	return &SMSError{Code: "sms_response_invalid", Message: "SMSBower 响应未通过业务校验，请核对供应商订单和服务状态", Uncertain: mutation}
}

func (s *SMS) bowerOptions(ctx context.Context) ([]SMSOption, error) {
	pricesText, err := s.bowerRequest(ctx, "getPrices", url.Values{"service": {s.config.Service}}, false)
	if err != nil {
		return nil, err
	}
	countriesText, err := s.bowerRequest(ctx, "getCountries", nil, false)
	if err != nil {
		return nil, err
	}
	prices, err := decodeSMSObject([]byte(pricesText))
	if err != nil {
		return nil, bowerFailure(pricesText, false)
	}
	countries, err := decodeSMSValue([]byte(countriesText))
	if err != nil {
		return nil, bowerFailure(countriesText, false)
	}
	if failedMailEnvelope(prices, 0) || failedMailEnvelope(countries, 0) {
		return nil, smsFailure("sms_options_failed", "SMSBower 价格或国家接口报告失败，请检查供应商权限后重新查询", true)
	}
	index := map[string]map[string]any{}
	collectSMSCountries(countries, "", index, 0)
	options := map[string]SMSOption{}
	for key, services := range prices {
		details, ok := services.(map[string]any)
		if !ok {
			continue
		}
		if service, ok := details[s.config.Service].(map[string]any); ok {
			details = service
		}
		price := smsValueString(details["cost"])
		value, validPrice := decimalutil.Parse(price)
		count, countErr := strconv.ParseInt(smsValueString(details["count"]), 10, 64)
		if !smsPrice.MatchString(price) || !validPrice || value.Sign() < 0 || countErr != nil || count <= 0 {
			continue
		}
		country := index[strings.ToLower(key)]
		id := smsValueString(country["activate_org_code"])
		if id == "" {
			id = key
		}
		if !smsCountry.MatchString(id) {
			continue
		}
		title := "国家 " + id
		for _, field := range []string{"chn", "title", "eng"} {
			if text := smsValueString(country[field]); text != "" {
				title = text
				break
			}
		}
		title = strings.ReplaceAll(title, s.config.APIKey, "[已隐藏]")
		if len([]rune(title)) > 120 {
			title = string([]rune(title)[:120])
		}
		iso := strings.ToUpper(smsValueString(country["iso"]))
		if len(iso) != 2 || iso[0] < 'A' || iso[0] > 'Z' || iso[1] < 'A' || iso[1] > 'Z' {
			iso = ""
		}
		prefix := smsValueString(country["prefix"])
		if !smsCountry.MatchString(prefix) || len(prefix) > 4 {
			prefix = ""
		}
		option := SMSOption{Country: id, Title: title, ISO: iso, Prefix: prefix, Price: price, Count: count}
		previous, exists := options[id]
		if exists {
			previousPrice, _ := decimalutil.Parse(previous.Price)
			if value.Cmp(previousPrice) >= 0 {
				continue
			}
		}
		options[id] = option
	}
	result := make([]SMSOption, 0, len(options))
	for _, option := range options {
		result = append(result, option)
	}
	slices.SortFunc(result, func(left, right SMSOption) int {
		leftPrice, _ := decimalutil.Parse(left.Price)
		rightPrice, _ := decimalutil.Parse(right.Price)
		if comparison := leftPrice.Cmp(rightPrice); comparison != 0 {
			return comparison
		}
		if left.Count != right.Count {
			if left.Count > right.Count {
				return -1
			}
			return 1
		}
		return strings.Compare(left.Country, right.Country)
	})
	if len(result) == 0 {
		return nil, smsFailure("sms_no_stock", "SMSBower 当前没有该服务可购买的国家号码，请稍后查询", false)
	}
	return result, nil
}

func collectSMSCountries(value any, alias string, result map[string]map[string]any, depth int) {
	if depth > 5 {
		return
	}
	switch typed := value.(type) {
	case []any:
		for index, item := range typed {
			collectSMSCountries(item, strconv.Itoa(index), result, depth+1)
		}
	case map[string]any:
		if typed["id"] != nil && (typed["chn"] != nil || typed["title"] != nil || typed["eng"] != nil || typed["iso"] != nil) {
			result[strings.ToLower(alias)] = typed
			for _, field := range []string{"activate_org_code", "id", "slug", "title", "eng", "chn", "iso"} {
				if key := smsValueString(typed[field]); key != "" {
					result[strings.ToLower(key)] = typed
				}
			}
			return
		}
		for key, item := range typed {
			collectSMSCountries(item, key, result, depth+1)
		}
	}
}

func positiveSMSPrice(raw string) bool {
	value, ok := decimalutil.Parse(raw)
	return ok && value.Sign() > 0
}

func smsValueString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}
