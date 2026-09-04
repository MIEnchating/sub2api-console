package taskstore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type retrySaver struct {
	failures int
	calls    int
}

type capturedSaver struct {
	failures int
	tasks    []Task
}

func (s *capturedSaver) Save(_ context.Context, task Task) error {
	s.tasks = append(s.tasks, task)
	if len(s.tasks) <= s.failures {
		return errors.New("database busy")
	}
	return nil
}

func (s *retrySaver) Save(context.Context, Task) error {
	s.calls++
	if s.calls <= s.failures {
		return errors.New("database busy")
	}
	return nil
}

func TestSaveFinalRetriesTransientPersistenceFailures(t *testing.T) {
	saver := &retrySaver{failures: 2}
	if err := SaveFinal(context.Background(), saver, Task{ID: "task-1"}); err != nil {
		t.Fatal(err)
	}
	if saver.calls != 3 {
		t.Fatalf("calls=%d", saver.calls)
	}
}

func TestSaveFinalTruncatesOversizedArrayAndPersistsTerminalTask(t *testing.T) {
	saver := &capturedSaver{}
	items := make([]any, 5000)
	for index := range items {
		items[index] = map[string]any{"index": index, "detail": strings.Repeat("x", 2000)}
	}
	task := Task{ID: "task-large", Status: "succeeded", Result: map[string]any{"items": items}}
	if err := SaveFinal(context.Background(), saver, task); err != nil {
		t.Fatal(err)
	}
	if len(saver.tasks) != 1 || saver.tasks[0].Result["truncated"] != true {
		t.Fatalf("oversized terminal task was not truncated: %#v", saver.tasks)
	}
	total, totalOK := saver.tasks[0].Result["total"].(int)
	retained, retainedOK := saver.tasks[0].Result["retained"].(int)
	if !totalOK || !retainedOK || total != 5000 || retained >= total {
		t.Fatalf("truncation metadata is invalid: total=%#v retained=%#v", saver.tasks[0].Result["total"], saver.tasks[0].Result["retained"])
	}
	encoded, err := json.Marshal(saver.tasks[0].Result)
	if err != nil || len(encoded) > maximumTaskResultBytes {
		t.Fatalf("bounded result bytes=%d err=%v", len(encoded), err)
	}
}

func TestSaveFinalStopsAtThreeAttempts(t *testing.T) {
	saver := &retrySaver{failures: 10}
	err := SaveFinal(context.Background(), saver, Task{ID: "task-1"})
	if err == nil || saver.calls != 3 {
		t.Fatalf("calls=%d err=%v", saver.calls, err)
	}
}

func TestSaveFinalHonorsCallerCancellation(t *testing.T) {
	saver := &retrySaver{failures: 10}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	err := SaveFinal(ctx, saver, Task{ID: "task-1"})
	if err == nil || time.Since(started) > time.Second {
		t.Fatalf("elapsed=%s err=%v", time.Since(started), err)
	}
}

func TestSaveRunningPersistsFailureWhenInitialRunningWriteFails(t *testing.T) {
	saver := &capturedSaver{failures: 1}
	task := Task{ID: "task-1", Status: "running", Progress: 10, Message: "running", Result: map[string]any{}}
	if SaveRunning(context.Background(), saver, task) {
		t.Fatal("SaveRunning unexpectedly succeeded")
	}
	if len(saver.tasks) != 2 {
		t.Fatalf("writes=%d", len(saver.tasks))
	}
	final := saver.tasks[1]
	if final.Status != "failed" || final.Progress != 100 || final.Result["remote_write"] != false {
		t.Fatalf("final task = %#v", final)
	}
}

func TestSaveRunningPersistsContextFailureCause(t *testing.T) {
	saver := &capturedSaver{failures: 1}
	leaseErr := errors.New("mutation lease lost")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(leaseErr)
	task := Task{ID: "task-1", Status: "running", Progress: 10, Message: "running", Result: map[string]any{}}
	if SaveRunning(ctx, saver, task) {
		t.Fatal("SaveRunning unexpectedly succeeded")
	}
	final := saver.tasks[len(saver.tasks)-1]
	if final.Status != "failed" || final.Result["error"] != leaseErr.Error() || final.Result["cancelled"] == true {
		t.Fatalf("final task = %#v", final)
	}
}

func TestMarkCancelledOverridesFailureWithTerminalCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	task := Task{Status: "failed", Progress: 100, Message: "failed", Result: map[string]any{"error": "context canceled"}}
	if !MarkCancelled(ctx, &task, "任务已取消") {
		t.Fatal("expected cancellation")
	}
	if task.Status != "cancelled" || task.Message != "任务已取消" || task.Result["cancelled"] != true || task.Result["error"] != nil {
		t.Fatalf("cancelled task = %#v", task)
	}
}

func TestMarkCancelledDoesNotMaskContextFailureCause(t *testing.T) {
	leaseErr := errors.New("mutation lease lost")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(leaseErr)
	task := Task{Status: "failed", Progress: 100, Message: "failed", Result: map[string]any{"error": context.Canceled.Error(), "cancelled": true}}
	if MarkCancelled(ctx, &task, "任务已取消") {
		t.Fatal("lease loss was treated as an ordinary cancellation")
	}
	if task.Status != "failed" || task.Message != "任务执行失败："+leaseErr.Error() || task.Result["error"] != leaseErr.Error() ||
		task.Result["cancelled"] != nil || task.Result["operation_error"] != nil || !errors.Is(ContextFailureCause(ctx), leaseErr) {
		t.Fatalf("failed task = %#v cause=%v", task, ContextFailureCause(ctx))
	}
}

func TestMarkCancelledPreservesOperationErrorAlongsideContextFailure(t *testing.T) {
	leaseErr := errors.New("mutation lease lost")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(leaseErr)
	task := Task{Status: "failed", Message: "remote write failed", Result: map[string]any{"error": "upstream disconnected"}}

	if MarkCancelled(ctx, &task, "任务已取消") {
		t.Fatal("lease loss was treated as an ordinary cancellation")
	}
	if task.Status != "failed" || task.Result["error"] != leaseErr.Error() || task.Result["operation_error"] != "upstream disconnected" {
		t.Fatalf("failed task = %#v", task)
	}
}

func TestMarkCancelledDoesNotPreserveAnEmptyOperationError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	task := Task{Status: "failed", Result: map[string]any{"error": ""}}

	MarkCancelled(ctx, &task, "任务已取消")
	if task.Status != "failed" || task.Result["error"] != context.DeadlineExceeded.Error() || task.Result["operation_error"] != nil {
		t.Fatalf("failed task = %#v", task)
	}
}

func TestMarkCancelledPreservesWorkThatAlreadySucceeded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	task := Task{Status: "succeeded", Progress: 100, Message: "complete", Result: map[string]any{"value": true}}
	if MarkCancelled(ctx, &task, "任务已取消") {
		t.Fatal("late cancellation overwrote completed work")
	}
	if task.Status != "succeeded" || task.Result["value"] != true {
		t.Fatalf("completed task = %#v", task)
	}
}

func TestMarkCancelledPreservesSucceededTaskAfterContextFailure(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(errors.New("late lease failure"))
	task := Task{Status: "succeeded", Progress: 100, Message: "complete", Result: map[string]any{"value": true}}
	if MarkCancelled(ctx, &task, "任务已取消") {
		t.Fatal("late context failure changed the cancellation result")
	}
	if task.Status != "succeeded" || task.Message != "complete" || task.Result["value"] != true || task.Result["error"] != nil {
		t.Fatalf("completed task = %#v", task)
	}
}
