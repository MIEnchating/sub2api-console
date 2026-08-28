package inspection

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	_ "modernc.org/sqlite"
)

type blockingExecutor struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
	name    string
}

type reportingExecutor struct {
	taskID  string
	started chan struct{}
	release chan struct{}
}

func (e *reportingExecutor) Execute(ctx context.Context, _ business.AutoInspectionConfig) (ExecutionResult, error) {
	reportExecutionTask(ctx, e.taskID)
	close(e.started)
	<-e.release
	return ExecutionResult{TaskID: stringPointer(e.taskID), Status: "succeeded"}, nil
}

func (e *blockingExecutor) Execute(context.Context, business.AutoInspectionConfig) (ExecutionResult, error) {
	e.once.Do(func() { close(e.started) })
	<-e.release
	taskID := e.name
	return ExecutionResult{TaskID: &taskID, Status: "succeeded"}, nil
}

type immediateExecutor struct {
	calls      int
	monitoring *bool
	summary    *InspectionSummary
	skipped    bool
}

func (e *immediateExecutor) Execute(context.Context, business.AutoInspectionConfig) (ExecutionResult, error) {
	e.calls++
	taskID := "second"
	return ExecutionResult{
		TaskID: &taskID, Status: "succeeded", MonitoringEnabled: e.monitoring,
		Summary: e.summary, Skipped: e.skipped,
	}, nil
}

type cancellableExecutor struct {
	started chan struct{}
}

func (e *cancellableExecutor) Execute(ctx context.Context, _ business.AutoInspectionConfig) (ExecutionResult, error) {
	close(e.started)
	<-ctx.Done()
	return ExecutionResult{}, ctx.Err()
}

type blockingAcquireRepository struct {
	*business.Store
	entered chan struct{}
}

func (r *blockingAcquireRepository) AcquireInspectionLease(
	ctx context.Context,
	_ string,
	_ int,
	_ string,
	_ time.Time,
	_ time.Time,
	_ time.Duration,
) (bool, error) {
	close(r.entered)
	<-ctx.Done()
	return false, ctx.Err()
}

type previewingExecutor struct {
	immediateExecutor
	item QueueItem
}

type flakyConfigRepository struct {
	*business.Store
	mu    sync.Mutex
	calls int
}

func (r *flakyConfigRepository) AutoInspectionConfig(ctx context.Context) (business.AutoInspectionConfig, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.calls == 1 {
		return business.AutoInspectionConfig{}, errors.New("temporary configuration failure")
	}
	return r.Store.AutoInspectionConfig(ctx)
}

func (e *previewingExecutor) Preview(context.Context, time.Time) (QueueItem, error) {
	return e.item, nil
}

func TestZeroDelayRunsImmediatelyInsteadOfWaitingForReconfiguration(t *testing.T) {
	scheduler, err := NewScheduler(openInspectionRepository(t), &immediateExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan bool, 1)
	go func() { result <- scheduler.waitForReconfigure(context.Background(), 0) }()
	select {
	case reconfigured := <-result:
		if reconfigured {
			t.Fatal("zero delay was reported as a configuration change")
		}
	case <-time.After(time.Second):
		t.Fatal("due inspection remained blocked waiting for reconfiguration")
	}
}

func TestStartCanRetryAfterConfigurationReadFails(t *testing.T) {
	repository := &flakyConfigRepository{Store: openInspectionRepository(t)}
	scheduler, err := NewScheduler(repository, &immediateExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Start(context.Background()); err == nil {
		t.Fatal("expected the first configuration read to fail")
	}
	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatalf("scheduler could not restart after transient failure: %v", err)
	}
	scheduler.Stop()
}

