package workbenchprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

const lubanEndpoint = "https://lubansms.com/v2/api/"

func (s *SMS) lubanRequest(ctx context.Context, endpoint string, params url.Values, mutation bool) (map[string]any, error) {
	params.Set("apikey", s.config.APIKey)
	body, err := s.http.Do(ctx, Request{Method: http.MethodGet, URL: lubanEndpoint + endpoint + "?" + params.Encode(), NonReplayable: mutation})
	if err != nil {
		if mutation {
			return nil, uncertainSMS(err)
		}
		return nil, err
	}
	data, err := decodeSMSObject(body)
	if err != nil {
		return nil, &SMSError{Code: "sms_response_invalid", Message: "LubanSMS 响应格式无效，请核对供应商订单", Uncertain: mutation}
	}
	code := smsValueString(data["code"])
	if code == "" {
		return nil, &SMSError{Code: "sms_response_invalid", Message: "LubanSMS 响应缺少业务状态，请核对供应商订单", Uncertain: mutation}
	}
	if code != "0" || failedMailEnvelope(data, 0) {
		return nil, smsFailure("sms_luban_failed", "LubanSMS 未成功处理请求，请检查余额、供应商编号和订单状态", code == "400" || code == "401")
	}
	return data, nil
}

func (s *SMS) lubanAcquire(ctx context.Context) (SMSNumber, error) {
	data, err := s.lubanRequest(ctx, "getNumber", url.Values{"service_id": {s.config.ServiceID}}, true)
	if err != nil {
		return SMSNumber{}, err
	}
	requestID := smsValueString(data["request_id"])
	if !smsRequestID.MatchString(requestID) {
		return SMSNumber{}, &SMSError{Code: "sms_response_invalid", Message: "LubanSMS 未返回有效订单 ID，请核对供应商订单", Uncertain: true}
	}
	phone, err := normalizeSMSPhone(smsValueString(data["number"]))
	if err != nil {
		return SMSNumber{RequestID: requestID}, err
	}
	return SMSNumber{RequestID: requestID, Phone: phone}, nil
}

func (s *SMS) lubanPoll(ctx context.Context, requestID string) (SMSMessage, error) {
	data, err := s.lubanRequest(ctx, "getSms", url.Values{"request_id": {requestID}}, false)
	if err != nil {
		return SMSMessage{}, err
	}
	if strings.EqualFold(smsValueString(data["msg"]), "wait") {
		return SMSMessage{Pending: true}, nil
	}
	for _, key := range []string{"sms_code", "sms_msg", "message", "text", "msg"} {
		if code := providerSMSCode(data[key], 0); code != "" {
			return SMSMessage{Code: code}, nil
		}
	}
	return SMSMessage{}, smsFailure("sms_code_invalid", "LubanSMS 短信不含有效的独立六位验证码，请核对短信", true)
}

func (s *SMS) lubanRelease(ctx context.Context, requestID string) error {
	_, err := s.lubanRequest(ctx, "setStatus", url.Values{"request_id": {requestID}, "status": {"reject"}}, true)
	return err
}

func decodeSMSValue(body []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return nil, smsFailure("sms_response_invalid", "接码供应商 JSON 响应无效", true)
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return nil, smsFailure("sms_response_invalid", "接码供应商 JSON 响应包含多余内容", true)
	}
	return value, nil
}

func decodeSMSObject(body []byte) (map[string]any, error) {
	value, err := decodeSMSValue(body)
	if err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok || object == nil {
		return nil, smsFailure("sms_response_invalid", "接码供应商响应不是有效对象", true)
	}
	return object, nil
}

func independentSMSCode(text string) string {
	for _, index := range mailDigits.FindAllStringIndex(text, -1) {
		if (index[0] > 0 && asciiDigit(text[index[0]-1])) || (index[1] < len(text) && asciiDigit(text[index[1]])) {
			continue
		}
		return text[index[0]:index[1]]
	}
	return ""
}

func providerSMSCode(value any, depth int) string {
	if depth > 8 {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return independentSMSCode(typed)
	case json.Number:
		return independentSMSCode(typed.String())
	case []any:
		for _, item := range typed {
			if code := providerSMSCode(item, depth+1); code != "" {
				return code
			}
		}
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, explicit := range []bool{true, false} {
			for _, key := range keys {
				canonical := strings.ReplaceAll(strings.ToLower(key), "_", "")
				if slices.Contains([]string{"requestid", "applicationid", "countryid", "serviceid", "number", "phone", "id"}, canonical) || mailCodeField.MatchString(key) != explicit {
					continue
				}
				if code := providerSMSCode(typed[key], depth+1); code != "" {
					return code
				}
			}
		}
	}
	return ""
}
