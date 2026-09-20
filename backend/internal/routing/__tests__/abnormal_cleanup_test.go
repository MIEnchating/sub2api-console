package routing_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestPersistentAbnormalCleanupObservesIndependentDuration(t *testing.T) {
	for _, action := range []string{"pause", "disable", "delete"} {
		t.Run(action, func(t *testing.T) {
			store, db := healthEvidenceStore(t)
			_, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{
				"breaker":          map[string]any{"min_pool_size": 0},
				"abnormal_cleanup": map[string]any{"enabled": true, "action": action, "duration_minutes": 60, "keep_last_in_group": false},
			}}, "test")
			if err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			if _, err := store.PersistTrafficSamples(t.Context(), []business.TrafficSample{{AccountID: "41", GroupName: "codex", Result: "失败", EvidenceKey: "credential-error", ObservedAt: now.Add(-time.Second).Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 401}}}); err != nil {
				t.Fatal(err)
			}
			service := routing.NewService(store)
			result, err := service.Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			if result.AccountTargets["41"].CleanupAction != nil {
				t.Fatal("first observation disposed of account")
			}
			states, err := store.AbnormalCleanupStates(t.Context(), nil)
			if err != nil || states["41"].IsZero() {
				t.Fatalf("observation not persisted: %v %v", states, err)
			}
			authStates, err := store.CleanupStates(t.Context(), nil)
			if err != nil || len(authStates) != 0 {
				t.Fatalf("shared authentication observation: %v %v", authStates, err)
			}
			if _, err := db.Exec(`UPDATE abnormal_cleanup_states SET eligible_since=? WHERE account_id='41'`, now.Add(-time.Hour).Format(time.RFC3339Nano)); err != nil {
				t.Fatal(err)
			}
			// A new service instance must resume the durable observation.
			result, err = routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			if got := result.AccountTargets["41"].CleanupAction; got == nil || *got != action {
				t.Fatalf("elapsed observation did not queue %s: %+v", action, result.AccountDecisions["41"])
			}
		})
	}
}

func TestPersistentAbnormalCleanupResetsIneligibleObservations(t *testing.T) {
	for _, scenario := range []string{"disabled", "success", "stale", "rate-limit", "manual-fuse", "manual-pause", "excluded", "no-evidence", "intervening-success", "manual-priority"} {
		t.Run(scenario, func(t *testing.T) {
			store, db := healthEvidenceStore(t)
			policy := map[string]any{"breaker": map[string]any{"min_pool_size": 0}, "abnormal_cleanup": map[string]any{"enabled": scenario != "disabled", "duration_minutes": 1, "keep_last_in_group": false}}
			if scenario == "manual-fuse" {
				policy["scope"] = map[string]any{"manual_fused_account_ids": []any{"41"}}
			}
			if scenario == "excluded" {
				policy["scope"] = map[string]any{"excluded_account_ids": []any{"41"}}
			}
			if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": policy}, "test"); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			if _, err := db.Exec(`INSERT INTO abnormal_cleanup_states VALUES('41',?,?); UPDATE accounts SET routing_state='fused' WHERE id='41'`, now.Add(-time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
				t.Fatal(err)
			}
			if scenario == "manual-pause" {
				if _, err := db.Exec(`UPDATE accounts SET paused=1,paused_reason='人工暂停' WHERE id='41'`); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "manual-priority" {
				if _, err := db.Exec(`INSERT INTO manual_priority_accounts(account_id,priority,created_at,updated_at) VALUES('41',1,'now','now')`); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "intervening-success" {
				if _, err := store.PersistTrafficSamples(t.Context(), []business.TrafficSample{{AccountID: "41", GroupName: "codex", Result: "通过", EvidenceKey: "intervening-success", ObservedAt: now.Add(-2 * time.Second).Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 200}}}); err != nil {
					t.Fatal(err)
				}
			}
			age, code, outcome := time.Second, 401, "失败"
			if scenario == "stale" {
				age = 48 * time.Hour
			}
			if scenario == "success" {
				code = 200
				outcome = "通过"
			}
			if scenario == "rate-limit" {
				code = 429
			}
			if scenario != "no-evidence" {
				if _, err := store.PersistTrafficSamples(t.Context(), []business.TrafficSample{{AccountID: "41", GroupName: "codex", Result: outcome, EvidenceKey: "latest", ObservedAt: now.Add(-age).Format(time.RFC3339Nano), Payload: map[string]any{"status_code": code}}}); err != nil {
					t.Fatal(err)
				}
			}
			result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			if result.AccountTargets["41"].CleanupAction != nil {
				t.Fatalf("protected account queued: %+v", result.AccountDecisions["41"])
			}
			states, err := store.AbnormalCleanupStates(t.Context(), nil)
			if err != nil || len(states) != 0 {
				t.Fatalf("observation survived reset: %v %v", states, err)
			}
		})
	}
}

