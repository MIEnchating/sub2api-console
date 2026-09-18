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
		if err := validateAnimationSchedule(schedule); err != nil {
			return err
		}
		schedule.PrecheckQuestions, _ = normalizePrecheckQuestions(schedule.Mode, schedule.PrecheckQuestions)
		s.animation.schedules[schedule.AccountID] = schedule
		s.animation.next[schedule.AccountID] = time.Now().Add(time.Duration(schedule.IntervalMinutes) * time.Minute)
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
	if value.IntervalMinutes < 1 || value.IntervalMinutes > 1440 {
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
		schedule.PrecheckQuestions = slices.Clone(schedule.PrecheckQuestions)
		view := AnimationScheduleView{AnimationSchedule: schedule, LastTaskID: s.animation.lastTask[id], LastError: s.animation.lastError[id]}
		if next := s.animation.next[id]; schedule.Enabled && !next.IsZero() {
			view.NextAt = next.UTC().Format(time.RFC3339Nano)
		}
		result = append(result, view)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].AccountID < result[j].AccountID })
	return result
}

func (s *Service) SaveAnimationSchedule(ctx context.Context, value AnimationSchedule, actor string) ([]AnimationScheduleView, error) {
	if s.animation.repository == nil {
		return nil, errors.New("自动检测配置存储尚未就绪")
	}
	if err := validateAnimationSchedule(value); err != nil {
		return nil, err
	}
	value.PrecheckQuestions, _ = normalizePrecheckQuestions(value.Mode, value.PrecheckQuestions)
	if value.Enabled {
		request, _, err := s.prepareAnimation(ctx, AnimationRequest{Targets: []AnimationTarget{{AccountID: value.AccountID, Model: value.Model}}, TimeoutSeconds: value.TimeoutSeconds, Mode: value.Mode, PrecheckQuestions: value.PrecheckQuestions})
		if err != nil {
			return nil, err
		}
		value.Model = request.Targets[0].Model
	}
	// Serialize schedule writes with launches so disabling returns after all prior launches.
	s.animation.launchMu.Lock()
	defer s.animation.launchMu.Unlock()
	s.animation.scheduleMu.Lock()
	previous := s.animation.schedules[value.AccountID]
	if previous.Version != value.Version {
		s.animation.scheduleMu.Unlock()
		return nil, errors.New("自动检测配置已变化，请刷新后重试")
	}
	if !value.Enabled && previous.Version == 0 {
		s.animation.scheduleMu.Unlock()
		return nil, errors.New("该账号尚未配置自动检测")
	}
	value.Version++
	next := make([]AnimationSchedule, 0, len(s.animation.schedules)+1)
	for id, item := range s.animation.schedules {
		if id != value.AccountID {
			next = append(next, item)
		}
	}
	next = append(next, value)
	raw, err := json.Marshal(next)
	if err == nil {
		err = s.animation.repository.SaveAnimationConfiguration(ctx, raw, actor)
	}
	if err != nil {
		s.animation.scheduleMu.Unlock()
		return nil, err
	}
	s.animation.schedules[value.AccountID] = value
	s.animation.next[value.AccountID] = time.Now().Add(time.Duration(value.IntervalMinutes) * time.Minute)
	delete(s.animation.lastError, value.AccountID)
	s.animation.scheduleMu.Unlock()
	return s.AnimationSchedules(), nil
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
			}
		}
	})
}

// RunDueAnimations executes one scheduler tick. Deadlines are reset after completion,
// including failed runs; a restart starts a fresh interval without replaying missed runs.
func (s *Service) RunDueAnimations(ctx context.Context, now time.Time) {
	s.animation.launchMu.Lock()
	defer s.animation.launchMu.Unlock()
	s.animation.scheduleMu.Lock()
	var due []AnimationSchedule
	for id, schedule := range s.animation.schedules {
		if schedule.Enabled && !s.animation.next[id].After(now) {
			due = append(due, schedule)
			s.animation.next[id] = now.Add(time.Duration(schedule.IntervalMinutes) * time.Minute)
		}
	}
	s.animation.scheduleMu.Unlock()
	for _, schedule := range due {
		if ctx.Err() != nil {
			return
		}
		task, err := s.EnqueueAnimation(ctx, AnimationRequest{Targets: []AnimationTarget{{AccountID: schedule.AccountID, Model: schedule.Model}}, TimeoutSeconds: schedule.TimeoutSeconds, Mode: schedule.Mode, PrecheckQuestions: schedule.PrecheckQuestions})
		s.animation.scheduleMu.Lock()
		if err != nil {
			s.animation.lastError[schedule.AccountID] = safeCredentialError(err)
		} else {
			s.animation.lastTask[schedule.AccountID] = task.ID
			delete(s.animation.lastError, schedule.AccountID)
		}
		s.animation.scheduleMu.Unlock()
	}
}
