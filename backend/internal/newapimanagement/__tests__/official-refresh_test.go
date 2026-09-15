package newapimanagement_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
	pricing "github.com/MIEnchating/sub2api-console/backend/internal/officialpricing"
)

var migratedProviders = []struct{ id, name, model, url, fixture string }{
	{"kimi", "Kimi", "kimi-k3", pricing.KimiURL, "kimi-chat.md"},
	{"minimax", "MiniMax", "MiniMax-M3", pricing.MiniMaxURL, "minimax.md"},
}

func setupOfficialRefresh(t *testing.T, address, model string, page *string) *newapimanagement.Service {
	t.Helper()
	deepseek := officialPage
	return setupCatalog(t, &deepseek, transport(func(r *http.Request) (*http.Response, error) {
		var body string
		switch {
		case r.URL.Host == "raw.githubusercontent.com":
			body = `{"` + model + `":{"input_cost_per_token":1,"output_cost_per_token":2}}`
		case r.URL.String() == address+".md":
			body = *page
		default:
			return nil, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	}))
}

func TestOfficialRefreshAfterPageRecoveryClearsStaleWarning(t *testing.T) {
	for _, provider := range migratedProviders {
		t.Run(provider.id, func(t *testing.T) {
			page := "maintenance"
			service := setupOfficialRefresh(t, provider.url, provider.model, &page)
			stale, err := service.ModelPriceCatalog(context.Background(), "test", true)
			if err != nil {
				t.Fatal(err)
			}
			if !stale.Stale || !strings.Contains(stale.Warning, provider.name) {
				t.Fatalf("provider failure not reported: %+v", stale)
			}

			raw, err := os.ReadFile("../../officialpricing/__tests__/testdata/" + provider.fixture)
			if err != nil {
				t.Fatal(err)
			}
			page = string(raw)
			fresh, err := service.ModelPriceCatalog(context.Background(), "test", true)
			if err != nil {
				t.Fatal(err)
			}
			if fresh.Stale || fresh.Warning != "" {
				t.Fatalf("successful refresh retained stale warning: %s", fresh.Warning)
			}
			price := priceByName(t, fresh, provider.model)
			if price.Source != "official" || price.SourceURL != provider.url {
				t.Fatalf("recovery did not restore official prices: %+v", price)
			}
		})
	}
}

func TestOfficialRefreshWithInvalidPageRetainsLastSuccessfulPrices(t *testing.T) {
	for _, provider := range migratedProviders {
		t.Run(provider.id, func(t *testing.T) {
			raw, err := os.ReadFile("../../officialpricing/__tests__/testdata/" + provider.fixture)
			if err != nil {
				t.Fatal(err)
			}
			page := string(raw)
			service := setupOfficialRefresh(t, provider.url, provider.model, &page)
			fresh, err := service.ModelPriceCatalog(context.Background(), "test", true)
			if err != nil {
				t.Fatal(err)
			}
			before := priceByName(t, fresh, provider.model)

			page = "maintenance"
			stale, err := service.ModelPriceCatalog(context.Background(), "test", true)
			if err != nil {
				t.Fatal(err)
			}
			if !stale.Stale || !strings.Contains(stale.Warning, provider.name) {
				t.Fatalf("invalid official page not reported: %s", stale.Warning)
			}
			if after := priceByName(t, stale, provider.model); !reflect.DeepEqual(before, after) {
				t.Fatalf("last official price changed on failure: before=%+v after=%+v", before, after)
			}
		})
	}
}
