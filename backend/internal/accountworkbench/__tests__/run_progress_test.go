package accountworkbench_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

type progressStartHookRunner struct {
	*observedRunner
	before func(context.Context)
}

func (r *progressStartHookRunner) GoTask(id string, run func(context.Context)) error {
	return r.observedRunner.GoTask(id, func(ctx context.Context) {
		r.before(ctx)
		run(ctx)
	})
}

func TestRunTaskProgressCountsOnlyCurrentWork(t *testing.T) {
	for _, tc := range []struct {
		name   string
		manual bool
		stop   bool
		states []string
		total  int
	}{
		{name: "entire batch counts every attempted item", states: []string{"failed", "failed", "failed"}, total: 3},
		{name: "retry single item between completed items", states: []string{"exported", "failed", "exported"}, total: 1},
		{name: "retry multiple items before completed tail", states: []string{"failed", "failed", "exported"}, total: 2},
		{name: "manual enable only selected review item", manual: true, states: []string{"review", "review", "completed"}, total: 1},
		{name: "target changed before selected item keeps zero progress", manual: true, stop: true, states: []string{"review", "review", "completed"}, total: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, store := fixture(t, `{}`)
			owner, _ := previewOwner(t, store)
			runner, tasks := runTasks(t, service)
			if tc.stop {
				hooked := &progressStartHookRunner{observedRunner: runner, before: func(ctx context.Context) {
					if err := store.ConfigureTarget(ctx, "https://changed.invalid", "test-admin", 10); err != nil {
						t.Error(err)
					}
				}}
				service.UseExecution(tasks, hooked, nil, t.TempDir())
			}
			var credentials map[string]any
			if err := json.Unmarshal([]byte(signedRunInput(t, service)), &credentials); err != nil {
				t.Fatal(err)
			}
			action, scope := "export", "local-export"
			if tc.manual {
				action, scope = "import", "managed"
			}
			ctx := context.Background()
			target, err := store.TargetSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			id := "33333333333333333333333333333333"
			public := accountworkbench.Run{ID: id, Action: action, Status: "needs_attention", CreatedAt: time.Now().UTC(), ExpiresAt: time.Now().Add(time.Hour)}
			items := []any{}
			phases := map[string]string{}
			for index, status := range tc.states {
				item := accountworkbench.InputItem{ID: fmt.Sprint(index), Index: index, Kind: "codex_json", Email: "run@example.test"}
				public.Items = append(public.Items, accountworkbench.RunItem{InputItem: item, Status: status, AccountID: "41", Check: map[string]any{"verdict": "INCONCLUSIVE"}})
				items = append(items, map[string]any{"item": item, "credentials": credentials})
				phases[item.ID] = "configured"
			}
			raw, err := json.Marshal(map[string]any{"scope": scope, "owner": owner, "target": target, "public": public, "items": items, "exports": make([]any, len(items)), "phases": phases})
			if err != nil {
				t.Fatal(err)
			}
			revision, err := store.SaveWorkbenchDocument(ctx, "run:"+owner+":"+id, 0, raw)
			if err != nil {
				t.Fatal(err)
			}
			confirmation := accountworkbench.RunConfirmation{ID: id, Revision: revision}
			var run accountworkbench.Run
			if tc.manual {
				// A missing account version rejects the attempted enable before a write.
				run, err = service.EnableReview(ctx, owner, confirmation, []string{"0"})
			} else {
				run, err = service.Retry(ctx, owner, confirmation)
			}
			if err != nil {
				t.Fatal(err)
			}
			awaitRun(t, runner)
			task, err := tasks.Get(ctx, run.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			progress := 100
			if tc.stop {
				progress = 0
			}
			if fmt.Sprint(task.Result["total"]) != fmt.Sprint(tc.total) || task.Progress != progress {
				t.Fatalf("current work should have progress %d with total %d: total=%v progress=%d", progress, tc.total, task.Result["total"], task.Progress)
			}
			result, err := service.Run(ctx, owner, id)
			if err != nil {
				t.Fatal(err)
			}
			if tc.manual && result.Items[1].Status != "review" {
				t.Fatal("unselected review item was processed")
			}
			if tc.stop && result.Items[0].Status != "interrupted" {
				t.Fatal("target change did not interrupt the selected item")
			}
			if !tc.manual && result.Status != "completed" {
				t.Fatalf("retry export failed: %+v", result.Items)
			}
		})
	}
}
