package accountautomation_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountautomation"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type lifecycleStore struct {
	*memoryTasks
	read func(context.Context, taskstore.Task) (taskstore.Task, error)
}

func (s *lifecycleStore) Get(ctx context.Context, id string) (taskstore.Task, error) {
	task, err := s.memoryTasks.Get(ctx, id)
	if err != nil {
		return task, err
	}
	return s.read(ctx, task)
}

func TestStopSchedulesCancelsBlockedFollowupRead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, accounts, management, tasks, request := automationFixture()
		release := make(chan struct{})
		cancelled := make(chan struct{})
		store := &lifecycleStore{memoryTasks: tasks, read: func(ctx context.Context, task taskstore.Task) (taskstore.Task, error) {
			select {
			case <-ctx.Done():
				close(cancelled)
				return taskstore.Task{}, ctx.Err()
			case <-release:
				task.Status = "failed"
				return task, nil
			}
		}}
		service := accountautomation.New(accounts, management, nil, store, immediateRunner{}, accounts)
		service.UseScheduleRepository(&memorySchedule{})
		saved := saveTaskGroupSchedule(t, service, request, "停止读取")
		if _, err := service.RunSchedule(t.Context(), saved.ID); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		service.StopSchedules()
		synctest.Wait()
		select {
		case <-cancelled:
		default:
			t.Error("stopped schedules left the followup read running")
		}
		close(release)
		if accounts.syncCalls != 0 {
			t.Error("stop started a write")
		}
	})
}

func TestMissingScheduledPreviewStopsPolling(t *testing.T) {
	var reads atomic.Int32
	synctest.Test(t, func(t *testing.T) {
		finished := make(chan struct{})
		_, accounts, management, tasks, request := automationFixture()
		store := &lifecycleStore{memoryTasks: tasks, read: func(_ context.Context, task taskstore.Task) (taskstore.Task, error) {
			count := reads.Add(1)
			if count == 1 {
				return taskstore.Task{}, taskstore.ErrNotFound
			}
			task.Status = "failed"
			close(finished)
			return task, nil
		}}
		service := accountautomation.New(accounts, management, nil, store, immediateRunner{}, accounts)
		service.UseScheduleRepository(&memorySchedule{})
		saved := saveTaskGroupSchedule(t, service, request, "已删除预览")
		if _, err := service.RunSchedule(t.Context(), saved.ID); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if reads.Load() != 1 {
			t.Errorf("missing preview polled %d times", reads.Load())
		}
		if _, err := service.RunSchedule(t.Context(), saved.ID); err != nil {
			t.Errorf("missing preview kept the schedule reserved: %v", err)
		}
		<-finished
		synctest.Wait()
		service.StopSchedules()
	})
}

func TestScheduledReadFailureRetriesAreBounded(t *testing.T) {
	var reads atomic.Int32
	synctest.Test(t, func(t *testing.T) {
		third := make(chan struct{})
		finished := make(chan struct{})
		_, accounts, management, tasks, request := automationFixture()
		store := &lifecycleStore{memoryTasks: tasks, read: func(_ context.Context, task taskstore.Task) (taskstore.Task, error) {
			count := reads.Add(1)
			if count == 3 {
				close(third)
			}
			if count <= 3 {
				return taskstore.Task{}, errors.New("fixture storage unavailable")
			}
			task.Status = "failed"
			close(finished)
			return task, nil
		}}
		service := accountautomation.New(accounts, management, nil, store, immediateRunner{}, accounts)
		service.UseScheduleRepository(&memorySchedule{})
		saved := saveTaskGroupSchedule(t, service, request, "存储失败")
		if _, err := service.RunSchedule(t.Context(), saved.ID); err != nil {
			t.Fatal(err)
		}
		<-third
		synctest.Wait()
		if reads.Load() != 3 {
			t.Errorf("storage failure polled %d times", reads.Load())
		}
		if _, err := service.RunSchedule(t.Context(), saved.ID); err != nil {
			t.Errorf("failed reads kept the schedule reserved: %v", err)
		}
		<-finished
		synctest.Wait()
		service.StopSchedules()
	})
}

func TestTaskGroupCancellationBeforePreviewReadReturnsDoesNotStartWrite(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, accounts, management, tasks, request := automationFixture()
		release := make(chan struct{})
		var reads sync.WaitGroup
		reads.Add(1)
		var first sync.Once
		store := &lifecycleStore{memoryTasks: tasks, read: func(_ context.Context, task taskstore.Task) (taskstore.Task, error) {
			if task.Result["dry_run"] == true && task.Status == "succeeded" {
				first.Do(reads.Done)
				<-release // Simulate a late successful storage response after cancellation.
			}
			return task, nil
		}}
		service := accountautomation.New(accounts, management, nil, store, immediateRunner{}, accounts)
		service.UseScheduleRepository(&taskGroupSettings{})
		pool := taskrunner.New(t.Context())
		service.UseGroupRunner(pool)
		firstSchedule := saveTaskGroupSchedule(t, service, request, "第一项")
		secondSchedule := saveTaskGroupSchedule(t, service, request, "第二项")
		group, err := service.SaveTaskGroup(t.Context(), accountautomation.TaskGroup{Name: "取消组", ScheduleIDs: []string{firstSchedule.ID, secondSchedule.ID}})
		if err != nil {
			t.Fatal(err)
		}
		task, err := service.RunTaskGroup(t.Context(), group.ID)
		if err != nil {
			t.Fatal(err)
		}
		reads.Wait()
		pool.CancelTask(task.ID)
		close(release)
		synctest.Wait()
		service.StopSchedules()
		if err := pool.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
		if accounts.syncCalls != 0 || management.publishCalls != 0 || management.priceCalls != 0 {
			t.Fatalf("cancelled group started writes: sync=%d publish=%d price=%d", accounts.syncCalls, management.publishCalls, management.priceCalls)
		}
	})
}
