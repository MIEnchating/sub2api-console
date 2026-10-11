package business_test

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/sqliteutil"
)

func BenchmarkProbeCredentialAccountRead(b *testing.B) {
	path := filepath.Join(b.TempDir(), "probe-binding.db")
	store, err := business.Open(path)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(b.Context()); err != nil {
		b.Fatal(err)
	}
	db, err := sql.Open("sqlite", sqliteutil.DSN(path, ""))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = db.Close() })
	metadata, err := json.Marshal(map[string]any{
		"base_url": "https://fixture.example/v1", "notes": strings.Repeat("fixture ", 512),
	})
	if err != nil {
		b.Fatal(err)
	}
	for id := 1; id <= 100; id++ {
		accountID := strconv.Itoa(id)
		if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES(?, 'probe account',?,'now');
			INSERT INTO account_groups(account_id,group_name) VALUES(?,'probe group');
			INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,
			upstream_group_id,local_group,status,updated_at) VALUES(?,'fixture.example',?,'bound key','7','probe group','active','now')`,
			accountID, string(metadata), accountID, accountID, accountID); err != nil {
			b.Fatal(err)
		}
	}
	b.Run("account-detail", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			detail, err := store.Account(b.Context(), "41")
			if err != nil {
				b.Fatal(err)
			}
			if detail.ID != "41" || len(detail.Bindings) != 1 {
				b.Fatal("incomplete account detail")
			}
		}
	})
	b.Run("probe-binding", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			binding, err := store.ProbeAccountBinding(b.Context(), "41")
			if err != nil {
				b.Fatal(err)
			}
			if binding.ID != "41" || len(binding.Bindings) != 1 {
				b.Fatal("incomplete probe binding")
			}
		}
	})
}
