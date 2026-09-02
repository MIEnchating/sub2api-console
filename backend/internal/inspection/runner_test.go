package inspection

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/alerting"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/evidence"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/pricing"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
	"github.com/MIEnchating/sub2api-console/backend/internal/routingwrite"
	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type runnerRepositoryStub struct {
	trafficDue   bool
	upstreamDue  bool
	pricingDue   bool
	alertEnabled bool
	routingDue   bool
	mode         string
	controlErr   error
	markErr      error
	marked       []string
	policy       map[string]any
}

type releaseFailRepository struct {
	*business.Store
}

func (r *releaseFailRepository) ReleaseInspectionLease(ctx context.Context, ownerID string) error {
	if err := r.Store.ReleaseInspectionLease(ctx, ownerID); err != nil {
		return err
	}
	return errors.New("lease release failed")
}

func (r *runnerRepositoryStub) ControlPolicy(context.Context) (map[string]any, error) {
	if r.controlErr != nil {
		return nil, r.controlErr
	}
	if r.policy != nil {
		return r.policy, nil
	}
	return map[string]any{
		"traffic":             map[string]any{"enabled": true, "refresh_seconds": int64(60)},
		"upstream_multiplier": map[string]any{"interval_seconds": int64(120)},
	}, nil
}

func TestRunTaskMarksQueuedTaskCancelledWhenPlanningIsCancelled(t *testing.T) {
	tasks := openRunnerTaskStore(t)
	runner := NewRunner(&runnerRepositoryStub{controlErr: context.Canceled}, nil, nil, nil, nil, nil, nil, tasks)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{
		ID: "manual-cancelled", Skill: "inspection", Operation: "manual-inspection", Status: "queued",
		Message: "queued", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := runner.RunTask(ctx, task, RunRequest{Actor: "operator"})

	stored, err := tasks.Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "cancelled" || stored.Status != "cancelled" || stored.Result["cancelled"] != true {
		t.Fatalf("result=%#v task=%#v", result, stored)
	}
}

func openRunnerTaskStore(t *testing.T) *taskstore.Store {
	t.Helper()
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestManualInspectionKeepsTaskResultWhenSchedulerFinalizationFails(t *testing.T) {
	repository := &releaseFailRepository{Store: openInspectionRepository(t)}
	tasks := &countingTaskStore{}
	runner := NewRunner(&runnerRepositoryStub{}, nil, &evidencePlannerStub{}, nil, nil, nil, nil, tasks)
	scheduler, err := NewScheduler(repository, &immediateExecutor{})
	if err != nil {
		t.Fatal(err)
	}
	service := NewManualService(scheduler, runner, tasks)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	task := taskstore.Task{
		ID: "manual-finalized", Skill: "inspection", Operation: "manual-inspection", Status: "queued",
		Message: "queued", Result: map[string]any{}, CreatedAt: now, UpdatedAt: now,
	}

	service.execute(context.Background(), task, RunRequest{Actor: "operator"})

	if tasks.saves != 2 || tasks.last.Status != "succeeded" {
		t.Fatalf("saves=%d task=%#v", tasks.saves, tasks.last)
	}
}

func (r *runnerRepositoryStub) Mode(context.Context) (string, error) {
	if r.mode == "" {
		return runtimepolicy.Full, nil
	}
	return r.mode, nil
}

func (r *runnerRepositoryStub) AlertPolicy(context.Context) (business.AlertPolicy, error) {
	return business.AlertPolicy{Enabled: r.alertEnabled}, nil
}

func (r *runnerRepositoryStub) InspectionTaskDue(_ context.Context, name string, _ int, _ time.Time) (bool, error) {
	if name == "traffic" {
		return r.trafficDue, nil
	}
	if name == "price-management" {
		return r.pricingDue, nil
	}
	return r.upstreamDue, nil
}

func (r *runnerRepositoryStub) MarkInspectionTask(_ context.Context, name string, _ time.Time) error {
	r.marked = append(r.marked, name)
	return r.markErr
}

func (r *runnerRepositoryStub) RoutingWritebackPending(context.Context) (bool, error) {
	return r.routingDue, nil
}

type evidencePlannerStub struct {
	plan     evidence.Plan
	collects int
	options  []evidence.Options
}

func (e *evidencePlannerStub) Plan(context.Context, map[string]any, *string, *string, time.Time) (evidence.Plan, error) {
	return e.plan, nil
}

func (e *evidencePlannerStub) Collect(_ context.Context, _ map[string]any, _ evidence.Admin, options evidence.Options) (evidence.Result, error) {
	e.collects++
	e.options = append(e.options, options)
	return evidence.Result{}, nil
}

type alertEvaluatorStub struct {
	calls  int
	result alerting.Result
}

func (e *alertEvaluatorStub) Evaluate(context.Context) (alerting.Result, error) {
	e.calls++
	if e.result.Findings == 0 {
		e.result.Findings = 2
	}
	return e.result, nil
}

type countingTaskStore struct {
	saves int
	last  taskstore.Task
}

func (s *countingTaskStore) Save(_ context.Context, task taskstore.Task) error {
	s.saves++
	s.last = task
	return nil
}

type routerStub struct {
	calls            int
	persistDecisions bool
}

func (s *routerStub) Calculate(_ context.Context, _ routing.Scope, persistDecisions bool) (routing.Result, error) {
	s.calls++
	s.persistDecisions = persistDecisions
	return routing.Result{AccountTargets: map[string]business.AccountRoutingTarget{}}, nil
}

type writerStub struct{ calls int }

func (s *writerStub) Apply(context.Context, map[string]business.AccountRoutingTarget, string) (routingwrite.Result, error) {
	s.calls++
	return routingwrite.Result{}, nil
}

type accountRateSyncStub struct {
	calls int
	done  bool
}

func (s *accountRateSyncStub) SyncAllAccountRates(context.Context, string) (map[string]any, error) {
	s.calls++
	s.done = true
	return map[string]any{"requested": 2, "updated": 1, "unchanged": 1, "missing": 0, "failed": 0}, nil
}

type partialAccountRateSyncStub struct{}

func (partialAccountRateSyncStub) SyncAllAccountRates(context.Context, string) (map[string]any, error) {
	return map[string]any{
		"requested": 287, "updated": 0, "unchanged": 264, "missing": 0, "failed": 23,
	}, nil
}

type rateAwareRouterStub struct {
	rateSync *accountRateSyncStub
	calls    int
}

type priceAllocatorStub struct {
	calls int
	done  bool
}

func (stub *priceAllocatorStub) ApplyNow(context.Context, string) (pricing.Result, error) {
	stub.calls++
	stub.done = true
	return pricing.Result{Requested: 2, Changed: 1, Unchanged: 1, RemoteWrite: true}, nil
}

type priceAwareRouterStub struct{ pricing *priceAllocatorStub }

func (stub *priceAwareRouterStub) Calculate(context.Context, routing.Scope, bool) (routing.Result, error) {
	if !stub.pricing.done {
		return routing.Result{}, errors.New("routing ran before price management")
	}
	return routing.Result{AccountTargets: map[string]business.AccountRoutingTarget{}}, nil
}

func (s *rateAwareRouterStub) Calculate(_ context.Context, _ routing.Scope, _ bool) (routing.Result, error) {
	s.calls++
	if !s.rateSync.done {
		return routing.Result{}, errors.New("routing ran before account rate sync")
	}
	return routing.Result{AccountTargets: map[string]business.AccountRoutingTarget{}}, nil
}

type parallelEvidenceStub struct {
	evidencePlannerStub
	started         chan struct{}
	upstreamStarted <-chan struct{}
}

func (s *parallelEvidenceStub) Collect(ctx context.Context, _ map[string]any, _ evidence.Admin, _ evidence.Options) (evidence.Result, error) {
	close(s.started)
	select {
	case <-s.upstreamStarted:
		return evidence.Result{}, nil
	case <-ctx.Done():
		return evidence.Result{}, ctx.Err()
	}
}

type parallelUpstreamStub struct {
	started         chan struct{}
	evidenceStarted <-chan struct{}
}

type inspectionContextRecorder struct {
	mu     sync.Mutex
	values map[string]bool
}

func (recorder *inspectionContextRecorder) record(name string, ctx context.Context) {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.values[name] = mutationguard.IsAutomaticInspection(ctx)
}

func (recorder *inspectionContextRecorder) snapshot() map[string]bool {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	result := make(map[string]bool, len(recorder.values))
	for name, value := range recorder.values {
		result[name] = value
	}
	return result
}

type contextRecordingEvidence struct{ recorder *inspectionContextRecorder }

func (stub contextRecordingEvidence) Plan(context.Context, map[string]any, *string, *string, time.Time) (evidence.Plan, error) {
	return evidence.Plan{RequestedSource: "traffic"}, nil
}

func (stub contextRecordingEvidence) Collect(ctx context.Context, _ map[string]any, _ evidence.Admin, _ evidence.Options) (evidence.Result, error) {
	stub.recorder.record("evidence", ctx)
	return evidence.Result{}, nil
}

type contextRecordingUpstreams struct{ recorder *inspectionContextRecorder }

func (stub contextRecordingUpstreams) SyncAllNow(ctx context.Context, _ upstreamsync.Scope, _ string) (upstreamsync.BatchResult, error) {
	stub.recorder.record("upstreams", ctx)
	return upstreamsync.BatchResult{Total: 1, Succeeded: 1}, nil
}

type contextRecordingRates struct{ recorder *inspectionContextRecorder }

func (stub contextRecordingRates) SyncAllAccountRates(ctx context.Context, _ string) (map[string]any, error) {
	stub.recorder.record("rates", ctx)
	return map[string]any{"requested": 1, "unchanged": 1}, nil
}

type contextRecordingPricing struct{ recorder *inspectionContextRecorder }

func (stub contextRecordingPricing) ApplyNow(ctx context.Context, _ string) (pricing.Result, error) {
	stub.recorder.record("pricing", ctx)
	return pricing.Result{}, nil
}

type contextRecordingRouter struct{ recorder *inspectionContextRecorder }

func (stub contextRecordingRouter) Calculate(ctx context.Context, _ routing.Scope, _ bool) (routing.Result, error) {
	stub.recorder.record("routing", ctx)
	return routing.Result{AccountTargets: map[string]business.AccountRoutingTarget{}}, nil
}

type contextRecordingWriter struct{ recorder *inspectionContextRecorder }

func (stub contextRecordingWriter) Apply(ctx context.Context, _ map[string]business.AccountRoutingTarget, _ string) (routingwrite.Result, error) {
	stub.recorder.record("writeback", ctx)
	return routingwrite.Result{}, nil
}

type contextRecordingAlerts struct{ recorder *inspectionContextRecorder }

func (stub contextRecordingAlerts) Evaluate(ctx context.Context) (alerting.Result, error) {
	stub.recorder.record("alerts", ctx)
	return alerting.Result{}, nil
}

func TestInspectionContextPriorityPropagatesToEveryExecutionStage(t *testing.T) {
	for _, test := range []struct {
		name      string
		automatic bool
	}{
		{name: "automatic", automatic: true},
		{name: "manual", automatic: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := &inspectionContextRecorder{values: map[string]bool{}}
			repository := &runnerRepositoryStub{
				trafficDue: true, upstreamDue: true, pricingDue: true, alertEnabled: true, routingDue: true,
				mode: runtimepolicy.Full,
				policy: map[string]any{
					"traffic":             map[string]any{"enabled": true, "refresh_seconds": int64(60)},
					"probe":               map[string]any{"enabled": false},
					"recovery":            map[string]any{"enabled": false},
					"upstream_multiplier": map[string]any{"interval_seconds": int64(120)},
					"price_management": map[string]any{
						"enabled": true, "profit_margin": 0.2, "exchange_group_sets": []any{[]any{"6", "7"}},
						"interval_seconds": int64(120), "write_concurrency": int64(1),
					},
				},
			}
			runner := NewRunner(
				repository,
				nil,
				contextRecordingEvidence{recorder: recorder},
				contextRecordingRouter{recorder: recorder},
				contextRecordingWriter{recorder: recorder},
				contextRecordingAlerts{recorder: recorder},
				contextRecordingUpstreams{recorder: recorder},
				&countingTaskStore{},
				contextRecordingRates{recorder: recorder},
				contextRecordingPricing{recorder: recorder},
			)

			result, err := runner.Run(context.Background(), RunRequest{Actor: test.name, Automatic: test.automatic})
			if err != nil || result.Status != "succeeded" {
				t.Fatalf("inspection result=%#v err=%v", result, err)
			}
			values := recorder.snapshot()
			for _, stage := range []string{"evidence", "upstreams", "rates", "pricing", "routing", "writeback", "alerts"} {
				if value, found := values[stage]; !found || value != test.automatic {
					t.Errorf("stage %s automatic=%t found=%t, want %t", stage, value, found, test.automatic)
				}
			}
		})
	}
}

func (s *parallelUpstreamStub) SyncAllNow(ctx context.Context, _ upstreamsync.Scope, _ string) (upstreamsync.BatchResult, error) {
	close(s.started)
	select {
	case <-s.evidenceStarted:
		return upstreamsync.BatchResult{}, nil
	case <-ctx.Done():
		return upstreamsync.BatchResult{}, ctx.Err()
	}
}

func TestInspectionRunsUpstreamSyncAndEvidenceCollectionInParallel(t *testing.T) {
	evidenceStarted := make(chan struct{})
	upstreamStarted := make(chan struct{})
	repository := &runnerRepositoryStub{trafficDue: true, upstreamDue: true, mode: runtimepolicy.Monitoring}
	collector := &parallelEvidenceStub{
		evidencePlannerStub: evidencePlannerStub{plan: evidence.Plan{RequestedSource: "traffic"}},
		started:             evidenceStarted, upstreamStarted: upstreamStarted,
	}
	upstreams := &parallelUpstreamStub{started: upstreamStarted, evidenceStarted: evidenceStarted}
	runner := NewRunner(repository, nil, collector, &routerStub{}, nil, nil, upstreams, &countingTaskStore{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	result, err := runner.Run(ctx, RunRequest{Actor: "operator"})

	if err != nil || result.Status != "succeeded" {
		t.Fatalf("parallel collection phase failed: result=%#v err=%v", result, err)
	}
	if len(result.OperationTiming) < 2 || result.OperationTiming[0].StartedAt == nil || result.OperationTiming[1].StartedAt == nil {
		t.Fatalf("parallel operation start times were not recorded: %#v", result.OperationTiming)
	}
}

func TestInspectionDoesNotRepeatExternalWorkWhenDueStateCannotBeSaved(t *testing.T) {
	repository := &runnerRepositoryStub{
		trafficDue: true, upstreamDue: true, mode: runtimepolicy.Monitoring,
		markErr: errors.New("database busy"),
	}
	planner := &evidencePlannerStub{plan: evidence.Plan{RequestedSource: "traffic", ProbeAccountIDs: []string{"41"}}}
	tasks := &countingTaskStore{}
	runner := NewRunner(repository, nil, planner, nil, nil, nil, nil, tasks)

	result, err := runner.Run(context.Background(), RunRequest{Actor: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "failed" || planner.collects != 0 {
		t.Fatalf("result=%#v collects=%d", result, planner.collects)
	}
	if len(repository.marked) != 2 || tasks.last.Result["error"] == nil {
		t.Fatalf("marked=%#v task=%#v", repository.marked, tasks.last)
	}
}

func TestAutomaticRunDoesNotCreateTaskWhenNothingIsDue(t *testing.T) {
	repository := &runnerRepositoryStub{}
	planner := &evidencePlannerStub{plan: evidence.Plan{RequestedSource: "traffic", ProbeAccountIDs: []string{}}}
	tasks := &countingTaskStore{}
	runner := NewRunner(repository, nil, planner, nil, nil, nil, nil, tasks)
	runner.now = func() time.Time { return time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC) }

	result, err := runner.Execute(context.Background(), business.AutoInspectionConfig{Enabled: true, IntervalSeconds: 15})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Skipped || result.Status != "succeeded" || result.TaskID != nil || tasks.saves != 0 {
		t.Fatalf("empty heartbeat created a business task: result=%#v saves=%d", result, tasks.saves)
	}
}

func TestPendingWritebackRunsWithoutNewCollection(t *testing.T) {
	repository := &runnerRepositoryStub{routingDue: true}
	planner := &evidencePlannerStub{plan: evidence.Plan{RequestedSource: "traffic"}}
	router := &routerStub{}
	writer := &writerStub{}
	runner := NewRunner(repository, nil, planner, router, writer, nil, nil, &countingTaskStore{})

	result, err := runner.Run(context.Background(), RunRequest{Actor: "自动巡检", Automatic: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" || planner.collects != 0 || router.calls != 1 || writer.calls != 1 {
		t.Fatalf("pending retry did not run calculation-only convergence: result=%#v collects=%d router=%d writer=%d",
			result, planner.collects, router.calls, writer.calls)
	}
	if len(result.Operations) != 2 || result.Operations[0] != operationRoutingCalculation || result.Operations[1] != operationRoutingWriteback {
		t.Fatalf("operations=%#v", result.Operations)
	}
}

func TestScopedManualInspectionIsDiagnosticOnly(t *testing.T) {
	accountID := "41"
	repository := &runnerRepositoryStub{mode: runtimepolicy.Full}
	planner := &evidencePlannerStub{plan: evidence.Plan{RequestedSource: "traffic"}}
	router := &routerStub{}
	writer := &writerStub{}
	tasks := &countingTaskStore{}
	runner := NewRunner(repository, nil, planner, router, writer, nil, nil, tasks)
	runner.now = func() time.Time { return time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC) }

	result, err := runner.Run(context.Background(), RunRequest{AccountID: &accountID, Actor: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" || router.calls != 1 || router.persistDecisions || writer.calls != 0 {
		t.Fatalf("scoped inspection performed scheduling mutation: result=%#v router=%#v writer=%#v", result, router, writer)
	}
	for _, operation := range result.Operations {
		if operation == operationRoutingWriteback {
			t.Fatalf("scoped diagnostic advertised automatic execution: %#v", result.Operations)
		}
	}
	if tasks.last.Message != "诊断巡检完成：仅更新健康评估，未保存调度结果，未自动执行远程变更" || tasks.last.Result["diagnostic_only"] != true {
		t.Fatalf("scoped diagnostic semantics are not visible: %#v", tasks.last)
	}
}

func TestUnscopedFullModeInspectionCanPersistAndApply(t *testing.T) {
	repository := &runnerRepositoryStub{mode: runtimepolicy.Full, trafficDue: true}
	planner := &evidencePlannerStub{plan: evidence.Plan{RequestedSource: "traffic"}}
	router := &routerStub{}
	writer := &writerStub{}
	runner := NewRunner(repository, nil, planner, router, writer, nil, nil, &countingTaskStore{})
	runner.now = func() time.Time { return time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC) }

	result, err := runner.Run(context.Background(), RunRequest{Actor: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" || router.calls != 1 || !router.persistDecisions || writer.calls != 1 {
		t.Fatalf("unscoped full inspection did not apply policy: result=%#v router=%#v writer=%#v", result, router, writer)
	}
}

func TestUpstreamOnlyInspectionStillRecalculatesAndAppliesRouting(t *testing.T) {
	repository := &runnerRepositoryStub{mode: runtimepolicy.Full, upstreamDue: true}
	planner := &evidencePlannerStub{plan: evidence.Plan{RequestedSource: "traffic"}}
	router := &routerStub{}
	writer := &writerStub{}
	runner := NewRunner(repository, nil, planner, router, writer, nil, &parallelUpstreamStub{
		started: make(chan struct{}), evidenceStarted: closedChannel(),
	}, &countingTaskStore{})

	result, err := runner.Run(context.Background(), RunRequest{Actor: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" || router.calls != 1 || writer.calls != 1 {
		t.Fatalf("upstream-only inspection skipped routing: result=%#v router=%#v writer=%#v", result, router, writer)
	}
}

func TestFullInspectionSyncsAccountRatesBeforeRouting(t *testing.T) {
	repository := &runnerRepositoryStub{mode: runtimepolicy.Full, upstreamDue: true}
	planner := &evidencePlannerStub{plan: evidence.Plan{RequestedSource: "traffic"}}
	rateSync := &accountRateSyncStub{}
	router := &rateAwareRouterStub{rateSync: rateSync}
	runner := NewRunner(repository, nil, planner, router, &writerStub{}, nil, &parallelUpstreamStub{
		started: make(chan struct{}), evidenceStarted: closedChannel(),
	}, &countingTaskStore{}, rateSync)

	result, err := runner.Run(context.Background(), RunRequest{Actor: "auto-inspection"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" || rateSync.calls != 1 || router.calls != 1 {
		t.Fatalf("result=%#v rate_sync=%d router=%d", result, rateSync.calls, router.calls)
	}
	want := []string{operationUpstreamSync, operationAccountRateSync, operationRoutingCalculation, operationRoutingWriteback}
	if len(result.Operations) != len(want) {
		t.Fatalf("operations=%#v", result.Operations)
	}
	for index := range want {
		if result.Operations[index] != want[index] {
			t.Fatalf("operations=%#v want=%#v", result.Operations, want)
		}
	}
}

func TestInspectionReportsAccountRateItemFailuresAsPartial(t *testing.T) {
	repository := &runnerRepositoryStub{mode: runtimepolicy.Full, upstreamDue: true}
	tasks := openRunnerTaskStore(t)
	runner := NewRunner(repository, nil, &evidencePlannerStub{}, &routerStub{}, &writerStub{}, nil, &parallelUpstreamStub{
		started: make(chan struct{}), evidenceStarted: closedChannel(),
	}, tasks, partialAccountRateSyncStub{})

	result, err := runner.Run(context.Background(), RunRequest{Actor: "auto-inspection"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "partial" || result.Error == nil ||
		*result.Error != "账号倍率与名称同步部分失败：缺失 0，失败 23" {
		t.Fatalf("partial item failures were promoted to a full failure: %#v", result)
	}
	stored, err := tasks.Get(context.Background(), *result.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != "partial" || stored.Message != "巡检完成，但存在部分失败" {
		t.Fatalf("partial task status was not persisted: %#v", stored)
	}
}

func TestEnabledPriceManagementRunsBeforeRoutingAndDefaultRemainsOff(t *testing.T) {
	allocator := &priceAllocatorStub{}
	defaultRepository := &runnerRepositoryStub{mode: runtimepolicy.Full, pricingDue: true}
	defaultRunner := NewRunner(defaultRepository, nil, &evidencePlannerStub{}, &routerStub{}, &writerStub{}, nil, nil, &countingTaskStore{}, allocator)
	defaultResult, err := defaultRunner.Run(context.Background(), RunRequest{Actor: "auto", Automatic: true})
	if err != nil {
		t.Fatal(err)
	}
	if !defaultResult.Skipped || allocator.calls != 0 {
		t.Fatalf("disabled-by-default pricing ran: result=%#v calls=%d", defaultResult, allocator.calls)
	}

	repository := &runnerRepositoryStub{mode: runtimepolicy.Full, pricingDue: true, policy: map[string]any{
		"traffic": map[string]any{"enabled": false, "refresh_seconds": int64(60)},
		"probe":   map[string]any{"enabled": false}, "recovery": map[string]any{"enabled": false},
		"upstream_multiplier": map[string]any{"interval_seconds": int64(120)},
		"price_management": map[string]any{
			"enabled": true, "profit_margin": 0.2, "exchange_group_sets": []any{[]any{"6", "7"}},
			"interval_seconds": int64(120), "write_concurrency": int64(4),
		},
	}}
	allocator = &priceAllocatorStub{}
	runner := NewRunner(repository, nil, &evidencePlannerStub{}, &priceAwareRouterStub{pricing: allocator}, &writerStub{}, nil, nil, &countingTaskStore{}, allocator)
	result, err := runner.Run(context.Background(), RunRequest{Actor: "auto", Automatic: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" || allocator.calls != 1 {
		t.Fatalf("enabled pricing did not run: result=%#v calls=%d", result, allocator.calls)
	}
	want := []string{operationPriceManagement, operationRoutingCalculation, operationRoutingWriteback}
	if !reflect.DeepEqual(result.Operations, want) {
		t.Fatalf("operations=%#v want=%#v", result.Operations, want)
	}
}

func TestLegacyPriceManagementConfigIsNotScheduled(t *testing.T) {
	repository := &runnerRepositoryStub{mode: runtimepolicy.Full, pricingDue: true, policy: map[string]any{
		"traffic": map[string]any{"enabled": false, "refresh_seconds": int64(60)},
		"probe":   map[string]any{"enabled": false}, "recovery": map[string]any{"enabled": false},
		"upstream_multiplier": map[string]any{"interval_seconds": int64(120)},
		"price_management": map[string]any{
			"enabled": true, "profit_margin": 0.2, "managed_group_ids": []any{"6", "7"},
			"interval_seconds": int64(120), "write_concurrency": int64(4),
		},
	}}
	allocator := &priceAllocatorStub{}
	runner := NewRunner(repository, nil, &evidencePlannerStub{}, &routerStub{}, &writerStub{}, nil, nil, &countingTaskStore{}, allocator)

	result, err := runner.Run(context.Background(), RunRequest{Actor: "auto", Automatic: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Skipped || allocator.calls != 0 {
		t.Fatalf("legacy pricing config was scheduled: result=%#v calls=%d", result, allocator.calls)
	}
}

func TestInspectionTaskPersistsPlannedActiveAndCompletedOperations(t *testing.T) {
	repository := &runnerRepositoryStub{mode: runtimepolicy.Full, trafficDue: true, alertEnabled: true}
	planner := &evidencePlannerStub{plan: evidence.Plan{RequestedSource: "traffic", ProbeAccountIDs: []string{"41"}}}
	tasks := &countingTaskStore{}
	runner := NewRunner(repository, nil, planner, &routerStub{}, &writerStub{}, &alertEvaluatorStub{}, nil, tasks)

	result, err := runner.Run(context.Background(), RunRequest{Actor: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" {
		t.Fatalf("inspection failed: %#v", result)
	}
	planned, ok := tasks.last.Result["planned_operations"].([]QueueOperation)
	if !ok || len(planned) != 5 {
		t.Fatalf("planned operations were not persisted: %#v", tasks.last.Result)
	}
	completed, ok := tasks.last.Result["completed_operations"].([]string)
	if !ok || len(completed) != 5 {
		t.Fatalf("completed operations were not persisted: %#v", tasks.last.Result)
	}
	active, ok := tasks.last.Result["active_operations"].([]string)
	if !ok || len(active) != 0 {
		t.Fatalf("terminal task kept active operations: %#v", tasks.last.Result)
	}
}

func closedChannel() <-chan struct{} {
	value := make(chan struct{})
	close(value)
	return value
}

func TestGlobalManualRoundUsesTheSameDuePlanAsAutomaticHeartbeat(t *testing.T) {
	repository := &runnerRepositoryStub{mode: runtimepolicy.Monitoring, trafficDue: true, upstreamDue: true, alertEnabled: true}
	planner := &evidencePlannerStub{plan: evidence.Plan{RequestedSource: "traffic", ProbeAccountIDs: []string{"41", "42"}}}
	runner := NewRunner(repository, nil, planner, nil, nil, nil, nil, &countingTaskStore{})
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

	manual, err := runner.plan(context.Background(), RunRequest{}, now)
	if err != nil {
		t.Fatal(err)
	}
	automatic, err := runner.plan(context.Background(), RunRequest{Automatic: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if manual.traffic != automatic.traffic || manual.probes != automatic.probes || manual.upstreams != automatic.upstreams || manual.alert != automatic.alert {
		t.Fatalf("manual plan=%#v automatic plan=%#v", manual, automatic)
	}
	if !manual.traffic || manual.probes || !manual.upstreams || !manual.alert {
		t.Fatalf("manual heartbeat omitted due operations: %#v", manual)
	}
}

func TestMonitoringModeDisablesAutomaticActiveProbes(t *testing.T) {
	repository := &runnerRepositoryStub{mode: runtimepolicy.Monitoring, trafficDue: true}
	planner := &evidencePlannerStub{plan: evidence.Plan{RequestedSource: "traffic", ProbeAccountIDs: []string{"41"}}}
	runner := NewRunner(repository, nil, planner, &routerStub{}, nil, nil, nil, &countingTaskStore{})

	result, err := runner.Run(context.Background(), RunRequest{Actor: "自动巡检", Automatic: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" || planner.collects != 1 || len(planner.options) != 1 {
		t.Fatalf("monitoring inspection did not collect read-only evidence: result=%#v planner=%#v", result, planner)
	}
	if planner.options[0].ProbesAllowed {
		t.Fatalf("monitoring inspection enabled active probes: %#v", planner.options[0])
	}
	for _, operation := range result.Operations {
		if operation == operationActiveProbe {
			t.Fatalf("monitoring inspection reported an active probe: %#v", result.Operations)
		}
	}
}

func TestMonitoringModePreviewOmitsActiveProbe(t *testing.T) {
	repository := &runnerRepositoryStub{mode: runtimepolicy.Monitoring, trafficDue: true}
	planner := &evidencePlannerStub{plan: evidence.Plan{RequestedSource: "traffic", ProbeAccountIDs: []string{"41"}}}
	runner := NewRunner(repository, nil, planner, nil, nil, nil, nil, &countingTaskStore{})

	item, err := runner.Preview(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range item.Operations {
		if operation.Operation == operationActiveProbe {
			t.Fatalf("monitoring preview advertised an active probe: %#v", item)
		}
	}
	if item.TargetCount != nil {
		t.Fatalf("monitoring preview exposed probe targets: %#v", item)
	}
}

func TestMonitoringModeKeepsExplicitScopedProbeAvailable(t *testing.T) {
	accountID := "41"
	repository := &runnerRepositoryStub{mode: runtimepolicy.Monitoring}
	planner := &evidencePlannerStub{plan: evidence.Plan{RequestedSource: "traffic", ProbeAccountIDs: []string{"41"}}}
	runner := NewRunner(repository, nil, planner, nil, nil, nil, nil, &countingTaskStore{})

	plan, err := runner.plan(context.Background(), RunRequest{AccountID: &accountID, Actor: "operator"}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !plan.probes || len(plan.probeIDs) != 1 {
		t.Fatalf("explicit scoped probe was disabled: %#v", plan)
	}
}

func TestGlobalManualRoundDoesNotForceTasksThatAreNotDue(t *testing.T) {
	repository := &runnerRepositoryStub{mode: runtimepolicy.Monitoring}
	planner := &evidencePlannerStub{plan: evidence.Plan{RequestedSource: "traffic"}}
	runner := NewRunner(repository, nil, planner, nil, nil, nil, nil, &countingTaskStore{})

	plan, err := runner.plan(context.Background(), RunRequest{}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if plan.traffic || plan.probes || plan.upstreams || plan.alert {
		t.Fatalf("manual heartbeat forced operations that were not due: %#v", plan)
	}
}

func TestAutomaticRunExecutesAlertOnlyWithoutRunningInspectionCalculation(t *testing.T) {
	repository := &runnerRepositoryStub{alertEnabled: true}
	planner := &evidencePlannerStub{plan: evidence.Plan{RequestedSource: "traffic", ProbeAccountIDs: []string{}}}
	alerts := &alertEvaluatorStub{}
	tasks := &countingTaskStore{}
	runner := NewRunner(repository, nil, planner, nil, nil, alerts, nil, tasks)
	runner.now = func() time.Time { return time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC) }

	result, err := runner.Execute(context.Background(), business.AutoInspectionConfig{Enabled: true, IntervalSeconds: 15})
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped || result.Status != "succeeded" || alerts.calls != 1 || planner.collects != 0 {
		t.Fatalf("alert-only heartbeat ran the wrong operations: result=%#v alerts=%d collects=%d", result, alerts.calls, planner.collects)
	}
	if len(result.Operations) != 1 || result.Operations[0] != operationAlertEvaluation {
		t.Fatalf("alert-only heartbeat operations=%#v", result.Operations)
	}
}

func TestAutomaticRunReportsNotificationFailure(t *testing.T) {
	repository := &runnerRepositoryStub{alertEnabled: true}
	planner := &evidencePlannerStub{plan: evidence.Plan{RequestedSource: "traffic"}}
	alerts := &alertEvaluatorStub{result: alerting.Result{AlertEvaluationRecord: business.AlertEvaluationRecord{
		Status: "failed", Summary: "告警检测完成，通知发送未完成",
	}}}
	runner := NewRunner(repository, nil, planner, nil, nil, alerts, nil, &countingTaskStore{})
	runner.now = func() time.Time { return time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC) }

	result, err := runner.Execute(context.Background(), business.AutoInspectionConfig{Enabled: true, IntervalSeconds: 15})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "failed" || result.Error == nil || *result.Error != "告警检测完成，通知发送未完成" {
		t.Fatalf("notification failure was hidden: %#v", result)
	}
}

func TestAutomaticPlanCreatesOneMergedTaskForSeveralDueOperations(t *testing.T) {
	repository := &runnerRepositoryStub{trafficDue: true, upstreamDue: true, alertEnabled: true}
	planner := &evidencePlannerStub{plan: evidence.Plan{RequestedSource: "traffic", ProbeAccountIDs: []string{"41", "42"}}}
	runner := NewRunner(repository, nil, planner, nil, nil, nil, nil, &countingTaskStore{})

	plan, err := runner.plan(context.Background(), RunRequest{Automatic: true}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if !plan.traffic || !plan.upstreams || !plan.probes || len(plan.probeIDs) != 2 {
		t.Fatalf("due operations were not merged into one inspection plan: %#v", plan)
	}
}

func TestPreviewCollapsesSeveralDueOperationsIntoOneQueueItem(t *testing.T) {
	repository := &runnerRepositoryStub{trafficDue: true, upstreamDue: true, alertEnabled: true}
	planner := &evidencePlannerStub{plan: evidence.Plan{RequestedSource: "traffic", ProbeAccountIDs: []string{"41", "42"}}}
	runner := NewRunner(repository, nil, planner, nil, nil, nil, nil, &countingTaskStore{})

	item, err := runner.Preview(context.Background(), time.Date(2026, 8, 27, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if item.TaskType != "inspection" || item.State != "ready" || item.TargetCount == nil || *item.TargetCount != 2 {
		t.Fatalf("unexpected merged queue item: %#v", item)
	}
	if item.Label != "本轮巡检（6 项）" || len(item.Operations) != 6 ||
		item.Detail != "包含到期操作：上游数据同步、真实流量同步、主动探测、调度计算、自动执行、告警检测" {
		t.Fatalf("merged operations were not structured: %#v", item)
	}
	for _, label := range []string{"上游数据同步", "真实流量同步", "主动探测", "调度计算", "自动执行", "告警检测"} {
		found := false
		for _, operation := range item.Operations {
			found = found || operation.Label == label
		}
		if !found {
			t.Fatalf("merged queue operations are missing %q: %#v", label, item.Operations)
		}
	}
	if item.Operations[0].Cycle != "每2分钟" || item.Operations[1].Cycle != "每1分钟" ||
		item.Operations[2].Cycle != "按账号策略（常规每5分钟；回池每3分钟）" {
		t.Fatalf("queue operation cycles were not exposed: %#v", item.Operations)
	}
}

func TestPreviewKeepsConfiguredCyclesVisibleWhenNothingIsDue(t *testing.T) {
	repository := &runnerRepositoryStub{}
	planner := &evidencePlannerStub{plan: evidence.Plan{RequestedSource: "traffic"}}
	runner := NewRunner(repository, nil, planner, nil, nil, nil, nil, &countingTaskStore{})

	item, err := runner.Preview(context.Background(), time.Date(2026, 8, 27, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if item.State != "waiting" || len(item.Operations) != 5 {
		t.Fatalf("waiting queue hid configured task cycles: %#v", item)
	}
	if item.Label != "下一轮巡检计划" || item.Detail != "当前没有操作到期，下次心跳会重新检查" {
		t.Fatalf("waiting queue used an ambiguous label: %#v", item)
	}
	for _, operation := range item.Operations {
		if operation.Cycle == "" || operation.Due {
			t.Fatalf("unexpected waiting operation: %#v", operation)
		}
	}
}

func TestPreviewRejectsIntervalsBelowGuardianMinimums(t *testing.T) {
	tests := []struct {
		name   string
		policy map[string]any
		want   string
	}{
		{
			name: "regular probe",
			policy: map[string]any{
				"traffic":  map[string]any{"enabled": false},
				"probe":    map[string]any{"enabled": true, "interval_seconds": int64(29)},
				"recovery": map[string]any{"enabled": false},
			},
			want: "策略字段 interval_seconds 必须是 30 到 86400 之间的整数",
		},
		{
			name: "upstream multiplier",
			policy: map[string]any{
				"traffic":             map[string]any{"enabled": false},
				"probe":               map[string]any{"enabled": false},
				"recovery":            map[string]any{"enabled": false},
				"upstream_multiplier": map[string]any{"interval_seconds": int64(29)},
			},
			want: "策略字段 interval_seconds 必须是 30 到 86400 之间的整数",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &runnerRepositoryStub{mode: runtimepolicy.Full, policy: test.policy}
			runner := NewRunner(repository, nil, &evidencePlannerStub{}, nil, nil, nil, nil, &countingTaskStore{})
			_, err := runner.Preview(context.Background(), time.Now().UTC())
			if err == nil || err.Error() != test.want {
				t.Fatalf("invalid interval error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestPreviewRejectsInvalidRuntimeModeBeforeAdvertisingOperations(t *testing.T) {
	repository := &runnerRepositoryStub{trafficDue: true, upstreamDue: true, mode: "配置错误"}
	planner := &evidencePlannerStub{plan: evidence.Plan{RequestedSource: "traffic"}}
	runner := NewRunner(repository, nil, planner, nil, nil, nil, nil, &countingTaskStore{})

	if _, err := runner.Preview(context.Background(), time.Now().UTC()); err == nil || err.Error() != "运行模式无效：配置错误" {
		t.Fatalf("invalid mode error=%v", err)
	}
}

func TestAppliedCleanupCountOnlyIncludesChangedCleanupTargets(t *testing.T) {
	cleanup := "remove_group"
	targets := map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", CleanupAction: &cleanup},
		"42": {AccountID: "42", CleanupAction: &cleanup},
		"43": {AccountID: "43"},
	}
	result := routingwrite.Result{Results: []routingwrite.AccountResult{
		{AccountID: "41", Changed: true},
		{AccountID: "42", Changed: false},
		{AccountID: "43", Changed: true},
		{AccountID: "missing", Changed: true},
	}}
	if count := appliedCleanupCount(targets, result); count != 1 {
		t.Fatalf("cleanup count=%d, want 1", count)
	}
}

func TestAppliedRoutingTransitionsOnlyCountConfirmedWriteback(t *testing.T) {
	off, on := false, true
	targets := map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", Schedulable: &off},
		"42": {AccountID: "42", Schedulable: &on},
		"43": {AccountID: "43", Schedulable: &off},
		"44": {AccountID: "44", Schedulable: &off},
	}
	failure := "write failed"
	result := routingwrite.Result{Results: []routingwrite.AccountResult{
		{AccountID: "41", Changed: true, Before: map[string]any{"schedulable": true}, Effective: map[string]any{"schedulable": false}},
		{AccountID: "42", Changed: true, Before: map[string]any{"schedulable": false}, Effective: map[string]any{"schedulable": true}},
		{AccountID: "43", Changed: false, Error: &failure, Before: map[string]any{"schedulable": true}},
		{AccountID: "44", Changed: true, Error: &failure, Before: map[string]any{"schedulable": true}, Effective: map[string]any{"schedulable": false}},
	}}
	fused, recovered := appliedRoutingTransitions(targets, result)
	if fused != 2 || recovered != 1 {
		t.Fatalf("confirmed transitions fused=%d recovered=%d", fused, recovered)
	}
}

func TestGlobalManualRoundDoesNotRequireOptionalProbeFallback(t *testing.T) {
	if strictEvidenceFallback(RunRequest{}) {
		t.Fatal("global manual round should continue when optional probing is disabled")
	}
	accountID := "41"
	if !strictEvidenceFallback(RunRequest{AccountID: &accountID}) {
		t.Fatal("account-scoped inspection should require strict evidence")
	}
	groupName := "codex"
	if !strictEvidenceFallback(RunRequest{GroupName: &groupName}) {
		t.Fatal("group-scoped inspection should require strict evidence")
	}
}
