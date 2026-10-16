package onboarding

import (
	"context"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/redact"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type ProbeStep struct {
	Stage      string `json:"stage"`
	Status     string `json:"status"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at,omitempty"`
}

type probeReporterKey struct{}
type probeReporter func(string, string)

func reportProbe(ctx context.Context, stage, status string) {
	if report, ok := ctx.Value(probeReporterKey{}).(probeReporter); ok {
		report(stage, status)
	}
}

func probeStep(ctx context.Context, stage string) func(error) {
	reportProbe(ctx, stage, "running")
	finished := false
	return func(err error) {
		if finished {
			return
		}
		finished = true
		status := "succeeded"
		if err != nil {
			status = "failed"
		}
		reportProbe(ctx, stage, status)
	}
}

// EnqueueProbe exposes only stage metadata and the public result; credentials
// stay inside the synchronous domain operations.
func (s *Service) EnqueueProbe(ctx context.Context, action, host, groupID, model, mode string) (taskstore.Task, error) {
	return s.EnqueueProbeWithPrompt(ctx, action, host, groupID, model, mode, "")
}

func (s *Service) EnqueueProbeWithPrompt(ctx context.Context, action, host, groupID, model, mode, promptID string) (taskstore.Task, error) {
	if promptID != "" && (action != "probe" || mode != "animation") {
		return taskstore.Task{}, errors.New("仅动画探活支持选择动画提示词")
	}
	if action != "models" && action != "model-options" && action != "probe" && action != "cleanup" {
		return taskstore.Task{}, errors.New("不支持的探活操作")
	}
	if strings.TrimSpace(host) == "" || strings.TrimSpace(groupID) == "" {
		return taskstore.Task{}, errors.New("请选择有效的上游与分组")
	}
	if action == "probe" && (strings.TrimSpace(model) == "" || len(model) > 255) {
		return taskstore.Task{}, errors.New("请选择有效的测试模型")
	}
	if action == "probe" && mode != "" && mode != "default" && mode != "stream" && mode != "animation" && mode != "candy" && mode != "verification" && mode != "fingerprint" && mode != "reference" {
		return taskstore.Task{}, errors.New("不支持的探活模式")
	}
	animationPrompt := ""
	if action == "probe" && (mode == "verification" || mode == "fingerprint" || mode == "reference") && s.animations == nil {
		return taskstore.Task{}, errors.New("模型检测服务尚未就绪，请稍后重试")
	}
	if action == "probe" && (mode == "animation" || mode == "candy") {
		if s.animations == nil {
			return taskstore.Task{}, errors.New("模型测试服务尚未就绪，请稍后重试")
		}
		var err error
		animationPrompt, err = s.animations.ResolveAnimationPrompt(promptID)
		if err != nil {
			return taskstore.Task{}, err
		}
	}
	task, err := s.newQueuedTask("onboarding-probe-"+action, "探活任务已排队")
	if err != nil {
		return taskstore.Task{}, err
	}
	task.Result = map[string]any{"host": host, "group_id": groupID, "request_id": task.ID, "steps": []ProbeStep{}}
	if err = s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	if err = taskrunner.GoTask(s.taskRunner, task.ID, func(parent context.Context) {
		s.executeProbeTask(context.WithValue(parent, probeAnimationPromptKey{}, animationPrompt), task, action, host, groupID, model, mode)
	}); err != nil {
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
	return task, nil
}

func (s *Service) executeProbeTask(parent context.Context, task taskstore.Task, action, host, groupID, model, mode string) {
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()
	steps := []ProbeStep{}
	// Copy each snapshot so task stores and readers never share a mutable map.
	task.Result = map[string]any{"host": host, "group_id": groupID}
	var saveErr error
	var verification map[string]any
	persist := func(stage string) {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		task.Result = map[string]any{"host": host, "group_id": groupID, "request_id": task.ID, "stage": stage, "steps": append([]ProbeStep{}, steps...)}
		if verification != nil {
			task.Result["active"] = map[string]any{"verification": verification}
			task.Result["total"], task.Result["completed"], task.Result["phase"] = 1, 0, "testing"
		}
		task.Status, task.UpdatedAt = "running", now
		persistCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if err := s.tasks.Save(persistCtx, task); err != nil {
			saveErr = err
			cancel()
		}
	}
	report := probeReporter(func(stage, status string) {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if status == "running" || status == "skipped" {
			steps = append(steps, ProbeStep{Stage: stage, Status: status, StartedAt: now})
		} else if len(steps) > 0 && steps[len(steps)-1].Stage == stage {
			steps[len(steps)-1].Status, steps[len(steps)-1].FinishedAt = status, now
		}
		persist(stage)
	})
	ctx = context.WithValue(ctx, probeReporterKey{}, report)
	ctx = context.WithValue(ctx, probeVerificationReporterKey{}, probeVerificationReporter(func(value map[string]any, completed, total int) {
		verification = value
		task.Progress = completed * 95 / max(1, total)
		// Publish a fresh snapshot without appending duplicate request stages.
		persist("verification")
	}))
	var err error
	var models []string
	var result ProbeResult
	switch action {
	case "models":
		models, err = s.ProbeModels(ctx, host, groupID)
	case "model-options":
		models, err = s.probeModelOptions(ctx, host, groupID)
	case "probe":
		result, err = s.Probe(ctx, host, groupID, model, mode)
	case "cleanup":
		err = s.CancelProbe(ctx, host, groupID)
	}
	if err == nil && saveErr != nil {
		err = saveErr
	}
	task.Result = map[string]any{"host": host, "group_id": groupID, "request_id": task.ID, "steps": append([]ProbeStep{}, steps...)}
	if (action == "models" || action == "model-options") && err == nil {
		task.Result["models"] = models
	}
	if result.RequestModel != "" {
		task.Result["probe_result"] = result
	}
	if result.Verification != nil {
		task.Result["tests"] = []map[string]any{result.Verification}
		task.Result["total"], task.Result["completed"] = 1, 1
		task.Progress = 100
	}
	task.Status, task.Message = "succeeded", "探活操作已完成"
	if err != nil {
		task.Status, task.Message = "failed", redact.Secrets(safeError(err))
	} else if action == "probe" && result.Status != "passed" && result.Status != "completed" {
		task.Status, task.Message = "failed", result.Message
	}
	if ctx.Err() != nil {
		task.Status, task.Message = "cancelled", "探活已取消"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			task.Status, task.Message = "failed", "探活任务超时，已保留完成的结果；请缩小检测范围或检查上游响应速度后重试"
		}
		for _, step := range steps {
			if (step.Stage == "cleanup_key" || step.Stage == "reconcile_key") && step.Status == "failed" {
				task.Status, task.Message = "failed", "探活已取消，但临时 Key 清理失败，请重试清理"
			}
		}
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	taskstore.PersistFinal(s.tasks, task)
}
