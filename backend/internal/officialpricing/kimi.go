package officialpricing

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const KimiURL = "https://platform.kimi.com/docs/pricing/chat"

var kimiTable = regexp.MustCompile(`(?s)columns=\{\[(.*?)\]\}.*?rows=\{\[(.*?)\]\}`)
var kimiHeaders = regexp.MustCompile(`title:\s*["']([^"']+)["']`)
var kimiRow = regexp.MustCompile(`\[\s*"kimi-[^\]]+\]`)

func ParseKimi(raw []byte) ([]Price, error) {
	s := string(raw)
	matches := kimiTable.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil, errors.New("Kimi 官方价格表缺失")
	}
	prices := map[string]*Price{}
	for _, table := range matches {
		columns := kimiHeaders.FindAllStringSubmatch(table[1], -1)
		if len(columns) < 5 {
			return nil, errors.New("Kimi 官方价格表头已变更")
		}
		columnTitles := make([]string, len(columns))
		indexes := map[string]int{}
		for i, column := range columns {
			columnTitles[i] = column[1]
			indexes[column[1]] = i
		}
		validOrder := equalStrings(columnTitles, []string{"模型", "计费单位", "缓存写入（TTL 5min）", "缓存写入（TTL 1h）", "输入价格（缓存命中）", "输入价格（缓存未命中）", "输出价格", "上下文窗口"}) || equalStrings(columnTitles, []string{"模型", "计费单位", "输入价格（缓存命中）", "输入价格（缓存未命中）", "输出价格", "上下文窗口"})
		if !validOrder {
			return nil, errors.New("Kimi 官方价格列顺序已变更")
		}
		required := []string{"模型", "计费单位", "输入价格（缓存命中）", "输入价格（缓存未命中）", "输出价格"}
		for _, title := range required {
			if _, ok := indexes[title]; !ok {
				return nil, errors.New("Kimi 官方价格列顺序已变更")
			}
		}
		for _, row := range kimiRow.FindAllString(table[2], -1) {
			var cells []string
			if err := json.Unmarshal([]byte(row), &cells); err != nil {
				return nil, err
			}
			if len(cells) != len(columns) || cells[indexes["计费单位"]] != "1M tokens" {
				return nil, errors.New("Kimi 计价单位已变更")
			}
			price, err := kimiRates(cells, indexes)
			if err != nil {
				return nil, err
			}
			name := cells[indexes["模型"]]
			if !strings.HasPrefix(name, "kimi-") {
				return nil, errors.New("Kimi 官方模型名称格式已变更")
			}
			write5, err := kimiOptionalRate(cells, indexes, "缓存写入（TTL 5min）")
			if err != nil {
				return nil, err
			}
			write1h, err := kimiOptionalRate(cells, indexes, "缓存写入（TTL 1h）")
			if err != nil {
				return nil, err
			}
			if (write5 == "") != (write1h == "") {
				return nil, errors.New("Kimi 官方缓存写入价格档位不完整")
			}
			if write5 != "" {
				if err := addTier(prices, name, "实时调用", "TTL 5min", `param("cache_ttl") != "1h"`, priceWithCacheWrite(price, write5)); err != nil {
					return nil, err
				}
				if err := addTier(prices, name, "实时调用", "TTL 1h", `param("cache_ttl") == "1h"`, priceWithCacheWrite(price, write1h)); err != nil {
					return nil, err
				}
				continue
			}
			if err := addTier(prices, name, "实时调用", "标准", "", price); err != nil {
				return nil, err
			}
		}
	}
	return finish(prices)
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func kimiRates(cells []string, indexes map[string]int) (Rates, error) {
	rates := Rates{}
	for title, target := range map[string]*string{
		"输入价格（缓存命中）":  &rates.CacheReadPrice,
		"输入价格（缓存未命中）": &rates.InputPrice,
		"输出价格":        &rates.OutputPrice,
	} {
		value := strings.TrimPrefix(cells[indexes[title]], "¥")
		if value == cells[indexes[title]] {
			return Rates{}, errors.New("Kimi 中文价格格式已变更")
		}
		n, err := number(value, 1_000_000)
		if err != nil {
			return Rates{}, err
		}
		*target = n
	}
	return rates, nil
}

func kimiOptionalRate(cells []string, indexes map[string]int, title string) (string, error) {
	index, ok := indexes[title]
	if !ok {
		return "", nil
	}
	if cells[index] == "-" || cells[index] == "—" || cells[index] == "不支持" {
		return "", nil
	}
	value := strings.TrimPrefix(cells[index], "¥")
	if value == cells[index] {
		return "", fmt.Errorf("Kimi 官方%s单价格式已变更", title)
	}
	rate, err := number(value, 1_000_000)
	if err != nil {
		return "", fmt.Errorf("Kimi 官方%s单价无效：%w", title, err)
	}
	return rate, nil
}

func priceWithCacheWrite(price Rates, cacheWrite string) Rates {
	price.CacheWritePrice = cacheWrite
	return price
}
