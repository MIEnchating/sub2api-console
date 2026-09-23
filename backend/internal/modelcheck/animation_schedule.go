package modelcheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"sync"
	"time"
)

type animationRepository interface {
	LoadAnimationConfiguration(context.Context) ([]byte, error)
	SaveAnimationConfiguration(context.Context, []byte, string) error
}

type AnimationSchedule struct {
	ScheduleType      string   `json:"schedule_type,omitempty"`
	DailyTime         string   `json:"daily_time,omitempty"`
	DailyTimes        []string `json:"daily_times,omitempty"`
	Timezone          string   `json:"timezone,omitempty"`
	PrecheckQuestions []string `json:"precheck_questions,omitempty"`
	Mode              string   `json:"mode,omitempty"`
	AccountID         string   `json:"account_id"`
	Enabled           bool     `json:"enabled"`
	Model             string   `json:"model"`
	IntervalMinutes   int      `json:"interval_minutes"`
	TimeoutSeconds    int      `json:"timeout_seconds"`
	Version           int      `json:"version"`
}

type AnimationScheduleView struct {
	AnimationSchedule
	NextAt     string `json:"next_at,omitempty"`
	LastTaskID string `json:"last_task_id,omitempty"`
	LastError  string `json:"last_error,omitempty"`
}

type animationState struct {
	activeMu   sync.Mutex
	active     map[string]bool
	slots      chan struct{}
	launchMu   sync.Mutex
	scheduleMu sync.Mutex
	schedules  map[string]AnimationSchedule
	next       map[string]time.Time
	lastTask   map[string]string
	lastError  map[string]string
	repository animationRepository
}

func (s *Service) loadAnimationConfiguration(ctx context.Context) error {
	s.animation.active = map[string]bool{}
	s.animation.slots = make(chan struct{}, 3)
	s.animation.schedules = map[string]AnimationSchedule{}
	s.animation.next = map[string]time.Time{}
	s.animation.lastTask = map[string]string{}
	s.animation.lastError = map[string]string{}
	repository, ok := s.accounts.(animationRepository)
	if !ok {
		return nil
	}
	s.animation.repository = repository
	raw, err := repository.LoadAnimationConfiguration(ctx)
	if err != nil {
		return fmt.Errorf("自动动画检测配置读取失败：%w", err)
	}
	if len(raw) == 0 {
		return nil
	}
	var schedules []AnimationSchedule
	if json.Unmarshal(raw, &schedules) != nil {
		return errors.New("自动动画检测配置无效")
	}
	for _, schedule := range schedules {
		schedule.PrecheckQuestions = migrateLegacyPrecheckQuestions(schedule.Mode, schedule.PrecheckQuestions)
		if err := validateAnimationSchedule(schedule); err != nil {
			return err
		}
		schedule.PrecheckQuestions, _ = normalizePrecheckQuestions(schedule.Mode, schedule.PrecheckQuestions)
		for _, item := range splitAnimationSchedule(schedule) {
			key := animationScheduleKey(item.AccountID, item.Mode)
			if _, exists := s.animation.schedules[key]; exists {
				return errors.New("自动检测配置包含重复账号及检测类型")
			}
			s.animation.schedules[key] = item
			s.animation.next[key] = nextAnimationTime(item, time.Now())
		}
	}
	return nil
}

func validateAnimationSchedule(value AnimationSchedule) error {
	if !validAnimationMode(value.Mode) {
		return errors.New("自动检测内容必须为动画检测、前置检测或两者")
	}
	if _, err := normalizePrecheckQuestions(value.Mode, value.PrecheckQuestions); err != nil {
		return err
	}
	if !stablePositiveID(value.AccountID) || value.Version < 0 {
		return errors.New("自动检测账号 ID 或版本无效")
	}
	if err := validateAnimationTiming(value); err != nil {
		return err
	}
	if value.ScheduleType != "daily" && (value.IntervalMinutes < 1 || value.IntervalMinutes > 1440) {
		return errors.New("自动检测间隔必须在 1 到 1440 分钟之间")
	}
	if value.TimeoutSeconds < 5 || value.TimeoutSeconds > 120 {
		return errors.New("自动检测超时必须在 5 到 120 秒之间")
	}
	return nil
}

