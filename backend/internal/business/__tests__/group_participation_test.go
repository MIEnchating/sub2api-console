package business_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func groupParticipationStore(t *testing.T) (*business.Store, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "groups.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(t.Context()); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO local_groups(name,remote_id,updated_at) VALUES('codex','7','now'),('7','8','now');
		INSERT INTO accounts(id,name,multiplier,schedulable,updated_at) VALUES('41','managed','1',1,'now');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','codex','7')`); err != nil {
		t.Fatal(err)
	}
	return store, db
}

func TestGroupParticipationMatchesOnlyStableIDs(t *testing.T) {
	for _, test := range []struct {
		name  string
		patch map[string]any
		want7 string
		want8 string
	}{
		{"selected ID does not include matching name", map[string]any{"advanced_policy": map[string]any{"scope": map[string]any{"managed_group_mode": "selected", "managed_group_ids": []any{"7"}}}}, "participating", "out_of_scope"},
		{"excluded ID does not exclude matching name", map[string]any{"excluded_group_ids": []any{"7"}}, "out_of_scope", "participating"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, _ := groupParticipationStore(t)
			snapshot, err := store.UpdatePolicy(t.Context(), test.patch, "test")
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"7": test.want7, "8": test.want8}
			groups, err := store.Groups(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			for _, group := range groups {
				if group.ID == nil || group.ParticipationStatus != want[*group.ID] {
					t.Errorf("group matched its name instead of ID: %+v", group)
				}
				if group.ID != nil && *group.ID == "8" && group.Status == "excluded" {
					t.Errorf("unexcluded group cannot be shown as excluded: %+v", group)
				}
			}
			for _, group := range snapshot.GroupStrategies {
				if group.ID == nil || group.ParticipationStatus != want[*group.ID] {
					t.Errorf("policy snapshot matched group name instead of ID: %+v", group)
				}
			}
		})
	}
}

func TestDisabledGroupReportsNotParticipatingUntilEnabledOrOverrideCleared(t *testing.T) {
	for _, restore := range []string{"enable", "clear"} {
		t.Run(restore, func(t *testing.T) {
			store, _ := groupParticipationStore(t)
			payload := map[string]any{
				"enabled": false, "strategy": nil, "min_pool_size": 1, "weight_budget": 400,
				"balanced_price_ratio": 0.5, "breaker_enabled": true, "recovery_enabled": true,
				"weights_enabled": true, "scaling_enabled": false, "probe_enabled": true,
				"probe_interval_seconds": 300, "probe_model": nil,
			}
			group, err := store.UpdateGroupPolicy(t.Context(), "7", payload, "test")
			if err != nil {
				t.Fatal(err)
			}
			if group.ParticipationStatus != "out_of_scope" || group.Status != "skipped" || group.ParticipationReason == nil {
				t.Fatalf("disabled group must show why it is not managed: %+v", group)
			}
			snapshot, err := store.PolicySnapshot(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			for _, strategy := range snapshot.GroupStrategies {
				if strategy.ID != nil && *strategy.ID == "7" && strategy.ParticipationStatus != "out_of_scope" {
					t.Fatalf("policy snapshot still reports disabled group participating: %+v", strategy)
				}
			}
			if restore == "enable" {
				payload["enabled"] = true
				group, err = store.UpdateGroupPolicy(t.Context(), "7", payload, "test")
			} else {
				group, err = store.ClearGroupPolicy(t.Context(), "7", "test")
			}
			if err != nil {
				t.Fatal(err)
			}
			if group.ParticipationStatus != "participating" || group.Status != "healthy" || group.ParticipationReason != nil {
				t.Fatalf("restored group must participate again: %+v", group)
			}
		})
	}
}
