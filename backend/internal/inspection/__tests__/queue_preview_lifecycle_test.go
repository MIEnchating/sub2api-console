package inspection_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/inspection"
)

type previewQueue struct {
	work []func(context.Context)
}

func (q *previewQueue) Go(work func(context.Context)) error {
	q.work = append(q.work, work)
	return nil
}

func (q *previewQueue) take(t *testing.T) func(context.Context) {
	t.Helper()
	if len(q.work) == 0 {
		t.Fatal("updated plan did not schedule a preview")
	}
	work := q.work[0]
	q.work = q.work[1:]
	return work
}

type lifecyclePreviewExecutor struct {
	store   *business.Store
	preview func(context.Context, inspection.QueueItem) (inspection.QueueItem, error)
	rounds  int
}

func (e *lifecyclePreviewExecutor) Execute(context.Context, business.AutoInspectionConfig) (inspection.ExecutionResult, error) {
	e.rounds++
	return inspection.ExecutionResult{Status: "succeeded"}, nil
}

func (e *lifecyclePreviewExecutor) Preview(ctx context.Context, _ time.Time) (inspection.QueueItem, error) {
	config, err := e.store.AutoInspectionConfig(ctx)
	if err != nil {
		return inspection.QueueItem{}, err
	}
	item := inspection.QueueItem{
		TaskType: "inspection", State: "ready",
		Label:      fmt.Sprintf("interval=%d rounds=%d", config.IntervalSeconds, e.rounds),
		Operations: []inspection.QueueOperation{},
	}
	if e.preview != nil {
		return e.preview(ctx, item)
	}
	return item, nil
}

func previewLifecycle(t *testing.T) (*inspection.Scheduler, *lifecyclePreviewExecutor, *previewQueue) {
	t.Helper()
	store, err := business.Open(filepath.Join(t.TempDir(), "business.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	config := business.DefaultAutoInspectionConfig()
	config.Enabled, config.IntervalSeconds = true, 15
	if _, err := store.UpdateAutoInspectionConfig(t.Context(), config); err != nil {
		t.Fatal(err)
	}
	executor := &lifecyclePreviewExecutor{store: store}
	scheduler, err := inspection.NewScheduler(store, executor)
	if err != nil {
		t.Fatal(err)
	}
	queue := &previewQueue{}
	scheduler.UseTaskRunner(queue)
	return scheduler, executor, queue
}

func requirePlanLabel(t *testing.T, scheduler *inspection.Scheduler, expected string) {
	t.Helper()
	status, err := scheduler.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Queue) != 1 || status.Queue[0].Label != expected {
		t.Fatalf("queue=%+v, want %q", status.Queue, expected)
	}
}

func TestSavingInspectionConfigInvalidatesFreshPreview(t *testing.T) {
	scheduler, _, queue := previewLifecycle(t)
	requirePlanLabel(t, scheduler, "正在读取巡检计划")
	queue.take(t)(t.Context())
	requirePlanLabel(t, scheduler, "interval=15 rounds=0")
	status, err := scheduler.UpdateConfig(t.Context(), business.AutoInspectionConfig{Enabled: true, IntervalSeconds: 60})
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Queue) != 1 || status.Queue[0].Label != "正在读取巡检计划" {
		t.Fatalf("saved config returned previous plan: %+v", status.Queue)
	}
	queue.take(t)(t.Context())
	requirePlanLabel(t, scheduler, "interval=60 rounds=0")
}

func TestCompletedInspectionInvalidatesFreshPreview(t *testing.T) {
	scheduler, _, queue := previewLifecycle(t)
	requirePlanLabel(t, scheduler, "正在读取巡检计划")
	queue.take(t)(t.Context())
	ran, err := scheduler.RunDue(t.Context(), time.Now().UTC(), true)
	if err != nil || !ran {
		t.Fatalf("run=%v err=%v", ran, err)
	}
	requirePlanLabel(t, scheduler, "正在读取巡检计划")
	queue.take(t)(t.Context())
	requirePlanLabel(t, scheduler, "interval=15 rounds=1")
}

func TestConfigChangeSkipsObsoleteQueuedPreview(t *testing.T) {
	scheduler, executor, queue := previewLifecycle(t)
	calls := 0
	executor.preview = func(_ context.Context, item inspection.QueueItem) (inspection.QueueItem, error) {
		calls++
		return item, nil
	}
	requirePlanLabel(t, scheduler, "正在读取巡检计划")
	oldWork := queue.take(t)
	if _, err := scheduler.UpdateConfig(t.Context(), business.AutoInspectionConfig{Enabled: true, IntervalSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	oldWork(t.Context())
	if calls != 0 {
		t.Fatal("obsolete queued preview read the plan")
	}
	// Discarding old queued work must not clear the newer preview's busy state.
	requirePlanLabel(t, scheduler, "正在读取巡检计划")
	queue.take(t)(t.Context())
	requirePlanLabel(t, scheduler, "interval=60 rounds=0")
	if len(queue.work) != 0 {
		t.Fatal("status duplicated the pending preview")
	}
}

func TestObsoletePreviewCannotOverwriteUpdatedPlan(t *testing.T) {
	scheduler, executor, queue := previewLifecycle(t)
	entered := make(chan context.Context, 1)
	release := make(chan struct{})
	finished := make(chan struct{})
	executor.preview = func(ctx context.Context, item inspection.QueueItem) (inspection.QueueItem, error) {
		if item.Label == "interval=15 rounds=0" {
			entered <- ctx
			<-release
		}
		return item, nil
	}
	requirePlanLabel(t, scheduler, "正在读取巡检计划")
	oldWork := queue.take(t)
	go func() { oldWork(t.Context()); close(finished) }()
	t.Cleanup(func() { close(release); <-finished })
	var oldContext context.Context
	select {
	case oldContext = <-entered:
	case <-t.Context().Done():
		t.Fatal("old preview did not start")
	}
	if _, err := scheduler.UpdateConfig(t.Context(), business.AutoInspectionConfig{Enabled: true, IntervalSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	queue.take(t)(t.Context())
	requirePlanLabel(t, scheduler, "interval=60 rounds=0")
	if oldContext.Err() != context.Canceled {
		t.Fatal("obsolete preview was not cancelled")
	}
	// A dependency may finish despite cancellation; its result must stay obsolete.
	release <- struct{}{}
	<-finished
	requirePlanLabel(t, scheduler, "interval=60 rounds=0")
}
