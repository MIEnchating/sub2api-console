package newapimanagement_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/officialpricing"
)

func TestOfficialTablePreservesBothRatesAndPublishedSchedule(t *testing.T) {
	prices, err := officialpricing.ParseDeepSeek([]byte(officialPage))
	if err != nil {
		t.Fatal(err)
	}
	if len(prices) != 4 {
		t.Fatalf("expected two models and two published aliases, got %d", len(prices))
	}
	flash := prices[0]
	want := officialpricing.TimePricing{
		Timezone: "Asia/Shanghai", WeekdaysOnly: true,
		Periods: []officialpricing.Period{{StartTime: "09:00", EndTime: "12:00"}, {StartTime: "14:00", EndTime: "18:00"}},
		Peak:    officialpricing.Rates{InputPrice: "0.000002", OutputPrice: "0.000008", CacheReadPrice: "0.00000004"},
	}
	if !reflect.DeepEqual(flash.TimePricing, want) {
		t.Fatalf("schedule or peak values changed: %+v", flash.TimePricing)
	}
	if flash.CacheReadPrice != "0.00000002" {
		t.Fatalf("small cache price lost precision: %s", flash.CacheReadPrice)
	}
}

func TestOfficialTableRejectsIncompleteOrChangedPricing(t *testing.T) {
	for _, tc := range []struct{ name, old, replacement string }{
		{"missing price", "1元", ""},
		{"invalid price", "1元", "NaN元"},
		{"negative price", "1元", "-1元"},
		{"changed unit", "百万tokens输入", "千tokens输入"},
		{"unknown schedule", "周一至周五", "周一至周六"},
		{"invalid time", "9:00", "25:00"},
		{"overlapping periods", "14:00", "11:00"},
		{"missing peak tier", ">高峰时段<", ">其他时段<"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page := strings.ReplaceAll(officialPage, tc.old, tc.replacement)
			if page == officialPage {
				t.Fatal("fixture replacement did not apply")
			}
			if _, err := officialpricing.ParseDeepSeek([]byte(page)); err == nil {
				t.Fatal("incomplete pricing accepted")
			}
		})
	}
}

func TestFirstOfficialFailureDoesNotUseThirdPartyVendorPrices(t *testing.T) {
	page := `<article>maintenance</article>`
	service := setupCatalog(t, &page)
	catalog, err := service.ModelPriceCatalog(t.Context(), "test", true)
	if err != nil {
		t.Fatal(err)
	}
	if !catalog.Stale || !strings.Contains(catalog.Warning, "官方") {
		t.Fatal("official failure hidden")
	}
	for _, price := range catalog.Models {
		if strings.HasPrefix(price.Model, "deepseek-") {
			t.Fatal("official failure must not silently substitute third-party prices")
		}
	}
}
