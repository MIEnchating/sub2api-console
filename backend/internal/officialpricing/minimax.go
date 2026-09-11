package officialpricing

import (
	"errors"
	"regexp"
	"strings"
)

const MiniMaxURL = "https://platform.minimaxi.com/docs/guides/pricing-paygo"

var priorityTab = regexp.MustCompile(`(?s)<Tab title="优先[^"\n]*">.*?</Tab>`)
var minimaxModel = regexp.MustCompile(`\*\*(MiniMax-[A-Za-z0-9.-]+)\*\*`)
var minimaxRange = regexp.MustCompile(`(≤|>)\s*([0-9.]+)([kKmM])\s*输入`)

func ParseMiniMax(raw []byte) ([]Price, error) {
	s := string(raw)
	start := strings.Index(s, "## 语言模型")
	end := strings.Index(s, "## 语音")
	if start < 0 {
		return nil, errors.New("MiniMax 语言模型价格段落缺失")
	}
	if end > start {
		s = s[start:end]
	} else {
		s = s[start:]
	}
	s = priorityTab.ReplaceAllString(s, "")
	if !strings.Contains(s, "元/百万 tokens") {
		return nil, errors.New("MiniMax 计价单位已变更")
	}
	prices := map[string]*Price{}
	for _, row := range markdownRows([]byte(s)) {
		match := minimaxModel.FindStringSubmatch(row[0])
		if match == nil {
			continue
		}
		if len(row) != 4 && len(row) != 5 {
			return nil, errors.New("MiniMax 价格表列数已变更")
		}
		rates := Rates{}
		for i, target := range []*string{&rates.InputPrice, &rates.OutputPrice, &rates.CacheReadPrice, &rates.CacheWritePrice} {
			if i+1 >= len(row) {
				break
			}
			n, e := plainRate(row[i+1])
			if e != nil {
				return nil, e
			}
			*target = n
		}
		condition, label := "", "标准"
		if r := minimaxRange.FindStringSubmatch(row[0]); r != nil {
			op := r[1]
			if op == "≤" {
				op = "<="
			}
			condition = "len " + op + " " + tokenCount(r[2], r[3])
			label = "输入 " + r[1] + r[2] + strings.ToUpper(r[3])
		}
		if err := addTier(prices, match[1], "标准服务（不含 priority 优先调用）", label, condition, rates); err != nil {
			return nil, err
		}
	}
	for _, p := range prices {
		if len(p.Tiers) == 1 && p.Tiers[0].Condition != "" {
			return nil, errors.New("MiniMax 上下文价格档位不完整")
		}
		if len(p.Tiers) > 1 {
			if len(p.Tiers) != 2 || !strings.HasPrefix(p.Tiers[0].Condition, "len <= ") || p.Tiers[1].Condition != "len > "+strings.TrimPrefix(p.Tiers[0].Condition, "len <= ") {
				return nil, errors.New("MiniMax 上下文档位不连续")
			}
		}
	}
	return finish(prices)
}
