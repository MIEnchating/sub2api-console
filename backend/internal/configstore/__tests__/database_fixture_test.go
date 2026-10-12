package configstore_test

import (
	"context"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
	"testing"
)

func databaseConfigs(t *testing.T) []dbconn.Config { return dbtest.Configs(t) }

func openContractStore(t *testing.T, config dbconn.Config) *configstore.Store {
	t.Helper()
	store, err := configstore.OpenDatabase(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
		db, err := dbconn.Open(context.Background(), config)
		if err != nil {
			t.Error(err)
			return
		}
		defer db.Close()
		for _, table := range []string{"config_setting_locks", "settings", "console_sessions", "uptime_kuma_config", "uptime_kuma_monitor_templates", "account_workbench_documents", "model_pricing_cache", "auth_records", "vault_entries", "auth_recovery_preferences", "upstream_key_secrets", "newapi_platforms", "dictionary_entries"} {
			if table == "config_setting_locks" && config.Driver == dbconn.SQLite {
				continue
			}
			if _, err := db.Exec("DELETE FROM " + table); err != nil {
				t.Error(err)
			}
		}
	})
	return store
}
