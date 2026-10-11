package uptimekuma_test

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

func BenchmarkMaintenanceAssociationValidation(b *testing.B) {
	for _, selected := range []int{1, 100, 500} {
		b.Run(fmt.Sprintf("selected=%d", selected), func(b *testing.B) {
			objects := make(map[string]json.RawMessage, 1000)
			for id := 1; id <= 1000; id++ {
				raw, err := json.Marshal(map[string]any{
					"id": id, "name": "服务", "type": "http", "headers": strings.Repeat("x", 256),
				})
				if err != nil {
					b.Fatal(err)
				}
				objects[strconv.Itoa(id)] = raw
			}
			ids := make([]int64, selected)
			for i := range ids {
				ids[i] = int64(i + 1)
			}
			f := newResourceValidationFixture(b, objects, map[string]json.RawMessage{})
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				result, err := f.maintenance(ids, nil)
				if err != nil || result["resource_id"] != int64(37) {
					b.Fatalf("maintenance failed: %v, %v", result, err)
				}
				f.mu.Lock()
				if len(f.monitors) != selected || len(f.events) != 3 {
					b.Fatalf("wrong association count: %d", len(f.monitors))
				}
				f.events = f.events[:0]
				f.mu.Unlock()
			}
		})
	}
}
