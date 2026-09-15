package taskstore_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestHistoryDeleteChecksScopeVersionAndTerminalStateAtomically(t *testing.T) {
	for _, scenario := range []string{"running", "other-skill", "changed", "duplicate", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			ctx := context.Background()
			now := time.Now().UTC().Format(time.RFC3339Nano)
			first := taskstore.Task{ID: "first", Skill: "account-workbench", Operation: "account-workbench-import", Status: "succeeded", Message: "导入已完成", CreatedAt: now, UpdatedAt: now, Result: map[string]any{}}
			second := first
			second.ID = "second"
			switch scenario {
			case "running":
				second.Status = "running"
			case "other-skill":
				second.Skill = "other"
			case "changed":
				second.UpdatedAt = time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
			}
			if err := store.Save(ctx, first); err != nil {
				t.Fatal(err)
			}
			if err := store.Save(ctx, second); err != nil {
				t.Fatal(err)
			}
			selected := []taskstore.HistorySelection{{ID: first.ID, UpdatedAt: now}, {ID: second.ID, UpdatedAt: now}}
			if scenario == "duplicate" {
				selected[1] = selected[0]
			}
			if scenario == "missing" {
				selected[1].ID = "missing"
			}
			if err := store.DeleteTerminalBySkill(ctx, "account-workbench", selected); err == nil {
				t.Fatal("invalid scope deleted records")
			}
			if _, err := store.Get(ctx, first.ID); err != nil {
				t.Fatal("rejected batch partially deleted first record")
			}
		})
	}
}

func TestHistoryDeleteRemovesOnlyConfirmedRecords(t *testing.T) {
	store, err := taskstore.Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, id := range []string{"selected", "retained"} {
		if err := store.Save(ctx, taskstore.Task{ID: id, Skill: "account-workbench", Operation: "account-workbench-import", Status: "failed", Message: "导入失败", CreatedAt: now, UpdatedAt: now, Result: map[string]any{}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.DeleteTerminalBySkill(ctx, "account-workbench", []taskstore.HistorySelection{{ID: "selected", UpdatedAt: now}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "selected"); err != taskstore.ErrNotFound {
		t.Fatal("selected record retained")
	}
	if _, err := store.Get(ctx, "retained"); err != nil {
		t.Fatal("unselected record removed")
	}
}
