package officialpricing_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	pricing "github.com/MIEnchating/sub2api-console/backend/internal/officialpricing"
)

func TestKimiConsolidatedPricingWithoutModelIndexLinksReturnsAllPublishedPrices(t *testing.T) {
	// Official consolidated table captured on 2026-09-14; no live requests.
	page := string(fixture(t, "kimi-chat.md"))
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		var body string
		switch r.URL.String() {
		case "https://platform.kimi.com/docs/llms.txt":
			body = "- [模型推理价格说明](https://platform.kimi.com/docs/pricing/chat.md)"
		case "https://platform.kimi.com/docs/pricing/chat.md":
			body = page
		default:
			return nil, fmt.Errorf("test refuses external endpoint %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	prices, err := pricing.Fetch(context.Background(), client, pricing.Provider{ID: "kimi", URL: pricing.KimiURL})
	if err != nil {
		t.Fatal(err)
	}
	if len(prices) != 4 {
		t.Fatalf("expected all four published models, got %d", len(prices))
	}
	for _, expected := range []struct{ model, input, output, cache string }{
		{"kimi-k3", "0.00002", "0.0001", "0.000002"},
		{"kimi-k2.7-code", "0.0000065", "0.000027", "0.0000013"},
		{"kimi-k2.7-code-highspeed", "0.000013", "0.000054", "0.0000026"},
		{"kimi-k2.6", "0.0000065", "0.000027", "0.0000011"},
	} {
		price := find(t, prices, expected.model)
		if price.InputPrice != expected.input || price.OutputPrice != expected.output || price.CacheReadPrice != expected.cache || price.SourceURL != pricing.KimiURL {
			t.Errorf("consolidated official price not preserved: %+v", price)
		}
	}
}

func TestMiniMaxDomainMigrationReadsCanonicalOfficialPage(t *testing.T) {
	page := string(fixture(t, "minimax.md"))
	const canonicalURL = "https://platform.minimax.cn/docs/guides/pricing-paygo"
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		switch r.URL.String() {
		case "https://platform.minimaxi.com/docs/guides/pricing-paygo.md":
			return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{canonicalURL + ".md"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		case canonicalURL + ".md":
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(page))}, nil
		default:
			return nil, fmt.Errorf("test refuses external endpoint %s", r.URL)
		}
	})}
	prices, err := pricing.Fetch(context.Background(), client, pricing.Provider{ID: "minimax", URL: pricing.MiniMaxURL})
	if err != nil {
		t.Fatal(err)
	}
	price := find(t, prices, "MiniMax-M3")
	if price.SourceURL != canonicalURL || price.InputPrice != "0.0000021" || len(price.Tiers) != 2 {
		t.Fatalf("canonical official price not preserved: %+v", price)
	}
}

func TestCurrentOfficialSourcesRejectCrossOriginAndInsecureRedirects(t *testing.T) {
	for _, provider := range []pricing.Provider{{ID: "kimi", URL: pricing.KimiURL}, {ID: "minimax", URL: pricing.MiniMaxURL}} {
		for _, location := range []string{"https://other.test/prices.md", strings.Replace(provider.URL, "https://", "http://", 1)} {
			t.Run(provider.ID+"/"+location, func(t *testing.T) {
				client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
					if r.URL.String() != provider.URL+".md" {
						t.Errorf("followed disallowed redirect: %s", r.URL)
						return nil, fmt.Errorf("test refuses external endpoint %s", r.URL)
					}
					return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": []string{location}}, Body: io.NopCloser(strings.NewReader(""))}, nil
				})}
				if _, err := pricing.Fetch(context.Background(), client, provider); err == nil || !strings.Contains(err.Error(), "跳转超出允许范围") {
					t.Fatalf("unsafe redirect was not rejected: %v", err)
				}
			})
		}
	}
}
