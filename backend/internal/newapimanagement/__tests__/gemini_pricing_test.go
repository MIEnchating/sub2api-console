package newapimanagement_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	pricing "github.com/MIEnchating/sub2api-console/backend/internal/officialpricing"
)

func TestGeminiPlatformModelMissingFromRemoteCardLoadsOfficialPrices(t *testing.T) {
	raw, err := os.ReadFile("../../officialpricing/__tests__/testdata/gemini.html")
	if err != nil {
		t.Fatal(err)
	}
	page := officialPage
	service := setupCatalog(t, &page, transport(func(r *http.Request) (*http.Response, error) {
		var body string
		switch {
		case r.URL.String() == pricing.GeminiURL:
			body = string(raw)
		case r.URL.Host == "newapi.test" && strings.Contains(r.URL.Path, "models_enabled"):
			body = `{"success":true,"data":["gemini-4-argon","GEMINI-4-ARGON","gemini-4-argon-unpublished"]}`
		default:
			return nil, nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
	catalog, err := service.ModelPriceCatalog(context.Background(), "test", true)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Stale || catalog.Warning != "" {
		t.Fatalf("official lookup failed: %+v", catalog)
	}
	for _, name := range []string{"gemini-4-argon", "GEMINI-4-ARGON"} {
		p := priceByName(t, catalog, name)
		if p.Source != "official" || p.SourceURL != pricing.GeminiURL || p.Provider != "gemini" || len(p.PriceTiers) != 2 || p.SyncError == "" || p.ModelRatio != "" || p.CompletionRatio != "" {
			t.Fatalf("dated official reference lost its source or write guard: %+v", p)
		}
	}
	if len(catalog.MissingModels) != 1 || catalog.MissingModels[0] != "gemini-4-argon-unpublished" {
		t.Fatalf("unpublished alias was guessed: %+v", catalog.MissingModels)
	}
	p := priceByName(t, catalog, "gemini-3.1-pro-preview")
	if p.SyncError != "" || p.ModelRatio == "" || !strings.Contains(p.BillingExpr, "len <= 200000") {
		t.Fatalf("context-based prices cannot be synchronized: %+v", p)
	}
}

func TestGeminiFirstOfficialFailureDoesNotPresentThirdPartyPriceAsOfficial(t *testing.T) {
	page := "maintenance"
	service := setupOfficialRefresh(t, pricing.GeminiURL, "gemini-4-argon", &page)
	catalog, err := service.ModelPriceCatalog(context.Background(), "test", true)
	if err != nil {
		t.Fatal(err)
	}
	if !catalog.Stale || !strings.Contains(catalog.Warning, "Gemini") {
		t.Fatalf("missing official failure warning: %+v", catalog)
	}
	for _, p := range catalog.Models {
		if p.Model == "gemini-4-argon" {
			t.Fatalf("third-party price survived official failure: %+v", p)
		}
	}
}
