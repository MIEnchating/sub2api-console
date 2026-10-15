package configstore_test

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	_ "modernc.org/sqlite"
)

func TestLegacyVaultMigrationAddsIndependentStableIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.sqlite3")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE vault_entries(entry TEXT PRIMARY KEY,username TEXT,password TEXT,hosts_json TEXT NOT NULL,headers_json TEXT NOT NULL,updated_at TEXT NOT NULL)`,
		`INSERT INTO vault_entries VALUES('原名称','operator','secret','["api.example.test"]','{"X-Private":"retained"}','2026-10-15T00:00:00Z')`,
		`CREATE TABLE auth_recovery_preferences(host TEXT PRIMARY KEY,auth_mode TEXT NOT NULL,recovery_method TEXT NOT NULL,vault_entry TEXT,succeeded_at TEXT NOT NULL)`,
		`INSERT INTO auth_recovery_preferences VALUES('api.example.test','sub2api_user_token','vault','原名称','2026-10-15T00:00:00Z')`,
		`INSERT INTO auth_recovery_preferences VALUES('missing.example.test','sub2api_user_token','vault','已删除名称','2026-10-15T00:00:00Z')`,
		`PRAGMA user_version=1`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	entries, err := store.VaultEntryIndex(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(entries)
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	id, ok := rows[0]["id"].(string)
	if !ok || id == "" || id == "原名称" {
		t.Fatal("vault identity must be generated independently from its display name")
	}
	item, err := store.VaultEntry(t.Context(), id)
	if err != nil || item == nil || item.Password == nil || *item.Password != "secret" || item.Headers["X-Private"] != "retained" {
		t.Fatal("migration lost secret", err)
	}
	pref, err := store.AuthRecoveryPreference(t.Context(), "api.example.test")
	if err != nil || pref == nil || pref.VaultEntryID == nil || *pref.VaultEntryID != id {
		t.Fatal("migration lost binding", err)
	}
	missing, err := store.AuthRecoveryPreference(t.Context(), "missing.example.test")
	if err != nil || missing == nil || missing.VaultEntryID == nil || *missing.VaultEntryID == "已删除名称" {
		t.Fatal("missing binding must be a tombstone", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := configstore.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	again, err := reopened.VaultEntry(t.Context(), id)
	if err != nil || again == nil {
		t.Fatal("reopening changed identity", err)
	}

}

func TestDatabaseContractVaultIdentitySurvivesRenameAndRejectsReusedNames(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store := openContractStore(t, config)
			secret, id := "original-secret", configstore.NewVaultEntryID()
			if err := store.SaveVaultEntry(t.Context(), configstore.VaultEntry{ID: id, Entry: "主站", Password: &secret}, nil); err != nil {
				t.Fatal(err)
			}
			if err := store.SaveAuthRecoveryPreference(t.Context(), configstore.AuthRecoveryPreference{Host: "api.example.test", AuthMode: "sub2api_user_login", RecoveryMethod: "vault", VaultEntryID: &id}); err != nil {
				t.Fatal(err)
			}
			if err := store.UpdateVaultEntry(t.Context(), id, configstore.VaultEntry{Entry: "新名称"}, map[string]bool{}); err != nil {
				t.Fatal(err)
			}
			stored, err := store.VaultEntry(t.Context(), id)
			if err != nil || stored == nil || stored.Entry != "新名称" || stored.Password == nil || *stored.Password != secret {
				t.Fatal("rename changed credential identity or secret", err)
			}
			preference, err := store.AuthRecoveryPreference(t.Context(), "api.example.test")
			if err != nil || preference == nil || preference.VaultEntryID == nil || *preference.VaultEntryID != id {
				t.Fatal("rename changed binding", err)
			}
			if _, err := store.DeleteVaultEntry(t.Context(), id); err != nil {
				t.Fatal(err)
			}
			replacementID := configstore.NewVaultEntryID()
			if err := store.SaveVaultEntry(t.Context(), configstore.VaultEntry{ID: replacementID, Entry: "新名称"}, nil); err != nil {
				t.Fatal(err)
			}
			if got, err := store.VaultEntry(t.Context(), id); err != nil || got != nil {
				t.Fatal("old ID resolved to replacement", err)
			}
			if got, err := store.VaultEntry(t.Context(), "新名称"); err != nil || got != nil {
				t.Fatal("display name accepted as identity", err)
			}
			if err := store.UpdateVaultEntry(t.Context(), id, configstore.VaultEntry{Entry: "新名称"}, map[string]bool{}); err == nil {
				t.Fatal("stale edit recreated deleted credential")
			}
			if deleted, err := store.DeleteVaultEntry(t.Context(), "新名称"); err != nil || deleted {
				t.Fatal("display name accepted for deletion", err)
			}
		})
	}
}

func TestDatabaseContractVaultMigrationPreservesExistingCredentials(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		if config.Driver == dbconn.SQLite {
			continue
		}
		t.Run(string(config.Driver), func(t *testing.T) {
			original, err := configstore.OpenDatabase(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			if err := original.Close(); err != nil {
				t.Fatal(err)
			}
			db, err := dbconn.Open(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			for _, statement := range []string{
				`DROP TABLE auth_recovery_preferences`, `DROP TABLE vault_entries`,
				`CREATE TABLE vault_entries(entry VARCHAR(255) PRIMARY KEY,username TEXT,password TEXT,hosts_json TEXT NOT NULL,headers_json TEXT NOT NULL,updated_at TEXT NOT NULL)`,
				`CREATE TABLE auth_recovery_preferences(host VARCHAR(255) PRIMARY KEY,auth_mode TEXT NOT NULL,recovery_method TEXT NOT NULL,vault_entry TEXT,succeeded_at TEXT NOT NULL)`,
				`INSERT INTO vault_entries VALUES('legacy-name','operator','secret','["api.example.test"]','{"X-Private":"retained"}','2026-10-15T00:00:00Z')`,
				`INSERT INTO auth_recovery_preferences VALUES('api.example.test','sub2api_user_token','vault','legacy-name','2026-10-15T00:00:00Z')`,
				`DELETE FROM console_schema_migrations WHERE module='config' AND version=2`,
			} {
				if _, err := db.ExecContext(t.Context(), statement); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			store := openContractStore(t, config)
			index, err := store.VaultEntryIndex(t.Context())
			if err != nil || len(index) != 1 {
				t.Fatal("migration lost credential", err)
			}
			id := index[0].ID
			if id == "" || id == "legacy-name" {
				t.Fatal("migration did not assign independent ID")
			}
			item, err := store.VaultEntry(t.Context(), id)
			if err != nil || item == nil || item.Password == nil || *item.Password != "secret" || item.Headers["X-Private"] != "retained" {
				t.Fatal("migration changed secret", err)
			}
			pref, err := store.AuthRecoveryPreference(t.Context(), "api.example.test")
			if err != nil || pref == nil || pref.VaultEntryID == nil || *pref.VaultEntryID != id {
				t.Fatal("migration lost stable binding", err)
			}
		})
	}
}
