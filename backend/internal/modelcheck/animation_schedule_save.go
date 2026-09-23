package modelcheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"
)

// Validate the entire batch before one durable write. A stale row never partially applies.
func (s *Service) SaveAnimationSchedules(ctx context.Context, values []AnimationSchedule, actor string) ([]AnimationScheduleView, error) {
	if s.animation.repository == nil {
		return nil, errors.New("自动检测配置存储尚未就绪")
	}
	if len(values) == 0 || len(values) > 1000 {
		return nil, errors.New("每次请选择 1 到 1000 项自动检测配置")
	}
	updates := map[string]AnimationSchedule{}
	for _, value := range values {
		if err := validateAnimationSchedule(value); err != nil {
			return nil, err
		}
		value.PrecheckQuestions, _ = normalizePrecheckQuestions(value.Mode, value.PrecheckQuestions)
		value.PrecheckQuestions = slices.Clone(value.PrecheckQuestions)
		value.DailyTimes = slices.Clone(value.DailyTimes)
		slices.Sort(value.DailyTimes)
		if value.Enabled {
			request, _, err := s.prepareAnimation(ctx, AnimationRequest{Targets: []AnimationTarget{{AccountID: value.AccountID, Model: value.Model}}, TimeoutSeconds: value.TimeoutSeconds, Mode: value.Mode, PrecheckQuestions: value.PrecheckQuestions})
			if err != nil {
				return nil, err
			}
			value.Model = request.Targets[0].Model
		}
		for _, item := range splitAnimationSchedule(value) {
			key := animationScheduleKey(item.AccountID, item.Mode)
			if _, exists := updates[key]; exists {
				return nil, errors.New("自动检测配置包含重复账号及检测类型")
			}
			updates[key] = item
		}
	}
	s.animation.launchMu.Lock()
	defer s.animation.launchMu.Unlock()
	s.animation.scheduleMu.Lock()
	for key, value := range updates {
		previous := s.animation.schedules[key]
		if previous.Version != value.Version {
			s.animation.scheduleMu.Unlock()
			return nil, fmt.Errorf("账号 %s 的自动检测配置已变化，请刷新后重试", value.AccountID)
		}
		if !value.Enabled && previous.Version == 0 {
			s.animation.scheduleMu.Unlock()
			return nil, fmt.Errorf("账号 %s 尚未配置此类自动检测", value.AccountID)
		}
		value.Version++
		updates[key] = value
	}
	next := make([]AnimationSchedule, 0, len(s.animation.schedules)+len(updates))
	for key, item := range s.animation.schedules {
		if _, changed := updates[key]; !changed {
			next = append(next, item)
		}
	}
	for _, item := range updates {
		next = append(next, item)
	}
	raw, err := json.Marshal(next)
	if err == nil {
		err = s.animation.repository.SaveAnimationConfiguration(ctx, raw, actor)
	}
	if err != nil {
		s.animation.scheduleMu.Unlock()
		return nil, err
	}
	for key, item := range updates {
		s.animation.schedules[key] = item
		s.animation.next[key] = nextAnimationTime(item, time.Now())
		delete(s.animation.lastError, key)
	}
	s.animation.scheduleMu.Unlock()
	return s.AnimationSchedules(), nil
}
