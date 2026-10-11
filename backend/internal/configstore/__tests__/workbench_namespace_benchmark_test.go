package configstore_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func BenchmarkWorkbenchNamespaceReads(b *testing.B) {
	for _, unrelated := range []int{0, 10000} {
		for _, latest := range []bool{false, true} {
			b.Run(fmt.Sprintf("unrelated=%d/latest=%t", unrelated, latest), func(b *testing.B) {
				store, path := namespaceStore(b)
				db, err := sql.Open("sqlite", path)
				if err != nil {
					b.Fatal(err)
				}
				defer db.Close()
				// Matching records have large private payloads. Other namespaces are
				// inserted later, so reverse table scans must skip all of them.
				payload, err := json.Marshal(map[string]string{"private": strings.Repeat("x", 128<<10)})
				if err != nil {
					b.Fatal(err)
				}
				if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<100)
					INSERT INTO account_workbench_documents(id,revision,payload)
					SELECT printf('run:owner:%04d',x),1,? FROM n`, payload); err != nil {
					b.Fatal(err)
				}
				if unrelated > 0 {
					if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<?)
						INSERT INTO account_workbench_documents(id,revision,payload)
						SELECT printf('artifact:other:%05d',x),1,? FROM n`, unrelated, []byte(`{"private":"unrelated"}`)); err != nil {
						b.Fatal(err)
					}
				}
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					var count int
					if latest {
						records, readErr := store.WorkbenchDocuments(b.Context(), "run:owner:", 20)
						count, err = len(records), readErr
					} else {
						records, readErr := store.WorkbenchDocumentPage(b.Context(), "run:owner:", "", 20)
						count, err = len(records), readErr
					}
					if err != nil || count != 20 {
						b.Fatalf("records=%d error=%v", count, err)
					}
				}
			})
		}
	}
}
