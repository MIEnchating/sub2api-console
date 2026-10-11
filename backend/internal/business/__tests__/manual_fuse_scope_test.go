package business_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func manualFuseScopeStore(t testing.TB) (*business.Store, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manual-fuse.db")
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
	policy := business.DefaultAlertPolicy()
	policy.CostTrafficEnabled, policy.ProbeEnabled = false, false
	policy.GroupUnavailableEnabled, policy.GroupSurvivorEnabled = false, false
	raw, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateAlertPolicy(t.Context(), document); err != nil {
		t.Fatal(err)
	}
	return store, db
}

func setManualFuseScope(t testing.TB, store *business.Store, ids []any) {
	t.Helper()
	if _, err := store.UpdatePolicy(t.Context(), map[string]any{"advanced_policy": map[string]any{
		"scope": map[string]any{"manual_fused_account_ids": ids},
	}}, "test"); err != nil {
		t.Fatal(err)
	}
}

func TestManualFuseScopeSkipsSelectedAccountsAndKeepsAutomaticBreakerForUnselectedID(t *testing.T) {
	store, db := manualFuseScopeStore(t)
	for _, statement := range []string{
		`INSERT INTO accounts(id,name,updated_at) VALUES('2','same','now'),('10','same','now'),('30','empty','now'),('40','unselected','now')`,
		`INSERT INTO account_groups(account_id,group_name) VALUES('2','z'),('2','a'),('10','z'),('10','a'),('40','other')`,
		`INSERT INTO routing_decisions(account_id,group_name,routing_state,schedulable,updated_at) VALUES('2','routing-group','fused',0,'2099-01-01T00:00:00Z'),('40','other','fused',0,'2099-01-01T00:00:00Z')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	setManualFuseScope(t, store, []any{"30", "2", "10", "2", "999"})
	for range 2 {
		if _, err := store.EvaluateAlertIncidents(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := db.Query(`SELECT incident_key FROM alert_incidents WHERE event_type='account.routing_breaker' AND status='firing' ORDER BY object_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := []string{"console:routing:breaker:40:other"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("breaker incidents=%v want=%v", keys, want)
	}
}

func TestManualFuseScopeEmptyDoesNotFuseUnselectedAccounts(t *testing.T) {
	store, db := manualFuseScopeStore(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,updated_at) VALUES('2','unselected','now')`); err != nil {
		t.Fatal(err)
	}
	setManualFuseScope(t, store, []any{})
	if _, err := store.EvaluateAlertIncidents(t.Context()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM alert_incidents WHERE cause_code='MANUAL_FUSE'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("manual count=%d err=%v", count, err)
	}
}

func TestManualFuseScopeLargeSelectionDoesNotGenerateBreakerAlerts(t *testing.T) {
	store, db := manualFuseScopeStore(t)
	if _, err := db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<1100)
		INSERT INTO accounts(id,name,updated_at) SELECT CAST(x AS TEXT),'selected','now' FROM n;
		INSERT INTO routing_decisions(account_id,group_name,routing_state,schedulable,updated_at)
		SELECT id,'group','fused',0,'2099-01-01T00:00:00Z' FROM accounts`); err != nil {
		t.Fatal(err)
	}
	ids := make([]any, 1100)
	for index := range ids {
		ids[index] = strconv.Itoa(index + 1)
	}
	setManualFuseScope(t, store, ids)
	if _, err := store.EvaluateAlertIncidents(t.Context()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM alert_incidents WHERE event_type='account.routing_breaker' AND status='firing'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("manual count=%d err=%v", count, err)
	}
}

func TestManualFuseScopeCancelledEvaluationDoesNotCreateIncidents(t *testing.T) {
	store, db := manualFuseScopeStore(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,updated_at) VALUES('2','selected','now')`); err != nil {
		t.Fatal(err)
	}
	setManualFuseScope(t, store, []any{"2"})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.EvaluateAlertIncidents(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("evaluation error=%v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM alert_incidents`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("incidents=%d err=%v", count, err)
	}
}
