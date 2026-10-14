package officialpricing_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	pricing "github.com/MIEnchating/sub2api-console/backend/internal/officialpricing"
)

const geminiURL = "https://ai.google.dev/gemini-api/docs/pricing"

func fetchGeminiFixture(t *testing.T, raw string) ([]pricing.Price, error) {
	t.Helper()
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != geminiURL || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Errorf("unexpected public documentation request: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(raw))}, nil
	})}
	return pricing.Fetch(context.Background(), client, pricing.Provider{ID: "gemini", URL: geminiURL})
}

func TestGeminiOfficialProviderRecognizesPublishedFamily(t *testing.T) {
	if pricing.ProviderID("gemini-4-argon") != "gemini" || pricing.ProviderID("GEMINI-2.5-PRO") != "gemini" || pricing.ProviderID("reseller/gemini-4-argon") != "" {
		t.Fatal("Gemini provider matching is missing or guesses reseller aliases")
	}
}

func TestGeminiStandardPricesPreserveDatedRatesAndBlockUnsafeFixedSync(t *testing.T) {
	prices, err := fetchGeminiFixture(t, string(fixture(t, "gemini.html")))
	if err != nil {
		t.Fatal(err)
	}
	p := find(t, prices, "gemini-4-argon")
	if len(prices) != 4 || len(p.Tiers) != 2 || p.SourceURL != geminiURL {
		t.Fatalf("wrong published models or tiers: %+v", prices)
	}
	if p.Tiers[0].InputPrice != "0.000002" || p.Tiers[0].OutputPrice != "0.00001" || p.Tiers[0].CacheReadPrice != "0.0000001" || p.Tiers[1].InputPrice != "0.000004" || p.Tiers[1].OutputPrice != "0.00002" || p.Tiers[1].CacheReadPrice != "0.0000002" {
		t.Fatalf("dated prices lost: %+v", p)
	}
	if !strings.Contains(p.Tiers[0].Label, "2026-11-30") || !strings.Contains(p.Tiers[1].Label, "2026-12-01") || p.SyncError == "" || p.BillingExpr != "" || p.CacheWritePrice != "" {
		t.Fatalf("date conditions flattened or implicit cache treated as writes: %+v", p)
	}
}

func TestGeminiContextTiersUsePromptLengthAndPublishedAliases(t *testing.T) {
	prices, err := fetchGeminiFixture(t, string(fixture(t, "gemini.html")))
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"gemini-3.1-pro-preview", "gemini-3.1-pro-preview-customtools"} {
		p := find(t, prices, model)
		if len(p.Tiers) != 2 || p.Tiers[0].Condition != "len <= 200000" || p.Tiers[1].Condition != "len > 200000" || p.Tiers[1].OutputPrice != "0.000018" || p.Tiers[1].CacheReadPrice != "0.0000004" || !strings.Contains(p.BillingExpr, "len <= 200000") || p.CacheWritePrice != "" {
			t.Fatalf("context tiers lost: %+v", p)
		}
	}
}

func TestGeminiTextRatesExcludeAudioAndHourlyStorageCharges(t *testing.T) {
	prices, err := fetchGeminiFixture(t, string(fixture(t, "gemini.html")))
	if err != nil {
		t.Fatal(err)
	}
	p := find(t, prices, "gemini-3.1-flash-lite")
	if p.InputPrice != "0.00000025" || p.OutputPrice != "0.0000015" || p.CacheReadPrice != "0.000000025" || p.CacheWritePrice != "" || !strings.Contains(p.Scope, "文本") {
		t.Fatalf("wrong text scope: %+v", p)
	}
}

func TestGeminiIncompleteOrChangedPricesRejectPartialCatalog(t *testing.T) {
	raw := string(fixture(t, "gemini.html"))
	for _, tc := range []struct{ name, from, to string }{
		{"missing future output", "$20.00 starting December 1, 2026.", ""},
		{"mismatched future date", "$20.00 starting December 1, 2026.", "$20.00 starting December 2, 2026."},
		{"wrong unit", "Paid Tier, per 1M tokens in USD", "Paid Tier, per 1K tokens in USD"},
		{"invalid amount", "$2.00 through", "$invalid through"},
		{"missing context rate", "$18.00, prompts &gt; 200k", ""},
		{"different context threshold", "$18.00, prompts &gt; 200k", "$18.00, prompts &gt; 300k"},
		{"missing standard section", ">Standard<", ">Changed<"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prices, err := fetchGeminiFixture(t, strings.Replace(raw, tc.from, tc.to, 1))
			if err == nil || len(prices) != 0 {
				t.Fatalf("partial catalog accepted: prices=%+v err=%v", prices, err)
			}
		})
	}
}
