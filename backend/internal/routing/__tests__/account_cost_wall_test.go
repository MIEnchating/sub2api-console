package routing_test

import (
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func TestAccountCostWallExemptionRestoresWeightAndCanBeDisabled(t *testing.T) {
	for _, rate := range []string{"1", "1.25"} {
		t.Run(rate, func(t *testing.T) {
			store, db := costWallStore(t, rate)
			if _, err := db.Exec(`UPDATE accounts SET schedulable=0,routing_state='cost_blocked' WHERE id='41'`); err != nil {
				t.Fatal(err)
			}
			for _, enabled := range []bool{true, false} {
				if err := store.SetAccountIgnoreCostWall(t.Context(), "41", enabled, "test"); err != nil {
					t.Fatal(err)
				}
				result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
				if err != nil {
					t.Fatal(err)
				}
				d := result.AccountDecisions["41"]
				if enabled {
					if !d.Schedulable || d.Weight <= 0 || d.RoutingState == "survivor" || d.RoutingState == "cost_blocked" {
						t.Fatalf("exemption did not restore ordinary allocation: %+v", d)
					}
				} else if d.Schedulable || d.Weight != 0 || d.RoutingState != "cost_blocked" {
					t.Fatalf("disabled exemption did not restore wall: %+v", d)
				}
			}
		})
	}
}

func TestAccountCostWallExemptionAllowsProbesAcrossAllMemberships(t *testing.T) {
	store, db := costWallStore(t, "1.25")
	if _, err := db.Exec(`INSERT INTO local_groups(name,remote_id,rate_multiplier,updated_at) VALUES('premium','8','0.8','now'); INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','premium','8')`); err != nil {
		t.Fatal(err)
	}
	accounts, err := store.RoutingAccounts(t.Context(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, true, false} {
		if err := store.SetAccountIgnoreCostWall(t.Context(), "41", enabled, "test"); err != nil {
			t.Fatal(err)
		}
		policy, err := store.ControlPolicy(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		blocked, err := routing.CostWallProbeBlocks(accounts, policy)
		if err != nil || blocked["41"] == enabled {
			t.Fatalf("enabled=%v blocks=%v err=%v", enabled, blocked, err)
		}
	}
}

func TestExemptAccountPreventsUnnecessaryCostFallback(t *testing.T) {
	store, db := costWallStore(t, "1.25")
	if _, err := db.Exec(`UPDATE accounts SET multiplier='2' WHERE id='42'`); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAccountIgnoreCostWall(t.Context(), "42", true, "test"); err != nil {
		t.Fatal(err)
	}
	result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if d := result.AccountDecisions["41"]; d.Schedulable || d.RoutingState != "cost_blocked" {
		t.Fatalf("unnecessary fallback: %+v", d)
	}
	if d := result.AccountDecisions["42"]; !d.Schedulable || d.Weight <= 0 || d.RoutingState == "survivor" {
		t.Fatalf("exempt account unavailable: %+v", d)
	}
}

func TestAccountCostWallExemptionPreservesOtherRestrictions(t *testing.T) {
	for _, test := range []struct{ name, sql, state string }{
		{"manual pause", `UPDATE accounts SET paused=1,schedulable=0 WHERE id='41'`, "paused"},
		{"fused", `UPDATE accounts SET routing_state='fused',schedulable=0 WHERE id='41'`, "fused"},
		{"unknown scheduling", `UPDATE accounts SET schedulable=NULL WHERE id='41'`, "unknown"},
		{"disabled", `UPDATE accounts SET metadata_json='{"status":"disabled"}' WHERE id='41'`, "disabled"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, db := costWallStore(t, "1.25")
			if _, err := db.Exec(test.sql); err != nil {
				t.Fatal(err)
			}
			if err := store.SetAccountIgnoreCostWall(t.Context(), "41", true, "test"); err != nil {
				t.Fatal(err)
			}
			result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
			if err != nil {
				t.Fatal(err)
			}
			if d := result.AccountDecisions["41"]; d.Schedulable || d.RoutingState != test.state || d.Weight != 0 {
				t.Fatalf("exemption bypassed %s: %+v", test.name, d)
			}
		})
	}
}
