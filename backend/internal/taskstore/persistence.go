package taskstore

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type Saver interface {
	Save(context.Context, Task) error
}

func SaveFinal(ctx context.Context, saver Saver, task Task) error {
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
