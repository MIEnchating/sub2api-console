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

func TestOpenBackfillsDefaultRechargeRateForLegacyUpstream(t *testing.T) {
	path := filepath.Join(t.TempDir(), "business.sqlite3")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Bootstrap(context.Background()); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,updated_at)
		VALUES('legacy.example','https://legacy.example','sub2api','未确认','now')`); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	var rate, note string
	if err := reopened.db.QueryRow(`SELECT recharge_rate,note FROM recharge_rates WHERE host='legacy.example'`).Scan(&rate, &note); err != nil {
		t.Fatal(err)
	}
	if rate != "1" || note != "console-migration-default" {
		t.Fatalf("rate=%q note=%q", rate, note)
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

func TestOpenStartsDecisionEpochWhenLegacyDecisionsHaveNoEpoch(t *testing.T) {
	path := createBusinessDatabase(t, `{"keys":[],"mode":"监控模式"}`)
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`DELETE FROM app_state WHERE key='routing-decision-epoch'`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE routing_decisions (
		account_id TEXT NOT NULL,group_name TEXT NOT NULL,priority INTEGER,schedulable INTEGER,role TEXT,routing_state TEXT,
		rank INTEGER,reason TEXT,updated_at TEXT NOT NULL,payload_json TEXT NOT NULL DEFAULT '{}',PRIMARY KEY(account_id,group_name)
	)`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO routing_decisions(account_id,group_name,updated_at,payload_json)
		VALUES('41','codex','2026-08-23T16:32:36Z','{"weight":0.5}')`); err != nil {
		database.Close()
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	var epoch string
	if err := store.db.QueryRow(`SELECT updated_at FROM app_state WHERE key='routing-decision-epoch'`).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	if epoch <= "2026-08-23T16:32:36Z" {
		t.Fatalf("legacy decision epoch was not advanced: %s", epoch)
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
	if policy["strategy"] != "balanced" || policy["schema_version"] != int64(9) {
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
	var legacyCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM policies`).Scan(&legacyCount); err != nil || legacyCount != 0 {
		t.Fatalf("fresh policy must use typed nodes: count=%d err=%v", legacyCount, err)
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

func TestOpenMigratesLegacyColumnsAndHealthEvidenceKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.sqlite3")
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE health_samples(id INTEGER PRIMARY KEY AUTOINCREMENT,account_id TEXT NOT NULL,group_name TEXT NOT NULL,result TEXT,
		 latency_p50 TEXT,latency_p95 TEXT,latency_p99 TEXT,sample_count INTEGER,attempts INTEGER,failure_reason TEXT,observed_at TEXT,
		 source TEXT NOT NULL,payload_json TEXT NOT NULL DEFAULT '{}',UNIQUE(account_id, group_name, observed_at))`,
		`INSERT INTO health_samples(account_id,group_name,observed_at,source) VALUES('41','codex','now','traffic')`,
		`CREATE TABLE migration_runs(id INTEGER PRIMARY KEY AUTOINCREMENT,status TEXT NOT NULL,row_count INTEGER NOT NULL DEFAULT 0,error TEXT,started_at TEXT NOT NULL,completed_at TEXT)`,
		`CREATE TABLE upstreams(host TEXT PRIMARY KEY,base_url TEXT NOT NULL,upstream_type TEXT NOT NULL,auth_status TEXT NOT NULL,balance REAL,metadata_json TEXT NOT NULL DEFAULT '{}',updated_at TEXT NOT NULL)`,
		`INSERT INTO upstreams VALUES('api.example','https://api.example','sub2api','已鉴权',12,'{}','now')`,
		`CREATE TABLE local_groups(name TEXT PRIMARY KEY,strategy TEXT NOT NULL DEFAULT 'balanced',account_count INTEGER NOT NULL DEFAULT 0,updated_at TEXT NOT NULL)`,
		`CREATE TABLE account_groups(account_id TEXT NOT NULL,group_name TEXT NOT NULL,PRIMARY KEY(account_id,group_name))`,
		`CREATE TABLE operation_audit(source_id INTEGER PRIMARY KEY,operation_id TEXT NOT NULL,operation_type TEXT NOT NULL,state TEXT NOT NULL,phase TEXT NOT NULL,remote_confirmed INTEGER,created_at TEXT NOT NULL)`,
	}
	for _, statement := range statements {
		if _, err := database.Exec(statement); err != nil {
			database.Close()
			t.Fatalf("legacy fixture failed: %v\n%s", err, statement)
		}
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for table, columns := range map[string][]string{
		"health_samples": {"evidence_key"}, "upstreams": {"raw_balance", "mapped_balance"},
		"local_groups": {"remote_id", "strategy_source", "platform"}, "operation_audit": {"group_names_json", "writeback"},
	} {
		existing, err := tableColumns(context.Background(), store.db, table)
		if err != nil {
			t.Fatal(err)
		}
		for _, column := range columns {
			if _, present := existing[column]; !present {
				t.Fatalf("%s.%s was not migrated", table, column)
			}
		}
	}
	var evidence, rawBalance, mappedBalance string
	if err := store.db.QueryRow(`SELECT evidence_key FROM health_samples WHERE id=1`).Scan(&evidence); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT raw_balance,mapped_balance FROM upstreams WHERE host='api.example'`).Scan(&rawBalance, &mappedBalance); err != nil {
		t.Fatal(err)
	}
	if evidence != "legacy:1" || rawBalance != "12.0" && rawBalance != "12" || mappedBalance != "12.0" && mappedBalance != "12" {
		t.Fatalf("unexpected migrated values: evidence=%q raw=%q mapped=%q", evidence, rawBalance, mappedBalance)
	}
}

