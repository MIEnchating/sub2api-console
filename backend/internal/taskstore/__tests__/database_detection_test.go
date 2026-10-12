package taskstore_test

import (
	"encoding/json"
	"fmt"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"testing"
	"time"
)

func TestDatabaseContractAnimationHistoryRanksNanosecondsBeyondRecentPage(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store := contractTaskStore(t, config)
			fixtures := []taskstore.Task{
				historyTimeTask("newest", "41", "2026-10-08T00:00:00Z", "2026-10-08T01:05:00.123456790+01:00"),
				historyTimeTask("older", "41", "2026-10-08T00:01:00Z", "2026-10-08T00:05:00.123456789Z"),
				historyTimeTask("attempt", "41", "2026-10-08T00:02:00Z", ""),
			}
			for i := range 21 {
				fixtures = append(fixtures, historyTimeTask(fmt.Sprintf("unrelated-%02d", i), "42", "2026-10-08T01:00:00Z", "2026-10-08T01:00:00Z"))
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
				if len(task.Result) != 0 {
					t.Fatal("history contains private payload")
				}
			}
			if !ids["newest"] || !ids["attempt"] || ids["older"] {
				t.Fatalf("wrong ranking: %v", ids)
			}
		})
	}
}

func TestDatabaseContractModelHistoryKeepsPublicVerdictAndExactRequests(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store := contractTaskStore(t, config)
			task := taskstore.Task{ID: "verdict", Skill: "sub2api-model-check", Operation: "account-model-behavior-check", Status: "succeeded", Message: "完成", CreatedAt: "2026-10-01T00:00:00Z", UpdatedAt: "2026-10-01T00:00:00Z", Result: map[string]any{"tests": []any{nil, 42, map[string]any{"account_id": "41", "verdict": "match", "requests": map[string]any{"total": json.Number("9007199254740993")}, "secret": "private"}}}}
			if err := store.Save(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			history, err := store.ModelCheckHistory(t.Context(), time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC))
			if err != nil || len(history) != 1 {
				t.Fatalf("history: %v %v", history, err)
			}
			results := history[0].Result["tests"].([]any)
			if len(results) != 1 {
				t.Fatalf("malformed entries retained: %v", results)
			}
			verdict := results[0].(map[string]any)
			if verdict["secret"] != nil || verdict["requests"].(map[string]any)["total"] != json.Number("9007199254740993") {
				t.Fatalf("projection: %v", verdict)
			}
		})
	}
}
