package officialpricing

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"sync"
)

var priceFootnote = regexp.MustCompile(`(?s)<sup>.*?</sup>`)
var publishedID = regexp.MustCompile("^[a-z0-9][a-z0-9.-]*$")

func perMillion(value string) string {
	r, _ := new(big.Rat).SetString(value)
	r.Mul(r, big.NewRat(1_000_000, 1))
	return formatDecimal(r)
}

func markdownSection(raw, heading string) (string, error) {
	lines := strings.Split(raw, "\n")
	start := -1
	level := len(heading) - len(strings.TrimLeft(heading, "#"))
	for i, line := range lines {
		if strings.TrimSpace(line) == heading {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return "", fmt.Errorf("官方价格章节缺失：%s", heading)
	}
	end := len(lines)
	for i := start; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		n := len(line) - len(strings.TrimLeft(line, "#"))
		if n > 0 && n <= level && strings.HasPrefix(line[n:], " ") {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n"), nil
}

func firstPriceTable(section string, headers []string) ([][]string, error) {
	var lines []string
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "|") {
			lines = append(lines, line)
		} else if len(lines) > 0 {
			break
		}
	}
	rows := markdownRows([]byte(strings.Join(lines, "\n")))
	if len(rows) < 3 || !equalStrings(rows[0], headers) {
		return nil, errors.New("官方价格表缺失或表头已变更")
	}
	if len(rows) > 130 {
		return nil, errors.New("官方价格表行数超出限制")
	}
	for _, row := range rows[2:] {
		if len(row) != len(headers) {
			return nil, errors.New("官方价格表列数已变更")
		}
	}
	return rows[2:], nil
}

func dollarRate(cell string, perMillion bool) (string, error) {
	cell = strings.TrimSpace(priceFootnote.ReplaceAllString(cell, ""))
	if cell == "-" || cell == "—" {
		return "", nil
	}
	if !strings.HasPrefix(cell, "$") {
		return "", errors.New("官方美元单价格式已变更")
	}
	divisor := int64(1)
	if perMillion {
		divisor = 1_000_000
	}
	return number(strings.TrimPrefix(cell, "$"), divisor)
}

// Bound workers as well as requests. A failed detail invalidates the provider
// catalog, so cancel active reads and stop scheduling the remaining models.
func enrichModelPrices(ctx context.Context, prices []Price, enrich func(context.Context, Price) ([]Price, error)) ([]Price, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make([][]Price, len(prices))
	jobs := make(chan int)
	var firstError error
	var failed sync.Once
	var wg sync.WaitGroup
	for range min(4, len(prices)) {
		wg.Go(func() {
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				result, err := enrich(ctx, prices[i])
				if err != nil {
					failed.Do(func() {
						firstError = fmt.Errorf("%s：%w", prices[i].Model, err)
						cancel()
					})
					return
				}
				results[i] = result
			}
		})
	}
schedule:
	for i := range prices {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break schedule
		}
	}
	close(jobs)
	wg.Wait()
	if firstError != nil {
		return nil, firstError
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var out []Price
	for _, result := range results {
		out = append(out, result...)
	}
	return out, nil
}
