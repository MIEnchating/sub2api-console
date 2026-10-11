package routingwrite_test

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
)

func TestAmbiguousBatchReadbacksUseConfiguredConcurrency(t *testing.T) {
	t.Parallel()
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	var active, maximum atomic.Int32
	fixture := &batchAcknowledgementFixture{
		reply: map[string]any{},
		onReadback: func(request *http.Request) {
			current := active.Add(1)
			defer active.Add(-1)
			for observed := maximum.Load(); current > observed; observed = maximum.Load() {
				if maximum.CompareAndSwap(observed, current) {
					break
				}
			}
			started <- struct{}{}
			select {
			case <-release:
			case <-request.Context().Done():
			}
		},
	}
	service, targets := newBatchAcknowledgementService(t, fixture, 4, false)
	t.Cleanup(unblock)
	result := make(chan routingwrite.Result, 1)
	errors := make(chan error, 1)
	go func() {
		applied, err := service.Apply(t.Context(), targets, "test")
		result <- applied
		errors <- err
	}()
	for range 2 {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("configured readback workers did not start")
		}
	}
	unblock()
	applied := <-result
	if err := <-errors; err != nil || applied.Failed != 0 || applied.Changed != 4 || maximum.Load() != 2 {
		t.Fatalf("readbacks must overlap within concurrency=2: max=%d result=%+v err=%v", maximum.Load(), applied, err)
	}
}

func TestCancelledBatchReadbacksDoNotReportSuccessfulWrites(t *testing.T) {
	t.Parallel()
	started := make(chan struct{}, 4)
	fixture := &batchAcknowledgementFixture{
		reply: map[string]any{},
		onReadback: func(request *http.Request) {
			started <- struct{}{}
			<-request.Context().Done()
		},
	}
	service, targets := newBatchAcknowledgementService(t, fixture, 4, false)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan routingwrite.Result, 1)
	errors := make(chan error, 1)
	go func() {
		applied, err := service.Apply(ctx, targets, "test")
		result <- applied
		errors <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("readback did not start")
	}
	cancel()
	select {
	case applied := <-result:
		if err := <-errors; err != nil || applied.Failed != 4 || applied.Succeeded != 0 {
			t.Fatalf("cancelled confirmation must not succeed: %+v err=%v", applied, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled readbacks did not finish")
	}
}
