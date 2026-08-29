package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestOpenReservesWriteLockWhenTransactionBegins(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	first, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Rollback()

	type transactionResult struct {
		transaction *sql.Tx
		err         error
	}
	result := make(chan transactionResult, 1)
	go func() {
		second, beginErr := store.db.BeginTx(context.Background(), nil)
		result <- transactionResult{transaction: second, err: beginErr}
	}()

	select {
	case second := <-result:
		if second.transaction != nil {
			_ = second.transaction.Rollback()
		}
		t.Fatal("second writer began before the active write transaction released its lock")
	case <-time.After(50 * time.Millisecond):
	}
	if err := first.Rollback(); err != nil {
		t.Fatal(err)
	}
	select {
	case second := <-result:
		if second.err != nil {
			t.Fatal(second.err)
		}
		if err := second.transaction.Rollback(); err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiting writer did not begin after the active transaction released its lock")
	}
}

func TestOpenEnforcesDeclaredForeignKeyCascades(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','demo','{}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO account_groups(account_id,group_name) VALUES('41','codex')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DELETE FROM accounts WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM account_groups WHERE account_id='41'`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("account group cascade did not run: %d rows remain", remaining)
	}
}

func TestOpenEnforcesOneRoutingStateAndHealthEvaluationPerAccount(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','demo','{}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO routing_decisions(account_id,group_name,updated_at) VALUES('41','primary','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO routing_decisions(account_id,group_name,updated_at) VALUES('41','secondary','now')`); err == nil {
		t.Fatal("同一账号可以保存多份最终调度状态")
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO account_health_evaluations(account_id,group_name,evaluated_at) VALUES('41','primary','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO account_health_evaluations(account_id,group_name,evaluated_at) VALUES('41','secondary','now')`); err == nil {
		t.Fatal("同一账号可以保存多份健康评估")
	}
}

