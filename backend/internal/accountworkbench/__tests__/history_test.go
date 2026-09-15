package accountworkbench_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestHistoryCancellationValidatesWholeScopeBeforeStoppingTasks(t *testing.T) {
	ctx := context.Background()
	tasks, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer tasks.Close()
	runner := taskrunner.New(ctx)
	defer runner.Shutdown(ctx)
	service := accountworkbench.New(nil, tasks, nil, nil, runner)
	started := make(chan context.Context, 1)
	finished := make(chan struct{})
	if err := runner.GoTask("selected", func(run context.Context) { started <- run; <-run.Done(); close(finished) }); err != nil {
		t.Fatal(err)
	}
	run := <-started
	defer runner.Cancel()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for id, skill := range map[string]string{"selected": accountworkbench.Skill, "unrelated": "another-module"} {
		if err := tasks.Save(ctx, taskstore.Task{ID: id, Skill: skill, Operation: "operation", Status: "running", Message: "处理中", CreatedAt: now, UpdatedAt: now, Result: map[string]any{}}); err != nil {
			t.Fatal(err)
		}
	}
	input := accountworkbench.HistoryActionInput{Confirmed: true, Items: []taskstore.HistorySelection{{ID: "selected", UpdatedAt: now}, {ID: "unrelated", UpdatedAt: now}}}
	if _, err := service.CancelHistory(ctx, input); err == nil {
		t.Fatal("cross-module selection accepted")
	}
	if run.Err() != nil {
		t.Fatal("invalid scope partially cancelled task")
	}
	input.Items = input.Items[:1]
	input.Confirmed = false
	if _, err := service.CancelHistory(ctx, input); err == nil {
		t.Fatal("unconfirmed selection accepted")
	}
	if run.Err() != nil {
		t.Fatal("unconfirmed task cancelled")
	}
	input.Confirmed = true
	result, err := service.CancelHistory(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 || !result[0].Cancelled {
		t.Fatalf("cancel result = %+v", result)
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("selected task did not stop")
	}
	other, err := tasks.Get(ctx, "unrelated")
	if err != nil || other.Status != "running" {
		t.Fatal("unrelated task changed")
	}
}

func TestHistoryDeletionRequiresExplicitConfirmation(t *testing.T) {
	ctx := context.Background()
	tasks, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer tasks.Close()
	service := accountworkbench.New(nil, tasks, nil, nil, nil)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := tasks.Save(ctx, taskstore.Task{ID: "completed", Skill: accountworkbench.Skill, Operation: "operation", Status: "succeeded", Message: "已完成", CreatedAt: now, UpdatedAt: now, Result: map[string]any{}}); err != nil {
		t.Fatal(err)
	}
	input := accountworkbench.HistoryActionInput{Items: []taskstore.HistorySelection{{ID: "completed", UpdatedAt: now}}}
	if err := service.DeleteHistory(ctx, input); err == nil {
		t.Fatal("unconfirmed deletion accepted")
	}
	if _, err := tasks.Get(ctx, "completed"); err != nil {
		t.Fatal("unconfirmed record removed")
	}
	input.Confirmed = true
	if err := service.DeleteHistory(ctx, input); err != nil {
		t.Fatal(err)
	}
	if _, err := tasks.Get(ctx, "completed"); err != taskstore.ErrNotFound {
		t.Fatal("confirmed record retained")
	}
}
