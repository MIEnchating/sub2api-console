package uptimekuma_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskrunner"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/uptimekuma"
)

// Observe actual temporary SQLite persistence; only the injected storage failure
// bypasses a write. No production endpoint or credentials are used.
type observedKumaTasks struct {
	store       *taskstore.Store
	terminal    chan taskstore.Task
	failRunning bool
}

func (s *observedKumaTasks) Save(ctx context.Context, task taskstore.Task) error {
	if s.failRunning && task.Status == "running" {
		return errors.New("isolated task database write failure")
	}
	if err := s.store.Save(ctx, task); err != nil {
		return err
	}
	if task.Status == "succeeded" || task.Status == "failed" || task.Status == "cancelled" {
		stored, err := s.store.Get(ctx, task.ID)
		if err != nil {
			return err
		}
		s.terminal <- stored
	}
	return nil
}

type kumaTaskFixture struct {
	ctx     context.Context
	config  *configstore.Store
	tasks   *observedKumaTasks
	runner  *taskrunner.Group
	service *uptimekuma.TaskService
	server  *httptest.Server
}

func newKumaTaskFixture(t *testing.T, handler http.Handler) *kumaTaskFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	config, err := configstore.Open(filepath.Join(t.TempDir(), "kuma-config.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = config.Close() })
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "kuma-tasks.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	runner := taskrunner.NewQueued(context.Background(), 1, 1)
	t.Cleanup(func() {
		shutdown, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := runner.Shutdown(shutdown); err != nil {
			t.Errorf("runner shutdown: %v", err)
		}
	})
	tasks := &observedKumaTasks{store: store, terminal: make(chan taskstore.Task, 4)}
	service := uptimekuma.NewTasks(uptimekuma.New(config, server.Client()), tasks, runner)
	return &kumaTaskFixture{ctx, config, tasks, runner, service, server}
}

func (f *kumaTaskFixture) enqueueConfig(t *testing.T) taskstore.Task {
	t.Helper()
	task, err := f.service.Save(f.ctx, uptimekuma.ConfigInput{BaseURL: f.server.URL, APIKey: "private-test-key"})
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func (f *kumaTaskFixture) completed(t *testing.T) taskstore.Task {
	t.Helper()
	select {
	case task := <-f.tasks.terminal:
		return task
	case <-f.ctx.Done():
		t.Fatal("task did not persist terminal state")
		return taskstore.Task{}
	}
}

func awaitKumaEvent(t *testing.T, ctx context.Context, event <-chan struct{}) {
	t.Helper()
	select {
	case <-event:
	case <-ctx.Done():
		t.Fatal("controlled Kuma request did not reach expected state")
	}
}

func blockedKumaMetrics(entered, stopped chan struct{}) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics" {
			http.NotFound(w, r)
			return
		}
		close(entered)
		<-r.Context().Done()
		close(stopped)
	})
}

