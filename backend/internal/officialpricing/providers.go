package officialpricing

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"sync"
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
	case "kimi":
		ps, err = fetchKimi(ctx, client)
	default:
		var raw []byte
		raw, err = fetchRetry(ctx, client, provider.URL+markdownSuffix(provider.ID))
		if err == nil {
			switch provider.ID {
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
	if id == "minimax" || id == "glm" {
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

var kimiLink = regexp.MustCompile(`https://platform\.kimi\.com/docs/pricing/chat-[a-z0-9-]+\.md`)

func fetchKimi(ctx context.Context, client *http.Client) ([]Price, error) {
	index, err := fetchRetry(ctx, client, "https://platform.kimi.com/docs/llms.txt")
	if err != nil {
		return nil, err
	}
	urls := []string{}
	seen := map[string]bool{}
	for _, url := range kimiLink.FindAllString(string(index), -1) {
		if !seen[url] {
			urls = append(urls, url)
			seen[url] = true
		}
	}
	if len(urls) == 0 || len(urls) > 16 {
		return nil, errors.New("Kimi 官方模型定价目录已变更")
	}
	type result struct {
		ps  []Price
		err error
	}
	results := make([]result, len(urls))
	var wg sync.WaitGroup
	// The index is bounded, and no discovered URL can escape the official prefix.
	for i, url := range urls {
		wg.Go(func() {
			raw, e := fetchRetry(ctx, client, url)
			if e != nil {
				results[i].err = e
				return
			}
			ps, e := ParseKimi(raw)
			for j := range ps {
				ps[j].SourceURL = strings.TrimSuffix(url, ".md")
			}
			results[i] = result{ps, e}
		})
	}
	wg.Wait()
	var ps []Price
	for _, r := range results {
		if r.err != nil {
			return nil, r.err
		}
		ps = append(ps, r.ps...)
	}
	return ps, nil
}
