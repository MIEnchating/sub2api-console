package accountautomation

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
)

type scheduleSettings struct {
	schedules []Schedule
	groups    []TaskGroup
}

// All configuration mutations share the repository's write transaction,
// including membership validation and removal across both settings keys.
func (s *Service) updateScheduleSettings(ctx context.Context, keys []string, update func(*scheduleSettings) error) error {
	if s.scheduleRepo == nil {
		return errors.New("定时任务存储尚未就绪")
	}
	return s.scheduleRepo.UpdateJSONSettings(ctx, keys, func(values map[string]string) (map[string]string, error) {
		var settings scheduleSettings
		var err error
		for _, key := range keys {
			switch key {
			case scheduleSettingKey:
				settings.schedules, err = decodeSchedules(values[key])
			case taskGroupSettingKey:
				settings.groups, err = decodeTaskGroups(values[key])
			}
			if err != nil {
				return nil, err
			}
		}
		if err := update(&settings); err != nil {
			return nil, err
		}
		changes := make(map[string]string, len(keys))
		for _, key := range keys {
			var value any = settings.schedules
			if key == taskGroupSettingKey {
				value = settings.groups
			}
			raw, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			changes[key] = string(raw)
		}
		return changes, nil
	})
}

func (s *Service) recordScheduleStart(ctx context.Context, original, started Schedule) error {
	return s.updateScheduleSettings(ctx, []string{scheduleSettingKey}, func(settings *scheduleSettings) error {
		for index, current := range settings.schedules {
			if current.ID != original.ID {
				continue
			}
			if !reflect.DeepEqual(current, original) {
				return errors.New("运营任务配置已变化，请刷新后重新运行")
			}
			settings.schedules[index] = started
			return nil
		}
		return errors.New("运营任务已删除，未启动正式执行")
	})
}
