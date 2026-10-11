package business_test

import (
	"fmt"
	"strconv"
	"testing"
)

func BenchmarkModelCatalogBatch(b *testing.B) {
	for _, size := range []int{1, 100, 1000} {
		b.Run(fmt.Sprintf("accounts=%d", size), func(b *testing.B) {
			store, db := pricingProjectionFixture(b)
			tx, err := db.Begin()
			if err != nil {
				b.Fatal(err)
			}
			defer tx.Rollback()
			ids := make([]string, size)
			for i := range size {
				ids[i] = strconv.Itoa(i + 1)
				if _, err := tx.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES(?,'fixture','{"platform":"openai","known_models":["model-a","model-b"]}','now')`, ids[i]); err != nil {
					b.Fatal(err)
				}
			}
			if err := tx.Commit(); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				got, err := store.AccountModelSyncPreview(b.Context(), ids)
				if err != nil || len(got.Accounts) != size {
					b.Fatalf("accounts=%d err=%v", len(got.Accounts), err)
				}
			}
		})
	}
}

func BenchmarkDetectionScopeBatch(b *testing.B) {
	for _, size := range []int{1, 100, 500} {
		b.Run(fmt.Sprintf("groups=%d", size), func(b *testing.B) {
			store, db := pricingProjectionFixture(b)
			tx, err := db.Begin()
			if err != nil {
				b.Fatal(err)
			}
			defer tx.Rollback()
			if _, err := tx.Exec(`INSERT INTO accounts(id,name,updated_at) VALUES('41','fixture','now')`); err != nil {
				b.Fatal(err)
			}
			ids := make([]string, size)
			for i := range size {
				ids[i] = strconv.Itoa(i + 1)
				if _, err := tx.Exec(`INSERT INTO local_groups(name,remote_id,updated_at) VALUES(?,?,'now'); INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41',?,?)`, ids[i], ids[i], ids[i], ids[i]); err != nil {
					b.Fatal(err)
				}
			}
			if err := tx.Commit(); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				got, err := store.DetectionTaskScope(b.Context(), ids)
				if err != nil || len(got.GroupIDsByAccount["41"]) != size {
					b.Fatalf("groups=%d err=%v", len(got.GroupIDsByAccount["41"]), err)
				}
			}
		})
	}
}
