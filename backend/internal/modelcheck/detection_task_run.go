package modelcheck

import (
	"context"
	"errors"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"log/slog"
	"slices"
	"sync"
	"time"
)

func (s *Service) RunDetectionTask(ctx context.Context, id string, version int) (taskstore.Task, error) {
	return s.enqueueDetectionTask(ctx, id, version, false, time.Now())
}
func (s *Service) enqueueDetectionTask(ctx context.Context, id string, version int, automatic bool, now time.Time) (taskstore.Task, error) {
	state := s.detectionTasks
	state.mu.Lock()
	value, ok := state.values[id]
	if !ok || value.Version != version {
		state.mu.Unlock()
		return taskstore.Task{}, errors.New("检测任务已变化，请刷新后重试")
	}
	if automatic && (!value.Automatic || state.next[id].After(now)) {
		state.mu.Unlock()
		return taskstore.Task{}, nil
	}
	if state.active[id] != "" {
		state.mu.Unlock()
		return taskstore.Task{}, errors.New("此检测任务正在执行，请等待结束")
	}
	taskID, err := randomTaskID()
	if err != nil {
		state.mu.Unlock()
		return taskstore.Task{}, err
	}
	value = cloneDetectionTask(value)
	state.active[id] = taskID
	if automatic {
		state.next[id] = nextAnimationTime(detectionTiming(value), now)
	}
	state.mu.Unlock()
	release := func() {
		state.mu.Lock()
		defer state.mu.Unlock()
		delete(state.active, id)
		if current, ok := state.values[id]; ok && current.Version == value.Version && current.ScheduleType != "daily" {
			state.next[id] = nextAnimationTime(detectionTiming(current), time.Now())
		}
	}
	created := now.UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: taskID, Skill: animationSkill, Operation: "managed-model-detection", Status: "queued", Message: "检测任务已排队", CreatedAt: created, UpdatedAt: created, Result: map[string]any{"detection_task_id": id, "detection_task_name": value.Name, "configuration": value, "automatic": automatic, "remote_write": false}}
	task.Result["mode"] = detectionTaskMode(value)
	if err := s.prepareRecovery(ctx, &task, detectionRecovery{Configuration: &value}, nil); err != nil {
		release()
		return taskstore.Task{}, err
	}
	if err := s.tasks.Save(ctx, task); err != nil {
		release()
		return taskstore.Task{}, err
	}
	err = taskrunner.GoTask(s.taskRunner, taskID, func(parent context.Context) { defer release(); s.executeDetectionTask(parent, task, value) })
	if err != nil {
		release()
		taskstore.PersistLaunchFailure(s.tasks, task, err)
	}
	state.mu.Lock()
	if err != nil {
		state.errors[id] = safeCredentialError(err)
	} else {
		state.last[id] = taskID
		delete(state.errors, id)
	}
	state.mu.Unlock()
	if err != nil {
		return taskstore.Task{}, err
	}
	return task, nil
}
func (s *Service) RunDueDetectionTasks(ctx context.Context, now time.Time) {
	for _, value := range s.DetectionTasks() {
		if ctx.Err() != nil {
			return
		}
		if !value.Automatic || value.Running {
			continue
		}
		next, err := time.Parse(time.RFC3339Nano, value.NextAt)
		if err != nil || next.After(now) {
			continue
		}
		if _, err := s.enqueueDetectionTask(ctx, value.ID, value.Version, true, now); err != nil {
			s.detectionTasks.mu.Lock()
			s.detectionTasks.errors[value.ID] = safeCredentialError(err)
			s.detectionTasks.mu.Unlock()
		}
	}
}

func (s *Service) prepareDetectionTask(ctx context.Context, value DetectionTask, ids []string) (AnimationRequest, []selectedAccount, error) {
	if len(ids) == 0 {
		return AnimationRequest{}, nil, errors.New("所选分组当前没有账号，请同步目录或调整分组")
	}
	request := AnimationRequest{TimeoutSeconds: value.TimeoutSeconds, Targets: make([]AnimationTarget, 0, len(ids))}
	for _, id := range ids {
		request.Targets = append(request.Targets, AnimationTarget{AccountID: id, Model: value.Model})
	}
	return s.prepareAnimation(ctx, request)
}

