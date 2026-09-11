package newapimanagement

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type pricingTestStore struct {
	*configstore.Store
	target configstore.TargetSettings
}

func (s *pricingTestStore) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return s.target, nil
}
func (s *pricingTestStore) NewAPIPlatform(_ context.Context, id string) (*configstore.NewAPIPlatform, error) {
	if id != "primary" {
		return nil, nil
	}
	return &configstore.NewAPIPlatform{ID: id, BaseURL: "https://newapi.example", AdminKey: "newapi-secret", UserID: "1"}, nil
}

type pricingTransport func(*http.Request) (*http.Response, error)

func (f pricingTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPricingCatalogPersistsRemotePriorityAndDefaultFallback(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "prices.sqlite3")
	db, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	private := &pricingTestStore{Store: db, target: configstore.TargetSettings{BaseURL: "https://sub2api.example", AdminKey: "sub2-secret", TimeoutSeconds: 1}}
	calls := map[string]int{}
	var callsMu sync.Mutex
	client := &http.Client{Transport: pricingTransport(func(r *http.Request) (*http.Response, error) {
		key := r.URL.Host + r.URL.Path
		callsMu.Lock()
		calls[key]++
		callsMu.Unlock()
		body := ""
		switch key {
		case "raw.githubusercontent.com/Wei-Shaw/model-price-repo/main/model_prices_and_context_window.json":
			if r.Header.Get("X-API-Key") != "" || r.Header.Get("Authorization") != "" {
				t.Fatal("credential leaked to public catalog")
			}
			body = `{"remote":{"input_cost_per_token":0.000002,"output_cost_per_token":0.000006}}`
		case "newapi.example/api/option/":
			body = `{"success":true,"data":[{"key":"ModelRatio","value":"{\"remote\":1,\"fallback-model\":1}"}]}`
		case "newapi.example/api/channel/models_enabled":
			body = `{"success":true,"data":["missing","fallback-model"]}`
		case "sub2api.example/api/v1/admin/channels/model-pricing":
			if r.Header.Get("X-API-Key") != "sub2-secret" {
				t.Fatal("wrong admin credential")
			}
			if r.URL.Query().Get("model") == "remote" {
				t.Fatal("remote model must not query fallback")
			}
			body = `{"code":0,"data":{"found":false}}`
			if r.URL.Query().Get("model") == "fallback-model" {
				body = `{"code":0,"data":{"found":true,"input_price":0.00000123,"output_price":0.00000492,"cache_read_price":0,"cache_write_price":0.00000246}}`
			}
		default:
			return nil, fmt.Errorf("unexpected test endpoint %s", key)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	service := New(private, &repositoryStub{}, client, nil, nil)
	first, err := service.ModelPriceCatalog(ctx, "primary", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Models) != 2 || first.Models[0].Model != "fallback-model" || first.Models[0].Source != "sub2api" || first.Models[0].ModelRatio != "0.615" || first.Models[0].CacheRatio != "0" || first.Models[1].Source != "remote" {
		t.Fatalf("unexpected catalog: %#v", first)
	}
	if first.Stale || len(first.MissingModels) != 1 || first.MissingModels[0] != "missing" {
		t.Fatalf("unexpected state: %#v", first)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	private.Store = db
	service = New(private, &repositoryStub{}, client, nil, nil)
	if _, err := service.ModelPriceCatalog(ctx, "primary", false); err != nil {
		t.Fatal(err)
	}
	if calls["sub2api.example/api/v1/admin/channels/model-pricing"] != 2 || calls["newapi.example/api/option/"] != 1 {
		t.Fatalf("restart did not reuse cache: %v", calls)
	}
	refreshed, err := service.ModelPriceCatalog(ctx, "primary", true)
	if err != nil {
		t.Fatal(err)
	}
	if calls["sub2api.example/api/v1/admin/channels/model-pricing"] != 4 || calls["raw.githubusercontent.com/Wei-Shaw/model-price-repo/main/model_prices_and_context_window.json"] != 2 {
		t.Fatalf("manual refresh did not bypass cache: %v", calls)
	}
	service.client.Transport = pricingTransport(func(*http.Request) (*http.Response, error) { return nil, fmt.Errorf("offline") })
	stale, err := service.ModelPriceCatalog(ctx, "primary", true)
	if err != nil || !stale.Stale || len(stale.Models) != 2 || stale.FetchedAt != refreshed.FetchedAt || stale.Warning == "" {
		t.Fatalf("failed refresh lost last success: %#v, %v", stale, err)
	}
	if _, err := service.ModelPriceCatalog(ctx, "absent", false); err == nil {
		t.Fatal("unknown platform accepted")
	}
}

func TestPricingCatalogTargetChangeDoesNotReuseInstancePrices(t *testing.T) {
	db, err := configstore.Open(filepath.Join(t.TempDir(), "cache.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	private := &pricingTestStore{Store: db, target: configstore.TargetSettings{BaseURL: "https://old.example", AdminKey: "test-key", TimeoutSeconds: 1}}
	client := &http.Client{Transport: pricingTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"success":true,"data":[]}`
		switch {
		case r.URL.Host == "raw.githubusercontent.com":
			body = `{"remote":{"input_cost_per_token":0.000002,"output_cost_per_token":0.000004}}`
		case r.URL.Path == "/api/channel/models_enabled":
			body = `{"success":true,"data":["local-model"]}`
		case r.URL.Host == "old.example":
			body = `{"code":0,"data":{"found":true,"input_price":0.000001,"output_price":0.000004}}`
		case r.URL.Host == "new.example":
			body = `{"code":0,"data":{"found":false}}`
		case r.URL.Host != "newapi.example":
			return nil, fmt.Errorf("unexpected endpoint")
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	service := New(private, &repositoryStub{}, client, nil, nil)
	old, err := service.ModelPriceCatalog(context.Background(), "primary", false)
	if err != nil || len(old.Models) != 2 {
		t.Fatalf("old=%#v err=%v", old, err)
	}
	private.target.BaseURL = "https://new.example"
	next, err := service.ModelPriceCatalog(context.Background(), "primary", false)
	if err != nil || len(next.Models) != 1 || len(next.MissingModels) != 1 || next.MissingModels[0] != "local-model" {
		t.Fatalf("cross-target cache leaked: %#v err=%v", next, err)
	}
}

func TestExpiredRemoteCacheIsRetainedWithoutAdvancingTimestamp(t *testing.T) {
	db, err := configstore.Open(filepath.Join(t.TempDir(), "cache.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	at := time.Now().UTC().Add(-25 * time.Hour)
	key := "remote-v1:" + defaultSub2APIPricingURL
	ctx := context.Background()
	if err := db.SaveModelPricingCache(ctx, key, configstore.ModelPricingCache{Content: `{"remote":{"input_cost_per_token":0.000001,"output_cost_per_token":0.000003}}`, FetchedAt: at}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	client := &http.Client{Transport: pricingTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 502, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})}
	service := New(&pricingTestStore{Store: db}, nil, client, nil, nil)
	prices, source, err := service.loadRemotePricing(ctx, false)
	if err != nil || calls != 1 || len(prices) != 1 || !source.Stale || source.FetchedAt != at.Format(time.RFC3339Nano) {
		t.Fatalf("prices=%#v source=%#v calls=%d err=%v", prices, source, calls, err)
	}
	cached, err := db.ModelPricingCache(ctx, key)
	if err != nil || !cached.FetchedAt.Equal(at) {
		t.Fatalf("failed refresh extended TTL: %#v, %v", cached, err)
	}
}

func TestPricingCacheExpiresAfter24Hours(t *testing.T) {
	now := time.Now().UTC()
	if !pricingCacheFresh(now.Add(-23*time.Hour), now) || pricingCacheFresh(now.Add(-24*time.Hour), now) || pricingCacheFresh(now.Add(time.Hour), now) {
		t.Fatal("invalid expiry boundary")
	}
}
