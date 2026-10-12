package configstore_test

import (
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"testing"
)

func TestDatabaseContractSettingsAndVersionedConfiguration(t *testing.T) {
	for _, config := range databaseConfigs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store := openContractStore(t, config)
			for _, value := range []string{"first", "中文第二次"} {
				if err := store.SaveJSONSetting(t.Context(), "contract.value", value); err != nil {
					t.Fatal(err)
				}
				got, err := store.JSONSetting(t.Context(), "contract.value")
				if err != nil || got != value {
					t.Fatalf("settings roundtrip: %q %v", got, err)
				}
			}
			item := configstore.UptimeKumaConfig{BaseURL: "https://kuma.example.test"}
			if err := store.SaveUptimeKuma(t.Context(), item); err != nil {
				t.Fatal(err)
			}
			if err := store.SaveUptimeKuma(t.Context(), item); !errors.Is(err, configstore.ErrKumaConfigConflict) {
				t.Fatalf("stale create: %v", err)
			}
			item.Revision = 1
			item.Username = "operator"
			if err := store.SaveUptimeKuma(t.Context(), item); err != nil {
				t.Fatal(err)
			}
			if err := store.SaveUptimeKuma(t.Context(), item); !errors.Is(err, configstore.ErrKumaConfigConflict) {
				t.Fatalf("stale update: %v", err)
			}
			got, err := store.UptimeKuma(t.Context())
			if err != nil || got.Revision != 2 || got.Username != "operator" {
				t.Fatalf("configuration: %+v %v", got, err)
			}
		})
	}
}
