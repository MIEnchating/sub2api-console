// Package taskrecovery dispatches only domain-validated restartable tasks.
package taskrecovery

import (
	"context"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

var ErrBusy = errors.New("恢复任务所需资源仍在使用")

type Handler func(context.Context, taskstore.Task) error

func Run(ctx context.Context, store *taskstore.Store, pending []taskstore.Task, handlers map[string]Handler) {
	for len(pending) > 0 && ctx.Err() == nil {
		remaining := make([]taskstore.Task, 0, len(pending))
		for _, task := range pending {
			if ctx.Err() != nil {
				return
			}
			current, err := store.Get(ctx, task.ID)
			if errors.Is(err, taskstore.ErrNotFound) {
				continue
			}
			if err != nil {
				remaining = append(remaining, task)
				continue
			}
			if current.Status != "queued" {
				continue
			}
			handler := handlers[task.Operation]
			if handler == nil {
				err = errors.New("此任务不支持自动恢复，请重新提交")
			} else {
				err = handler(ctx, task)
			}
			if err == nil || errors.Is(err, taskrunner.ErrDuplicateTask) {
				continue
			}
			if errors.Is(err, taskrunner.ErrCapacity) || errors.Is(err, ErrBusy) {
				remaining = append(remaining, task)
				continue
			}
			if ctx.Err() != nil || errors.Is(err, taskrunner.ErrStopped) {
				return
			}
			task.Status = "failed"
			task.Message = "任务恢复失败：" + err.Error()
			task.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
			task.Result["error"] = task.Message
			taskstore.PersistFinal(store, task)
		}
		pending = remaining
		if len(pending) == 0 {
			return
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
