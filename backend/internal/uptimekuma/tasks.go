package uptimekuma

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type taskRepository interface {
	Save(context.Context, taskstore.Task) error
}
type TaskService struct {
	Service *Service
	tasks   taskRepository
	runner  taskrunner.Runner
}

func NewTasks(service *Service, tasks taskRepository, runner taskrunner.Runner) *TaskService {
	return &TaskService{service, tasks, runner}
}
func (s *TaskService) Save(ctx context.Context, input ConfigInput) (taskstore.Task, error) {
	return s.enqueue(ctx, "uptime-kuma-config", func(ctx context.Context) (map[string]any, error) {
		cfg, err := s.Service.Save(ctx, input)
		return map[string]any{"revision": cfg.Revision}, err
	})
}
func (s *TaskService) Write(ctx context.Context, id int64, input WriteInput) (taskstore.Task, error) {
	return s.enqueue(ctx, "uptime-kuma-"+input.Action, func(ctx context.Context) (map[string]any, error) {
		id, err := s.Service.Write(ctx, id, input)
		return map[string]any{"monitor_id": id, "action": input.Action}, err
	})
}
func (s *TaskService) WriteResource(ctx context.Context, kind string, id int64, input ResourceInput) (taskstore.Task, error) {
	return s.enqueue(ctx, "uptime-kuma-"+kind+"-"+input.Action, func(ctx context.Context) (map[string]any, error) {
		return s.Service.WriteResource(ctx, kind, id, input)
	})
}
func (s *TaskService) enqueue(ctx context.Context, operation string, run func(context.Context) (map[string]any, error)) (taskstore.Task, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return taskstore.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: hex.EncodeToString(id), Skill: "uptime-kuma", Operation: operation, Status: "queued", Message: "等待连接 Uptime Kuma", Result: map[string]any{"phase": "queued", "request_id": hex.EncodeToString(id)}, CreatedAt: now, UpdatedAt: now}
	if err := s.tasks.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	queued := task
	err := s.runner.Go(func(ctx context.Context) {
		task.Status = "running"
		task.Progress = 20
		task.Message = "正在验证或更新 Uptime Kuma"
		task.Result = map[string]any{"phase": "requesting", "request_id": task.ID}
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := s.tasks.Save(ctx, task); err != nil {
			task.Status = "failed"
			task.Progress = 100
			task.Message = "任务进度保存失败，未执行远端操作，请稍后重试"
			s.persist(ctx, task)
			return
		}
		result, err := run(ctx)
		if result == nil {
			result = map[string]any{}
		}
		result["request_id"] = task.ID
		result["phase"] = "complete"
		task.Status = "succeeded"
		task.Progress = 100
		task.Message = "Uptime Kuma 操作已完成"
		if err != nil {
			e := PublicError(err)
			task.Status = "failed"
			task.Message = e.Message
			result["error_code"] = e.Code
			result["error"] = e.Message
		}
		task.Result = result
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		s.persist(ctx, task)
	})
	if err != nil {
		task.Status = "failed"
		task.Progress = 100
		task.Message = "后台任务繁忙，请稍后重试"
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		s.persist(ctx, task)
		return taskstore.Task{}, failure("kuma_task_unavailable", task.Message, 503)
	}
	return queued, nil
}

func (s *TaskService) persist(ctx context.Context, task taskstore.Task) {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.tasks.Save(persistCtx, task); err != nil {
		slog.Error("Uptime Kuma 任务结果保存失败，请核对远端状态", "task_id", task.ID, "status", task.Status)
	}
}
