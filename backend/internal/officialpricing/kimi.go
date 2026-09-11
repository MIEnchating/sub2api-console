package officialpricing

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

const KimiURL = "https://platform.kimi.com/docs/pricing/chat"

var kimiHeaders = regexp.MustCompile(`title:\s*"([^"]+)"`)

var kimiRows = regexp.MustCompile(`(?s)rows=\{\[(.*?)\]\}`)
var kimiRow = regexp.MustCompile(`\[\s*"kimi-[^\]]+\]`)

func ParseKimi(raw []byte) ([]Price, error) {
	s := string(raw)
	columns := kimiHeaders.FindAllStringSubmatch(s, -1)
	expected := []string{"模型", "计费单位", "输入价格（缓存命中）", "输入价格（缓存未命中）", "输出价格", "上下文窗口"}
	if len(columns) != len(expected) {
		return nil, errors.New("Kimi 官方价格表头已变更")
	}
	for i, title := range expected {
		if columns[i][1] != title {
			return nil, errors.New("Kimi 官方价格列顺序已变更")
		}
	}

	matches := kimiRows.FindStringSubmatch(s)
	if matches == nil {
		return nil, errors.New("Kimi 官方价格表缺失")
	}
	prices := map[string]*Price{}
	for _, row := range kimiRow.FindAllString(matches[1], -1) {
		var cells []string
		if err := json.Unmarshal([]byte(row), &cells); err != nil {
			return nil, err
		}
		if len(cells) != 6 || cells[1] != "1M tokens" {
			return nil, errors.New("Kimi 计价单位已变更")
		}
		rates := Rates{}
		for i, target := range []*string{&rates.CacheReadPrice, &rates.InputPrice, &rates.OutputPrice} {
			value := strings.TrimPrefix(cells[i+2], "¥")
			if value == cells[i+2] {
				return nil, errors.New("Kimi 中文价格格式已变更")
			}
			n, e := number(value, 1_000_000)
			if e != nil {
				return nil, e
			}
			*target = n
		}
		if err := addTier(prices, cells[0], "实时调用", "标准", "", rates); err != nil {
			return nil, err
		}
	}
	return finish(prices)
}
