package onboarding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/redact"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

const onboardingProbeResponseLimit = 4 << 20

type ProbeResult struct {
	Status       string `json:"status"`
	Message      string `json:"message"`
	RequestModel string `json:"request_model"`
	ActualModel  string `json:"actual_model"`
	LatencyMS    int64  `json:"latency_ms"`
	HTTPStatus   int    `json:"http_status"`
	TemporaryKey bool   `json:"temporary_key"`
}

type probeKeyDeleter interface {
	DeleteKey(context.Context, configstore.AuthRecord, string) error
}

type probeCredential struct {
	auth      configstore.AuthRecord
	candidate business.OnboardingCandidate
	key       upstreamsync.CreatedKey
	temporary bool
}

func (s *Service) ProbeModels(ctx context.Context, host, groupID string) ([]string, error) {
	credential, err := s.acquireProbeCredential(ctx, host, groupID)
	if err != nil {
		return nil, err
	}
	models, requestErr := fetchProbeModels(ctx, credential.auth.BaseURL, credential.key.Secret)
	cleanupErr := s.cleanupProbeCredential(credential)
	if requestErr != nil {
		if cleanupErr != nil {
			return nil, fmt.Errorf("%v；临时测试 Key 清理失败：%w", requestErr, cleanupErr)
		}
		return nil, requestErr
	}
	if cleanupErr != nil {
		return nil, fmt.Errorf("模型读取完成，但临时测试 Key 清理失败：%w", cleanupErr)
	}
	if len(models) == 0 {
		return nil, errors.New("上游模型接口未返回可选择的模型")
	}
	return models, nil
}

func (s *Service) Probe(ctx context.Context, host, groupID, model string) (ProbeResult, error) {
	model = strings.TrimSpace(model)
	if model == "" || len(model) > 255 {
		return ProbeResult{}, errors.New("请选择有效的测试模型")
	}
	credential, err := s.acquireProbeCredential(ctx, host, groupID)
	if err != nil {
		return ProbeResult{}, err
	}
	result, requestErr := runGatewayProbe(ctx, credential.auth.BaseURL, credential.key.Secret, model, credential.candidate.Platform)
	result.TemporaryKey = credential.temporary
	cleanupErr := s.cleanupProbeCredential(credential)
	if requestErr != nil {
		if cleanupErr != nil {
			return result, fmt.Errorf("%v；临时测试 Key 清理失败：%w", requestErr, cleanupErr)
		}
		return result, requestErr
	}
	if cleanupErr != nil {
		return result, fmt.Errorf("探活请求完成，但临时测试 Key 清理失败：%w", cleanupErr)
	}
	return result, nil
}

func (s *Service) acquireProbeCredential(ctx context.Context, host, groupID string) (probeCredential, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" || len(groupID) > 255 {
		return probeCredential{}, errors.New("上游分组 ID 无效")
	}
	auth, err := s.private.AuthRecord(ctx, host)
	if err != nil {
		return probeCredential{}, err
	}
	if auth == nil {
		return probeCredential{}, errors.New("探活前必须先配置该 Host 的鉴权记录")
	}
	candidates, err := s.repository.OnboardingCandidates(ctx, auth.Host)
	if err != nil {
		return probeCredential{}, err
	}
	var candidate *business.OnboardingCandidate
	for index := range candidates {
		if candidates[index].GroupID != nil && strings.TrimSpace(*candidates[index].GroupID) == groupID {
			candidate = &candidates[index]
			break
		}
	}
	if candidate == nil {
		return probeCredential{}, errors.New("上游分组不存在或不在 Console 业务库中")
	}
	credential := probeCredential{auth: *auth, candidate: *candidate}
	if candidate.UpstreamKeyID != nil && strings.TrimSpace(*candidate.UpstreamKeyID) != "" {
		credential.key, err = s.keys.RevealKey(ctx, *auth, strings.TrimSpace(*candidate.UpstreamKeyID), groupID)
		if err != nil {
			return probeCredential{}, fmt.Errorf("上游已有 Key 读取失败：%w", err)
		}
		return credential, nil
	}
	if !candidate.CanCreateKey {
		if candidate.UnavailableReason != nil && strings.TrimSpace(*candidate.UnavailableReason) != "" {
			return probeCredential{}, errors.New(strings.TrimSpace(*candidate.UnavailableReason))
		}
		return probeCredential{}, errors.New("该上游分组当前无法创建临时测试 Key")
	}
	if _, ok := s.keys.(probeKeyDeleter); !ok {
		return probeCredential{}, errors.New("当前上游客户端不支持安全清理临时测试 Key")
	}
	id, err := randomID(6)
	if err != nil {
		return probeCredential{}, err
	}
	credential.key, err = createKey(ctx, s.keys, *auth, "console-probe-"+id, groupID, true)
	if err != nil {
		return probeCredential{}, fmt.Errorf("临时测试 Key 创建失败：%w", err)
	}
	credential.temporary = true
	return credential, nil
}

