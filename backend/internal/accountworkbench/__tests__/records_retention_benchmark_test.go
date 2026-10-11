package accountworkbench_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func BenchmarkRecordRetentionWithPrivatePayloads(b *testing.B) {
	store, err := configstore.Open(filepath.Join(b.TempDir(), "workbench.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = store.Close() })
	owner := strings.Repeat("a", 64)
	var oldest [2]json.RawMessage
	for index := range 102 {
		id := fmt.Sprintf("%032x", index+1)
		raw, err := json.Marshal(map[string]any{
			"owner":   owner,
			"public":  accountworkbench.Run{ID: id, Action: "export", Status: "completed", CreatedAt: time.Unix(int64(index), 0).UTC()},
			"exports": []any{map[string]any{"credentials": map[string]any{"refresh_token": strings.Repeat("x", 128<<10)}}},
		})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := store.SaveWorkbenchDocument(b.Context(), "run:"+owner+":"+id, 0, raw); err != nil {
			b.Fatal(err)
		}
		if index < len(oldest) {
			oldest[index] = raw
		}
	}
	service := accountworkbench.New(store)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := service.PruneRecords(b.Context(), owner); err != nil {
			b.Fatal(err)
		}
		b.StopTimer()
		for index, raw := range oldest {
			id := fmt.Sprintf("%032x", index+1)
			if _, err := store.SaveWorkbenchDocument(b.Context(), "run:"+owner+":"+id, 0, raw); err != nil {
				b.Fatal(err)
			}
		}
		b.StartTimer()
	}
	b.StopTimer()
	rows, err := store.WorkbenchDocuments(b.Context(), "run:"+owner+":", 501)
	if err != nil || len(rows) != 102 {
		b.Fatalf("retention fixture changed: count=%d err=%v", len(rows), err)
	}
}
