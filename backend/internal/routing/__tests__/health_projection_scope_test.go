package routing_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestProjectHealthSecondaryGroupUsesCanonicalPrimaryRecoveryEvidence(t *testing.T) {
	store, db := healthEvidenceStore(t)
	now := time.Now().UTC()
	primarySince := now.Add(-5 * time.Minute).Format(time.RFC3339Nano)
	_, err := db.Exec(`UPDATE accounts SET schedulable=0,routing_state='fused' WHERE id='41';
		UPDATE account_groups SET group_id='10' WHERE account_id='41';
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','z-primary','2');
		INSERT INTO routing_decisions(account_id,group_name,routing_state,updated_at,payload_json)
		VALUES('41','z-primary','fused',?,json_object('state_since',?))`, primarySince, primarySince)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.PersistTrafficSamples(t.Context(), []business.TrafficSample{
		{AccountID: "41", GroupName: "codex", Result: "失败", EvidenceKey: "pre-primary-fuse", ObservedAt: now.Add(-10 * time.Minute).Format(time.RFC3339Nano), Payload: map[string]any{}},
		{AccountID: "41", GroupName: "codex", Result: "通过", EvidenceKey: "recovery", ObservedAt: now.Add(-time.Minute).Format(time.RFC3339Nano), Payload: map[string]any{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	service := routing.NewService(store)
	all, err := service.ProjectHealth(t.Context(), routing.Scope{})
	if err != nil {
		t.Fatal(err)
	}
	group := "codex"
	scoped, err := service.ProjectHealth(t.Context(), routing.Scope{GroupName: &group})
	if err != nil {
		t.Fatal(err)
	}
	health := scoped["41"]
	if len(all) != 1 || len(scoped) != 1 || health.HealthScore == nil || *health.HealthScore != 100 || health.SampleCount != 1 {
		t.Fatalf("secondary membership changed canonical recovery evidence: %+v", scoped)
	}
	health.HealthEvaluatedAt = all["41"].HealthEvaluatedAt
	if !reflect.DeepEqual(health, all["41"]) {
		t.Fatalf("group-scoped health differs from account health: all=%+v scoped=%+v", all["41"], health)
	}
	scheduled, err := service.Calculate(t.Context(), routing.Scope{GroupName: &group}, true)
	if err != nil {
		t.Fatal(err)
	}
	decision := scheduled.AccountDecisions["41"]
	if decision.GroupName != "z-primary" || decision.HealthScore != *health.HealthScore || int64(decision.SampleCount) != health.SampleCount {
		t.Fatalf("projection and scheduling chose different primary memberships: %+v", decision)
	}
}

func TestProjectHealthInapplicableAccountsHaveNoProjection(t *testing.T) {
	for _, reason := range []string{"manual_priority", "account_type", "group_excluded", "group_disabled", "external_control"} {
		t.Run(reason, func(t *testing.T) {
			store, db := healthEvidenceStore(t)
			if _, err := db.Exec(`UPDATE account_groups SET group_id='7' WHERE account_id='41'`); err != nil {
				t.Fatal(err)
			}
			var err error
			switch reason {
			case "manual_priority":
				_, err = db.Exec(`INSERT INTO manual_priority_accounts(account_id,priority,created_at,updated_at) VALUES('41',1,'now','now')`)
			case "account_type":
				_, err = store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"scope": map[string]any{"account_types": []any{"oauth"}}}}, "test")
			case "group_excluded":
				_, err = store.UpdatePolicy(t.Context(), map[string]any{"excluded_group_ids": []any{"7"}}, "test")
			case "group_disabled":
				_, err = db.Exec(`INSERT INTO local_groups(name,remote_id,strategy,strategy_source,updated_at) VALUES('codex','7','balanced','global_default','now')`)
				if err == nil {
					_, err = store.UpdateGroupPolicy(t.Context(), "7", map[string]any{
						"enabled": false, "strategy": "balanced", "min_pool_size": 1, "weight_budget": 1000,
						"balanced_price_ratio": 0.5, "breaker_enabled": true, "recovery_enabled": true,
						"weights_enabled": true, "scaling_enabled": false, "probe_enabled": true,
						"probe_interval_seconds": 300, "probe_model": "gpt-5",
					}, "test")
				}
			case "external_control":
				_, err = db.Exec(`INSERT INTO routing_baselines(account_id,captured_at,ownership_version) VALUES('41','now',2)`)
				if err == nil {
					_, err = store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{"scope": map[string]any{"manage_all_accounts": false}}}, "test")
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			projected, err := routing.NewService(store).ProjectHealth(t.Context(), routing.Scope{})
			if err != nil {
				t.Fatal(err)
			}
			if _, found := projected["41"]; found {
				t.Fatalf("inapplicable account acquired an empty health projection: %+v", projected)
			}
		})
	}
}
