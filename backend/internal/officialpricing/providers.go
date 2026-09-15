package officialpricing

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

type Provider struct{ ID, Name, URL string }

var Providers = []Provider{{"deepseek", "DeepSeek", DeepSeekURL}, {"kimi", "Kimi", KimiURL}, {"minimax", "MiniMax", MiniMaxURL}, {"glm", "GLM", GLMURL}, {"qwen", "Qwen", QwenURL}}

func ProviderID(model string) string {
	model = strings.ToLower(model)
	for _, p := range []struct{ prefix, id string }{{"deepseek-", "deepseek"}, {"kimi-", "kimi"}, {"moonshot-", "kimi"}, {"minimax-", "minimax"}, {"glm-", "glm"}, {"qwen", "qwen"}, {"qwq-", "qwen"}, {"qvq-", "qwen"}} {
		if strings.HasPrefix(model, p.prefix) {
			return p.id
		}
	}
	return ""
}
func Fetch(ctx context.Context, client *http.Client, provider Provider) ([]Price, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var ps []Price
	var err error
	switch provider.ID {
	case "deepseek":
		ps, err = FetchDeepSeek(ctx, client)
	default:
		var raw []byte
		raw, err = fetchRetry(ctx, client, provider.URL+markdownSuffix(provider.ID))
		if err == nil {
			switch provider.ID {
			case "kimi":
				ps, err = ParseKimi(raw)
			case "minimax":
				ps, err = ParseMiniMax(raw)
			case "glm":
				ps, err = ParseGLM(raw)
			case "qwen":
				ps, err = ParseQwen(raw)
			default:
				err = errors.New("不支持的官方价格来源")
			}
		}
	}
	if err == nil {
		for i := range ps {
			if ps[i].SourceURL == "" {
				ps[i].SourceURL = provider.URL
			}
		}
	}
	return ps, err
}
func markdownSuffix(id string) string {
	if id == "kimi" || id == "minimax" || id == "glm" {
		return ".md"
	}
	return ""
}
func fetchRetry(ctx context.Context, client *http.Client, address string) ([]byte, error) {
	raw, err := fetchURL(ctx, client, address)
	if err == nil {
		return raw, nil
	}
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
	}
	return fetchURL(ctx, client, address)
}
