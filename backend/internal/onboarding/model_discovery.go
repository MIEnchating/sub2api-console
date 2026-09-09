package onboarding

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
)

func discoverOnboardingModels(ctx context.Context, client *adminclient.Client, platform, accountType, baseURL, secret string) ([]string, error) {
	models, previewErr := client.PreviewAccountModels(ctx, platform, accountType, baseURL, secret)
	if previewErr == nil {
		return models, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Keep any gateway prefix, but do not append a second /v1 to an API base URL.
	fallbackBaseURL := strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/v1")
	models, fallbackErr := fetchProbeModels(ctx, fallbackBaseURL, secret)
	if fallbackErr == nil && len(models) == 0 {
		fallbackErr = errors.New("上游模型接口未返回可用模型")
	}
	if fallbackErr != nil {
		return nil, fmt.Errorf("管理接口同步失败：%v；/v1/models 兜底失败：%w", previewErr, fallbackErr)
	}
	return models, nil
}
