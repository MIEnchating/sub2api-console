package business_test

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func BenchmarkUpstreamUsageCatalog(b *testing.B) {
	path := filepath.Join(b.TempDir(), "catalog.db")
	store, err := business.Open(path)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = store.Close() })
	db, err := sql.Open("sqlite", path)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = db.Close() })
	if _, err = store.CreateUpstreamConfiguration(b.Context(), business.UpstreamConfigurationWrite{Host: "fixture.example", BaseURL: "https://fixture.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1"}); err != nil {
		b.Fatal(err)
	}
	if _, err = db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<100)
 INSERT INTO accounts(id,name,updated_at) SELECT CAST(x AS TEXT),printf('account-%d',x),'now' FROM n;
 INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,updated_at)
 SELECT id,'fixture.example',id,name,'group','now' FROM accounts;
 INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,updated_at) SELECT b.id,h.upstream_id,b.upstream_key_id,'now' FROM bindings b JOIN upstream_identity_hosts h ON h.host=b.upstream_host;`); err != nil {
		b.Fatal(err)
	}
	stamp := time.Now().UTC().Add(-time.Hour).Format("2006-01-02T15:04:05.000000000Z")
	if _, err = db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<50000)
 INSERT INTO usage_records(account_id,request_id,group_name,source,observed_at)
 SELECT CAST((x%100)+1 AS TEXT),printf('request-%d',x),'group','traffic',? FROM n`, stamp); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		summary, err := store.Upstreams(b.Context())
		if err != nil || len(summary.Hosts) != 1 || summary.Hosts[0].RequestCount != 50000 {
			b.Fatalf("catalog: %+v, %v", summary, err)
		}
	}
}
