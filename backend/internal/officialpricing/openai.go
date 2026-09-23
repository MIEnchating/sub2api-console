package officialpricing

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
)

const OpenAIURL = "https://developers.openai.com/api/docs/pricing"
const openAIModelsURL = "https://developers.openai.com/api/docs/models/"

var openAIContext = regexp.MustCompile(`(?i)prompts with\s+(?:more than|>)\s*([0-9.]+)\s*([KM]?)\s*input tokens`)
var openAIContextLabel = regexp.MustCompile(`^([a-z0-9.-]+) \(<([0-9.]+)([KM]) context length\)$`)
var openAISnapshot = regexp.MustCompile("(?m)^- `([a-z0-9.-]+)`\\s*$")

func fetchOpenAI(ctx context.Context, client *http.Client) ([]Price, error) {
	raw, err := fetchRetry(ctx, client, OpenAIURL+".md")
	if err != nil {
		return nil, err
	}
	prices, err := parseOpenAI(raw)
	if err != nil {
		return nil, err
	}
	return enrichModelPrices(ctx, prices, func(p Price) ([]Price, error) {
		if len(p.Tiers) == 0 {
			return []Price{p}, nil
		}
		raw, err := fetchRetry(ctx, client, openAIModelsURL+p.Model+".md")
		if err != nil {
			return nil, err
		}
		match := openAIContext.FindStringSubmatch(string(raw))
		if match != nil {
			threshold, err := tokenCount(match[1], match[2])
			if err != nil || threshold == "0" {
				return nil, errors.New("OpenAI 官方上下文阈值无效")
			}
			p.Tiers[0].Condition = "len <= " + threshold
		}
		if p.Tiers[0].Condition == "" {
			return nil, errors.New("OpenAI 官方长上下文计费条件缺失")
		}
		p.Scope += "；标准档条件：" + p.Tiers[0].Condition + "，超出后整单按长上下文计费"
		finished, err := finish(map[string]*Price{p.Model: &p})
		if err != nil {
			return nil, err
		}
		p = finished[0]
		out := []Price{p}
		snapshots, err := markdownSection(string(raw), "## Snapshots")
		if err != nil {
			return nil, err
		}
		seen := map[string]bool{p.Model: true}
		for _, snapshot := range openAISnapshot.FindAllStringSubmatch(snapshots, -1) {
			if snapshot[1] != p.Model && !strings.HasPrefix(snapshot[1], p.Model+"-") {
				return nil, errors.New("OpenAI 官方快照与模型不一致")
			}
			if seen[snapshot[1]] {
				continue
			}
			seen[snapshot[1]] = true
			alias := p
			alias.Model = snapshot[1]
			out = append(out, alias)
		}
		return out, nil
	})
}

func parseOpenAI(raw []byte) ([]Price, error) {
	section, err := markdownSection(string(raw), "### Standard pricing data")
	if err != nil {
		return nil, err
	}
	rows, err := firstPriceTable(section, []string{"Model", "Short context input", "Short context cached input", "Short context cache writes", "Short context output", "Long context input", "Long context cached input", "Long context cache writes", "Long context output"})
	if err != nil {
		return nil, err
	}
	var prices []Price
	seen := map[string]bool{}
	for _, row := range rows {
		model, condition := row[0], ""
		if match := openAIContextLabel.FindStringSubmatch(model); match != nil {
			model = match[1]
			threshold, err := tokenCount(match[2], match[3])
			if err != nil || threshold == "0" {
				return nil, errors.New("OpenAI 官方上下文阈值无效")
			}
			condition = "len < " + threshold
		}
		if !publishedID.MatchString(model) || ProviderID(model) != "openai" || seen[model] {
			return nil, errors.New("OpenAI 官方模型名称无效或重复")
		}
		seen[model] = true
		standard, err := openAIRates(row[1:5])
		if err != nil || standard.InputPrice == "" || standard.OutputPrice == "" {
			return nil, errors.New("OpenAI 官方标准单价不完整")
		}
		long, err := openAIRates(row[5:9])
		if err != nil {
			return nil, err
		}
		p := Price{Model: model, Scope: "USD；OpenAI Standard／全球标准 API；不含区域处理附加费", Rates: standard}
		if long != (Rates{}) {
			if long.InputPrice == "" || long.OutputPrice == "" || (standard.CacheReadPrice != "" && long.CacheReadPrice == "") || (standard.CacheWritePrice != "" && long.CacheWritePrice == "") {
				return nil, errors.New("OpenAI 官方长上下文单价不完整")
			}
			p.Tiers = []Tier{{Label: "标准", Condition: condition, Rates: standard}, {Label: "长上下文", Rates: long}}
		} else if condition != "" {
			return nil, errors.New("OpenAI 官方长上下文档位缺失")
		}
		prices = append(prices, p)
	}
	// This section lists standard Codex/search/chat prices separately. Never
	// read its Fast, embedding, moderation or fine-tuning rows as chat prices.
	start := strings.Index(string(raw), "\nSpecialized models\n")
	if start < 0 {
		return nil, errors.New("OpenAI 专用模型官方价格章节缺失")
	}
	specialized, err := firstPriceTable(string(raw)[start:], []string{"Category", "Model", "Input", "Cached input", "Output"})
	if err != nil {
		return nil, err
	}
	for _, row := range specialized {
		if row[0] == "Embedding" || row[0] == "Moderation" {
			continue
		}
		if !publishedID.MatchString(row[1]) || ProviderID(row[1]) != "openai" || seen[row[1]] {
			return nil, errors.New("OpenAI 专用模型 ID 无效或重复")
		}
		rates, err := openAIRates([]string{row[2], row[3], "-", row[4]})
		if err != nil || rates.InputPrice == "" || rates.OutputPrice == "" {
			return nil, errors.New("OpenAI 专用模型单价不完整")
		}
		seen[row[1]] = true
		prices = append(prices, Price{Model: row[1], Scope: "USD；OpenAI Standard／全球标准 API", Rates: rates})
	}
	images, err := parseOpenAIImages(raw)
	if err != nil {
		return nil, err
	}
	return append(prices, images...), nil
}

func openAIRates(cells []string) (Rates, error) {
	values := make([]string, 4)
	for i := range values {
		var err error
		values[i], err = dollarRate(cells[i], true)
		if err != nil {
			return Rates{}, err
		}
	}
	return Rates{InputPrice: values[0], CacheReadPrice: values[1], CacheWritePrice: values[2], OutputPrice: values[3]}, nil
}
