package pricing_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/pricing"
)

func minimumPolicy(limits any) map[string]any {
	section := map[string]any{
		"enabled":             true,
		"exchange_group_sets": []any{[]any{"6", "7"}},
	}
	if limits != nil {
		section["group_min_cost_multipliers"] = limits
	}
	return map[string]any{"price_management": section}
}

func TestMinimumConfigRejectsInvalidDecimalOrUnmanagedStableID(t *testing.T) {
	for _, test := range []struct {
		name   string
		limits any
	}{
		{name: "negative minimum", limits: map[string]any{"7": "-0.1"}},
		{name: "fraction", limits: map[string]any{"7": "1/2"}},
		{name: "hexadecimal", limits: map[string]any{"7": "0x10"}},
		{name: "unbounded exponent", limits: map[string]any{"7": "1e1001"}},
		{name: "unbounded length", limits: map[string]any{"7": strings.Repeat("1", 129)}},
		{name: "number instead of decimal string", limits: map[string]any{"7": json.Number("0.5")}},
		{name: "unmanaged group", limits: map[string]any{"9": "0.5"}},
		{name: "group name instead of ID", limits: map[string]any{"standard": "0.5"}},
		{name: "noncanonical ID", limits: map[string]any{"07": "0.5"}},
		{name: "array instead of object", limits: []any{"0.5"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := pricing.ConfigFromPolicy(minimumPolicy(test.limits))
			if err == nil {
				t.Fatal("invalid minimum configuration was accepted")
			}
		})
	}
}

func TestMinimumConfigAcceptsUnsetZeroAndExactDecimalValues(t *testing.T) {
	for _, test := range []struct {
		name   string
		limits any
		want   map[string]string
	}{
		{name: "missing minimum", want: map[string]string{}},
		{name: "empty object", limits: map[string]any{}, want: map[string]string{}},
		{name: "blank value", limits: map[string]any{"7": " \t"}, want: map[string]string{}},
		{name: "zero", limits: map[string]any{"7": "0"}, want: map[string]string{"7": "0"}},
		{name: "precise decimal", limits: map[string]any{"7": " 0.40000000000000000000001 "}, want: map[string]string{"7": "0.40000000000000000000001"}},
		{name: "scientific decimal", limits: map[string]any{"7": "4e-1"}, want: map[string]string{"7": "4e-1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			config, err := pricing.ConfigFromPolicy(minimumPolicy(test.limits))
			if err != nil {
				t.Fatal(err)
			}
			if got := minimumConfigValues(t, config); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("minimum configuration = %v, want %v", got, test.want)
			}
		})
	}
}

func TestMinimumConfigPersistsExactValuesAndClearsRemovedLimits(t *testing.T) {
	service := newMinimumService(t, "0.4", []string{"6"}, map[string]string{"6": "1", "7": "0.5"})
	config, err := pricing.ConfigFromPolicy(minimumPolicy(map[string]any{"7": "0.40000000000000000000001"}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdateConfig(context.Background(), config, "test"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := minimumConfigValues(t, snapshot.Config); !reflect.DeepEqual(got, map[string]string{"7": "0.40000000000000000000001"}) {
		t.Fatalf("persisted minimum = %v", got)
	}
	config, err = pricing.ConfigFromPolicy(minimumPolicy(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdateConfig(context.Background(), config, "test"); err != nil {
		t.Fatal(err)
	}
	snapshot, err = service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := minimumConfigValues(t, snapshot.Config); len(got) != 0 {
		t.Fatalf("removed minimum was retained: %v", got)
	}
	if got := snapshot.Decisions[0].DesiredGroupIDs; !reflect.DeepEqual(got, []string{"7"}) {
		t.Fatalf("clearing minimum did not restore price allocation: %v", got)
	}
}

func minimumConfigValues(t *testing.T, config pricing.Config) map[string]string {
	t.Helper()
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	var fields struct {
		Limits map[string]string `json:"group_min_cost_multipliers"`
	}
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	return fields.Limits
}
