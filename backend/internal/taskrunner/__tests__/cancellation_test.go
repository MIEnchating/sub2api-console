package taskrunner_test

import (
	"context"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
)

func TestCompositeCancellerCancelsTasksInEitherOwningGroup(t *testing.T) {
	for _, owner := range []int{0, 1, 2} {
		t.Run(string(rune('A'+owner)), func(t *testing.T) {
			groups := []*taskrunner.Group{taskrunner.New(context.Background()), taskrunner.New(context.Background()), taskrunner.NewBounded(context.Background(), 4)}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				for _, group := range groups {
					if err := group.Shutdown(ctx); err != nil {
						t.Error(err)
					}
				}
			})
			done := make(chan struct{})
			if err := groups[owner].GoTask("workbench-operation", func(ctx context.Context) { <-ctx.Done(); close(done) }); err != nil {
				t.Fatal(err)
			}
			canceller := taskrunner.CompositeCanceller{Groups: []taskrunner.TaskRunner{groups[0], nil, groups[1], groups[2]}}
			if !canceller.CancelTask("workbench-operation") {
				t.Fatal("active task was not found in its owning group")
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("task did not observe cancellation")
			}
			if canceller.CancelTask("missing-operation") {
				t.Fatal("missing task reported cancelled")
			}
		})
	}
}
