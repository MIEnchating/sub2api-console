package taskstore_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func BenchmarkAnimationHistoryRanking(b *testing.B) {
	store, err := taskstore.Open(filepath.Join(b.TempDir(), "history.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = store.Close() })
	// Each of 100 accounts has ten historical results. The query must retain
	// its latest result, even if unrelated accounts fill the recent-20 window.
	for account := range 100 {
		for attempt := range 10 {
			stamp := fmt.Sprintf("2026-10-08T00:%02d:00.123456789Z", attempt)
			task := historyTimeTask(fmt.Sprintf("task-%03d-%02d", account, attempt), fmt.Sprint(account), stamp, stamp)
			if err := store.Save(b.Context(), task); err != nil {
				b.Fatal(err)
			}
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		tasks, err := store.AnimationHistory(b.Context())
		if err != nil || len(tasks) != 100 {
			b.Fatalf("history size=%d error=%v", len(tasks), err)
		}
	}
}
