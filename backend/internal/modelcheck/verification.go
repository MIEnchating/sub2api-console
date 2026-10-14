package modelcheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

func runVerification(ctx context.Context, directClient, oauthClient *http.Client, credential credentialResolution, input targetRequest) (map[string]any, error) {
	report := VerificationReport{Version: verificationVersion, Evidence: []VerificationEvidence{}, Probes: []VerificationProbe{}}
	report.Total = input.Rounds * 7
	report.Authenticity = VerificationDimension{Status: "unverified", Summary: "第三方接口的身份尚未证实；返回型号和协议字段不能证明实际模型。"}
	source := "third_party"
	if credential.oauth != nil {
		source = "official_oauth"
	} else if officialVerificationEndpoint(credential.value.BaseURL, credential.value.Platform) {
		source = "official_endpoint"
	}
	report.Evidence = append(report.Evidence, VerificationEvidence{ID: "source", Label: "请求来源", Status: "info", Detail: map[string]string{"third_party": "第三方接口；请求路径由账号配置决定。", "official_endpoint": "官方 API 地址；使用 HTTPS 且禁止重定向。", "official_oauth": "固定官方 OAuth 端点；未经过生产中转路径。"}[source]})
	if source != "third_party" {
		report.Authenticity.Summary = "请求发送到官方端点；该证据只覆盖本次检测路径，不代表生产路由，也不证明具体权重版本。"
	}
	successful := 0
	models := []string{}
	started := time.Now()
	for round := 1; round <= input.Rounds; round++ {
		challenges := slices.Clone(input.Challenges[round-1])
		// Repeat one identical question to reveal unstable results without treating
		// sampling variation as proof of a substituted model.
		repeated := challenges[0]
		repeated.id = "consistency"
		repeated.label = "重复请求一致性"
		challenges = append(challenges, repeated)
		for _, challenge := range challenges {
			if ctx.Err() != nil {
				break
			}
			requestID, err := randomTaskID()
			if err != nil {
				return nil, err
			}
			row := VerificationProbe{ID: challenge.id, Label: challenge.label, Round: round, RequestID: requestID, Prompt: challenge.prompt, Expected: challenge.expected, Status: "error", Usage: &AnimationUsage{}}
			trace := &verificationTransport{base: directClient.Transport}
			direct := *directClient
			direct.Transport = trace
			oauth := *oauthClient
			if credential.oauth != nil {
				trace.base = oauthClient.Transport
				oauth.Transport = trace
			}
			var sender reasoningSender
			if credential.oauth != nil {
				sender = oauthBundleSender{client: &oauth, credential: *credential.oauth, models: []string{input.Model}, slots: make(chan struct{}, 1), requestID: requestID, stream: true, usage: row.Usage}
			} else {
				value := credential.value
				// A failed host must not silently change the path being verified.
				value.FallbackBaseURL = ""
				if value.Platform == "claude" {
					value.Platform = "anthropic"
				}
				sender = directBundleSender{client: &direct, credential: value, requestID: requestID, stream: true, usage: row.Usage}
			}
			probeStarted := time.Now()
			answer, model, sendErr := sender.SendWithReasoning(ctx, input.AccountID, input.Model, challenge.prompt, input.TimeoutSeconds, "medium")
			row.DurationMS = time.Since(probeStarted).Milliseconds()
			row.Protocol, row.HTTPStatus, row.Attempts = trace.protocol, trace.status, trace.attempts
			row.ResponseModel = model
			if sendErr == nil && strings.TrimSpace(answer) == "" {
				sendErr = fmt.Errorf("上游未返回有效文本")
			}
			if sendErr == nil && credential.value.Secret != "" && strings.Contains(answer, credential.value.Secret) {
				sendErr = fmt.Errorf("上游回答包含敏感信息，已拒绝展示")
			}
			if sendErr != nil {
				row.Error = safeCredentialError(sendErr)
				row.ErrorCode = verificationErrorCode(row.Error, ctx, trace.status)
				row.Error += verificationErrorAdvice(row.ErrorCode)
			} else {
				successful++
				row.Answer, row.AnswerTruncated = precheckAnswer(answer)
				row.ResponseModel = model
				if model != "" && !slices.Contains(models, model) {
					models = append(models, model)
				}
				row.Status = "failed"
				if verificationAnswerMatches(challenge, answer) {
					row.Status = "passed"
					report.Passed++
				}
			}
			report.Probes = append(report.Probes, row)
			if input.Progress != nil {
				input.Progress(report)
			}
			// Auth and rate-limit failures are operational. Do not hammer the endpoint
			// with the remaining probes or count unexecuted probes as wrong answers.
			if row.ErrorCode == "rate_limited" || row.ErrorCode == "unauthorized" || row.ErrorCode == "unsupported_request" {
				break
			}
		}
		if ctx.Err() != nil || (len(report.Probes) > 0 && (report.Probes[len(report.Probes)-1].ErrorCode == "rate_limited" || report.Probes[len(report.Probes)-1].ErrorCode == "unauthorized" || report.Probes[len(report.Probes)-1].ErrorCode == "unsupported_request")) {
			break
		}
	}
	report.Total = input.Rounds * 7
	report.Capability = VerificationDimension{Status: "inconclusive", Summary: "检测未完整完成，请查看请求状态后重试；失败请求不计为答错。"}
	wrong := successful - report.Passed
	if wrong > 0 {
		report.Capability = VerificationDimension{Status: "failed", Summary: fmt.Sprintf("%d 项已完成题目未通过，请核对逐项回答；不据此断言模型被替换。", wrong)}
	} else if successful == report.Total {
		report.Capability = VerificationDimension{Status: "passed", Summary: "本次计算、逻辑、状态推演、信息提取与指令测试全部通过；不代表所有任务的能力保证。"}
	}
	modelStatus, modelDetail := "unknown", "上游未返回型号，无法核对声明。"
	if len(models) > 0 {
		modelStatus, modelDetail = "consistent", "返回型号与请求名称一致；该字段由上游报告，不能作为身份证明。"
		for _, model := range models {
			if model != input.Model {
				modelStatus, modelDetail = "warning", "返回型号与请求名称不同，可能存在别名映射或模型替换，需要核对。"
			}
		}
	}
	if len(models) > 1 {
		modelStatus, modelDetail = "warning", "同一次检测返回多个型号，可能存在混合路由或别名变化，需要核对。"
	}
	report.Evidence = append(report.Evidence, VerificationEvidence{ID: "model", Label: "返回型号", Status: modelStatus, Detail: modelDetail})
	report.Evidence = append(report.Evidence, verificationProtocolEvidence(report.Probes), verificationRepeatEvidence(report.Probes))
	report.Evidence = append(report.Evidence, VerificationEvidence{ID: "identity_limit", Label: "证据边界", Status: "unknown", Detail: "没有可信的同版本官方对照时，不能确认具体型号；签名字段存在、自报身份和答题通过均不是身份认证。"})
	verdict := "INCONCLUSIVE"
	if successful == 0 {
		verdict = "ERROR"
	} else if wrong > 0 {
		verdict = "ANOMALY"
	} else if successful == report.Total {
		verdict = "NO_ANOMALIES"
	}
	raw, _ := json.Marshal(report)
	result := map[string]any{}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	result["checker"], result["verdict"] = "verification", verdict
	result["account_id"], result["account_name"], result["claimed_model"] = input.AccountID, input.AccountName, input.Model
	result["response_models"] = models
	result["requests"] = map[string]any{"successful": successful, "total": len(report.Probes), "planned": report.Total}
	result["elapsed_seconds"] = time.Since(started).Seconds()
	result["production_path_equivalent"] = false
	result["source"] = source
	if credential.oauth != nil {
		result["transport"] = "oauth-direct"
	}
	return result, nil
}