func TestOpenNormalizesVersionSevenPolicyToCurrentContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "version-seven-policy.sqlite3")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	policy, err := store.readPolicyDocument(context.Background(), store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	policy["schema_version"] = int64(7)
	policy["health"] = map[string]any{"source": "active_probe"}
	traffic := policy["traffic"].(map[string]any)
	traffic["concurrency"] = int64(17)
	delete(traffic, "enabled")
	probe := policy["probe"].(map[string]any)
	probe["default_model"] = "version-seven-model"
	delete(probe, "model")
	policy["auto_apply"] = map[string]any{
		"schedulable": "apply", "priority": "shadow", "load_factor": "apply", "concurrency": "shadow",
	}
	scope := policy["scope"].(map[string]any)
	for _, field := range []string{"account_types", "platforms", "paused_account_ids", "excluded_account_ids", "manual_fused_account_ids"} {
		delete(scope, field)
	}
	delete(policy, "account_test_models")
	policy["notifications"] = map[string]any{"enabled": true}
	tx, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.writePolicyDocument(context.Background(), tx, "control-plane", policy, "2026-08-27T12:00:00Z"); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`PRAGMA user_version=7`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	policy, err = store.readPolicyDocument(context.Background(), store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	probe = policy["probe"].(map[string]any)
	if probe["model"] != "version-seven-model" {
		t.Fatalf("probe.model=%#v, want version-seven-model; probe=%#v", probe["model"], probe)
	}
	if _, present := probe["default_model"]; present {
		t.Fatalf("version-seven probe.default_model remained after migration: %#v", probe)
	}
	traffic = policy["traffic"].(map[string]any)
	if traffic["enabled"] != false {
		t.Fatalf("active_probe mode was not migrated to traffic.enabled=false: %#v", traffic)
	}
	if _, present := traffic["concurrency"]; present {
		t.Fatalf("retired traffic.concurrency remained after migration: %#v", traffic)
	}
	if _, present := policy["health"]; present {
		t.Fatalf("retired health source remained after migration: %#v", policy["health"])
	}
	autoApply := policy["auto_apply"].(map[string]any)
	if autoApply["schedulable"] != true || autoApply["priority"] != false || autoApply["load_factor"] != true || autoApply["concurrency"] != false {
		t.Fatalf("auto_apply was not normalized to booleans: %#v", autoApply)
	}
	scope = policy["scope"].(map[string]any)
	for _, field := range []string{"account_types", "platforms", "paused_account_ids", "excluded_account_ids", "manual_fused_account_ids"} {
		if values, present := scope[field].([]any); !present || len(values) != 0 {
			t.Fatalf("scope.%s was not initialized as an empty list: %#v", field, scope[field])
		}
	}
	if models, present := policy["account_test_models"].(map[string]any); !present || len(models) != 0 {
		t.Fatalf("account_test_models was not initialized: %#v", policy["account_test_models"])
	}
	if policy["notifications"].(map[string]any)["enabled"] != true {
		t.Fatal("unrelated notification policy was changed")
	}
	if policy["schema_version"] != int64(9) {
		t.Fatalf("schema_version=%#v, want 9", policy["schema_version"])
	}
}

func TestEnableNotificationChannelPreservesRulesAndMigratesPolicyToTypedNodes(t *testing.T) {
	path := createBusinessDatabase(t, `{"keys":[],"mode":"完全模式"}`)
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE policies (key TEXT PRIMARY KEY,value_json TEXT NOT NULL,updated_at TEXT NOT NULL)`,
		`CREATE TABLE policy_nodes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,policy_key TEXT NOT NULL,parent_id INTEGER,
			key_name TEXT,list_index INTEGER,node_type TEXT NOT NULL,scalar_value TEXT,updated_at TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX ux_policy_nodes_root ON policy_nodes(policy_key) WHERE parent_id IS NULL`,
		`CREATE TABLE operational_snapshots (
			namespace TEXT NOT NULL,state_key TEXT NOT NULL,value_json TEXT NOT NULL,updated_at TEXT NOT NULL,
			PRIMARY KEY(namespace,state_key)
		)`,
		`INSERT INTO policies(key,value_json,updated_at) VALUES(
			'control-plane','{"notifications":{"delivery_enabled":true,"latency_group_ids":["8"]}}','now'
		)`,
		`INSERT INTO operational_snapshots(namespace,state_key,value_json,updated_at) VALUES(
			'sub2api','sub2api-notify-rules.json','{"enabled":false,"channels":[]}','now'
		)`,
	}
	for _, statement := range statements {
		if _, err := database.Exec(statement); err != nil {
			database.Close()
			t.Fatal(err)
		}
	}
	database.Close()
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	if err := store.EnableNotificationChannel(context.Background(), "qqbot"); err != nil {
		t.Fatal(err)
	}
	policy, err := store.readPolicyDocument(context.Background(), store.db, "control-plane")
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
	var legacyCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM policies WHERE key='control-plane'`).Scan(&legacyCount); err != nil {
		t.Fatal(err)
	}
	if legacyCount != 0 {
		t.Fatal("legacy policy JSON must be removed after typed-node write")
	}
}

