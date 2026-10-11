package accountautomation_test

import (
	"context"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountautomation"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type followupStore struct {
	*memoryTasks
	updates chan taskstore.Task
}

func (s *followupStore) Save(ctx context.Context, task taskstore.Task) error {
	if err := s.memoryTasks.Save(ctx, task); err != nil {
		return err
	}
	if task.Result["execution_task_id"] != nil || task.Result["execution_error"] != nil {
		s.updates <- task
	}
	return nil
}

type readonlyMode struct{}

func (readonlyMode) Mode(context.Context) (string, error) { return "readonly", nil }

func TestScheduledRunLinksPreviewToExecutionTask(t *testing.T) {
	_, accounts, management, tasks, request := automationFixture()
	store := &followupStore{memoryTasks: tasks, updates: make(chan taskstore.Task, 2)}
	service := accountautomation.New(accounts, management, nil, store, immediateRunner{}, accounts)
	service.UseScheduleRepository(&memorySchedule{})
	schedule, err := service.SaveSchedule(context.Background(), accountautomation.Schedule{Name: "上架", Request: request, IntervalMinutes: 60})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.RunSchedule(context.Background(), schedule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Result["schedule_id"] != schedule.ID {
		t.Fatalf("run response lacks schedule ownership: %v", preview.Result)
	}
	select {
	case linked := <-store.updates:
		id, ok := linked.Result["execution_task_id"].(string)
		if !ok || id == "" || linked.ID != preview.ID {
			t.Fatalf("preview not linked: %v", linked)
		}
		execution, err := store.Get(context.Background(), id)
		if err != nil || execution.Result["dry_run"] != false || execution.Status != "succeeded" {
			t.Fatalf("invalid execution: %v %v", execution, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("execution link was not saved")
	}
}

func TestScheduledRunPersistsExecutionStartupFailure(t *testing.T) {
	_, accounts, management, tasks, request := automationFixture()
	store := &followupStore{memoryTasks: tasks, updates: make(chan taskstore.Task, 2)}
	service := accountautomation.New(accounts, management, readonlyMode{}, store, immediateRunner{}, accounts)
	service.UseScheduleRepository(&memorySchedule{})
	schedule, err := service.SaveSchedule(context.Background(), accountautomation.Schedule{Name: "上架", Request: request, IntervalMinutes: 60})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.RunSchedule(context.Background(), schedule.ID)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case failed := <-store.updates:
		if failed.ID != preview.ID || failed.Result["execution_error"] != "账号模型写入需要完全模式" || failed.Result["execution_task_id"] != nil {
			t.Fatalf("startup failure was hidden: %v", failed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("execution startup failure was not saved")
	}
}
