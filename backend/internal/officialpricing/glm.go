package officialpricing

import (
	"errors"
	"regexp"
	"strings"
)

const GLMURL = "https://docs.bigmodel.cn/cn/guide/start/pricing"

var glmModel = regexp.MustCompile(`^GLM-[A-Za-z0-9.-]+$`)
var glmInput = regexp.MustCompile(`输入(?:长度)?\s*\[\s*([0-9.]+)([KM]?),\s*([0-9.]+)([KM]?)\)`)
var glmOutput = regexp.MustCompile(`输出\s*\[\s*([0-9.]+)([KM]?),\s*([0-9.]+)([KM]?)\)`)
var glmLower = regexp.MustCompile(`(输入(?:长度)?|输出)\s*≥\s*([0-9.]+)([KM]?)`)

func glmCondition(label string) (string, error) {
	s := strings.ReplaceAll(label, `\`, "")
	parts := []string{}
	for _, entry := range []struct {
		re       *regexp.Regexp
		variable string
	}{{glmInput, "len"}, {glmOutput, "c"}} {
		if m := entry.re.FindStringSubmatch(s); m != nil {
			if m[1] != "0" {
				parts = append(parts, entry.variable+" >= "+tokenCount(m[1], m[2]))
			}
			parts = append(parts, entry.variable+" < "+tokenCount(m[3], m[4]))
		}
	}
	for _, m := range glmLower.FindAllStringSubmatch(s, -1) {
		variable := "len"
		if m[1] == "输出" {
			variable = "c"
		}
		parts = append(parts, variable+" >= "+tokenCount(m[2], m[3]))
	}
	if len(parts) == 0 && (strings.Contains(s, "输入") || strings.Contains(s, "输出")) {
		return "", errors.New("GLM 阶梯条件格式已变更")
	}
	return strings.Join(parts, " && "), nil
}
func ParseGLM(raw []byte) ([]Price, error) {
	prices := map[string]*Price{}
	validTable := false
	for _, row := range markdownRows(raw) {
		if strings.Contains(row[0], "模型名称") {
			validTable = len(row) >= 6 && strings.Contains(row[2], "输入单价（元/百万 Tokens）") && strings.Contains(row[3], "输出单价（元/百万 Tokens）") && strings.Contains(row[5], "缓存命中")
			continue
		}
		if !validTable || !glmModel.MatchString(row[0]) {
			continue
		}
		if len(row) < 6 {
			return nil, errors.New("GLM 价格表列数已变更")
		}
		rates := Rates{}
		for i, target := range []*string{&rates.InputPrice, &rates.OutputPrice, &rates.CacheReadPrice} {
			col := i + 2
			if i == 2 {
				col = 5
			}
			n, e := plainRate(row[col])
			if e != nil {
				return nil, e
			}
			*target = n
		}
		condition, e := glmCondition(row[1])
		if e != nil {
			return nil, e
		}
		if err := addTier(prices, row[0], "实时调用", strings.ReplaceAll(row[1], `\`, ""), condition, rates); err != nil {
			return nil, err
		}
	}
	for _, p := range prices {
		conditions := map[string]bool{}
		for _, tier := range p.Tiers {
			conditions[tier.Condition] = true
		}
		if len(p.Tiers) == 1 && p.Tiers[0].Condition != "" {
			return nil, errors.New("GLM 上下文价格档位不完整")
		}
		for _, tier := range p.Tiers {
			complement := ""
			if strings.Contains(tier.Condition, "c < ") {
				complement = strings.Replace(tier.Condition, "c < ", "c >= ", 1)
			}
			if strings.Contains(tier.Condition, "c >= ") {
				complement = strings.Replace(tier.Condition, "c >= ", "c < ", 1)
			}
			if complement != "" && !conditions[complement] {
				return nil, errors.New("GLM 输出长度价格档位不完整")
			}
		}
		if strings.Contains(p.Tiers[len(p.Tiers)-1].Condition, "c ") {
			return nil, errors.New("GLM 长上下文档位缺失")
		}
	}
	return finish(prices)
}
