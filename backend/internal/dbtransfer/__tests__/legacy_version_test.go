package dbtransfer_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestLegacyStoreRejectsFutureSchemaWithoutCreatingTables(t *testing.T) {
	for _, module := range []string{"config", "tasks"} {
		t.Run(module, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "future.sqlite3")
			db, err := sql.Open("sqlite", sqliteutil.DSN(path, ""))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.Exec(`PRAGMA user_version=999;CREATE TABLE future_data(id INTEGER PRIMARY KEY)`); err != nil {
				t.Fatal(err)
			}
			if module == "config" {
				store, err := configstore.Open(path)
				if err == nil {
					store.Close()
					t.Fatal("future config schema accepted")
				}
			} else {
				store, err := taskstore.Open(path)
				if err == nil {
					store.Close()
					t.Fatal("future task schema accepted")
				}
			}
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema WHERE type='table' AND name NOT LIKE 'sqlite_%'`).Scan(&count); err != nil || count != 1 {
				t.Fatalf("future database changed: %d %v", count, err)
			}
		})
	}
}
