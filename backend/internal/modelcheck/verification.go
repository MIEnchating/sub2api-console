package modelcheck

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelfingerprint"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

func runVerification(ctx context.Context, directClient, oauthClient *http.Client, credential credentialResolution, input targetRequest) (map[string]any, error) {
	publicIncompatibility := ""
	if input.PublicReference {
		if credential.oauth != nil {
			publicIncompatibility = "当前 OAuth 请求不能复现公开参考要求的温度、系统提示与输出上限；未发送额外参考请求。请使用支持这些参数的 API 账号。"
		} else {
			input.publicPlan = modelfingerprint.Plan(input.Model)
		}
	}
	report := VerificationReport{Version: verificationVersion, Evidence: []VerificationEvidence{}, Probes: []VerificationProbe{}}
	report.Total = verificationProbeCount(input.Rounds, input.Fingerprint, input.BehaviorFingerprint) + len(input.publicPlan)
	report.Authenticity = VerificationDimension{Status: "inconclusive", Summary: "检测进行中，等待有效响应与来源证据。"}
	source := "third_party"
	if credential.oauth != nil {
		source = "official_oauth"
	} else if officialVerificationEndpoint(credential.value.BaseURL, credential.value.Platform) {
		source = "official_endpoint"
	}
	report.Evidence = append(report.Evidence, VerificationEvidence{ID: "source", Label: "请求来源", Status: "info", Detail: map[string]string{"third_party": "第三方接口；请求路径由账号配置决定。", "official_endpoint": "官方 API 地址；使用 HTTPS 且禁止重定向。", "official_oauth": "固定官方 OAuth 端点；未经过生产中转路径。"}[source]})
	successful, capabilitySuccessful := 0, 0
	report.CapabilityTotal = input.Rounds * 7
	report.PublicReference = publicReferenceReport(input, nil, publicIncompatibility)
	models := []string{}
	started := time.Now()
	for round, challenges := range input.Challenges {
		for _, challenge := range verificationPlannedRound(challenges, round+1, input) {
			report.ProbePlan = append(report.ProbePlan, VerificationProbe{ID: challenge.id, Label: challenge.label, Round: round + 1, Prompt: challenge.prompt, Expected: challenge.expected, Status: "pending", Category: verificationCategory(challenge)})
		}
	}
	if err := runVerificationProbes(ctx, directClient, oauthClient, credential, input, &report); err != nil {
		return nil, err
	}
	for _, row := range report.Probes {
		if row.Status == "error" {
			continue
		}
		successful++
		if row.ResponseModel != "" && !slices.Contains(models, row.ResponseModel) {
			models = append(models, row.ResponseModel)
		}
		if row.Category == "capability" {
			capabilitySuccessful++
		}
	}
	report.Total = verificationProbeCount(input.Rounds, input.Fingerprint, input.BehaviorFingerprint) + len(input.publicPlan)
	report.Capability = VerificationDimension{Status: "inconclusive", Summary: "检测未完整完成，请查看请求状态后重试；失败请求不计为答错。"}
	wrong := capabilitySuccessful - report.Passed
	if wrong > 0 {
		report.Capability = VerificationDimension{Status: "failed", Summary: fmt.Sprintf("%d 项已完成题目未通过，请核对逐项回答；不据此断言模型被替换。", wrong)}
	} else if capabilitySuccessful == report.CapabilityTotal {
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
	report.Evidence = append(report.Evidence, VerificationEvidence{ID: "identity_limit", Label: "证据边界", Status: "unknown", Detail: "官方来源只确认本次请求路径；能力与指纹检查不能证明具体模型权重。签名字段存在、自报身份和答题通过均不是身份认证。"})
	report.Authenticity = verificationAuthenticity(source, successful, modelStatus)
	report.BehaviorFingerprint = verificationBehaviorReport(report.Probes, input.BehaviorFingerprint)
	if report.BehaviorFingerprint != nil {
		report.BehaviorFingerprint.Parameters = BehaviorSamplingParameters{Stream: true, Temperature: "provider_default", ReasoningEffort: "medium", MaxOutputTokens: 4096}
		report.BehaviorFingerprint.Profile += ":" + credential.value.Platform
		if credential.value.Platform == "gemini" {
			report.BehaviorFingerprint.Parameters.ReasoningEffort = "provider_default"
		}
		if credential.oauth != nil {
			report.BehaviorFingerprint.Profile = "verification-medium-v1:oauth"
			report.BehaviorFingerprint.Parameters.MaxOutputTokens = 0
			report.BehaviorFingerprint.Parameters.Instructions = oauthCheckInstructions
		}
	}
	report.PublicReference = publicReferenceReport(input, report.Probes, publicIncompatibility)
	report.Fingerprint = verificationFingerprintReport(report.Probes, input.Fingerprint)
	report.Evidence = append(report.Evidence, fingerprintEvidence(report.Fingerprint))
	verdict := "INCONCLUSIVE"
	if successful == 0 {
		verdict = "ERROR"
	} else if wrong > 0 {
		verdict = "ANOMALY"
	} else if successful == report.Total {
		verdict = "NO_ANOMALIES"
	}
	if report.PublicReference != nil && successful > 0 {
		if report.PublicReference.Status == "mismatch" {
			verdict = "ANOMALY"
		} else if report.PublicReference.Status != "matched" && verdict == "NO_ANOMALIES" {
			verdict = "INCONCLUSIVE"
		}
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

func verificationRound(challenges []verificationChallenge) []verificationChallenge {
	result := slices.Clone(challenges)
	// Reuse one question to observe repeatability, not to authenticate identity.
	repeated := challenges[0]
	repeated.id, repeated.label = "consistency", "重复请求一致性"
	return append(result, repeated)
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
		return "；已停止本组合，请核对接口、模型及本项检测参数是否受支持。"
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

func verificationPlannedRound(challenges []verificationChallenge, round int, input targetRequest) []verificationChallenge {
	result := verificationRound(challenges)
	if input.Fingerprint && round == input.Rounds {
		result = append(result, verificationFingerprintChallenges()...)
	}
	if input.BehaviorFingerprint && round == input.Rounds {
		result = append(result, verificationBehaviorChallenges()...)
	}
	if input.PublicReference && round == input.Rounds {
		result = append(result, publicReferenceChallenges(input.publicPlan)...)
	}
	return result
}
func verificationCategory(challenge verificationChallenge) string {
	if challenge.public != nil {
		return "public_reference"
	}
	if challenge.behavior {
		return "behavior_fingerprint"
	}
	if challenge.measurement {
		return "fingerprint"
	}
	return "capability"
}
