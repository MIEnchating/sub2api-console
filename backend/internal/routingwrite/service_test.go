package routingwrite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountops"
	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/mutationguard"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	_ "modernc.org/sqlite"
)

type concurrentWriteAdmin struct {
	mu           sync.Mutex
	state        map[string]any
	dbPath       string
	accounts     int
	mutates      int
	mutations    []routingMutation
	deletes      int
	deleteChecks []bool
	mutateErr    error
	mutateErrors map[string]error
}

type routingMutation struct {
	method string
	path   string
	body   map[string]any
}

type staticRoutingTarget struct {
	value configstore.TargetSettings
}

var testRoutingTarget = configstore.TargetSettings{
	BaseURL: "https://routing.test", AdminKey: "routing-test-key", TimeoutSeconds: 2,
}

func (target staticRoutingTarget) TargetSettings(context.Context) (configstore.TargetSettings, error) {
	return target.value, nil
}

type legacySchedulingAdmin struct {
	calls []routingMutation
}

type blockingDeleteAdmin struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

type routingLeaseRoundKey struct{}

type routingLeaseAcquisition struct {
	round     string
	resources []string
}

type orchestratedRoutingLeaseRepository struct {
	*business.Store
	allowFirstRoundAccount42 chan struct{}
	acquired                 chan routingLeaseAcquisition
}

type observedRoutingBaselineRepository struct {
	*business.Store
	firstRead chan struct{}
	once      sync.Once
}

func (repository *observedRoutingBaselineRepository) RoutingBaselines(ctx context.Context) ([]business.RoutingBaseline, error) {
	baselines, err := repository.Store.RoutingBaselines(ctx)
	if err == nil {
		repository.once.Do(func() { close(repository.firstRead) })
	}
	return baselines, err
}

func (repository *orchestratedRoutingLeaseRepository) AcquireMutationLease(
	ctx context.Context,
	ownerID string,
	resources []string,
	now time.Time,
	ttl time.Duration,
) (bool, error) {
	round, _ := ctx.Value(routingLeaseRoundKey{}).(string)
	if round == "first" && len(resources) == 1 && resources[0] == mutationguard.Account("42") {
		select {
		case <-repository.allowFirstRoundAccount42:
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
	acquired, err := repository.Store.AcquireMutationLease(ctx, ownerID, resources, now, ttl)
	if acquired {
		repository.acquired <- routingLeaseAcquisition{
			round: round, resources: append([]string(nil), resources...),
		}
	}
	return acquired, err
}

func (repository *orchestratedRoutingLeaseRepository) RenewMutationLease(
	ctx context.Context,
	ownerID string,
	resources []string,
	now time.Time,
	ttl time.Duration,
) (bool, error) {
	return repository.Store.RenewMutationLease(ctx, ownerID, resources, now, ttl)
}

func (repository *orchestratedRoutingLeaseRepository) ReleaseMutationLease(
	ctx context.Context,
	ownerID string,
	resources []string,
) error {
	return repository.Store.ReleaseMutationLease(ctx, ownerID, resources)
}

func (admin *blockingDeleteAdmin) Account(context.Context, string) (map[string]any, error) {
	return map[string]any{
		"id": json.Number("41"), "name": "alpha", "schedulable": false,
		"priority": int64(20), "load_factor": int64(3), "concurrency": int64(4),
	}, nil
}

func (admin *blockingDeleteAdmin) Mutate(context.Context, string, string, map[string]any) (map[string]any, error) {
	return nil, errors.New("unexpected mutation")
}

func (admin *blockingDeleteAdmin) DeleteAccount(ctx context.Context, accountID string) (map[string]any, error) {
	return admin.DeleteAccountWithVerification(ctx, accountID, true)
}

func (admin *blockingDeleteAdmin) DeleteAccountWithVerification(ctx context.Context, _ string, _ bool) (map[string]any, error) {
	admin.once.Do(func() { close(admin.started) })
	select {
	case <-admin.release:
		return map[string]any{"confirmed_absent": true}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *legacySchedulingAdmin) Account(context.Context, string) (map[string]any, error) {
	return nil, errors.New("unexpected account read")
}

func (a *legacySchedulingAdmin) DeleteAccount(context.Context, string) (map[string]any, error) {
	return nil, errors.New("unexpected account delete")
}

func (a *legacySchedulingAdmin) Mutate(_ context.Context, method, path string, body map[string]any) (map[string]any, error) {
	a.calls = append(a.calls, routingMutation{method: method, path: path, body: copyMap(body)})
	if len(a.calls) == 1 {
		return nil, &adminclient.HTTPError{StatusCode: 404, Detail: "route not found"}
	}
	return map[string]any{"success": true}, nil
}

func newTestService(repository Repository, admin Admin) *Service {
	return &Service{
		targets: staticRoutingTarget{value: testRoutingTarget}, repository: repository, admin: admin, now: time.Now,
	}
}

func TestCleanupTargetsAreOrderedAfterAllRoutingWritebacks(t *testing.T) {
	deleteAction := "delete"
	ids := orderedTargetIDs(map[string]business.AccountRoutingTarget{
		"1": {AccountID: "1", CleanupAction: &deleteAction},
		"2": {AccountID: "2"},
		"3": {AccountID: "3", CleanupAction: &deleteAction},
		"4": {AccountID: "4"},
	})
	want := []string{"2", "4", "1", "3"}
	if len(ids) != len(want) {
		t.Fatalf("执行顺序长度=%d want=%d: %#v", len(ids), len(want), ids)
	}
	for index := range want {
		if ids[index] != want[index] {
			t.Fatalf("清理必须排在全轮调度写回之后：got=%#v want=%#v", ids, want)
		}
	}
}

func TestApplyRejectsTargetIDsThatDoNotMatchStableMapKeys(t *testing.T) {
	for name, targets := range map[string]map[string]business.AccountRoutingTarget{
		"mismatched embedded ID": {
			"41": {AccountID: "42", GroupNames: []string{"codex"}},
		},
		"noncanonical stable ID": {
			"041": {AccountID: "041", GroupNames: []string{"codex"}},
		},
		"duplicate embedded ID": {
			"41": {AccountID: "41", GroupNames: []string{"codex"}},
			"42": {AccountID: "41", GroupNames: []string{"codex"}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			admin := &concurrentWriteAdmin{}
			service := newTestService(nil, admin)
			if _, err := service.Apply(context.Background(), targets, "scheduler"); err == nil {
				t.Fatal("invalid routing target identity was accepted")
			}
			admin.mu.Lock()
			defer admin.mu.Unlock()
			if admin.accounts != 0 || admin.mutates != 0 || admin.deletes != 0 {
				t.Fatalf("invalid identity reached remote admin: %#v", admin)
			}
		})
	}
}

func TestApplyRejectsUpstreamManagementTargetChangeBeforeRemoteWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing-target-drift.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	ctx := context.Background()
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.UpdatePolicy(ctx, map[string]any{
		"auto_apply": map[string]any{
			"schedulable": false, "priority": true, "load_factor": false, "concurrency": false,
		},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("41"), "name": "alpha", "priority": json.Number("20"),
		"groups": []any{json.Number("7")},
	}}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test"); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	replacement := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer replacement.Close()
	service := New(staticRoutingTarget{value: configstore.TargetSettings{
		BaseURL: replacement.URL, AdminKey: "target-b", TimeoutSeconds: 2,
	}}, repository)
	ctx = targetguard.Expect(ctx, configstore.TargetSettings{
		BaseURL: "https://target-a.example", AdminKey: "target-a", TimeoutSeconds: 2,
	})
	priority := int64(10)
	result, err := service.Apply(ctx, map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"codex"}, Priority: &priority},
	}, "scheduler")
	if !errors.Is(err, targetguard.ErrChanged) {
		t.Fatalf("target drift error=%v result=%#v", err, result)
	}
	if requests.Load() != 0 || result.RemoteWrite {
		t.Fatalf("target drift reached replacement target: requests=%d result=%#v", requests.Load(), result)
	}
}

