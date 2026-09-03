package taskrunner

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGoWithoutRunnerRejectsNilTask(t *testing.T) {
	if err := Go(nil, nil); !errors.Is(err, ErrNilTask) {
		t.Fatalf("Go(nil, nil) error = %v, want ErrNilTask", err)
	}
}

func TestGroupGoRejectsNilTask(t *testing.T) {
	runner := New(context.Background())
	defer runner.Cancel()
	if err := runner.Go(nil); !errors.Is(err, ErrNilTask) {
		t.Fatalf("Group.Go(nil) error = %v, want ErrNilTask", err)
	}
	if err := Go(runner, nil); !errors.Is(err, ErrNilTask) {
		t.Fatalf("Go(runner, nil) error = %v, want ErrNilTask", err)
	}
}

func TestBoundedGroupRejectsTasksBeyondActiveLimit(t *testing.T) {
	runner := NewBounded(context.Background(), 2)
	t.Cleanup(runner.Cancel)
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	for range 2 {
		if err := runner.Go(func(context.Context) {
			started <- struct{}{}
			<-release
		}); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("bounded runner did not start accepted task")
		}
	}
	if err := runner.Go(func(context.Context) {}); !errors.Is(err, ErrCapacity) {
		t.Fatalf("task beyond capacity error = %v, want ErrCapacity", err)
	}
	close(release)
}

func TestShutdownCancelsAndWaitsForRunningTasks(t *testing.T) {
	runner := New(context.Background())
	started := make(chan struct{})
	finished := make(chan struct{})
	if err := runner.Go(func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(finished)
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := runner.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	default:
		t.Fatal("shutdown returned before the running task finished")
	}
	if err := runner.Go(func(context.Context) {}); !errors.Is(err, ErrStopped) {
		t.Fatalf("Go after shutdown error = %v", err)
	}
}

func TestShutdownAndGoDoNotRaceWaitGroupAdd(t *testing.T) {
	for iteration := 0; iteration < 100; iteration++ {
		runner := New(context.Background())
		var accepted atomic.Int64
		var completed atomic.Int64
		start := make(chan struct{})
		var callers sync.WaitGroup
		for index := 0; index < 64; index++ {
			callers.Add(1)
			go func() {
				defer callers.Done()
				<-start
				if runner.Go(func(context.Context) { completed.Add(1) }) == nil {
					accepted.Add(1)
				}
			}()
		}
		close(start)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		if err := runner.Shutdown(ctx); err != nil {
			cancel()
			t.Fatal(err)
		}
		cancel()
		callers.Wait()
		if completed.Load() != accepted.Load() {
			t.Fatalf("iteration %d: accepted=%d completed=%d", iteration, accepted.Load(), completed.Load())
		}
	}
}

func TestShutdownHonorsWaitContext(t *testing.T) {
	runner := New(context.Background())
	release := make(chan struct{})
	if err := runner.Go(func(context.Context) { <-release }); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if err := runner.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown error = %v", err)
	}
	close(release)
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := runner.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestRepeatedTimedOutShutdownsStillAllowFinalWait(t *testing.T) {
	runner := New(context.Background())
	release := make(chan struct{})
	if err := runner.Go(func(context.Context) { <-release }); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := runner.Shutdown(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("Shutdown error = %v, want context cancellation", err)
		}
	}
	close(release)
	if err := runner.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestShutdownPrefersCompletedDrainOverCancelledWaitContext(t *testing.T) {
	runner := New(context.Background())
	runner.Cancel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for iteration := 0; iteration < 100; iteration++ {
		if err := runner.Shutdown(ctx); err != nil {
			t.Fatalf("iteration %d: drained shutdown returned %v", iteration, err)
		}
	}
}
