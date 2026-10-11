package accountautomation_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountautomation"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type heldGroupChild struct {
	id  string
	run func(context.Context)
}
type heldGroupChildRunner struct{ queued chan heldGroupChild }

func (*heldGroupChildRunner) Go(func(context.Context)) error { panic("task identity required") }
func (r *heldGroupChildRunner) GoTask(id string, run func(context.Context)) error {
	r.queued <- heldGroupChild{id, run}
	return nil
}
func (*heldGroupChildRunner) CancelTask(string) bool { return false }

type cancellationObservedStore struct {
	*taskstore.Store
	cancelled chan string
}

func (s *cancellationObservedStore) CancelTask(ctx context.Context, id string) (bool, error) {
	changed, err := s.Store.CancelTask(ctx, id)
	if changed && err == nil {
		s.cancelled <- id
	}
	return changed, err
}

func TestTaskGroupCancellationPersistsQueuedChildWithoutDispatchAndSurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.db")
	db, err := taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := &cancellationObservedStore{Store: db, cancelled: make(chan string, 4)}
	pool := &heldGroupChildRunner{queued: make(chan heldGroupChild, 4)}
	groups := taskrunner.New(t.Context())
	defer groups.Shutdown(context.Background())
	accounts := &accountSource{models: []string{"model-a"}}
	service := accountautomation.New(accounts, nil, nil, store, pool, accounts)
	defer service.StopSchedules()
	service.UseScheduleRepository(&taskGroupSettings{})
	service.UseGroupRunner(groups)
	request := accountautomation.Request{GroupIDs: []string{"7"}, PlatformID: "fixture", DryRun: true}
	first := saveTaskGroupSchedule(t, service, request, "第一项")
	second := saveTaskGroupSchedule(t, service, request, "第二项")
	group, err := service.SaveTaskGroup(t.Context(), accountautomation.TaskGroup{Name: "取消后不恢复", ScheduleIDs: []string{first.ID, second.ID}})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := service.RunTaskGroup(t.Context(), group.ID)
	if err != nil {
		t.Fatal(err)
	}
	var child heldGroupChild
	select {
	case child = <-pool.queued:
	case <-time.After(5 * time.Second):
		t.Fatal("child was not queued")
	}
	queued, err := store.RecoveryTask(t.Context(), child.id)
	if err != nil {
		t.Fatal(err)
	}
	var owner struct {
		TaskGroupTaskID string `json:"task_group_task_id"`
	}
	if err := taskstore.RecoveryInput(queued, &owner); err != nil || owner.TaskGroupTaskID != parent.ID {
		t.Fatalf("queued child lost durable parent identity: %q %v", owner.TaskGroupTaskID, err)
	}
	if changed, err := db.CancelTask(t.Context(), parent.ID); err != nil || !changed {
		t.Fatalf("parent cancellation: %v %v", changed, err)
	}
	groups.CancelTask(parent.ID)
	select {
	case id := <-store.cancelled:
		if id != child.id {
			t.Fatalf("cancelled unrelated task %s", id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("queued child cancellation was not persisted")
	}
	service.StopSchedules()
	if err := groups.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, err := reopened.RecoverInterrupted(t.Context()); err != nil {
		t.Fatal(err)
	}
	pending, err := reopened.PendingRecovery(t.Context())
	if err != nil || len(pending) != 0 {
		t.Fatalf("cancelled child recovered: %v %v", pending, err)
	}
	got, err := reopened.Get(t.Context(), child.id)
	if err != nil || got.Status != "cancelled" || accounts.previewCalls != 0 || accounts.syncCalls != 0 {
		t.Fatalf("queued child state=%s reads=%d writes=%d err=%v", got.Status, accounts.previewCalls, accounts.syncCalls, err)
	}
}

func TestCancelledTaskGroupPreventsChildRecoveryBeforeAndAfterQueueing(t *testing.T) {
	for _, cancelAfterQueue := range []bool{false, true} {
		name := "before-dispatch"
		if cancelAfterQueue {
			name = "while-queued"
		}
		t.Run(name, func(t *testing.T) {
			service, store, pool, accounts, child := queuedAutomation(t)
			parent := taskstore.Task{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Skill: accountautomation.Skill, Operation: accountautomation.GroupOperation, Status: "running", Message: "等待子任务", Result: map[string]any{}, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
			if err := store.Save(t.Context(), parent); err != nil {
				t.Fatal(err)
			}
			var input struct {
				Request         accountautomation.Request `json:"request"`
				TaskGroupTaskID string                    `json:"task_group_task_id,omitempty"`
			}
			if err := taskstore.RecoveryInput(child, &input); err != nil {
				t.Fatal(err)
			}
			input.TaskGroupTaskID = parent.ID
			if err := taskstore.WithRecovery(&child, input); err != nil {
				t.Fatal(err)
			}
			if err := store.Save(t.Context(), child); err != nil {
				t.Fatal(err)
			}
			if cancelAfterQueue {
				if err := service.ResumeTask(t.Context(), child); err != nil {
					t.Fatal(err)
				}
			}
			// This is the committed parent state at a crash before the worker receives cancellation.
			parent.Status = "cancelled"
			parent.Result["cancelled"] = true
			if err := store.Save(t.Context(), parent); err != nil {
				t.Fatal(err)
			}
			if !cancelAfterQueue {
				if err := service.ResumeTask(t.Context(), child); err != nil {
					t.Fatal(err)
				}
			}
			if pool.run != nil {
				pool.run(t.Context())
			}
			got, err := store.Get(t.Context(), child.ID)
			if err != nil || got.Status != "cancelled" || accounts.previewCalls != 0 || accounts.syncCalls != 0 {
				t.Fatalf("cancelled group child resumed: status=%s reads=%d writes=%d err=%v", got.Status, accounts.previewCalls, accounts.syncCalls, err)
			}
			pending, err := store.PendingRecovery(t.Context())
			if err != nil || len(pending) != 0 {
				t.Fatalf("cancelled child retained recovery: %v %v", pending, err)
			}
		})
	}
}

func TestTaskGroupChildRecoveryRejectsMissingStoppedOrMismatchedParent(t *testing.T) {
	for _, fixture := range []struct{ name, status, operation, skill string }{
		{"missing", "", accountautomation.GroupOperation, accountautomation.Skill},
		{"stopped", "failed", accountautomation.GroupOperation, accountautomation.Skill},
		{"completed", "succeeded", accountautomation.GroupOperation, accountautomation.Skill},
		{"wrong-operation", "running", "fixture", accountautomation.Skill},
		{"wrong-skill", "running", accountautomation.GroupOperation, "fixture"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			service, store, pool, accounts, child := queuedAutomation(t)
			parent := taskstore.Task{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Skill: fixture.skill, Operation: fixture.operation, Status: fixture.status, Message: "父任务", Result: map[string]any{}, CreatedAt: child.CreatedAt, UpdatedAt: child.UpdatedAt}
			if fixture.status != "" {
				if err := store.Save(t.Context(), parent); err != nil {
					t.Fatal(err)
				}
			}
			var input struct {
				Request         accountautomation.Request `json:"request"`
				TaskGroupTaskID string                    `json:"task_group_task_id,omitempty"`
			}
			if err := taskstore.RecoveryInput(child, &input); err != nil {
				t.Fatal(err)
			}
			input.TaskGroupTaskID = parent.ID
			if err := taskstore.WithRecovery(&child, input); err != nil {
				t.Fatal(err)
			}
			if err := store.Save(t.Context(), child); err != nil {
				t.Fatal(err)
			}
			if err := service.ResumeTask(t.Context(), child); err == nil {
				t.Fatal("orphaned or mismatched child accepted")
			}
			if pool.run != nil || accounts.previewCalls != 0 || accounts.syncCalls != 0 {
				t.Fatal("rejected child dispatched account work")
			}
		})
	}
}

func TestLegacyScheduledPreviewWithoutOwnerIdentityRequiresExplicitRerun(t *testing.T) {
	service, store, pool, accounts, child := queuedAutomation(t)
	var legacy struct {
		Request accountautomation.Request `json:"request"`
	}
	if err := taskstore.RecoveryInput(child, &legacy); err != nil {
		t.Fatal(err)
	}
	if err := taskstore.WithRecovery(&child, legacy); err != nil {
		t.Fatal(err)
	}
	child.Result["schedule_id"] = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := store.Save(t.Context(), child); err != nil {
		t.Fatal(err)
	}
	if err := service.ResumeTask(t.Context(), child); err == nil {
		t.Fatal("legacy scheduled preview resumed without known group ownership")
	}
	if pool.run != nil || accounts.previewCalls != 0 {
		t.Fatal("unknown ownership dispatched account reads")
	}
}

func TestScheduledPreviewWithExplicitStandaloneOwnershipRemainsRecoverable(t *testing.T) {
	service, store, pool, accounts, child := queuedAutomation(t)
	child.Result["schedule_id"] = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := store.Save(t.Context(), child); err != nil {
		t.Fatal(err)
	}
	if err := service.ResumeTask(t.Context(), child); err != nil {
		t.Fatal(err)
	}
	if pool.run == nil {
		t.Fatal("standalone preview was not dispatched")
	}
	pool.run(t.Context())
	got, err := store.Get(t.Context(), child.ID)
	if err != nil || got.Status != "succeeded" || accounts.previewCalls != 1 || accounts.syncCalls != 0 {
		t.Fatalf("standalone preview recovery: %s reads=%d writes=%d err=%v", got.Status, accounts.previewCalls, accounts.syncCalls, err)
	}
}

func TestCancelledTaskGroupMarksInterruptedWriteCancelledWithoutReplay(t *testing.T) {
	service, store, pool, accounts, child := queuedAutomation(t)
	parent := taskstore.Task{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Skill: accountautomation.Skill, Operation: accountautomation.GroupOperation, Status: "cancelled", Message: "手动取消", Result: map[string]any{"cancelled": true}, CreatedAt: child.CreatedAt, UpdatedAt: child.UpdatedAt}
	if err := store.Save(t.Context(), parent); err != nil {
		t.Fatal(err)
	}
	var input struct {
		Request         accountautomation.Request `json:"request"`
		TaskGroupTaskID string                    `json:"task_group_task_id"`
	}
	if err := taskstore.RecoveryInput(child, &input); err != nil {
		t.Fatal(err)
	}
	input.Request.DryRun = false
	input.TaskGroupTaskID = parent.ID
	if err := taskstore.WithRecovery(&child, input); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), child); err != nil {
		t.Fatal(err)
	}
	if err := service.ResumeTask(t.Context(), child); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(t.Context(), child.ID)
	if err != nil || got.Status != "cancelled" || pool.run != nil || accounts.syncCalls != 0 {
		t.Fatalf("manual cancellation classified as failure or replayed: %s dispatched=%v writes=%d err=%v", got.Status, pool.run != nil, accounts.syncCalls, err)
	}
}
