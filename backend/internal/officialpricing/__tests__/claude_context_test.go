package officialpricing_test

import (
	"context"
	pricing "github.com/MIEnchating/sub2api-console/backend/internal/officialpricing"
	"strings"
	"testing"
)

func TestClaudePromptContextPricesPreserveThresholdAndBothCacheDurations(t *testing.T) {
	prices, err := pricing.Fetch(context.Background(), westernClient(t, "claude", nil), westernProvider("claude"))
	if err != nil {
		t.Fatal(err)
	}
	p := find(t, prices, "claude-haiku-5-5")
	want := `len <= 100000 ? tier("输入 ≤ 100000 Token", p * 0.1 + c * 0.5 + cr * 0.01 + cc * 0.125 + cc1h * 0.2) : tier("输入 > 100000 Token", p * 0.5 + c * 2.5 + cr * 0.05 + cc * 0.625 + cc1h * 1)`
	if p.BillingExpr != want || len(p.Tiers) != 2 || p.Tiers[0].Condition != "len <= 100000" || p.Tiers[1].Condition != "len > 100000" || p.Tiers[1].CacheWrite1hPrice != "0.000001" {
		t.Fatalf("context prices lost: %+v", p)
	}
	if p.InputPrice != "0.0000001" || p.CacheWrite1hPrice != "0.0000002" {
		t.Fatalf("base rates lost: %+v", p)
	}
	if find(t, prices, "claude-sonnet-5-5").CacheReadPrice != "0.0000001" {
		t.Fatal("new Sonnet price lost")
	}
}

func TestClaudeIncompleteOrUnknownContextPricesAreRejected(t *testing.T) {
	for _, tc := range []struct{ name, old, next string }{
		{"mismatched thresholds", "over 100,000 tokens", "over 200,000 tokens"},
		{"unknown modifier", "for prompts up to 100,000 tokens", "for regional prompts"},
		{"invalid threshold", "100,000 tokens", "0 tokens"},
		{"missing lower tier", "| Claude Haiku 5.5 (for prompts up to", "REMOVE"},
		{"missing upper tier", "| Claude Haiku 5.5 (for prompts over", "REMOVE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := westernClient(t, "claude", func(file, raw string) string {
				if file != "claude.md" {
					return raw
				}
				if tc.next == "REMOVE" {
					var kept []string
					for _, line := range strings.Split(raw, "\n") {
						if !strings.HasPrefix(line, tc.old) {
							kept = append(kept, line)
						}
					}
					return strings.Join(kept, "\n")
				}
				return strings.Replace(raw, tc.old, tc.next, 1)
			})
			if _, err := pricing.Fetch(context.Background(), client, westernProvider("claude")); err == nil {
				t.Fatal("invalid context prices accepted")
			}
		})
	}
}

func TestClaudeStaticAndContextRowsForSameModelAreRejected(t *testing.T) {
	for _, position := range []string{"before", "after"} {
		t.Run(position, func(t *testing.T) {
			client := westernClient(t, "claude", func(file, raw string) string {
				if file != "claude.md" {
					return raw
				}
				for _, row := range strings.Split(raw, "\n") {
					if strings.HasPrefix(row, "| Claude Haiku 5.5 (for prompts up to") {
						plain := strings.Replace(row, " (for prompts up to 100,000 tokens)", "", 1)
						replacement := plain + "\n" + row
						if position == "after" {
							replacement = row + "\n" + plain
						}
						return strings.Replace(raw, row, replacement, 1)
					}
				}
				t.Fatal("missing Haiku fixture row")
				return raw
			})
			if _, err := pricing.Fetch(context.Background(), client, westernProvider("claude")); err == nil {
				t.Fatal("mixed static and context pricing accepted")
			}
		})
	}
}

func TestClaudeReversedContextRowsKeepLowerTierFirst(t *testing.T) {
	client := westernClient(t, "claude", func(file, raw string) string {
		if file != "claude.md" {
			return raw
		}
		lines := strings.Split(raw, "\n")
		lower, upper := -1, -1
		for i, line := range lines {
			if strings.HasPrefix(line, "| Claude Haiku 5.5 (for prompts up to") {
				lower = i
			}
			if strings.HasPrefix(line, "| Claude Haiku 5.5 (for prompts over") {
				upper = i
			}
		}
		if lower < 0 || upper < 0 {
			t.Fatal("missing Haiku fixture rows")
		}
		lines[lower], lines[upper] = lines[upper], lines[lower]
		return strings.Join(lines, "\n")
	})
	prices, err := pricing.Fetch(context.Background(), client, westernProvider("claude"))
	if err != nil {
		t.Fatal(err)
	}
	price := find(t, prices, "claude-haiku-5-5")
	if len(price.Tiers) != 2 || price.Tiers[0].Condition != "len <= 100000" || price.InputPrice != "0.0000001" {
		t.Fatalf("reordered rows changed base tier: %+v", price)
	}
}
