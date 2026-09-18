package taskrunner_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
)

func TestRaiseLimitStartsQueuedTaskAndLowerLimitDrainsExistingTasks(t *testing.T) {
	g := taskrunner.NewQueued(context.Background(), 1, 3)
	defer g.Cancel()
	first := make(chan struct{})
	second := make(chan struct{})
	started := make(chan struct{}, 3)
	if err := g.Go(func(context.Context) { started <- struct{}{}; <-first }); err != nil {
		t.Fatal(err)
	}
	<-started
	if err := g.Go(func(context.Context) { started <- struct{}{}; <-second }); err != nil {
		t.Fatal(err)
	}
	g.Configure(2, 3)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("raising capacity did not dispatch queue")
	}
	g.Configure(1, 3)
	if err := g.Go(func(context.Context) { started <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	if s := g.Snapshot(); s.Running != 2 || s.Waiting != 1 || s.Limit != 1 {
		t.Fatalf("snapshot = %+v", s)
	}
	close(first)
	close(second)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("queue did not drain")
	}
}

func TestCancelQueuedTaskRunsCleanupWithCancelledContextAndReleasesQueue(t *testing.T) {
	g := taskrunner.NewQueued(context.Background(), 1, 1)
	defer g.Cancel()
	if err := g.Go(func(ctx context.Context) { <-ctx.Done() }); err != nil {
		t.Fatal(err)
	}
	cancelled := make(chan error, 1)
	if err := g.GoTask("waiting", func(ctx context.Context) { cancelled <- ctx.Err() }); err != nil {
		t.Fatal(err)
	}
	if !g.CancelTask("waiting") {
		t.Fatal("queued task not found")
	}
	select {
	case err := <-cancelled:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cleanup not called")
	}
	if err := g.Go(func(ctx context.Context) { <-ctx.Done() }); err != nil {
		t.Fatalf("queue capacity not released: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := g.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if s := g.Snapshot(); s.Running != 0 || s.Waiting != 0 {
		t.Fatalf("shutdown = %+v", s)
	}
}

func TestSeparateGroupsDoNotShareCapacity(t *testing.T) {
	a, b := taskrunner.NewQueued(context.Background(), 1, 0), taskrunner.NewQueued(context.Background(), 1, 0)
	defer a.Cancel()
	defer b.Cancel()
	if err := a.Go(func(ctx context.Context) { <-ctx.Done() }); err != nil {
		t.Fatal(err)
	}
	if err := a.Go(func(context.Context) {}); !errors.Is(err, taskrunner.ErrCapacity) {
		t.Fatal(err)
	}
	if err := b.Go(func(ctx context.Context) { <-ctx.Done() }); err != nil {
		t.Fatal(err)
	}
}
