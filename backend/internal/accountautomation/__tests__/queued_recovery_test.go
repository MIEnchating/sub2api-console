package accountautomation_test

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountautomation"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskcontext"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

// Hold the scheduling boundary so persistent changes can be made before dispatch.
type recoveryRunner struct{ run func(context.Context) }

func (r *recoveryRunner) Go(run func(context.Context)) error               { r.run = run; return nil }
func (r *recoveryRunner) GoTask(_ string, run func(context.Context)) error { return r.Go(run) }
func (*recoveryRunner) CancelTask(string) bool                             { return false }

func queuedAutomation(t testing.TB) (*accountautomation.Service, *taskstore.Store, *recoveryRunner, *accountSource, taskstore.Task) {
	t.Helper()
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "automation.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	pool := &recoveryRunner{}
	accounts := &accountSource{models: []string{"model-a"}}
	service := accountautomation.New(accounts, nil, nil, store, pool, accounts)
	task, err := service.Enqueue(context.Background(), accountautomation.Request{
		GroupIDs: []string{"7"}, PlatformID: "fixture-platform", DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pool.run = nil
	return service, store, pool, accounts, task
}

func TestQueuedAutomationShutdownPreservesLatestProgress(t *testing.T) {
	service, store, pool, accounts, task := queuedAutomation(t)
	if err := service.ResumeTask(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	task.Progress = 70
	task.Result = maps.Clone(task.Result)
	task.Result["completed"] = "latest result"
	if err := store.Save(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(taskcontext.ErrInterrupted)
	pool.run(ctx)
	got, err := store.RecoveryTask(t.Context(), task.ID)
	if err != nil || got.Progress != 70 || got.Result["completed"] != "latest result" {
		t.Fatalf("shutdown replaced queued progress: status=%s progress=%d result=%v err=%v", got.Status, got.Progress, got.Result, err)
	}
	if accounts.previewCalls != 0 || accounts.syncCalls != 0 {
		t.Fatal("shutdown executed account work")
	}
}

func TestQueuedAutomationManualCancellationPreservesLatestResults(t *testing.T) {
	service, store, pool, _, task := queuedAutomation(t)
	if err := service.ResumeTask(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	task.Progress = 70
	task.Result = maps.Clone(task.Result)
	task.Result["completed"] = "latest result"
	if err := store.Save(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	pool.run(ctx)
	got, err := store.Get(t.Context(), task.ID)
	if err != nil || got.Status != "cancelled" || got.Progress != 100 || got.Result["completed"] != "latest result" {
		t.Fatalf("cancellation lost progress: %#v err=%v", got, err)
	}
	if _, err := store.RecoveryTask(t.Context(), task.ID); !errors.Is(err, taskstore.ErrNotFound) {
		t.Fatalf("cancelled checkpoint retained: %v", err)
	}
}

func TestQueuedAutomationChangedCheckpointStopsBeforeModelRead(t *testing.T) {
	service, store, pool, accounts, task := queuedAutomation(t)
	if err := service.ResumeTask(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	var input struct {
		Request accountautomation.Request `json:"request"`
	}
	if err := taskstore.RecoveryInput(task, &input); err != nil {
		t.Fatal(err)
	}
	input.Request.GroupIDs = []string{"8"}
	if err := taskstore.WithRecovery(&task, input); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	pool.run(t.Context())
	got, err := store.Get(t.Context(), task.ID)
	if err != nil || got.Status != "failed" || accounts.previewCalls != 0 || accounts.syncCalls != 0 {
		t.Fatalf("changed checkpoint executed: status=%s reads=%d writes=%d err=%v", got.Status, accounts.previewCalls, accounts.syncCalls, err)
	}
}

func TestQueuedAutomationPersistedCancellationIsNotRewritten(t *testing.T) {
	service, store, pool, accounts, task := queuedAutomation(t)
	if err := service.ResumeTask(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	if ok, err := store.CancelRecovery(t.Context(), task.ID); err != nil || !ok {
		t.Fatalf("cancel=%v err=%v", ok, err)
	}
	want, err := store.Get(t.Context(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	pool.run(t.Context())
	got, err := store.Get(t.Context(), task.ID)
	if err != nil || !reflect.DeepEqual(got, want) || accounts.previewCalls != 0 {
		t.Fatalf("durable cancellation changed: %#v err=%v", got, err)
	}
}

func TestQueuedAutomationNoChannelsPreviewResumesWithLatestStoredResult(t *testing.T) {
	service, store, pool, accounts, task := queuedAutomation(t)
	if err := service.ResumeTask(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	task.Result = maps.Clone(task.Result)
	task.Result["checkpoint_note"] = "saved while queued"
	if err := store.Save(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	pool.run(t.Context())
	got, err := store.Get(t.Context(), task.ID)
	if err != nil || got.Status != "succeeded" || got.Result["checkpoint_note"] != "saved while queued" || got.Result["account_count"] != json.Number("1") || accounts.previewCalls != 1 || accounts.syncCalls != 0 {
		t.Fatalf("preview recovery=%#v reads=%d writes=%d err=%v", got, accounts.previewCalls, accounts.syncCalls, err)
	}
}

func TestNewQueuedAutomationStartsWithLatestStoredResult(t *testing.T) {
	service, store, pool, _, first := queuedAutomation(t)
	if ok, err := store.CancelRecovery(t.Context(), first.ID); err != nil || !ok {
		t.Fatalf("cancel fixture task = %v, %v", ok, err)
	}
	task, err := service.Enqueue(t.Context(), accountautomation.Request{
		GroupIDs: []string{"7"}, PlatformID: "fixture-platform", DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	task.Result = maps.Clone(task.Result)
	task.Result["checkpoint_note"] = "saved while queued"
	if err := store.Save(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	pool.run(t.Context())
	got, err := store.Get(t.Context(), task.ID)
	if err != nil || got.Status != "succeeded" || got.Result["checkpoint_note"] != "saved while queued" {
		t.Fatalf("new queued task lost current result: %#v err=%v", got, err)
	}
}

func TestNewQueuedAutomationRejectsChangedCheckpointBeforeReadingAccounts(t *testing.T) {
	service, store, pool, accounts, first := queuedAutomation(t)
	if ok, err := store.CancelRecovery(t.Context(), first.ID); err != nil || !ok {
		t.Fatalf("cancel fixture task = %v, %v", ok, err)
	}
	task, err := service.Enqueue(t.Context(), accountautomation.Request{
		GroupIDs: []string{"7"}, PlatformID: "fixture-platform", DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var input struct {
		Request accountautomation.Request `json:"request"`
	}
	if err := taskstore.RecoveryInput(task, &input); err != nil {
		t.Fatal(err)
	}
	input.Request.GroupIDs = []string{"8"}
	if err := taskstore.WithRecovery(&task, input); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	pool.run(t.Context())
	got, err := store.Get(t.Context(), task.ID)
	if err != nil || got.Status != "failed" || accounts.previewCalls != 0 || accounts.syncCalls != 0 {
		t.Fatalf("changed checkpoint executed: status=%s reads=%d writes=%d err=%v", got.Status, accounts.previewCalls, accounts.syncCalls, err)
	}
}

func TestQueuedAutomationWriteRecoveryIsRejectedBeforeDispatch(t *testing.T) {
	service, _, pool, accounts, task := queuedAutomation(t)
	var input struct {
		Request accountautomation.Request `json:"request"`
	}
	if err := taskstore.RecoveryInput(task, &input); err != nil {
		t.Fatal(err)
	}
	input.Request.DryRun = false
	if err := taskstore.WithRecovery(&task, input); err != nil {
		t.Fatal(err)
	}
	if err := service.ResumeTask(t.Context(), task); err == nil {
		t.Fatal("write recovery accepted")
	}
	if pool.run != nil || accounts.syncCalls != 0 {
		t.Fatal("write recovery dispatched")
	}
}