func TestExistingBusinessDatabaseOverviewAndModeAreCompatible(t *testing.T) {
	path := createBusinessDatabase(t, `{"keys":["config/a"],"mode":"监控模式"}`)
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()

	ready, err := store.Ready(ctx)
	if err != nil || !ready {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
	summary, err := store.OverviewSummary(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Accounts != 2 || summary.Groups != 1 || summary.Alerts != 1 || summary.Runs != 2 || summary.LastActivity == nil || *summary.LastActivity != "2026-08-26T09:00:00Z" {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	snapshot, err := store.SetMode(ctx, "完全模式")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Mode != "完全模式" {
		t.Fatalf("unexpected mode: %#v", snapshot)
	}
	items, ok := snapshot.Keys.([]any)
	if !ok || len(items) != 1 || items[0] != "config/a" {
		t.Fatalf("mode update changed config keys: %#v", snapshot.Keys)
	}
}

func TestSetModeRejectsMalformedPersistedKeys(t *testing.T) {
	path := createBusinessDatabase(t, `{"keys":"broken","mode":"监控模式"}`)
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err := store.SetMode(context.Background(), "完全模式"); err == nil {
		t.Fatal("malformed keys must block a mode write")
	}
}

func TestSetModeStartsNewVisibleRoutingDecisionEpochOnlyWhenModeChanges(t *testing.T) {
	path := createBusinessDatabase(t, `{"keys":[],"mode":"监控模式"}`)
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()

	if _, err := store.SetMode(ctx, "完全模式"); err != nil {
		t.Fatal(err)
	}
	var first string
	if err := store.db.QueryRowContext(ctx, `SELECT updated_at FROM app_state WHERE key='routing-decision-epoch'`).Scan(&first); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetMode(ctx, "完全模式"); err != nil {
		t.Fatal(err)
	}
	var second string
	if err := store.db.QueryRowContext(ctx, `SELECT updated_at FROM app_state WHERE key='routing-decision-epoch'`).Scan(&second); err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("selecting the active mode invalidated current decisions: first=%s second=%s", first, second)
	}
}

func TestEmptyBusinessDatabaseCanOpenButIsNotReady(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ready, err := store.Ready(context.Background())
	if err != nil || ready {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
}

func TestGoBootstrapInitializesFreshDatabaseWithFullModeAndTypedPolicy(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	ready, err := store.Ready(ctx)
	if err != nil || !ready {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
	snapshot, err := store.RuntimeSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Mode != "完全模式" {
		t.Fatalf("fresh mode = %q", snapshot.Mode)
	}
	policy, err := store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	if policy["strategy"] != "balanced" {
		t.Fatalf("unexpected initial policy: %#v", policy)
	}
	traffic := policy["traffic"].(map[string]any)
	if traffic["enabled"] != true || traffic["max_samples_per_account"] != int64(60) {
		t.Fatalf("fresh traffic policy does not match Guardian defaults: %#v", traffic)
	}
	if _, present := traffic["concurrency"]; present {
		t.Fatalf("fresh traffic policy contains retired concurrency field: %#v", traffic)
	}
	if _, present := policy["health"]; present {
		t.Fatalf("fresh policy contains retired health source: %#v", policy["health"])
	}
	probe := policy["probe"].(map[string]any)
	if probe["model"] != "" {
		t.Fatalf("fresh probe.model=%#v, want empty string", probe["model"])
	}
	if _, present := probe["default_model"]; present {
		t.Fatalf("fresh policy contains retired probe.default_model: %#v", probe)
	}
	autoApply := policy["auto_apply"].(map[string]any)
	if autoApply["schedulable"] != true || autoApply["priority"] != true || autoApply["load_factor"] != true || autoApply["concurrency"] != false {
		t.Fatalf("fresh auto_apply is not Guardian boolean contract: %#v", autoApply)
	}
	scope := policy["scope"].(map[string]any)
	for _, field := range []string{"account_types", "platforms", "paused_account_ids", "excluded_account_ids", "manual_fused_account_ids"} {
		if values, present := scope[field].([]any); !present || len(values) != 0 {
			t.Fatalf("fresh scope.%s is not an empty list: %#v", field, scope[field])
		}
	}
	if models, present := policy["account_test_models"].(map[string]any); !present || len(models) != 0 {
		t.Fatalf("fresh account_test_models is not an empty object: %#v", policy["account_test_models"])
	}
}

func TestGoBootstrapRefusesToCoverPartialBusinessState(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err := store.db.Exec(`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,updated_at)
		VALUES('example.test','https://example.test','newapi','待验证','now')`); err != nil {
		t.Fatal(err)
	}
	if err := store.Bootstrap(context.Background()); err == nil || !strings.Contains(err.Error(), "拒绝覆盖已有数据") {
		t.Fatalf("partial state must fail closed: %v", err)
	}
}

func TestEnableNotificationChannelPreservesRules(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	policy, err := store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	policy["notifications"] = map[string]any{"delivery_enabled": true, "latency_group_ids": []any{"8"}}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.writePolicyDocument(ctx, tx, "control-plane", policy, "now"); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO operational_snapshots(namespace,state_key,value_json,updated_at)
		VALUES('sub2api','sub2api-notify-rules.json','{"enabled":false,"channels":[]}','now')`); err != nil {
		t.Fatal(err)
	}

	if err := store.EnableNotificationChannel(ctx, "qqbot"); err != nil {
		t.Fatal(err)
	}
	policy, err = store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	notifications, ok := policy["notifications"].(map[string]any)
	if !ok || notifications["enabled"] != true || notifications["delivery_enabled"] != true {
		t.Fatalf("unexpected notifications: %#v", policy["notifications"])
	}
	groups, ok := notifications["latency_group_ids"].([]any)
	if !ok || len(groups) != 1 || groups[0] != "8" {
		t.Fatalf("latency filters were not preserved: %#v", notifications)
	}
	channels, ok := notifications["channels"].([]any)
	if !ok || len(channels) != 1 || channels[0].(map[string]any)["type"] != "qqbot" {
		t.Fatalf("channel not enabled: %#v", notifications)
	}
}

func TestSetProbeEnabledPreservesOtherProbePolicyFields(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	policy, err := store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	policy["probe"] = map[string]any{"enabled": false, "interval_seconds": int64(300), "prompt": "ping"}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.writePolicyDocument(ctx, tx, "control-plane", policy, "now"); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	if err := store.SetProbeEnabled(ctx, true); err != nil {
		t.Fatal(err)
	}
	enabled, err := store.ProbeEnabled(context.Background())
	if err != nil || !enabled {
		t.Fatalf("probe enabled=%v err=%v", enabled, err)
	}
	policy, err = store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	probe := policy["probe"].(map[string]any)
	if probe["enabled"] != true || probe["interval_seconds"] != int64(300) || probe["prompt"] != "ping" {
		t.Fatalf("unexpected probe policy: %#v", probe)
	}
}

func createBusinessDatabase(t *testing.T, configJSON string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "business.sqlite3")
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	statements := []string{
		`CREATE TABLE app_state (key TEXT PRIMARY KEY, value_json TEXT NOT NULL, updated_at TEXT NOT NULL)`,
		`CREATE TABLE accounts (id TEXT PRIMARY KEY)`,
		`INSERT INTO accounts(id) VALUES('1'),('2')`,
		`CREATE TABLE local_groups (name TEXT PRIMARY KEY)`,
		`INSERT INTO local_groups(name) VALUES('codex')`,
		`CREATE TABLE alert_incidents (incident_key TEXT PRIMARY KEY, status TEXT NOT NULL)`,
		`INSERT INTO alert_incidents(incident_key,status) VALUES('open','firing'),('closed','recovered')`,
		`CREATE TABLE runtime_events (source_id INTEGER PRIMARY KEY, created_at TEXT NOT NULL)`,
		`INSERT INTO runtime_events(source_id,created_at) VALUES(1,'2026-08-26T08:00:00+00:00'),(2,'2026-08-26T09:00:00+00:00')`,
	}
	for _, statement := range statements {
		if _, err := database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if !json.Valid([]byte(configJSON)) {
		t.Fatal("invalid test config JSON")
	}
	if _, err := database.Exec(`INSERT INTO app_state(key,value_json,updated_at) VALUES('config',?,'now')`, configJSON); err != nil {
		t.Fatal(err)
	}
	return path
}
