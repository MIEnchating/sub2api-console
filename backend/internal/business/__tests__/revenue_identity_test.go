package business_test

import (
	"context"
	"testing"
)

func TestRevenueCatalogPreservesStableUpstreamIdentityFromAuthenticationHost(t *testing.T) {
	store, db := openTrafficRankingStore(t)
	_, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','账号','{}','now');
 INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES('stable-upstream','now','now');
 INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES('auth.example','stable-upstream',1,'now');
 INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,source_auth_host,metadata_json,updated_at)
 VALUES('41','edge.example','91','key','group','auth.example','{}','now')`)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := store.RevenueCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Accounts) != 1 || len(catalog.Accounts[0].Bindings) != 1 || catalog.Accounts[0].Bindings[0].UpstreamID != "stable-upstream" {
		t.Fatalf("catalog=%+v", catalog)
	}
}
