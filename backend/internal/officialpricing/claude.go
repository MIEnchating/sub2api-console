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
	return enrichModelPrices(ctx, prices, func(ctx context.Context, p Price) ([]Price, error) {
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
	seen := map[string]Price{}
	tierRows := map[string][]Tier{}
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
		condition, label, err := claudeContextCondition(row[0], name[1], name[2])
		if err != nil {
			return nil, err
		}
		if condition != "" {
			if _, exists := seen[model]; exists && len(tierRows[model]) == 0 {
				return nil, errors.New("Claude 官方模型混合了标准与上下文价格")
			}
			tierRows[model] = append(tierRows[model], Tier{Label: label, Condition: condition, Rates: p.Rates, CacheWrite1hPrice: p.CacheWrite1hPrice})
			if _, exists := seen[model]; exists {
				continue
			}
		}
		if condition == "" && len(tierRows[model]) > 0 {
			return nil, errors.New("Claude 官方模型混合了标准与上下文价格")
		}
		if previous, exists := seen[model]; exists {
			if previous.InputPrice != p.InputPrice || previous.OutputPrice != p.OutputPrice || previous.CacheWritePrice != p.CacheWritePrice || previous.CacheWrite1hPrice != p.CacheWrite1hPrice || previous.CacheReadPrice != p.CacheReadPrice {
				return nil, errors.New("Claude 官方模型价格重复且数值冲突")
			}
			continue
		}
		// cc and cc1h count disjoint cache-write tokens; a request can contain both.
		p.BillingExpr = `tier("标准", ` + ratesExpr(p.Rates) + " + cc1h * " + perMillion(p.CacheWrite1hPrice) + ")"
		seen[model] = p
		prices = append(prices, p)
	}
	if len(prices) == 0 {
		return nil, errors.New("Claude 官方模型价格缺失")
	}
	for i := range prices {
		if tiers, ok := tierRows[prices[i].Model]; ok {
			if err := applyClaudeContextTiers(&prices[i], tiers); err != nil {
				return nil, err
			}
		}
	}
	return prices, nil
}

var claudeContext = regexp.MustCompile(`^\(for prompts (up to|over) ([0-9]+|[0-9]{1,3}(?:,[0-9]{3})+) tokens\)$`)
var claudeAvailability = regexp.MustCompile(`^\(\[limited availability\]\(https://[^ )]+\)\)$`)

func claudeContextCondition(row, family, version string) (string, string, error) {
	prefix := "Claude " + family + " " + version
	modifier := strings.TrimSpace(strings.TrimPrefix(row, prefix))
	if modifier == "" || claudeAvailability.MatchString(modifier) {
		return "", "", nil
	}
	m := claudeContext.FindStringSubmatch(modifier)
	if m == nil {
		return "", "", errors.New("Claude 官方上下文价格条件已变更")
	}
	threshold := strings.ReplaceAll(m[2], ",", "")
	if threshold != "100000" {
		return "", "", errors.New("Claude 官方上下文价格阈值已变更")
	}
	if m[1] == "up to" {
		return "len <= 100000", "输入 ≤ 100000 Token", nil
	}
	return "len > 100000", "输入 > 100000 Token", nil
}

func applyClaudeContextTiers(price *Price, tiers []Tier) error {
	if len(tiers) != 2 {
		return errors.New("Claude 官方上下文价格档位缺失")
	}
	var lower, upper *Tier
	for i := range tiers {
		switch tiers[i].Condition {
		case "len <= 100000":
			lower = &tiers[i]
		case "len > 100000":
			upper = &tiers[i]
		default:
			return errors.New("Claude 官方上下文价格条件已变更")
		}
	}
	if lower == nil || upper == nil {
		return errors.New("Claude 官方上下文价格档位缺失")
	}
	price.Tiers = []Tier{*lower, *upper}
	price.Scope += "；输入超过 100000 Token 时整单按长上下文计费"
	price.InputPrice, price.OutputPrice = lower.InputPrice, lower.OutputPrice
	price.CacheReadPrice, price.CacheWritePrice = lower.CacheReadPrice, lower.CacheWritePrice
	price.CacheWrite1hPrice = lower.CacheWrite1hPrice
	tierExpr := func(t *Tier) string {
		return `tier("` + t.Label + `", ` + ratesExpr(t.Rates) + " + cc1h * " + perMillion(t.CacheWrite1hPrice) + ")"
	}
	price.BillingExpr = "len <= 100000 ? " + tierExpr(lower) + " : " + tierExpr(upper)
	return nil
}
