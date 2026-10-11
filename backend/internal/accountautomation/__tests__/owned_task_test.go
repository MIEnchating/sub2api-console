package accountautomation_test

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountautomation"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestAcceptedScheduleContinuesAfterRequestCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, accounts, management, tasks, request := automationFixture()
		store := &followupStore{memoryTasks: tasks, updates: make(chan taskstore.Task, 1)}
		pool := &recoveryRunner{}
		service := accountautomation.New(accounts, management, nil, store, pool, accounts)
		service.UseScheduleRepository(&memorySchedule{})
		saved := saveTaskGroupSchedule(t, service, request, "继续后台执行")
		ctx, cancel := context.WithCancel(t.Context())
		if _, err := service.RunSchedule(ctx, saved.ID); err != nil {
			t.Fatal(err)
		}
		cancel()
		pool.run(t.Context())
		linked := <-store.updates
		pool.run(t.Context())
		synctest.Wait()
		service.StopSchedules()
		id, _ := linked.Result["execution_task_id"].(string)
		completed, err := tasks.Get(t.Context(), id)
		if err != nil || completed.Status != "succeeded" || accounts.syncCalls != 1 {
			t.Fatalf("accepted task cancelled with request: %#v err=%v", completed, err)
		}
	})
}

func TestTaskGroupCancellationWhileExecutionIsQueuedPreventsWrites(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, accounts, management, tasks, request := automationFixture()
		store := &followupStore{memoryTasks: tasks, updates: make(chan taskstore.Task, 1)}
		pool := &recoveryRunner{}
		groups := taskrunner.New(t.Context())
		service := accountautomation.New(accounts, management, nil, store, pool, accounts)
		service.UseScheduleRepository(&taskGroupSettings{})
		service.UseGroupRunner(groups)
		first := saveTaskGroupSchedule(t, service, request, "第一项")
		second := saveTaskGroupSchedule(t, service, request, "第二项")
		group, err := service.SaveTaskGroup(t.Context(), accountautomation.TaskGroup{Name: "排队取消", ScheduleIDs: []string{first.ID, second.ID}})
		if err != nil {
			t.Fatal(err)
		}
		created, err := service.RunTaskGroup(t.Context(), group.ID)
		if err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		pool.run(t.Context())
		linked := <-store.updates
		groups.CancelTask(created.ID)
		synctest.Wait()
		pool.run(t.Context())
		service.StopSchedules()
		if err := groups.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
		id, _ := linked.Result["execution_task_id"].(string)
		completed, err := tasks.Get(t.Context(), id)
		if err != nil || completed.Status != "cancelled" || accounts.syncCalls != 0 || management.publishCalls != 0 || management.priceCalls != 0 {
			t.Fatalf("cancelled queued execution wrote: status=%s sync=%d publish=%d prices=%d err=%v", completed.Status, accounts.syncCalls, management.publishCalls, management.priceCalls, err)
		}
	})
}

func TestScheduleShutdownKeepsQueuedPreviewRecoverable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, accounts, management, tasks, request := automationFixture()
		pool := &recoveryRunner{}
		service := accountautomation.New(accounts, management, nil, tasks, pool, accounts)
		service.UseScheduleRepository(&memorySchedule{})
		saved := saveTaskGroupSchedule(t, service, request, "停止后恢复预览")
		preview, err := service.RunSchedule(t.Context(), saved.ID)
		if err != nil {
			t.Fatal(err)
		}
		service.StopSchedules()
		pool.run(t.Context())
		got, err := tasks.Get(t.Context(), preview.ID)
		if err != nil || got.Status != "queued" || got.Recovery == nil || got.Result["interrupted"] != true || accounts.previewCalls != 0 {
			t.Fatalf("shutdown became manual cancellation: %#v err=%v", got, err)
		}
	})
}

func TestScheduleStopDeadlineAllowsLaterDrainWithoutNewWork(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, accounts, management, tasks, request := automationFixture()
		release := make(chan struct{})
		store := &lifecycleStore{memoryTasks: tasks, read: func(_ context.Context, task taskstore.Task) (taskstore.Task, error) { <-release; return task, nil }}
		service := accountautomation.New(accounts, management, nil, store, immediateRunner{}, accounts)
		service.UseScheduleRepository(&memorySchedule{})
		saved := saveTaskGroupSchedule(t, service, request, "等待迟到读取")
		if _, err := service.RunSchedule(t.Context(), saved.ID); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		if err := service.StopSchedulesContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("stop deadline: %v", err)
		}
		if _, err := service.RunSchedule(t.Context(), saved.ID); err == nil {
			t.Error("stopping service accepted another schedule")
		}
		close(release)
		if err := service.StopSchedulesContext(t.Context()); err != nil {
			t.Fatal(err)
		}
		if accounts.syncCalls != 0 {
			t.Error("late response started a write after shutdown")
		}
	})
}
