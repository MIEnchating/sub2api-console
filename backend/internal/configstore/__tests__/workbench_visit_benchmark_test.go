package configstore_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func BenchmarkWorkbenchDocumentVisit(b *testing.B) {
	for _, matchAt := range []int{1, 100, 0} {
		b.Run(fmt.Sprintf("match_at=%d", matchAt), func(b *testing.B) {
			store, path := namespaceStore(b)
			db, err := sql.Open("sqlite", path)
			if err != nil {
				b.Fatal(err)
			}
			raw := json.RawMessage(`{"private":"` + strings.Repeat("x", 128<<10) + `"}`)
			_, err = db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<100)
				INSERT INTO account_workbench_documents(id,revision,payload)
				SELECT printf('run:owner:%03d',x),1,? FROM n`, []byte(raw))
			if err != nil {
				b.Fatal(err)
			}
			if err := db.Close(); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				seen := 0
				err := store.VisitWorkbenchDocuments(b.Context(), "run:owner:", 501, func(record configstore.WorkbenchDocumentRecord) (bool, error) {
					if len(record.Payload) != len(raw) {
						b.Fatal("incomplete payload")
					}
					seen++
					return seen != matchAt, nil
				})
				want := matchAt
				if want == 0 {
					want = 100
				}
				if err != nil || seen != want {
					b.Fatalf("seen=%d want=%d err=%v", seen, want, err)
				}
			}
		})
	}
}
