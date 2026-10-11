package logs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func (m *Maintenance) UseTaskRunner(runner taskrunner.Runner, saver taskstore.Saver) {
	m.runner, m.saver = runner, saver
}

func (m *Maintenance) Enqueue(ctx context.Context, days int) (taskstore.Task, error) {
	if err := ctx.Err(); err != nil {
		return taskstore.Task{}, err
	}
	if days < 1 || days > 3650 {
		return taskstore.Task{}, errors.New("日志保留天数必须在 1 到 3650 之间")
	}
	if m.runner == nil || m.saver == nil {
		return taskstore.Task{}, errors.New("日志清理任务服务尚未就绪")
	}
	// Reserve before dispatch: another click must not wait behind a long cleanup.
	select {
	case m.runGate <- struct{}{}:
	default:
		return taskstore.Task{}, errors.New("日志清理正在执行，请等待当前清理完成")
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		m.releaseRun()
		return taskstore.Task{}, err
	}
	now := m.now().UTC()
	cutoff := now.AddDate(0, 0, -days)
	task := taskstore.Task{ID: "log-cleanup-" + hex.EncodeToString(id), Skill: "console", Operation: "log-cleanup", Status: "queued", Message: "日志清理已排队", Result: map[string]any{"retention_days": days, "cutoff_at": cutoff.Format(time.RFC3339Nano), "phase": "queued"}, CreatedAt: now.Format(time.RFC3339Nano), UpdatedAt: now.Format(time.RFC3339Nano)}
	if err := m.saver.Save(ctx, task); err != nil {
		m.releaseRun()
		return taskstore.Task{}, err
	}
	m.mu.Lock()
	m.currentTaskID = task.ID
	m.mu.Unlock()
	if err := taskrunner.GoTask(m.runner, task.ID, func(parent context.Context) { m.executeCleanup(parent, task, cutoff, days) }); err != nil {
		m.releaseRun()
		taskstore.PersistLaunchFailure(m.saver, task, err)
		return taskstore.Task{}, err
	}
	return task, nil
}

func (m *Maintenance) executeCleanup(ctx context.Context, task taskstore.Task, cutoff time.Time, days int) {
	defer m.releaseRun()
	// Do not mutate the map returned to the HTTP caller.
	task.Result = map[string]any{"retention_days": days, "cutoff_at": cutoff.Format(time.RFC3339Nano)}
	task.Status = "running"
	task.Message = "正在清理过期任务记录"
	if !taskstore.SaveRunning(ctx, m.saver, task) {
		return
	}
	lastReport := time.Time{}
	report := func(result CleanupResult, phase string) error {
		now := time.Now()
		if now.Sub(lastReport) < time.Second {
			return ctx.Err()
		}
		task.Result = cleanupTaskResult(result, phase)
		task.Message = fmt.Sprintf("%s，已清理 %d 条", phase, result.Total)
		task.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
		lastReport = now
		return m.saver.Save(ctx, task)
	}
	result, err := m.clearWithProgress(ctx, cutoff, days, report)
	task.Result = cleanupTaskResult(result, "completed")
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	task.Status = "succeeded"
	task.Progress = 100
	task.Message = fmt.Sprintf("日志清理完成，已清理 %d 条", result.Total)
	if err != nil {
		task.Status = "failed"
		task.Result["phase"] = "failed"
		task.Result["error"] = err.Error()
		task.Message = fmt.Sprintf("日志清理未完成，已清理 %d 条：%s；可重新按期限清理剩余记录", result.Total, err)
		if taskstore.MarkCancelled(ctx, &task, fmt.Sprintf("清理已取消，已提交的 %d 条删除保留", result.Total)) {
			task.Result["phase"] = task.Status
		}
	}
	taskstore.PersistFinal(m.saver, task)
}

func cleanupTaskResult(r CleanupResult, phase string) map[string]any {
	return map[string]any{"tasks": r.Tasks, "runs": r.Runs, "events": r.Events, "changes": r.Changes, "protected_tasks": r.ProtectedTasks, "total": r.Total, "retention_days": r.RetentionDays, "cutoff_at": r.CutoffAt, "completed_at": r.CompletedAt, "phase": phase}
}

func (m *Maintenance) clear(ctx context.Context, cutoff time.Time, days int) (CleanupResult, error) {
	return m.clearWithProgress(ctx, cutoff, days, nil)
}

func (m *Maintenance) clearWithProgress(ctx context.Context, cutoff time.Time, days int, report func(CleanupResult, string) error) (CleanupResult, error) {
	result := CleanupResult{RetentionDays: days, CutoffAt: cutoff.Format(time.RFC3339Nano)}
	if m.business == nil || m.tasks == nil || m.settings == nil {
		return result, errors.New("日志清理服务尚未就绪")
	}
	if m.invalidate != nil {
		// Partial deletion on cancellation or failure also changes the log list.
		defer m.invalidate()
	}
	notify := func(phase string) error {
		result.Total = result.Tasks + result.Runs + result.Events + result.Changes
		if report != nil {
			return report(result, phase)
		}
		return nil
	}
	var err error
	if cleaner, ok := m.tasks.(interface {
		ClearLogsWithProgress(context.Context, *time.Time, func(int64, int64) error) (int64, int64, error)
	}); ok {
		result.Tasks, result.ProtectedTasks, err = cleaner.ClearLogsWithProgress(ctx, &cutoff, func(n, p int64) error {
			result.Tasks, result.ProtectedTasks = n, p
			return notify("清理任务记录")
		})
	} else {
		result.Tasks, result.ProtectedTasks, err = m.tasks.ClearLogs(ctx, &cutoff)
	}
	result.Total = result.Tasks
	if err != nil {
		return result, err
	}
	var records business.LogCleanupResult
	if cleaner, ok := m.business.(interface {
		ClearLogRecordsWithProgress(context.Context, *time.Time, func(business.LogCleanupResult) error) (business.LogCleanupResult, error)
	}); ok {
		records, err = cleaner.ClearLogRecordsWithProgress(ctx, &cutoff, func(r business.LogCleanupResult) error {
			result.Runs, result.Events, result.Changes = r.Runs, r.Events, r.Changes
			return notify("清理运行、事件和审计记录")
		})
	} else {
		records, err = m.business.ClearLogRecords(ctx, &cutoff)
	}
	result.Runs, result.Events, result.Changes = records.Runs, records.Events, records.Changes
	result.Total = result.Tasks + result.Runs + result.Events + result.Changes
	if err != nil {
		return result, err
	}
	completed := m.now().UTC()
	if err := m.settings.MarkLogCleanupRun(ctx, completed); err != nil {
		return result, err
	}
	result.CompletedAt = completed.Format(time.RFC3339Nano)
	return result, nil
}
