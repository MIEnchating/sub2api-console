package inspection

import (
	"context"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type ManualService struct {
	scheduler *Scheduler
	runner    *Runner
	tasks     InspectionTaskStore
	timeout   time.Duration
}

type manualExecutor struct {
	runner  *Runner
	task    taskstore.Task
	request RunRequest
}

func NewManualService(scheduler *Scheduler, runner *Runner, tasks InspectionTaskStore) *ManualService {
	return &ManualService{scheduler: scheduler, runner: runner, tasks: tasks, timeout: 30 * time.Minute}
}

func (s *ManualService) Enqueue(ctx context.Context, request RunRequest) (taskstore.Task, error) {
	if s.scheduler == nil || s.runner == nil || s.tasks == nil {
		return taskstore.Task{}, errors.New("巡检服务尚未就绪")
	}
	request.Automatic = false
	task, err := s.runner.QueueTask(ctx, false)
	if err != nil {
		return taskstore.Task{}, err
	}
	go s.execute(task, request)
	return task, nil
}

func (s *ManualService) execute(task taskstore.Task, request RunRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	started, err := s.scheduler.RunWithExecutor(ctx, time.Now().UTC(), &manualExecutor{runner: s.runner, task: task, request: request})
	if err == nil && started {
		return
	}
	if err == nil {
		err = errors.New("已有巡检正在执行，本次手动巡检未启动")
	}
	status, message := "failed", err.Error()
	result := map[string]any{"error": err.Error(), "remote_write": false}
	if errors.Is(err, context.Canceled) {
		status, message = "cancelled", "巡检已取消"
		result = map[string]any{"cancelled": true, "remote_write": false}
	}
	task.Status, task.Progress, task.Message, task.UpdatedAt = status, 100, message, time.Now().UTC().Format(time.RFC3339Nano)
	task.Result = result
	taskstore.PersistFinal(s.tasks, task)
}

func (e *manualExecutor) Execute(ctx context.Context, _ business.AutoInspectionConfig) (ExecutionResult, error) {
	return e.runner.RunTask(ctx, e.task, e.request), nil
}
