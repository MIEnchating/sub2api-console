package modelcheck

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelfingerprint"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelrequest"
)

type PublicReferenceParameters struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	Instructions    string   `json:"instructions,omitempty"`
	MaxOutputTokens int      `json:"max_output_tokens"`
	Stream          bool     `json:"stream"`
}

func publicReferenceChallenges(plan []modelfingerprint.Probe) []verificationChallenge {
	result := make([]verificationChallenge, 0, len(plan))
	for _, probe := range plan {
		result = append(result, verificationChallenge{id: probe.ID, label: probe.Label, prompt: probe.Prompt, expected: "与固定版本的公开参考比较，不计入能力得分", measurement: true, public: &probe})
	}
	return result
}
func sendPublicReference(ctx context.Context, client *http.Client, credential directCredential, model string, probe modelfingerprint.Probe, requestID string, usage *AnimationUsage, timeout int) (string, string, error) {
	ctx, cancel := modelrequest.Start(ctx, time.Duration(timeout)*time.Second, false)
	defer cancel()
	credential.FallbackBaseURL = ""
	sender := directBundleSender{client: client, credential: credential, requestID: requestID, usage: usage, allowTokenLimit: probe.Kind == "tokenizer"}
	protocol := modelrequest.Chat
	switch credential.Platform {
	case "claude", "anthropic":
		protocol = modelrequest.Anthropic
	case "gemini":
		protocol = modelrequest.Gemini
	}
	parameters := modelrequest.Parameters{Model: model, Prompt: probe.Prompt, Instructions: probe.Instructions, Temperature: probe.Temperature, MaxOutputTokens: probe.MaxOutputTokens}
	payload, status, raw, err := sender.request(ctx, protocol, parameters)
	if err != nil {
		return "", "", err
	}
	if status < 200 || status >= 300 {
		return "", "", directStatusError(status, raw, credential.Secret)
	}
	responseModel := stringField(payload, "model")
	answer := ""
	switch protocol {
	case modelrequest.Chat:
		answer = openAIChatText(payload)
	case modelrequest.Anthropic:
		answer = anthropicResponseText(payload)
	case modelrequest.Gemini:
		responseModel = stringField(payload, "modelVersion")
		if credential.Secret != "" {
			responseModel = strings.ReplaceAll(responseModel, credential.Secret, "[已隐藏]")
		}
		responseModel = safeCredentialText(responseModel)
		metadata, _ := payload["usageMetadata"].(map[string]any)
		usage.gemini = true
		usage.InputTokens = animationTokenCount(metadata["promptTokenCount"])
		usage.OutputTokens = animationTokenCount(metadata["candidatesTokenCount"])
		usage.TotalTokens = animationTokenCount(metadata["totalTokenCount"])
		usage.ReasoningTokens = animationTokenCount(metadata["thoughtsTokenCount"])
		candidates, _ := payload["candidates"].([]any)
		if len(candidates) > 0 {
			candidate, _ := candidates[0].(map[string]any)
			reason := stringField(candidate, "finishReason")
			if reason != "STOP" && !(probe.Kind == "tokenizer" && reason == "MAX_TOKENS") {
				return "", responseModel, errors.New("参考检测回复被截断或拦截，未完整结束")
			}
			content, _ := candidate["content"].(map[string]any)
			parts, _ := content["parts"].([]any)
			for _, part := range parts {
				p, _ := part.(map[string]any)
				if thought, _ := p["thought"].(bool); !thought {
					answer += stringField(p, "text")
				}
			}
		}
	}
	if probe.Kind == "tokenizer" && strings.TrimSpace(answer) == "" {
		answer = "[仅采集输入用量]"
	}
	return answer, responseModel, nil
}
func publicReferenceReport(input targetRequest, probes []VerificationProbe, incompatibility string) *modelfingerprint.Result {
	if !input.PublicReference {
		return nil
	}
	observations := []modelfingerprint.Observation{}
	for _, p := range probes {
		if p.Category != "public_reference" {
			continue
		}
		var tokens *int64
		if p.Usage != nil && p.Usage.InputTokens != nil {
			v := int64(*p.Usage.InputTokens)
			tokens = &v
		}
		observations = append(observations, modelfingerprint.Observation{ID: p.ID, Answer: p.Answer, InputTokens: tokens, Protocol: p.Protocol, ResponseModel: p.ResponseModel, Error: p.Status == "error", Truncated: p.AnswerTruncated})
	}
	result := modelfingerprint.Evaluate(input.Model, observations, incompatibility)
	return &result
}
func publicReferenceCount(model string, enabled bool) int {
	if !enabled {
		return 0
	}
	return modelfingerprint.RequestCount(model)
}
func verificationInputCount(input targetRequest) int {
	return verificationProbeCount(input.Rounds, input.Fingerprint, input.BehaviorFingerprint) + publicReferenceCount(input.Model, input.PublicReference)
}