func officialVerificationEndpoint(raw, platform string) bool {
	endpoint, err := url.Parse(raw)
	if err != nil || endpoint.Scheme != "https" || endpoint.User != nil || (endpoint.Port() != "" && endpoint.Port() != "443") {
		return false
	}
	switch platform {
	case "anthropic", "claude":
		return endpoint.Hostname() == "api.anthropic.com"
	case "gemini":
		return endpoint.Hostname() == "generativelanguage.googleapis.com"
	default:
		return endpoint.Hostname() == "api.openai.com"
	}
}

func verificationErrorCode(message string, ctx context.Context, status int) string {
	if ctx.Err() != nil {
		return "interrupted"
	}
	switch {
	case status == 429:
		return "rate_limited"
	case status == 401, status == 403:
		return "unauthorized"
	case status == 400 || status == 404 || status == 405 || status == 422:
		return "unsupported_request"
	case strings.Contains(message, "有效文本"), strings.Contains(message, "空回复"):
		return "empty_response"
	case strings.Contains(message, "JSON"), strings.Contains(message, "尾随数据"):
		return "invalid_response"
	case strings.Contains(message, "超时"):
		return "timeout"
	case strings.Contains(message, "截断"), strings.Contains(message, "未正常结束"), strings.Contains(message, "未完整"):
		return "incomplete"
	case strings.Contains(message, "敏感"):
		return "sensitive_response"
	default:
		return "request_failed"
	}
}

func verificationErrorAdvice(code string) string {
	switch code {
	case "rate_limited":
		return "；已停止本组合，请等待限流解除后重新检测。"
	case "unauthorized":
		return "；已停止本组合，请检查授权、余额及模型权限后重新检测。"
	case "unsupported_request":
		return "；已停止本组合，请核对接口、模型及 medium 思考等级是否受支持。"
	case "timeout":
		return "；请检查接口状态或调整请求超时后重新检测。"
	case "empty_response", "invalid_response", "incomplete":
		return "；本项未计为答错，请检查接口完整输出后重新检测。"
	case "interrupted":
		return "；已保留完成的检测证据。"
	default:
		return ""
	}
}
