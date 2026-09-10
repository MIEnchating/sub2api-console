package uptimekuma

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

var maintenanceStrategies = map[string]bool{"manual": true, "single": true, "cron": true, "recurring-interval": true, "recurring-weekday": true, "recurring-day-of-month": true}

func maintenanceConfig(raw map[string]json.RawMessage) *MaintenanceConfig {
	m := &MaintenanceConfig{Title: rawString(raw, "title"), Description: rawString(raw, "description"), Strategy: rawString(raw, "strategy"), Active: rawBool(raw, "active"), Timezone: rawString(raw, "timezoneOption"), Cron: rawString(raw, "cron"), DurationMinutes: rawInt(raw, "durationMinutes", 60), IntervalDays: rawInt(raw, "intervalDay", 1), Weekdays: []int{}, DaysOfMonth: []int{}, MonitorIDs: []int64{}, StatusPageIDs: []int64{}, Status: rawString(raw, "status")}
	if m.Timezone == "" {
		m.Timezone = "SAME_AS_SERVER"
	}
	var dates []*string
	_ = json.Unmarshal(raw["dateRange"], &dates)
	if len(dates) > 0 && dates[0] != nil {
		m.Start = *dates[0]
	}
	if len(dates) > 1 && dates[1] != nil {
		m.End = *dates[1]
	}
	var times []struct {
		Hours   int `json:"hours"`
		Minutes int `json:"minutes"`
	}
	_ = json.Unmarshal(raw["timeRange"], &times)
	if len(times) > 1 {
		m.StartTime = fmt.Sprintf("%02d:%02d", times[0].Hours, times[0].Minutes)
		m.EndTime = fmt.Sprintf("%02d:%02d", times[1].Hours, times[1].Minutes)
	}
	_ = json.Unmarshal(raw["weekdays"], &m.Weekdays)
	var days []json.RawMessage
	_ = json.Unmarshal(raw["daysOfMonth"], &days)
	for _, day := range days {
		if string(day) == `"lastDay1"` {
			m.LastDay = true
			continue
		}
		var n int
		if json.Unmarshal(day, &n) == nil {
			m.DaysOfMonth = append(m.DaysOfMonth, n)
		}
	}
	if m.DurationMinutes < 1 {
		m.DurationMinutes = 60
	}
	if m.IntervalDays < 1 {
		m.IntervalDays = 1
	}
	return m
}
func maintenancePayload(m *MaintenanceConfig) (map[string]json.RawMessage, error) {
	bad := func(msg string) (map[string]json.RawMessage, error) {
		return nil, failure("kuma_invalid_maintenance", msg, 422)
	}
	if m == nil || strings.TrimSpace(m.Title) == "" || len(m.Title) > 150 || len(m.Description) > 10000 || !maintenanceStrategies[m.Strategy] {
		return bad("维护标题或策略无效")
	}
	if m.Timezone != "SAME_AS_SERVER" {
		if _, e := time.LoadLocation(m.Timezone); e != nil {
			return bad("时区无效，请使用 Asia/Shanghai 等 IANA 时区")
		}
	}
	dates := []any{nil, nil}
	var parsed [2]time.Time
	for i, value := range []string{m.Start, m.End} {
		if value == "" {
			continue
		}
		v := strings.ReplaceAll(value, "T", " ")
		t, e := time.Parse("2006-01-02 15:04:05", v)
		if e != nil {
			t, e = time.Parse("2006-01-02 15:04", v)
		}
		if e != nil {
			return bad("请填写有效的维护开始和结束时间")
		}
		parsed[i] = t
		dates[i] = t.Format("2006-01-02 15:04:05")
	}
	if m.Strategy == "single" && (parsed[0].IsZero() || parsed[1].IsZero()) {
		return bad("单次维护必须填写开始和结束时间")
	}
	if !parsed[0].IsZero() && !parsed[1].IsZero() && !parsed[1].After(parsed[0]) {
		return bad("维护结束时间必须晚于开始时间")
	}
	times := []map[string]int{{"hours": 0, "minutes": 0}, {"hours": 0, "minutes": 0}}
	if strings.HasPrefix(m.Strategy, "recurring-") {
		for i, value := range []string{m.StartTime, m.EndTime} {
			t, e := time.Parse("15:04", value)
			if e != nil {
				return bad("请填写有效的每日开始和结束时间")
			}
			times[i] = map[string]int{"hours": t.Hour(), "minutes": t.Minute()}
		}
	}
	if m.Strategy == "recurring-interval" && (m.IntervalDays < 1 || m.IntervalDays > 365) {
		return bad("维护间隔必须为 1 到 365 天")
	}
	if m.Strategy == "recurring-weekday" && len(m.Weekdays) == 0 {
		return bad("请至少选择一个星期")
	}
	for _, v := range m.Weekdays {
		if v < 0 || v > 6 {
			return bad("星期参数无效")
		}
	}
	if m.Strategy == "recurring-day-of-month" && len(m.DaysOfMonth) == 0 && !m.LastDay {
		return bad("请至少填写一个月日期")
	}
	for _, v := range m.DaysOfMonth {
		if v < 1 || v > 31 {
			return bad("月日期必须为 1 到 31")
		}
	}
	if m.Strategy == "cron" {
		fields := strings.Fields(m.Cron)
		if len(fields) < 5 || len(fields) > 6 || len(m.Cron) > 256 || m.DurationMinutes < 1 || m.DurationMinutes > 525600 {
			return bad("请检查 Cron 表达式与持续分钟数")
		}
	}
	raw := map[string]any{"title": strings.TrimSpace(m.Title), "description": m.Description, "strategy": m.Strategy, "active": m.Active, "timezoneOption": m.Timezone, "dateRange": dates, "timeRange": times, "weekdays": m.Weekdays, "daysOfMonth": m.DaysOfMonth, "cron": m.Cron, "durationMinutes": m.DurationMinutes, "intervalDay": m.IntervalDays}
	days := make([]any, 0, len(m.DaysOfMonth)+1)
	for _, day := range m.DaysOfMonth {
		days = append(days, day)
	}
	if m.LastDay {
		days = append(days, "lastDay1")
	}
	raw["daysOfMonth"] = days
	data, _ := json.Marshal(raw)
	var result map[string]json.RawMessage
	_ = json.Unmarshal(data, &result)
	return result, nil
}
func idReferences(ids []int64) []IDReference {
	r := make([]IDReference, 0, len(ids))
	for _, id := range ids {
		r = append(r, IDReference{ID: id})
	}
	return r
}