func TestStopWaitsForTheRunningLoopToExit(t *testing.T) {
	repository := openInspectionRepository(t)
	if _, err := repository.UpdateAutoInspectionConfig(context.Background(), business.AutoInspectionConfig{Enabled: true, IntervalSeconds: 15}); err != nil {
		t.Fatal(err)
	}
	executor := &cancellableExecutor{started: make(chan struct{})}
	scheduler, err := NewScheduler(repository, executor)
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)
	scheduler.mu.Lock()
	scheduler.nextRunAt = &past
	scheduler.mu.Unlock()
	select {
	case scheduler.reconfigure <- struct{}{}:
	default:
	}
	select {
	case <-executor.started:
	case <-time.After(3 * time.Second):
		t.Fatal("overdue inspection did not start")
	}
	scheduler.Stop()
	scheduler.mu.Lock()
	running, currentCancel, loopDone := scheduler.running, scheduler.currentCancel, scheduler.loopDone
	scheduler.mu.Unlock()
	if running || currentCancel != nil || loopDone != nil {
		t.Fatalf("scheduler returned before shutdown completed: running=%v cancel=%v done=%v", running, currentCancel != nil, loopDone != nil)
	}
}

func TestDatabaseLeasePreventsOverlappingSchedulers(t *testing.T) {
	repository := openInspectionRepository(t)
	ctx := context.Background()
	if _, err := repository.UpdateAutoInspectionConfig(ctx, business.AutoInspectionConfig{Enabled: true, IntervalSeconds: 15}); err != nil {
		t.Fatal(err)
	}
	firstExecutor := &blockingExecutor{started: make(chan struct{}), release: make(chan struct{}), name: "first"}
	secondExecutor := &immediateExecutor{}
	first, err := NewScheduler(repository, firstExecutor)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewScheduler(repository, secondExecutor)
	if err != nil {
		t.Fatal(err)
	}
	firstDone := make(chan error, 1)
	go func() {
		_, runErr := first.RunDue(ctx, time.Now().UTC(), true)
		firstDone <- runErr
	}()
	select {
	case <-firstExecutor.started:
	case <-time.After(3 * time.Second):
		t.Fatal("first inspection did not start")
	}
	started, err := second.RunDue(ctx, time.Now().UTC(), true)
	if err != nil {
		t.Fatal(err)
	}
	if started || secondExecutor.calls != 0 {
		t.Fatalf("overlapping scheduler executed: started=%v calls=%d", started, secondExecutor.calls)
	}
	close(firstExecutor.release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	history, err := repository.InspectionHeartbeats(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Status != "succeeded" || history[0].TaskID == nil || *history[0].TaskID != "first" {
		t.Fatalf("unexpected heartbeat history: %#v", history)
	}
}

func TestRunningHeartbeatPublishesTaskIDBeforeExecutionCompletes(t *testing.T) {
	repository := openInspectionRepository(t)
	executor := &reportingExecutor{taskID: "task-running", started: make(chan struct{}), release: make(chan struct{})}
	scheduler, err := NewScheduler(repository, executor)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, runErr := scheduler.RunDue(context.Background(), time.Now().UTC(), true)
		done <- runErr
	}()
	select {
	case <-executor.started:
	case <-time.After(3 * time.Second):
		t.Fatal("executor did not start")
	}
	history, err := repository.InspectionHeartbeats(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Status != "running" || history[0].TaskID == nil || *history[0].TaskID != "task-running" {
		t.Fatalf("running task ID was not published: %#v", history)
	}
	close(executor.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCancelDisablesFutureRunsAndInterruptsCurrentExecution(t *testing.T) {
	repository := openInspectionRepository(t)
	ctx := context.Background()
	if _, err := repository.UpdateAutoInspectionConfig(ctx, business.AutoInspectionConfig{Enabled: true, IntervalSeconds: 15}); err != nil {
		t.Fatal(err)
	}
	executor := &cancellableExecutor{started: make(chan struct{})}
	scheduler, err := NewScheduler(repository, executor)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, runErr := scheduler.RunDue(ctx, time.Now().UTC(), true)
		done <- runErr
	}()
	select {
	case <-executor.started:
	case <-time.After(3 * time.Second):
		t.Fatal("inspection did not start")
	}
	status, canceled, err := scheduler.Cancel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !canceled || status.Enabled || status.NextRunAt != nil {
		t.Fatalf("cancel did not disable scheduling: canceled=%v status=%#v", canceled, status)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("canceled inspection did not stop")
	}
	history, err := repository.InspectionHeartbeats(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].Status != "cancelled" || history[0].Error != nil {
		t.Fatalf("unexpected canceled heartbeat: %#v", history)
	}
}

func TestCancelInterruptsTheRunBeforeLeaseAcquisitionCompletes(t *testing.T) {
	store := openInspectionRepository(t)
	repository := &blockingAcquireRepository{Store: store, entered: make(chan struct{})}
	ctx := context.Background()
	if _, err := store.UpdateAutoInspectionConfig(ctx, business.AutoInspectionConfig{Enabled: true, IntervalSeconds: 15}); err != nil {
		t.Fatal(err)
	}
	executor := &immediateExecutor{}
	scheduler, err := NewScheduler(repository, executor)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, runErr := scheduler.RunDue(ctx, time.Now().UTC(), true)
		done <- runErr
	}()
	select {
	case <-repository.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("lease acquisition did not start")
	}
	status, canceled, err := scheduler.Cancel(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !canceled || status.Enabled {
		t.Fatalf("early cancellation was not accepted: canceled=%v status=%#v", canceled, status)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("run returned %v, want context cancellation", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("lease acquisition was not interrupted")
	}
	if executor.calls != 0 {
		t.Fatalf("executor ran after early cancellation: %d", executor.calls)
	}
}

func TestStatusPublishesConfirmedMonitoringAvailability(t *testing.T) {
	repository := openInspectionRepository(t)
	available := true
	scheduler, err := NewScheduler(repository, &immediateExecutor{monitoring: &available})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scheduler.RunDue(context.Background(), time.Now().UTC(), true); err != nil {
		t.Fatal(err)
	}
	status, err := scheduler.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.MonitoringEnabled || status.MonitoringCheckedAt == nil {
		t.Fatalf("monitoring availability was not published: %#v", status)
	}
}

func TestStatusKeepsLastCompletedRoundSummaryAcrossSkippedHeartbeats(t *testing.T) {
	repository := openInspectionRepository(t)
	ctx := context.Background()
	start := time.Date(2026, 8, 27, 1, 0, 0, 0, time.UTC)
	summary := &InspectionSummary{
		Channels: 233, Probed: 10, Samples: 112, Fused: 2,
		Recovered: 1, Applied: 24, CleanedUp: 3, Alerts: 4,
	}
	executor := &immediateExecutor{summary: summary}
	scheduler, err := NewScheduler(repository, executor)
	if err != nil {
		t.Fatal(err)
	}
	completedAt := start.Add(29681 * time.Millisecond)
	scheduler.now = func() time.Time { return completedAt }
	if _, err := scheduler.RunDue(ctx, start, true); err != nil {
		t.Fatal(err)
	}
	status, err := scheduler.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.LastRunDurationMS != 29681 || status.LastSummary != *summary {
		t.Fatalf("completed round summary was not published: %#v", status)
	}

	executor.summary = nil
	executor.skipped = true
	completedAt = start.Add(45 * time.Second)
	if _, err := scheduler.RunDue(ctx, start.Add(30*time.Second), true); err != nil {
		t.Fatal(err)
	}
	status, err = scheduler.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.LastSummary != *summary {
		t.Fatalf("skipped heartbeat erased the last completed round summary: %#v", status.LastSummary)
	}
}

func TestStatusRestoresDurationSummaryAndMonitoringAfterRestart(t *testing.T) {
	repository := openInspectionRepository(t)
	ctx := context.Background()
	start := time.Date(2026, 8, 27, 2, 0, 0, 0, time.UTC)
	completedAt := start.Add(18432 * time.Millisecond)
	available := true
	summary := &InspectionSummary{
		Channels: 8, Probed: 5, Samples: 3, Fused: 1,
		Recovered: 2, Applied: 4, CleanedUp: 1, Alerts: 2,
	}
	first, err := NewScheduler(repository, &immediateExecutor{monitoring: &available, summary: summary})
	if err != nil {
		t.Fatal(err)
	}
	first.now = func() time.Time { return completedAt }
	if _, err := first.RunDue(ctx, start, true); err != nil {
		t.Fatal(err)
	}

	restarted, err := NewScheduler(repository, &immediateExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	restarted.now = func() time.Time { return completedAt.Add(time.Second) }
	status, err := restarted.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.LastRunDurationMS != 18432 || status.LastSummary != *summary {
		t.Fatalf("restart lost completed round metrics: %#v", status)
	}
	if !status.MonitoringEnabled || status.MonitoringCheckedAt == nil || *status.MonitoringCheckedAt != completedAt.Format(time.RFC3339Nano) {
		t.Fatalf("restart lost monitoring result: %#v", status)
	}
}

func TestExpiredLocalLeaseDoesNotBlockTheNextInspection(t *testing.T) {
	repository := openInspectionRepository(t)
	ctx := context.Background()
	now := time.Now().UTC()
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	acquired, err := repository.AcquireInspectionLease(ctx, "expired-owner", os.Getpid(), host, now, now, time.Second)
	if err != nil || !acquired {
		t.Fatalf("initial lease acquisition failed: acquired=%v err=%v", acquired, err)
	}
	active, err := repository.ActiveInspectionCheckedAt(ctx, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if active != nil {
		t.Fatalf("expired local lease remained active: %s", *active)
	}
	reacquired, err := repository.AcquireInspectionLease(ctx, "next-owner", os.Getpid(), host, now.Add(2*time.Second), now.Add(2*time.Second), time.Second)
	if err != nil || !reacquired {
		t.Fatalf("expired lease blocked the next inspection: acquired=%v err=%v", reacquired, err)
	}
}

func TestStatusProjectsUnleasedRunningHeartbeatAsInterrupted(t *testing.T) {
	repository := openInspectionRepository(t)
	checkedAt := time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC)
	if err := repository.RecordInspectionHeartbeat(context.Background(), business.InspectionHeartbeat{
		CheckedAt: checkedAt.Format(time.RFC3339Nano), Status: "running",
		Operations: []string{}, OperationTiming: []business.OperationTiming{},
	}); err != nil {
		t.Fatal(err)
	}
	scheduler, err := NewScheduler(repository, &immediateExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	scheduler.now = func() time.Time { return checkedAt.Add(5 * time.Minute) }
	status, err := scheduler.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Running || len(status.HeartbeatHistory) != 1 || status.HeartbeatHistory[0].Status != "failed" ||
		status.HeartbeatHistory[0].Error == nil || *status.HeartbeatHistory[0].Error != business.InspectionInterruptedText() {
		t.Fatalf("stale heartbeat remained running: %#v", status)
	}
	persisted, err := repository.InspectionHeartbeats(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted) != 1 || persisted[0].Status != "failed" || persisted[0].CompletedAt == nil {
		t.Fatalf("stale heartbeat projection was not persisted: %#v", persisted)
	}
}

func TestStatusPublishesOnlyOneInspectionPlanQueueItem(t *testing.T) {
	repository := openInspectionRepository(t)
	ctx := context.Background()
	if _, err := repository.UpdateAutoInspectionConfig(ctx, business.AutoInspectionConfig{Enabled: true, IntervalSeconds: 15}); err != nil {
		t.Fatal(err)
	}
	targets := 3
	executor := &previewingExecutor{item: QueueItem{
		TaskType: "inspection", Label: "本轮巡检（3 项）", State: "ready",
		Detail: "包含到期操作：主动探测、调度计算、告警检测", TargetCount: &targets,
	}}
	scheduler, err := NewScheduler(repository, executor)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 27, 1, 0, 0, 0, time.UTC)
	scheduler.now = func() time.Time { return now }
	next := now.Add(15 * time.Second).Format(time.RFC3339Nano)
	scheduler.nextRunAt = &next

	status, err := scheduler.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Queue) != 1 || status.Queue[0].TaskType != "inspection" || status.Queue[0].ScheduledFor == nil || *status.Queue[0].ScheduledFor != next {
		t.Fatalf("unexpected queue: %#v", status.Queue)
	}
}

func TestReconcileKeepsOnlyLeaseBackedHeartbeatRunning(t *testing.T) {
	repository := openInspectionRepository(t)
	ctx := context.Background()
	now := time.Now().UTC()
	old := now.Add(-time.Hour)
	if err := repository.RecordInspectionHeartbeat(ctx, business.InspectionHeartbeat{
		CheckedAt: old.Format(time.RFC3339Nano), Status: "running", Operations: []string{}, OperationTiming: []business.OperationTiming{},
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.RecordInspectionHeartbeat(ctx, business.InspectionHeartbeat{
		CheckedAt: now.Format(time.RFC3339Nano), Status: "running", Operations: []string{}, OperationTiming: []business.OperationTiming{},
	}); err != nil {
		t.Fatal(err)
	}
	host, _ := os.Hostname()
	acquired, err := repository.AcquireInspectionLease(ctx, "owner", os.Getpid(), host, now, now, 2*time.Minute)
	if err != nil || !acquired {
		t.Fatalf("lease acquisition failed: acquired=%v err=%v", acquired, err)
	}
	interrupted, err := repository.ReconcileInterruptedInspections(ctx, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if interrupted != 1 {
		t.Fatalf("interrupted=%d", interrupted)
	}
	history, err := repository.InspectionHeartbeats(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	if history[0].Status != "running" || history[1].Status != "failed" {
		t.Fatalf("wrong active/stale projection: %#v", history)
	}
}

func TestClearHistoryRemovesCompletedHeartbeatsAndResetsSummary(t *testing.T) {
	repository := openInspectionRepository(t)
	ctx := context.Background()
	if err := repository.RecordInspectionHeartbeat(ctx, business.InspectionHeartbeat{
		CheckedAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano), Status: "succeeded",
		Operations: []string{}, OperationTiming: []business.OperationTiming{},
	}); err != nil {
		t.Fatal(err)
	}
	scheduler, err := NewScheduler(repository, &immediateExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	status, err := scheduler.Status(ctx)
	if err != nil || len(status.HeartbeatHistory) != 1 {
		t.Fatalf("unexpected initial history: %#v err=%v", status, err)
	}
	deleted, err := scheduler.ClearHistory(ctx)
	if err != nil || deleted != 1 {
		t.Fatalf("deleted=%d err=%v", deleted, err)
	}
	status, err = scheduler.Status(ctx)
	if err != nil || len(status.HeartbeatHistory) != 0 || status.LastStatus != nil || status.LastRunAt != nil ||
		status.LastRunDurationMS != 0 || status.LastSummary != (InspectionSummary{}) {
		t.Fatalf("history summary was not reset: %#v err=%v", status, err)
	}
}

func openInspectionRepository(t *testing.T) *business.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "inspection.sqlite3")
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE migration_runs(id INTEGER PRIMARY KEY,status TEXT NOT NULL)`,
		`INSERT INTO migration_runs VALUES(1,'succeeded')`,
		`CREATE TABLE app_state(key TEXT PRIMARY KEY,value_json TEXT NOT NULL,updated_at TEXT NOT NULL)`,
		`CREATE TABLE policies(key TEXT PRIMARY KEY,value_json TEXT NOT NULL,updated_at TEXT NOT NULL)`,
		`CREATE TABLE policy_nodes(id INTEGER PRIMARY KEY AUTOINCREMENT,policy_key TEXT NOT NULL,parent_id INTEGER,key_name TEXT,list_index INTEGER,node_type TEXT NOT NULL,scalar_value TEXT,updated_at TEXT NOT NULL)`,
		`CREATE UNIQUE INDEX ux_policy_nodes_root ON policy_nodes(policy_key) WHERE parent_id IS NULL`,
		`CREATE TABLE scheduler_leases(lease_name TEXT PRIMARY KEY,owner_id TEXT NOT NULL,owner_pid INTEGER NOT NULL,owner_host TEXT NOT NULL,checked_at TEXT NOT NULL,acquired_at TEXT NOT NULL,renewed_at TEXT NOT NULL,expires_at TEXT NOT NULL)`,
	}
	for _, statement := range statements {
		if _, err := database.Exec(statement); err != nil {
			database.Close()
			t.Fatalf("fixture statement failed: %v\n%s", err, statement)
		}
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repository.Close() })
	return repository
}