func (s *Service) AnimationSchedules() []AnimationScheduleView {
	s.animation.scheduleMu.Lock()
	defer s.animation.scheduleMu.Unlock()
	result := make([]AnimationScheduleView, 0, len(s.animation.schedules))
	for id, schedule := range s.animation.schedules {
		schedule.DailyTimes = slices.Clone(schedule.DailyTimes)
		schedule.PrecheckQuestions = slices.Clone(schedule.PrecheckQuestions)
		view := AnimationScheduleView{AnimationSchedule: schedule, LastTaskID: s.animation.lastTask[id], LastError: s.animation.lastError[id]}
		if next := s.animation.next[id]; schedule.Enabled && !next.IsZero() {
			view.NextAt = next.UTC().Format(time.RFC3339Nano)
		}
		result = append(result, view)
	}
	sort.Slice(result, func(i, j int) bool {
		return animationScheduleKey(result[i].AccountID, result[i].Mode) < animationScheduleKey(result[j].AccountID, result[j].Mode)
	})
	return result
}

func (s *Service) SaveAnimationSchedule(ctx context.Context, value AnimationSchedule, actor string) ([]AnimationScheduleView, error) {
	return s.SaveAnimationSchedules(ctx, []AnimationSchedule{value}, actor)
}

func (s *Service) StartAnimationScheduler() error {
	if s.animationRunner == nil {
		return errors.New("自动检测任务执行器尚未就绪")
	}
	return s.animationRunner.Go(func(ctx context.Context) {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				s.RunDueAnimations(ctx, now)
				s.RunDueDetectionTasks(ctx, now)
			}
		}
	})
}

// RunDueAnimations executes one scheduler tick. Interval deadlines reset on completion;
// daily deadlines advance on dispatch. A restart never replays missed runs.
func (s *Service) RunDueAnimations(ctx context.Context, now time.Time) {
	s.animation.launchMu.Lock()
	defer s.animation.launchMu.Unlock()
	s.animation.scheduleMu.Lock()
	var due []AnimationSchedule
	for id, schedule := range s.animation.schedules {
		if schedule.Enabled && !s.animation.next[id].After(now) {
			due = append(due, schedule)
		}
	}
	s.animation.scheduleMu.Unlock()
	sort.Slice(due, func(i, j int) bool {
		if due[i].AccountID == due[j].AccountID {
			return due[i].Mode == precheckMode && due[j].Mode != precheckMode
		}
		return animationScheduleKey(due[i].AccountID, due[i].Mode) < animationScheduleKey(due[j].AccountID, due[j].Mode)
	})
	for _, schedule := range due {
		if ctx.Err() != nil {
			return
		}
		s.animation.activeMu.Lock()
		busy := s.animation.active[schedule.AccountID]
		s.animation.activeMu.Unlock()
		if busy {
			continue
		}
		key := animationScheduleKey(schedule.AccountID, schedule.Mode)
		s.animation.scheduleMu.Lock()
		s.animation.next[key] = nextAnimationTime(schedule, now)
		s.animation.scheduleMu.Unlock()
		task, err := s.EnqueueAnimation(ctx, AnimationRequest{Targets: []AnimationTarget{{AccountID: schedule.AccountID, Model: schedule.Model}}, TimeoutSeconds: schedule.TimeoutSeconds, Mode: schedule.Mode, PrecheckQuestions: schedule.PrecheckQuestions})
		s.animation.scheduleMu.Lock()
		if err != nil {
			s.animation.lastError[key] = safeCredentialError(err)
		} else {
			s.animation.lastTask[key] = task.ID
			delete(s.animation.lastError, key)
		}
		s.animation.scheduleMu.Unlock()
	}
}
