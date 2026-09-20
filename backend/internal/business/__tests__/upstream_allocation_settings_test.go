package business_test

import (
	"errors"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestAllocationSettingsOverridesResetAndNewAccountInheritance(t *testing.T) {
	store, db := concurrencyStore(t)
	var upstreamID string
	if err := db.QueryRow(`SELECT upstream_id FROM upstream_identity_hosts WHERE host='fixture.example'`).Scan(&upstreamID); err != nil {
		t.Fatal(err)
	}
	policy, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"upstream_concurrency": map[string]any{"enabled": true, "account_mode": "selected", "account_ids": []any{}}}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	yes, no := true, false
	status, err := store.SetUpstreamAllocationSetting(t.Context(), "upstreams", upstreamID, business.UpstreamAllocationUpdate{Override: &yes, ExpectedRevision: policy.Revision}, "test")
	if err != nil || !status.Effective {
		t.Fatalf("upstream opt-in: %+v %v", status, err)
	}
	if _, err := db.Exec(`INSERT INTO accounts(id,name,upstream_host,updated_at) VALUES('147','new','fixture.example','now')`); err != nil {
		t.Fatal(err)
	}
	account, err := store.UpstreamAllocationSetting(t.Context(), "accounts", "147")
	if err != nil || !account.Effective || account.Source != "upstream" || account.Override != nil {
		t.Fatalf("new account inheritance: %+v %v", account, err)
	}
	account, err = store.SetUpstreamAllocationSetting(t.Context(), "accounts", "147", business.UpstreamAllocationUpdate{Override: &no, ExpectedRevision: account.Revision, ExpectedUpstreamID: upstreamID}, "test")
	if err != nil || account.Effective || account.Override == nil || *account.Override {
		t.Fatalf("account off: %+v %v", account, err)
	}
	account, err = store.SetUpstreamAllocationSetting(t.Context(), "accounts", "147", business.UpstreamAllocationUpdate{ExpectedRevision: account.Revision, ExpectedUpstreamID: upstreamID}, "test")
	if err != nil || !account.Effective || account.Source != "upstream" || account.Override != nil {
		t.Fatalf("reset: %+v %v", account, err)
	}
	_, err = store.SetUpstreamAllocationSetting(t.Context(), "upstreams", upstreamID, business.UpstreamAllocationUpdate{Override: &no, ExpectedRevision: policy.Revision}, "test")
	if !errors.Is(err, business.ErrPolicyRevisionConflict) {
		t.Fatalf("stale policy must reject: %v", err)
	}
	_, err = store.SetUpstreamAllocationSetting(t.Context(), "accounts", "147", business.UpstreamAllocationUpdate{Override: &no, ExpectedRevision: account.Revision, ExpectedUpstreamID: "other"}, "test")
	if err == nil {
		t.Fatal("changed binding accepted")
	}
	latest, err := store.UpstreamAllocationSetting(t.Context(), "accounts", "147")
	if err != nil || !latest.Effective || latest.Revision != account.Revision {
		t.Fatalf("rejected write changed policy: %+v %v", latest, err)
	}
}

func TestAllocationSettingsMasterOffPreservesExplicitIntentWithoutEnablingExecution(t *testing.T) {
	store, db := concurrencyStore(t)
	var id string
	if err := db.QueryRow(`SELECT upstream_id FROM upstream_identity_hosts WHERE host='fixture.example'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	policy, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"upstream_concurrency": map[string]any{"enabled": false, "account_mode": "selected"}}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	yes := true
	status, err := store.SetUpstreamAllocationSetting(t.Context(), "upstreams", id, business.UpstreamAllocationUpdate{Override: &yes, ExpectedRevision: policy.Revision}, "test")
	if err != nil || status.Effective || status.GlobalEnabled || !status.Selected {
		t.Fatalf("master disabled: %+v %v", status, err)
	}
}
