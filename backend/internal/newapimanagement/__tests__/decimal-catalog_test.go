package newapimanagement_test

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestRemoteCatalogOverflowingContextThresholdDoesNotCreateTier(t *testing.T) {
	page := officialPage
	service := setupCatalog(t, &page, transport(func(request *http.Request) (*http.Response, error) {
		body := `{"success":true,"data":[]}`
		if request.URL.Host == "raw.githubusercontent.com" {
			threshold := strconv.Itoa(math.MaxInt/1000 + 1)
			body = `{"remote-model":{"input_cost_per_token":0.000001,"output_cost_per_token":0.000002,"input_cost_per_token_above_` + threshold + `k_tokens":0.000002}}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
	catalog, err := service.ModelPriceCatalog(context.Background(), "test", true)
	if err != nil {
		t.Fatal(err)
	}
	if got := priceByName(t, catalog, "remote-model"); got.LongContextThreshold != 0 {
		t.Fatalf("overflowing context threshold created a billing tier: %d", got.LongContextThreshold)
	}
}

func TestRemoteCatalogInvalidPricesRetainSuccessfulCache(t *testing.T) {
	for _, value := range []string{"1/2", "0x10", "1_000", "1e1001", "-1"} {
		t.Run(value, func(t *testing.T) {
			page := officialPage
			price := "0.000002"
			service := setupCatalog(t, &page, transport(func(request *http.Request) (*http.Response, error) {
				body := `{"success":true,"data":[]}`
				if request.URL.Host == "raw.githubusercontent.com" {
					raw, err := json.Marshal(map[string]any{"remote-model": map[string]string{
						"input_cost_per_token": "0.000001", "output_cost_per_token": price,
						"cache_read_input_token_cost": price,
					}})
					if err != nil {
						t.Fatal(err)
					}
					body = string(raw)
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			}))
			if _, err := service.ModelPriceCatalog(context.Background(), "test", true); err != nil {
				t.Fatal(err)
			}
			price = value
			catalog, err := service.ModelPriceCatalog(context.Background(), "test", true)
			if err != nil {
				t.Fatal(err)
			}
			if !catalog.Stale || catalog.Warning == "" {
				t.Fatal("invalid remote price refresh was accepted")
			}
			if got := priceByName(t, catalog, "remote-model"); got.OutputPrice != "0.000002" || got.CacheReadPrice != "0.000002" {
				t.Fatalf("invalid prices replaced the usable cache: %+v", got)
			}
		})
	}
}

func TestPricingComparisonTreatsEquivalentDecimalsAndDefaultModesAsEqual(t *testing.T) {
	for _, completion := range []string{"2", "3"} {
		t.Run("completion "+completion, func(t *testing.T) {
			page := officialPage
			service := setupCatalog(t, &page, transport(func(request *http.Request) (*http.Response, error) {
				body := `{"success":true,"data":[]}`
				switch request.URL.Path {
				case "/api/option/":
					body = `{"success":true,"data":{"ModelRatio":"{\"same-model\":1.0}","CompletionRatio":"{\"same-model\":2.0}"}}`
				case "/api/pricing":
					body = `{"success":true,"data":[{"model_name":"same-model","quota_type":0,"model_ratio":1,"completion_ratio":` + completion + `}]}`
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			}))
			result, err := service.Refresh(context.Background(), "test")
			if err != nil {
				t.Fatal(err)
			}
			wantDifferences := 0
			if completion == "3" {
				wantDifferences = 1
			}
			if len(result.Differences) != wantDifferences {
				t.Fatalf("differences=%+v, want %d", result.Differences, wantDifferences)
			}
		})
	}
}
