package business_test

import (
	"context"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"testing"
	"time"
)

func TestIndependentUsageIncludesZeroUsageAndDeduplicatesAliasBindings(t *testing.T) {
	store, db := concurrencyStore(t)
	seedCapacityAccounts(t, db)
	if _, err := store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{Host: "empty.example", BaseURL: "https://empty.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,updated_at) SELECT b.id,h.upstream_id,b.upstream_key_id,'now' FROM bindings b JOIN upstream_identity_hosts h ON h.host=b.upstream_host;
 INSERT INTO usage_records(account_id,request_id,group_name,source,observed_at) VALUES('41','request-1','a','traffic',?),('41','request-1','b','traffic',?)`, time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano), time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	rows, err := store.UpstreamUsage(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected two stable upstreams: %+v", rows)
	}
	var withUsage, withoutUsage int
	for _, row := range rows {
		if row.RequestWindowDays != 30 || row.AccountRequestUsage == nil {
			t.Fatalf("invalid usage contract: %+v", row)
		}
		if row.RequestCount == 0 {
			withoutUsage++
			if len(row.AccountRequestUsage) != 0 {
				t.Fatalf("unbound upstream has accounts: %+v", row)
			}
			continue
		}
		withUsage++
		if row.RequestCount != 1 || len(row.AccountRequestUsage) != 1 || row.AccountRequestUsage[0].Share != 100 {
			t.Fatalf("alias binding double counted: %+v", row)
		}
	}
	if withUsage != 1 || withoutUsage != 1 {
		t.Fatalf("unexpected totals: %+v", rows)
	}
}

func TestIndependentUsagePropagatesCancellation(t *testing.T) {
	store, _ := concurrencyStore(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.UpstreamUsage(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
