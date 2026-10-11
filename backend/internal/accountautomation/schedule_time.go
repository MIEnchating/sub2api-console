package accountautomation

import (
	"errors"
	"slices"
	"time"
	_ "time/tzdata"
)

// Run history belongs to the scheduler. Editing unrelated fields must not
// erase it or postpone an already scheduled run, even from an older form.
func preserveScheduleRunTimes(schedule *Schedule, current Schedule, now time.Time) {
	schedule.LastRunAt = current.LastRunAt
	schedule.NextRunAt = nil
	if !schedule.Enabled {
		return
	}
	if current.Enabled && current.NextRunAt != nil && sameScheduleTiming(*schedule, current) {
		schedule.NextRunAt = current.NextRunAt
		return
	}
	next := schedule.NextRunAfter(now).UTC().Format(time.RFC3339Nano)
	schedule.NextRunAt = &next
}

func sameScheduleTiming(a, b Schedule) bool {
	if a.ScheduleType != b.ScheduleType {
		return false
	}
	if a.ScheduleType != "daily" {
		return a.IntervalMinutes == b.IntervalMinutes
	}
	if a.Timezone != b.Timezone || len(a.DailyTimes) != len(b.DailyTimes) {
		return false
	}
	for _, clock := range a.DailyTimes {
		if !slices.Contains(b.DailyTimes, clock) {
			return false
		}
	}
	return true
}

func validateScheduleTiming(schedule Schedule) error {
	switch schedule.ScheduleType {
	case "", "interval":
		if schedule.IntervalMinutes < 1 || schedule.IntervalMinutes > 10080 {
			return errors.New("运行间隔必须在 1 分钟到 7 天之间")
		}
		if len(schedule.DailyTimes) > 0 || schedule.Timezone != "" {
			return errors.New("按间隔运行不能同时指定每天时间")
		}
	case "daily":
		if len(schedule.DailyTimes) < 1 || len(schedule.DailyTimes) > 24 {
			return errors.New("每天请选择 1 到 24 个运行时间")
		}
		if schedule.Timezone != "Asia/Shanghai" {
			return errors.New("每天定时必须使用 Asia/Shanghai（北京时间）")
		}
		seen := map[string]bool{}
		for _, value := range schedule.DailyTimes {
			clock, err := time.Parse("15:04", value)
			if err != nil || clock.Format("15:04") != value {
				return errors.New("运行时间必须为 HH:mm")
			}
			if seen[value] {
				return errors.New("运行时间不能重复")
			}
			seen[value] = true
		}
	default:
		return errors.New("定时方式必须为按间隔或每天指定时间")
	}
	return nil
}

// NextRunAfter returns the next occurrence of a validated schedule, strictly
// after the supplied instant. Daily schedules always use Beijing time.
func (schedule Schedule) NextRunAfter(after time.Time) time.Time {
	if schedule.ScheduleType != "daily" {
		return after.Add(time.Duration(schedule.IntervalMinutes) * time.Minute)
	}
	zone, _ := time.LoadLocation("Asia/Shanghai") // Embedded tzdata includes this fixed supported zone.
	local := after.In(zone)
	var next time.Time
	for _, value := range schedule.DailyTimes {
		clock, _ := time.Parse("15:04", value)
		candidate := time.Date(local.Year(), local.Month(), local.Day(), clock.Hour(), clock.Minute(), 0, 0, zone)
		if !candidate.After(after) {
			candidate = candidate.AddDate(0, 0, 1)
		}
		if next.IsZero() || candidate.Before(next) {
			next = candidate
		}
	}
	return next
}
