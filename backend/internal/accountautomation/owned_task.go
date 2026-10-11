package accountautomation

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskrecovery"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type taskGroupOwnerKey struct{}

func taskGroupOwnerID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(taskGroupOwnerKey{}).(string)
	return id
}

// Check both before enqueueing recovery and immediately before a worker starts.
// A parent that has disappeared or stopped cannot authorize an orphaned child.
func (s *Service) allowOwnedTask(ctx context.Context, task taskstore.Task) (bool, error) {
	if task.Recovery == nil {
		return true, nil
	}
	var owner struct {
		TaskGroupTaskID *string `json:"task_group_task_id"`
	}
	if err := taskstore.RecoveryInput(task, &owner); err != nil {
		return false, err
	}
	if owner.TaskGroupTaskID == nil {
		// Old scheduled previews did not distinguish standalone runs from group
		// children. Do not infer ownership after their parent may have been cancelled.
		if task.Result["schedule_id"] != nil {
			return false, errors.New("旧计划预览缺少任务组归属信息，请重新运行计划")
		}
		return true, nil
	}
	if *owner.TaskGroupTaskID == "" {
		return true, nil
	}
	if !validTaskGroupID(*owner.TaskGroupTaskID) {
		return false, errors.New("子任务的任务组归属无效，不能恢复")
	}
	parent, err := s.store.Get(ctx, *owner.TaskGroupTaskID)
	if err != nil {
		return false, errors.New("子任务所属任务组无法读取，不能恢复，请核对任务记录")
	}
	if parent.Skill != Skill || parent.Operation != GroupOperation {
		return false, errors.New("子任务所属任务组身份不一致，不能恢复")
	}
	if parent.Status == "cancelled" {
		_, err := s.store.CancelTask(ctx, task.ID)
		return false, err
	}
	if parent.Status != "queued" && parent.Status != "running" {
		return false, errors.New("所属任务组已经停止，不能单独恢复子任务，请核对后重新运行任务组")
	}
	return true, nil
}

func (s *Service) cancelOwnedTask(owner context.Context, id string) {
	if context.Cause(owner) == context.Canceled {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := s.store.CancelTask(ctx, id); err != nil {
			slog.Error("运营子任务取消状态保存失败", "task_id", id, "error", err)
		}
	}
	s.cancelAutomationTask(id)
}

// Schedule workers retain their owner's cancellation even across the interval
// between persisting a child and exposing its ID to the group follower.
func (s *Service) dispatchOwned(owner context.Context, task taskstore.Task) error {
	if owner == nil {
		return s.enqueueOwnedTask(task, nil, s.runStored)
	}
	id := task.ID
	stopWaiting := context.AfterFunc(owner, func() { s.cancelOwnedTask(owner, id) })
	err := s.enqueueOwnedTask(task, func() { stopWaiting() }, func(ctx context.Context, current taskstore.Task) {
		run, cancel := context.WithCancelCause(context.WithoutCancel(ctx))
		propagate := func() {
			if cause := context.Cause(owner); cause != nil {
				cancel(cause)
			} else if cause := context.Cause(ctx); cause != nil {
				cancel(cause)
			}
		}
		stopOwner := context.AfterFunc(owner, propagate)
		stopWorker := context.AfterFunc(ctx, propagate)
		defer stopOwner()
		defer stopWorker()
		defer cancel(context.Canceled)
		propagate()
		s.runStored(run, current)
	})
	if err != nil {
		stopWaiting()
		return err
	}
	// Cancellation may have raced registration of the child in the task runner.
	if owner.Err() != nil {
		s.cancelOwnedTask(owner, id)
	}
	return nil
}

func (s *Service) enqueueOwnedTask(task taskstore.Task, release func(), run func(context.Context, taskstore.Task)) error {
	if _, ok := s.store.(interface {
		RecoveryTask(context.Context, string) (taskstore.Task, error)
	}); ok {
		return taskrecovery.Enqueue(s.runner, s.store, task, release, run)
	}
	return taskrunner.GoTask(s.runner, task.ID, func(ctx context.Context) {
		if release != nil {
			defer release()
		}
		run(ctx, task)
	})
}
