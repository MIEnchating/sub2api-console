package taskstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestLatestTaskUsesActualTimeAcrossFractionWidthsAndOffsets(t *testing.T) {
	for _, pair := range [][2]string{
		{"2026-09-07T12:00:00Z", "2026-09-07T12:00:00.1Z"},
		{"2026-09-07T12:00:00.1Z", "2026-09-07T12:00:00.12Z"},
		{"2026-09-07T13:00:00+01:00", "2026-09-07T12:01:00Z"},
	} {
		t.Run(pair[0], func(t *testing.T) {
			store, err := Open(filepath.Join(t.TempDir(), "tasks.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			for i, id := range []string{"older", "newer"} {
				task := Task{ID: id, Skill: "billing", Operation: "revenue-calculation", Status: "succeeded", Progress: 100, Message: "完成", Result: map[string]any{}, CreatedAt: pair[i], UpdatedAt: pair[i]}
				if err := store.Save(context.Background(), task); err != nil {
					t.Fatal(err)
				}
			}
			latest, err := store.LatestByOperation(context.Background(), "revenue-calculation", "succeeded")
			if err != nil {
				t.Fatal(err)
			}
			if latest.ID != "newer" {
				t.Fatalf("latest task = %s, want newer", latest.ID)
			}
		})
	}
}

func TestClearLogsPreservesTasksAtOrAfterFractionalCutoff(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "tasks.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	cutoff := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	for i, id := range []string{"older", "equal", "newer"} {
		at := cutoff.Add(time.Duration(i-1) * 100 * time.Millisecond).Format(time.RFC3339Nano)
		if err := store.Save(context.Background(), Task{ID: id, Skill: "console", Operation: "inspect", Status: "succeeded", Progress: 100, Message: "完成", Result: map[string]any{}, CreatedAt: at, UpdatedAt: at}); err != nil {
			t.Fatal(err)
		}
	}
	deleted, _, err := store.ClearLogs(context.Background(), &cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted %d tasks, want only older task", deleted)
	}
	for _, id := range []string{"equal", "newer"} {
		if _, err := store.Get(context.Background(), id); err != nil {
			t.Fatalf("retained task %s: %v", id, err)
		}
	}
}
