package modelcheck_test

import (
	"context"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"net/http"
	"testing"
)

func TestManagedDetectionWithAllAccountsExcludedFinishesWithoutGeneration(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("excluded account contacted") })
	f.catalog.groupMembers = map[string][]string{"7": {"1"}, "8": {"1"}}
	plans, err := f.service.SaveDetectionTask(t.Context(), managedTask(), "test")
	if err != nil {
		t.Fatal(err)
	}
	runner := &deferredRunner{}
	f.service.UseTaskRunner(runner)
	if _, err = f.service.RunDetectionTask(t.Context(), plans[0].ID, plans[0].Version); err != nil {
		t.Fatal(err)
	}
	f.catalog.excluded = map[string]bool{"1": true}
	runner.runs[0](context.Background())
	task := finished(t, f)
	if task.Status != "succeeded" {
		t.Fatalf("excluded detection failed: %+v", task)
	}
}

func TestManagedDetectionExclusionDuringRunDoesNotFailRemainingTask(t *testing.T) {
	var f *fixture
	f = setup(t, 2, "openai", func(w http.ResponseWriter, r *http.Request) {
		f.catalog.mu.Lock()
		f.catalog.excluded = map[string]bool{"2": true}
		f.catalog.mu.Unlock()
		fmt.Fprintf(w, `{"output_text":%q}`, fixtureSVG)
	})
	f.catalog.groupMembers = map[string][]string{"7": {"1", "2"}, "8": {"1", "2"}}
	value := managedTask()
	value.Precheck = false
	value.Concurrency = 1
	plans, err := f.service.SaveDetectionTask(t.Context(), value, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.RunDetectionTask(t.Context(), plans[0].ID, plans[0].Version); err != nil {
		t.Fatal(err)
	}
	task := finished(t, f)
	if task.Status != "succeeded" || task.Result["total"] != 1 {
		t.Fatalf("late exclusion: %+v", task)
	}
	results := task.Result["animations"].([]modelcheck.AnimationResult)
	if len(results) != 1 || results[0].AccountID != "1" {
		t.Fatalf("excluded generated evidence: %+v", results)
	}
}

func TestDirectAnimationExcludedAccountsDoNotFailBatch(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(fmt.Sprint(late), func(t *testing.T) {
			f := setup(t, 2, "openai", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, `{"output_text":%q}`, fixtureSVG) })
			runner := &deferredRunner{}
			f.service.UseTaskRunner(runner)
			if !late {
				f.catalog.excluded = map[string]bool{"1": true}
			}
			if _, err := f.service.EnqueueAnimation(t.Context(), request("1", "2")); err != nil {
				t.Fatal(err)
			}
			f.catalog.excluded = map[string]bool{"1": true}
			runner.runs[0](t.Context())
			task := finished(t, f)
			if task.Status != "succeeded" {
				t.Fatalf("excluded animation: %+v", task)
			}
			results := task.Result["animations"].([]modelcheck.AnimationResult)
			if len(results) != 1 || results[0].AccountID != "2" {
				t.Fatalf("results: %+v", results)
			}
		})
	}
}

func TestDirectBehaviorAllExcludedAfterQueueCompletesWithoutRequest(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("excluded account contacted") })
	runner := &deferredRunner{}
	f.service.UseTaskRunner(runner)
	if _, err := f.service.Enqueue(t.Context(), modelcheck.Request{AccountIDs: []string{"1"}, Models: []string{"gpt-6-astra"}}); err != nil {
		t.Fatal(err)
	}
	f.catalog.excluded = map[string]bool{"1": true}
	runner.runs[0](t.Context())
	if task := finished(t, f); task.Status != "succeeded" || task.Result["total"] != 0 {
		t.Fatalf("excluded behavior: %+v", task)
	}
}

func TestAnimationRecoveryOmitsExcludedAccounts(t *testing.T) {
	f := setup(t, 2, "openai", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, `{"output_text":%q}`, fixtureSVG) })
	runner := &deferredRunner{}
	f.service.UseTaskRunner(runner)
	queued, err := f.service.EnqueueAnimation(t.Context(), request("1", "2"))
	if err != nil {
		t.Fatal(err)
	}
	f.catalog.excluded = map[string]bool{"1": true}
	restarted, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	resumed := &deferredRunner{}
	restarted.UseTaskRunner(resumed)
	pending, err := f.tasks.RecoveryTask(t.Context(), queued.ID)
	if err != nil {
		t.Fatal(err)
	}
	pending.Result["animations"] = []modelcheck.AnimationResult{{AccountID: "1", Model: "test-model", Status: "succeeded", Phase: "completed"}}
	if err := f.tasks.Save(t.Context(), pending); err != nil {
		t.Fatal(err)
	}
	if err := restarted.ResumeTask(t.Context(), pending); err != nil {
		t.Fatal(err)
	}
	resumed.runs[0](t.Context())
	final := finished(t, f)
	if final.Status != "succeeded" {
		t.Fatalf("recovery: %+v", final)
	}
	results := final.Result["animations"].([]modelcheck.AnimationResult)
	if len(results) != 2 || final.Result["total"] != 2 || final.Result["completed"] != 2 {
		t.Fatalf("recovery results: %+v", results)
	}
}
