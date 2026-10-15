package taskstore_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestDatabaseContractDetectionConfidenceHistory(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			checkDetectionConfidenceHistory(t, contractTaskStore(t, config))
		})
	}
}

func checkDetectionConfidenceHistory(t *testing.T, store *taskstore.Store) {
	at := "2026-10-10T12:00:00Z"
	for _, task := range []taskstore.Task{
		{ID: "combined", Skill: "sub2api-model-animation", Operation: "managed-model-detection", Status: "partial", Message: "完成", CreatedAt: at, UpdatedAt: at, Result: map[string]any{"animations": []any{
			map[string]any{"account_id": "1", "mode": "animation", "completed_at": at, "status": "succeeded", "manual_verdict": map[string]any{"verdict": "passed"}, "quality": map[string]any{"status": "suspicious"}, "html": "private output"},
			map[string]any{"account_id": "1", "mode": "precheck", "completed_at": at, "status": "succeeded", "precheck": map[string]any{"verdict": "passed", "questions": []any{"private answer"}}},
			map[string]any{"account_id": "3", "request_id": "normal", "mode": "animation", "completed_at": at, "status": "succeeded", "quality": map[string]any{"status": "normal"}},
			map[string]any{"account_id": "4", "request_id": "unknown", "mode": "animation", "completed_at": at, "status": "succeeded", "quality": map[string]any{"status": "unknown"}, "manual_verdict": map[string]any{"verdict": "passed"}},
			map[string]any{"account_id": "5", "request_id": "legacy", "mode": "animation", "completed_at": at, "status": "succeeded", "manual_verdict": map[string]any{"verdict": "passed"}},
		}}},
		{ID: "legacy", Skill: "sub2api-model-animation", Operation: "account-model-precheck", Status: "succeeded", Message: "完成", CreatedAt: at, UpdatedAt: at, Result: map[string]any{"animations": []any{
			map[string]any{"account_id": "2", "completed_at": "2026-10-10T20:00:00+08:00", "status": "succeeded", "precheck": map[string]any{"verdict": "passed"}},
			nil, "invalid", map[string]any{"account_id": 3},
		}}},
		{ID: "bad-container", Skill: "sub2api-model-animation", Operation: "account-model-animation", Status: "succeeded", Message: "完成", CreatedAt: at, UpdatedAt: at, Result: map[string]any{"animations": "not-array"}},
	} {
		if err := store.Save(t.Context(), task); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := store.DetectionConfidenceHistory(t.Context(), time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC))
	if err != nil || len(rows) != 6 {
		t.Fatalf("rows=%+v err=%v", rows, err)
	}
	counts := map[string]int{}
	for _, row := range rows {
		if row.CompletedAt != "2026-10-10T12:00:00.000000000Z" || row.Status != "succeeded" {
			t.Fatalf("row=%+v", row)
		}
		counts[row.AccountID+":"+row.Verdict]++
		if row.AccountID == "3" && (!row.Animation || row.TaskID != "combined" || row.RequestID != "normal") {
			t.Fatalf("lost quality identity: %+v", row)
		}
	}
	if counts["1:passed"] != 1 || counts["1:not_passed"] != 1 || counts["2:passed"] != 1 {
		t.Fatalf("verdicts=%v", counts)
	}
	if counts["3:passed"] != 1 || counts["4:inconclusive"] != 1 || counts["5:inconclusive"] != 1 {
		t.Fatalf("quality mapping=%v", counts)
	}
}
