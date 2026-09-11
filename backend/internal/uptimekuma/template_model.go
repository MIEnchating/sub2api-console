package uptimekuma

import (
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Change only the model of this monitor's request; leave the stored template intact.
func templateBodyWithModel(body, encoding, kind, model string) (string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return body, nil
	}
	if utf8.RuneCountInString(model) > 200 || strings.IndexFunc(model, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) >= 0 {
		return "", failure("kuma_invalid_template_model", "请输入不含空白或控制字符的模型名称，最多 200 个字符", 422)
	}
	if !isHTTP(kind) || encoding != "" && encoding != "json" {
		return "", failure("kuma_invalid_template_model", "自定义模型需要使用 JSON 请求体的 HTTP 模板，请更换模板", 422)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &fields); err != nil || fields == nil {
		return "", failure("kuma_invalid_template_model", "模板请求体须为 JSON 对象，请先修改模板请求体", 422)
	}
	fields["model"], _ = json.Marshal(model)
	updated, err := json.Marshal(fields)
	return string(updated), err
}
