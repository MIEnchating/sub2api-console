package taskrunner

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFeedStopsAndClosesChannelAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	jobs := make(chan int)
	done := make(chan error, 1)
	go func() { done <- Feed(ctx, jobs, []int{1, 2, 3}) }()

	if first := <-jobs; first != 1 {
		t.Fatalf("first job=%d, want 1", first)
	}
	cancel()

	select {
	case _, open := <-jobs:
		if open {
			t.Fatal("feed dispatched another job after cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("feed did not close the job channel after cancellation")
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Feed error=%v, want context cancellation", err)
	}
}

func TestFeedDispatchesAllItemsAndClosesChannel(t *testing.T) {
	jobs := make(chan string)
	done := make(chan error, 1)
	go func() { done <- Feed(context.Background(), jobs, []string{"a", "b"}) }()

	var received []string
	for job := range jobs {
		received = append(received, job)
	}
	if len(received) != 2 || received[0] != "a" || received[1] != "b" {
		t.Fatalf("received=%#v", received)
	}
	if err := <-done; err != nil {
		t.Fatalf("Feed error=%v", err)
	}
}

func TestFeedIndicesStopsAtCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	jobs := make(chan int)
	done := make(chan error, 1)
	go func() { done <- FeedIndices(ctx, jobs, 3) }()

	if first := <-jobs; first != 0 {
		t.Fatalf("first index=%d, want 0", first)
	}
	cancel()
	if _, open := <-jobs; open {
		t.Fatal("index feed dispatched another job after cancellation")
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("FeedIndices error=%v, want context cancellation", err)
	}
}