func TestKumaTaskStartsAtZeroWhileVerificationIsPending(t *testing.T) {
	entered, stopped := make(chan struct{}), make(chan struct{})
	f := newKumaTaskFixture(t, blockedKumaMetrics(entered, stopped))
	queued := f.enqueueConfig(t)
	awaitKumaEvent(t, f.ctx, entered)
	running, err := f.tasks.store.Get(f.ctx, queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	if queued.Progress != 0 || running.Status != "running" || running.Progress != 0 {
		t.Fatalf("uncompleted operation reports progress: queued=%d running=%s/%d", queued.Progress, running.Status, running.Progress)
	}
}

func TestKumaTaskCancellationStopsPendingVerificationAndPreservesManualIntent(t *testing.T) {
	entered, stopped := make(chan struct{}), make(chan struct{})
	f := newKumaTaskFixture(t, blockedKumaMetrics(entered, stopped))
	queued := f.enqueueConfig(t)
	awaitKumaEvent(t, f.ctx, entered)
	// The API persists cancellation before signalling the owning task runner.
	accepted, err := f.tasks.store.CancelTask(f.ctx, queued.ID)
	if err != nil || !accepted {
		t.Fatalf("persist cancellation: accepted=%v err=%v", accepted, err)
	}
	if !f.runner.CancelTask(queued.ID) {
		t.Fatal("running Kuma task is not registered for cancellation")
	}
	awaitKumaEvent(t, f.ctx, stopped)
	final := f.completed(t)
	if final.Status != "cancelled" || final.Result["cancelled"] != true || final.Result["phase"] != "cancelled" || final.Result["interrupted"] != nil {
		t.Fatalf("manual cancellation lost: %#v", final)
	}
	stored, err := f.config.UptimeKuma(f.ctx)
	if err != nil || stored.APIKey != "" {
		t.Fatalf("cancelled verification saved credentials: %v", err)
	}
	encoded, err := json.Marshal(final)
	if err != nil || strings.Contains(string(encoded), "private-test-key") {
		t.Fatal("cancelled task leaked credentials")
	}
}

func TestKumaTaskShutdownIsInterruptedInsteadOfUserCancellation(t *testing.T) {
	entered, stopped := make(chan struct{}), make(chan struct{})
	f := newKumaTaskFixture(t, blockedKumaMetrics(entered, stopped))
	f.enqueueConfig(t)
	awaitKumaEvent(t, f.ctx, entered)
	f.runner.Cancel()
	awaitKumaEvent(t, f.ctx, stopped)
	final := f.completed(t)
	if final.Status != "failed" || final.Result["interrupted"] != true || final.Result["cancelled"] != nil || final.Progress != 0 {
		t.Fatalf("shutdown classified incorrectly: %#v", final)
	}
	if final.Recovery != nil || final.Result["phase"] != "interrupted" || !strings.Contains(final.Message, "核对远端状态") {
		t.Fatalf("unsafe recovery or retry guidance: %#v", final)
	}
	if _, err := f.tasks.store.RecoveryTask(f.ctx, final.ID); !errors.Is(err, taskstore.ErrNotFound) {
		t.Fatalf("interrupted write retained a recovery checkpoint: %v", err)
	}
}

func TestKumaQueuedCancellationFinishesWithoutStartingRemoteRequest(t *testing.T) {
	var calls atomic.Int32
	f := newKumaTaskFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	occupied := make(chan struct{})
	if err := f.runner.GoTask("occupy-worker", func(ctx context.Context) {
		close(occupied)
		<-ctx.Done()
	}); err != nil {
		t.Fatal(err)
	}
	awaitKumaEvent(t, f.ctx, occupied)
	queued := f.enqueueConfig(t)
	if !f.runner.CancelTask(queued.ID) {
		t.Fatal("queued Kuma task is not registered for cancellation")
	}
	final := f.completed(t)
	if final.Status != "cancelled" || final.Result["remote_write"] != false || final.Result["phase"] != "cancelled" || calls.Load() != 0 {
		t.Fatalf("queued cancellation performed work: task=%#v calls=%d", final, calls.Load())
	}
}

func TestKumaProgressStorageFailurePreventsRemoteRequest(t *testing.T) {
	var calls atomic.Int32
	f := newKumaTaskFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	f.tasks.failRunning = true
	f.enqueueConfig(t)
	final := f.completed(t)
	if final.Status != "failed" || final.Progress != 100 || final.Result["phase"] != "complete" || calls.Load() != 0 {
		t.Fatalf("storage failure did not prevent work: task=%#v calls=%d", final, calls.Load())
	}
}

type unkeyedKumaRunner struct {
	called bool
}

func (r *unkeyedKumaRunner) Go(func(context.Context)) error {
	r.called = true
	return nil
}

func TestKumaTaskRejectsMissingOrUncancellableRunner(t *testing.T) {
	for _, name := range []string{"missing", "unkeyed"} {
		t.Run(name, func(t *testing.T) {
			f := newKumaTaskFixture(t, http.NotFoundHandler())
			unkeyed := &unkeyedKumaRunner{}
			var runner taskrunner.Runner
			if name == "unkeyed" {
				runner = unkeyed
			}
			service := uptimekuma.NewTasks(f.service.Service, f.tasks, runner)
			_, err := service.Save(f.ctx, uptimekuma.ConfigInput{BaseURL: f.server.URL, APIKey: "private-test-key"})
			if err == nil || uptimekuma.PublicError(err).Code != "kuma_task_unavailable" || unkeyed.called {
				t.Fatalf("uncancellable execution accepted: err=%v dispatched=%v", err, unkeyed.called)
			}
			if name == "unkeyed" && f.completed(t).Status != "failed" {
				t.Fatal("rejected task did not persist failure")
			}
		})
	}
}
