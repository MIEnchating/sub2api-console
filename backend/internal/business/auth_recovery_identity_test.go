package business

import (
	"context"
	"path/filepath"
	"testing"
)

func TestAuthRecoveryProjectionDoesNotGuessWWWIdentity(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	name := "Unrelated www upstream"
	if _, err := store.CreateUpstreamConfiguration(ctx, UpstreamConfigurationWrite{Host: "www.api.example", Name: &name, BaseURL: "https://www.api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1"}); err != nil {
		t.Fatal(err)
	}
	var before string
	if err := store.db.QueryRow(`SELECT auth_status FROM upstreams WHERE host='www.api.example'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PersistAuthRecoveryOutcomes(ctx, []AuthRecoveryOutcome{{Host: "api.example", Success: false, Attempted: true}}, "operator"); err != nil {
		t.Fatal(err)
	}
	var after string
	if err := store.db.QueryRow(`SELECT auth_status FROM upstreams WHERE host='www.api.example'`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("unrelated www upstream was changed: %q -> %q", before, after)
	}
}

func TestAuthRecoveryProjectionResolvesExplicitStableAlias(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	name := "Canonical upstream"
	if _, err := store.CreateUpstreamConfiguration(ctx, UpstreamConfigurationWrite{Host: "primary.example", Name: &name, BaseURL: "https://primary.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) SELECT 'alias.example',upstream_id,0,'now' FROM upstream_identity_hosts WHERE host='primary.example'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PersistAuthRecoveryOutcomes(ctx, []AuthRecoveryOutcome{{Host: "alias.example", Success: false, Attempted: true}}, "operator"); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := store.db.QueryRow(`SELECT auth_status FROM upstreams WHERE host='primary.example'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != UpstreamAuthStatusInvalid {
		t.Fatalf("stable alias outcome was not projected: status=%q", status)
	}
}
