package business

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
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
