package newapimanagement_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
)

func TestClaudeContextPricesSurviveCatalogCacheAndRefreshRecovery(t *testing.T) {
	fail := false
	service := westernCatalog(t, &fail)
	fresh, err := service.ModelPriceCatalog(context.Background(), "test", true)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Stale || fresh.Warning != "" {
		t.Fatalf("official context tiers prevented refresh: %s", fresh.Warning)
	}
	haiku := priceByName(t, fresh, "claude-haiku-5-5")
	if haiku.Source != "official" || len(haiku.PriceTiers) != 2 || haiku.BillingExpr == "" {
		t.Fatalf("official context tiers missing from catalog: %+v", haiku)
	}
	if haiku.PriceTiers[0].Condition != "len <= 100000" || haiku.PriceTiers[0].InputPrice != "0.0000001" || haiku.PriceTiers[1].InputPrice != "0.0000005" || haiku.PriceTiers[1].CacheWrite1hPrice != "0.000001" {
		t.Fatalf("context boundaries or cache rates lost: %+v", haiku.PriceTiers)
	}
	if sonnet := priceByName(t, fresh, "claude-sonnet-5-5"); sonnet.Source != "official" || sonnet.CacheReadPrice != "0.0000001" {
		t.Fatalf("new Sonnet official rate missing: %+v", sonnet)
	}

	fail = true
	stale, err := service.ModelPriceCatalog(context.Background(), "test", true)
	if err != nil {
		t.Fatal(err)
	}
	if !stale.Stale || stale.Warning == "" {
		t.Fatal("failed refresh did not retain warning")
	}
	// Exercise the API representation after the catalog is restored from storage.
	raw, err := json.Marshal(stale)
	if err != nil {
		t.Fatal(err)
	}
	var decoded newapimanagement.ModelPriceCatalog
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if cached := priceByName(t, decoded, haiku.Model); !reflect.DeepEqual(cached.PriceTiers, haiku.PriceTiers) || cached.BillingExpr != haiku.BillingExpr {
		t.Fatalf("cached context prices changed after failed refresh: %+v", cached)
	}

	fail = false
	recovered, err := service.ModelPriceCatalog(context.Background(), "test", true)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Stale || recovered.Warning != "" {
		t.Fatalf("successful official refresh retained failure warning: %s", recovered.Warning)
	}
	if got := priceByName(t, recovered, haiku.Model); !reflect.DeepEqual(got.PriceTiers, haiku.PriceTiers) || got.BillingExpr != haiku.BillingExpr {
		t.Fatalf("recovered official context prices changed: %+v", got)
	}
}
