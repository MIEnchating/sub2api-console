package business

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuthRecoveryProjectionStoresOnlyRedactedPublicOutcome(t *testing.T) {
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
		Host: "api.example", Name: &name, BaseURL: "https://api.example", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1",
	}); err != nil {
		t.Fatal(err)
	}
	reason, attempt, code := "Authorization=super-secret", "refresh_token=hidden", "refresh_failed"
	summary, err := store.PersistAuthRecoveryOutcomes(ctx, []AuthRecoveryOutcome{{
		Host: "HTTPS://API.EXAMPLE/", Success: false, Attempted: true, Transient: false,
		Code: &code, Reason: &reason, RefreshAttempt: &attempt,
	}}, "tester")
	if err != nil || summary.Hosts != 1 || summary.Failed != 1 {
		t.Fatalf("summary=%#v err=%v", summary, err)
	}
	var raw string
	if err := store.db.QueryRow(`SELECT value_json FROM operational_snapshots WHERE state_key='auth-recovery-runtime-snapshot'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "super-secret") || strings.Contains(raw, "refresh_token=hidden") {
		t.Fatalf("snapshot leaked credentials: %s", raw)
	}
	var payload struct {
		Results []AuthRecoveryOutcome `json:"results"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Results) != 1 || payload.Results[0].Reason == nil || strings.Contains(*payload.Results[0].Reason, "super-secret") || strings.Contains(*payload.Results[0].RefreshAttempt, "hidden") {
		t.Fatalf("snapshot outcome is not redacted: %#v", payload.Results)
	}
}

func TestAuthRecoveryFailureIgnoresSnapshotOlderThanCurrentUpstreamState(t *testing.T) {
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
		Host: "api.example", Name: &name, BaseURL: "https://api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", RechargeRate: "1",
	}); err != nil {
		t.Fatal(err)
	}
	reason, code := "HTTP 404", "vault_login_failed"
	if _, err := store.PersistAuthRecoveryOutcomes(ctx, []AuthRecoveryOutcome{{
		Host: "api.example", Success: false, Attempted: true, Code: &code, Reason: &reason,
	}}, "tester"); err != nil {
		t.Fatal(err)
	}
	newer := time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)
	if _, err := store.db.ExecContext(ctx, `UPDATE upstreams SET auth_status='已鉴权',updated_at=? WHERE host='api.example'`, newer); err != nil {
		t.Fatal(err)
	}
	if failure, err := store.authRecoveryFailure(ctx, "api.example", newer); err != nil || failure != nil {
		t.Fatalf("stale failure=%v err=%v", failure, err)
	}
}

func TestAuthRecoverySuccessProjectsLastSuccessfulMethod(t *testing.T) {
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
		Host: "api.example", Name: &name, BaseURL: "https://api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", RechargeRate: "1",
	}); err != nil {
		t.Fatal(err)
	}
	method, recoveryMethod := "sub2api_user_token", "refresh_token"
	if _, err := store.PersistAuthRecoveryOutcomes(ctx, []AuthRecoveryOutcome{{
		Host: "api.example", Success: true, Attempted: true, AuthMethod: &method, RefreshKind: &recoveryMethod,
	}}, "tester"); err != nil {
		t.Fatal(err)
	}
	upstreams, err := store.Upstreams(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(upstreams.Hosts) != 1 || upstreams.Hosts[0].LastAuthSuccessMethod == nil || *upstreams.Hosts[0].LastAuthSuccessMethod != method || upstreams.Hosts[0].LastAuthRecoveryMethod == nil || *upstreams.Hosts[0].LastAuthRecoveryMethod != recoveryMethod || upstreams.Hosts[0].LastAuthSuccessAt == nil {
		t.Fatalf("upstreams=%#v", upstreams.Hosts)
	}
}

func TestAuthRecoveryRequiredHostsAppliesRetryBackoff(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"fresh.example", "retried.example"} {
		name := host
		if _, err := store.CreateUpstreamConfiguration(ctx, UpstreamConfigurationWrite{
			Host: host, Name: &name, BaseURL: "https://" + host, UpstreamType: "sub2api",
			AuthMode: "sub2api_user_token", RechargeRate: "1",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.db.ExecContext(ctx, `UPDATE upstreams SET auth_status=? WHERE host=?`, UpstreamAuthStatusInvalid, host); err != nil {
			t.Fatal(err)
		}
	}
	reason := "refresh failed"
	if _, err := store.PersistAuthRecoveryOutcomes(ctx, []AuthRecoveryOutcome{{
		Host: "retried.example", Attempted: true, Reason: &reason,
	}}, "自动巡检"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	hosts, err := store.AuthRecoveryRequiredHosts(ctx, now.Add(-5*time.Minute))
	if err != nil || len(hosts) != 1 || hosts[0] != "fresh.example" {
		t.Fatalf("hosts=%#v err=%v", hosts, err)
	}
	hosts, err = store.AuthRecoveryRequiredHosts(ctx, now.Add(time.Minute))
	if err != nil || len(hosts) != 2 {
		t.Fatalf("hosts after backoff=%#v err=%v", hosts, err)
	}
}
