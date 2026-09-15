package upstreamconfig_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamconfig"
)

func TestConfigurationIncludesSharedConcurrencyWithoutDefaultingMissingLimitToZero(t *testing.T) {
	for _, test := range []struct {
		name  string
		limit *int64
	}{
		{name: "known", limit: new(int64(12))},
		{name: "unlimited", limit: new(int64(0))},
		{name: "unknown"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, err := business.Open(filepath.Join(t.TempDir(), "business.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			if err := store.Bootstrap(t.Context()); err != nil {
				t.Fatal(err)
			}
			private, err := configstore.Open(filepath.Join(t.TempDir(), "private.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = private.Close() })
			_, err = store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{
				Host: "capacity.example", BaseURL: "https://capacity.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1",
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{
				Host: "capacity.example", Balance: &business.UpstreamBalanceObservation{ConcurrencyLimit: test.limit, ConcurrencyStatus: test.name, ProfileUserID: "17"},
			})
			if err != nil {
				t.Fatal(err)
			}
			configuration, err := upstreamconfig.New(store, private, nil).Get(t.Context(), "capacity.example")
			if err != nil {
				t.Fatal(err)
			}
			summary, err := store.Upstreams(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			actualJSON, err := json.Marshal(configuration)
			if err != nil {
				t.Fatal(err)
			}
			publicJSON, err := json.Marshal(summary.Hosts[0])
			if err != nil {
				t.Fatal(err)
			}
			var actual, public map[string]json.RawMessage
			if err := json.Unmarshal(actualJSON, &actual); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(publicJSON, &public); err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{"concurrency_limit", "concurrency_status", "concurrency_checked_at", "allocated_concurrency", "target_concurrency"} {
				if string(actual[field]) != string(public[field]) {
					t.Errorf("configuration %s = %s, want shared summary %s", field, actual[field], public[field])
				}
			}
		})
	}
}
