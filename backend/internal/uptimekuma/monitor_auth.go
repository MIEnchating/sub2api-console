package uptimekuma

import (
	"encoding/json"
	"strings"
)

func unchangedMonitorAuth(raw map[string]json.RawMessage, options *MonitorOptions) bool {
	return rawString(raw, "authMethod") == options.AuthMethod && options.AuthUsername == "" && options.AuthPassword == "" && !options.ClearAuth
}

func prepareMonitorAuthEdit(raw map[string]json.RawMessage, in *MonitorInput) error {
	o := in.Options
	// A changed method cannot borrow credentials belonging to a different scheme.
	if o.AuthMethod != rawString(raw, "authMethod") && !o.ClearAuth {
		if (o.AuthMethod == "basic" && o.AuthUsername == "") || ((o.AuthMethod == "basic" || o.AuthMethod == "bearer") && o.AuthPassword == "") {
			return failure("kuma_invalid_auth", "切换鉴权方式时，请填写完整的用户名和密码或 Token", 422)
		}
	}
	if in.appliedTemplate == nil || in.TemplateRetain || !unchangedMonitorAuth(raw, o) {
		return nil
	}
	// Template headers may change, but authentication headers remain monitor-owned.
	previous := map[string]string{}
	if headers := rawString(raw, "headers"); headers != "" {
		if err := json.Unmarshal([]byte(headers), &previous); err != nil {
			return failure("kuma_invalid_auth", "原监控请求头无法解析，无法安全保留鉴权，请检查后重试", 422)
		}
	}
	headers := map[string]string{}
	if o.Headers != "" {
		if err := json.Unmarshal([]byte(o.Headers), &headers); err != nil {
			return protocolError()
		}
	}
	for key, value := range previous {
		if strings.EqualFold(key, "Authorization") || strings.EqualFold(key, "x-api-key") {
			headers[key] = value
		}
	}
	data, err := json.Marshal(headers)
	if err != nil {
		return err
	}
	o.Headers = string(data)
	return nil
}
