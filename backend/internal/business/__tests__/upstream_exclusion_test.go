package business_test

import (
	"database/sql"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"path/filepath"
	"testing"
)

func TestUpstreamExclusionUpgradeAndRestartPreserveExistingAccounts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{Host: "fixture.example", BaseURL: "https://fixture.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1"}); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`DROP TABLE upstream_exclusions; INSERT INTO accounts(id,name,upstream_host,schedulable,updated_at) VALUES('41','preserved','fixture.example',1,'now')`); err != nil {
		t.Fatal(err)
	}
	store, err = business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := store.UpstreamExclusionPreview(t.Context(), "fixture.example")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SetUpstreamExcluded(t.Context(), preview.UpstreamID, true, "test"); err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	protection, err := store.AccountMutationProtection(t.Context(), "41")
	if err != nil || !protection.UpstreamExcluded {
		t.Fatalf("reopened protection: %+v %v", protection, err)
	}
	var enabled bool
	if err = db.QueryRow(`SELECT schedulable FROM accounts WHERE id='41'`).Scan(&enabled); err != nil || !enabled {
		t.Fatalf("upgrade changed scheduling: %v %v", enabled, err)
	}
}

func TestUpstreamExclusionProtectsExistingAndNewAccountsWithoutChangingScheduling(t *testing.T) {
	store, db := concurrencyStore(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,upstream_host,schedulable,updated_at) VALUES('41','existing','fixture.example',1,'now')`); err != nil {
		t.Fatal(err)
	}
	preview, err := store.UpstreamExclusionPreview(t.Context(), "fixture.example")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetUpstreamExcluded(t.Context(), preview.UpstreamID, true, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO accounts(id,name,upstream_host,schedulable,updated_at) VALUES('42','new','fixture.example',1,'now'),('43','unrelated','other.example',1,'now')`); err != nil {
		t.Fatal(err)
	}
	protections, err := store.AccountMutationProtections(t.Context(), []string{"41", "42", "43"})
	if err != nil {
		t.Fatal(err)
	}
	if !protections["41"].UpstreamExcluded || !protections["42"].Protected() || protections["43"].Protected() {
		t.Fatalf("unexpected protection: %+v", protections)
	}
	var scheduling int
	if err := db.QueryRow(`SELECT schedulable FROM accounts WHERE id='41'`).Scan(&scheduling); err != nil || scheduling != 1 {
		t.Fatalf("scheduling changed: %d %v", scheduling, err)
	}
	summary, err := store.Upstreams(t.Context())
	if err != nil || len(summary.Hosts) != 1 || !summary.Hosts[0].Excluded {
		t.Fatalf("summary: %+v %v", summary, err)
	}
	if err := store.SetUpstreamExcluded(t.Context(), preview.UpstreamID, false, "test"); err != nil {
		t.Fatal(err)
	}
	protection, err := store.AccountMutationProtection(t.Context(), "41")
	if err != nil || protection.UpstreamExcluded {
		t.Fatalf("restore: %+v %v", protection, err)
	}
}

func TestUpstreamExclusionSurvivesRenameAndExcludesProbeAndEvidenceTargets(t *testing.T) {
	store, db := concurrencyStore(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,upstream_host,schedulable,updated_at) VALUES('41','bound','fixture.example',1,'now'); INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','group','7')`); err != nil {
		t.Fatal(err)
	}
	preview, err := store.UpstreamExclusionPreview(t.Context(), "fixture.example")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SetUpstreamExcluded(t.Context(), preview.UpstreamID, true, "test"); err != nil {
		t.Fatal(err)
	}
	if err = store.RenameUpstreamHost(t.Context(), "fixture.example", "renamed.example"); err != nil {
		t.Fatal(err)
	}
	excluded, err := store.UpstreamExcluded(t.Context(), "renamed.example")
	if err != nil || !excluded {
		t.Fatalf("renamed exclusion: %v %v", excluded, err)
	}
	targets, err := store.EvidenceTargets(t.Context(), nil, nil)
	if err != nil || len(targets) != 0 {
		t.Fatalf("evidence targets: %+v %v", targets, err)
	}
	probes, err := store.ProbeCandidates(t.Context(), nil, nil, nil)
	if err != nil || len(probes) != 0 {
		t.Fatalf("probe targets: %+v %v", probes, err)
	}
	routing, err := store.RoutingAccounts(t.Context(), nil, nil)
	if err != nil || len(routing) != 1 || !routing[0].UpstreamExcluded {
		t.Fatalf("routing exclusion: %+v %v", routing, err)
	}
}
