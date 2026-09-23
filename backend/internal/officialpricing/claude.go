package officialpricing

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
)

const ClaudeURL = "https://platform.claude.com/docs/en/about-claude/pricing"
const claudeModelsURL = "https://platform.claude.com/docs/en/models/"

var claudeDisplayName = regexp.MustCompile(`^Claude ([A-Za-z]+) ([0-9]+(?:\.[0-9]+)?)(?: \(|$)`)
var claudeModelID = regexp.MustCompile("(?m)^Model ID: `(claude-[a-z0-9-]+)`$")
var claudeAlias = regexp.MustCompile("(?m)^\\| Claude API alias\\s*\\| `(claude-[a-z0-9-]+)`\\s*\\|")

func fetchClaude(ctx context.Context, client *http.Client) ([]Price, error) {
	raw, err := fetchRetry(ctx, client, ClaudeURL+".md")
	if err != nil {
		return nil, err
	}
	prices, err := parseClaude(raw)
	if err != nil {
		return nil, err
	}
	return enrichModelPrices(ctx, prices, func(p Price) ([]Price, error) {
		raw, err := fetchRetry(ctx, client, claudeModelsURL+strings.TrimPrefix(p.Model, "claude-")+"/overview.md")
		if err != nil {
			return nil, err
		}
		id := claudeModelID.FindStringSubmatch(string(raw))
		if id == nil || (id[1] != p.Model && !strings.HasPrefix(id[1], p.Model+"-")) {
			return nil, errors.New("Claude 官方模型 ID 未确认")
		}
		alias := claudeAlias.FindStringSubmatch(string(raw))
		if alias != nil && alias[1] != p.Model {
			return nil, errors.New("Claude 官方兼容别名与模型不一致")
		}
		p.Model = id[1]
		result := []Price{p}
		if alias != nil && alias[1] != id[1] {
			p.Model = alias[1]
			result = append(result, p)
		}
		return result, nil
	})
}

func parseClaude(raw []byte) ([]Price, error) {
	section, err := markdownSection(string(raw), "## Model pricing")
	if err != nil {
		return nil, err
	}
	rows, err := firstPriceTable(section, []string{"Model", "Base input tokens", "5m cache writes", "1h cache writes", "Cache hits and refreshes", "Output tokens"})
	if err != nil {
		return nil, err
	}
	var prices []Price
	seen := map[string]bool{}
	for _, row := range rows {
		// Partner-only retired models have independent cloud pricing, not a live
		// first-party reference for this console.
		if strings.Contains(row[0], "[retired") {
			continue
		}
		name := claudeDisplayName.FindStringSubmatch(row[0])
		if name == nil {
			return nil, errors.New("Claude 官方模型名称格式已变更")
		}
		model := "claude-" + strings.ToLower(name[1]) + "-" + strings.ReplaceAll(name[2], ".", "-")
		if seen[model] {
			return nil, errors.New("Claude 官方模型价格重复")
		}
		seen[model] = true
		values := make([]string, 5)
		for i := range values {
			cell := strings.TrimSpace(priceFootnote.ReplaceAllString(row[i+1], ""))
			if !strings.HasSuffix(cell, " / MTok") {
				return nil, errors.New("Claude 官方计价单位已变更")
			}
			values[i], err = dollarRate(strings.TrimSuffix(cell, " / MTok"), true)
			if err != nil || values[i] == "" {
				return nil, errors.New("Claude 官方单价不完整或无效")
			}
		}
		p := Price{Model: model, Scope: "USD；Claude 标准 API／全球路由；缓存写入分别按 5 分钟与 1 小时计费", CacheWrite1hPrice: values[2], Rates: Rates{InputPrice: values[0], CacheWritePrice: values[1], CacheReadPrice: values[3], OutputPrice: values[4]}}
		// cc and cc1h count disjoint cache-write tokens; a request can contain both.
		p.BillingExpr = `tier("标准", ` + ratesExpr(p.Rates) + " + cc1h * " + perMillion(p.CacheWrite1hPrice) + ")"
		prices = append(prices, p)
	}
	if len(prices) == 0 {
		return nil, errors.New("Claude 官方模型价格缺失")
	}
	return prices, nil
}
