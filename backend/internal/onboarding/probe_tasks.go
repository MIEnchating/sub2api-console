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
	if action != "models" && action != "probe" && action != "cleanup" {
		return taskstore.Task{}, errors.New("不支持的探活操作")
	}
	if strings.TrimSpace(host) == "" || strings.TrimSpace(groupID) == "" {
		return taskstore.Task{}, errors.New("请选择有效的上游与分组")
	}
	if action == "probe" && (strings.TrimSpace(model) == "" || len(model) > 255) {
		return taskstore.Task{}, errors.New("请选择有效的测试模型")
	}
	if action == "probe" && mode != "" && mode != "default" && mode != "stream" {
		return taskstore.Task{}, errors.New("不支持的探活模式")
	}
	task, err := s.newQueuedTask("onboarding-probe-"+action, "探活任务已排队")
	if err != nil {
		return taskstore.Task{}, err
	}
	task.Result = map[string]any{"host": host, "group_id": groupID, "request_id": task.ID, "steps": []ProbeStep{}}
	if err = s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	if err = taskrunner.GoTask(s.taskRunner, task.ID, func(parent context.Context) { s.executeProbeTask(parent, task, action, host, groupID, model, mode) }); err != nil {
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
	report := probeReporter(func(stage, status string) {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if status == "running" || status == "skipped" {
			steps = append(steps, ProbeStep{Stage: stage, Status: status, StartedAt: now})
		} else if len(steps) > 0 && steps[len(steps)-1].Stage == stage {
			steps[len(steps)-1].Status, steps[len(steps)-1].FinishedAt = status, now
		}
		task.Result = map[string]any{"host": host, "group_id": groupID, "request_id": task.ID, "stage": stage, "steps": append([]ProbeStep{}, steps...)}
		task.Status, task.UpdatedAt = "running", now
		persistCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if err := s.tasks.Save(persistCtx, task); err != nil {
			saveErr = err
			cancel()
		}
	})
	ctx = context.WithValue(ctx, probeReporterKey{}, report)
	var err error
	var models []string
	var result ProbeResult
	switch action {
	case "models":
		models, err = s.ProbeModels(ctx, host, groupID)
	case "probe":
		result, err = s.Probe(ctx, host, groupID, model, mode)
	case "cleanup":
		err = s.CancelProbe(ctx, host, groupID)
	}
	if err == nil && saveErr != nil {
		err = saveErr
	}
	task.Result = map[string]any{"host": host, "group_id": groupID, "request_id": task.ID, "steps": append([]ProbeStep{}, steps...)}
	if action == "models" && err == nil {
		task.Result["models"] = models
	}
	if result.RequestModel != "" {
		task.Result["probe_result"] = result
	}
	task.Status, task.Message = "succeeded", "探活操作已完成"
	if err != nil {
		task.Status, task.Message = "failed", redact.Secrets(safeError(err))
	} else if action == "probe" && result.Status != "passed" {
		task.Status, task.Message = "failed", result.Message
	}
	if ctx.Err() != nil {
		task.Status, task.Message = "cancelled", "探活已取消"
		for _, step := range steps {
			if (step.Stage == "cleanup_key" || step.Stage == "reconcile_key") && step.Status == "failed" {
				task.Status, task.Message = "failed", "探活已取消，但临时 Key 清理失败，请重试清理"
			}
		}
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	taskstore.PersistFinal(s.tasks, task)
}
