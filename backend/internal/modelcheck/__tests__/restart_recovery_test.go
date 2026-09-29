package modelcheck_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskcontext"
)

func TestManagedDetectionRestartKeepsCompletedAccountAndOriginalTaskID(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	var calls atomic.Int32
	f := setup(t, 2, "openai", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != "test-model" {
			t.Errorf("recovery overwrote requested model: %q", body.Model)
		}
		if calls.Add(1) == 2 {
			cancel(taskcontext.ErrInterrupted)
			return
		}
		fmt.Fprintf(w, `{"output_text":%q}`, fixtureSVG)
	})
	f.catalog.groupMembers = map[string][]string{"7": {"1", "2"}, "8": {}}
	value := managedTask()
	value.Precheck = false
	value.Terminal = false
	plans, err := f.service.SaveDetectionTask(context.Background(), value, "test")
	if err != nil {
		t.Fatal(err)
	}
	runner := &deferredRunner{}
	f.service.UseTaskRunner(runner)
	task, err := f.service.RunDetectionTask(context.Background(), plans[0].ID, plans[0].Version)
	if err != nil {
		t.Fatal(err)
	}
	runner.runs[0](ctx)
	stored, err := f.tasks.Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != "queued" || len(stored.Result["animations"].([]any)) != 1 {
		t.Fatalf("interruption lost completed stage: %#v", stored)
	}
	restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	resumed := &deferredRunner{}
	restarted.UseTaskRunner(resumed)
	pending, err := f.tasks.PendingRecovery(context.Background())
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending: %#v %v", pending, err)
	}
	if err := restarted.ResumeTask(context.Background(), pending[0]); err != nil {
		t.Fatal(err)
	}
	resumed.runs[0](context.Background())
	final := finished(t, f)
	if final.ID != task.ID || final.Status != "succeeded" || calls.Load() != 3 {
		t.Fatalf("restart repeated completed work: %s %s calls=%d", final.ID, final.Status, calls.Load())
	}
	if len(final.Result["animations"].([]modelcheck.AnimationResult)) != 2 {
		t.Fatal("final result missing an account")
	}
}

func TestTerminalRestartContinuesAtFirstUnfinishedRound(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	var calls atomic.Int32
	f := setup(t, 1, "openai", func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 2 {
			cancel(taskcontext.ErrInterrupted)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"output_text": `{"tool":"exec_command","command":"git status --short"}`})
	})
	runner := &deferredRunner{}
	f.service.UseTaskRunner(runner)
	task, err := f.service.EnqueueTerminalContinuity(context.Background(), modelcheck.TerminalContinuityRequest{Targets: request("1").Targets, TimeoutSeconds: 5, Rounds: 3})
	if err != nil {
		t.Fatal(err)
	}
	runner.runs[0](ctx)
	pending, err := f.tasks.PendingRecovery(context.Background())
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending: %#v %v", pending, err)
	}
	restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	resumed := &deferredRunner{}
	restarted.UseTaskRunner(resumed)
	if err := restarted.ResumeTask(context.Background(), pending[0]); err != nil {
		t.Fatal(err)
	}
	resumed.runs[0](context.Background())
	final := finished(t, f)
	if final.ID != task.ID || final.Status != "succeeded" || calls.Load() != 4 {
		t.Fatalf("completed terminal round replayed: %s calls=%d", final.Status, calls.Load())
	}
	checks := final.Result["checks"].([]modelcheck.TerminalContinuityResult)
	if len(checks) != 1 || len(checks[0].RoundResults) != 3 {
		t.Fatalf("rounds lost: %#v", checks)
	}
}

func TestDetectionConfigurationChangeRefusesOldTaskRecovery(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("unexpected upstream request") })
	f.catalog.groupMembers = map[string][]string{"7": {"1"}, "8": {}}
	plans, err := f.service.SaveDetectionTask(context.Background(), managedTask(), "test")
	if err != nil {
		t.Fatal(err)
	}
	runner := &deferredRunner{}
	f.service.UseTaskRunner(runner)
	_, err = f.service.RunDetectionTask(context.Background(), plans[0].ID, plans[0].Version)
	if err != nil {
		t.Fatal(err)
	}
	// Crash before dispatch, then change the persisted configuration in a new process.
	restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	value := plans[0].DetectionTask
	value.Name = "已修改"
	if _, err := restarted.SaveDetectionTask(context.Background(), value, "test"); err != nil {
		t.Fatal(err)
	}
	pending, err := f.tasks.PendingRecovery(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.ResumeTask(context.Background(), pending[0]); err == nil {
		t.Fatal("changed configuration was resumed")
	}
}

func TestCustomAnimationNeverPersistsCredentialRecovery(t *testing.T) {
	f := setup(t, 0, "openai", func(http.ResponseWriter, *http.Request) { t.Error("unexpected upstream request") })
	runner := &deferredRunner{}
	f.service.UseTaskRunner(runner)
	task, err := f.service.EnqueueAnimation(context.Background(), modelcheck.AnimationRequest{Custom: &modelcheck.AnimationCustomEndpoint{BaseURL: "https://example.invalid", Platform: "openai", Model: "gpt-6-astra", APIKey: "sk-secret-memory-only"}, TimeoutSeconds: 5})
	if err != nil {
		t.Fatal(err)
	}
	if task.Recovery != nil {
		t.Fatal("custom credential persisted")
	}
	pending, err := f.tasks.PendingRecovery(context.Background())
	if err != nil || len(pending) != 0 {
		t.Fatalf("custom recovery: %#v %v", pending, err)
	}
}
