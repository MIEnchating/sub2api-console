// Package officialpricing reads published price tables without currency conversion.
package officialpricing

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const DeepSeekURL = "https://api-docs.deepseek.com/zh-cn/quick_start/pricing"

type Rates struct {
	InputPrice      string `json:"input_price"`
	OutputPrice     string `json:"output_price"`
	CacheReadPrice  string `json:"cache_read_price"`
	CacheWritePrice string `json:"cache_write_price,omitempty"`
}

type Period struct {
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

// Base rates are off-peak; peak rates are copied separately, never inferred from a discount.
type TimePricing struct {
	Timezone     string   `json:"timezone"`
	WeekdaysOnly bool     `json:"weekdays_only"`
	Periods      []Period `json:"periods"`
	Peak         Rates    `json:"peak"`
}

type Price struct {
	Model       string `json:"model"`
	SourceURL   string `json:"source_url,omitempty"`
	Scope       string `json:"scope,omitempty"`
	Tiers       []Tier `json:"tiers,omitempty"`
	BillingExpr string `json:"billing_expr,omitempty"`
	Rates
	TimePricing TimePricing `json:"time_pricing"`
}

func FetchDeepSeek(ctx context.Context, client *http.Client) ([]Price, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	var raw []byte
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(100 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		raw, err = fetchURL(ctx, client, DeepSeekURL)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	return ParseDeepSeek(raw)
}

var peakHours = regexp.MustCompile(`高峰时段为北京时间周一至周五\s*([0-9: 、,，–—\-]+)（其余为空闲时段）`)
var clockRange = regexp.MustCompile(`(\d{1,2}:\d{2})\s*[-–—]\s*(\d{1,2}:\d{2})`)
var amount = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)元$`)
var modelName = regexp.MustCompile(`^deepseek-[a-z0-9-]+$`)

// ParseDeepSeek rejects incomplete tables or changed schedule wording instead of
// silently turning time-dependent pricing into an all-day static price.
func ParseDeepSeek(raw []byte) ([]Price, error) {
	doc, err := html.Parse(strings.NewReader(string(raw)))
	if err != nil {
		return nil, err
	}
	articles := elements(doc, "article")
	if len(articles) != 1 {
		return nil, errors.New("DeepSeek 官方价格正文缺失")
	}
	article := articles[0]
	match := peakHours.FindStringSubmatch(nodeText(article))
	if match == nil {
		return nil, errors.New("DeepSeek 官方峰谷时段格式已变更")
	}
	var periods []Period
	for _, span := range clockRange.FindAllStringSubmatch(match[1], -1) {
		start, e1 := time.Parse("15:04", span[1])
		end, e2 := time.Parse("15:04", span[2])
		if e1 != nil || e2 != nil || !start.Before(end) {
			return nil, errors.New("DeepSeek 官方峰谷时段无效")
		}
		if len(periods) > 0 && start.Format("15:04") < periods[len(periods)-1].EndTime {
			return nil, errors.New("DeepSeek 官方峰谷时段重叠")
		}
		periods = append(periods, Period{StartTime: start.Format("15:04"), EndTime: end.Format("15:04")})
	}
	if len(periods) == 0 {
		return nil, errors.New("DeepSeek 官方峰谷时段为空")
	}
	for _, table := range elements(article, "table") {
		grid, err := tableGrid(table)
		if err != nil {
			return nil, err
		}
		prices, err := parsePrices(grid, periods)
		if err != nil {
			return nil, err
		}
		if len(prices) == 0 {
			continue
		}
		return withPublishedAliases(article, prices), nil
	}
	return nil, errors.New("DeepSeek 官方价格表缺失")
}

func parsePrices(grid [][]string, periods []Period) ([]Price, error) {
	if len(grid) == 0 || len(grid[0]) < 4 || grid[0][0] != "模型" {
		return nil, nil
	}
	var prices []Price
	for _, name := range grid[0][3:] {
		if !modelName.MatchString(name) {
			return nil, errors.New("DeepSeek 官方模型名称格式已变更")
		}
		prices = append(prices, Price{Model: name, TimePricing: TimePricing{Timezone: "Asia/Shanghai", WeekdaysOnly: true, Periods: periods}})
	}
	for _, row := range grid {
		if len(row) != len(prices)+3 || row[0] != "价格" {
			continue
		}
		for i := range prices {
			rates := &prices[i].Rates
			if row[2] == "高峰时段" {
				rates = &prices[i].TimePricing.Peak
			} else if row[2] != "空闲时段" {
				return nil, errors.New("DeepSeek 官方价格档位格式已变更")
			}
			var target *string
			switch row[1] {
			case "百万tokens输入 （缓存命中）":
				target = &rates.CacheReadPrice
			case "百万tokens输入 （缓存未命中）":
				target = &rates.InputPrice
			case "百万tokens输出":
				target = &rates.OutputPrice
			default:
				return nil, errors.New("DeepSeek 官方计价单位格式已变更")
			}
			match := amount.FindStringSubmatch(row[i+3])
			if match == nil || *target != "" {
				return nil, errors.New("DeepSeek 官方单价无效或重复")
			}
			value, ok := new(big.Rat).SetString(match[1])
			if !ok || value.Sign() < 0 || len(match[1]) > 32 {
				return nil, errors.New("DeepSeek 官方单价无效")
			}
			value.Quo(value, big.NewRat(1_000_000, 1))
			*target = strings.TrimRight(strings.TrimRight(value.FloatString(38), "0"), ".")
		}
	}
	for _, price := range prices {
		for _, rates := range []Rates{price.Rates, price.TimePricing.Peak} {
			if rates.InputPrice == "" || rates.OutputPrice == "" || rates.CacheReadPrice == "" {
				return nil, errors.New("DeepSeek 官方价格档位不完整")
			}
		}
	}
	return prices, nil
}

func withPublishedAliases(article *html.Node, prices []Price) []Price {
	for _, paragraph := range elements(article, "p") {
		text := nodeText(paragraph)
		if !strings.Contains(text, "旧模型名") || !strings.Contains(text, "并按 Flash 价格计费") {
			continue
		}
		codes := elements(paragraph, "code")
		if len(codes) < 2 {
			continue
		}
		for _, price := range prices {
			if price.Model != nodeText(codes[0]) {
				continue
			}
			for _, code := range codes[1:] {
				name := nodeText(code)
				if modelName.MatchString(name) {
					alias := price
					alias.Model = name
					prices = append(prices, alias)
				}
			}
			break
		}
	}
	return prices
}
