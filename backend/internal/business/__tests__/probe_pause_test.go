package business_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestProbePauseWindowUsesConfiguredTimezoneAndExclusiveEnd(t *testing.T) {
	for _, test := range []struct {
		name, start, end, at string
		enabled, paused      bool
	}{
		{"before midnight start", "23:00", "08:00", "2026-09-16T14:59:59Z", true, false},
		{"midnight start included", "23:00", "08:00", "2026-09-16T15:00:00Z", true, true},
		{"after midnight paused", "23:00", "08:00", "2026-09-16T23:59:59Z", true, true},
		{"end resumes", "23:00", "08:00", "2026-09-17T00:00:00Z", true, false},
		{"daytime start included", "09:00", "17:00", "2026-09-16T01:00:00Z", true, true},
		{"daytime end resumes", "09:00", "17:00", "2026-09-16T09:00:00Z", true, false},
		{"disabled allows probes", "23:00", "08:00", "2026-09-16T15:00:00Z", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			at, err := time.Parse(time.RFC3339, test.at)
			if err != nil {
				t.Fatal(err)
			}
			reason, err := business.ProbePauseReason(map[string]any{"probe": map[string]any{
				"pause_window": map[string]any{"enabled": test.enabled, "start": test.start, "end": test.end, "timezone": "Asia/Shanghai"},
			}}, at)
			if err != nil || (reason != "") != test.paused {
				t.Fatalf("reason=%q err=%v", reason, err)
			}
		})
	}
	reason, err := business.ProbePauseReason(map[string]any{}, time.Now())
	if err != nil || reason != "" {
		t.Fatalf("missing window should allow probes: %q %v", reason, err)
	}
}

func TestProbePausePolicyPersistsAndRejectsInvalidWindowWithoutSaving(t *testing.T) {
	store, _ := concurrencyStore(t)
	window := map[string]any{"enabled": true, "start": "23:00", "end": "08:00", "timezone": "Asia/Shanghai"}
	saved, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"probe": map[string]any{"pause_window": window}}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	section := saved.AdvancedPolicy["probe"].(map[string]any)
	if !reflect.DeepEqual(section["pause_window"], window) {
		t.Fatalf("window not persisted: %#v", section)
	}
	for _, test := range []struct {
		field string
		value any
	}{
		{"enabled", "true"}, {"start", "24:00"}, {"start", "1:00"}, {"end", "23:00"},
		{"end", "08:60"}, {"timezone", "Local"}, {"timezone", ""}, {"timezone", "Invalid/Zone"}, {"unknown", true},
	} {
		t.Run(test.field+" invalid", func(t *testing.T) {
			invalid := map[string]any{}
			for key, value := range window {
				invalid[key] = value
			}
			invalid[test.field] = test.value
			_, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"probe": map[string]any{"pause_window": invalid}}}, "test")
			if err == nil {
				t.Fatalf("accepted invalid %s=%v", test.field, test.value)
			}
			after, err := store.PolicySnapshot(t.Context())
			if err != nil || after.Revision != saved.Revision {
				t.Fatalf("invalid write changed policy: %v", err)
			}
		})
	}
}
