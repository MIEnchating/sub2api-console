package accountautomation_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountautomation"
)

func TestDailyScheduleSelectsNextBeijingTime(t *testing.T) {
	schedule := accountautomation.Schedule{ScheduleType: "daily", DailyTimes: []string{"18:30", "09:00"}, Timezone: "Asia/Shanghai"}
	for _, tc := range []struct{ name, after, want string }{
		{"before first time", "2026-10-08T00:00:00Z", "2026-10-08T01:00:00Z"},
		{"exact time advances", "2026-10-08T01:00:00Z", "2026-10-08T10:30:00Z"},
		{"after last time rolls over", "2026-10-08T10:30:01Z", "2026-10-09T01:00:00Z"},
		{"Beijing date differs from UTC", "2026-10-08T23:00:00Z", "2026-10-09T01:00:00Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			after, _ := time.Parse(time.RFC3339, tc.after)
			if got := schedule.NextRunAfter(after).UTC().Format(time.RFC3339); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestDailySchedulePersistsTimesAndCalculatesFutureOccurrence(t *testing.T) {
	service, _, _, _, request := automationFixture()
	service.UseScheduleRepository(&memorySchedule{})
	start := time.Now()
	saved, err := service.SaveSchedule(context.Background(), accountautomation.Schedule{
		Name: "每天运行", Request: request, Enabled: true,
		ScheduleType: "daily", DailyTimes: []string{"09:00", "18:30"}, Timezone: "Asia/Shanghai",
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := service.Schedules(context.Background())
	if err != nil || len(loaded) != 1 {
		t.Fatalf("read schedules: %v %v", loaded, err)
	}
	if loaded[0].ScheduleType != "daily" || !reflect.DeepEqual(loaded[0].DailyTimes, saved.DailyTimes) || loaded[0].Timezone != "Asia/Shanghai" {
		t.Fatalf("timing not preserved: %#v", loaded[0])
	}
	if saved.NextRunAt == nil {
		t.Fatal("missing next run")
	}
	next, err := time.Parse(time.RFC3339Nano, *saved.NextRunAt)
	if err != nil || !next.After(start) || next.After(time.Now().Add(24*time.Hour)) {
		t.Fatalf("invalid next run %v %v", next, err)
	}
	zone, _ := time.LoadLocation("Asia/Shanghai")
	clock := next.In(zone).Format("15:04")
	if clock != "09:00" && clock != "18:30" {
		t.Fatalf("unexpected Beijing time: %s", clock)
	}
}

func TestInvalidDailyScheduleIsNotPersisted(t *testing.T) {
	for _, tc := range []struct {
		name  string
		times []string
		zone  string
	}{
		{"empty", nil, "Asia/Shanghai"}, {"duplicate", []string{"09:00", "09:00"}, "Asia/Shanghai"},
		{"invalid clock", []string{"25:00"}, "Asia/Shanghai"}, {"unpadded", []string{"9:00"}, "Asia/Shanghai"},
		{"unsupported timezone", []string{"09:00"}, "UTC"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, _, _, _, request := automationFixture()
			repo := &memorySchedule{}
			service.UseScheduleRepository(repo)
			_, err := service.SaveSchedule(context.Background(), accountautomation.Schedule{Name: "任务", Request: request, ScheduleType: "daily", DailyTimes: tc.times, Timezone: tc.zone})
			if err == nil || repo.raw != "" {
				t.Fatalf("invalid schedule persisted: %v", err)
			}
		})
	}
}

func TestLegacyIntervalScheduleRetainsItsNextRun(t *testing.T) {
	service, _, _, _, request := automationFixture()
	next := "2026-10-09T00:00:00Z"
	raw, err := json.Marshal([]accountautomation.Schedule{{ID: "old", Name: "旧任务", Request: request, Enabled: true, IntervalMinutes: 45, NextRunAt: &next}})
	if err != nil {
		t.Fatal(err)
	}
	service.UseScheduleRepository(&memorySchedule{raw: string(raw)})
	loaded, err := service.Schedules(context.Background())
	if err != nil || len(loaded) != 1 {
		t.Fatalf("load legacy: %v", err)
	}
	if loaded[0].ScheduleType != "interval" || *loaded[0].NextRunAt != next {
		t.Fatalf("changed legacy schedule: %#v", loaded[0])
	}
	after, _ := time.Parse(time.RFC3339, next)
	if got := loaded[0].NextRunAfter(after); !got.Equal(after.Add(45 * time.Minute)) {
		t.Fatalf("changed interval: %v", got)
	}
}