func TestApplySkipsManualPriorityTargetsBeforeRemoteWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing-manual-priority.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	ctx := context.Background()
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.UpdatePolicy(ctx, map[string]any{
		"auto_apply": map[string]any{
			"schedulable": true, "priority": true, "load_factor": true, "concurrency": true,
		},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("41"), "name": "manual", "schedulable": true,
		"priority": json.Number("20"), "load_factor": json.Number("3"), "concurrency": json.Number("4"),
		"groups": []any{json.Number("7")},
	}}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.AssignManualPriority(ctx, "41", 3, "100", 100, false, "operator"); err != nil {
		t.Fatal(err)
	}
	admin := &concurrentWriteAdmin{state: map[string]any{
		"id": json.Number("41"), "schedulable": true, "priority": int64(20), "load_factor": int64(3), "concurrency": int64(4),
	}}
	service := newTestService(repository, admin)
	schedulable := false
	result, err := service.Apply(ctx, map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"codex"}, Schedulable: &schedulable},
	}, "scheduler")
	if err != nil {
		t.Fatal(err)
	}
	if admin.mutates != 0 || len(result.Results) != 1 || !result.Results[0].Skipped || result.Results[0].RemoteWrite {
		t.Fatalf("result=%#v remote writes=%d", result, admin.mutates)
	}
}

func TestManualPauseWaitsForInFlightCleanupDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing-delete-reservation.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	ctx := context.Background()
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("41"), "name": "alpha", "schedulable": false,
		"priority": json.Number("20"), "load_factor": json.Number("3"), "concurrency": json.Number("4"),
		"groups": []any{json.Number("7")},
	}}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test"); err != nil {
		t.Fatal(err)
	}
	admin := &blockingDeleteAdmin{started: make(chan struct{}), release: make(chan struct{})}
	routingService := newTestService(repository, admin)
	cleanup := "delete"
	applyDone := make(chan error, 1)
	go func() {
		_, err := routingService.Apply(ctx, map[string]business.AccountRoutingTarget{
			"41": {AccountID: "41", GroupNames: []string{"codex"}, CleanupAction: &cleanup},
		}, "scheduler")
		applyDone <- err
	}()
	select {
	case <-admin.started:
	case <-time.After(3 * time.Second):
		t.Fatal("cleanup delete did not reach the remote call")
	}

	accountService := accountops.New(nil, repository, nil)
	pauseDone := make(chan error, 1)
	go func() {
		_, err := accountService.Control(ctx, "41", "pause", "operator")
		pauseDone <- err
	}()
	select {
	case err := <-pauseDone:
		t.Fatalf("manual pause overtook in-flight delete: %v", err)
	case <-time.After(75 * time.Millisecond):
	}
	close(admin.release)
	select {
	case err := <-applyDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cleanup delete did not complete")
	}
	select {
	case err := <-pauseDone:
		if err == nil {
			t.Fatal("manual pause unexpectedly succeeded after account deletion")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("manual pause stayed blocked after delete released its reservation")
	}
}

func TestCleanupDeleteWaitsForAccountCatalogReservation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing-delete-catalog-reservation.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	ctx := context.Background()
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("41"), "name": "alpha", "schedulable": false,
		"priority": json.Number("20"), "load_factor": json.Number("3"), "concurrency": json.Number("4"),
		"groups": []any{json.Number("7")},
	}}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test"); err != nil {
		t.Fatal(err)
	}
	_, releaseCatalog, err := mutationguard.Acquire(ctx, repository, mutationguard.AccountCatalog())
	if err != nil {
		t.Fatal(err)
	}

	admin := &blockingDeleteAdmin{started: make(chan struct{}), release: make(chan struct{})}
	service := newTestService(repository, admin)
	cleanup := "delete"
	applyDone := make(chan error, 1)
	go func() {
		_, err := service.Apply(ctx, map[string]business.AccountRoutingTarget{
			"41": {AccountID: "41", GroupNames: []string{"codex"}, CleanupAction: &cleanup},
		}, "scheduler")
		applyDone <- err
	}()
	select {
	case <-admin.started:
		t.Fatal("cleanup delete bypassed the account catalog reservation")
	case <-time.After(75 * time.Millisecond):
	}
	if err := releaseCatalog(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-admin.started:
	case <-time.After(3 * time.Second):
		t.Fatal("cleanup delete did not continue after the catalog reservation was released")
	}
	close(admin.release)
	select {
	case err := <-applyDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cleanup delete did not complete")
	}
}

func TestConcurrentOverlappingApplyBatchesDoNotDeadlockOnPartialAccountLeases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing-overlapping-batches.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	repository := &orchestratedRoutingLeaseRepository{
		Store:                    store,
		allowFirstRoundAccount42: make(chan struct{}),
		acquired:                 make(chan routingLeaseAcquisition, 4),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.UpdatePolicy(ctx, map[string]any{
		"auto_apply": map[string]any{
			"schedulable": false, "priority": true, "load_factor": false, "concurrency": false,
		},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.SyncManagementSnapshot(ctx, []map[string]any{
		{
			"id": json.Number("41"), "name": "alpha", "priority": json.Number("20"),
			"groups": []any{json.Number("7")},
		},
		{
			"id": json.Number("42"), "name": "beta", "priority": json.Number("20"),
			"groups": []any{json.Number("7")},
		},
	}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test"); err != nil {
		t.Fatal(err)
	}
	admin := &concurrentWriteAdmin{dbPath: path, state: map[string]any{
		"id": json.Number("41"), "priority": int64(20),
	}}
	service := newTestService(repository, admin)
	priority := int64(10)
	targets := map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"codex"}, Priority: &priority},
		"42": {AccountID: "42", GroupNames: []string{"codex"}, Priority: &priority},
	}
	type applyOutcome struct {
		result Result
		err    error
	}
	start := func(round string) <-chan applyOutcome {
		done := make(chan applyOutcome, 1)
		go func() {
			result, err := service.Apply(context.WithValue(ctx, routingLeaseRoundKey{}, round), targets, "scheduler")
			done <- applyOutcome{result: result, err: err}
		}()
		return done
	}
	waitAcquisition := func(wantRound string) routingLeaseAcquisition {
		for {
			select {
			case acquisition := <-repository.acquired:
				if acquisition.round == wantRound {
					return acquisition
				}
			case <-ctx.Done():
				t.Fatalf("waiting for %s routing lease: %v", wantRound, ctx.Err())
				return routingLeaseAcquisition{}
			}
		}
	}

	firstDone := start("first")
	firstLease := waitAcquisition("first")
	secondDone := start("second")
	if len(firstLease.resources) == 1 {
		secondLease := waitAcquisition("second")
		if len(secondLease.resources) != 1 || secondLease.resources[0] != mutationguard.Account("42") {
			t.Fatalf("second round acquired unexpected partial lease: %#v", secondLease.resources)
		}
		close(repository.allowFirstRoundAccount42)
	} else if len(firstLease.resources) != 2 ||
		firstLease.resources[0] != mutationguard.Account("41") ||
		firstLease.resources[1] != mutationguard.Account("42") {
		t.Fatalf("first round did not reserve the complete batch atomically: %#v", firstLease.resources)
	}

	for round, done := range map[string]<-chan applyOutcome{"first": firstDone, "second": secondDone} {
		select {
		case outcome := <-done:
			if outcome.err != nil || outcome.result.Failed != 0 {
				t.Fatalf("%s overlapping batch failed: result=%#v err=%v", round, outcome.result, outcome.err)
			}
		case <-ctx.Done():
			t.Fatalf("%s overlapping batch deadlocked: %v", round, ctx.Err())
		}
	}
}

type failingCommitRepository struct {
	Repository
	err error
}

type policyAccessRepository struct {
	Repository
	calls int
}

type protectedRestoreRepository struct{ Repository }

type recordingRoutingRepository struct {
	Repository
	mu         sync.Mutex
	operations []business.AccountOperation
	events     []string
}

func (r *recordingRoutingRepository) RecordAccountOperation(_ context.Context, operation business.AccountOperation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.operations = append(r.operations, operation)
	return nil
}

func (r *recordingRoutingRepository) RecordRuntimeEvent(
	_ context.Context,
	eventType string,
	_ string,
	_ string,
	_ map[string]any,
) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, eventType)
	return int64(len(r.events)), nil
}

func (r *recordingRoutingRepository) operationCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.operations)
}

func (r *recordingRoutingRepository) eventCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

