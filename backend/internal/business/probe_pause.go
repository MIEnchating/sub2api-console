package business

import (
	"errors"
	"fmt"
	"time"
	_ "time/tzdata"
)

type ProbePauseWindow struct {
	Enabled  bool   `json:"enabled"`
	Start    string `json:"start"`
	End      string `json:"end"`
	Timezone string `json:"timezone"`
}

func parseProbePauseWindow(raw any) (ProbePauseWindow, *time.Location, error) {
	values, ok := raw.(map[string]any)
	if !ok {
		return ProbePauseWindow{}, nil, errors.New("探活暂停时段必须是对象")
	}
	for key := range values {
		if key != "enabled" && key != "start" && key != "end" && key != "timezone" {
			return ProbePauseWindow{}, nil, fmt.Errorf("探活暂停时段包含未知字段：%s", key)
		}
	}
	var window ProbePauseWindow
	window.Enabled, ok = values["enabled"].(bool)
	if !ok {
		return window, nil, errors.New("探活暂停时段开关必须是布尔值")
	}
	window.Start, _ = values["start"].(string)
	window.End, _ = values["end"].(string)
	for _, clock := range []string{window.Start, window.End} {
		parsed, err := time.Parse("15:04", clock)
		if err != nil || parsed.Format("15:04") != clock {
			return window, nil, errors.New("探活暂停时间必须使用 HH:mm 格式，例如 23:00")
		}
	}
	if window.Start == window.End {
		return window, nil, errors.New("探活暂停开始和结束时间不能相同")
	}
	window.Timezone, _ = values["timezone"].(string)
	if window.Timezone == "" || window.Timezone == "Local" || len(window.Timezone) > 128 {
		return window, nil, errors.New("探活暂停时段必须指定有效时区，例如 Asia/Shanghai")
	}
	location, err := time.LoadLocation(window.Timezone)
	if err != nil {
		return window, nil, errors.New("探活暂停时段时区无效，请使用 Asia/Shanghai 或其他 IANA 时区")
	}
	return window, location, nil
}

// ProbePauseReason uses wall-clock time in the configured timezone: start is
// inclusive and end exclusive, including windows spanning midnight.
func ProbePauseReason(policy map[string]any, now time.Time) (string, error) {
	section, exists := policy["probe"]
	if !exists {
		return "", nil
	}
	values, ok := section.(map[string]any)
	if !ok {
		return "", errors.New("主动探测策略必须是对象")
	}
	raw, exists := values["pause_window"]
	if !exists {
		return "", nil
	}
	window, location, err := parseProbePauseWindow(raw)
	if err != nil || !window.Enabled {
		return "", err
	}
	clock := now.In(location).Format("15:04")
	paused := clock >= window.Start && clock < window.End
	if window.Start > window.End {
		paused = clock >= window.Start || clock < window.End
	}
	if paused {
		return fmt.Sprintf("自动探活处于每日暂停时段 %s 至 %s（%s）", window.Start, window.End, window.Timezone), nil
	}
	return "", nil
}
