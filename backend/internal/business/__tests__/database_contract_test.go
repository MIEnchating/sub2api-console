package business_test

import (
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
	"testing"
)

func TestDatabaseContractBusinessBootstrapPreservesPolicyOnReopen(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store, err := business.OpenDatabase(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Bootstrap(t.Context()); err != nil {
				store.Close()
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			store, err = business.OpenDatabase(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			ready, err := store.Ready(t.Context())
			if err != nil || !ready {
				t.Fatalf("bootstrap lost: %v %v", ready, err)
			}
			if err := store.Bootstrap(t.Context()); err != nil {
				t.Fatal(err)
			}
			mode, err := store.Mode(t.Context())
			if err != nil || mode != "完全模式" {
				t.Fatalf("policy unavailable: %q %v", mode, err)
			}
		})
	}
}
