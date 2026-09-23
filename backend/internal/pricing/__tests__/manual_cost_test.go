package pricing_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/pricing"
	"path/filepath"
	"testing"
)

func TestManualAccountWithoutExemptionMovesOutOfLossGroup(t *testing.T) {
	store, err := business.Open(filepath.Join(t.TempDir(), "manual-cost.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, err = store.SyncManagementSnapshot(t.Context(), []map[string]any{{"id": "41", "name": "多多Ai-0.32", "platform": "openai", "rate_multiplier": "0.32", "group_ids": []any{"7"}}}, []map[string]any{{"id": "7", "name": "平价", "platform": "openai", "rate_multiplier": "0.25"}, {"id": "8", "name": "旗舰", "platform": "openai", "rate_multiplier": "0.5"}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AssignManualPriority(t.Context(), "41", 1, "10", 10, true, "test"); err != nil {
		t.Fatal(err)
	}
	service := pricing.New(store, nil, nil)
	snapshot, err := service.UpdateConfig(t.Context(), minimumConfig(t, nil, [][]string{{"7", "8"}}), "test")
	if err != nil {
		t.Fatal(err)
	}
	d := snapshot.Decisions[0]
	if d.Skipped || !d.Changed || len(d.DesiredGroupIDs) != 1 || d.DesiredGroupIDs[0] != "8" {
		t.Fatalf("manual account escaped cost migration: %+v", d)
	}
}
