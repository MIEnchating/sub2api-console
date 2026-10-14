package officialpricing

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

const GeminiURL = "https://ai.google.dev/gemini-api/docs/pricing"

// This provider covers standard text generation. Other output modalities need
// their own billing dimensions and must never inherit a text-only price.
func geminiTextModel(name string) bool {
	if !strings.HasPrefix(name, "gemini-") || !publishedID.MatchString(name) {
		return false
	}
	for _, part := range []string{"image", "banana", "tts", "audio", "live", "transcribe", "embedding", "omni"} {
		if strings.Contains(name, part) {
			return false
		}
	}
	return true
}

func ParseGemini(raw []byte) ([]Price, error) {
	doc, err := html.Parse(strings.NewReader(string(raw)))
	if err != nil {
		return nil, err
	}
	var result []Price
	var models []string
	section := ""
	parsed := false
	seen := map[string]bool{}
	var parseErr error
	complete := func() {
		if len(models) > 0 && !parsed && parseErr == nil {
			parseErr = fmt.Errorf("%s Gemini 官方标准价格表缺失", models[0])
		}
	}
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if parseErr != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "h2" {
			complete()
			models, section, parsed = nil, "", false
			if !geminiHeadingGroup(n.Parent) {
				return
			}
			// Only the published code IDs beside the model heading establish aliases.
			for _, em := range elements(n.Parent, "em") {
				for _, code := range elements(em, "code") {
					name := nodeText(code)
					if geminiTextModel(name) {
						models = append(models, name)
					}
				}
			}
		}
		if n.Type == html.ElementNode && n.Data == "h3" {
			section = nodeText(n)
		}
		if n.Type == html.ElementNode && n.Data == "table" {
			if len(models) == 0 || section != "Standard" {
				return
			}
			if parsed {
				parseErr = errors.New("Gemini 官方标准价格表重复")
				return
			}
			p, err := parseGeminiTable(n)
			if err != nil {
				parseErr = fmt.Errorf("%s：%w", models[0], err)
				return
			}
			for _, name := range models {
				if seen[name] {
					parseErr = fmt.Errorf("Gemini 官方模型 ID 重复：%s", name)
					return
				}
				seen[name] = true
				p.Model = name
				result = append(result, p)
			}
			parsed = true
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(doc)
	complete()
	if parseErr != nil {
		return nil, parseErr
	}
	if len(result) == 0 {
		return nil, errors.New("Gemini 官方模型价格表缺失或格式已变更")
	}
	return result, nil
}

func geminiHeadingGroup(n *html.Node) bool {
	for _, attr := range n.Attr {
		if attr.Key == "class" {
			for _, class := range strings.Fields(attr.Val) {
				if class == "heading-group" {
					return true
				}
			}
		}
	}
	return false
}

func parseGeminiTable(table *html.Node) (Price, error) {
	rows := elements(table, "tr")
	grid, err := tableGrid(table)
	if err != nil {
		return Price{}, err
	}
	if len(grid) < 3 || len(grid[0]) != 3 || grid[0][1] != "Free Tier" || grid[0][2] != "Paid Tier, per 1M tokens in USD" {
		return Price{}, errors.New("Gemini 官方计价单位或表头已变更")
	}
	columns := map[string][]geminiRate{}
	for i, row := range grid[1:] {
		field := ""
		switch row[0] {
		case "Input price", "Input price (text, image, video)":
			field = "input"
		case "Output price", "Output price (including thinking tokens)":
			field = "output"
		case "Context caching price":
			field = "cache"
		}
		if field == "" {
			continue
		}
		if _, ok := columns[field]; ok {
			return Price{}, errors.New("Gemini 官方单价重复")
		}
		cells := elements(rows[i+1], "td")
		if len(cells) != 3 {
			return Price{}, errors.New("Gemini 官方单价列缺失")
		}
		values, err := parseGeminiRates(cells[2], field == "cache")
		if err != nil {
			return Price{}, err
		}
		columns[field] = values
	}
	input, output := columns["input"], columns["output"]
	if len(input) == 0 || len(input) != len(output) {
		return Price{}, errors.New("Gemini 官方输入或输出档位不完整")
	}
	cache := columns["cache"]
	if len(cache) > 0 && len(cache) != len(input) {
		return Price{}, errors.New("Gemini 官方缓存档位不完整")
	}
	p := Price{Scope: "USD；Gemini API Standard 付费档；文本输入/输出（含思考 Token）；不含音频、图片生成、工具调用及缓存存储时长费用"}
	prices := map[string]*Price{}
	for i, in := range input {
		if in.condition != output[i].condition || in.label != output[i].label {
			return Price{}, errors.New("Gemini 官方输入输出档位不一致")
		}
		rates := Rates{InputPrice: in.value, OutputPrice: output[i].value}
		if len(cache) > 0 {
			if in.condition != cache[i].condition || in.label != cache[i].label {
				return Price{}, errors.New("Gemini 官方缓存档位不一致")
			}
			rates.CacheReadPrice = cache[i].value
		}
		if i == 0 {
			p.Rates = rates
		}
		p.Tiers = append(p.Tiers, Tier{Label: in.label, Condition: in.condition, Rates: rates})
	}
	if input[0].dated {
		// Dates are published without a timezone. Keep both quotes visible, but do
		// not invent an executable boundary or sync an introductory quote forever.
		p.SyncError = "官网价格包含日期切换且未注明时区，暂不支持自动同步；请核对官方生效日期后手动设置"
		p.Scope += "；日期按官网原文，切换时区未注明；两档价格仅供参考"
	} else {
		prices["gemini"] = &p
		finished, err := finish(prices)
		if err != nil {
			return Price{}, err
		}
		p = finished[0]
	}
	return p, nil
}