func (s *Service) cleanupProbeCredential(credential probeCredential) error {
	if !credential.temporary {
		return nil
	}
	deleter, ok := s.keys.(probeKeyDeleter)
	if !ok {
		return errors.New("上游客户端不支持删除 Key")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return deleter.DeleteKey(ctx, credential.auth, credential.key.KeyID)
}

func fetchProbeModels(ctx context.Context, baseURL, secret string) ([]string, error) {
	body, status, err := gatewayRequest(ctx, baseURL, secret, http.MethodGet, "/v1/models", nil, nil)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, gatewayStatusError(status, body)
	}
	var payload any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, errors.New("上游模型接口返回不是有效 JSON")
	}
	seen := map[string]struct{}{}
	models := make([]string, 0)
	var walk func(any)
	walk = func(value any) {
		switch item := value.(type) {
		case map[string]any:
			for _, key := range []string{"id", "model_id", "slug"} {
				if text, ok := item[key].(string); ok && strings.TrimSpace(text) != "" {
					model := strings.TrimSpace(text)
					if _, exists := seen[model]; !exists {
						seen[model] = struct{}{}
						models = append(models, model)
					}
					break
				}
			}
			for _, key := range []string{"data", "models", "items"} {
				if child, present := item[key]; present {
					walk(child)
				}
			}
		case []any:
			for _, child := range item {
				walk(child)
			}
		}
	}
	walk(payload)
	sort.Strings(models)
	return models, nil
}

func runGatewayProbe(ctx context.Context, baseURL, secret, model string, platform *string) (ProbeResult, error) {
	started := time.Now()
	path := "/v1/responses"
	body := map[string]any{"model": model, "input": "hi", "max_output_tokens": 16, "stream": false}
	headers := map[string]string{}
	if platform != nil {
		switch strings.ToLower(strings.TrimSpace(*platform)) {
		case "anthropic", "claude":
			path = "/v1/messages"
			body = map[string]any{"model": model, "max_tokens": 16, "messages": []map[string]string{{"role": "user", "content": "hi"}}}
			headers["anthropic-version"] = "2023-06-01"
		case "gemini", "google":
			path = "/v1beta/models/" + url.PathEscape(model) + ":generateContent"
			body = map[string]any{"contents": []map[string]any{{"role": "user", "parts": []map[string]string{{"text": "hi"}}}}}
		}
	}
	raw, status, err := gatewayRequest(ctx, baseURL, secret, http.MethodPost, path, body, headers)
	result := ProbeResult{Status: "failed", Message: "探活请求失败", RequestModel: model, LatencyMS: time.Since(started).Milliseconds(), HTTPStatus: status}
	if err != nil {
		result.Message = err.Error()
		return result, err
	}
	result.ActualModel = responseModel(raw)
	if status < 200 || status >= 300 {
		err := gatewayStatusError(status, raw)
		result.Message = err.Error()
		return result, err
	}
	result.Status = "passed"
	result.Message = "上游已返回成功响应"
	return result, nil
}

func gatewayRequest(ctx context.Context, baseURL, secret, method, path string, payload map[string]any, headers map[string]string) ([]byte, int, error) {
	normalized, err := configstore.ValidateBaseURL(baseURL)
	if err != nil {
		return nil, 0, err
	}
	var reader io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, errors.New("探活请求参数编码失败")
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(normalized, "/")+path, reader)
	if err != nil {
		return nil, 0, errors.New("探活请求创建失败")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(secret))
	request.Header.Set("User-Agent", "Sub2API-Console/1.0")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	client := &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, 0, errors.New("探活请求超时")
		}
		return nil, 0, errors.New("上游网络请求失败")
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, onboardingProbeResponseLimit+1))
	if err != nil || len(raw) > onboardingProbeResponseLimit {
		return nil, response.StatusCode, errors.New("上游响应过大或读取失败")
	}
	return raw, response.StatusCode, nil
}

func gatewayStatusError(status int, raw []byte) error {
	detail := strings.TrimSpace(redact.Secrets(string(raw)))
	if len(detail) > 300 {
		detail = detail[:300]
	}
	if detail == "" {
		detail = "未提供错误详情"
	}
	return fmt.Errorf("上游返回 HTTP %d：%s", status, detail)
}

func responseModel(raw []byte) string {
	var payload any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&payload) != nil {
		return ""
	}
	var find func(any) string
	find = func(value any) string {
		switch item := value.(type) {
		case map[string]any:
			if model, ok := item["model"].(string); ok && strings.TrimSpace(model) != "" {
				return strings.TrimSpace(model)
			}
			for _, child := range item {
				if model := find(child); model != "" {
					return model
				}
			}
		case []any:
			for _, child := range item {
				if model := find(child); model != "" {
					return model
				}
			}
		}
		return ""
	}
	return find(payload)
}
