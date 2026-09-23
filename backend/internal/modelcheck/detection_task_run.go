package modelcheck

import (
	"context"
	"errors"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"slices"
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
	if value.Precheck {
		task.Result["mode"] = combinedMode
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
	ctx, cancel := context.WithTimeout(parent, s.taskTimeout)
	defer cancel()
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
	target, err := s.prepareOAuthTarget(ctx, accounts)
	if err != nil {
		fail(err)
		return
	}
	if target != nil {
		ctx = targetguard.Expect(ctx, *target)
	}
	release, err := s.reserveDetectionAccounts(accounts)
	if err != nil {
		fail(err)
		return
	}
	defer release()
	ids := make([]string, len(accounts))
	for i, account := range accounts {
		ids[i] = account.ID
	}
	accountNames := make(map[string]string, len(accounts))
	for _, account := range accounts {
		accountNames[account.ID] = account.Name
	}
	stages := 1
	if value.Precheck {
		stages++
	}
	if value.Terminal {
		stages++
	}
	total := len(accounts) * stages
	animations := []AnimationResult{}
	checks := []TerminalContinuityResult{}
	completed, success := 0, 0
	publish := func(message string) {
		result := map[string]any{"detection_task_id": value.ID, "detection_task_name": value.Name, "configuration": value, "automatic": task.Result["automatic"], "account_ids": ids, "targets": request.Targets, "group_ids_by_account": scope.GroupIDsByAccount, "group_names_by_id": scope.GroupNamesByID, "account_names_by_id": accountNames, "total": total, "completed": completed, "animations": slices.Clone(animations), "checks": slices.Clone(checks), "remote_write": false, "started_at": begin.UTC().Format(time.RFC3339Nano)}
		if value.Precheck {
			result["mode"] = combinedMode
		}
		task.Result = result
		task.Progress = completed * 100 / total
		task.Message = message
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		taskstore.PersistProgress(s.tasks, task)
	}
	questions, _ := normalizePrecheckQuestions(precheckMode, value.PrecheckQuestions)
	for _, account := range accounts {
		modes := []string{"animation"}
		if value.Precheck {
			modes = append([]string{precheckMode}, modes...)
		}
		for _, mode := range modes {
			if ctx.Err() != nil {
				break
			}
			if mode == "animation" && value.Terminal {
				publish(fmt.Sprintf("账号 %s：终端检测 %d 轮", account.ID, value.TerminalRounds))
				result := s.runTerminalRounds(ctx, account, value.TimeoutSeconds, value.Model, task.ID+"-"+account.ID+"-terminal", value.TerminalRounds)
				checks = append(checks, result)
				completed++
				if result.Verdict != "error" {
					success++
				}
				if ctx.Err() != nil {
					break
				}
			}
			publish(fmt.Sprintf("账号 %s：%s", account.ID, animationModeLabel(mode)))
			result := AnimationResult{AccountID: account.ID, AccountName: account.Name, Model: value.Model, RequestID: task.ID + "-" + account.ID + "-" + mode, Mode: mode, Status: "failed"}
			if err := s.runAnimationWithRetry(ctx, account, value.TimeoutSeconds, nil, questions, &result); err != nil {
				result.Error = safeCredentialError(err)
			} else {
				result.Status = "succeeded"
				success++
			}
			result.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
			animations = append(animations, result)
			completed++
			publish(fmt.Sprintf("已完成 %d/%d 项检测", completed, total))
		}
		if ctx.Err() != nil {
			break
		}
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
