package evidence

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
)

func TestLiveSubscribersShareAccountFetchAndStopWhenLastViewerLeaves(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := taskrunner.New(ctx)
	defer runner.Shutdown(context.Background())
	started := make(chan string, 4)
	stopped := make(chan struct{}, 4)
	var calls atomic.Int32
	live := NewLive(func(ctx context.Context, id string) error {
		calls.Add(1)
		started <- id
		<-ctx.Done()
		stopped <- struct{}{}
		return ctx.Err()
	}, runner)
	_, stopFirst, err := live.Subscribe([]string{"41"})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("fetch did not start")
	}
	_, stopSecond, err := live.Subscribe([]string{"41"})
	if err != nil {
		t.Fatal(err)
	}
	stopFirst()
	if calls.Load() != 1 {
		t.Fatal("duplicate account fetch")
	}
	stopSecond()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("last viewer did not cancel fetch")
	}
	stopSecond()
}

func TestLivePublishesFastAccountWithoutWaitingForSlowAccount(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := taskrunner.New(ctx)
	defer runner.Shutdown(context.Background())
	live := NewLive(func(ctx context.Context, id string) error {
		if id == "42" {
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}, runner)
	updates, stop, err := live.Subscribe([]string{"41", "42"})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	select {
	case update := <-updates:
		if update.AccountID != "41" || update.Failed {
			t.Fatalf("unexpected update: %#v", update)
		}
	case <-time.After(time.Second):
		t.Fatal("fast account blocked on slow account")
	}
}
