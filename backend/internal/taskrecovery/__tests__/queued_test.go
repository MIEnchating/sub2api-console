package taskrecovery_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskcontext"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrecovery"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

type waitingRunner struct {
	run func(context.Context)
	err error
}

func (r *waitingRunner) Go(run func(context.Context)) error               { r.run = run; return r.err }
func (r *waitingRunner) GoTask(_ string, run func(context.Context)) error { return r.Go(run) }
func (*waitingRunner) CancelTask(string) bool                             { return false }

func TestQueuedRecoveryReadsLatestResultsAndReleasesReservation(t *testing.T) {
	store, refs := pendingTask(t)
	task, err := store.RecoveryTask(t.Context(), refs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	pool := &waitingRunner{}
	released, called := 0, 0
	if err := taskrecovery.Enqueue(pool, store, task, func() { released++ }, func(_ context.Context, current taskstore.Task) {
		called++
		if current.Result["completed"] != json.Number("2") || current.Progress != 70 {
			t.Errorf("stale snapshot: %#v", current)
		}
	}); err != nil {
		t.Fatal(err)
	}
	task.Result["completed"], task.Progress = 2, 70
	if err := store.Save(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	pool.run(t.Context())
	if released != 1 || called != 1 {
		t.Fatalf("called=%d released=%d", called, released)
	}
}

func TestQueuedRecoveryCancellationPreservesResultsAndSkipsExecution(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		cause   error
		durable bool
		want    string
	}{
		{"shutdown", taskcontext.ErrInterrupted, false, "queued"},
		{"manual worker cancellation", context.Canceled, false, "cancelled"},
		{"persisted cancellation", context.Canceled, true, "cancelled"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store, refs := pendingTask(t)
			task, err := store.RecoveryTask(t.Context(), refs[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			pool := &waitingRunner{}
			released := 0
			if err := taskrecovery.Enqueue(pool, store, task, func() { released++ }, func(context.Context, taskstore.Task) { t.Error("cancelled task executed") }); err != nil {
				t.Fatal(err)
			}
			if scenario.durable {
				if _, err := store.CancelRecovery(t.Context(), task.ID); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithCancelCause(t.Context())
			cancel(scenario.cause)
			pool.run(ctx)
			got, err := store.Get(t.Context(), task.ID)
			if err != nil || got.Status != scenario.want || got.Result["completed"] != json.Number("1") || released != 1 {
				t.Fatalf("status=%s results=%v released=%d err=%v", got.Status, got.Result, released, err)
			}
			_, err = store.RecoveryTask(t.Context(), task.ID)
			if scenario.want == "queued" && err != nil {
				t.Fatal(err)
			}
			if scenario.want == "cancelled" && !errors.Is(err, taskstore.ErrNotFound) {
				t.Fatalf("cancelled checkpoint retained: %v", err)
			}
		})
	}
}

func TestQueuedRecoveryRejectsChangedOrUnavailableCheckpointAndReleasesReservation(t *testing.T) {
	for _, scenario := range []struct{ name, statement, status string }{
		{"changed input", `UPDATE task_recovery SET payload='{"version":1,"input":["42"]}'`, "failed"},
		{"corrupt input", `UPDATE task_recovery SET payload='invalid'`, "failed"},
		{"removed checkpoint", `DELETE FROM task_recovery`, "queued"},
		{"already running", `UPDATE tasks SET status='running'`, "running"},
		{"completed task", `UPDATE tasks SET status='succeeded'`, "succeeded"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "queued.db")
			store, err := taskstore.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			task := taskstore.Task{ID: "queued", Skill: "test", Operation: "test-recovery", Status: "queued", Message: "等待恢复", CreatedAt: "2026-10-01T00:00:00Z", UpdatedAt: "2026-10-01T00:00:00Z", Result: map[string]any{"completed": 1}}
			if err := taskstore.WithRecovery(&task, []string{"41"}); err != nil {
				t.Fatal(err)
			}
			if err := store.Save(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			pool := &waitingRunner{}
			released := 0
			if err := taskrecovery.Enqueue(pool, store, task, func() { released++ }, func(context.Context, taskstore.Task) { t.Error("invalid task executed") }); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", sqliteutil.DSN(path, ""))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.ExecContext(t.Context(), scenario.statement); err != nil {
				t.Fatal(err)
			}
			pool.run(t.Context())
			got, err := store.Get(t.Context(), task.ID)
			if err != nil || got.Status != scenario.status || got.Result["completed"] != json.Number("1") || released != 1 {
				t.Fatalf("state=%#v released=%d err=%v", got, released, err)
			}
		})
	}
}

func TestQueuedRecoveryCapacityErrorLeavesReservationWithCaller(t *testing.T) {
	store, refs := pendingTask(t)
	task, err := store.RecoveryTask(t.Context(), refs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	pool := &waitingRunner{err: taskrunner.ErrCapacity}
	err = taskrecovery.Enqueue(pool, store, task, func() { t.Error("unaccepted reservation released twice") }, func(context.Context, taskstore.Task) { t.Error("unaccepted task executed") })
	if !errors.Is(err, taskrunner.ErrCapacity) {
		t.Fatal(err)
	}
}

func TestQueuedRecoveryReadFailureReleasesReservationWithoutReplacingStoredResults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unavailable.db")
	store, err := taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	task := taskstore.Task{ID: "queued", Skill: "test", Operation: "test-recovery", Status: "queued", Progress: 40, Message: "等待恢复", CreatedAt: "2026-10-01T00:00:00Z", UpdatedAt: "2026-10-01T00:00:00Z", Result: map[string]any{"completed": 1}}
	if err := taskstore.WithRecovery(&task, []string{"41"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	pool := &waitingRunner{}
	released := 0
	if err := taskrecovery.Enqueue(pool, store, task, func() { released++ }, func(context.Context, taskstore.Task) { t.Error("unreadable task executed") }); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	pool.run(t.Context())
	reopened, err := taskstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.RecoveryTask(t.Context(), task.ID)
	if err != nil || got.Progress != 40 || got.Result["completed"] != json.Number("1") || released != 1 {
		t.Fatalf("read failure lost checkpoint: %#v released=%d err=%v", got, released, err)
	}
}

func TestQueuedRecoveryRealWorkerCancellationReleasesWaitingSlot(t *testing.T) {
	store, refs := pendingTask(t)
	task, err := store.RecoveryTask(t.Context(), refs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	pool := taskrunner.NewQueued(t.Context(), 1, 1)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := pool.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	started := make(chan struct{})
	if err := pool.GoTask("blocker", func(ctx context.Context) { close(started); <-ctx.Done() }); err != nil {
		t.Fatal(err)
	}
	<-started
	released := make(chan struct{})
	if err := taskrecovery.Enqueue(pool, store, task, func() { close(released) }, func(context.Context, taskstore.Task) { t.Error("cancelled waiting task executed") }); err != nil {
		t.Fatal(err)
	}
	if pool.Snapshot().Waiting != 1 {
		t.Fatal("task did not wait for capacity")
	}
	if !pool.CancelTask(task.ID) {
		t.Fatal("waiting task could not be cancelled")
	}
	select {
	case <-released:
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled waiting task kept reservation")
	}
	got, err := store.Get(t.Context(), task.ID)
	if err != nil || got.Status != "cancelled" || got.Result["completed"] != json.Number("1") {
		t.Fatalf("cancellation lost result: %#v, %v", got, err)
	}
}
