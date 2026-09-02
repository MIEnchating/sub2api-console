package inspection

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type ManualService struct {
	scheduler  *Scheduler
	runner     *Runner
	tasks      InspectionTaskStore
	taskRunner taskrunner.Runner
	timeout    time.Duration
}

type manualExecutor struct {
	runner  *Runner
	task    taskstore.Task
	request RunRequest
}

func NewManualService(scheduler *Scheduler, runner *Runner, tasks InspectionTaskStore) *ManualService {
	return &ManualService{scheduler: scheduler, runner: runner, tasks: tasks, timeout: 30 * time.Minute}
}

func (s *ManualService) UseTaskRunner(runner taskrunner.Runner) { s.taskRunner = runner }

func (s *ManualService) Enqueue(ctx context.Context, request RunRequest) (taskstore.Task, error) {
	if s.scheduler == nil || s.runner == nil || s.tasks == nil {
		return taskstore.Task{}, errors.New("巡检服务尚未就绪")
	}
	request.Automatic = false
	task, err := s.runner.QueueTask(ctx, false)
	if err != nil {
		return taskstore.Task{}, err
	}
	if err := taskrunner.Go(s.taskRunner, func(parent context.Context) { s.execute(parent, task, request) }); err != nil {
		taskstore.PersistLaunchFailure(s.tasks, task, err)
		return taskstore.Task{}, err
	}
	return task, nil
}

func (s *ManualService) execute(parent context.Context, task taskstore.Task, request RunRequest) {
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()
	started, err := s.scheduler.RunWithExecutor(ctx, time.Now().UTC(), &manualExecutor{runner: s.runner, task: task, request: request})
	if started {
		if err != nil {
			slog.Error("手动巡检调度收尾失败", "task_id", task.ID, "error", err)
		}
		return
	}
	task = manualTaskNotStarted(task, request, err, time.Now().UTC())
	taskstore.PersistFinal(s.tasks, task)
}

func manualTaskNotStarted(task taskstore.Task, request RunRequest, err error, now time.Time) taskstore.Task {
	status, message := "failed", "已有巡检正在执行，本次手动巡检未启动"
	result := map[string]any{"error": message, "remote_write": false}
	if err != nil {
		message = err.Error()
		result["error"] = message
	}
	if errors.Is(err, context.Canceled) {
		status, message = "cancelled", "巡检已取消"
		result = map[string]any{"cancelled": true, "remote_write": false}
	}
	task.Status, task.Progress, task.Message = status, 100, message
	task.UpdatedAt, task.Result = now.Format(time.RFC3339Nano), result
	return task
}

func (e *manualExecutor) Execute(ctx context.Context, _ business.AutoInspectionConfig) (ExecutionResult, error) {
	return e.runner.RunTask(ctx, e.task, e.request), nil
}
