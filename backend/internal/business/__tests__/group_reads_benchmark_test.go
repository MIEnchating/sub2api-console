package business_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func BenchmarkGroupReads(b *testing.B) {
	for _, size := range []struct{ groups, accounts int }{{1, 20}, {100, 2000}} {
		b.Run(fmt.Sprintf("%dgroups_%daccounts", size.groups, size.accounts), func(b *testing.B) {
			path := filepath.Join(b.TempDir(), "group-reads.sqlite3")
			store, err := business.Open(path)
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = store.Close() })
			if err := store.Bootstrap(b.Context()); err != nil {
				b.Fatal(err)
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = db.Close() })
			tx, err := db.Begin()
			if err != nil {
				b.Fatal(err)
			}
			defer tx.Rollback()
			for group := range size.groups {
				if _, err := tx.Exec(`INSERT INTO local_groups(name,remote_id,updated_at) VALUES(?,?,'now')`, fmt.Sprintf("group-%03d", group), fmt.Sprint(group+1)); err != nil {
					b.Fatal(err)
				}
			}
			for account := range size.accounts {
				id := fmt.Sprint(account + 1)
				if _, err := tx.Exec(`INSERT INTO accounts(id,name,schedulable,metadata_json,updated_at) VALUES(?,?,1,'{"type":"apikey","platform":"openai","known_models":["model-a"]}','now')`, id, "account-"+id); err != nil {
					b.Fatal(err)
				}
				for offset := range min(3, size.groups) {
					group := (account + offset) % size.groups
					if _, err := tx.Exec(`INSERT INTO account_groups(account_id,group_name,group_id) VALUES(?,?,?)`, id, fmt.Sprintf("group-%03d", group), fmt.Sprint(group+1)); err != nil {
						b.Fatal(err)
					}
				}
			}
			if err := tx.Commit(); err != nil {
				b.Fatal(err)
			}
			b.Run("summary", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := store.Groups(b.Context()); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("write_readback", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := store.SetGroupExcluded(b.Context(), "1", false, "benchmark"); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run("models", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := store.GroupProbeModels(b.Context(), "1"); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}
