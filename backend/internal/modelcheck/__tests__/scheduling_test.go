package modelcheck_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
)

func TestSameAccountRejectsDuplicateWhileRequestIsRunningAndCancellationFinishes(t *testing.T) {
	started := make(chan struct{}, 1)
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		started <- struct{}{}
		<-r.Context().Done()
	})
	queued, err := f.service.EnqueueAnimation(context.Background(), request("1"))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not start")
	}
	if _, err := f.service.EnqueueAnimation(context.Background(), request("1")); err == nil {
		t.Fatal("duplicate accepted")
	}
	if !f.runner.CancelTask(queued.ID) {
		t.Fatal("cannot cancel animation task")
	}
	task := finished(t, f)
	if task.Status != "cancelled" {
		t.Fatalf("status=%s", task.Status)
	}
}

func TestBatchLimitsConcurrentGenerationToThree(t *testing.T) {
	var active, maximum atomic.Int32
	started := make(chan struct{}, 6)
	release := make(chan struct{})
	f := setup(t, 6, "openai", func(w http.ResponseWriter, r *http.Request) {
		current := active.Add(1)
		defer active.Add(-1)
		for old := maximum.Load(); current > old; old = maximum.Load() {
			if maximum.CompareAndSwap(old, current) {
				break
			}
		}
		started <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = fmt.Fprintf(w, `{"output_text":%q}`, fixtureSVG)
	})
	if _, err := f.service.EnqueueAnimation(context.Background(), request("1", "2", "3", "4", "5", "6")); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("three requests did not start")
		}
	}
	close(release)
	task := finished(t, f)
	if task.Status != "succeeded" || maximum.Load() != 3 {
		t.Fatalf("status=%s maximum=%d", task.Status, maximum.Load())
	}
}

type deferredRunner struct{ runs []func(context.Context) }

func (r *deferredRunner) Go(run func(context.Context)) error {
	r.runs = append(r.runs, run)
	return nil
}
func (r *deferredRunner) GoTask(_ string, run func(context.Context)) error { return r.Go(run) }
func (r *deferredRunner) CancelTask(string) bool                           { return false }

func TestAutomaticDetectionPersistsAndWaitsForIntervalAfterCompletion(t *testing.T) {
	var calls atomic.Int32
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = fmt.Fprintf(w, `{"output_text":%q}`, fixtureSVG)
	})
	runner := &deferredRunner{}
	f.service.UseTaskRunner(runner)
	if len(f.service.AnimationSchedules()) != 0 {
		t.Fatal("automatic detection must default off")
	}
	f.service.RunDueAnimations(context.Background(), time.Now().Add(24*time.Hour))
	if len(runner.runs) != 0 {
		t.Fatal("default schedules made requests")
	}
	value := modelcheck.AnimationSchedule{AccountID: "1", Enabled: true, Model: "test-model", IntervalMinutes: 10, TimeoutSeconds: 5}
	views, err := f.service.SaveAnimationSchedule(context.Background(), value, "test-operator")
	if err != nil {
		t.Fatal(err)
	}
	next, err := time.Parse(time.RFC3339Nano, views[0].NextAt)
	if err != nil {
		t.Fatal(err)
	}
	f.service.RunDueAnimations(context.Background(), next.Add(-time.Second))
	if len(runner.runs) != 0 {
		t.Fatal("first request started before interval")
	}
	f.service.RunDueAnimations(context.Background(), next)
	if len(runner.runs) != 1 {
		t.Fatal("due schedule not enqueued")
	}
	f.service.RunDueAnimations(context.Background(), next.Add(11*time.Minute))
	if len(runner.runs) != 1 {
		t.Fatal("same account overlapped")
	}
	runner.runs[0](context.Background())
	_ = finished(t, f)
	views = f.service.AnimationSchedules()
	finishedNext, _ := time.Parse(time.RFC3339Nano, views[0].NextAt)
	f.service.RunDueAnimations(context.Background(), finishedNext.Add(-time.Second))
	if len(runner.runs) != 1 || calls.Load() != 1 {
		t.Fatal("completion must reset interval")
	}
	restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	restored := restarted.AnimationSchedules()
	if len(restored) != 1 || !restored[0].Enabled || restored[0].Model != "test-model" || restored[0].Version != 1 {
		t.Fatalf("restored=%#v", restored)
	}
	stop := views[0].AnimationSchedule
	stop.Enabled = false
	if _, err := f.service.SaveAnimationSchedule(context.Background(), stop, "test-operator"); err != nil {
		t.Fatal(err)
	}
	f.service.RunDueAnimations(context.Background(), time.Now().Add(24*time.Hour))
	if len(runner.runs) != 1 {
		t.Fatal("disabled schedule enqueued new task")
	}
	if _, err := f.service.SaveAnimationSchedule(context.Background(), stop, "test-operator"); err == nil {
		t.Fatal("stale version accepted")
	}
}

func TestFailedScheduleSaveDoesNotEnableDetection(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("unexpected request") })
	f.catalog.failSave = true
	_, err := f.service.SaveAnimationSchedule(context.Background(), modelcheck.AnimationSchedule{AccountID: "1", Enabled: true, Model: "test-model", IntervalMinutes: 1, TimeoutSeconds: 5}, "test")
	if err == nil || len(f.service.AnimationSchedules()) != 0 {
		t.Fatal("failed persistence changed schedule")
	}
}

func TestInvalidScheduleIntervalsAndMissingAccountAreRejected(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("unexpected request") })
	for _, interval := range []int{0, 1441} {
		t.Run(strconv.Itoa(interval), func(t *testing.T) {
			_, err := f.service.SaveAnimationSchedule(context.Background(), modelcheck.AnimationSchedule{AccountID: "1", Enabled: true, Model: "test-model", IntervalMinutes: interval, TimeoutSeconds: 5}, "test")
			if err == nil {
				t.Fatal("invalid interval accepted")
			}
		})
	}
	_, err := f.service.SaveAnimationSchedule(context.Background(), modelcheck.AnimationSchedule{AccountID: "99", Enabled: true, Model: "test-model", IntervalMinutes: 1, TimeoutSeconds: 5}, "test")
	if err == nil {
		t.Fatal("missing account schedule accepted")
	}
}
