package business_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

func BenchmarkAccountsWithLargeMetadata(b *testing.B) {
	path := filepath.Join(b.TempDir(), "projection.db")
	store, err := business.Open(path)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		b.Fatal(err)
	}
	db, err := sql.Open("sqlite", sqliteutil.DSN(path, ""))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = db.Close() })
	metadata, err := json.Marshal(map[string]any{
		"platform": "openai", "type": "apikey", "status": "active",
		"base_url": "https://fixture.example/v1", "notes": strings.Repeat("fixture ", 4096),
	})
	if err != nil {
		b.Fatal(err)
	}
	for id := 1; id <= 20; id++ {
		if _, err := db.Exec(`INSERT INTO accounts(id,name,schedulable,metadata_json,updated_at)
			VALUES(?,?,1,?,'2026-09-14T00:00:00Z')`, strconv.Itoa(id), "fixture-account", string(metadata)); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		accounts, err := store.Accounts(ctx)
		if err != nil || len(accounts) != 20 {
			b.Fatalf("accounts=%d, error=%v", len(accounts), err)
		}
	}
}
