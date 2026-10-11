package modelcheck_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestRemovedTerminalDetectionRejectsNewRequestsWithoutDispatch(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("removed detection called upstream") })
	runner := &deferredRunner{}
	f.service.UseTaskRunner(runner)
	for _, rounds := range []int{0, 1, 20, 21} {
		_, err := f.service.EnqueueTerminalContinuity(context.Background(), modelcheck.TerminalContinuityRequest{Targets: request("1").Targets, TimeoutSeconds: 5, Rounds: rounds})
		if err == nil || !strings.Contains(err.Error(), "已移除") {
			t.Fatalf("removed detection accepted: %v", err)
		}
	}
	if len(runner.runs) != 0 {
		t.Fatal("removed detection dispatched")
	}
	history, err := f.service.TerminalContinuityHistory(context.Background())
	if err != nil || len(history) != 0 {
		t.Fatalf("unexpected terminal history: %v %v", history, err)
	}
}

func TestRemovedTerminalDetectionRefusesHistoricalTaskRecovery(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("recovery called upstream") })
	for _, managed := range []bool{false, true} {
		task := taskstore.Task{ID: "old", Operation: "account-terminal-continuity"}
		if managed {
			task.Operation = "managed-model-detection"
			value := managedTask()
			value.Terminal = true
			if err := taskstore.WithRecovery(&task, struct {
				Configuration *modelcheck.DetectionTask `json:"configuration"`
			}{&value}); err != nil {
				t.Fatal(err)
			}
		}
		if err := f.service.ResumeTask(context.Background(), task); err == nil || !strings.Contains(err.Error(), "已移除") {
			t.Fatalf("old terminal task resumed: %v", err)
		}
	}
}

func TestLegacyTerminalOnlyPlanLoadsDisabledAndCannotRun(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("legacy plan called upstream") })
	f.catalog.plans = []byte(`[{"id":"legacy","version":3,"name":"旧终端任务","group_ids":["7"],"model":"test-model","animation":false,"precheck":false,"terminal":true,"terminal_rounds":3,"automatic":true,"schedule_type":"interval","interval_minutes":60,"timeout_seconds":5}]`)
	service, err := modelcheck.New(f.tasks, credentials{}, f.catalog, credentials{})
	if err != nil {
		t.Fatal(err)
	}
	plan := service.DetectionTasks()[0]
	if plan.Terminal || plan.Automatic || plan.TerminalRounds != 0 || plan.NextAt != "" || plan.Version != 3 {
		t.Fatalf("legacy plan not disabled: %+v", plan)
	}
	runner := &deferredRunner{}
	service.UseTaskRunner(runner)
	service.RunDueDetectionTasks(context.Background(), time.Now().Add(24*time.Hour))
	if _, err := service.RunDetectionTask(context.Background(), plan.ID, plan.Version); err == nil {
		t.Fatal("empty legacy plan ran")
	}
	if len(runner.runs) != 0 {
		t.Fatal("empty plan dispatched")
	}
}

func TestDetectionTaskRejectsRemovedTerminalStageWithoutSaving(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("save called upstream") })
	value := managedTask()
	value.Terminal = true
	if _, err := f.service.SaveDetectionTask(context.Background(), value, "test"); err == nil || !strings.Contains(err.Error(), "已移除") {
		t.Fatalf("terminal stage saved: %v", err)
	}
	if len(f.catalog.plans) != 0 {
		t.Fatal("invalid plan persisted")
	}
}

func TestRemovedTerminalHistoryRetainsOriginalResults(t *testing.T) {
	f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("history called upstream") })
	task := taskstore.Task{ID: "old-terminal", Skill: "sub2api-terminal-continuity", Operation: "account-terminal-continuity", Status: "succeeded", Message: "历史检测完成", CreatedAt: "2026-09-23T00:00:00Z", UpdatedAt: "2026-09-23T00:00:00Z", Result: map[string]any{"checks": []modelcheck.TerminalContinuityResult{{AccountID: "1", Verdict: "normal", Response: "original answer"}}}}
	if err := f.tasks.Save(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	history, err := f.service.TerminalContinuityHistory(context.Background())
	if err != nil || len(history) != 1 || history[0].ID != task.ID {
		t.Fatalf("history lost: %v %v", history, err)
	}
	checks := history[0].Result["checks"].([]any)
	if checks[0].(map[string]any)["response"] != "original answer" {
		t.Fatal("historical result changed")
	}
}
