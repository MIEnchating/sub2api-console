package newapimanagement

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"maps"
	"strconv"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type ChannelTarget struct {
	ID      string `json:"id" binding:"required,max=20"`
	Version string `json:"version" binding:"required,len=64"`
}

type ChannelBatchInput struct {
	Channels []ChannelTarget `json:"channels" binding:"required,min=1,max=50,dive"`
	Action   string          `json:"action" binding:"required,oneof=add remove"`
	Models   []string        `json:"models" binding:"required,min=1,max=1000,dive,required,max=255"`
}

type ChannelBatchItem struct {
	ChannelID string `json:"channel_id"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

type channelTaskStore interface {
	Save(context.Context, taskstore.Task) error
}

type ChannelTasks struct {
	service *Service
	store   channelTaskStore
	runner  taskrunner.Runner
}

func NewChannelTasks(service *Service, store channelTaskStore, runner taskrunner.Runner) *ChannelTasks {
	return &ChannelTasks{service: service, store: store, runner: runner}
}

func (s *ChannelTasks) Enqueue(ctx context.Context, platformID string, input ChannelBatchInput) (taskstore.Task, error) {
	if s.store == nil || s.runner == nil {
		return taskstore.Task{}, serviceError(ErrorUnavailable, "渠道任务服务不可用")
	}
	if len(input.Channels) == 0 || len(input.Channels) > 50 {
		return taskstore.Task{}, serviceError(ErrorValidation, "每次请选择 1 到 50 个渠道")
	}
	seen := map[string]bool{}
	targets := append([]ChannelTarget{}, input.Channels...)
	for _, target := range targets {
		if err := validateChannelModelChange(target.ID, ChannelModelChange{Action: input.Action, Models: input.Models, Version: target.Version}); err != nil {
			return taskstore.Task{}, err
		}
		if seen[target.ID] {
			return taskstore.Task{}, serviceError(ErrorValidation, "不能重复选择同一渠道")
		}
		seen[target.ID] = true
	}
	if _, err := s.service.requirePlatform(ctx, platformID); err != nil {
		return taskstore.Task{}, err
	}
	input.Channels = targets
	input.Models = append([]string{}, input.Models...)
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return taskstore.Task{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{ID: hex.EncodeToString(id), Skill: "newapi", Operation: "newapi-channel-models", Status: "queued", Message: "等待批量更新渠道模型", CreatedAt: now, UpdatedAt: now}
	task.Result = map[string]any{"phase": "queued", "request_id": task.ID, "platform_id": platformID, "total": len(targets), "completed": 0, "items": []ChannelBatchItem{}}
	if err := s.store.Save(ctx, task); err != nil {
		return taskstore.Task{}, err
	}
	queued := task
	queued.Result = maps.Clone(task.Result)
	// Each channel is submitted once. The task runner owns cancellation and lifetime.
	if err := taskrunner.GoTask(s.runner, task.ID, func(ctx context.Context) { s.run(ctx, task, platformID, input) }); err != nil {
		task.Status = "failed"
		task.Message = "后台任务繁忙，未执行渠道变更，请稍后重试"
		task.Result["phase"] = "complete"
		s.persist(ctx, task)
		return taskstore.Task{}, serviceError(ErrorUnavailable, task.Message)
	}
	return queued, nil
}

func validateChannelModelChange(channelID string, input ChannelModelChange) error {
	id, err := strconv.ParseInt(channelID, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != channelID {
		return serviceError(ErrorValidation, "渠道 ID 无效")
	}
	if input.Action != "add" && input.Action != "remove" {
		return serviceError(ErrorValidation, "请选择上架或下架模型")
	}
	if len(input.Version) != 64 || len(input.Models) == 0 || len(input.Models) > 1000 {
		return serviceError(ErrorValidation, "请选择模型并重新读取渠道版本")
	}
	for _, model := range input.Models {
		if !validChannelModelName(model) {
			return serviceError(ErrorValidation, "模型名称不能为空、超过 255 个字符或包含逗号及换行")
		}
	}
	return nil
}

func (s *ChannelTasks) run(ctx context.Context, task taskstore.Task, platformID string, input ChannelBatchInput) {
	items := make([]ChannelBatchItem, 0, len(input.Channels))
	succeeded := 0
	task.Status = "running"
	task.Result["phase"] = "updating"
	for index, target := range input.Channels {
		if ctx.Err() != nil {
			break
		}
		task.Message = fmt.Sprintf("正在处理渠道 %s（%d / %d）", target.ID, index+1, len(input.Channels))
		if !s.persist(ctx, task) {
			return
		}
		_, err := s.service.ChangeChannelModels(ctx, platformID, target.ID, ChannelModelChange{Action: input.Action, Models: input.Models, Version: target.Version})
		item := ChannelBatchItem{ChannelID: target.ID, Status: "succeeded", Message: "模型配置已核对"}
		if err != nil {
			item.Status = "failed"
			item.Message = "渠道变更未完成，请刷新核对后再操作"
			if KindOf(err) == ErrorConflict || KindOf(err) == ErrorValidation {
				item.Message = err.Error()
			}
		} else {
			succeeded++
		}
		items = append(items, item)
		task.Result["items"] = append([]ChannelBatchItem{}, items...)
		task.Result["completed"] = len(items)
		task.Progress = len(items) * 100 / len(input.Channels)
		// Persist each confirmed result before starting another remote write.
		if !s.persist(ctx, task) {
			return
		}
	}
	processed := len(items)
	for _, target := range input.Channels[processed:] {
		items = append(items, ChannelBatchItem{ChannelID: target.ID, Status: "cancelled", Message: "任务已取消，未执行"})
	}
	task.Status = "succeeded"
	if processed < len(input.Channels) || ctx.Err() != nil {
		task.Status = "cancelled"
	} else if succeeded == 0 {
		task.Status = "failed"
	} else if succeeded != len(input.Channels) {
		task.Status = "partial"
	}
	task.Result["items"] = items
	task.Result["phase"] = "complete"
	task.Result["succeeded"] = succeeded
	task.Message = fmt.Sprintf("渠道模型变更结束：%d 成功，%d 失败，%d 未执行", succeeded, processed-succeeded, len(input.Channels)-processed)
	s.persist(ctx, task)
}

func (s *ChannelTasks) persist(ctx context.Context, task taskstore.Task) bool {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := s.store.Save(persistCtx, task); err != nil {
		slog.Error("渠道任务进度保存失败，停止后续变更，请核对远端结果", "task_id", task.ID)
		task.Status = "failed"
		task.Message = "任务结果保存失败，已停止后续变更，请刷新渠道核对结果"
		task.Result["phase"] = "complete"
		task.Result["error_code"] = "channel_task_storage_failed"
		// Retry only local result persistence; never replay the remote update.
		if saveErr := s.store.Save(persistCtx, task); saveErr != nil {
			slog.Error("渠道任务终态保存失败", "task_id", task.ID)
		}
		return false
	}
	return true
}
