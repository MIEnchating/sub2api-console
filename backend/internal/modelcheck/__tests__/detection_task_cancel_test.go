package modelcheck_test

import (
	"context"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestDetectionTaskCancellationStopsRemainingTerminalRoundsAndAnimation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var requests atomic.Int32
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) { requests.Add(1); cancel() })
	f.catalog.groupMembers = map[string][]string{"7": {"1"}, "8": {"1"}}
	value := managedTask()
	value.Precheck = false
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
	if len(task.Result["animations"].([]modelcheck.AnimationResult)) != 0 {
		t.Fatal("animation started after cancel")
	}
	if f.service.DetectionTasks()[0].Running {
		t.Fatal("cancelled plan still busy")
	}
	if _, err := f.service.DeleteDetectionTask(context.Background(), plans[0].ID, plans[0].Version, "test"); err != nil {
		t.Fatal(err)
	}
}

func TestDetectionTaskEmptyGroupCreatesFailedRecordWithoutGeneration(t *testing.T) {
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
	if task.Status != "failed" || task.Message != "所选分组当前没有账号，请同步目录或调整分组" {
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
