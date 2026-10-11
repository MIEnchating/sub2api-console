package accountautomation_test

import (
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountautomation"
)

func TestScheduleNameCountsUnicodeCharacters(t *testing.T) {
	service, _, _, _, request := automationFixture()
	service.UseScheduleRepository(&memorySchedule{})
	for _, tc := range []struct {
		name string
		ok   bool
	}{
		{strings.Repeat("中", 128), true},
		{strings.Repeat("中", 129), false},
	} {
		_, err := service.SaveSchedule(t.Context(), accountautomation.Schedule{Name: tc.name, Request: request, IntervalMinutes: 60})
		if (err == nil) != tc.ok {
			t.Fatalf("name length=%d accepted=%t error=%v", len([]rune(tc.name)), tc.ok, err)
		}
	}
}

func TestScheduleEditPreservesRunHistoryAndUnchangedTiming(t *testing.T) {
	for _, timing := range []string{"interval", "daily"} {
		t.Run(timing, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				service, _, _, _, request := automationFixture()
				service.UseScheduleRepository(&memorySchedule{})
				last := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
				next := time.Now().Add(15 * time.Minute).UTC().Format(time.RFC3339Nano)
				original := accountautomation.Schedule{Name: "原计划", Request: request, Enabled: true, ScheduleType: timing, IntervalMinutes: 60, LastRunAt: &last, NextRunAt: &next}
				if timing == "daily" {
					original.DailyTimes, original.Timezone = []string{"09:00", "18:30"}, "Asia/Shanghai"
				}
				saved, err := service.SaveSchedule(t.Context(), original)
				if err != nil {
					t.Fatal(err)
				}
				edited := saved
				edited.Name, edited.LastRunAt, edited.NextRunAt = "已重命名", nil, nil // API exposes timing as output only.
				if timing == "daily" {
					edited.DailyTimes = []string{"18:30", "09:00"}
				}
				got, err := service.SaveSchedule(t.Context(), edited)
				if err != nil {
					t.Fatal(err)
				}
				if got.LastRunAt == nil || *got.LastRunAt != last || got.NextRunAt == nil || *got.NextRunAt != next {
					t.Fatalf("editing lost history or postponed the next run: last=%v next=%v", got.LastRunAt, got.NextRunAt)
				}
				loaded, err := service.Schedules(t.Context())
				if err != nil || len(loaded) != 1 || !reflect.DeepEqual(loaded[0], got) {
					t.Fatalf("saved edit differs from response: %v err=%v", loaded, err)
				}
			})
		})
	}
}

func TestScheduleTimingChangesReplaceStaleNextRunAndPreserveCurrentHistory(t *testing.T) {
	for _, change := range []string{"interval", "daily", "disable", "enable"} {
		t.Run(change, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				service, _, _, _, request := automationFixture()
				service.UseScheduleRepository(&memorySchedule{})
				last := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
				staleNext := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339Nano)
				saved, err := service.SaveSchedule(t.Context(), accountautomation.Schedule{Name: "时间修改", Request: request, Enabled: change != "enable", IntervalMinutes: 60, LastRunAt: &last, NextRunAt: &staleNext})
				if err != nil {
					t.Fatal(err)
				}
				switch change {
				case "interval":
					saved.IntervalMinutes = 120
				case "daily":
					saved.ScheduleType, saved.DailyTimes, saved.Timezone = "daily", []string{"09:00"}, "Asia/Shanghai"
				case "disable":
					saved.Enabled = false
				case "enable":
					saved.Enabled = true
				}
				staleLast := "2000-01-01T00:00:00Z"
				saved.LastRunAt = &staleLast
				got, err := service.SaveSchedule(t.Context(), saved)
				if err != nil {
					t.Fatal(err)
				}
				if got.LastRunAt == nil || *got.LastRunAt != last {
					t.Fatal("stale edit replaced the latest run history")
				}
				if !saved.Enabled {
					if got.NextRunAt != nil {
						t.Fatal("disabled schedule retains a next run")
					}
				} else {
					want := saved.NextRunAfter(time.Now().UTC()).UTC().Format(time.RFC3339Nano)
					if got.NextRunAt == nil || *got.NextRunAt != want {
						t.Fatalf("changed timing retained stale next run: got=%v want=%s", got.NextRunAt, want)
					}
				}
			})
		})
	}
}
