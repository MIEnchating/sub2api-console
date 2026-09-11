package newapimanagement_test

import (
	"context"
	pricing "github.com/MIEnchating/sub2api-console/backend/internal/officialpricing"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestCatalogUsesAllOfficialProvidersAndRetainsFailedProviderCache(t *testing.T) {
	fixtures := map[string]string{}
	for _, name := range []string{"kimi.md", "minimax.md", "glm.md", "qwen.html"} {
		b, e := os.ReadFile("../../officialpricing/__tests__/testdata/" + name)
		if e != nil {
			t.Fatal(e)
		}
		fixtures[name] = string(b)
	}
	var mu sync.Mutex
	hits := map[string]int{}
	glmFailed := false
	page := officialPage
	service := setupCatalog(t, &page, transport(func(r *http.Request) (*http.Response, error) {
		body := ""
		switch r.URL.Host {
		case "raw.githubusercontent.com":
			body = `{"kimi-k3":{"input_cost_per_token":1,"output_cost_per_token":2},"minimax-m3":{"input_cost_per_token":1,"output_cost_per_token":2},"glm-4.7":{"input_cost_per_token":1,"output_cost_per_token":2},"qwen-turbo":{"input_cost_per_token":1,"output_cost_per_token":2}}`
		case "platform.kimi.com":
			body = fixtures["kimi.md"]
			if strings.HasSuffix(r.URL.Path, "llms.txt") {
				body = "https://platform.kimi.com/docs/pricing/chat-k3.md"
			}
		case "platform.minimaxi.com":
			body = fixtures["minimax.md"]
		case "docs.bigmodel.cn":
			body = fixtures["glm.md"]
			if glmFailed {
				body = "maintenance"
			}
		case "help.aliyun.com":
			body = fixtures["qwen.html"]
		default:
			return nil, nil
		}
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-API-Key") != "" {
			t.Fatal("public request contains credentials")
		}
		mu.Lock()
		hits[r.URL.Host]++
		mu.Unlock()
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
	catalog, e := service.ModelPriceCatalog(context.Background(), "test", true)
	if e != nil {
		t.Fatal(e)
	}
	if catalog.Stale {
		t.Fatal(catalog.Warning)
	}
	for _, name := range []string{"kimi-k3", "minimax-m3", "glm-4.7", "qwen-turbo"} {
		p := priceByName(t, catalog, name)
		if p.Source != "official" || p.SourceURL == "" {
			t.Fatalf("not official: %+v", p)
		}
	}
	p := priceByName(t, catalog, "glm-4.7")
	if len(p.PriceTiers) != 3 || !strings.Contains(p.BillingExpr, "c < 200") {
		t.Fatal("lost GLM tier contract")
	}
	if priceByName(t, catalog, "kimi-k3").TimePricing != nil {
		t.Fatal("static model assigned a time schedule")
	}
	mu.Lock()
	before := hits["platform.kimi.com"]
	mu.Unlock()
	if _, e := service.ModelPriceCatalog(context.Background(), "test", false); e != nil {
		t.Fatal(e)
	}
	mu.Lock()
	after := hits["platform.kimi.com"]
	mu.Unlock()
	if after != before {
		t.Fatal("fresh official cache was fetched again")
	}
	glmFailed = true
	stale, e := service.ModelPriceCatalog(context.Background(), "test", true)
	if e != nil {
		t.Fatal(e)
	}
	if !stale.Stale || !strings.Contains(stale.Warning, "GLM") || !strings.Contains(stale.Warning, "格式已变更") {
		t.Fatalf("missing actionable failure: %s", stale.Warning)
	}
	if priceByName(t, stale, "glm-4.7").BillingExpr != p.BillingExpr {
		t.Fatal("failed provider lost last successful tiers")
	}
	if priceByName(t, stale, "qwen-turbo").SourceURL != pricing.QwenURL {
		t.Fatal("other provider was lost")
	}
}
