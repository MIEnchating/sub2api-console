package accountautomation

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskcontext"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type scheduledRun struct {
	ctx     context.Context
	cancel  context.CancelCauseFunc
	done    chan struct{}
	outcome scheduleOutcome
}

func (s *Service) reserveSchedule(ctx context.Context, id string, fromGroup bool) (*scheduledRun, error) {
	s.scheduleMu.Lock()
	defer s.scheduleMu.Unlock()
	if s.schedulesStopped {
		return nil, errors.New("运营任务调度器已停止")
	}
	if s.runningTaskGroup && !fromGroup {
		return nil, errors.New("账号模型任务组正在执行")
	}
	if s.runningSchedules[id] != nil {
		return nil, errors.New("运营任务正在执行")
	}
	if !fromGroup {
		ctx = context.WithoutCancel(ctx)
	}
	ctx, cancel := context.WithCancelCause(ctx)
	run := &scheduledRun{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	if s.runningSchedules == nil {
		s.runningSchedules = map[string]*scheduledRun{}
	}
	s.runningSchedules[id] = run
	return run, nil
}

func (s *Service) releaseSchedule(id string, run *scheduledRun) {
	run.cancel(context.Canceled)
	s.scheduleMu.Lock()
	delete(s.runningSchedules, id)
	close(run.done)
	s.scheduleMu.Unlock()
}

func (s *Service) StartSchedules(ctx context.Context) {
	s.scheduleMu.Lock()
	if s.scheduleCancel != nil || s.scheduleRepo == nil || len(s.runningSchedules) > 0 && s.schedulesStopped {
		s.scheduleMu.Unlock()
		return
	}
	loopCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	s.scheduleCancel, s.scheduleDone, s.schedulesStopped = cancel, done, false
	s.scheduleMu.Unlock()
	go func() {
		defer close(done)
		defer s.cancelScheduledRuns()
		s.scheduleLoop(loopCtx)
	}()
}

func (s *Service) cancelScheduledRuns() []<-chan struct{} {
	s.scheduleMu.Lock()
	defer s.scheduleMu.Unlock()
	s.schedulesStopped = true
	if s.scheduleCancel != nil {
		s.scheduleCancel()
	}
	done := make([]<-chan struct{}, 0, len(s.runningSchedules)+1)
	if s.scheduleDone != nil {
		done = append(done, s.scheduleDone)
	}
	for _, run := range s.runningSchedules {
		run.cancel(taskcontext.ErrInterrupted)
		done = append(done, run.done)
	}
	return done
}

func (s *Service) StopSchedules() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.StopSchedulesContext(ctx); err != nil {
		slog.Error("运营任务调度器停止失败", "error", err)
	}
}

func (s *Service) StopSchedulesContext(ctx context.Context) error {
	for _, done := range s.cancelScheduledRuns() {
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s.scheduleMu.Lock()
	s.scheduleCancel, s.scheduleDone = nil, nil
	s.scheduleMu.Unlock()
	return nil
}

func (s *Service) scheduleLoop(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			schedules, err := s.Schedules(ctx)
			if err != nil {
				continue
			}
			for _, schedule := range schedules {
				if ctx.Err() != nil {
					return
				}
				if !schedule.Enabled || schedule.NextRunAt == nil {
					continue
				}
				due, err := time.Parse(time.RFC3339Nano, *schedule.NextRunAt)
				if err == nil && !due.After(now.UTC()) {
					_, _ = s.RunSchedule(ctx, schedule.ID)
				}
			}
		}
	}
}

// A deleted task is terminal for its follower. Other read errors are retried
// three times; each read and every interval is bounded by the owner's lifetime.
func (s *Service) waitScheduledTask(ctx context.Context, id string) (taskstore.Task, error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	failures := 0
	for {
		if err := ctx.Err(); err != nil {
			return taskstore.Task{}, err
		}
		readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		task, err := s.store.Get(readCtx, id)
		cancel()
		if ctx.Err() != nil {
			return taskstore.Task{}, ctx.Err()
		}
		if err != nil {
			failures++
			if errors.Is(err, taskstore.ErrNotFound) || failures >= 3 {
				return taskstore.Task{}, err
			}
		} else {
			failures = 0
			if terminal(task.Status) {
				return task, nil
			}
		}
		select {
		case <-ctx.Done():
			return taskstore.Task{}, ctx.Err()
		case <-ticker.C:
		}
	}
}
