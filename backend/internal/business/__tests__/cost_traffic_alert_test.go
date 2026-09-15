package business_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func seedCostTraffic(t *testing.T, db *sql.DB, rate any, groupRate any, source string, age time.Duration) {
	t.Helper()
	now := time.Now().UTC()
	for _, statement := range []string{
		`INSERT INTO accounts(id,name,multiplier,updated_at) VALUES('41','同名账号','0.3','now')`,
		`INSERT INTO local_groups(name,remote_id,rate_multiplier,updated_at) VALUES('平价','7','0.3','now'),('旗舰','8','0.6','now')`,
		`INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','平价','7'),('41','旗舰','8')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`UPDATE accounts SET multiplier=? WHERE id='41'`, rate); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE local_groups SET rate_multiplier=? WHERE remote_id='7'`, groupRate); err != nil {
		t.Fatal(err)
	}
	if source != "" {
		if _, err := db.Exec(`INSERT INTO usage_records(request_id,account_id,group_name,observed_at,source,payload_json) VALUES('request-1','41','平价',?,?, '{"request_group_id":"7"}')`, now.Add(-age).Format(time.RFC3339Nano), source); err != nil {
			t.Fatal(err)
		}
	}
}

func evaluateCostTraffic(t *testing.T, store *business.Store, db *sql.DB) (int, string) {
	t.Helper()
	if _, err := store.EvaluateAlertIncidents(context.Background()); err != nil {
		t.Fatal(err)
	}
	var count int
	var cause string
	if err := db.QueryRow(`SELECT COUNT(*),COALESCE(MAX(cause_code),'') FROM alert_incidents WHERE event_type='account.cost_traffic' AND status='firing'`).Scan(&count, &cause); err != nil {
		t.Fatal(err)
	}
	return count, cause
}

func TestCostTrafficUsesActualRecentRequestsAndExactDecimalComparison(t *testing.T) {
	for _, tc := range []struct {
		name            string
		rate, groupRate any
		source          string
		age             time.Duration
		want            int
	}{
		{"equal cost with traffic", "0.3", "0.30", "traffic", time.Minute, 1},
		{"higher cost with traffic", "0.31", "0.3", "traffic", time.Minute, 1},
		{"lower cost at full decimal precision", "0.299999999999999999", "0.3", "traffic", time.Minute, 0},
		{"no traffic", "0.3", "0.3", "", 0, 0},
		{"probe is not traffic", "0.3", "0.3", "active-probe", time.Minute, 0},
		{"expired traffic", "0.3", "0.3", "traffic", 6 * time.Minute, 0},
		{"future traffic", "0.3", "0.3", "traffic", -time.Minute, 0},
		{"missing account rate", nil, "0.3", "traffic", time.Minute, 0},
		{"invalid group rate", "0.3", "invalid", "traffic", time.Minute, 0},
		{"negative account rate", "-1", "0.3", "traffic", time.Minute, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, db := openAlertRuleStore(t)
			seedCostTraffic(t, db, tc.rate, tc.groupRate, tc.source, tc.age)
			count, cause := evaluateCostTraffic(t, store, db)
			if count != tc.want {
				t.Fatalf("firing=%d want=%d cause=%s", count, tc.want, cause)
			}
			if tc.want > 0 {
				for _, detail := range []string{"当前账号倍率", "分组倍率", "最近 5 分钟", "1 次", "最近调用"} {
					if !strings.Contains(cause, detail) {
						t.Fatalf("missing %s in %s", detail, cause)
					}
				}
			}
		})
	}
}

