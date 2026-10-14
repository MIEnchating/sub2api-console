package officialpricing

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

type geminiRate struct {
	value, label, condition string
	dated                   bool
}

var geminiDateRate = regexp.MustCompile(`^[$]([0-9]+(?:[.][0-9]+)?) (through|starting) ([A-Z][a-z]+ [0-9]{1,2}, [0-9]{4})[.]$`)
var geminiContextRate = regexp.MustCompile(`^[$]([0-9]+(?:[.][0-9]+)?), prompts (<=|>) ([0-9]+)([kKmM]?)(?: tokens)?$`)
var geminiModalities = regexp.MustCompile(` *[(](?:text|image|video|audio)(?: */ *(?:text|image|video|audio))*[)]`)
var geminiStorage = regexp.MustCompile(`^[$][0-9]+(?:[.][0-9]+)? / 1,000,000 tokens per hour [(]storage price[)](?: (?:through|starting) [A-Z][a-z]+ [0-9]{1,2}, [0-9]{4}[.])?$`)

func geminiCellLines(cell *html.Node) []string {
	var b strings.Builder
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "sup" || n.Data == "script" || n.Data == "style") {
			return
		}
		if n.Data == "br" || n.Data == "p" || n.Data == "div" {
			b.WriteByte(10)
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(cell)
	var lines []string
	for _, line := range strings.Split(b.String(), string(rune(10))) {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func parseGeminiRates(cell *html.Node, cache bool) ([]geminiRate, error) {
	var result []geminiRate
	for _, line := range geminiCellLines(cell) {
		if cache && (geminiStorage.MatchString(line) || line == "Implicit caching only (explicit caching is not supported).") {
			continue
		}
		if cache && line == "Not available" {
			if len(geminiCellLines(cell)) != 1 {
				return nil, errors.New("Gemini 官方缓存价格冲突")
			}
			return nil, nil
		}
		modality := geminiModalities.FindString(line)
		if modality != "" && !strings.Contains(modality, "text") {
			if _, err := dollarRate(strings.TrimSpace(geminiModalities.ReplaceAllString(line, "")), true); err != nil {
				return nil, err
			}
			continue
		}
		line = strings.TrimSpace(geminiModalities.ReplaceAllString(line, ""))
		rate := geminiRate{label: "标准"}
		var err error
		if m := geminiDateRate.FindStringSubmatch(line); m != nil {
			date, dateErr := time.Parse("January 2, 2006", m[3])
			if dateErr != nil {
				return nil, errors.New("Gemini 官方价格生效日期无效")
			}
			rate.value, err = number(m[1], 1_000_000)
			rate.dated = true
			rate.condition = m[2] + " " + date.Format(time.DateOnly)
			rate.label = "截至 " + date.Format(time.DateOnly)
			if m[2] == "starting" {
				rate.label = date.Format(time.DateOnly) + " 起"
			}
		} else if m := geminiContextRate.FindStringSubmatch(line); m != nil {
			threshold, countErr := tokenCount(m[3], m[4])
			if countErr != nil || threshold == "0" {
				return nil, errors.New("Gemini 官方上下文阈值无效")
			}
			rate.value, err = number(m[1], 1_000_000)
			rate.condition = "len " + m[2] + " " + threshold
			rate.label = "输入 ≤ " + threshold + " Token"
			if m[2] == ">" {
				rate.label = "输入 > " + threshold + " Token"
			}
		} else {
			rate.value, err = dollarRate(line, true)
		}
		if err != nil || rate.value == "" {
			return nil, fmt.Errorf("Gemini 官方单价格式已变更：%s", line)
		}
		result = append(result, rate)
	}
	if err := validateGeminiRates(result); err != nil {
		return nil, err
	}
	return result, nil
}

func validateGeminiRates(rates []geminiRate) error {
	if len(rates) == 1 && rates[0].condition == "" {
		return nil
	}
	if len(rates) != 2 {
		return errors.New("Gemini 官方价格档位缺失或重复")
	}
	first, second := rates[0], rates[1]
	if first.dated && second.dated && strings.HasPrefix(first.condition, "through ") && strings.HasPrefix(second.condition, "starting ") {
		end, _ := time.Parse(time.DateOnly, strings.TrimPrefix(first.condition, "through "))
		start, _ := time.Parse(time.DateOnly, strings.TrimPrefix(second.condition, "starting "))
		if end.AddDate(0, 0, 1).Equal(start) {
			return nil
		}
	}
	if !first.dated && !second.dated && strings.HasPrefix(first.condition, "len <= ") && second.condition == "len > "+strings.TrimPrefix(first.condition, "len <= ") {
		return nil
	}
	return errors.New("Gemini 官方价格档位边界不完整或不一致")
}
