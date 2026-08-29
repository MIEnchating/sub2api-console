package notificationtarget

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type listenerFunc func(context.Context, Request, func()) (Target, error)

func (function listenerFunc) Listen(ctx context.Context, request Request, ready func()) (Target, error) {
	return function(ctx, request, ready)
}

type memoryTasks struct {
	mu      sync.Mutex
	updates chan taskstore.Task
	latest  taskstore.Task
}

type cancelRaceTasks struct {
	mu          sync.Mutex
	saves       int
	secondSave  chan struct{}
	releaseSave chan struct{}
	updates     chan taskstore.Task
}

func (tasks *cancelRaceTasks) Save(ctx context.Context, task taskstore.Task) error {
	tasks.mu.Lock()
	tasks.saves++
	call := tasks.saves
	tasks.mu.Unlock()
	if call == 2 {
		close(tasks.secondSave)
		<-tasks.releaseSave
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	tasks.updates <- task
	return nil
}

func (tasks *memoryTasks) Save(_ context.Context, task taskstore.Task) error {
	tasks.mu.Lock()
	tasks.latest = task
	tasks.mu.Unlock()
	select {
	case tasks.updates <- task:
	default:
	}
	return nil
}

func TestDiscoveryTaskWaitsForInputThenPersistsOnlyTheCapturedTarget(t *testing.T) {
	tasks := &memoryTasks{updates: make(chan taskstore.Task, 8)}
	service := newService(listenerFunc(func(_ context.Context, request Request, ready func()) (Target, error) {
		if request.AppID != "app" || request.ClientSecret != "secret" || request.TargetType != "c2c" {
			t.Errorf("request = %#v", request)
		}
		ready()
		return Target{ID: "user-1", Type: "c2c", EventType: "C2C_MESSAGE_CREATE", CapturedAt: "2026-08-29T10:00:00Z"}, nil
	}), tasks, time.Second)
	task, err := service.Enqueue(context.Background(), Request{AppID: " app ", ClientSecret: " secret ", TargetType: "c2c"})
	if err != nil {
		t.Fatal(err)
	}
	waitForTask(t, tasks.updates, "waiting_input")
	finished := waitForTask(t, tasks.updates, "succeeded")
	if finished.ID != task.ID || finished.Result["target_id"] != "user-1" || finished.Result["event_type"] != "C2C_MESSAGE_CREATE" {
		t.Fatalf("finished task = %#v", finished)
	}
	if resultContains(finished.Result, "app") || resultContains(finished.Result, "secret") {
		t.Fatalf("credentials leaked into task result: %#v", finished.Result)
	}
}

func TestDiscoveryRejectsConcurrentSessionsAndSupportsCancellation(t *testing.T) {
	tasks := &memoryTasks{updates: make(chan taskstore.Task, 8)}
	service := newService(listenerFunc(func(ctx context.Context, _ Request, ready func()) (Target, error) {
		ready()
		<-ctx.Done()
		return Target{}, ctx.Err()
	}), tasks, time.Second)
	first, err := service.Enqueue(context.Background(), Request{AppID: "app", ClientSecret: "secret", TargetType: "group"})
	if err != nil {
		t.Fatal(err)
	}
	waitForTask(t, tasks.updates, "waiting_input")
	if _, err := service.Enqueue(context.Background(), Request{AppID: "app", ClientSecret: "secret", TargetType: "group"}); !errors.Is(err, ErrDiscoveryActive) {
		t.Fatalf("concurrent error = %v", err)
	}
	if !service.Cancel(first.ID) {
		t.Fatal("active task was not cancelled")
	}
	waitForTask(t, tasks.updates, "cancelled")
	if service.Cancel(first.ID) {
		t.Fatal("completed task remained cancellable")
	}
}

func TestDiscoveryPersistsCancellationDuringTheFirstProgressUpdate(t *testing.T) {
	tasks := &cancelRaceTasks{
		secondSave: make(chan struct{}), releaseSave: make(chan struct{}), updates: make(chan taskstore.Task, 8),
	}
	service := newService(listenerFunc(func(ctx context.Context, _ Request, _ func()) (Target, error) {
		<-ctx.Done()
		return Target{}, ctx.Err()
	}), tasks, time.Second)
	task, err := service.Enqueue(context.Background(), Request{AppID: "app", ClientSecret: "secret", TargetType: "c2c"})
	if err != nil {
		t.Fatal(err)
	}
	<-tasks.secondSave
	if !service.Cancel(task.ID) {
		t.Fatal("task was not cancelled during its first progress update")
	}
	close(tasks.releaseSave)
	waitForTask(t, tasks.updates, "cancelled")
}

func waitForTask(t *testing.T, updates <-chan taskstore.Task, status string) taskstore.Task {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case task := <-updates:
			if task.Status == status {
				return task
			}
		case <-deadline:
			t.Fatalf("timed out waiting for task status %q", status)
		}
	}
}

func resultContains(result map[string]any, value string) bool {
	for _, item := range result {
		if text, ok := item.(string); ok && text == value {
			return true
		}
	}
	return false
}
