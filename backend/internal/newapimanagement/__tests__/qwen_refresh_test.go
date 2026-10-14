package newapimanagement_test

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	pricing "github.com/MIEnchating/sub2api-console/backend/internal/officialpricing"
)

func TestQwenRefreshIgnoresUnrelatedTableAndClearsLayoutFailureWarning(t *testing.T) {
	raw, err := os.ReadFile("../../officialpricing/__tests__/testdata/qwen.html")
	if err != nil {
		t.Fatal(err)
	}
	unrelated, err := os.ReadFile("../../officialpricing/__tests__/testdata/qwen-unrelated-table.html")
	if err != nil {
		t.Fatal(err)
	}
	page := `<h4>华北2（北京）</h4>` + string(unrelated)
	service := setupOfficialRefresh(t, pricing.QwenURL, "qwen-turbo", &page)
	stale, err := service.ModelPriceCatalog(context.Background(), "test", true)
	if err != nil {
		t.Fatal(err)
	}
	if !stale.Stale || !strings.Contains(stale.Warning, "Qwen") {
		t.Fatalf("missing Qwen prices did not report failure: %+v", stale)
	}

	page = string(raw) + `<h4>华北2（北京）</h4>` + string(unrelated)
	fresh, err := service.ModelPriceCatalog(context.Background(), "test", true)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Stale || fresh.Warning != "" {
		t.Fatalf("unrelated layout retained stale warning: %s", fresh.Warning)
	}
	price := priceByName(t, fresh, "qwen-turbo")
	if price.Source != "official" || price.SourceURL != pricing.QwenURL || len(price.PriceTiers) != 2 {
		t.Fatalf("Qwen official mode tiers were not restored: %+v", price)
	}
}

func TestQwenRelevantTableLayoutFailureRetainsCompleteOfficialCache(t *testing.T) {
	raw, err := os.ReadFile("../../officialpricing/__tests__/testdata/qwen.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(raw)
	service := setupOfficialRefresh(t, pricing.QwenURL, "qwen-turbo", &page)
	fresh, err := service.ModelPriceCatalog(context.Background(), "test", true)
	if err != nil {
		t.Fatal(err)
	}
	before := priceByName(t, fresh, "qwen-turbo")
	if fresh.Stale || len(before.PriceTiers) != 2 {
		t.Fatalf("expected complete initial Qwen cache: %+v", fresh)
	}

	page += `<h4>华北2（北京）</h4><table><tr><th>模型 ID</th><th>输入单价（每百万Token）</th><th>输出单价（每百万Token）</th></tr><tr><td>qwen-plus</td><td rowspan="3">0.8元</td><td>2元</td></tr></table>`
	stale, err := service.ModelPriceCatalog(context.Background(), "test", true)
	if err != nil {
		t.Fatal(err)
	}
	if !stale.Stale || !strings.Contains(stale.Warning, "Qwen") || !strings.Contains(stale.Warning, "官方价格表超出边界") {
		t.Fatalf("invalid Qwen layout was not reported: %+v", stale)
	}
	if after := priceByName(t, stale, "qwen-turbo"); !reflect.DeepEqual(before, after) {
		t.Fatalf("invalid Qwen layout replaced the last complete price: before=%+v after=%+v", before, after)
	}
}
