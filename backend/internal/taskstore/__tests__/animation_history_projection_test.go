package taskstore_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestAnimationHistoryRankingIgnoresMalformedFieldsAndPreservesBothStages(t *testing.T) {
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	animation := historyTimeTask("animation", "41", "2026-10-08T00:00:00Z", "2026-10-08T00:00:00Z")
	payload := strings.Repeat("preserved-html-", 4096)
	animation.Result["animations"].([]any)[0].(map[string]any)["html"] = payload
	precheck := historyTimeTask("precheck", "41", "2026-10-08T00:01:00Z", "2026-10-08T00:01:00Z")
	precheck.Operation = "account-model-combined"
	row := precheck.Result["animations"].([]any)[0].(map[string]any)
	delete(row, "mode")
	row["precheck"] = map[string]any{"verdict": "passed", "questions": []any{}}
	invalid := historyTimeTask("invalid", "41", "2026-10-08T00:02:00Z", "")
	invalid.Result["animations"] = []any{
		nil, 42, "malformed", []any{"41", "animation"},
		map[string]any{"account_id": 41, "status": "succeeded", "completed_at": "2026-10-08T00:03:00Z"},
		map[string]any{"account_id": "41", "status": []any{"succeeded"}, "completed_at": "2026-10-08T00:03:00Z"},
		map[string]any{"account_id": "41", "status": "succeeded", "completed_at": map[string]any{"time": "2026-10-08T00:03:00Z"}},
	}
	latestAttempt := historyTimeTask("attempt", "41", "2026-10-08T00:04:00Z", "")
	latestAttempt.Operation = "account-model-combined"
	latestAttempt.Status = "cancelled"
	fixtures := []taskstore.Task{animation, precheck, invalid, latestAttempt}
	// Ranking must retain both stages beyond the recent-task fallback.
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
			t.Fatal("history references contain result payloads")
		}
	}
	if !ids[animation.ID] || !ids[precheck.ID] || !ids[latestAttempt.ID] || ids[invalid.ID] {
		t.Fatalf("stage selection changed: %v", ids)
	}
	stored, err := store.Get(t.Context(), animation.ID)
	if err != nil || stored.Result["animations"].([]any)[0].(map[string]any)["html"] != payload {
		t.Fatal("history projection changed the stored animation", err)
	}
}
