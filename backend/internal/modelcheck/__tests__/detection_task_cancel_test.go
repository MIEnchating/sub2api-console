package modelcheck_test

import (
	"context"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestDetectionTaskCancellationDuringPrecheckStopsAnimation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var requests atomic.Int32
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) { requests.Add(1); cancel() })
	f.catalog.groupMembers = map[string][]string{"7": {"1"}, "8": {"1"}}
	value := managedTask()
	plans, err := f.service.SaveDetectionTask(context.Background(), value, "test")
	if err != nil {
		t.Fatal(err)
	}
	runner := &deferredRunner{}
	f.service.UseTaskRunner(runner)
	if _, err := f.service.RunDetectionTask(context.Background(), plans[0].ID, plans[0].Version); err != nil {
		t.Fatal(err)
	}
	runner.runs[0](ctx)
	task := finished(t, f)
	if task.Status != "cancelled" || requests.Load() != 1 {
		t.Fatalf("cancellation continued: %s %d", task.Status, requests.Load())
	}
	results := task.Result["animations"].([]modelcheck.AnimationResult)
	if len(results) != 1 || results[0].Mode != "precheck" {
		t.Fatalf("cancelled precheck did not stop animation: %+v", results)
	}
	if f.service.DetectionTasks()[0].Running {
		t.Fatal("cancelled plan still busy")
	}
	if _, err := f.service.DeleteDetectionTask(context.Background(), plans[0].ID, plans[0].Version, "test"); err != nil {
		t.Fatal(err)
	}
}

func TestDetectionTaskEmptyGroupCompletesWithoutGeneration(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("empty group generated") })
	f.catalog.groupMembers = map[string][]string{"7": {}, "8": {}}
	plans, err := f.service.SaveDetectionTask(context.Background(), managedTask(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.RunDetectionTask(context.Background(), plans[0].ID, plans[0].Version); err != nil {
		t.Fatal(err)
	}
	task := finished(t, f)
	if task.Status != "succeeded" || task.Progress != 100 {
		t.Fatalf("bad empty-group result: %#v", task)
	}
	restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	if restarted.DetectionTasks()[0].LastTaskID != task.ID {
		t.Fatal("restart lost latest run")
	}
}