func TestCostTrafficDeduplicatesRequestsAndSeparatesActualGroup(t *testing.T) {
	store, db := openAlertRuleStore(t)
	seedCostTraffic(t, db, "0.3", "0.3", "traffic", time.Minute)
	if _, err := db.Exec(`INSERT INTO usage_records(request_id,account_id,group_name,observed_at,source) VALUES
 ('request-1','41','平价',?,'traffic'),('request-2','41','平价',?,'traffic'),('request-3','41','旗舰',?,'traffic'),('other','42','平价',?,'traffic')`, time.Now().UTC().Add(-30*time.Second).Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE usage_records SET payload_json=CASE WHEN group_name='平价' THEN '{"request_group_id":"7"}' ELSE '{"request_group_id":"8"}' END`); err != nil {
		t.Fatal(err)
	}
	count, cause := evaluateCostTraffic(t, store, db)
	if count != 1 || !strings.Contains(cause, "2 次") {
		t.Fatalf("count=%d cause=%s", count, cause)
	}
	var key string
	if err := db.QueryRow(`SELECT incident_key FROM alert_incidents WHERE event_type='account.cost_traffic'`).Scan(&key); err != nil {
		t.Fatal(err)
	}
	if key != "console:cost-traffic:41:平价" {
		t.Fatalf("unexpected group scope: %s", key)
	}
}

func TestCostTrafficReconcilesRecoveryAndDisabledRule(t *testing.T) {
	for _, tc := range []struct{ name, statement, status string }{
		{"price drops", `UPDATE accounts SET multiplier='0.2'`, "recovered"},
		{"traffic leaves window", `UPDATE usage_records SET observed_at='2000-01-01T00:00:00Z'`, "recovered"},
		{"missing price does not announce recovery", `UPDATE accounts SET multiplier=NULL`, "closed"},
		{"binding changes do not announce recovery", `UPDATE account_groups SET group_id='99' WHERE group_name='平价'`, "closed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, db := openAlertRuleStore(t)
			seedCostTraffic(t, db, "0.3", "0.3", "traffic", time.Minute)
			if count, _ := evaluateCostTraffic(t, store, db); count != 1 {
				t.Fatal("expected initial alert")
			}
			if _, err := db.Exec(tc.statement); err != nil {
				t.Fatal(err)
			}
			evaluateCostTraffic(t, store, db)
			var status string
			if err := db.QueryRow(`SELECT status FROM alert_incidents WHERE event_type='account.cost_traffic'`).Scan(&status); err != nil {
				t.Fatal(err)
			}
			if status != tc.status {
				t.Fatalf("status=%s want=%s", status, tc.status)
			}
		})
	}
}

func TestCostTrafficDisabledRuleSuppressesPendingNotifications(t *testing.T) {
	store, db := openAlertRuleStore(t)
	seedCostTraffic(t, db, "0.3", "0.3", "traffic", time.Minute)
	if count, _ := evaluateCostTraffic(t, store, db); count != 1 {
		t.Fatal("expected initial alert")
	}
	policy := business.DefaultAlertPolicy()
	policy.CostTrafficEnabled = false
	policy.RecoveryNotificationTypes = []string{"cost_traffic"}
	updateAlertRulePolicy(t, store, policy)
	if count, _ := evaluateCostTraffic(t, store, db); count != 0 {
		t.Fatal("disabled rule still firing")
	}
	queue, err := store.NotificationQueueDetails(t.Context(), "fixture-channel", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue.ConsumerItems) != 0 {
		t.Fatalf("disabled rule still queued: %+v", queue.ConsumerItems)
	}
}

func TestCostTrafficRecoversOneGroupWhileAnotherRemainsFiring(t *testing.T) {
	store, db := openAlertRuleStore(t)
	seedCostTraffic(t, db, "0.7", "0.3", "traffic", time.Minute)
	if _, err := db.Exec(`INSERT INTO usage_records(request_id,account_id,group_name,observed_at,source,payload_json)
 VALUES('flagship-request','41','旗舰',?,'traffic','{"request_group_id":"8"}')`, time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	if count, _ := evaluateCostTraffic(t, store, db); count != 2 {
		t.Fatalf("expected two group alerts, got %d", count)
	}
	if _, err := db.Exec(`UPDATE accounts SET multiplier='0.4' WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	if count, _ := evaluateCostTraffic(t, store, db); count != 1 {
		t.Fatalf("expected one remaining group, got %d", count)
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM alert_incidents WHERE incident_key='console:cost-traffic:41:旗舰'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "recovered" {
		t.Fatalf("profitable group recovery suppressed by unrelated group: %s", status)
	}
}
