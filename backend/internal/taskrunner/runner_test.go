package taskrunner

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskcontext"
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

func TestCancelTaskOnlyCancelsSelectedTask(t *testing.T) {
	runner := New(context.Background())
	t.Cleanup(runner.Cancel)
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	if err := runner.GoTask("first", func(ctx context.Context) { <-ctx.Done(); firstDone <- ctx.Err() }); err != nil {
		t.Fatal(err)
	}
	if err := runner.GoTask("second", func(ctx context.Context) { <-ctx.Done(); secondDone <- ctx.Err() }); err != nil {
		t.Fatal(err)
	}
	if !runner.CancelTask("first") {
		t.Fatal("registered task was not cancelled")
	}
	select {
	case err := <-firstDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled task error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled task did not stop")
	}
	select {
	case <-secondDone:
		t.Fatal("cancelling first task stopped second task")
	default:
	}
	if runner.CancelTask("missing") {
		t.Fatal("missing task reported as cancelled")
	}
}

func TestGoTaskCarriesTaskIDInContext(t *testing.T) {
	runner := New(context.Background())
	t.Cleanup(runner.Cancel)
	ready := make(chan string, 1)
	if err := runner.GoTask("task-context", func(ctx context.Context) {
		ready <- taskcontext.ID(ctx)
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-ready:
		if id != "task-context" {
			t.Fatalf("task context id=%q", id)
		}
	case <-time.After(time.Second):
		t.Fatal("task did not run")
	}
}

func TestGoTaskRejectsEmptyAndDuplicateIDs(t *testing.T) {
	runner := New(context.Background())
	t.Cleanup(runner.Cancel)
	if err := runner.GoTask(" ", func(context.Context) {}); !errors.Is(err, ErrTaskID) {
		t.Fatalf("empty task ID error=%v", err)
	}
	release := make(chan struct{})
	if err := runner.GoTask("same", func(context.Context) { <-release }); err != nil {
		t.Fatal(err)
	}
	if err := runner.GoTask("same", func(context.Context) {}); !errors.Is(err, ErrDuplicateTask) {
		t.Fatalf("duplicate task ID error=%v", err)
	}
	close(release)
}

type goOnlyRunner struct{}

func (goOnlyRunner) Go(func(context.Context)) error { return nil }

func TestGoTaskRejectsRunnerWithoutCancellationSupport(t *testing.T) {
	err := GoTask(goOnlyRunner{}, "task-1", func(context.Context) {})
	if !errors.Is(err, ErrTaskCancellationUnsupported) {
		t.Fatalf("GoTask error=%v, want ErrTaskCancellationUnsupported", err)
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
