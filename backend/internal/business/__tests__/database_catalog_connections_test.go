package business_test

import (
	"context"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
)

func TestDatabaseContractCatalogReadsReleaseConnectionBeforeSupplementaryQueries(t *testing.T) {
	for _, cfg := range dbtest.Configs(t) {
		t.Run(string(cfg.Driver), func(t *testing.T) {
			cfg.MaxOpenConns, cfg.MaxIdleConns = 1, 1
			store, err := business.OpenDatabase(t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			db, err := dbconn.Open(t.Context(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			const host = "catalog-connection.contract.test"
			created, err := store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{Host: host, BaseURL: "https://" + host, UpstreamType: "sub2api", AuthMode: "apikey", RechargeRate: "1"})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				for _, table := range []string{"recharge_rates", "upstreams"} {
					if _, err := db.ExecContext(t.Context(), "DELETE FROM "+table+" WHERE host=?", host); err != nil {
						t.Error(err)
					}
				}
				if _, err := db.ExecContext(t.Context(), `DELETE FROM upstream_identities WHERE upstream_id=?`, created.UpstreamID); err != nil {
					t.Error(err)
				}
			}()
			t.Run("editor read completes with a single pooled connection", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				got, err := store.UpstreamConfigurationHost(ctx, host)
				if err != nil {
					t.Fatal(err)
				}
				if got == nil || got.UpstreamID != created.UpstreamID {
					t.Fatalf("editor lost stable upstream: %+v", got)
				}
			})
			t.Run("catalog read preserves the caller snapshot without overlapping queries", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				err := store.WithReadSnapshot(ctx, func(snapshot context.Context) error {
					got, err := store.UpstreamConfigurationHost(snapshot, host)
					if err != nil {
						return err
					}
					if got == nil || got.UpstreamID != created.UpstreamID {
						t.Fatalf("snapshot lost stable upstream: %+v", got)
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}
