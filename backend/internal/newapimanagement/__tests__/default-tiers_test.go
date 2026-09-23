package newapimanagement_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
)

const defaultTierPricing = `{"input_price":0.000002,"output_price":0.00001,"cache_read_price":0.0000002,"intervals":[{"min_tokens":0,"max_tokens":272000,"tier_label":"标准","input_price":0.000002,"output_price":0.00001,"cache_write_price":0.0000025,"cache_write_1h_price":0.000004,"cache_read_price":0.0000002},{"min_tokens":272000,"max_tokens":null,"tier_label":"长上下文","input_price":0.000004,"output_price":0.000015,"cache_write_price":0.000005,"cache_write_1h_price":0.000008,"cache_read_price":0.0000004}]}`

func defaultPriceService(t *testing.T, plaza *string) *newapimanagement.Service {
	t.Helper()
	page := officialPage
	return setupCatalog(t, &page, transport(func(r *http.Request) (*http.Response, error) {
		body := ""
		switch r.URL.Path {
		case "/api/channel/models_enabled":
			body = `{"success":true,"data":["fallback-model"]}`
		case "/api/v1/model-plaza":
			if r.Header.Get("Authorization") != "" || r.Header.Get("X-API-Key") != "" {
				t.Error("public model plaza must not receive admin credentials")
			}
			body = *plaza
		case "/api/v1/admin/channels/model-pricing":
			body = `{"code":0,"data":{"found":true,"input_price":0.000002,"output_price":0.00001}}`
		default:
			return nil, nil
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
}

func TestDefaultPriceCatalogPreservesOfficialTiersAndRetainsThemOnFailure(t *testing.T) {
	plaza := `{"code":0,"data":{"groups":[{"models":[{"name":"fallback-model","official_pricing":` + defaultTierPricing + `,"pricing":{"input_price":9,"output_price":99}}]}]}}`
	service := defaultPriceService(t, &plaza)
	catalog, err := service.ModelPriceCatalog(context.Background(), "test", true)
	if err != nil {
		t.Fatal(err)
	}
	price := priceByName(t, catalog, "fallback-model")
	want := `len <= 272000 ? tier("标准", p * 2 + c * 10 + cr * 0.2 + cc * 2.5 + cc1h * 4) : tier("长上下文", p * 4 + c * 15 + cr * 0.4 + cc * 5 + cc1h * 8)`
	if price.Source != "sub2api" || len(price.PriceTiers) != 2 || price.BillingExpr != want {
		t.Fatalf("default tiers lost: %+v", price)
	}
	plaza = `{"code":503,"message":"unavailable"}`
	stale, err := service.ModelPriceCatalog(context.Background(), "test", true)
	if err != nil {
		t.Fatal(err)
	}
	if stale.Stale || stale.Warning == "" || priceByName(t, stale, "fallback-model").BillingExpr != want {
		t.Fatalf("failed read flattened cached tiers: %+v", stale)
	}
	assertDefaultSyncError(t, priceByName(t, stale, "fallback-model"), true)
	if priceByName(t, stale, "other-model").ModelRatio == "" {
		t.Fatal("unrelated remote price became unavailable")
	}
	plaza = `{"code":0,"data":{"groups":[{"models":[{"name":"fallback-model","official_pricing":` + defaultTierPricing + `}]}]}}`
	fresh, err := service.ModelPriceCatalog(context.Background(), "test", true)
	if err != nil {
		t.Fatal(err)
	}
	assertDefaultSyncError(t, priceByName(t, fresh, "fallback-model"), false)
	if fresh.Stale || fresh.Warning != "" {
		t.Fatalf("successful refresh did not clear partial failure: %+v", fresh)
	}
}

func TestIncompleteDefaultTiersCannotBeSyncedAsFlatPrices(t *testing.T) {
	for _, tc := range []struct{ name, reference string }{
		{"missing reference", `null`},
		{"negative tier price", strings.Replace(defaultTierPricing, `"input_price":0.000004`, `"input_price":-0.000004`, 1)},
		{"gap between tiers", strings.Replace(defaultTierPricing, `"min_tokens":272000`, `"min_tokens":272001`, 1)},
		{"overlap between tiers", strings.Replace(defaultTierPricing, `"min_tokens":272000`, `"min_tokens":271999`, 1)},
		{"missing tier output", strings.Replace(defaultTierPricing, `"output_price":0.000015`, `"output_price":null`, 1)},
		{"bounded last tier", strings.Replace(defaultTierPricing, `"max_tokens":null`, `"max_tokens":500000`, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plaza := `{"code":0,"data":{"groups":[{"models":[{"name":"fallback-model","official_pricing":` + tc.reference + `,"pricing":{"input_price":9,"output_price":99}}]}]}}`
			catalog, err := defaultPriceService(t, &plaza).ModelPriceCatalog(context.Background(), "test", true)
			if err != nil {
				t.Fatal(err)
			}
			price := priceByName(t, catalog, "fallback-model")
			if catalog.Stale || price.ModelRatio != "" || price.BillingExpr != "" || price.InputPrice != "0.000002" {
				t.Fatalf("incomplete reference became writable: %+v", price)
			}
			assertDefaultSyncError(t, price, true)
			if !strings.Contains(catalog.Warning, "fallback-model") {
				t.Fatalf("warning does not identify affected model: %s", catalog.Warning)
			}
		})
	}
}

func assertDefaultSyncError(t *testing.T, price newapimanagement.Sub2APIModelPrice, want bool) {
	t.Helper()
	raw, err := json.Marshal(price)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		SyncError string `json:"sync_error"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	if (response.SyncError != "") != want {
		t.Fatalf("sync error = %q, want error %v", response.SyncError, want)
	}
}

func TestSingleDefaultTierPreservesZeroPriceAndExactDecimal(t *testing.T) {
	plaza := `{"code":0,"data":{"groups":[{"models":[{"name":"fallback-model","official_pricing":{"input_price":0.000001234567890123,"output_price":0,"intervals":[{"min_tokens":0,"max_tokens":null,"input_price":0.000001234567890123,"output_price":0,"cache_read_price":0}]}}]}]}}`
	catalog, err := defaultPriceService(t, &plaza).ModelPriceCatalog(context.Background(), "test", true)
	if err != nil {
		t.Fatal(err)
	}
	price := priceByName(t, catalog, "fallback-model")
	if catalog.Stale || !strings.Contains(price.BillingExpr, "p * 1.234567890123 + c * 0 + cr * 0") {
		t.Fatalf("precision or zero lost: %+v", price)
	}
}
