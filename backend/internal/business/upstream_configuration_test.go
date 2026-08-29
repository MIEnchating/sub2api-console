package business

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestUpstreamConfigurationCreateAndRateRecalculationUseDecimalText(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	name := "Example"
	created, err := store.CreateUpstreamConfiguration(ctx, UpstreamConfigurationWrite{
		Host: "HTTPS://API.EXAMPLE/", Name: &name, BaseURL: "https://api.example/",
		UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "0.3",
	})
	if err != nil || created.Host != "api.example" || created.RechargeRate != "0.3" {
		t.Fatalf("unexpected create: %#v err=%v", created, err)
	}
	if _, err := store.db.Exec(`UPDATE upstreams SET raw_balance='1' WHERE host='api.example'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO upstream_groups(host,group_id,name,status,raw_rate,updated_at)
		VALUES('api.example','7','pro','active','0.2','now')`); err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpdateUpstreamConfiguration(ctx, UpstreamConfigurationWrite{
		Host: "api.example", Name: &name, BaseURL: "https://api.example",
		UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "0.3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Balance == nil || *updated.Balance != "3.3333333333333333333333333333" || updated.ConvertedGroups != 1 {
		t.Fatalf("unexpected decimal projection: %#v", updated)
	}
	groups, err := store.UpstreamGroups(ctx, "api.example", true)
	if err != nil || len(groups) != 1 || groups[0].EffectiveRate == nil || *groups[0].EffectiveRate != "0.6666666666666666666666666667" {
		t.Fatalf("unexpected groups: %#v err=%v", groups, err)
	}
}

func TestUpstreamConfigurationCreateAdoptsExistingOrphanRechargeRate(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO recharge_rates(host,recharge_rate,note,updated_at) VALUES('api.example','0.5','account-import','now')`); err != nil {
		t.Fatal(err)
	}
	name := "Example"
	created, err := store.CreateUpstreamConfiguration(ctx, UpstreamConfigurationWrite{
		Host: "api.example", Name: &name, BaseURL: "https://api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", RechargeRate: "0.75",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.RechargeRate != "0.75" {
		t.Fatalf("created=%#v", created)
	}
	var rate, note string
	if err := store.db.QueryRowContext(ctx, `SELECT recharge_rate,note FROM recharge_rates WHERE host='api.example'`).Scan(&rate, &note); err != nil {
		t.Fatal(err)
	}
	if rate != "0.75" || note != "console-upstream-create" {
		t.Fatalf("rate=%q note=%q", rate, note)
	}
}

func TestUpstreamConfigurationUpdateRollsBackInvalidMetadata(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	name := "Example"
	if _, err := store.CreateUpstreamConfiguration(ctx, UpstreamConfigurationWrite{Host: "api.example", Name: &name, BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE upstreams SET metadata_json='[]' WHERE host='api.example'`); err != nil {
		t.Fatal(err)
	}
	_, err = store.UpdateUpstreamConfiguration(ctx, UpstreamConfigurationWrite{Host: "api.example", Name: &name, BaseURL: "https://changed.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "2"})
	if err == nil {
		t.Fatal("invalid metadata must reject update")
	}
	var baseURL string
	if err := store.db.QueryRow(`SELECT base_url FROM upstreams WHERE host='api.example'`).Scan(&baseURL); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if baseURL != "https://api.example" {
		t.Fatalf("partial update committed: %s", baseURL)
	}
}
