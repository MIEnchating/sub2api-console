package modelcheck_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/modelcheck"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

// Substitute only the storage boundary to measure aggregation independently of
// SQLite query planning and JSON decoding. Each task respects the 100-check limit.
type statisticsHistory struct {
	*taskstore.Store
	tasks []taskstore.Task
}

func (s *statisticsHistory) ModelCheckHistory(context.Context, time.Time) ([]taskstore.Task, error) {
	return append([]taskstore.Task(nil), s.tasks...), nil
}

func BenchmarkAccountStatusAggregation(b *testing.B) {
	store, err := taskstore.Open(filepath.Join(b.TempDir(), "tasks.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	history := &statisticsHistory{Store: store}
	now := time.Now().UTC()
	for index := range 200 {
		ids := make([]string, 20)
		rows := make([]any, 0, 100)
		for account := range 20 {
			ids[account] = fmt.Sprint(account + 1)
			for range 5 {
				rows = append(rows, map[string]any{"account_id": ids[account], "verdict": "MATCH"})
			}
		}
		history.tasks = append(history.tasks, taskstore.Task{ID: fmt.Sprint(index), Status: "succeeded", UpdatedAt: now.Add(-time.Duration(index+1) * time.Minute).Format(time.RFC3339Nano), Result: map[string]any{"account_ids": ids, "tests": rows}})
	}
	service, err := modelcheck.New(history, credentials{}, &catalog{}, credentials{})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		statuses, err := service.AccountStatuses(context.Background())
		if err != nil || len(statuses) != 20 || statuses[0].Confidence.Short.Passed != 1000 {
			b.Fatalf("aggregation: count=%d err=%v", len(statuses), err)
		}
	}
}
