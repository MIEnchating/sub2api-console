package modelcheck

import (
	"fmt"
	"net/http"
	"strings"
)

// Capture only transport facts. Never retain headers, URLs, credentials or bodies.
type verificationTransport struct {
	base             http.RoundTripper
	protocol         string
	status, attempts int
}

func (t *verificationTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.attempts++
	t.status = 0
	switch {
	case strings.HasSuffix(r.URL.Path, "/messages"):
		t.protocol = "anthropic_messages"
	case strings.HasSuffix(r.URL.Path, "/responses"):
		t.protocol = "openai_responses"
	case strings.HasSuffix(r.URL.Path, "/chat/completions"):
		t.protocol = "openai_chat"
	case strings.Contains(r.URL.Path, ":generateContent"), strings.Contains(r.URL.Path, ":streamGenerateContent"):
		t.protocol = "gemini_native"
	default:
		t.protocol = "other"
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	response, err := base.RoundTrip(r)
	if response != nil {
		t.status = response.StatusCode
	}
	return response, err
}

func verificationProtocolEvidence(probes []VerificationProbe) VerificationEvidence {
	observed, usage, attempts := 0, 0, 0
	for _, probe := range probes {
		attempts += probe.Attempts
		if probe.HTTPStatus > 0 {
			observed++
		}
		if probe.Usage != nil && probe.Usage.InputTokens != nil && probe.Usage.OutputTokens != nil {
			usage++
		}
	}
	return VerificationEvidence{ID: "protocol", Label: "协议与用量", Status: "info", Detail: fmt.Sprintf("实际发送 %d 次 HTTP 请求，%d 项收到 HTTP 响应，%d 项有输入和输出用量。兼容协议和 Token 数由上游提供，仅作线索；缺失不等于假模型。", attempts, observed, usage)}
}

func verificationRepeatEvidence(probes []VerificationProbe) VerificationEvidence {
	first := map[int]VerificationProbe{}
	compared, changed := 0, 0
	for _, probe := range probes {
		if probe.ID == "arithmetic" {
			first[probe.Round] = probe
		}
		if probe.ID != "consistency" {
			continue
		}
		baseline, ok := first[probe.Round]
		if !ok || baseline.Status == "error" || probe.Status == "error" {
			continue
		}
		compared++
		if baseline.Status != probe.Status || baseline.ResponseModel != probe.ResponseModel {
			changed++
		}
	}
	if compared == 0 {
		return VerificationEvidence{ID: "repeat", Label: "重复请求", Status: "unknown", Detail: "没有完成可比较的重复请求，无法评估稳定性。"}
	}
	return VerificationEvidence{ID: "repeat", Label: "重复请求", Status: "info", Detail: fmt.Sprintf("完成 %d 组相同问题比较，其中 %d 组的正确性或返回型号发生变化。采样波动与路由变化均可能导致差异，不据此认证或否定模型身份。", compared, changed)}
}
