package routing_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
)

func costWallStore(t *testing.T, rate string) (*business.Store, *sql.DB) {
	t.Helper()
	store, db := healthEvidenceStore(t)
	if _, err := db.Exec(`INSERT INTO local_groups(name,remote_id,rate_multiplier,updated_at) VALUES('codex','7','1','now');
		UPDATE account_groups SET group_id='7';
		INSERT INTO accounts(id,name,multiplier,schedulable,metadata_json,updated_at) VALUES('42','affordable','0.5',1,'{}','now');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('42','codex','7')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE accounts SET multiplier=? WHERE id='41'`, rate); err != nil {
		t.Fatal(err)
	}
	return store, db
}

func TestCostWallClosesEqualAndAboveAccountsWhileAffordableAccountIsAvailable(t *testing.T) {
	for _, rate := range []string{"1", "1.25"} {
		t.Run(rate, func(t *testing.T) {
			store, _ := costWallStore(t, rate)
			id := "41"
			result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{AccountID: &id}, true)
			if err != nil {
				t.Fatal(err)
			}
			decision := result.AccountDecisions[id]
			if decision.Schedulable || decision.RoutingState != "cost_blocked" || decision.Weight != 0 {
				t.Fatalf("scoped calculation must close the cost-wall account while its peer is available: %+v", decision)
			}
		})
	}
}

func TestCostWallEnablesOnlyOneFallbackAndClosesItWhenAffordableAccountReturns(t *testing.T) {
	store, db := costWallStore(t, "1")
	if _, err := db.Exec(`UPDATE accounts SET paused=1,schedulable=0 WHERE id='42';
		INSERT INTO accounts(id,name,multiplier,schedulable,metadata_json,updated_at) VALUES('43','expensive','2',1,'{}','now');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('43','codex','7')`); err != nil {
		t.Fatal(err)
	}
	service := routing.NewService(store)
	result, err := service.Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if fallback := result.AccountDecisions["41"]; !fallback.Schedulable || fallback.RoutingState != "survivor" || fallback.Weight <= 0 {
		t.Fatalf("no available account must enable a cost fallback with traffic allocation: %+v", fallback)
	}
	if other := result.AccountDecisions["43"]; other.Schedulable || other.RoutingState != "cost_blocked" {
		t.Fatalf("fallback must not enable every expensive account: %+v", other)
	}
	if _, err := db.Exec(`UPDATE accounts SET routing_state='survivor',schedulable=1 WHERE id='41';
		UPDATE accounts SET paused=0,schedulable=1 WHERE id='42'`); err != nil {
		t.Fatal(err)
	}
	result, err = service.Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if fallback := result.AccountDecisions["41"]; fallback.Schedulable || fallback.RoutingState != "cost_blocked" {
		t.Fatalf("affordable recovery must close the former cost fallback: %+v", fallback)
	}
}

func TestCostWallFallbackDoesNotEnableManuallyPausedAccounts(t *testing.T) {
	store, db := costWallStore(t, "1.25")
	if _, err := db.Exec(`UPDATE accounts SET paused=1,schedulable=0`); err != nil {
		t.Fatal(err)
	}
	result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if decision := result.AccountDecisions["41"]; decision.Schedulable || decision.RoutingState != "paused" {
		t.Fatalf("lack of capacity must not bypass a manual pause: %+v", decision)
	}
}

func TestCostWallReopensPreviouslyClosedAboveWallAccountOnlyAsFallback(t *testing.T) {
	store, db := costWallStore(t, "1.25")
	if _, err := db.Exec(`UPDATE accounts SET schedulable=0,routing_state='cost_blocked' WHERE id='41';
		UPDATE accounts SET paused=1,schedulable=0 WHERE id='42'`); err != nil {
		t.Fatal(err)
	}
	id := "41"
	result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{AccountID: &id}, true)
	if err != nil {
		t.Fatal(err)
	}
	if decision := result.AccountDecisions[id]; !decision.Schedulable || decision.RoutingState != "survivor" || decision.Weight <= 0 {
		t.Fatalf("closed above-wall account must receive usable fallback allocation: %+v", decision)
	}
}

func TestCostWallKeepsAccountAvailableWhenAnotherManagedMembershipIsBelowWall(t *testing.T) {
	store, db := costWallStore(t, "1")
	if _, err := db.Exec(`INSERT INTO local_groups(name,remote_id,rate_multiplier,updated_at) VALUES('premium','8','2','now');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','premium','8')`); err != nil {
		t.Fatal(err)
	}
	result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if decision := result.AccountDecisions["41"]; !decision.Schedulable || decision.RoutingState == "cost_blocked" || decision.RoutingState == "survivor" {
		t.Fatalf("account-wide scheduling must preserve an affordable managed membership: %+v", decision)
	}
}

func TestCostWallProfitAdjustedEqualityClosesAccount(t *testing.T) {
	store, db := costWallStore(t, "0.8")
	if _, err := db.Exec(`UPDATE local_groups SET profit_control_enabled=1,profit_min_margin='0.1',profit_safety_buffer='0.1'`); err != nil {
		t.Fatal(err)
	}
	result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if decision := result.AccountDecisions["41"]; decision.Schedulable || decision.RoutingState != "cost_blocked" {
		t.Fatalf("equality uses the profit-adjusted cost wall: %+v", decision)
	}
}

func TestCostWallFallbackDoesNotReopenAccountWithFatalCredentialEvidence(t *testing.T) {
	store, db := costWallStore(t, "1.25")
	if _, err := db.Exec(`UPDATE accounts SET paused=1,schedulable=0 WHERE id='42'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PersistTrafficSamples(t.Context(), []business.TrafficSample{{
		AccountID: "41", GroupName: "codex", Result: "失败", EvidenceKey: "fatal-cost-fallback",
		ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), Payload: map[string]any{"status_code": 401},
	}}); err != nil {
		t.Fatal(err)
	}
	result, err := routing.NewService(store).Calculate(t.Context(), routing.Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if decision := result.AccountDecisions["41"]; decision.Schedulable {
		t.Fatalf("cost fallback must not bypass fatal credential evidence: %+v", decision)
	}
}
