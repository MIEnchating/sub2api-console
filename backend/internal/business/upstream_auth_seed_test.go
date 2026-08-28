package business

import (
	"context"
	"path/filepath"
	"testing"
)

func TestUpstreamAuthSeedReadsOnlyStableHostMetadata(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO upstreams(host,base_url,upstream_type,auth_mode,auth_status,updated_at)
		VALUES('api.example','https://api.example','sub2api',NULL,'认证过期','now')`); err != nil {
		t.Fatal(err)
	}
	seed, err := store.UpstreamAuthSeed(ctx, "HTTPS://API.EXAMPLE/")
	if err != nil || seed == nil || seed.Host != "api.example" || seed.BaseURL != "https://api.example" || seed.UpstreamType != "sub2api" || seed.AuthMode != nil {
		t.Fatalf("seed=%#v err=%v", seed, err)
	}
	missing, err := store.UpstreamAuthSeed(ctx, "missing.example")
	if err != nil || missing != nil {
		t.Fatalf("missing=%#v err=%v", missing, err)
	}
}
