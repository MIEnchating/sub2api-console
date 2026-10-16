package onboarding

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

type probeVerificationReporterKey struct{}
type probeVerificationReporter func(map[string]any, int, int)

type probeVerificationSnapshot struct {
	modelcheck.VerificationReport
	Checker        string `json:"checker"`
	AccountName    string `json:"account_name"`
	Model          string `json:"claimed_model"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

func (s *Service) runProbeVerification(ctx context.Context, credential probeCredential, model string, behavior bool, reference ...bool) (ProbeResult, error) {
	result := ProbeResult{Status: "failed", RequestModel: model}
	if s.animations == nil {
		return result, errors.New("模型检测服务尚未就绪，请稍后重试")
	}
	platform := "openai"
	if credential.candidate.Platform != nil {
		switch strings.ToLower(strings.TrimSpace(*credential.candidate.Platform)) {
		case "anthropic", "claude":
			platform = "anthropic"
		case "gemini", "google":
			platform = "gemini"
		}
	}
	started := time.Now()
	check, err := s.animations.RunEndpointVerification(ctx, modelcheck.AnimationCustomEndpoint{
		BaseURL: credential.auth.BaseURL, APIKey: credential.key.Secret, Platform: platform, Model: model,
	}, func(report modelcheck.VerificationReport) {
		if publish, ok := ctx.Value(probeVerificationReporterKey{}).(probeVerificationReporter); ok {
			snapshot := probeVerificationSnapshot{VerificationReport: report, Checker: "verification", AccountName: "接入前探活", Model: model, TimeoutSeconds: 45}
			if public := publicProbeVerification(snapshot, credential.key.Secret); public != nil {
				publish(public, len(report.Probes), report.Total)
			}
		}
	}, behavior, len(reference) > 0 && reference[0])
	result.LatencyMS = time.Since(started).Milliseconds()
	if check != nil {
		result.Verification = publicProbeVerification(check, credential.key.Secret)
		if models, ok := check["response_models"].([]string); ok {
			result.ActualModel = probeCredentialText(strings.Join(models, "、"), credential.key.Secret)
		}
	}
	if err != nil {
		return result, err
	}
	var report modelcheck.VerificationReport
	raw, _ := json.Marshal(result.Verification)
	if err := json.Unmarshal(raw, &report); err != nil {
		return result, errors.New("模型检测报告无法读取，请重新检测")
	}
	result.Message = "模型检测未完整完成，请查看逐项请求状态后重试"
	for _, probe := range report.Probes {
		if probe.Status == "error" {
			result.Message = probe.Error
			return result, nil
		}
	}
	if report.Total > 0 && len(report.Probes) == report.Total {
		result.Status = "completed"
		result.Message = "检测请求已完成；能力结论与指纹证据请查看报告"
	}

	return result, nil
}

func publicProbeVerification(value any, secret string) map[string]any {
	// Normalize typed slices and structs before recursively removing credentials.
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var public map[string]any
	if json.Unmarshal(raw, &public) != nil {
		return nil
	}
	return redactProbeResponse(public, secret).(map[string]any)
}
