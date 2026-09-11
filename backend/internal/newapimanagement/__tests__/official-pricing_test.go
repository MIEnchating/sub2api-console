package newapimanagement_test

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
)

// Captured official table and footnotes, 2026-09-10; tests never fetch the live page.
//
//go:embed testdata/deepseek-pricing.html
var officialPage string

type catalogStore struct{ *configstore.Store }

func (s *catalogStore) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return configstore.TargetSettings{BaseURL: "https://sub2api.test", AdminKey: "private-test-key", TimeoutSeconds: 1}, nil
}

func (s *catalogStore) NewAPIPlatform(context.Context, string) (*configstore.NewAPIPlatform, error) {
	return &configstore.NewAPIPlatform{ID: "test", BaseURL: "https://newapi.test", AdminKey: "private-test-key", UserID: "1"}, nil
}

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func setupCatalog(t *testing.T, page *string, overrides ...transport) *newapimanagement.Service {
	t.Helper()
	db, err := configstore.Open(filepath.Join(t.TempDir(), "prices.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		var body string
		if len(overrides) > 0 {
			response, err := overrides[0](r)
			if response != nil || err != nil {
				return response, err
			}
		}
		switch r.URL.Host {
		case "api-docs.deepseek.com":
			if r.URL.Path != "/zh-cn/quick_start/pricing" && r.URL.Path != "/zh-cn/quick_start/pricing/" {
				t.Errorf("must request Chinese official price page, got %s", r.URL.Path)
			}
			if r.Header.Get("Authorization") != "" || r.Header.Get("X-API-Key") != "" {
				t.Error("credentials sent to official public page")
			}
			if r.URL.Path == "/zh-cn/quick_start/pricing" {
				return &http.Response{StatusCode: 308, Header: http.Header{"Location": []string{"/zh-cn/quick_start/pricing/"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
			}
			body = *page
		case "raw.githubusercontent.com":
			body = `{"deepseek-v4-flash":{"input_cost_per_token":0.00000044,"output_cost_per_token":0.00000132,"cache_read_input_token_cost":0.000000014},"other-model":{"input_cost_per_token":0.000002,"output_cost_per_token":0.000004}}`
		case "newapi.test":
			body = `{"success":true,"data":[]}`
			if strings.Contains(r.URL.Path, "models_enabled") {
				body = `{"success":true,"data":["deepseek-v4-flash-0731"]}`
			}
		case "sub2api.test":
			body = `{"code":0,"data":{"found":true,"input_price":0.00000022,"output_price":0.00000066}}`
		default:
			return nil, fmt.Errorf("test refuses external endpoint %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	return newapimanagement.New(&catalogStore{db}, nil, client, nil, nil)
}

func priceByName(t *testing.T, catalog newapimanagement.ModelPriceCatalog, name string) newapimanagement.Sub2APIModelPrice {
	t.Helper()
	for _, price := range catalog.Models {
		if price.Model == name {
			return price
		}
	}
	t.Fatalf("missing model %s", name)
	return newapimanagement.Sub2APIModelPrice{}
}

func TestOfficialPricesReplaceOldCatalogAndExplicitAliases(t *testing.T) {
	page := officialPage
	service := setupCatalog(t, &page)
	catalog, err := service.ModelPriceCatalog(context.Background(), "test", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"deepseek-flash", "deepseek-v4-flash", "deepseek-v4-flash-vision-exp"} {
		price := priceByName(t, catalog, name)
		if price.Source != "official" || price.SourceURL != "https://api-docs.deepseek.com/zh-cn/quick_start/pricing" || price.InputPrice != "0.000001" || price.OutputPrice != "0.000004" || price.CacheReadPrice != "0.00000002" {
			t.Errorf("%s did not preserve official off-peak values: %+v", name, price)
		}
	}
	price := priceByName(t, catalog, "deepseek-flash")
	raw, _ := json.Marshal(price)
	var fields map[string]any
	_ = json.Unmarshal(raw, &fields)
	if fields["time_pricing"] == nil {
		t.Fatal("official peak schedule was discarded")
	}
	if len(catalog.MissingModels) != 1 || catalog.MissingModels[0] != "deepseek-v4-flash-0731" {
		t.Fatalf("unpublished model must stay missing: %+v", catalog.MissingModels)
	}

	if other := priceByName(t, catalog, "other-model"); other.Source != "remote" || other.InputPrice != "0.000002" {
		t.Errorf("unrelated source modified: %+v", other)
	}
}

func TestOfficialRefreshReadsChangedPricesWithoutCurrencyConversion(t *testing.T) {
	page := officialPage
	service := setupCatalog(t, &page)
	if _, err := service.ModelPriceCatalog(context.Background(), "test", false); err != nil {
		t.Fatal(err)
	}
	page = strings.ReplaceAll(page, ">1元<", ">1.7元<")
	cached, err := service.ModelPriceCatalog(context.Background(), "test", false)
	if err != nil {
		t.Fatal(err)
	}
	if priceByName(t, cached, "deepseek-flash").InputPrice != "0.000001" {
		t.Fatal("fresh cache was not reused")
	}
	fresh, err := service.ModelPriceCatalog(context.Background(), "test", true)
	if err != nil {
		t.Fatal(err)
	}
	if priceByName(t, fresh, "deepseek-flash").InputPrice != "0.0000017" {
		t.Fatal("official value was not refreshed verbatim")
	}
}

func TestInvalidOfficialRefreshRetainsLastSuccessAndReportsStale(t *testing.T) {
	page := officialPage
	service := setupCatalog(t, &page)
	if _, err := service.ModelPriceCatalog(context.Background(), "test", true); err != nil {
		t.Fatal(err)
	}
	page = `<html>temporarily unavailable</html>`
	catalog, err := service.ModelPriceCatalog(context.Background(), "test", true)
	if err != nil {
		t.Fatal(err)
	}
	if !catalog.Stale || !strings.Contains(catalog.Warning, "官方") {
		t.Fatalf("official failure hidden: %+v", catalog)
	}
	if priceByName(t, catalog, "deepseek-v4-flash").InputPrice != "0.000001" {
		t.Fatal("official cache replaced by obsolete remote price")
	}
}
