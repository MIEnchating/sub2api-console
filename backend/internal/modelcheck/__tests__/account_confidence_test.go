package modelcheck_test

import (
	"context"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"net/http"
	"testing"
	"time"
)

func TestAccountConfidenceIncludesHistoryBeyond100TasksAndIsolatesAccountIDs(t *testing.T) {
	f := setup(t, 2, "openai", func(http.ResponseWriter, *http.Request) { t.Error("statistics must not call upstream") })
	now := time.Now().UTC()
	for i := 0; i < 105; i++ {
		at := now.Add(-time.Duration(i+1) * time.Minute).Format(time.RFC3339Nano)
		task := taskstore.Task{ID: fmt.Sprintf("history-%d", i), Skill: "sub2api-model-check", Operation: "account-model-behavior-check", Status: "succeeded", Progress: 100, Message: "test", CreatedAt: at, UpdatedAt: at,
			Result: map[string]any{"account_ids": []string{"1"}, "tests": []map[string]any{{"account_id": "1", "claimed_model": "gpt-5.6-sol", "verdict": "SOL_CONSISTENT", "requests": map[string]any{"successful": 2, "total": 2}}}}}
		if err := f.tasks.Store.Save(context.Background(), task); err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []struct {
		id, skill, verdict string
		age                time.Duration
	}{
		{"2", "sub2api-model-check", "MISMATCH", 48 * time.Hour},
		{"3", "sub2api-model-check", "MATCH", 31 * 24 * time.Hour},
		{"1", "sub2api-model-animation", "MISMATCH", time.Hour},
		{"4", "sub2api-model-check", "ERROR", time.Hour},
	} {
		at := now.Add(-row.age).Format(time.RFC3339Nano)
		err := f.tasks.Store.Save(context.Background(), taskstore.Task{ID: "extra-" + row.id, Skill: row.skill, Operation: "test", Status: "succeeded", Progress: 100, Message: "test", CreatedAt: at, UpdatedAt: at, Result: map[string]any{"account_ids": []string{row.id}, "tests": []map[string]any{{"account_id": row.id, "verdict": row.verdict}}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	statuses, err := f.service.AccountStatuses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 4 {
		t.Fatalf("statuses: %+v", statuses)
	}
	for _, status := range statuses {
		switch status.AccountID {
		case "1":
			if status.Confidence.Short.Samples != 105 || status.Confidence.Short.Passed != 105 {
				t.Fatalf("truncated confidence: %+v", status)
			}
		case "2":
			if status.Confidence.Short.Score != nil || status.Confidence.Long.Failed != 1 {
				t.Fatalf("wrong window: %+v", status)
			}
		case "3":
			if status.TaskID != "extra-3" || status.Confidence.Long.Samples != 0 {
				t.Fatalf("old result: %+v", status)
			}
		case "4":
			if status.Confidence.Short.Score != nil || status.Confidence.Short.Inconclusive != 1 {
				t.Fatalf("request failure classified as mismatch: %+v", status)
			}
		}
	}
}
