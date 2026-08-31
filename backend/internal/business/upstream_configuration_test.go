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
	if err != nil || len(groups) != 1 || groups[0].EffectiveRate == nil || *groups[0].EffectiveRate != "0.666667" {
		t.Fatalf("unexpected groups: %#v err=%v", groups, err)
	}
}

func TestUpstreamGroupsRoundsPreviouslyStoredConvertedMultiplier(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	name := "Example"
	if _, err := store.CreateUpstreamConfiguration(ctx, UpstreamConfigurationWrite{
		Host: "api.example", Name: &name, BaseURL: "https://api.example",
		UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "10",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO upstream_groups(
		host,group_id,name,status,raw_rate,effective_rate,updated_at
	) VALUES('api.example','7','pro','active','1.1','0.11000000000000001','now')`); err != nil {
		t.Fatal(err)
	}
	groups, err := store.UpstreamGroups(ctx, "api.example", true)
	if err != nil || len(groups) != 1 || groups[0].EffectiveRate == nil || *groups[0].EffectiveRate != "0.11" {
		t.Fatalf("stored floating-point noise remains visible: groups=%#v err=%v", groups, err)
	}
	candidates, err := store.OnboardingCandidates(ctx, "api.example")
	if err != nil || len(candidates) != 1 || candidates[0].Multiplier == nil || *candidates[0].Multiplier != "0.11" {
		t.Fatalf("onboarding multiplier was not rounded: candidates=%#v err=%v", candidates, err)
	}
}

func TestConvertMultiplierRoundsHalfUpToSixDecimalPlaces(t *testing.T) {
	for _, test := range []struct {
		name     string
		raw      string
		recharge string
		want     string
	}{
		{name: "remove floating point noise", raw: "1.1000000000000001", recharge: "10", want: "0.11"},
		{name: "round seventh decimal up", raw: "1.234566", recharge: "10", want: "0.123457"},
		{name: "preserve meaningful decimals", raw: "1.98", recharge: "10", want: "0.198"},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, err := ConvertMultiplier(test.raw, test.recharge)
			if err != nil || value != test.want {
				t.Fatalf("ConvertMultiplier(%q, %q)=%q err=%v, want %q", test.raw, test.recharge, value, err, test.want)
			}
		})
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

func TestUpdateUpstreamClassificationPreservesConfiguration(t *testing.T) {
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
	if _, err := store.CreateUpstreamConfiguration(ctx, UpstreamConfigurationWrite{
		Host: "api.example", Name: &name, BaseURL: "https://edge.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_login", RechargeRate: "0.3",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateUpstreamClassification(ctx, "api.example", "newapi", "newapi_user_login"); err != nil {
		t.Fatal(err)
	}
	seed, err := store.UpstreamAuthSeed(ctx, "api.example")
	if err != nil || seed == nil {
		t.Fatalf("seed=%#v err=%v", seed, err)
	}
	if seed.BaseURL != "https://edge.example" || seed.UpstreamType != "newapi" || seed.AuthMode == nil || *seed.AuthMode != "newapi_user_login" {
		t.Fatalf("unexpected repaired seed: %#v", seed)
	}
	var recharge string
	if err := store.db.QueryRowContext(ctx, `SELECT recharge_rate FROM recharge_rates WHERE host='api.example'`).Scan(&recharge); err != nil {
		t.Fatal(err)
	}
	if recharge != "0.3" {
		t.Fatalf("recharge rate changed: %s", recharge)
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
