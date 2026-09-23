package officialpricing

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const GrokURL = "https://docs.x.ai/developers/models"

var grokContext = regexp.MustCompile(`^(grok-[a-z0-9.-]+) \((<|≥|>=) ([0-9.]+)([kKmM]?) prompt tokens\)$`)

func ParseGrok(raw []byte) ([]Price, error) {
	section, err := markdownSection(string(raw), "### Text API Pricing")
	if err != nil {
		return nil, err
	}
	rows, err := firstPriceTable(section, []string{"Model", "Context", "Input / 1M tokens", "Cached input / 1M tokens", "Output / 1M tokens"})
	if err != nil {
		return nil, err
	}
	type contextPrice struct {
		threshold   string
		short, long *Rates
	}
	contexts := map[string]*contextPrice{}
	prices := map[string]*Price{}
	for _, row := range rows {
		values := make([]string, 3)
		for i := range values {
			values[i], err = dollarRate(row[i+2], true)
			if err != nil {
				return nil, err
			}
		}
		rates := Rates{InputPrice: values[0], CacheReadPrice: values[1], OutputPrice: values[2]}
		if rates.InputPrice == "" || rates.OutputPrice == "" || rates.CacheReadPrice == "" {
			return nil, errors.New("Grok 官方 Token 单价不完整")
		}
		match := grokContext.FindStringSubmatch(row[0])
		if match == nil {
			if !publishedID.MatchString(row[0]) || !strings.HasPrefix(row[0], "grok-") {
				return nil, errors.New("Grok 官方模型或阶梯条件已变更")
			}
			if err := addTier(prices, row[0], "USD；xAI 标准 API Token 价格", "标准", "", rates); err != nil {
				return nil, err
			}
			continue
		}
		threshold, err := tokenCount(match[3], match[4])
		if err != nil || threshold == "0" {
			return nil, errors.New("Grok 官方阶梯阈值无效")
		}
		entry := contexts[match[1]]
		if entry == nil {
			entry = &contextPrice{threshold: threshold}
			contexts[match[1]] = entry
		}
		if entry.threshold != threshold {
			return nil, errors.New("Grok 官方阶梯阈值不一致")
		}
		target := &entry.long
		if match[2] == "<" {
			target = &entry.short
		}
		if *target != nil {
			return nil, errors.New("Grok 官方阶梯重复")
		}
		*target = &rates
	}
	for name, entry := range contexts {
		if entry.short == nil || entry.long == nil || prices[name] != nil {
			return nil, fmt.Errorf("%s 官方长上下文价格不完整", name)
		}
		scope := "USD；xAI 标准 API；输入达到 " + entry.threshold + " Token 时整单按长上下文计费"
		if err := addTier(prices, name, scope, "标准", "len < "+entry.threshold, *entry.short); err != nil {
			return nil, err
		}
		if err := addTier(prices, name, scope, "长上下文", "", *entry.long); err != nil {
			return nil, err
		}
	}
	result, err := finish(prices)
	if err != nil {
		return nil, err
	}
	imagine, err := markdownSection(string(raw), "### Imagine Pricing")
	if err != nil {
		return nil, err
	}
	media, err := firstPriceTable(imagine, []string{"Model", "Cost"})
	if err != nil {
		return nil, err
	}
	for _, row := range media {
		parts := strings.Split(row[1], " / ")
		if !publishedID.MatchString(row[0]) || !strings.HasPrefix(row[0], "grok-imagine-") || len(parts) != 2 {
			return nil, errors.New("Grok 官方图片或视频价格格式已变更")
		}
		value, err := dollarRate(parts[0], false)
		if err != nil || value == "" {
			return nil, errors.New("Grok 官方图片或视频单价无效")
		}
		unit, mode := "每张图片", "image_generation"
		if parts[1] == "sec" {
			unit, mode = "每秒视频", "video_generation"
		} else if parts[1] != "image" {
			return nil, errors.New("Grok 官方计价单位已变更")
		}
		p := Price{Model: row[0], Mode: mode, Scope: "USD；" + unit + " " + value, SyncError: "该模型按图片数量或视频时长计费，暂不支持自动同步；请按官方单位配置计费"}
		if parts[1] == "image" {
			p.ImageOutputPrice, p.ImageOutputUnit = value, "image"
		}
		result = append(result, p)
	}
	return result, nil
}
