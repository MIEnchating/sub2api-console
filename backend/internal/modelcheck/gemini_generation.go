package modelcheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelrequest"
	"net/http"
	"strings"
)

type geminiPart struct {
	Text    string `json:"text"`
	Thought bool   `json:"thought,omitempty"`
}
type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}
type geminiGenerationResponse struct {
	ModelVersion string `json:"modelVersion"`
	Candidates   []struct {
		Content      geminiContent `json:"content"`
		FinishReason string        `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
	Error         json.RawMessage `json:"error"`
	UsageMetadata *struct {
		PromptTokenCount     *json.Number `json:"promptTokenCount"`
		CandidatesTokenCount *json.Number `json:"candidatesTokenCount"`
		ThoughtsTokenCount   *json.Number `json:"thoughtsTokenCount"`
		TotalTokenCount      *json.Number `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

func geminiModelID(model string) (string, error) {
	model = strings.TrimPrefix(strings.TrimSpace(model), "models/")
	if model == "" || strings.ContainsAny(model, "/\\?#%") || model == "." || model == ".." {
		return "", errors.New("Gemini 模型必须是有效模型 ID，请检查模型配置")
	}
	return model, nil
}

func isGeminiGenerationPath(path string) bool {
	return strings.HasPrefix(path, "/v1beta/models/") && (strings.HasSuffix(path, ":streamGenerateContent") || strings.HasSuffix(path, ":generateContent"))
}

func sendGeminiGeneration(ctx context.Context, client *http.Client, credential directCredential, model, prompt, requestID string, budget int, usage *AnimationUsage, onFirstOutput func(), streaming ...bool) (string, string, error) {
	stream := true
	if len(streaming) > 0 {
		stream = streaming[0]
	}
	parameters := modelrequest.Parameters{Model: model, Prompt: prompt, MaxOutputTokens: budget, Stream: stream}
	text, actual, status, raw, header, err := animationHTTP(ctx, client, credential, modelrequest.Gemini, parameters, requestID, usage, onFirstOutput)
	if err != nil {
		return "", "", animationVisibleError(err, credential.Secret)
	}
	if status < 200 || status >= 300 {
		return "", "", animationRetryableStatusError(status, header, raw, directStatusError(status, raw, credential.Secret))
	}
	if strings.TrimSpace(text) == "" {
		return "", "", errors.New("Gemini 未返回有效正文，请检查模型或安全限制后重试")
	}
	return text, safeCredentialText(strings.ReplaceAll(actual, credential.Secret, "[已隐藏]")), nil
}

func geminiGenerationEvent(raw []byte, usage *AnimationUsage) (string, string, bool, error) {
	var response geminiGenerationResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return "", "", false, errors.New("Gemini 返回了无效的生成响应，请检查接口后重试")
	}
	if len(response.Error) > 0 && string(response.Error) != "null" {
		return "", "", false, fmt.Errorf("Gemini 生成失败：%s", responseErrorDetail(raw))
	}
	if reason := response.PromptFeedback.BlockReason; reason != "" && reason != "BLOCK_REASON_UNSPECIFIED" {
		return "", "", false, fmt.Errorf("Gemini 拦截了生成请求（%s），请调整提示词或模型后重试", reason)
	}
	if len(response.Candidates) > 1 {
		return "", "", false, errors.New("Gemini 返回多个候选结果，请检查接口配置")
	}
	if usage != nil && response.UsageMetadata != nil {
		usage.gemini = true
		values := response.UsageMetadata
		for _, field := range []struct {
			source *json.Number
			target **int64
		}{
			{values.PromptTokenCount, &usage.InputTokens}, {values.CandidatesTokenCount, &usage.OutputTokens},
			{values.ThoughtsTokenCount, &usage.ReasoningTokens}, {values.TotalTokenCount, &usage.TotalTokens},
		} {
			if field.source != nil {
				*field.target = animationTokenCount(*field.source)
			}
		}
	}
	if len(response.Candidates) == 0 {
		return "", response.ModelVersion, false, nil
	}
	candidate := response.Candidates[0]
	if candidate.FinishReason != "" && candidate.FinishReason != "FINISH_REASON_UNSPECIFIED" && candidate.FinishReason != "STOP" {
		return "", "", false, fmt.Errorf("Gemini 生成被截断或拦截（%s），请调整提示词或模型后重试", candidate.FinishReason)
	}
	var output strings.Builder
	for _, part := range candidate.Content.Parts {
		if !part.Thought {
			output.WriteString(part.Text)
		}
	}
	return output.String(), response.ModelVersion, candidate.FinishReason == "STOP", nil
}
