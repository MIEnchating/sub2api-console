package modelcheck

import (
	"errors"
	"time"
	_ "time/tzdata"
)

func animationScheduleKey(accountID, mode string) string {
	if mode == "" {
		mode = "animation"
	}
	return accountID + ":" + mode
}

func splitAnimationSchedule(value AnimationSchedule) []AnimationSchedule {
	if value.Mode == "" {
		value.Mode = "animation"
	}
	if value.Mode != combinedMode {
		return []AnimationSchedule{value}
	}
	precheck := value
	precheck.Mode = precheckMode
	value.Mode, value.PrecheckQuestions = "animation", nil
	return []AnimationSchedule{precheck, value}
}

func validateAnimationTiming(value AnimationSchedule) error {
	switch value.ScheduleType {
	case "", "interval":
		if value.DailyTime != "" || value.DailyTimes != nil || value.Timezone != "" {
			return errors.New("按间隔检测不能同时指定每日时间")
		}
	case "daily":
		if value.DailyTime != "" && value.DailyTimes != nil {
			return errors.New("每日检测时间不能同时使用单时间与多时间配置")
		}
		times := animationDailyTimes(value)
		if len(times) < 1 || len(times) > 24 {
			return errors.New("每天请选择 1 到 24 个检测时间")
		}
		seen := map[string]bool{}
		for _, item := range times {
			clock, err := time.Parse("15:04", item)
			if err != nil || clock.Format("15:04") != item {
				return errors.New("每日检测时间必须为 HH:mm")
			}
			if seen[item] {
				return errors.New("每日检测时间不能重复")
			}
			seen[item] = true
		}
		if value.Timezone != "Asia/Shanghai" {
			return errors.New("每日检测时区必须为 Asia/Shanghai（北京时间）")
		}
	default:
		return errors.New("检测计划必须为按间隔或每天定时")
	}
	return nil
}

func animationDailyTimes(value AnimationSchedule) []string {
	if value.DailyTimes != nil {
		return value.DailyTimes
	}
	if value.DailyTime != "" {
		return []string{value.DailyTime}
	}
	return nil
}

func nextAnimationTime(value AnimationSchedule, after time.Time) time.Time {
	if value.ScheduleType != "daily" {
		return after.Add(time.Duration(value.IntervalMinutes) * time.Minute)
	}
	zone, _ := time.LoadLocation("Asia/Shanghai")
	local := after.In(zone)
	var next time.Time
	for _, item := range animationDailyTimes(value) {
		clock, _ := time.Parse("15:04", item)
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
