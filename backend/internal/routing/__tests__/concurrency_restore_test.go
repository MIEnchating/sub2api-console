package routing_test

import (
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func restorationAccount(id string) business.RoutingAccount {
	account := upstreamCapacityAccount(id, 1, 10)
	original, managed := int64(100), int64(1)
	account.BaselineConcurrency, account.ManagedConcurrency, account.HasRoutingBaseline = &original, &managed, true
	return account
}

func TestDisabledCapacityRestoresAllOwnedReductionsButPreservesProtectedValues(t *testing.T) {
	for _, scenario := range []string{"disabled", "outside selected upstream", "account override off", "upstream override off", "manual priority", "paused", "external value", "external ownership", "missing baseline", "original one", "unknown current", "unlimited current", "capacity enabled"} {
		t.Run(scenario, func(t *testing.T) {
			account := restorationAccount("41")
			r := reductionFixture(t, account, restorationAccount("42"))
			r.policy["upstream_concurrency"] = map[string]any{"enabled": false}
			wantRestore := false
			switch scenario {
			case "disabled":
				wantRestore = true
			case "outside selected upstream":
				r.policy["upstream_concurrency"] = map[string]any{"enabled": true, "account_mode": "upstreams", "upstream_ids": []any{"other"}}
				wantRestore = true
			case "account override off":
				r.policy["upstream_concurrency"] = map[string]any{"enabled": true, "account_overrides": map[string]any{"41": false}}
				wantRestore = true
			case "upstream override off":
				r.policy["upstream_concurrency"] = map[string]any{"enabled": true, "upstream_overrides": map[string]any{"upstream-1": false}}
				wantRestore = true
			case "manual priority":
				priority := int64(1)
				r.accounts[0].ManualPriority = &priority
			case "paused":
				r.accounts[0].Paused = true
			case "external value":
				current := int64(2)
				r.accounts[0].Concurrency = &current
			case "external ownership":
				r.accounts[0].ExternalControl = true
			case "missing baseline":
				r.accounts[0].BaselineConcurrency = nil
			case "original one":
				r.accounts[0].BaselineConcurrency = r.accounts[0].Concurrency
			case "unknown current":
				r.accounts[0].Concurrency = nil
			case "unlimited current":
				zero := int64(0)
				r.accounts[0].Concurrency = &zero
			case "capacity enabled":
				r.policy["upstream_concurrency"] = map[string]any{"enabled": true}
			}
			result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			target := result.AccountTargets["41"]
			if target.RestoreConcurrency != wantRestore {
				t.Fatalf("restore=%t, target=%+v", wantRestore, target)
			}
			if wantRestore && (target.Concurrency == nil || *target.Concurrency != 100) {
				t.Fatalf("must restore original concurrency: %+v", target)
			}
			if scenario == "disabled" && !result.AccountTargets["42"].RestoreConcurrency {
				t.Fatalf("restoration must apply to every eligible account: %+v", result.AccountTargets)
			}
		})
	}
}

func TestRestorationReservesCapacityBeforeAllocatingSelectedSibling(t *testing.T) {
	account := restorationAccount("41")
	original := int64(8)
	account.BaselineConcurrency = &original
	r := reductionFixture(t, account, upstreamCapacityAccount("42", 1, 10))
	r.policy["upstream_concurrency"] = map[string]any{"enabled": true, "account_mode": "selected", "account_ids": []any{"42"}}
	result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["42"]; target.Concurrency == nil || *target.Concurrency != 2 {
		t.Fatalf("selected sibling must reserve the restored eight slots: %+v", target)
	}
}

func TestRestorationComputesFollowingLoadAgainstRestoredConcurrency(t *testing.T) {
	zero := "0"
	for _, load := range []*string{nil, &zero} {
		account := restorationAccount("41")
		current := int64(51)
		account.Concurrency, account.ManagedConcurrency, account.LoadFactor = &current, &current, load
		r := reductionFixture(t, account)
		r.policy["upstream_concurrency"] = map[string]any{"enabled": false}
		result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
		if err != nil {
			t.Fatal(err)
		}
		target := result.AccountTargets["41"]
		if target.LoadFactor == nil || *target.LoadFactor != "51" {
			t.Fatalf("following load must not jump from 51 to restored concurrency 100: %+v", target)
		}
	}
}

func TestRestorationUsesStablePrimaryGroupCapacityPolicyAcrossMemberships(t *testing.T) {
	for _, primaryScaling := range []bool{false, true} {
		first := restorationAccount("41")
		second := first
		secondID := "8"
		second.GroupID, second.GroupName = &secondID, "another-group"
		r := reductionFixture(t, second, first)
		r.policy["upstream_concurrency"] = map[string]any{"enabled": false}
		r.policy["group_policy_bindings"] = map[string]any{
			"7": map[string]any{"enabled": true, "scaling_enabled": primaryScaling},
			"8": map[string]any{"enabled": true, "scaling_enabled": !primaryScaling},
		}
		result, err := routing.NewService(r).Calculate(t.Context(), routing.Scope{}, true)
		if err != nil {
			t.Fatal(err)
		}
		if target := result.AccountTargets["41"]; target.RestoreConcurrency == primaryScaling || len(result.AccountTargets) != 1 {
			t.Fatalf("primary group must determine one account-level restoration: %+v", result.AccountTargets)
		}
	}
}

func TestRestorationYieldsToPendingAbnormalCleanup(t *testing.T) {
	store, db := healthEvidenceStore(t)
	if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{
		"scaling": map[string]any{"enabled": false}, "upstream_concurrency": map[string]any{"enabled": false},
		"abnormal_cleanup": map[string]any{"enabled": true, "duration_minutes": 1, "keep_last_in_group": false},
	}}, "test"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(`UPDATE accounts SET concurrency=1 WHERE id='41';
		INSERT INTO routing_baselines(account_id,concurrency,managed_concurrency,captured_at) VALUES('41',100,1,'now');
		INSERT INTO abnormal_cleanup_states VALUES('41',?,?)`, now.Add(-time.Hour).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
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
	if target := result.AccountTargets["41"]; target.CleanupAction == nil || *target.CleanupAction != "pause" || target.RestoreConcurrency || target.Concurrency != nil {
		t.Fatalf("pending cleanup must not combine a concurrency restoration: %+v", target)
	}
}
