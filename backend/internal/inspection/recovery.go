package inspection

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrecovery"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type inspectionRecovery struct {
	Request RunRequest `json:"request"`
	Target  string     `json:"target,omitempty"`
}

func (r *Runner) queueRequest(ctx context.Context, request RunRequest) (taskstore.Task, error) {
	task, err := newInspectionTask(r.now().UTC(), request.Automatic)
	if err != nil {
		return taskstore.Task{}, err
	}
	input := inspectionRecovery{Request: request}
	// Restore using current inspection settings, never an obsolete policy snapshot.
	input.Request.AutoConfig = nil
	if r.targets != nil {
		target, err := targetguard.Expected(ctx, r.targets)
		if err != nil {
			return taskstore.Task{}, err
		}
		input.Target = targetguard.Fingerprint(target)
	}
	if err := taskstore.WithRecovery(&task, input); err != nil {
		return taskstore.Task{}, err
	}
	if err := r.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	return task, nil
}

func (s *ManualService) ResumeTask(ctx context.Context, task taskstore.Task) error {
	var input inspectionRecovery
	if err := taskstore.RecoveryInput(task, &input); err != nil {
		return err
	}
	var workerContext = context.Background()
	if input.Target != "" {
		if s.runner.targets == nil {
			return targetguard.ErrChanged
		}
		target, err := targetguard.Expected(ctx, s.runner.targets)
		if err != nil {
			return err
		}
		if targetguard.Fingerprint(target) != input.Target {
			return targetguard.ErrChanged
		}
		workerContext = targetguard.Expect(workerContext, target)
	}
	config, err := s.runner.repositoryAutoInspectionConfig(ctx)
	if err != nil {
		return err
	}
	if input.Request.Automatic && !config.Enabled {
		return errors.New("自动巡检已关闭，不再恢复旧任务")
	}
	input.Request.AutoConfig = &config
	if err := s.runner.restoreDueStages(ctx, task); err != nil {
		return err
	}
	s.scheduler.mu.Lock()
	busy := s.scheduler.running
	s.scheduler.mu.Unlock()
	if busy {
		return taskrecovery.ErrBusy
	}
	return taskrunner.GoTask(s.taskRunner, task.ID, func(parent context.Context) {
		if input.Target != "" {
			target, _ := targetguard.Expected(workerContext, s.runner.targets)
			parent = targetguard.Expect(parent, target)
		}
		s.execute(parent, task, input.Request)
	})
}

// Stage due markers are written before external work. A restart must release
// only markers still belonging to this run, never a newer completed heartbeat.
func (r *Runner) restoreDueStages(ctx context.Context, task taskstore.Task) error {
	raw, _ := task.Result["run_started_at"].(string)
	started, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil
	}
	decode := func(key string) []string {
		raw, _ := json.Marshal(task.Result[key])
		var values []string
		_ = json.Unmarshal(raw, &values)
		return values
	}
	completed := decode("completed_operations")
	active := append(decode("active_operations"), decode("interrupted_operations")...)
	rawPlan, _ := json.Marshal(task.Result["planned_operations"])
	var planned []QueueOperation
	if err := json.Unmarshal(rawPlan, &planned); err != nil {
		return err
	}
	markers := map[string]string{operationTrafficRefresh: "traffic", operationUpstreamSync: "upstream-sync", operationPriceManagement: "price-management", operationAccountRateSync: "account-rate-sync"}
	for _, operation := range planned {
		marker := markers[operation.Operation]
		if marker == "" || (slices.Contains(completed, operation.Operation) && !slices.Contains(active, operation.Operation)) {
			continue
		}
		if err := r.repository.ResetInspectionTask(ctx, marker, started); err != nil {
			return err
		}
	}
	return nil
}

// CancelTask stops only the matching running inspection, leaving future
// scheduled heartbeats enabled. The caller persists the cancellation first.
func (s *Scheduler) CancelTask(taskID string) bool {
	s.mu.Lock()
	if !s.running || s.lastTaskID == nil || *s.lastTaskID != taskID || s.currentCancel == nil {
		s.mu.Unlock()
		return false
	}
	cancel := s.currentCancel
	s.cancelRequested = true
	s.mu.Unlock()
	cancel()
	return true
}
