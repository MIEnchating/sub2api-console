package taskstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
)

type Saver interface {
	Save(context.Context, Task) error
}

func SaveFinal(ctx context.Context, saver Saver, task Task) error {
	task.Result = boundedTaskResult(task.Result, maximumTaskResultBytes)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := saver.Save(ctx, task); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if attempt == 2 {
			break
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("最终任务状态保存失败：%w", ctx.Err())
		case <-timer.C:
		}
	}
	return fmt.Errorf("最终任务状态保存失败（已重试 3 次）：%w", lastErr)
}

type truncationStat struct {
	Path     string `json:"path"`
	Total    int    `json:"total"`
	Retained int    `json:"retained"`
}

func boundedTaskResult(result map[string]any, maximum int) map[string]any {
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) <= maximum {
		return result
	}
	for capPerArray := 1000; capPerArray >= 0; capPerArray /= 2 {
		decoder := json.NewDecoder(bytes.NewReader(encoded))
		decoder.UseNumber()
		var candidate map[string]any
		if err := decoder.Decode(&candidate); err != nil {
			break
		}
		stats := make([]truncationStat, 0)
		truncateTaskArrays(candidate, "$", capPerArray, &stats)
		total, retained := 0, 0
		for _, stat := range stats {
			total += stat.Total
			retained += stat.Retained
		}
		candidate["truncated"] = true
		candidate["total"] = total
		candidate["retained"] = retained
		candidate["truncation"] = stats
		if bounded, marshalErr := json.Marshal(candidate); marshalErr == nil && len(bounded) <= maximum {
			return candidate
		}
		if capPerArray == 0 {
			break
		}
	}
	fallback := map[string]any{
		"truncated": true, "total": 0, "retained": 0,
		"truncation": []truncationStat{}, "detail": "任务结果超过持久化上限，明细未保留",
	}
	for _, key := range []string{"error", "summary", "host", "account_id", "request_id", "run_key"} {
		if text, ok := result[key].(string); ok && len(text) <= 4096 {
			fallback[key] = text
		}
	}
	return fallback
}

func truncateTaskArrays(value any, path string, capPerArray int, stats *[]truncationStat) {
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			childPath := path + "." + key
			if values, ok := child.([]any); ok {
				total := len(values)
				retained := min(total, capPerArray)
				if retained < total {
					item[key] = values[:retained]
					*stats = append(*stats, truncationStat{Path: childPath, Total: total, Retained: retained})
				}
				for index := 0; index < retained; index++ {
					truncateTaskArrays(values[index], fmt.Sprintf("%s[%d]", childPath, index), capPerArray, stats)
				}
				continue
			}
			truncateTaskArrays(child, childPath, capPerArray, stats)
		}
	case []any:
		for index := 0; index < min(len(item), capPerArray); index++ {
			truncateTaskArrays(item[index], fmt.Sprintf("%s[%d]", path, index), capPerArray, stats)
		}
	}
}

func PersistFinal(saver Saver, task Task) {
	if err := SaveFinal(context.Background(), saver, task); err != nil {
		slog.Error("最终任务状态持久化失败", "task_id", task.ID, "operation", task.Operation, "status", task.Status, "error", err)
	}
}

func PersistProgress(saver Saver, task Task) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := saver.Save(ctx, task); err != nil {
		slog.Error("任务进度持久化失败", "task_id", task.ID, "operation", task.Operation, "progress", task.Progress, "error", err)
	}
}

func SaveRunning(ctx context.Context, saver Saver, task Task) bool {
	if err := saver.Save(ctx, task); err == nil {
		return true
	} else {
		if cause := ContextFailureCause(ctx); cause != nil {
			err = cause
		}
		failed := task
		failed.Status = "failed"
		failed.Progress = 100
		failed.Message = "任务启动状态保存失败：" + err.Error()
		failed.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		failed.Result = maps.Clone(task.Result)
		if failed.Result == nil {
			failed.Result = map[string]any{}
		}
		failed.Result["error"] = err.Error()
		failed.Result["remote_write"] = false
		MarkCancelled(ctx, &failed, "任务在启动期间已取消")
		PersistFinal(saver, failed)
		return false
	}
}

// ContextFailureCause excludes an ordinary cancellation while preserving
// deadline, lease-loss, and other cancellation causes as task failures.
func ContextFailureCause(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	cause := context.Cause(ctx)
	if cause == nil || cause == context.Canceled {
		return nil
	}
	return cause
}

func MarkCancelled(ctx context.Context, task *Task, message string) bool {
	if ctx == nil || task == nil || task.Status == "succeeded" || task.Status == "partial" || task.Status == "waiting_input" {
		return false
	}
	if cause := ContextFailureCause(ctx); cause != nil {
		task.Status = "failed"
		task.Progress = 100
		task.Message = "任务执行失败：" + cause.Error()
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		task.Result = maps.Clone(task.Result)
		if task.Result == nil {
			task.Result = map[string]any{}
		}
		if operationError, found := task.Result["error"]; found && operationError != nil {
			operationErrorText, isText := operationError.(string)
			if !isText || (operationErrorText != "" && operationErrorText != cause.Error() && operationErrorText != context.Canceled.Error() &&
				operationErrorText != context.DeadlineExceeded.Error()) {
				task.Result["operation_error"] = operationError
			}
		}
		delete(task.Result, "cancelled")
		task.Result["error"] = cause.Error()
		return false
	}
	if ctx.Err() != context.Canceled {
		return false
	}
	task.Status = "cancelled"
	task.Progress = 100
	task.Message = message
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	task.Result = maps.Clone(task.Result)
	if task.Result == nil {
		task.Result = map[string]any{}
	}
	delete(task.Result, "error")
	task.Result["cancelled"] = true
	return true
}

func PersistLaunchFailure(saver Saver, task Task, err error) {
	task.Status = "cancelled"
	task.Progress = 100
	task.Message = "服务正在停止，任务未启动"
	if errors.Is(err, taskrunner.ErrCapacity) {
		task.Message = "后台任务并发容量已满，任务未启动"
	}
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	task.Result = maps.Clone(task.Result)
	if task.Result == nil {
		task.Result = map[string]any{}
	}
	task.Result["cancelled"] = true
	task.Result["error"] = err.Error()
	PersistFinal(saver, task)
}
