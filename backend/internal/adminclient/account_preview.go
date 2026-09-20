package adminclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/redact"
)

type AccountPreviewRequest struct {
	ModelID         string `json:"model_id"`
	Prompt          string `json:"prompt"`
	ReasoningEffort string `json:"reasoning_effort"`
	RequestID       string `json:"request_id"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
}

type AccountPreviewResult struct {
	AccountID json.Number `json:"account_id"`
	RequestID string      `json:"request_id"`
	Model     string      `json:"model"`
	Text      string      `json:"text"`
}

// GenerateAccountPreview never falls back to the legacy account test endpoint:
// that endpoint can recover accounts or mark them unhealthy. Do not replay a
// generation on transport failures or on ambiguous response bodies.
func (c *Client) GenerateAccountPreview(ctx context.Context, accountID string, input AccountPreviewRequest) (*AccountPreviewResult, error) {
	if !stableID(accountID) || input.TimeoutSeconds < 1 || input.TimeoutSeconds > 120 || strings.TrimSpace(input.RequestID) == "" || strings.TrimSpace(input.ModelID) == "" || strings.TrimSpace(input.Prompt) == "" {
		return nil, errors.New("生成预览账号或请求参数无效")
	}
	if err := AuthorizeMutation(ctx); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(input.TimeoutSeconds)*time.Second)
	defer cancel()
	body, err := json.Marshal(input)
	if err != nil {
		return nil, errors.New("生成预览请求编码失败")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/admin/accounts/"+accountID+"/generate-preview", bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("生成预览请求创建失败，请检查管理地址")
	}
	c.headers(request)
	request.Header.Set("X-Request-ID", input.RequestID)
	client := *c.http
	client.Timeout = time.Duration(input.TimeoutSeconds) * time.Second
	response, err := client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("生成预览已取消或超时：%w", ctx.Err())
		}
		return nil, errors.New("生成预览请求传输失败，请检查管理连接后重试")
	}
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusMethodNotAllowed {
		_ = response.Body.Close()
		return nil, errors.New("管理端未提供账号生成预览接口或账号已不存在，请升级 Sub2API 并刷新账号后重试")
	}
	payload, err, _ := decodeResponse(response)
	if err != nil {
		return nil, errors.New(redact.Secrets(strings.ReplaceAll(err.Error(), c.adminKey, "[已隐藏]")))
	}
	data, err := responseObject(payload, "生成预览")
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, errors.New("生成预览结果编码无效")
	}
	var result AccountPreviewResult
	if err := json.Unmarshal(raw, &result); err != nil || result.AccountID.String() != accountID || result.RequestID != input.RequestID || strings.TrimSpace(result.Text) == "" || len(result.Model) > 256 {
		return nil, errors.New("生成预览结果缺失或账号、请求 ID 不匹配，请重新检测")
	}
	if strings.Contains(result.Text, c.adminKey) || strings.Contains(result.Model, c.adminKey) {
		return nil, errors.New("生成预览结果包含敏感信息，已拒绝展示")
	}
	return &result, nil
}
