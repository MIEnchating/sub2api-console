package taskrecovery

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type recoveryStore interface {
	taskstore.Saver
	RecoveryTask(context.Context, string) (taskstore.Task, error)
}

// Enqueue retains only identity and a checkpoint digest while waiting for a
// worker slot. Domain callbacks must not capture the original task themselves.
// release runs even when cancellation, deletion, or a failed read skips work;
// the caller remains responsible for releasing reservations if enqueue fails.
func Enqueue(runner taskrunner.Runner, saver taskstore.Saver, task taskstore.Task, release func(), run func(context.Context, taskstore.Task)) error {
	store, ok := saver.(recoveryStore)
	if !ok {
		return errors.New("任务存储不支持恢复时重新读取，请重新提交")
	}
	if run == nil {
		return taskrunner.ErrNilTask
	}
	checkpoint, err := json.Marshal(task.Recovery)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(checkpoint)
	id, skill, operation := task.ID, task.Skill, task.Operation
	return taskrunner.GoTask(runner, id, func(parent context.Context) {
		if release != nil {
			defer release()
		}
		// A cancelled waiting worker still needs a bounded read to preserve
		// completed details and distinguish manual cancellation from shutdown.
		readContext, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
		current, err := store.RecoveryTask(readContext, id)
		cancel()
		if errors.Is(err, taskstore.ErrNotFound) {
			return
		}
		if errors.Is(err, taskstore.ErrInvalidRecovery) {
			failRecovery(store, current, err)
			return
		}
		if err != nil {
			// Do not replace unreadable persisted results with an empty task.
			// It remains queued for the next startup recovery attempt.
			slog.Error("排队恢复任务读取失败", "task_id", id, "error", err)
			return
		}
		raw, err := json.Marshal(current.Recovery)
		if err != nil || current.Skill != skill || current.Operation != operation || sha256.Sum256(raw) != digest {
			failRecovery(store, current, errors.New("任务恢复参数在排队期间已变化，请重新提交"))
			return
		}
		if parent.Err() != nil {
			taskstore.MarkCancelled(parent, &current, "恢复任务在排队期间已取消")
			taskstore.PersistFinal(store, current)
			return
		}
		run(parent, current)
	})
}
