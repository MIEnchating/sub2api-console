package newapimanagement_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskcontext"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type interruptChannelAfterResult struct {
	store  *taskstore.Store
	cancel context.CancelCauseFunc
}

func (s *interruptChannelAfterResult) Save(ctx context.Context, task taskstore.Task) error {
	if err := s.store.Save(ctx, task); err != nil {
		return err
	}
	if task.Status == "running" && task.Result["completed"] == 1 {
		s.cancel(taskcontext.ErrInterrupted)
	}
	return nil
}

func TestChannelBatchServiceStopKeepsCompletedResultsWithoutClaimingManualCancellation(t *testing.T) {
	for _, completed := range []int{0, 1} {
		name := "before execution"
		if completed == 1 {
			name = "after confirmed first channel"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newChannelFixture(t)
			channel := fixture.channel(t)
			store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			runner := &queuedChannelRunner{}
			saver := &interruptChannelAfterResult{store: store, cancel: cancel}
			tasks := newapimanagement.NewChannelTasks(fixture.service, saver, runner)
			task, err := tasks.Enqueue(context.Background(), "primary", newapimanagement.ChannelBatchInput{
				Action: "add", Models: []string{"gpt-5-nano"},
				Channels: []newapimanagement.ChannelTarget{{ID: channel.ID, Version: channel.Version}, {ID: "43", Version: channel.Version}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if completed == 0 {
				cancel(taskcontext.ErrInterrupted)
			}
			runner.run(ctx)
			result, err := store.Get(context.Background(), task.ID)
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != "failed" || result.Result["interrupted"] != true || result.Recovery != nil || result.Progress != completed*50 {
				t.Fatalf("service interruption lost state: %+v", result)
			}
			if !strings.Contains(result.Message, "核对") || strings.Contains(result.Message, "已取消") {
				t.Fatalf("unsafe interruption message: %s", result.Message)
			}
			items := result.Result["items"].([]any)
			if len(items) != 2 {
				t.Fatalf("missing interruption details: %v", items)
			}
			for i, item := range items {
				want := "interrupted"
				if i < completed {
					want = "succeeded"
				}
				if item.(map[string]any)["status"] != want {
					t.Fatalf("item %d=%v; want %s", i, item, want)
				}
			}
			fixture.mu.Lock()
			defer fixture.mu.Unlock()
			if len(fixture.writes) != completed {
				t.Fatalf("unexpected upstream writes: %v", fixture.writes)
			}
		})
	}
}
