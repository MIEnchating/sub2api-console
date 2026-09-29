package modelcheck_test

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestDetectionTaskRespectsConfiguredAccountConcurrency(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configured int
		concurrent int
	}{
		{name: "legacy default runs four", concurrent: 4},
		{name: "configured limit runs two", configured: 2, concurrent: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			started := make(chan struct{}, 4)
			release := make(chan struct{})
			var releaseOnce sync.Once
			var active, peak atomic.Int32
			f := setup(t, 4, "openai", func(w http.ResponseWriter, r *http.Request) {
				current := active.Add(1)
				for {
					old := peak.Load()
					if current <= old || peak.CompareAndSwap(old, current) {
						break
					}
				}
				defer active.Add(-1)
				started <- struct{}{}
				select {
				case <-release:
					fmt.Fprint(w, `{"output_text":"21"}`)
				case <-r.Context().Done():
				}
			})
			f.catalog.groupMembers = map[string][]string{"7": {"1", "2", "3", "4"}, "8": {}}
			value := managedTask()
			value.Animation = new(bool)
			value.Terminal = false
			value.Concurrency = tc.configured
			plans, err := f.service.SaveDetectionTask(context.Background(), value, "test")
			if err != nil {
				t.Fatal(err)
			}
			if plans[0].Concurrency != tc.concurrent {
				t.Fatalf("stored concurrency = %d, want %d", plans[0].Concurrency, tc.concurrent)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			runner := &deferredRunner{}
			f.service.UseTaskRunner(runner)
			if _, err := f.service.RunDetectionTask(context.Background(), plans[0].ID, plans[0].Version); err != nil {
				t.Fatal(err)
			}
			finishedRun := make(chan struct{})
			go func() { runner.runs[0](ctx); close(finishedRun) }()
			defer func() { cancel(); releaseOnce.Do(func() { close(release) }); <-finishedRun }()
			deadline := time.NewTimer(3 * time.Second)
			defer deadline.Stop()
			for range tc.concurrent {
				select {
				case <-started:
				case <-deadline.C:
					t.Fatalf("only %d accounts reached upstream concurrently", peak.Load())
				}
			}
			if got := peak.Load(); got != int32(tc.concurrent) {
				t.Fatalf("peak concurrent requests = %d, want %d", got, tc.concurrent)
			}
			releaseOnce.Do(func() { close(release) })
			<-finishedRun
			if got := peak.Load(); got != int32(tc.concurrent) {
				t.Fatalf("peak concurrent requests after completion = %d, want %d", got, tc.concurrent)
			}
			task := finished(t, f)
			if task.Status != "succeeded" || task.Result["completed"] != 4 || len(task.Result["animations"].([]modelcheck.AnimationResult)) != 4 {
				t.Fatalf("concurrent completion lost results: %#v", task)
			}
		})
	}
}

func TestDetectionTaskRejectsConcurrencyOutsideRange(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("unexpected upstream request") })
	f.catalog.groupMembers = map[string][]string{"7": {"1"}, "8": {}}
	for _, concurrency := range []int{-1, 17} {
		value := managedTask()
		value.Concurrency = concurrency
		if _, err := f.service.SaveDetectionTask(context.Background(), value, "test"); err == nil {
			t.Fatalf("accepted concurrency %d", concurrency)
		}
	}
}
