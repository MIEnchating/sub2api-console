package pricing_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/pricing"
)

func TestCompositeGroupsAllowPricingConfigurationAndSelectByAccountCost(t *testing.T) {
	for _, platform := range []string{"openai", "anthropic"} {
		t.Run(platform, func(t *testing.T) {
			service := compositeService(t, platform, "0.2", []any{json.Number("32")})
			config := minimumConfig(t, nil, [][]string{{"32", "35", "36"}})
			config.ProfitMargin = 0.2
			snapshot, err := service.UpdateConfig(context.Background(), config, "test")
			if err != nil {
				t.Fatal(err)
			}
			for _, group := range snapshot.Groups {
				if !group.Available || group.Reason != nil {
					t.Fatalf("valid composite group unavailable: %#v", group)
				}
			}
			decision := snapshot.Decisions[0]
			if !decision.Changed || !reflect.DeepEqual(decision.DesiredGroupIDs, []string{"35"}) {
				t.Fatalf("composite allocation did not select lowest profitable group: %#v", decision)
			}
		})
	}
}

func TestCompositeSelectionPreservesExchangeScopeAndMinimumCost(t *testing.T) {
	for _, test := range []struct {
		name     string
		current  []any
		minimums map[string]string
		want     []string
	}{
		{name: "outside exchange set stays outside", current: []any{json.Number("36")}, want: []string{"36"}},
		{name: "below minimum stays in current group", current: []any{json.Number("32")}, minimums: map[string]string{"35": "0.2"}, want: []string{"32"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := compositeService(t, "openai", "0.2", test.current)
			config := minimumConfig(t, nil, [][]string{{"32", "35"}})
			config.GroupMinCostMultipliers = test.minimums
			snapshot, err := service.UpdateConfig(context.Background(), config, "test")
			if err != nil {
				t.Fatal(err)
			}
			if got := snapshot.Decisions[0].DesiredGroupIDs; !reflect.DeepEqual(got, test.want) {
				t.Fatalf("selected groups = %v, want %v", got, test.want)
			}
		})
	}
}

func TestCompositeGroupWithInvalidPriceRemainsUnavailable(t *testing.T) {
	service := compositeService(t, "openai", "0", []any{json.Number("32")})
	snapshot, err := service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range snapshot.Groups {
		if group.ID == "35" && (group.Available || group.Reason == nil || *group.Reason != "分组售价倍率无效") {
			t.Fatalf("invalid price not rejected: %#v", group)
		}
	}
}

func compositeService(t *testing.T, platform, lowPrice string, current []any) *pricing.Service {
	t.Helper()
	ctx := context.Background()
	store, err := business.Open(filepath.Join(t.TempDir(), "composite-pricing.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("41"), "name": "国产账号", "platform": platform,
		"rate_multiplier": json.Number("0.15"), "group_ids": current,
	}}, []map[string]any{
		{"id": json.Number("32"), "name": "国产-平价", "platform": "composite", "rate_multiplier": json.Number("0.5")},
		{"id": json.Number("35"), "name": "国产-特价", "platform": "composite", "rate_multiplier": json.Number(lowPrice)},
		{"id": json.Number("36"), "name": "国产-旗舰", "platform": "composite", "rate_multiplier": json.Number("0.7")},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	return pricing.New(store, nil, nil)
}
