package taskstore_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestAnimationHistoryKeepsLatestCompletionAtNanosecondPrecision(t *testing.T) {
	for _, tc := range []struct{ name, older, newer string }{
		{"nanoseconds", "2026-10-08T00:05:00.123456789Z", "2026-10-08T00:05:00.123456790Z"},
		{"fraction widths", "2026-10-08T00:05:00.12Z", "2026-10-08T00:05:00.120000001Z"},
		{"positive offset", "2026-10-08T00:05:00.123456789Z", "2026-10-08T01:05:00.123456790+01:00"},
		{"negative offset", "2026-10-08T00:05:00.123456789Z", "2026-10-07T19:05:00.123456790-05:00"},
		{"second boundary", "2026-10-08T00:05:00.999999999Z", "2026-10-08T00:05:01.000000001Z"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, err := taskstore.Open(filepath.Join(t.TempDir(), "history.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			for _, task := range []taskstore.Task{
				historyTimeTask("latest-result", "1", "2026-10-08T00:00:00Z", tc.newer),
				historyTimeTask("older-result", "1", "2026-10-08T00:01:00Z", tc.older),
				historyTimeTask("latest-attempt", "1", "2026-10-08T00:02:00Z", ""),
			} {
				if err := store.Save(t.Context(), task); err != nil {
					t.Fatal(err)
				}
			}
			// Move these results outside the recent-20 fallback so only ranking
			// can retain the actual latest completion, independent of its start.
			for i := range 21 {
				if err := store.Save(t.Context(), historyTimeTask(fmt.Sprintf("unrelated-%02d", i), "2", "2026-10-08T01:00:00Z", "2026-10-08T01:00:00Z")); err != nil {
					t.Fatal(err)
				}
			}
			history, err := store.AnimationHistory(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			ids := map[string]bool{}
			for _, task := range history {
				ids[task.ID] = true
			}
			if !ids["latest-result"] || !ids["latest-attempt"] || ids["older-result"] {
				t.Fatalf("completion ranking retained wrong history: %v", ids)
			}
		})
	}
}

func historyTimeTask(id, accountID, createdAt, completedAt string) taskstore.Task {
	result := map[string]any{"account_ids": []string{accountID}}
	if completedAt != "" {
		result["animations"] = []any{map[string]any{
			"account_id": accountID, "mode": "animation", "status": "succeeded", "completed_at": completedAt,
		}}
	}
	return taskstore.Task{ID: id, Skill: "sub2api-model-animation", Operation: "account-model-animation", Status: "succeeded", Progress: 100, Message: "完成", CreatedAt: createdAt, UpdatedAt: createdAt, Result: result}
}

func TestAnimationHistoryMalformedCompletionDoesNotHideValidResult(t *testing.T) {
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	fixtures := []taskstore.Task{
		historyTimeTask("valid", "1", "2026-10-08T00:00:00Z", "2026-10-08T00:00:00.123456789Z"),
		historyTimeTask("latest-attempt", "1", "2026-10-08T00:02:00Z", ""),
	}
	for i, value := range []any{nil, 123, "not-a-time", "2026-99-01T00:00:00Z"} {
		task := historyTimeTask(fmt.Sprintf("invalid-%d", i), "1", "2026-10-08T00:01:00Z", "placeholder")
		task.Result["animations"].([]any)[0].(map[string]any)["completed_at"] = value
		fixtures = append(fixtures, task)
	}
	for i := range 21 {
		fixtures = append(fixtures, historyTimeTask(fmt.Sprintf("unrelated-%02d", i), "2", "2026-10-08T01:00:00Z", "2026-10-08T01:00:00Z"))
	}
	for _, task := range fixtures {
		if err := store.Save(t.Context(), task); err != nil {
			t.Fatal(err)
		}
	}
	history, err := store.AnimationHistory(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, task := range history {
		ids[task.ID] = true
	}
	if !ids["valid"] || !ids["latest-attempt"] {
		t.Fatalf("valid history lost: %v", ids)
	}
	for i := range 4 {
		if ids[fmt.Sprintf("invalid-%d", i)] {
			t.Fatalf("malformed completion selected: %v", ids)
		}
	}
}