func (protectedRestoreRepository) AccountMutationProtection(context.Context, string) (business.AccountMutationProtection, error) {
	return business.AccountMutationProtection{Paused: true}, nil
}

func TestRecordOperationSkipsRepositoryAfterContextCancellation(t *testing.T) {
	repository := &recordingRoutingRepository{}
	service := newTestService(repository, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	service.recordOperation(ctx, business.AccountOperation{OperationID: "cancelled-operation", ObjectID: "41"})

	if repository.operationCount() != 0 {
		t.Fatal("cancelled operation attempted a repository write")
	}
}

func TestRecordRuntimeEventSkipsRepositoryAfterContextCancellation(t *testing.T) {
	repository := &recordingRoutingRepository{}
	service := newTestService(repository, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	service.recordRuntimeEvent(ctx, "routing.apply_failed", "failed", "cancelled", nil)

	if repository.eventCount() != 0 {
		t.Fatal("cancelled runtime event attempted a repository write")
	}
}

func TestRecordOperationPersistsBusinessFailureWhileContextIsActive(t *testing.T) {
	repository := &recordingRoutingRepository{}
	service := newTestService(repository, nil)

	service.recordOperation(context.Background(), business.AccountOperation{
		OperationID: "business-failure", ObjectID: "41", State: "failed",
	})

	if repository.operationCount() != 1 {
		t.Fatal("active business failure was not recorded")
	}
}

func TestRecordRuntimeEventPersistsBusinessFailureWhileContextIsActive(t *testing.T) {
	repository := &recordingRoutingRepository{}
	service := newTestService(repository, nil)

	service.recordRuntimeEvent(context.Background(), "routing.apply_failed", "failed", "upstream rejected", nil)

	if repository.eventCount() != 1 {
		t.Fatal("active business failure event was not recorded")
	}
}

func TestApplySkipsPerAccountEventsAfterBatchCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing-cancelled-events.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{
		{
			"id": json.Number("41"), "name": "alpha", "schedulable": true,
			"priority": json.Number("20"), "load_factor": json.Number("3"), "concurrency": json.Number("4"),
			"groups": []any{json.Number("7")},
		},
		{
			"id": json.Number("42"), "name": "beta", "schedulable": true,
			"priority": json.Number("20"), "load_factor": json.Number("3"), "concurrency": json.Number("4"),
			"groups": []any{json.Number("7")},
		},
	}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test"); err != nil {
		t.Fatal(err)
	}
	repository := &recordingRoutingRepository{Repository: store}
	admin := &batchCoordinatorAdmin{
		states: map[string]map[string]any{
			"41": coordinatorState("41", 20),
			"42": coordinatorState("42", 20),
		},
		started: make(chan struct{}, 1), release: make(chan struct{}), ignoreCancellation: true,
	}
	service := newTestService(repository, admin)
	priority := int64(10)
	applyCtx, cancel := context.WithCancel(ctx)
	type applyResult struct {
		result Result
		err    error
	}
	done := make(chan applyResult, 1)
	go func() {
		result, applyErr := service.Apply(applyCtx, map[string]business.AccountRoutingTarget{
			"41": {AccountID: "41", GroupNames: []string{"codex"}, Priority: &priority},
			"42": {AccountID: "42", GroupNames: []string{"codex"}, Priority: &priority},
		}, "scheduler")
		done <- applyResult{result: result, err: applyErr}
	}()
	select {
	case <-admin.started:
	case <-time.After(3 * time.Second):
		t.Fatal("batch write did not start")
	}
	cancel()
	select {
	case outcome := <-done:
		t.Fatalf("apply returned before the remote write completed: %#v", outcome)
	case <-time.After(100 * time.Millisecond):
	}
	close(admin.release)
	select {
	case outcome := <-done:
		if outcome.err != nil || outcome.result.Failed != 2 {
			t.Fatalf("cancelled apply result=%#v err=%v", outcome.result, outcome.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("apply did not return after the remote write completed")
	}
	if repository.eventCount() != 0 {
		t.Fatalf("cancelled apply recorded %d per-account runtime events", repository.eventCount())
	}
}

func (r *policyAccessRepository) ControlPolicy(context.Context) (map[string]any, error) {
	r.calls++
	return nil, errors.New("automatic scheduling policy unavailable")
}

type listingWriteAdmin struct {
	*concurrentWriteAdmin
	lists int
}

func (a *listingWriteAdmin) Accounts(context.Context) ([]map[string]any, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lists++
	result := make(map[string]any, len(a.state))
	for key, value := range a.state {
		result[key] = value
	}
	return []map[string]any{result}, nil
}

func (r *failingCommitRepository) CommitRoutingReadback(
	context.Context,
	string,
	string,
	business.RoutingReadback,
	*business.RoutingManagedIntent,
	bool,
	business.AccountOperation,
) error {
	return r.err
}

func (a *concurrentWriteAdmin) DeleteAccount(context.Context, string) (map[string]any, error) {
	return a.DeleteAccountWithVerification(context.Background(), "", true)
}

func (a *concurrentWriteAdmin) DeleteAccountWithVerification(_ context.Context, _ string, verification bool) (map[string]any, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.deletes++
	a.deleteChecks = append(a.deleteChecks, verification)
	return map[string]any{"confirmed_absent": true}, nil
}

func (a *concurrentWriteAdmin) Account(context.Context, string) (map[string]any, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.accounts++
	result := make(map[string]any, len(a.state))
	for key, value := range a.state {
		result[key] = value
	}
	return result, nil
}

func (a *concurrentWriteAdmin) Mutate(_ context.Context, method, path string, body map[string]any) (map[string]any, error) {
	database, err := sql.Open("sqlite", "file:"+a.dbPath+"?_pragma=busy_timeout%28100%29")
	if err != nil {
		return nil, err
	}
	_, err = database.Exec(`INSERT INTO app_state(key,value_json,updated_at) VALUES('network-callback','{}','now')
		ON CONFLICT(key) DO UPDATE SET updated_at=excluded.updated_at`)
	_ = database.Close()
	if err != nil {
		return nil, errors.New("database transaction leaked across network: " + err.Error())
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.mutates++
	a.mutations = append(a.mutations, routingMutation{method: method, path: path, body: copyMap(body)})
	if err := a.mutateErrors[path]; err != nil {
		return nil, err
	}
	if a.mutateErr != nil {
		return nil, a.mutateErr
	}
	for key, value := range body {
		switch typed := value.(type) {
		case json.Number:
			number, conversionErr := typed.Int64()
			if conversionErr != nil {
				return nil, conversionErr
			}
			a.state[key] = number
		default:
			a.state[key] = value
		}
	}
	return map[string]any{"success": true}, nil
}

func TestApplyDoesNotHoldDatabaseTransactionAcrossAdminNetwork(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing-write.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	ctx := context.Background()
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = repository.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("41"), "name": "alpha", "schedulable": true,
		"priority": json.Number("20"), "load_factor": json.Number("3"), "concurrency": json.Number("4"),
		"groups": []any{json.Number("7")},
	}}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	admin := &concurrentWriteAdmin{dbPath: path, state: map[string]any{
		"id": json.Number("41"), "schedulable": true, "priority": int64(20), "load_factor": int64(3), "concurrency": int64(4),
	}}
	service := newTestService(repository, admin)
	schedulable, priority, concurrency, load := false, int64(10), int64(6), "5"

	result, err := service.Apply(ctx, map[string]business.AccountRoutingTarget{
		"41": {
			AccountID: "41", GroupNames: []string{"codex"}, DesiredHealth: "degraded",
			Schedulable: &schedulable, Priority: &priority, Concurrency: &concurrency, LoadFactor: &load,
		},
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 0 || result.Changed != 1 || !result.RemoteWrite || admin.mutates != 2 {
		t.Fatalf("unexpected writeback result: %#v mutates=%d", result, admin.mutates)
	}
	if admin.mutations[0].method != "PUT" || admin.mutations[0].path != "/admin/accounts/41" ||
		admin.mutations[1].method != "POST" || admin.mutations[1].path != "/admin/accounts/41/schedulable" {
		t.Fatalf("调度状态和普通字段没有使用各自的接口：%#v", admin.mutations)
	}
	baselines, err := repository.RoutingBaselines(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(baselines) != 1 || baselines[0].Priority == nil || *baselines[0].Priority != 20 || baselines[0].LoadFactor == nil || *baselines[0].LoadFactor != "3" ||
		baselines[0].ManagedSchedulable == nil || *baselines[0].ManagedSchedulable ||
		baselines[0].ManagedPriority == nil || *baselines[0].ManagedPriority != 10 ||
		baselines[0].ManagedLoadFactor == nil || *baselines[0].ManagedLoadFactor != "5" ||
		baselines[0].ManagedConcurrency != nil {
		t.Fatalf("baseline was not captured before remote mutation: %#v", baselines)
	}
	detail, err := repository.Account(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Priority == nil || *detail.Priority != 10 || detail.LoadFactor == nil || *detail.LoadFactor != "5" || detail.Schedulable == nil || *detail.Schedulable {
		t.Fatalf("local projection did not commit verified readback: %#v", detail)
	}
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var fields string
	if err := database.QueryRowContext(ctx, `SELECT field_name FROM operation_audit
		WHERE operation_type='routing.writeback' AND remote_confirmed=1 ORDER BY source_id LIMIT 1`).Scan(&fields); err != nil {
		t.Fatal(err)
	}
	if fields != "load_factor,priority,schedulable" {
		t.Fatalf("写回审计没有记录真实变化字段：%q", fields)
	}
}

func TestApplyPersistsSuccessfulFieldsWhenSchedulableWriteFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing-partial-write.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	ctx := context.Background()
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = repository.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("41"), "name": "alpha", "schedulable": true,
		"priority": json.Number("20"), "load_factor": json.Number("3"), "concurrency": json.Number("4"),
		"groups": []any{json.Number("7")},
	}}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	admin := &concurrentWriteAdmin{
		dbPath: path,
		state: map[string]any{
			"id": json.Number("41"), "schedulable": true, "priority": int64(20), "load_factor": int64(3), "concurrency": int64(4),
		},
		mutateErrors: map[string]error{"/admin/accounts/41/schedulable": errors.New("scheduler unavailable")},
	}
	service := newTestService(repository, admin)
	schedulable, priority := false, int64(10)

	result, err := service.Apply(ctx, map[string]business.AccountRoutingTarget{
		"41": {
			AccountID: "41", GroupNames: []string{"codex"}, DesiredHealth: "fused",
			Schedulable: &schedulable, Priority: &priority,
		},
	}, "scheduler")
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 || !result.RemoteWrite || result.Changed != 1 || len(result.Results) != 1 || !result.Results[0].Changed {
		t.Fatalf("部分成功没有如实返回：%#v", result)
	}
	detail, err := repository.Account(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Priority == nil || *detail.Priority != 10 || detail.Schedulable == nil || !*detail.Schedulable {
		t.Fatalf("成功字段没有按读回值提交：%#v", detail)
	}
	if detail.RoutingState != nil && *detail.RoutingState == "fused" {
		t.Fatalf("可调度状态写回失败却确认了熔断：%#v", detail)
	}
	baselines, err := repository.RoutingBaselines(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(baselines) != 1 || baselines[0].ManagedPriority == nil || *baselines[0].ManagedPriority != 10 ||
		baselines[0].ManagedSchedulable != nil {
		t.Fatalf("部分写回保存了未确认字段的托管意图：%#v", baselines)
	}
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var state, phase, fields string
	var remoteConfirmed, readbackConfirmed int
	if err := database.QueryRowContext(ctx, `SELECT state,phase,field_name,remote_confirmed,readback_confirmed
		FROM operation_audit WHERE operation_type='routing.writeback' ORDER BY source_id DESC LIMIT 1`).Scan(
		&state, &phase, &fields, &remoteConfirmed, &readbackConfirmed,
	); err != nil {
		t.Fatal(err)
	}
	if state != "failed" || phase != "remote-partial" || fields != "priority" || remoteConfirmed != 1 || readbackConfirmed != 1 {
		t.Fatalf("部分写回审计错误：state=%s phase=%s fields=%s remote=%d readback=%d",
			state, phase, fields, remoteConfirmed, readbackConfirmed)
	}
}

func TestWriteRoutingValuesAttemptsSchedulableAfterFieldFailure(t *testing.T) {
	admin := &concurrentWriteAdmin{
		dbPath:       filepath.Join(t.TempDir(), "routing-independent-steps.sqlite3"),
		state:        map[string]any{"schedulable": true, "priority": int64(20)},
		mutateErrors: map[string]error{"/admin/accounts/41": errors.New("field update failed")},
	}
	database, err := sql.Open("sqlite", "file:"+admin.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE app_state(key TEXT PRIMARY KEY,value_json TEXT,updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	_ = database.Close()

	outcome := writeRoutingValues(context.Background(), admin, "41", map[string]any{"priority": int64(10), "schedulable": false})
	if outcome.err == nil || !outcome.remoteConfirmed || len(admin.mutations) != 2 {
		t.Fatalf("两个写回维度没有独立执行：outcome=%#v mutations=%#v", outcome, admin.mutations)
	}
	if admin.state["priority"] != int64(20) || admin.state["schedulable"] != false {
		t.Fatalf("远端部分结果错误：%#v", admin.state)
	}
}

func TestSchedulableWriteFallsBackForLegacySub2API(t *testing.T) {
	admin := &legacySchedulingAdmin{}
	if err := writeSchedulable(context.Background(), admin, "41", false); err != nil {
		t.Fatal(err)
	}
	if len(admin.calls) != 2 || admin.calls[0].method != "POST" || admin.calls[0].path != "/admin/accounts/41/schedulable" ||
		admin.calls[1].method != "PUT" || admin.calls[1].path != "/admin/accounts/41" {
		t.Fatalf("旧版 Sub2API 调度接口回退错误：%#v", admin.calls)
	}
}

func TestApplyRecordsLocalCommitFailureAfterConfirmedRemoteWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing-local-commit.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	ctx := context.Background()
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = repository.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("41"), "name": "alpha", "schedulable": true,
		"priority": json.Number("20"), "load_factor": json.Number("3"), "concurrency": json.Number("4"),
		"groups": []any{json.Number("7")},
	}}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	admin := &concurrentWriteAdmin{dbPath: path, state: map[string]any{
		"id": json.Number("41"), "schedulable": true, "priority": int64(20), "load_factor": int64(3), "concurrency": int64(4),
	}}
	commitFailure := errors.New("local database unavailable")
	service := newTestService(&failingCommitRepository{Repository: repository, err: commitFailure}, admin)
	schedulable := false

	result, err := service.Apply(ctx, map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"codex"}, DesiredHealth: "fused", Schedulable: &schedulable},
	}, "scheduler")
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 1 || !result.RemoteWrite || admin.mutates != 1 {
		t.Fatalf("本地提交失败没有保留远端写入事实：result=%#v mutates=%d", result, admin.mutates)
	}
	baselines, err := repository.RoutingBaselines(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(baselines) != 1 || baselines[0].ManagedSchedulable != nil {
		t.Fatalf("本地事务失败却提前保存了托管意图：%#v", baselines)
	}
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var state, phase, failure string
	var remoteConfirmed, readbackConfirmed, writeback int
	if err := database.QueryRowContext(ctx, `SELECT state,phase,error,remote_confirmed,readback_confirmed,writeback
		FROM operation_audit WHERE operation_type='routing.writeback' ORDER BY created_at DESC,source_id ASC LIMIT 1`).Scan(
		&state, &phase, &failure, &remoteConfirmed, &readbackConfirmed, &writeback,
	); err != nil {
		t.Fatal(err)
	}
	if state != "failed" || phase != "local-commit" || failure != commitFailure.Error() || remoteConfirmed != 1 || readbackConfirmed != 0 || writeback != 1 {
		t.Fatalf("本地提交失败审计语义错误：state=%s phase=%s error=%s remote=%d readback=%d writeback=%d",
			state, phase, failure, remoteConfirmed, readbackConfirmed, writeback)
	}
}

func TestUnconfirmedRemoteAttemptDoesNotBlockOtherAccounts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing-attempt-limit.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	ctx := context.Background()
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = repository.SyncManagementSnapshot(ctx, []map[string]any{
		{
			"id": json.Number("41"), "name": "alpha", "schedulable": true,
			"priority": json.Number("20"), "load_factor": json.Number("3"), "concurrency": json.Number("4"),
			"groups": []any{json.Number("7")},
		},
		{
			"id": json.Number("42"), "name": "beta", "schedulable": true,
			"priority": json.Number("20"), "load_factor": json.Number("3"), "concurrency": json.Number("4"),
			"groups": []any{json.Number("7")},
		},
	}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	admin := &concurrentWriteAdmin{dbPath: path, mutateErr: errors.New("request timeout"), state: map[string]any{
		"schedulable": true, "priority": int64(20), "load_factor": int64(3), "concurrency": int64(4),
	}}
	service := newTestService(repository, admin)
	schedulable := false
	targets := map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"codex"}, DesiredHealth: "fused", Schedulable: &schedulable},
		"42": {AccountID: "42", GroupNames: []string{"codex"}, DesiredHealth: "fused", Schedulable: &schedulable},
	}

	result, err := service.Apply(ctx, targets, "scheduler")
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 2 || !result.RemoteWrite || admin.mutates != 1 || len(result.Results) != 2 {
		t.Fatalf("单账号写回失败错误阻止了其他账号执行：result=%#v mutates=%d", result, admin.mutates)
	}
	if result.Results[1].Skipped || result.Results[1].Error == nil {
		t.Fatalf("后续账号没有独立执行并报告失败：%#v", result.Results[1])
	}
	baselines, err := repository.RoutingBaselines(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(baselines) != 2 {
		t.Fatalf("远程尝试前没有保存可恢复基线：%#v", baselines)
	}
	for _, baseline := range baselines {
		if baseline.ManagedSchedulable != nil || baseline.ManagedPriority != nil || baseline.ManagedLoadFactor != nil || baseline.ManagedConcurrency != nil {
			t.Fatalf("未确认远程写入留下了托管意图：%#v", baseline)
		}
	}
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var attemptedAudits int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM operation_audit WHERE object_id='42'`).Scan(&attemptedAudits); err != nil {
		t.Fatal(err)
	}
	if attemptedAudits != 1 {
		t.Fatalf("后续账号的远程失败没有留下独立审计：count=%d", attemptedAudits)
	}
}

func TestApplyHonorsFieldLevelShadowMode(t *testing.T) {
	policy := map[string]any{
		"auto_apply": map[string]any{"schedulable": true, "priority": false, "load_factor": false, "concurrency": false},
		"weights":    map[string]any{},
	}
	parsed, err := parseWritePolicy(policy)
	if err != nil {
		t.Fatal(err)
	}
	currentPriority := int64(20)
	currentSchedulable := true
	desiredPriority := int64(1)
	desiredSchedulable := false
	desired, err := desiredValues(business.AccountRoutingTarget{
		Priority: &desiredPriority, Schedulable: &desiredSchedulable,
	}, parsed, values{priority: &currentPriority, schedulable: &currentSchedulable})
	if err != nil {
		t.Fatal(err)
	}
	if len(desired) != 1 || desired["schedulable"] != false {
		t.Fatalf("shadow fields leaked into remote payload: %#v", desired)
	}
}

func TestApplyDoesNotConfirmRoutingStateWhenSchedulableIsShadow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing-shadow-state.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	ctx := context.Background()
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.UpdatePolicy(ctx, map[string]any{
		"auto_apply": map[string]any{
			"schedulable": false, "priority": true, "load_factor": false, "concurrency": false,
		},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	_, err = repository.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("41"), "name": "shadow-fuse", "schedulable": true,
		"priority": json.Number("20"), "load_factor": json.Number("3"), "concurrency": json.Number("4"),
		"routing_state": "healthy", "groups": []any{json.Number("7")},
	}}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	admin := &concurrentWriteAdmin{dbPath: path, state: map[string]any{
		"id": json.Number("41"), "schedulable": true, "priority": int64(20), "load_factor": int64(3), "concurrency": int64(4),
	}}
	service := newTestService(repository, admin)
	schedulable, priority := false, int64(10)

	result, err := service.Apply(ctx, map[string]business.AccountRoutingTarget{
		"41": {
			AccountID: "41", GroupNames: []string{"codex"}, DesiredHealth: "fused",
			Schedulable: &schedulable, Priority: &priority,
		},
	}, "scheduler")
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 0 || result.Changed != 1 || admin.mutates != 1 {
		t.Fatalf("优先级写回应成功：result=%#v mutates=%d", result, admin.mutates)
	}
	detail, err := repository.Account(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Schedulable == nil || !*detail.Schedulable || detail.Priority == nil || *detail.Priority != 10 {
		t.Fatalf("远端读回值没有正确保存：%#v", detail)
	}
	if detail.RoutingState == nil || *detail.RoutingState != "healthy" {
		t.Fatalf("仅写回优先级时不应把未执行的熔断目标标成已生效：%#v", detail)
	}
}

func TestApplyConfirmsDegradedStateWhenShadowSchedulableAlreadyMatches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing-shadow-matched-state.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	ctx := context.Background()
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.UpdatePolicy(ctx, map[string]any{
		"auto_apply": map[string]any{
			"schedulable": false, "priority": false, "load_factor": false, "concurrency": false,
		},
	}, "test"); err != nil {
		t.Fatal(err)
	}
	_, err = repository.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("41"), "name": "shadow-degraded", "schedulable": true,
		"priority": json.Number("20"), "load_factor": json.Number("3"), "concurrency": json.Number("4"),
		"routing_state": "healthy", "groups": []any{json.Number("7")},
	}}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	admin := &concurrentWriteAdmin{dbPath: path, state: map[string]any{
		"id": json.Number("41"), "schedulable": true, "priority": int64(20), "load_factor": int64(3), "concurrency": int64(4),
	}}
	service := newTestService(repository, admin)
	schedulable := true

	result, err := service.Apply(ctx, map[string]business.AccountRoutingTarget{
		"41": {
			AccountID: "41", GroupNames: []string{"codex"}, DesiredHealth: "degraded",
			Schedulable: &schedulable,
		},
	}, "scheduler")
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 0 || result.Succeeded != 1 || result.Changed != 0 || admin.mutates != 0 {
		t.Fatalf("远端调度状态已匹配时只应确认本地状态：result=%#v mutates=%d", result, admin.mutates)
	}
	detail, err := repository.Account(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if detail.RoutingState == nil || *detail.RoutingState != "degraded" {
		t.Fatalf("只计算模式没有确认已匹配的降级状态：%#v", detail)
	}
}

func TestFailedRemoteOperationRemainsVisibleAsWriteback(t *testing.T) {
	cause := errors.New("network timeout")
	op := operation("op-1", "routing.writeback", business.AccountRoutingTarget{
		AccountID: "41", GroupNames: []string{"codex"},
	}, "scheduler", nil, nil, false, false, cause)
	if op.State != "failed" || op.Phase != "remote-write" || !op.Writeback || op.RemoteConfirmed || op.ReadbackConfirmed {
		t.Fatalf("失败写回审计语义错误：%#v", op)
	}
}

func TestApplyConfirmsEffectiveRoutingStateWhenRemoteAlreadyMatches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing-readback.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	ctx := context.Background()
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = repository.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("41"), "name": "already-fused", "schedulable": false,
		"priority": json.Number("20"), "load_factor": json.Number("3"), "concurrency": json.Number("4"),
		"groups": []any{json.Number("7")},
	}}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	admin := &concurrentWriteAdmin{dbPath: path, state: map[string]any{
		"id": json.Number("41"), "schedulable": false, "priority": int64(20), "load_factor": int64(3), "concurrency": int64(4),
	}}
	service := newTestService(repository, admin)
	schedulable := false

	result, err := service.Apply(ctx, map[string]business.AccountRoutingTarget{
		"41": {
			AccountID: "41", GroupNames: []string{"codex"}, DesiredHealth: "fused",
			Schedulable: &schedulable,
		},
	}, "scheduler")
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 0 || result.Succeeded != 1 || result.Changed != 0 || admin.mutates != 0 {
		t.Fatalf("远端已符合目标时不应重复写入：result=%#v mutates=%d", result, admin.mutates)
	}
	detail, err := repository.Account(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if detail.RoutingState == nil || *detail.RoutingState != "fused" {
		t.Fatalf("远端读回符合目标后没有确认本地生效状态：%#v", detail)
	}
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var state, phase string
	var remoteConfirmed, readbackConfirmed int
	if err := database.QueryRowContext(ctx, `SELECT state,phase,remote_confirmed,readback_confirmed FROM operation_audit
		WHERE operation_type='routing.writeback' ORDER BY created_at DESC,source_id DESC LIMIT 1`).Scan(
		&state, &phase, &remoteConfirmed, &readbackConfirmed,
	); err != nil {
		t.Fatal(err)
	}
	if state != "succeeded" || phase != "readback" || remoteConfirmed != 0 || readbackConfirmed != 1 {
		t.Fatalf("无写入读回审计语义错误：state=%s phase=%s remote=%d readback=%d", state, phase, remoteConfirmed, readbackConfirmed)
	}
}

func TestRecoveryClearsSub2APIRuntimeBlocksAfterSchedulableWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing-recovery.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	ctx := context.Background()
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = repository.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("41"), "name": "recovering", "schedulable": false,
		"priority": json.Number("20"), "load_factor": json.Number("3"), "concurrency": json.Number("4"),
		"rate_limit_reset_at": "2026-08-28T13:00:00Z", "temp_unschedulable_until": "2026-08-28T13:00:00Z",
		"groups": []any{json.Number("7")},
	}}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	admin := &concurrentWriteAdmin{dbPath: path, state: map[string]any{
		"id": json.Number("41"), "schedulable": false, "priority": int64(20), "load_factor": int64(3), "concurrency": int64(4),
	}}
	service := newTestService(repository, admin)
	schedulable := true
	result, err := service.Apply(ctx, map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"codex"}, DesiredHealth: "healthy", Schedulable: &schedulable},
	}, "scheduler")
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 0 || len(admin.mutations) != 3 {
		t.Fatalf("recovery result=%#v mutations=%#v", result, admin.mutations)
	}
	paths := []string{admin.mutations[0].path, admin.mutations[1].path, admin.mutations[2].path}
	want := []string{"/admin/accounts/41/schedulable", "/admin/accounts/41/clear-error", "/admin/accounts/41/recover-state"}
	for index := range want {
		if paths[index] != want[index] {
			t.Fatalf("recovery paths=%v", paths)
		}
	}
	detail, err := repository.Account(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if _, present := detail.Metadata["rate_limit_reset_at"]; present {
		t.Fatalf("confirmed recovery left local rate-limit block: %#v", detail.Metadata)
	}
	if _, present := detail.Metadata["temp_unschedulable_until"]; present {
		t.Fatalf("confirmed recovery left local temporary block: %#v", detail.Metadata)
	}
}

func TestRestoreControlRejectsBaselinesOwnedByAnotherManagementTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing-restore-target.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	ctx := context.Background()
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("41"), "name": "old-target-account", "priority": json.Number("10"),
		"groups": []any{json.Number("7")},
	}}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test"); err != nil {
		t.Fatal(err)
	}
	oldTarget := configstore.TargetSettings{
		BaseURL: "https://old-target.example", AdminKey: "old-target-key", TimeoutSeconds: 2,
	}
	baselinePriority := int64(20)
	if err := repository.CaptureRoutingBaseline(ctx, business.RoutingBaseline{
		AccountID: "41", Priority: &baselinePriority, TargetFingerprint: routingTargetFingerprint(oldTarget),
	}); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	replacement := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
			"id": 41, "priority": 10,
		}})
	}))
	defer replacement.Close()
	service := New(staticRoutingTarget{value: configstore.TargetSettings{
		BaseURL: replacement.URL, AdminKey: "replacement-key", TimeoutSeconds: 2,
	}}, repository)

	result, err := service.RestoreControl(ctx, "operator")
	if !errors.Is(err, ErrRoutingBaselineTargetChanged) {
		t.Fatalf("restore target error=%v result=%#v", err, result)
	}
	if requests.Load() != 0 || result.RemoteWrite {
		t.Fatalf("old target baseline reached replacement target: requests=%d result=%#v", requests.Load(), result)
	}
}

func TestRestoreControlRevalidatesBaselinesAfterAccountLeaseWait(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing-restore-revalidate.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	repository := &observedRoutingBaselineRepository{Store: store, firstRead: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("41"), "name": "restore-me", "priority": json.Number("10"),
		"groups": []any{json.Number("7")},
	}}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test"); err != nil {
		t.Fatal(err)
	}
	fingerprint := routingTargetFingerprint(testRoutingTarget)
	baselinePriority, firstManagedPriority := int64(20), int64(10)
	if err := repository.CaptureRoutingBaseline(ctx, business.RoutingBaseline{
		AccountID: "41", Priority: &baselinePriority, TargetFingerprint: fingerprint,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.UpdateRoutingManagedIntent(
		ctx, "41", fingerprint, business.RoutingManagedIntent{Priority: &firstManagedPriority},
	); err != nil {
		t.Fatal(err)
	}
	guarded, release, err := mutationguard.Acquire(ctx, repository, mutationguard.Account("41"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = release() }()
	admin := &listingWriteAdmin{concurrentWriteAdmin: &concurrentWriteAdmin{dbPath: path, state: map[string]any{
		"id": json.Number("41"), "priority": int64(10),
	}}}
	service := newTestService(repository, admin)
	type restoreOutcome struct {
		result Result
		err    error
	}
	done := make(chan restoreOutcome, 1)
	go func() {
		result, restoreErr := service.RestoreControl(ctx, "operator")
		done <- restoreOutcome{result: result, err: restoreErr}
	}()
	select {
	case <-repository.firstRead:
	case <-ctx.Done():
		t.Fatalf("restore did not read its initial baseline: %v", ctx.Err())
	}
	updatedManagedPriority := int64(5)
	if err := repository.UpdateRoutingManagedIntent(
		guarded, "41", fingerprint, business.RoutingManagedIntent{Priority: &updatedManagedPriority},
	); err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	select {
	case outcome := <-done:
		if !errors.Is(outcome.err, ErrRoutingBaselineChanged) {
			t.Fatalf("restore revalidation error=%v result=%#v", outcome.err, outcome.result)
		}
		if outcome.result.RemoteWrite {
			t.Fatalf("changed baseline produced a remote write: %#v", outcome.result)
		}
	case <-ctx.Done():
		t.Fatalf("restore did not finish after the account lease was released: %v", ctx.Err())
	}
	admin.mu.Lock()
	defer admin.mu.Unlock()
	if admin.accounts != 0 || admin.mutates != 0 {
		t.Fatalf("changed baseline reached remote admin: accounts=%d mutates=%d", admin.accounts, admin.mutates)
	}
}

func TestRestoreControlClearsConsoleConfirmedRoutingState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing-restore-state.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	ctx := context.Background()
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = repository.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("41"), "name": "restore-me", "schedulable": false,
		"priority": json.Number("10"), "load_factor": json.Number("2"), "concurrency": json.Number("3"),
		"routing_state": "fused", "groups": []any{json.Number("7")},
	}}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	baselineSchedulable, baselinePriority, baselineConcurrency, baselineLoad := true, int64(20), int64(6), "5"
	if err := repository.CaptureRoutingBaseline(ctx, business.RoutingBaseline{
		AccountID: "41", Schedulable: &baselineSchedulable, Priority: &baselinePriority,
		LoadFactor: &baselineLoad, Concurrency: &baselineConcurrency,
		TargetFingerprint: routingTargetFingerprint(testRoutingTarget),
	}); err != nil {
		t.Fatal(err)
	}
	admin := &listingWriteAdmin{concurrentWriteAdmin: &concurrentWriteAdmin{dbPath: path, state: map[string]any{
		"id": json.Number("41"), "schedulable": false, "priority": int64(10), "load_factor": int64(2), "concurrency": int64(3),
	}}}
	policyRepository := &policyAccessRepository{Repository: repository}
	service := newTestService(policyRepository, admin)

	result, err := service.RestoreControl(ctx, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 0 || result.Restored != 1 || result.Changed != 1 || admin.mutates != 2 || admin.lists != 1 || policyRepository.calls != 0 {
		t.Fatalf("交还控制权结果异常：result=%#v mutates=%d", result, admin.mutates)
	}
	detail, err := repository.Account(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if detail.RoutingState == nil || *detail.RoutingState != "" {
		t.Fatalf("交还控制权后仍保留 Console 已确认状态：%#v", detail)
	}
	baselines, err := repository.RoutingBaselines(ctx)
	if err != nil || len(baselines) != 0 {
		t.Fatalf("交还成功后基线未删除：baselines=%#v err=%v", baselines, err)
	}
}

func TestRestoreAccountDoesNotReportProtectedSkipAsRestored(t *testing.T) {
	admin := &legacySchedulingAdmin{}
	service := newTestService(protectedRestoreRepository{}, admin)
	coordinator := newBatchWriteCoordinator(context.Background(), admin, 1, true)

	item := service.restoreAccount(context.Background(), admin, business.RoutingBaseline{
		AccountID: "41", TargetFingerprint: routingTargetFingerprint(testRoutingTarget),
	}, "operator", routingTargetFingerprint(testRoutingTarget), coordinator)

	if !item.Skipped || item.Restored || item.Error != nil {
		t.Fatalf("protected restore result=%#v", item)
	}
	if len(admin.calls) != 0 {
		t.Fatalf("protected restore reached remote admin: %#v", admin.calls)
	}
}

func TestDesiredValuesKeepsSafetyWriteDuringIndependentCooldowns(t *testing.T) {
	policy := writePolicy{
		autoApply: map[string]bool{"schedulable": true, "priority": true, "load_factor": true, "concurrency": true},
	}
	currentSchedulable := true
	currentPriority := int64(20)
	currentConcurrency := int64(4)
	currentLoad := "3"
	wantedSchedulable := false
	wantedPriority := int64(10)
	wantedConcurrency := int64(8)
	wantedLoad := "6"

	desired, err := desiredValues(business.AccountRoutingTarget{
		Schedulable: &wantedSchedulable, Priority: &wantedPriority, LoadFactor: &wantedLoad,
		Concurrency: &wantedConcurrency, WriteCooldown: true, ScalingCooldown: true,
	}, policy, values{
		schedulable: &currentSchedulable, priority: &currentPriority,
		loadFactor: &currentLoad, concurrency: &currentConcurrency,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(desired) != 2 || desired["schedulable"] != false || desired["priority"] != int64(10) {
		t.Fatalf("冷却期只能阻止负载因子和扩容，不能阻止摘流量或优先级：%#v", desired)
	}
}

func TestDeadbandOnlySuppressesLoadFactor(t *testing.T) {
	currentSchedulable := true
	currentPriority, currentConcurrency := int64(100), int64(10)
	currentLoad := "100"
	desired := map[string]any{"priority": int64(101), "load_factor": "105", "concurrency": int64(11), "schedulable": false}
	filtered := applyDeadband(desired, values{
		schedulable: &currentSchedulable, priority: &currentPriority,
		loadFactor: &currentLoad, concurrency: &currentConcurrency,
	}, big.NewRat(1, 10), false)
	if _, present := filtered["load_factor"]; present {
		t.Fatalf("负载因子的小幅变化没有被防抖：%#v", filtered)
	}
	for _, field := range []string{"priority", "concurrency", "schedulable"} {
		if _, present := filtered[field]; !present {
			t.Fatalf("%s 被负载因子防抖错误抑制：%#v", field, filtered)
		}
	}
}

func TestBaselineRestorePreservesExternallyChangedFields(t *testing.T) {
	baseSchedulable, managedSchedulable, currentSchedulable := true, false, false
	basePriority, managedPriority, currentPriority := int64(20), int64(10), int64(15)
	baseLoad, managedLoad, currentLoad := "5", "2", "2"
	desired, conflicts := restorableBaselineValues(business.RoutingBaseline{
		Schedulable: &baseSchedulable, Priority: &basePriority, LoadFactor: &baseLoad,
		OwnershipVersion: 1, ManagedSchedulable: &managedSchedulable,
		ManagedPriority: &managedPriority, ManagedLoadFactor: &managedLoad,
	}, values{schedulable: &currentSchedulable, priority: &currentPriority, loadFactor: &currentLoad})
	if desired["schedulable"] != true || desired["load_factor"] != json.Number("5") {
		t.Fatalf("仍由 Console 持有的字段没有恢复基线：%#v", desired)
	}
	if _, present := desired["priority"]; present {
		t.Fatalf("外部修改后的 priority 被基线覆盖：%#v", desired)
	}
	if len(conflicts) != 1 || conflicts[0] != "priority" {
		t.Fatalf("外部修改冲突没有被准确报告：%v", conflicts)
	}
}

func (s *Service) applyAccount(
	ctx context.Context,
	admin Admin,
	target business.AccountRoutingTarget,
	policy writePolicy,
	actor string,
) AccountResult {
	coordinator := newBatchWriteCoordinator(ctx, admin, 1, policy.verifyAfterWrite)
	return s.applyAccountCoordinated(
		ctx, admin, target, policy, actor, routingTargetFingerprint(testRoutingTarget), coordinator,
	)
}

func TestReleaseControlReportsExternallyChangedFieldsWithoutRemoteWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing-release-conflict.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	ctx := context.Background()
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = repository.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("41"), "name": "external-change", "priority": json.Number("15"),
		"groups": []any{json.Number("7")},
	}}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	baselinePriority, managedPriority := int64(20), int64(10)
	if err := repository.CaptureRoutingBaseline(ctx, business.RoutingBaseline{
		AccountID: "41", Priority: &baselinePriority, OwnershipVersion: 1,
		TargetFingerprint: routingTargetFingerprint(testRoutingTarget),
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.UpdateRoutingManagedIntent(
		ctx, "41", routingTargetFingerprint(testRoutingTarget), business.RoutingManagedIntent{Priority: &managedPriority},
	); err != nil {
		t.Fatal(err)
	}
	admin := &concurrentWriteAdmin{dbPath: path, state: map[string]any{
		"id": json.Number("41"), "priority": int64(15),
	}}
	service := newTestService(repository, admin)
	item := service.applyAccount(ctx, admin, business.AccountRoutingTarget{
		AccountID: "41", GroupNames: []string{"codex"}, DesiredHealth: "excluded", ReleaseControl: true,
	}, writePolicy{autoApply: map[string]bool{}}, "test")
	if item.Error != nil {
		t.Fatalf("交还控制权失败：%s", *item.Error)
	}
	if admin.mutates != 0 || item.RemoteWrite || !item.Restored {
		t.Fatalf("仅保留外部修改时不应写回远端：item=%#v mutates=%d", item, admin.mutates)
	}
	if item.Reason == nil || *item.Reason != "以下字段已被外部修改，交还时保留当前值：priority" {
		t.Fatalf("外部修改说明被通用成功结果覆盖：%#v", item.Reason)
	}
}

func TestAbandonControlKeepsExternalValuesAndDeletesOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing-abandon-external.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	ctx := context.Background()
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = repository.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("41"), "name": "external-change", "schedulable": false,
		"priority": json.Number("15"), "load_factor": json.Number("3"), "concurrency": json.Number("4"),
		"routing_state": "healthy", "groups": []any{json.Number("7")},
	}}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	baselineSchedulable, managedSchedulable := true, true
	if err := repository.CaptureRoutingBaseline(ctx, business.RoutingBaseline{
		AccountID: "41", Schedulable: &baselineSchedulable, OwnershipVersion: 1,
		TargetFingerprint: routingTargetFingerprint(testRoutingTarget),
	}); err != nil {
		t.Fatal(err)
	}
	if err := repository.UpdateRoutingManagedIntent(
		ctx, "41", routingTargetFingerprint(testRoutingTarget), business.RoutingManagedIntent{
			Schedulable: &managedSchedulable,
		},
	); err != nil {
		t.Fatal(err)
	}
	admin := &concurrentWriteAdmin{state: map[string]any{
		"id": json.Number("41"), "schedulable": false, "priority": int64(15), "load_factor": int64(3), "concurrency": int64(4),
	}}
	service := newTestService(repository, admin)
	item := service.applyAccount(ctx, admin, business.AccountRoutingTarget{
		AccountID: "41", GroupNames: []string{"codex"}, DesiredHealth: "external_control", AbandonControl: true,
	}, writePolicy{autoApply: map[string]bool{"schedulable": true}}, "scheduler")
	if item.Error != nil {
		t.Fatalf("停止托管失败：%s", *item.Error)
	}
	if admin.mutates != 0 || item.RemoteWrite || !item.Released {
		t.Fatalf("停止托管不应改写人工值：item=%#v mutates=%d", item, admin.mutates)
	}
	detail, err := repository.Account(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Schedulable == nil || *detail.Schedulable || detail.Priority == nil || *detail.Priority != 15 {
		t.Fatalf("人工值未保留：%#v", detail)
	}
	baselines, err := repository.RoutingBaselines(ctx)
	if err != nil || len(baselines) != 0 {
		t.Fatalf("停止托管后仍保留 Console 所有权：baselines=%#v err=%v", baselines, err)
	}
	routingAccounts, err := repository.RoutingAccounts(ctx, nil, nil)
	if err != nil || len(routingAccounts) != 1 || !routingAccounts[0].ExternalControl {
		t.Fatalf("停止托管状态没有持久化：accounts=%#v err=%v", routingAccounts, err)
	}
	currentSchedulable, currentPriority, currentLoad, currentConcurrency := false, int64(15), "3", int64(4)
	if err := repository.CaptureRoutingBaseline(ctx, business.RoutingBaseline{
		AccountID: "41", Schedulable: &currentSchedulable, Priority: &currentPriority,
		LoadFactor: &currentLoad, Concurrency: &currentConcurrency, OwnershipVersion: 1,
		TargetFingerprint: routingTargetFingerprint(testRoutingTarget),
	}); err != nil {
		t.Fatal(err)
	}
	routingAccounts, err = repository.RoutingAccounts(ctx, nil, nil)
	if err != nil || len(routingAccounts) != 1 || routingAccounts[0].ExternalControl {
		t.Fatalf("重新开启全部托管后没有清除外部控制标记：accounts=%#v err=%v", routingAccounts, err)
	}
}

func TestMatchingTargetStillCapturesNewAccountOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing-new-account-ownership.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	ctx := context.Background()
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = repository.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("41"), "name": "new-account", "schedulable": true,
		"priority": json.Number("20"), "load_factor": json.Number("3"), "concurrency": json.Number("4"),
		"groups": []any{json.Number("7")},
	}}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	admin := &concurrentWriteAdmin{state: map[string]any{
		"id": json.Number("41"), "schedulable": true, "priority": int64(20), "load_factor": int64(3), "concurrency": int64(4),
	}}
	service := newTestService(repository, admin)
	schedulable := true
	item := service.applyAccount(ctx, admin, business.AccountRoutingTarget{
		AccountID: "41", GroupNames: []string{"codex"}, DesiredHealth: "healthy", Schedulable: &schedulable,
	}, writePolicy{autoApply: map[string]bool{"schedulable": true}}, "scheduler")
	if item.Error != nil || admin.mutates != 0 {
		t.Fatalf("已匹配目标不应远程写入：item=%#v mutates=%d", item, admin.mutates)
	}
	baselines, err := repository.RoutingBaselines(ctx)
	if err != nil || len(baselines) != 1 || baselines[0].ManagedSchedulable == nil || !*baselines[0].ManagedSchedulable {
		t.Fatalf("首次纳管没有记录调度所有权：baselines=%#v err=%v", baselines, err)
	}
}

func TestCleanupDeletePredisablesRemoteAndRemovesLocalProjection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cleanup-delete.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	ctx := context.Background()
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = repository.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("41"), "name": "expired", "schedulable": true,
		"priority": json.Number("20"), "load_factor": json.Number("3"), "concurrency": json.Number("4"),
		"groups": []any{json.Number("7")},
	}}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	admin := &concurrentWriteAdmin{dbPath: path, state: map[string]any{
		"id": json.Number("41"), "schedulable": true, "priority": int64(20), "load_factor": int64(3), "concurrency": int64(4),
	}}
	service := newTestService(repository, admin)
	action := "delete"

	result, err := service.Apply(ctx, map[string]business.AccountRoutingTarget{
		"41": {AccountID: "41", GroupNames: []string{"codex"}, DesiredHealth: "fused", CleanupAction: &action},
	}, "scheduler")
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 0 || result.Changed != 1 || admin.mutates != 1 || admin.deletes != 1 || len(admin.deleteChecks) != 1 || !admin.deleteChecks[0] {
		t.Fatalf("删除链路没有完整执行：result=%#v mutates=%d deletes=%d", result, admin.mutates, admin.deletes)
	}
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var accounts int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM accounts WHERE id='41'`).Scan(&accounts); err != nil {
		t.Fatal(err)
	}
	if accounts != 0 {
		t.Fatal("远程删除成功后本地账号投影仍然存在")
	}
	var pending, deleted int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_events WHERE event_type='cleanup_delete_pending'`).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_events WHERE event_type='cleanup_deleted'`).Scan(&deleted); err != nil {
		t.Fatal(err)
	}
	if pending != 1 || deleted != 1 {
		t.Fatalf("删除前后审计不完整：pending=%d deleted=%d", pending, deleted)
	}
}

