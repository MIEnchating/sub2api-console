package accountautomation

import (
	"context"
	"log/slog"
	"maps"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type scheduleOutcome struct{ status, message, executionID string }

func scheduledReadFailure(ctx context.Context, id, stage string, err error) scheduleOutcome {
	if ctx.Err() != nil {
		return scheduleOutcome{status: "cancelled", message: "运营任务已停止", executionID: id}
	}
	message := stage + "读取失败：" + err.Error()
	slog.Error(message, "task_id", id)
	return scheduleOutcome{status: "failed", message: message, executionID: id}
}

func (s *Service) completeScheduledRun(ctx context.Context, schedule Schedule, previewID string) scheduleOutcome {
	preview, err := s.waitScheduledTask(ctx, previewID)
	if err != nil {
		outcome := scheduledReadFailure(ctx, previewID, "预览任务", err)
		outcome.executionID = ""
		return outcome
	}
	if preview.Status != "succeeded" {
		return scheduleOutcome{status: preview.Status, message: preview.Message}
	}
	request := schedule.Request
	request.DryRun, request.PreviewTaskID = false, previewID
	execution, enqueueErr := s.enqueueOwned(ctx, request, "", ctx)
	preview.Result = maps.Clone(preview.Result)
	preview.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if enqueueErr != nil {
		preview.Result["execution_error"] = enqueueErr.Error()
		preview.Message = "预览已通过，但正式执行未启动：" + enqueueErr.Error()
	} else {
		preview.Result["execution_task_id"] = execution.ID
		preview.Message = "预览已通过，已启动正式执行任务"
	}
	// Preserve the link even when cancellation arrives just after dispatch.
	saveErr := taskstore.SaveFinal(context.WithoutCancel(ctx), s.store, preview)
	if saveErr != nil {
		slog.Error("运营任务执行关联保存失败", "task_id", previewID, "execution_task_id", execution.ID, "error", saveErr)
	}
	if enqueueErr != nil {
		if ctx.Err() != nil {
			return scheduleOutcome{status: "cancelled", message: "运营任务已停止"}
		}
		return scheduleOutcome{status: "failed", message: preview.Message}
	}
	completed, err := s.waitScheduledTask(ctx, execution.ID)
	if err != nil {
		return scheduledReadFailure(ctx, execution.ID, "正式执行任务", err)
	}
	if saveErr != nil {
		return scheduleOutcome{status: "failed", message: "执行关联保存失败：" + saveErr.Error(), executionID: execution.ID}
	}
	return scheduleOutcome{status: completed.Status, message: completed.Message, executionID: execution.ID}
}
