package officialpricing

import (
	"errors"
	"fmt"
	"golang.org/x/net/html"
	"math/big"
	"regexp"
	"strings"
)

const QwenURL = "https://help.aliyun.com/zh/model-studio/model-pricing"

var qwenName = regexp.MustCompile(`^(?:qwen|qwq|qvq)[a-z0-9.-]*`)
var qwenMoney = regexp.MustCompile(`^(?:原价)?([0-9]+(?:\.[0-9]+)?)元(?:（限时([0-9]+(?:\.[0-9]+)?)折）)?$`)
var qwenRange = regexp.MustCompile(`^([0-9.]+)([KM]?)<Token≤([0-9.]+)([KM]?)$`)

func qwenRate(value string) (string, error) {
	value = strings.ReplaceAll(value, " ", "")
	m := qwenMoney.FindStringSubmatch(value)
	if m == nil {
		return "", fmt.Errorf("Qwen 单价格式已变更：%s", value)
	}
	n, e := number(m[1], 1_000_000)
	if e != nil {
		return "", e
	}
	if m[2] != "" {
		r, _ := new(big.Rat).SetString(n)
		discount, _ := new(big.Rat).SetString(m[2])
		if discount.Sign() <= 0 || discount.Cmp(big.NewRat(10, 1)) > 0 {
			return "", errors.New("Qwen 折扣无效")
		}
		r.Mul(r, discount)
		r.Quo(r, big.NewRat(10, 1))
		n = formatDecimal(r)
	}
	return n, nil
}
func ParseQwen(raw []byte) ([]Price, error) {
	doc, e := html.Parse(strings.NewReader(string(raw)))
	if e != nil {
		return nil, e
	}
	prices := map[string]*Price{}
	region := ""
	var parseErr error
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if parseErr != nil {
			return
		}
		if n.Type == html.ElementNode && len(n.Data) == 2 && n.Data[0] == 'h' && n.Data[1] >= '1' && n.Data[1] <= '6' {
			region = nodeText(n)
		}
		if n.Type == html.ElementNode && n.Data == "table" {
			if strings.Contains(region, "华北2（北京）") {
				parseErr = parseQwenTable(n, prices)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(doc)
	if parseErr != nil {
		return nil, parseErr
	}
	return finish(prices)
}
func parseQwenTable(table *html.Node, prices map[string]*Price) error {
	trs := elements(table, "tr")
	if len(trs) == 0 {
		return nil
	}
	header := nodeText(trs[0])
	if !strings.Contains(header, "输入单价（每百万Token）") || !strings.Contains(header, "输出单价（每百万Token）") {
		return nil
	}
	grid, e := tableGrid(table)
	if e != nil {
		return e
	}
	if len(grid) < 2 {
		return nil
	}
	input, output, contextCol := -1, -1, -1
	for i, h := range grid[0] {
		switch {
		case strings.Contains(h, "输入单价"):
			if input != -1 {
				return nil
			}
			input = i
		case strings.Contains(h, "输出单价"):
			if output == -1 {
				output = i
			}
		case strings.Contains(h, "单次请求"):
			contextCol = i
		}
	}
	if input < 0 || output < 0 {
		return nil
	}
	start := 1
	modeColumns := false
	if strings.Contains(strings.Join(grid[1], " "), "思考模式") && qwenName.FindString(grid[1][0]) == "" {
		modeColumns = true
		start = 2
	}
	// Other multirow headers represent modality-specific rates, not text prices.
	if qwenName.FindString(grid[1][0]) == "" && !modeColumns {
		return nil
	}
	if modeColumns && (grid[1][output] != "非思考模式" || output+1 >= len(grid[1]) || !strings.Contains(grid[1][output+1], "思考模式")) {
		return nil
	}
	for _, row := range grid[start:] {
		name := qwenName.FindString(row[0])
		if name == "" {
			continue
		}
		unsupported := false
		for _, part := range []string{"omni", "audio", "realtime", "tts", "asr", "embedding", "rerank", "image"} {
			if strings.Contains(name, part) {
				unsupported = true
			}
		}
		if unsupported {
			continue
		}
		in, e := qwenRate(row[input])
		if e != nil {
			return e
		}
		label, condition := "标准", ""
		if contextCol >= 0 && row[contextCol] != "无阶梯计价" {
			label = strings.ReplaceAll(row[contextCol], " ", "")
			r := qwenRange.FindStringSubmatch(label)
			if r == nil {
				return fmt.Errorf("%s 上下文条件格式已变更：%s", name, label)
			}
			condition = "len <= " + tokenCount(r[3], r[4])
			if r[1] != "0" {
				condition = "len > " + tokenCount(r[1], r[2]) + " && " + condition
			}
		}
		count := 1
		if modeColumns {
			count = 2
		}
		for mode := 0; mode < count; mode++ {
			if row[output+mode] == "-" || row[output+mode] == "—" {
				continue
			}
			out, e := qwenRate(row[output+mode])
			if e != nil {
				return fmt.Errorf("%s: %w", name, e)
			}
			tierLabel, tierCondition := label, condition
			if modeColumns {
				modeCondition := `param("enable_thinking") != true`
				modeLabel := "非思考"
				if mode == 1 {
					modeCondition = `param("enable_thinking") == true`
					modeLabel = "思考"
				}
				if tierCondition != "" {
					tierCondition += " && "
				}
				tierCondition += modeCondition
				tierLabel = modeLabel + " · " + label
			}
			scope := "华北2（北京）· 实时调用；缓存折扣另计"
			if modeColumns {
				scope += "；思考档按 enable_thinking=true"
			}
			if err := addTier(prices, name, scope, tierLabel, tierCondition, Rates{InputPrice: in, OutputPrice: out}); err != nil {
				return err
			}
		}
	}
	return nil
}
