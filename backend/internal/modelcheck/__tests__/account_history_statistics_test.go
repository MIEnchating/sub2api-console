package modelcheck_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestPersistedZeroSuccessfulRequestsRemainInconclusive(t *testing.T) {
	for _, verdict := range []string{"MATCH", "MISMATCH"} {
		t.Run(verdict, func(t *testing.T) {
			f := setup(t, 1, "openai", func(http.ResponseWriter, *http.Request) { t.Error("history must not call upstream") })
			at := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
			task := taskstore.Task{ID: "request-failed", Skill: "sub2api-model-check", Operation: "account-model-behavior-check", Status: "succeeded", Message: "检测结束", CreatedAt: at, UpdatedAt: at, Result: map[string]any{
				"account_ids": []string{"1"}, "tests": []map[string]any{{"account_id": "1", "verdict": verdict, "requests": map[string]any{"successful": 0, "total": 2}}},
			}}
			if err := f.tasks.Store.Save(context.Background(), task); err != nil {
				t.Fatal(err)
			}
			statuses, err := f.service.AccountStatuses(context.Background())
			if err != nil || len(statuses) != 1 {
				t.Fatalf("statuses=%+v err=%v", statuses, err)
			}
			status := statuses[0]
			if status.Status != "inconclusive" || status.Confidence.Short.Samples != 1 || status.Confidence.Short.Inconclusive != 1 || status.Confidence.Short.Score != nil {
				t.Fatalf("zero successful requests became model evidence: %+v", status)
			}
		})
	}
}

func TestMixedAccountHistoryKeepsVerdictsAndSampleCountsSeparate(t *testing.T) {
	f := setup(t, 0, "openai", func(http.ResponseWriter, *http.Request) { t.Error("history must not call upstream") })
	at := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	task := taskstore.Task{ID: "mixed", Skill: "sub2api-model-check", Operation: "account-model-behavior-check", Status: "succeeded", Message: "检测结束", CreatedAt: at, UpdatedAt: at, Result: map[string]any{
		"account_ids": []string{"1", "1", "2", "3", "4"},
		"tests": []any{
			map[string]any{"account_id": "1", "verdict": "MATCH"},
			map[string]any{"account_id": "1", "verdict": "GROUP_MATCH"},
			map[string]any{"account_id": "2", "verdict": "MATCH"},
			map[string]any{"account_id": "2", "verdict": "MISMATCH"},
			map[string]any{"account_id": "3", "verdict": "SOL_CONSISTENT"},
			map[string]any{"account_id": "3", "verdict": "ERROR"},
			map[string]any{"account_id": "5", "verdict": "TERRA_LIKE"},
			map[string]any{"account_id": 1, "verdict": "MISMATCH"},
			"legacy-invalid-row",
		},
	}}
	if err := f.tasks.Store.Save(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	statuses, err := f.service.AccountStatuses(context.Background())
	if err != nil || len(statuses) != 5 {
		t.Fatalf("statuses=%+v err=%v", statuses, err)
	}
	for _, status := range statuses {
		want := map[string]struct {
			status                       string
			passed, failed, inconclusive int
		}{
			"1": {"consistent", 2, 0, 0},
			"2": {"inconsistent", 1, 1, 0},
			"3": {"inconclusive", 1, 0, 1},
			"4": {"inconclusive", 0, 0, 0},
			"5": {"inconsistent", 0, 1, 0},
		}[status.AccountID]
		window := status.Confidence.Short
		if status.Status != want.status || window.Passed != want.passed || window.Failed != want.failed || window.Inconclusive != want.inconclusive || window.Samples != want.passed+want.failed+want.inconclusive {
			t.Fatalf("account history was duplicated or mixed across accounts: %+v", status)
		}
	}
}
