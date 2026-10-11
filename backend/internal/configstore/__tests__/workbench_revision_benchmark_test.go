package configstore_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func BenchmarkWorkbenchDocumentRevision(b *testing.B) {
	for _, size := range []int{1024, 128 << 10, 1 << 20} {
		b.Run(fmt.Sprintf("private_bytes=%d", size), func(b *testing.B) {
			store, _ := namespaceStore(b)
			raw := json.RawMessage(`{"private":"` + strings.Repeat("x", size) + `"}`)
			if _, err := store.SaveWorkbenchDocument(b.Context(), "maintenance:isolated", 0, raw); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if revision, err := store.WorkbenchDocumentRevision(b.Context(), "maintenance:isolated"); err != nil || revision != 1 {
					b.Fatalf("revision=%d err=%v", revision, err)
				}
			}
		})
	}
}
