package officialpricing

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Tier struct {
	Label     string `json:"label"`
	Condition string `json:"condition"`
	Rates
}

var decimal = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`)
var strike = regexp.MustCompile(`~~[^~]*~~`)
var tags = regexp.MustCompile(`<[^>]*>`)

func number(value string, divisor int64) (string, error) {
	if len(value) > 32 || !decimal.MatchString(value) {
		return "", fmt.Errorf("无效价格数值 %q", value)
	}
	r, ok := new(big.Rat).SetString(value)
	if !ok {
		return "", errors.New("无效价格")
	}
	r.Quo(r, big.NewRat(divisor, 1))
	return formatDecimal(r), nil
}
func formatDecimal(r *big.Rat) string {
	s := strings.TrimRight(strings.TrimRight(r.FloatString(38), "0"), ".")
	if s == "" {
		return "0"
	}
	return s
}
func plainRate(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(strike.ReplaceAllString(value, ""), "**", ""))
	if value == "免费" {
		return "0", nil
	}
	if value == "不支持" || value == "—" || value == "-" {
		return "", nil
	}
	return number(value, 1_000_000)
}
func markdownRows(raw []byte) [][]string {
	var rows [][]string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		rows = append(rows, cells)
	}
	return rows
}
func addTier(prices map[string]*Price, name, scope, label, condition string, rates Rates) error {
	if rates.InputPrice == "" || rates.OutputPrice == "" {
		return fmt.Errorf("%s 输入或输出价格缺失", name)
	}
	p := prices[name]
	if p == nil {
		p = &Price{Model: name, Scope: scope, Rates: rates}
		prices[name] = p
	}
	for _, t := range p.Tiers {
		if t.Condition == condition {
			return fmt.Errorf("%s 价格档位重复", name)
		}
	}
	p.Tiers = append(p.Tiers, Tier{Label: label, Condition: condition, Rates: rates})
	return nil
}
func finish(prices map[string]*Price) ([]Price, error) {
	if len(prices) == 0 {
		return nil, errors.New("官方模型价格表缺失或格式已变更")
	}
	out := make([]Price, 0, len(prices))
	for _, p := range prices {
		if len(p.Tiers) > 1 {
			var expr strings.Builder
			for i, t := range p.Tiers {
				if i < len(p.Tiers)-1 {
					if t.Condition == "" {
						return nil, fmt.Errorf("%s 阶梯条件缺失", p.Model)
					}
					expr.WriteString(t.Condition + " ? ")
				}
				expr.WriteString("tier(" + strconv.Quote(t.Label) + ", " + ratesExpr(t.Rates) + ")")
				if i < len(p.Tiers)-1 {
					expr.WriteString(" : ")
				}
			}
			p.BillingExpr = expr.String()
		} else {
			p.Tiers = nil
		}
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Model < out[j].Model })
	return out, nil
}
func ratesExpr(r Rates) string {
	parts := []string{}
	for _, field := range [][2]string{{"p", r.InputPrice}, {"c", r.OutputPrice}, {"cr", r.CacheReadPrice}, {"cc", r.CacheWritePrice}} {
		if field[1] == "" {
			continue
		}
		n, _ := new(big.Rat).SetString(field[1])
		n.Mul(n, big.NewRat(1_000_000, 1))
		parts = append(parts, field[0]+" * "+formatDecimal(n))
	}
	return strings.Join(parts, " + ")
}

// Providers use decimal K/M units on these pages, not Ki/Mi units.
func tokenCount(value, unit string) string {
	r, _ := new(big.Rat).SetString(value)
	if strings.EqualFold(unit, "k") {
		r.Mul(r, big.NewRat(1000, 1))
	}
	if strings.EqualFold(unit, "m") {
		r.Mul(r, big.NewRat(1_000_000, 1))
	}
	return r.FloatString(0)
}
