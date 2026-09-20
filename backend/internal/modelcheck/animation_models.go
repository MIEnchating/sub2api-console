package modelcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type animationModelsPage struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
	HasMore bool            `json:"has_more"`
	LastID  string          `json:"last_id"`
	Error   json.RawMessage `json:"error"`
	Success *bool           `json:"success"`
}

func (s *Service) CustomAnimationModels(ctx context.Context, input AnimationCustomEndpoint) ([]string, error) {
	custom, err := validateAnimationEndpoint(input)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	endpoint, err := directEndpoint(custom.BaseURL, "/v1/models")
	if err != nil {
		return nil, errors.New("Base URL 无效，请检查地址后重试")
	}
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	models := []string{}
	seen := map[string]bool{}
	cursors := map[string]bool{}
	cursor := ""
	for pageIndex := 0; pageIndex < 20; pageIndex++ {
		pageURL, _ := url.Parse(endpoint)
		if custom.Platform == "anthropic" || cursor != "" {
			query := pageURL.Query()
			query.Set("limit", "100")
			if cursor != "" {
				cursorKey := "after"
				if custom.Platform == "anthropic" {
					cursorKey = "after_id"
				}
				query.Set(cursorKey, cursor)
			}
			pageURL.RawQuery = query.Encode()
		}
		page, err := readAnimationModelsPage(ctx, client, custom, pageURL.String())
		if err != nil {
			return nil, err
		}
		for _, item := range page.Data {
			id := strings.TrimSpace(item.ID)
			if id == "" || utf8.RuneCountInString(id) > 256 || strings.ContainsFunc(id, unicode.IsControl) || strings.Contains(id, custom.APIKey) {
				return nil, errors.New("上游返回了无效或包含敏感信息的模型 ID，请检查模型列表接口")
			}
			if !seen[id] {
				seen[id] = true
				models = append(models, id)
			}
		}
		if !page.HasMore {
			sort.Strings(models)
			return models, nil
		}
		cursor = strings.TrimSpace(page.LastID)
		if cursor == "" || len(cursor) > 1024 || cursors[cursor] || len(page.Data) == 0 {
			return nil, errors.New("上游模型列表分页信息无效，请检查接口后重试")
		}
		cursors[cursor] = true
	}
	return nil, errors.New("上游模型列表超过 20 页，请手动输入模型 ID")
}

func readAnimationModelsPage(ctx context.Context, client *http.Client, custom AnimationCustomEndpoint, endpoint string) (animationModelsPage, error) {
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(200 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return animationModelsPage{}, safeTransportError(ctx.Err())
			case <-timer.C:
			}
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return animationModelsPage{}, errors.New("模型列表请求创建失败，请检查地址后重试")
		}
		request.Header.Set("Accept", "application/json")
		if custom.Platform == "anthropic" {
			request.Header.Set("x-api-key", custom.APIKey)
			request.Header.Set("anthropic-version", "2023-06-01")
		} else {
			request.Header.Set("Authorization", "Bearer "+custom.APIKey)
		}
		response, err := client.Do(request)
		if err != nil {
			if attempt == 0 && retryAnimationModelsRead(ctx, err) {
				continue
			}
			return animationModelsPage{}, safeTransportError(err)
		}
		raw, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
		_ = response.Body.Close()
		if err != nil {
			if attempt == 0 && retryAnimationModelsRead(ctx, err) {
				continue
			}
			return animationModelsPage{}, safeTransportError(err)
		}
		if len(raw) > 2<<20 {
			return animationModelsPage{}, errors.New("上游模型列表响应过大，请手动输入模型 ID")
		}
		if attempt == 0 && (response.StatusCode == 408 || response.StatusCode == 429 || response.StatusCode == 500 || response.StatusCode == 502 || response.StatusCode == 503 || response.StatusCode == 504) {
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return animationModelsPage{}, directStatusError(response.StatusCode, raw, custom.APIKey)
		}
		var page animationModelsPage
		if json.Unmarshal(raw, &page) != nil {
			return page, errors.New("上游模型列表不是有效 JSON，请检查接口后重试")
		}
		if len(page.Error) > 0 && !bytes.Equal(bytes.TrimSpace(page.Error), []byte("null")) || page.Success != nil && !*page.Success {
			return page, errors.New("上游拒绝读取模型列表，请检查 Key、权限或余额后重试")
		}
		if page.Data == nil {
			return page, errors.New("上游响应缺少模型列表，请检查接口后重试")
		}
		return page, nil
	}
	return animationModelsPage{}, errors.New("模型列表暂不可用，请稍后重试")
}

func retryAnimationModelsRead(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	var networkError net.Error
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		(errors.As(err, &networkError) && networkError.Timeout())
}
