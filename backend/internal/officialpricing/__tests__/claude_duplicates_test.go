package officialpricing_test

import (
	"context"
	"strings"
	"testing"

	pricing "github.com/MIEnchating/sub2api-console/backend/internal/officialpricing"
)

func TestClaudeIdenticalDuplicatePricesKeepOnePublishedModelAndAlias(t *testing.T) {
	client := westernClient(t, "claude", func(file, raw string) string {
		if file == "claude.md" {
			return duplicateClaudeRow(raw, "Claude Sonnet 4.5", "", "")
		}
		return raw
	})
	prices, err := pricing.Fetch(context.Background(), client, westernProvider("claude"))
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"claude-sonnet-4-5", "claude-sonnet-4-5-20250929"} {
		count := 0
		for _, price := range prices {
			if price.Model == model {
				count++
				if price.InputPrice != "0.000003" || price.CacheWrite1hPrice != "0.000006" {
					t.Fatalf("duplicate changed published prices: %+v", price)
				}
			}
		}
		if count != 1 {
			t.Fatalf("%s appeared %d times, want once", model, count)
		}
	}
}

func TestClaudeConflictingDuplicatePricesRejectEveryPriceDimension(t *testing.T) {
	for _, old := range []string{"$3 / MTok", "$3.75 / MTok", "$6 / MTok", "$0.30 / MTok", "$15 / MTok"} {
		t.Run(old, func(t *testing.T) {
			client := westernClient(t, "claude", func(file, raw string) string {
				if file == "claude.md" {
					return duplicateClaudeRow(raw, "Claude Sonnet 4.5", old, "$99 / MTok")
				}
				return raw
			})
			if _, err := pricing.Fetch(context.Background(), client, westernProvider("claude")); err == nil {
				t.Fatal("conflicting duplicate prices accepted")
			}
		})
	}
}

func duplicateClaudeRow(raw, model, old, next string) string {
	for _, row := range strings.Split(raw, "\n") {
		if strings.HasPrefix(row, "| "+model+" ") {
			duplicate := row
			if old != "" {
				duplicate = strings.Replace(duplicate, old, next, 1)
			}
			return strings.Replace(raw, row, row+"\n"+duplicate, 1)
		}
	}
	return raw
}