func TestPersistentAbnormalCleanupHonorsBatchAndLastGroupMember(t *testing.T) {
	for _, keepLast := range []bool{false, true} {
		t.Run(fmt.Sprint(keepLast), func(t *testing.T) {
			store, db := healthEvidenceStore(t)
			if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{
				"breaker": map[string]any{"min_pool_size": 0}, "abnormal_cleanup": map[string]any{"enabled": true, "duration_minutes": 1, "max_per_round": 1, "keep_last_in_group": keepLast},
			}}, "test"); err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			if !keepLast {
				if _, err := db.Exec(`INSERT INTO accounts(id,name,multiplier,schedulable,metadata_json,updated_at) VALUES('42','second','1',1,'{}','now'); INSERT INTO account_groups(account_id,group_name) VALUES('42','codex')`); err != nil {
					t.Fatal(err)
				}
			}
			ids := []string{"41"}
			if !keepLast {
				ids = append(ids, "42")
			}
			for _, id := range ids {
				if _, err := db.Exec(`INSERT INTO abnormal_cleanup_states VALUES(?,?,?)`, id, now.Add(-time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
					t.Fatal(err)
				}
				if _, err := store.PersistTrafficSamples(t.Context(), []business.TrafficSample{{AccountID: id, GroupName: "codex", Result: "失败", EvidenceKey: id, ObservedAt: now.Add(-time.Second).Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 401}}}); err != nil {
					t.Fatal(err)
				}
			}
			result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			queued := 0
			for _, target := range result.AccountTargets {
				if target.CleanupAction != nil {
					queued++
				}
			}
			expected := 1
			if keepLast {
				expected = 0
			}
			if queued != expected {
				t.Fatalf("queued=%d expected=%d", queued, expected)
			}
		})
	}
}

func TestPersistentAbnormalCleanupIncludesRepeatedGatewayFailures(t *testing.T) {
	store, db := healthEvidenceStore(t)
	if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{
		"abnormal_cleanup": map[string]any{"enabled": true, "duration_minutes": 1, "keep_last_in_group": false},
	}}, "test"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO abnormal_cleanup_states VALUES('41',?,?)`, now.Add(-time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 3; i++ {
		at := now.Add(-time.Duration(i) * time.Second).Format(time.RFC3339Nano)
		if _, err := store.PersistTrafficSamples(t.Context(), []business.TrafficSample{{AccountID: "41", GroupName: "codex", Result: "失败", EvidenceKey: at, ObservedAt: at, Payload: map[string]any{"status_code": 502}}}); err != nil {
			t.Fatal(err)
		}
	}
	result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	target := result.AccountTargets["41"]
	if target.CleanupAction == nil || *target.CleanupAction != "pause" || target.CleanupReason != "长期异常" {
		t.Fatalf("gateway degradation was not eligible: %+v %+v", target, result.AccountDecisions["41"])
	}
}

func TestPersistentAbnormalUsesProbeFreshnessIndependentlyOfTrafficWindow(t *testing.T) {
	store, db := healthEvidenceStore(t)
	if _, err := store.UpdatePolicy(t.Context(), map[string]any{"traffic_lookback_minutes": 1, "advanced_policy": map[string]any{
		"breaker":          map[string]any{"min_pool_size": 0},
		"probe":            map[string]any{"freshness_seconds": 900},
		"abnormal_cleanup": map[string]any{"enabled": true, "duration_minutes": 1, "keep_last_in_group": false},
	}}, "test"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO abnormal_cleanup_states VALUES('41',?,?)`, now.Add(-time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	reason := "invalid api key"
	if _, err := store.PersistProbeSamples(t.Context(), []business.ProbeSample{{AccountID: "41", GroupName: "codex", Result: "失败", FailureReason: &reason, ObservedAt: now.Add(-2 * time.Minute).Format(time.RFC3339Nano)}}); err != nil {
		t.Fatal(err)
	}
	result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.AccountTargets["41"].CleanupAction == nil {
		t.Fatalf("fresh probe discarded using traffic window: %+v", result.AccountDecisions["41"])
	}
}