// All requested stages share the same account reservation and task cancellation.
func (s *Service) reserveDetectionAccounts(accounts []selectedAccount) (func(), error) {
	s.animation.activeMu.Lock()
	defer s.animation.activeMu.Unlock()
	s.terminalMu.Lock()
	defer s.terminalMu.Unlock()
	for _, account := range accounts {
		if s.animation.active[account.ID] || s.terminalActive[account.ID] {
			return nil, fmt.Errorf("账号 %s 正在检测，请等待结束后重试", account.ID)
		}
	}
	for _, account := range accounts {
		s.animation.active[account.ID] = true
		s.terminalActive[account.ID] = true
	}
	return func() {
		s.animation.activeMu.Lock()
		s.terminalMu.Lock()
		defer s.animation.activeMu.Unlock()
		defer s.terminalMu.Unlock()
		for _, account := range accounts {
			delete(s.animation.active, account.ID)
			delete(s.terminalActive, account.ID)
		}
	}, nil
}
func (s *Service) executeDetectionTask(parent context.Context, task taskstore.Task, value DetectionTask) {
	parent = context.WithValue(parent, managedDetectionSlotsKey{}, true)
	ctx, cancelPreparation := context.WithTimeout(parent, s.taskTimeout)
	defer cancelPreparation()
	begin := time.Now()
	task.Status = "running"
	task.Message = "正在读取分组当前账号"
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if !taskstore.SaveRunning(ctx, s.tasks, task) {
		return
	}
	fail := func(err error) {
		task.Status = "failed"
		task.Message = safeCredentialError(err)
		taskstore.MarkCancelled(ctx, &task, "检测任务已取消")
		taskstore.PersistFinal(s.tasks, task)
	}
	scope, err := s.detectionTasks.repository.DetectionTaskScope(ctx, value.GroupIDs)
	if err != nil {
		fail(err)
		return
	}
	task.Result["account_ids"] = scope.AccountIDs
	task.Result["group_ids_by_account"] = scope.GroupIDsByAccount
	task.Result["group_names_by_id"] = scope.GroupNamesByID
	request, accounts, err := s.prepareDetectionTask(ctx, value, scope.AccountIDs)
	if err != nil {
		fail(err)
		return
	}
	cancelPreparation()
	ctx, cancel := context.WithTimeout(parent, detectionTaskTimeout(value, len(accounts), s.taskTimeout))
	defer cancel()
	if err := bindDetectionRecovery(&task, accounts); err != nil {
		fail(err)
		return
	}
	if !taskstore.SaveRunning(ctx, s.tasks, task) {
		return
	}
	target, err := s.prepareOAuthTarget(ctx, accounts)
	if err != nil {
		fail(err)
		return
	}
	if target != nil {
		ctx = targetguard.Expect(ctx, *target)
	}
	if reserved, _ := ctx.Value(reservedDetectionKey{}).(bool); !reserved {
		release, err := s.reserveDetectionAccounts(accounts)
		if err != nil {
			fail(err)
			return
		}
		defer release()
	}
	ids := make([]string, len(accounts))
	for i, account := range accounts {
		ids[i] = account.ID
	}
	accountNames := make(map[string]string, len(accounts))
	for _, account := range accounts {
		accountNames[account.ID] = account.Name
	}
	stages := []string{}
	if value.Precheck {
		stages = append(stages, precheckMode)
	}
	if value.Terminal {
		stages = append(stages, "terminal")
	}
	if value.Animation == nil || *value.Animation {
		stages = append(stages, "animation")
	}
	total := len(accounts) * len(stages)
	animations := previousResults[AnimationResult](task, "animations")
	checks := previousResults[TerminalContinuityResult](task, "checks")
	done := map[string]bool{}
	terminalCompleted, terminalSuccess := terminalCounts(checks)
	completed, success := len(animations)+terminalCompleted, terminalSuccess
	for _, result := range animations {
		done[animationResultKey(result.AccountID, result.Mode)] = true
		if result.Status == "succeeded" {
			success++
		}
	}
	if err := s.persistAnimationEvidence(ctx, task.ID, animations); err != nil {
		slog.Error("保存分组动画检测证据失败", "task_id", task.ID, "error", err)
	}
	for _, result := range checks {
		done[animationResultKey(result.AccountID, "terminal")] = !result.Incomplete
	}
	publish := func(message string) {
		result := map[string]any{"detection_task_id": value.ID, "detection_task_name": value.Name, "configuration": value, "automatic": task.Result["automatic"], "account_ids": ids, "targets": request.Targets, "group_ids_by_account": scope.GroupIDsByAccount, "group_names_by_id": scope.GroupNamesByID, "account_names_by_id": accountNames, "total": total, "completed": completed, "animations": slices.Clone(animations), "checks": slices.Clone(checks), "remote_write": false, "started_at": begin.UTC().Format(time.RFC3339Nano)}
		result["mode"] = detectionTaskMode(value)
		task.Result = result
		task.Progress = completed * 100 / total
		task.Message = message
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		taskstore.PersistProgress(s.tasks, task)
	}
	questions, _ := normalizePrecheckQuestions(precheckMode, value.PrecheckQuestions)
	publish(fmt.Sprintf("正在并发检测 %d 个账号，同时检测上限 %d", len(accounts), value.Concurrency))
	type stageEvent struct {
		animation *AnimationResult
		terminal  *TerminalContinuityResult
		final     bool
	}
	priorChecks := make(map[string]TerminalContinuityResult, len(checks))
	for _, check := range checks {
		priorChecks[check.AccountID] = check
	}
	jobs := make(chan selectedAccount)
	events := make(chan stageEvent)
	var workers sync.WaitGroup
	for range min(value.Concurrency, len(accounts)) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for account := range jobs {
				for _, mode := range stages {
					if ctx.Err() != nil {
						return
					}
					if done[animationResultKey(account.ID, mode)] {
						continue
					}
					if mode == "terminal" {
						result := s.runTerminalCheckpoint(ctx, account, value.TimeoutSeconds, value.Model, task.ID+"-"+account.ID+"-terminal", value.TerminalRounds, priorChecks[account.ID], func(snapshot TerminalContinuityResult) {
							snapshot.Incomplete = true
							events <- stageEvent{terminal: &snapshot}
						})
						if taskstore.Interrupted(ctx) {
							return
						}
						events <- stageEvent{terminal: &result, final: true}
						continue
					}
					result := AnimationResult{AccountID: account.ID, AccountName: account.Name, Model: value.Model, RequestID: task.ID + "-" + account.ID + "-" + mode, Mode: mode, Status: "failed"}
					if err := s.runAnimationWithRetry(ctx, account, value.TimeoutSeconds, nil, questions, &result); err != nil {
						if taskstore.Interrupted(ctx) {
							return
						}
						result.Error = safeCredentialError(err)
					} else {
						result.Status = "succeeded"
					}
					result.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
					events <- stageEvent{animation: &result, final: true}
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, account := range accounts {
			select {
			case jobs <- account:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { workers.Wait(); close(events) }()
	for event := range events {
		if event.terminal != nil {
			checks = upsertTerminal(checks, *event.terminal)
			if event.final {
				completed++
				if event.terminal.Verdict != "error" {
					success++
				}
			}
		} else if event.animation != nil {
			animations = append(animations, *event.animation)
			if event.animation.Status == "succeeded" {
				success++
			}
			if err := s.persistAnimationEvidence(ctx, task.ID, []AnimationResult{*event.animation}); err != nil {
				slog.Error("保存分组动画检测证据失败", "task_id", task.ID, "account_id", event.animation.AccountID, "mode", event.animation.Mode, "error", err)
			}
			completed++
		}
		publish(fmt.Sprintf("已完成 %d/%d 项检测", completed, total))
	}
	publish(fmt.Sprintf("检测任务完成：成功 %d，失败或未执行 %d", success, total-success))
	task.Status = "succeeded"
	if success == 0 {
		task.Status = "failed"
	} else if success < total {
		task.Status = "partial"
	}
	task.Result["duration_ms"] = time.Since(begin).Milliseconds()
	task.Result["completed_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	taskstore.MarkCancelled(ctx, &task, "检测任务已取消")
	taskstore.PersistFinal(s.tasks, task)
}

func detectionTaskTimeout(value DetectionTask, accountCount int, minimum time.Duration) time.Duration {
	const perRequestOverhead = 15 * time.Second
	requestBudget := time.Duration(value.TimeoutSeconds)*time.Second + perRequestOverhead
	perAccount := time.Duration(0)
	if value.Precheck {
		perAccount += requestBudget
	}
	if value.Terminal {
		perAccount += time.Duration(value.TerminalRounds) * requestBudget
	}
	if value.Animation == nil || *value.Animation {
		perAccount += animationMaximumAttempts * requestBudget
	}
	concurrency := value.Concurrency
	if concurrency <= 0 {
		concurrency = defaultDetectionConcurrency
	}
	return max(minimum, time.Duration((accountCount+concurrency-1)/concurrency)*perAccount)
}

func detectionTaskMode(value DetectionTask) string {
	if value.Animation == nil || *value.Animation {
		if value.Precheck {
			return combinedMode
		}
		return "animation"
	}
	if value.Precheck {
		return precheckMode
	}
	return "terminal"
}