func TestSetProbeEnabledPreservesOtherProbePolicyFields(t *testing.T) {
	path := createBusinessDatabase(t, `{"keys":[],"mode":"完全模式"}`)
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE policies (key TEXT PRIMARY KEY,value_json TEXT NOT NULL,updated_at TEXT NOT NULL)`,
		`CREATE TABLE policy_nodes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,policy_key TEXT NOT NULL,parent_id INTEGER,
			key_name TEXT,list_index INTEGER,node_type TEXT NOT NULL,scalar_value TEXT,updated_at TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX ux_policy_nodes_root ON policy_nodes(policy_key) WHERE parent_id IS NULL`,
		`INSERT INTO policies(key,value_json,updated_at) VALUES(
			'control-plane','{"probe":{"enabled":false,"interval_seconds":300,"prompt":"ping"}}','now'
		)`,
	}
	for _, statement := range statements {
		if _, err := database.Exec(statement); err != nil {
			database.Close()
			t.Fatal(err)
		}
	}
	database.Close()
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })

	if err := store.SetProbeEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	enabled, err := store.ProbeEnabled(context.Background())
	if err != nil || !enabled {
		t.Fatalf("probe enabled=%v err=%v", enabled, err)
	}
	policy, err := store.readPolicyDocument(context.Background(), store.db, "control-plane")
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
		`CREATE TABLE migration_runs (id INTEGER PRIMARY KEY, status TEXT NOT NULL)`,
		`INSERT INTO migration_runs(id,status) VALUES(1,'succeeded')`,
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
