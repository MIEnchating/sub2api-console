package pricing_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/pricing"
)

func TestMinimumSelectionUsesInclusiveExactCostBoundary(t *testing.T) {
	for _, test := range []struct {
		name    string
		minimum any
		cost    string
		want    string
	}{
		{name: "unset minimum allows lowest price", cost: "0.4", want: "7"},
		{name: "blank minimum allows lowest price", minimum: " ", cost: "0.4", want: "7"},
		{name: "zero minimum allows lowest price", minimum: "0", cost: "0.4", want: "7"},
		{name: "equal minimum allows migration", minimum: "0.4", cost: "0.4", want: "7"},
		{name: "above minimum allows migration", minimum: "0.39", cost: "0.4", want: "7"},
		{name: "below minimum selects next profitable group", minimum: "0.41", cost: "0.4", want: "8"},
		{name: "sub-float precision excludes below minimum", minimum: "0.40000000000000000000001", cost: "0.4", want: "8"},
		{name: "scientific notation preserves exact equality", minimum: "4e-1", cost: "0.400", want: "7"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := newMinimumService(t, test.cost, []string{"6"}, map[string]string{"6": "1", "7": "0.5", "8": "0.75"})
			limits := map[string]any{}
			if test.minimum != nil {
				limits["7"] = test.minimum
			}
			config := minimumConfig(t, limits, [][]string{{"6", "7", "8"}})

			snapshot, err := service.UpdateConfig(context.Background(), config, "test")

			if err != nil {
				t.Fatal(err)
			}
			if got := snapshot.Decisions[0].DesiredGroupIDs; !reflect.DeepEqual(got, []string{test.want}) {
				t.Fatalf("selected groups = %v, want [%s]", got, test.want)
			}
		})
	}
}

func TestMinimumSelectionMovesExistingBelowMinimumAccountWithinItsExchangeSet(t *testing.T) {
	service := newMinimumService(t, "0.4", []string{"7", "9"}, map[string]string{
		"6": "1", "7": "0.5", "8": "0.75", "9": "0.1", "10": "0.48", "11": "0.49",
	})
	config := minimumConfig(t, map[string]any{"7": "0.5"}, [][]string{{"6", "7", "8"}, {"10", "11"}})

	snapshot, err := service.UpdateConfig(context.Background(), config, "test")

	if err != nil {
		t.Fatal(err)
	}
	decision := snapshot.Decisions[0]
	if !decision.Changed || !reflect.DeepEqual(decision.DesiredGroupIDs, []string{"8", "9"}) {
		t.Fatalf("below-minimum account did not move within its existing set: %#v", decision)
	}
}

func TestMinimumSelectionPreservesIndependentMembershipInMultipleExchangeSets(t *testing.T) {
	service := newMinimumService(t, "0.4", []string{"7", "10"}, map[string]string{
		"6": "1", "7": "0.5", "10": "0.8", "11": "0.6",
	})
	config := minimumConfig(t, map[string]any{"7": "0.5"}, [][]string{{"6", "7"}, {"10", "11"}})

	snapshot, err := service.UpdateConfig(context.Background(), config, "test")

	if err != nil {
		t.Fatal(err)
	}
	if got := snapshot.Decisions[0].DesiredGroupIDs; !reflect.DeepEqual(got, []string{"6", "11"}) {
		t.Fatalf("independent exchange sets selected %v", got)
	}
}

func TestMinimumSelectionExcludesRestrictedGroupFromNonLossFallback(t *testing.T) {
	service := newMinimumService(t, "0.9", []string{"7"}, map[string]string{"6": "1", "7": "0.8", "8": "0.95"})
	config := minimumConfig(t, map[string]any{"6": "1"}, [][]string{{"6", "7", "8"}})

	snapshot, err := service.UpdateConfig(context.Background(), config, "test")

	if err != nil {
		t.Fatal(err)
	}
	decision := snapshot.Decisions[0]
	if !reflect.DeepEqual(decision.DesiredGroupIDs, []string{"8"}) || decision.Reason == nil || !strings.Contains(*decision.Reason, "未达到目标盈利比例") {
		t.Fatalf("fallback bypassed minimum: %#v", decision)
	}
}

func TestMinimumSelectionPreservesCurrentGroupWhenNoAllowedGroupCoversCost(t *testing.T) {
	for _, test := range []struct {
		name   string
		limits map[string]any
	}{
		{name: "every group is below its minimum", limits: map[string]any{"6": "1", "7": "1"}},
		{name: "remaining allowed group loses money", limits: map[string]any{"6": "1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := newMinimumService(t, "0.9", []string{"7"}, map[string]string{"6": "1", "7": "0.8"})
			config := minimumConfig(t, test.limits, [][]string{{"6", "7"}})

			snapshot, err := service.UpdateConfig(context.Background(), config, "test")

			if err != nil {
				t.Fatal(err)
			}
			decision := snapshot.Decisions[0]
			if decision.Changed || !reflect.DeepEqual(decision.DesiredGroupIDs, []string{"7"}) || len(decision.EligibleGroups) != 0 {
				t.Fatalf("ineligible current group was moved or reported eligible: %#v", decision)
			}
			if decision.Reason == nil || !strings.Contains(*decision.Reason, "最低迁入倍率") {
				t.Fatalf("minimum rejection was not explained: %#v", decision.Reason)
			}
		})
	}
}

func minimumConfig(t *testing.T, limits map[string]any, sets [][]string) pricing.Config {
	t.Helper()
	config, err := pricing.ConfigFromPolicy(minimumPolicy(limits))
	if err != nil {
		t.Fatal(err)
	}
	config.ExchangeGroupSets = sets
	config.ExchangeGroupSetNames = nil
	return config
}

func newMinimumService(t *testing.T, cost string, current []string, rates map[string]string) *pricing.Service {
	t.Helper()
	ctx := context.Background()
	store, err := business.Open(filepath.Join(t.TempDir(), "minimum-pricing.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	groupIDs := make([]any, 0, len(current))
	for _, id := range current {
		groupIDs = append(groupIDs, json.Number(id))
	}
	groups := make([]map[string]any, 0, len(rates))
	for id, rate := range rates {
		groups = append(groups, map[string]any{"id": json.Number(id), "name": "group-" + id, "platform": "openai", "rate_multiplier": json.Number(rate)})
	}
	_, err = store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("41"), "name": "account", "platform": "openai", "rate_multiplier": json.Number(cost), "group_ids": groupIDs,
	}}, groups, "test")
	if err != nil {
		t.Fatal(err)
	}
	return pricing.New(store, nil, nil)
}