func TestCleanupPausePersistsLocalPauseWhenAlreadyUnschedulable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cleanup-pause.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	ctx := context.Background()
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = repository.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("41"), "name": "expired", "schedulable": false,
		"priority": json.Number("20"), "load_factor": json.Number("3"), "concurrency": json.Number("4"),
		"groups": []any{json.Number("7")},
	}}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	admin := &concurrentWriteAdmin{dbPath: path, state: map[string]any{
		"id": json.Number("41"), "schedulable": false, "priority": int64(20), "load_factor": int64(3), "concurrency": int64(4),
	}}
	service := newTestService(repository, admin)
	action := "pause"
	schedulable := false

	result, err := service.Apply(ctx, map[string]business.AccountRoutingTarget{
		"41": {
			AccountID: "41", GroupNames: []string{"codex"}, DesiredHealth: "fused",
			Schedulable: &schedulable, CleanupAction: &action,
		},
	}, "scheduler")
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 0 || admin.mutates != 0 {
		t.Fatalf("已摘流量的账号不应重复远端写入：result=%#v mutates=%d", result, admin.mutates)
	}
	detail, err := repository.Account(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Paused == nil || !*detail.Paused || detail.PausedReason == nil {
		t.Fatalf("远端无需变化时本地暂停状态仍应落库：%#v", detail)
	}
}

