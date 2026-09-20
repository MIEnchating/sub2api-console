package inspection

import (
	"context"
	"fmt"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

// Sync before planning so newly added and removed accounts affect this heartbeat.
func (r *Runner) syncManagement(ctx context.Context, task *taskstore.Task, actor string) (context.Context, error) {
	ctx = mutationguard.WithAutomaticInspection(ctx)
	if r.targets != nil {
		var err error
		ctx, err = targetguard.Capture(ctx, r.targets)
		if err != nil {
			return ctx, err
		}
	}
	task.Status, task.Progress, task.Message = "running", 5, "正在同步管理端账号与分组"
	task.UpdatedAt = r.now().UTC().Format(time.RFC3339Nano)
	task.Result = map[string]any{
		"origin":             "automatic-inspection",
		"active_operations":  []string{operationManagementSync},
		"planned_operations": []QueueOperation{{Operation: operationManagementSync, Label: "管理端账号与分组同步", Cycle: "每次自动巡检心跳", Due: true}},
	}
	if !taskstore.SaveRunning(ctx, r.tasks, *task) {
		return ctx, fmt.Errorf("管理端同步任务启动状态保存失败")
	}
	started := time.Now()
	result, err := r.management.Sync(ctx, actor)
	task.Result[operationManagementSync] = result
	task.Result["management_sync_timing"] = operationTimingDuration(operationManagementSync, started, time.Since(started))
	if err != nil {
		return ctx, fmt.Errorf("管理端账号与分组同步失败：%w", err)
	}
	return ctx, nil
}
