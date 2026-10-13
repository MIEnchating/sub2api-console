package modelcheck_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestAccountConfidenceIncludesPrecheckAndMarkedAnimationWithoutChangingRegularStatus(t *testing.T) {
	f := setup(t, 2, "openai", nil)
	ctx := context.Background()
	now := time.Now().UTC()
	for _, item := range []struct {
		id, accountID, mode, status, verdict string
		age                                  time.Duration
	}{
		{"precheck-pass", "1", "precheck", "succeeded", "passed", time.Hour},
		{"precheck-fail", "1", "precheck", "succeeded", "not_passed", 48 * time.Hour},
		{"animation-pass", "1", "animation", "succeeded", "passed", time.Hour},
		{"animation-unmarked", "1", "animation", "succeeded", "", time.Hour},
		{"other-account", "2", "precheck", "succeeded", "not_passed", time.Hour},
		{"expired", "1", "precheck", "succeeded", "passed", 31 * 24 * time.Hour},
	} {
		at := now.Add(-item.age).Format(time.RFC3339Nano)
		row := map[string]any{"account_id": item.accountID, "mode": item.mode, "status": item.status, "completed_at": at}
		if item.mode == "precheck" {
			row["precheck"] = map[string]any{"verdict": item.verdict}
		} else if item.verdict != "" {
			row["manual_verdict"] = map[string]any{"verdict": item.verdict}
		}
		task := taskstore.Task{ID: item.id, Skill: "sub2api-model-animation", Operation: "account-model-combined", Status: "succeeded", Message: "检测完成", CreatedAt: at, UpdatedAt: at, Result: map[string]any{"animations": []any{row}}}
		if err := f.tasks.Store.Save(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	statuses, err := f.service.AccountStatuses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 2 {
		t.Fatalf("statuses=%+v", statuses)
	}
	for _, status := range statuses {
		if status.Status != "not_checked" || status.TaskID != "" {
			t.Fatalf("detection history became a regular check: %+v", status)
		}
		switch status.AccountID {
		case "1":
			if status.Confidence.Short.Passed != 2 || status.Confidence.Short.Inconclusive != 1 || status.Confidence.Long.Failed != 1 || status.Confidence.Long.Samples != 4 {
				t.Fatalf("account 1 confidence=%+v", status.Confidence)
			}
		case "2":
			if status.Confidence.Short.Failed != 1 || status.Confidence.Short.Score == nil || *status.Confidence.Short.Score != 0 {
				t.Fatalf("account 2 confidence=%+v", status.Confidence)
			}
		default:
			t.Fatalf("unexpected account %+v", status)
		}
	}
}

func TestDetectionConfidenceExcludesUnfinishedCustomAndInvalidRows(t *testing.T) {
	for _, tc := range []struct {
		name, accountID, taskStatus, resultStatus, completed, verdict string
		wantSample                                                    bool
	}{
		{"request failure with stale mark", "1", "failed", "failed", "recent", "passed", true},
		{"unmarked", "1", "succeeded", "succeeded", "recent", "unmarked", true},
		{"inconclusive", "1", "succeeded", "succeeded", "recent", "inconclusive", true},
		{"active task", "1", "running", "succeeded", "recent", "passed", false},
		{"active result", "1", "cancelled", "running", "recent", "passed", false},
		{"custom endpoint", "custom", "succeeded", "succeeded", "recent", "passed", false},
		{"invalid time", "1", "succeeded", "succeeded", "invalid", "passed", false},
		{"future time", "1", "succeeded", "succeeded", "future", "passed", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := setup(t, 1, "openai", nil)
			at := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
			completed := at
			if tc.completed == "invalid" {
				completed = "invalid"
			}
			if tc.completed == "future" {
				completed = time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
			}
			row := map[string]any{"account_id": tc.accountID, "mode": "precheck", "status": tc.resultStatus, "completed_at": completed, "precheck": map[string]any{"verdict": tc.verdict}}
			task := taskstore.Task{ID: "check", Skill: "sub2api-model-animation", Operation: "account-model-precheck", Status: tc.taskStatus, Message: "检测", CreatedAt: at, UpdatedAt: at, Result: map[string]any{"animations": []any{"broken", nil, row}}}
			if err := f.tasks.Store.Save(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			statuses, err := f.service.AccountStatuses(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if !tc.wantSample {
				if len(statuses) != 0 {
					t.Fatalf("invalid sample included: %+v", statuses)
				}
				return
			}
			if len(statuses) != 1 || statuses[0].Confidence.Short.Samples != 1 || statuses[0].Confidence.Short.Inconclusive != 1 || statuses[0].Confidence.Short.Score != nil {
				t.Fatalf("inconclusive result affected score: %+v", statuses)
			}
		})
	}
}

func TestDetectionConfidenceRetainsAllHistoryAndMergesRegularChecks(t *testing.T) {
	f := setup(t, 1, "openai", nil)
	at := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	for i := range 105 {
		task := taskstore.Task{ID: fmt.Sprintf("precheck-%d", i), Skill: "sub2api-model-animation", Operation: "managed-model-detection", Status: "succeeded", Message: "完成", CreatedAt: at, UpdatedAt: at, Result: map[string]any{"animations": []any{map[string]any{"account_id": "1", "mode": "precheck", "status": "succeeded", "completed_at": at, "precheck": map[string]any{"verdict": "passed"}}}}}
		if err := f.tasks.Store.Save(t.Context(), task); err != nil {
			t.Fatal(err)
		}
	}
	regular := taskstore.Task{ID: "regular", Skill: "sub2api-model-check", Operation: "account-model-behavior-check", Status: "succeeded", Message: "完成", CreatedAt: at, UpdatedAt: at, Result: map[string]any{"tests": []any{map[string]any{"account_id": "1", "verdict": "MISMATCH"}}}}
	if err := f.tasks.Store.Save(t.Context(), regular); err != nil {
		t.Fatal(err)
	}
	statuses, err := f.service.AccountStatuses(t.Context())
	if err != nil || len(statuses) != 1 {
		t.Fatalf("statuses=%+v err=%v", statuses, err)
	}
	status := statuses[0]
	if status.Status != "inconsistent" || status.TaskID != "regular" || status.Confidence.Short.Passed != 105 || status.Confidence.Short.Failed != 1 || status.Confidence.Short.Samples != 106 {
		t.Fatalf("history truncated or regular result changed: %+v", status)
	}
}

func TestAnimationMarkChangesRecomputeConfidenceWithoutMovingCompletionWindow(t *testing.T) {
	f := setup(t, 1, "openai", nil)
	at := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	task := taskstore.Task{ID: "animation", Skill: "sub2api-model-animation", Operation: "account-model-animation", Status: "succeeded", Message: "完成", CreatedAt: at, UpdatedAt: at, Result: map[string]any{"animations": []any{map[string]any{"account_id": "1", "request_id": "request-1", "status": "succeeded", "completed_at": at}}}}
	if err := f.tasks.Store.Save(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	for _, verdict := range []string{"passed", "not_passed", "unmarked"} {
		if _, err := f.service.SetAnimationManualVerdict(t.Context(), task.ID, modelcheck.AnimationManualVerdictRequest{AccountID: "1", RequestID: "request-1", Verdict: verdict}, "console"); err != nil {
			t.Fatal(err)
		}
		statuses, err := f.service.AccountStatuses(t.Context())
		if err != nil || len(statuses) != 1 {
			t.Fatalf("statuses=%+v err=%v", statuses, err)
		}
		stats := statuses[0].Confidence
		if stats.Short.Samples != 0 || stats.Long.Samples != 1 {
			t.Fatalf("mark moved or duplicated sample: %+v", stats)
		}
		switch verdict {
		case "passed":
			if stats.Long.Passed != 1 {
				t.Fatalf("mark not counted: %+v", stats)
			}
		case "not_passed":
			if stats.Long.Failed != 1 {
				t.Fatalf("mark not changed: %+v", stats)
			}
		case "unmarked":
			if stats.Long.Inconclusive != 1 || stats.Long.Score != nil {
				t.Fatalf("cleared mark retained score: %+v", stats)
			}
		}
	}
}