func TestCleanupDisableRequiresStatusReadback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cleanup-disable.sqlite3")
	repository, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	ctx := context.Background()
	if err := repository.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = repository.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("41"), "name": "expired", "schedulable": true, "status": "active",
		"priority": json.Number("20"), "load_factor": json.Number("3"), "concurrency": json.Number("4"),
		"groups": []any{json.Number("7")},
	}}, []map[string]any{{"id": json.Number("7"), "name": "codex"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	admin := &concurrentWriteAdmin{dbPath: path, state: map[string]any{
		"id": json.Number("41"), "schedulable": true, "status": "active",
		"priority": int64(20), "load_factor": int64(3), "concurrency": int64(4),
	}}
	service := newTestService(repository, admin)
	action := "disable"
	schedulable := false

	result, err := service.Apply(ctx, map[string]business.AccountRoutingTarget{
		"41": {
			AccountID: "41", GroupNames: []string{"codex"}, DesiredHealth: "fused",
			Schedulable: &schedulable, CleanupAction: &action,
		},
	}, "scheduler")
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed != 0 || result.Changed != 1 || admin.mutates != 2 || admin.state["status"] != "inactive" {
		t.Fatalf("停用状态没有通过专用调度接口和普通更新接口完成写入：result=%#v mutates=%d remote=%#v", result, admin.mutates, admin.state)
	}
	detail, err := repository.Account(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Metadata["status"] != "inactive" {
		t.Fatalf("停用状态没有同步到本地投影：%#v", detail.Metadata)
	}
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var disabledEvents int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_events WHERE event_type='cleanup_disabled'`).Scan(&disabledEvents); err != nil {
		t.Fatal(err)
	}
	if disabledEvents != 1 {
		t.Fatalf("停用成功事件必须只记录一次：%d", disabledEvents)
	}
}
