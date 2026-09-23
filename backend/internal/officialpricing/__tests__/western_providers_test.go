package officialpricing_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"testing"

	pricing "github.com/MIEnchating/sub2api-console/backend/internal/officialpricing"
)

func westernClient(t *testing.T, provider string, change func(string, string) string) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-API-Key") != "" || r.Header.Get("Cookie") != "" {
			t.Error("credentials sent to public pricing documentation")
		}
		file := provider + ".md"
		switch {
		case r.URL.Host == "docs.x.ai" && r.URL.Path == "/developers/models.md":
		case r.URL.Host == "developers.openai.com" && r.URL.Path == "/api/docs/pricing.md":
		case r.URL.Host == "platform.claude.com" && r.URL.Path == "/docs/en/about-claude/pricing.md":
		case r.URL.Host == "developers.openai.com" && strings.HasPrefix(r.URL.Path, "/api/docs/models/"):
			file = "model-details/" + path.Base(r.URL.Path)
		case r.URL.Host == "platform.claude.com" && strings.HasPrefix(r.URL.Path, "/docs/en/models/"):
			file = "model-details/claude-" + path.Base(path.Dir(r.URL.Path)) + ".md"
		default:
			return nil, fmt.Errorf("test refuses endpoint %s", r.URL)
		}
		body := string(fixture(t, file))
		if change != nil {
			body = change(file, body)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
}

func westernProvider(id string) pricing.Provider {
	urls := map[string]string{
		"grok":   "https://docs.x.ai/developers/models",
		"claude": "https://platform.claude.com/docs/en/about-claude/pricing",
		"openai": "https://developers.openai.com/api/docs/pricing",
	}
	return pricing.Provider{ID: id, URL: urls[id]}
}

func TestWesternModelsUseOfficialProvider(t *testing.T) {
	for model, want := range map[string]string{"grok-4.7": "grok", "grok-imagine-image-2.0": "grok", "claude-opus-5-5": "claude", "GPT-5.4": "openai", "o3": "openai", "o4-mini": "openai", "chat-latest": "openai", "vendor/gpt-5": "", "other-model": ""} {
		if got := pricing.ProviderID(model); got != want {
			t.Errorf("ProviderID(%q) = %q, want %q", model, got, want)
		}
	}
}

func TestGrokOfficialPricesPreserveInclusiveLongContextThreshold(t *testing.T) {
	prices, err := pricing.Fetch(context.Background(), westernClient(t, "grok", nil), westernProvider("grok"))
	if err != nil {
		t.Fatal(err)
	}
	p := find(t, prices, "grok-4.7")
	if p.InputPrice != "0.000002" || p.CacheReadPrice != "0.0000005" || len(p.Tiers) != 2 || p.Tiers[0].Condition != "len < 200000" || p.Tiers[1].OutputPrice != "0.000012" {
		t.Fatalf("official Grok tiers lost: %+v", p)
	}
	image := find(t, prices, "grok-imagine-image-2.0")
	if image.InputPrice != "" || !strings.Contains(image.Scope, "0.04") || !strings.Contains(image.Scope, "图片") {
		t.Fatalf("per-image price flattened to token price: %+v", image)
	}
}

func TestClaudeOfficialPricesPreserveBothCacheWriteDurationsAndPublishedAliases(t *testing.T) {
	prices, err := pricing.Fetch(context.Background(), westernClient(t, "claude", nil), westernProvider("claude"))
	if err != nil {
		t.Fatal(err)
	}
	p := find(t, prices, "claude-opus-5-5")
	if p.InputPrice != "0.000004" || p.CacheReadPrice != "0.0000002" || !strings.Contains(p.BillingExpr, "cc * 5 + cc1h * 8") {
		t.Fatalf("Claude cache prices lost: %+v", p)
	}
	alias := find(t, prices, "claude-sonnet-4-5")
	dated := find(t, prices, "claude-sonnet-4-5-20250929")
	if alias.BillingExpr != dated.BillingExpr || alias.CacheWritePrice != "0.00000375" {
		t.Fatal("published alias lost")
	}
	for _, p := range prices {
		if p.Model == "claude-sonnet-4-5-20990101" {
			t.Fatal("invented dated model")
		}
	}
}

func TestOpenAIOfficialPricesUseStandardTierAndVerifiedContextRules(t *testing.T) {
	prices, err := pricing.Fetch(context.Background(), westernClient(t, "openai", nil), westernProvider("openai"))
	if err != nil {
		t.Fatal(err)
	}
	p := find(t, prices, "gpt-6-sol")
	if p.InputPrice != "0.000002" || p.CacheWritePrice != "0.0000025" || len(p.Tiers) != 2 || p.Tiers[0].Condition != "len <= 272000" || p.Tiers[1].OutputPrice != "0.000015" {
		t.Fatalf("wrong OpenAI pricing: %+v", p)
	}
	if find(t, prices, "gpt-5.4-2026-03-05").InputPrice != "0.0000025" {
		t.Fatal("published snapshot lost")
	}
	if find(t, prices, "gpt-5.3-codex").InputPrice != "0.00000175" {
		t.Fatal("specialized standard pricing lost")
	}
	if find(t, prices, "gpt-5").InputPrice != "0.00000125" {
		t.Fatal("batch or fast price replaced standard price")
	}
	image := find(t, prices, "gpt-image-2")
	if image.ImageInputPrice != "0.000008" || image.ImageOutputPrice != "0.00003" || image.SyncError == "" || !strings.Contains(image.Scope, "图片缓存读取 2") {
		t.Fatalf("image modality prices lost or flattened: %+v", image)
	}
}

func TestWesternIncompletePricesFailInsteadOfFlattening(t *testing.T) {
	for _, tc := range []struct{ name, provider, file, old, next string }{
		{"grok missing long tier", "grok", "grok.md", "| grok-4.7 (≥ 200k prompt tokens) | 500k | $4.00 | $1.00 | $12.00 |", ""},
		{"grok missing cache price", "grok", "grok.md", "$0.50", "invalid"},
		{"claude missing one hour price", "claude", "claude.md", "$8 / MTok", "invalid"},
		{"claude unpublished identity", "claude", "model-details/claude-opus-5-5.md", "claude-opus-5-5", "other-model"},
		{"openai missing long output", "openai", "openai.md", "$75.00 |", "- |"},
		{"openai missing context rule", "openai", "model-details/gpt-6-sol.md", "more than 272K", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := westernClient(t, tc.provider, func(file, raw string) string {
				if file == tc.file {
					return strings.Replace(raw, tc.old, tc.next, 1)
				}
				return raw
			})
			if _, err := pricing.Fetch(context.Background(), client, westernProvider(tc.provider)); err == nil {
				t.Fatal("incomplete official price accepted")
			}
		})
	}
}
