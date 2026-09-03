package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestOpenCreatesAndRepairsPerformanceIndexes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "business.sqlite3")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{
		"ix_account_groups_group_account",
		"ix_bindings_upstream_lookup",
		"ix_health_samples_account_recent",
		"ix_health_samples_normalized_source_latest",
		"ix_health_samples_probe_recent",
		"ix_health_samples_recent",
		"ix_operation_audit_apply_error_recent",
		"ix_operation_audit_log_recent_v2",
		"ix_operation_audit_recent",
		"ix_operation_audit_routing_lookup",
		"ix_operation_audit_type_object_recent",
		"ix_runtime_events_recent",
		"ix_runtime_events_log_order",
		"ix_usage_records_request_recent",
		"ix_usage_records_source_account_recent",
	}
	for _, name := range expected {
		var count int
		err := store.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, name).Scan(&count)
		if err != nil || count != 1 {
			t.Fatalf("business index %s missing: count=%d err=%v", name, count, err)
		}
	}
	if _, err := store.db.Exec(`DROP INDEX ix_operation_audit_recent`); err != nil {
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
	var repaired int
	if err := reopened.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='ix_operation_audit_recent'`).Scan(&repaired); err != nil || repaired != 1 {
		t.Fatalf("reopening an existing database did not repair indexes: count=%d err=%v", repaired, err)
	}
}

func TestOpenEnforcesSingleAccountCostAcrossMemberships(t *testing.T) {
	path := filepath.Join(t.TempDir(), "business.sqlite3")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,multiplier,metadata_json,updated_at)
		VALUES('41','account-0.15','0.15','{}','now');
		INSERT INTO account_groups(account_id,group_name,group_id,group_rate)
		VALUES('41','codex','7','1.5');
		INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,local_rate,metadata_json,updated_at)
		VALUES('41','api.example','91','key','codex','1.5','{}','now')`); err != nil {
		t.Fatal(err)
	}
	var groupRate, localRate string
	if err := store.db.QueryRowContext(ctx, `SELECT ag.group_rate,b.local_rate FROM account_groups ag
		JOIN bindings b ON b.local_account_id=ag.account_id WHERE ag.account_id='41'`).Scan(&groupRate, &localRate); err != nil {
		t.Fatal(err)
	}
	if groupRate != "0.15" || localRate != "0.15" {
		t.Fatalf("inserted costs: membership=%q binding=%q", groupRate, localRate)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE accounts SET multiplier='0.2' WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT ag.group_rate,b.local_rate FROM account_groups ag
		JOIN bindings b ON b.local_account_id=ag.account_id WHERE ag.account_id='41'`).Scan(&groupRate, &localRate); err != nil {
		t.Fatal(err)
	}
	if groupRate != "0.2" || localRate != "0.2" {
		t.Fatalf("updated costs: membership=%q binding=%q", groupRate, localRate)
	}
	if _, err := store.db.ExecContext(ctx, `DROP TRIGGER trg_account_groups_use_account_cost_after_update;
		DROP TRIGGER trg_bindings_use_account_cost_after_update;
		UPDATE account_groups SET group_rate='9.9' WHERE account_id='41';
		UPDATE bindings SET local_rate='8.8' WHERE local_account_id='41'`); err != nil {
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
	if err := reopened.db.QueryRowContext(ctx, `SELECT ag.group_rate,b.local_rate FROM account_groups ag
		JOIN bindings b ON b.local_account_id=ag.account_id WHERE ag.account_id='41'`).Scan(&groupRate, &localRate); err != nil {
		t.Fatal(err)
	}
	if groupRate != "0.2" || localRate != "0.2" {
		t.Fatalf("reopened costs: membership=%q binding=%q", groupRate, localRate)
	}
}

func TestOpenAddsManualPriorityBalanceSyncColumnToExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "business.sqlite3")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(
		id,name,priority,load_factor,concurrency,metadata_json,updated_at
	) VALUES('41','alpha',20,'5',8,'{}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO manual_priority_accounts(
		account_id,priority,created_at,updated_at
	) VALUES('41',3,'now','now')`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `ALTER TABLE manual_priority_accounts DROP COLUMN sync_balance_multiplier`); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	var syncBalanceMultiplier bool
	if err := reopened.db.QueryRowContext(ctx, `SELECT sync_balance_multiplier
		FROM manual_priority_accounts WHERE account_id='41'`).Scan(&syncBalanceMultiplier); err != nil {
		t.Fatal(err)
	}
	if syncBalanceMultiplier {
		t.Fatal("existing manual-priority account unexpectedly enabled balance and multiplier sync")
	}
	if _, err := reopened.HostBalanceSyncAllowed(ctx, "api.example"); err != nil {
		t.Fatalf("host balance policy still fails after reopening: %v", err)
	}
	assignment, err := reopened.AssignManualPriority(ctx, "41", 3, "100", 100, true, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if !assignment.SyncBalanceMultiplier {
		t.Fatalf("balance and multiplier sync was not persisted: %#v", assignment)
	}
	if err := reopened.db.QueryRowContext(ctx, `SELECT sync_balance_multiplier
		FROM manual_priority_accounts WHERE account_id='41'`).Scan(&syncBalanceMultiplier); err != nil {
		t.Fatal(err)
	}
	if !syncBalanceMultiplier {
		t.Fatal("enabled balance and multiplier sync was not stored")
	}
}

func TestOpenAddsRoutingBaselineColumnsToExistingDatabase(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		schema string
	}{
		{
			name: "previous schema",
			schema: `CREATE TABLE routing_baselines (
				account_id TEXT PRIMARY KEY,schedulable INTEGER,priority INTEGER,load_factor TEXT,
				concurrency INTEGER,status TEXT,captured_at TEXT NOT NULL,
				ownership_version INTEGER NOT NULL DEFAULT 1,managed_schedulable INTEGER,
				managed_priority INTEGER,managed_load_factor TEXT,managed_concurrency INTEGER,managed_status TEXT
			)`,
		},
		{
			name: "partially migrated schema",
			schema: `CREATE TABLE routing_baselines (
				account_id TEXT PRIMARY KEY,schedulable INTEGER,priority INTEGER,load_factor TEXT,
				concurrency INTEGER,status TEXT,captured_at TEXT NOT NULL,
				managed_priority INTEGER
			)`,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "business.sqlite3")
			database, err := sql.Open("sqlite", "file:"+path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := database.ExecContext(context.Background(), testCase.schema+`;
				INSERT INTO routing_baselines(account_id,schedulable,priority,load_factor,concurrency,status,captured_at)
				VALUES('829',1,10,'1',2,'active','old')`); err != nil {
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
			columns := map[string]bool{}
			rows, err := store.db.QueryContext(context.Background(), `PRAGMA table_info(routing_baselines)`)
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var id, notNull, primaryKey int
				var name, dataType string
				var defaultValue sql.NullString
				if err := rows.Scan(&id, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
					t.Fatal(err)
				}
				columns[name] = true
			}
			if err := rows.Close(); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{
				"target_fingerprint", "ownership_version", "managed_schedulable", "managed_priority",
				"managed_load_factor", "managed_concurrency", "managed_status",
			} {
				if !columns[name] {
					t.Fatalf("routing baseline column %s was not added", name)
				}
			}

			fingerprint := strings.Repeat("ab", 32)
			priority := int64(20)
			if err := store.CaptureRoutingBaseline(context.Background(), RoutingBaseline{
				AccountID: "829", TargetFingerprint: fingerprint, Priority: &priority,
			}); err != nil {
				t.Fatalf("routing baseline write still fails after migration: %v", err)
			}
			var storedFingerprint string
			if err := store.db.QueryRowContext(context.Background(), `SELECT target_fingerprint
				FROM routing_baselines WHERE account_id='829'`).Scan(&storedFingerprint); err != nil {
				t.Fatal(err)
			}
			if storedFingerprint != fingerprint {
				t.Fatalf("target fingerprint=%q, want %q", storedFingerprint, fingerprint)
			}
			if err := store.ensureSchema(context.Background()); err != nil {
				t.Fatalf("routing baseline migration is not idempotent: %v", err)
			}
		})
	}
}

func TestOpenRecoversRoutingBaselineMissingColumnAlerts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "business.sqlite3")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const incidentKey = "console:routing:apply:829"
	if _, err := store.db.ExecContext(ctx, `INSERT INTO alert_incidents(
		incident_key,event_type,object_kind,object_id,cause_code,status,first_seen_at,last_seen_at
	) VALUES(?, 'routing.apply_failure','account','829',
		'APPLY_FAILED:SQL logic error: table routing_baselines has no column named target_fingerprint (1)',
		'firing','old','old');
		INSERT INTO alert_deliveries(incident_key,channel_key,status,updated_at)
		VALUES(?,'qq','delivered','old')`, incidentKey, incidentKey); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `ALTER TABLE routing_baselines DROP COLUMN target_fingerprint`); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	var incidentStatus, deliveryStatus, incidentUpdatedAt, deliveryUpdatedAt string
	if err := reopened.db.QueryRowContext(ctx, `SELECT i.status,d.status,i.last_seen_at,d.updated_at
		FROM alert_incidents i JOIN alert_deliveries d ON d.incident_key=i.incident_key
		WHERE i.incident_key=?`, incidentKey).Scan(
		&incidentStatus, &deliveryStatus, &incidentUpdatedAt, &deliveryUpdatedAt,
	); err != nil {
		t.Fatal(err)
	}
	if incidentStatus != "recovered" || deliveryStatus != "transition" {
		t.Fatalf("incident=%q delivery=%q", incidentStatus, deliveryStatus)
	}
	if incidentUpdatedAt == "old" || deliveryUpdatedAt == "old" {
		t.Fatalf("recovery timestamps were not updated: incident=%q delivery=%q", incidentUpdatedAt, deliveryUpdatedAt)
	}
}

func TestOpenAddsStableIdentityColumnsBeforeCreatingOnboardingIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-onboarding.sqlite3")
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`CREATE TABLE onboarding_pending (
		operation_id TEXT PRIMARY KEY,upstream_host TEXT NOT NULL,upstream_type TEXT NOT NULL,
		upstream_key_id TEXT NOT NULL,upstream_key_name TEXT,upstream_group_id TEXT NOT NULL,
		upstream_group_name TEXT NOT NULL,local_group_id TEXT NOT NULL,local_group_name TEXT NOT NULL,
		multiplier TEXT NOT NULL,reason TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL
	);
	INSERT INTO onboarding_pending(operation_id,upstream_host,upstream_type,upstream_key_id,
		upstream_group_id,upstream_group_name,local_group_id,local_group_name,multiplier,reason,created_at,updated_at)
	VALUES('legacy','api.example','sub2api','91','7','codex','11','codex','0.5','retry','now','now')`); err != nil {
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
	var upstreamID, upstreamAccountID, localGroupIDs, intentHash string
	var keyUnknown, accountUnknown bool
	if err := store.db.QueryRow(`SELECT upstream_id,upstream_account_id,local_group_ids_json,
		intent_hash,key_commit_unknown,account_commit_unknown FROM onboarding_pending
		WHERE operation_id='legacy'`).Scan(
		&upstreamID, &upstreamAccountID, &localGroupIDs, &intentHash, &keyUnknown, &accountUnknown,
	); err != nil {
		t.Fatal(err)
	}
	if upstreamID != "" || upstreamAccountID != "" || localGroupIDs != "" || intentHash != "" || keyUnknown || accountUnknown {
		t.Fatalf("legacy defaults changed: upstream=%q account=%q groups=%q intent=%q key=%v account_unknown=%v",
			upstreamID, upstreamAccountID, localGroupIDs, intentHash, keyUnknown, accountUnknown)
	}
}

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

func TestOpenMigratesRemovedSchedulingModeToMonitoring(t *testing.T) {
	path := createBusinessDatabase(t, `{"keys":["config/a"],"mode":"调度模式","custom":{"enabled":true,"large_id":9007199254740993}}`)
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()

	snapshot, err := store.RuntimeSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Mode != "监控模式" {
		t.Fatalf("removed mode was not migrated safely: %#v", snapshot)
	}
	var raw string
	if err := store.db.QueryRowContext(ctx, `SELECT value_json FROM app_state WHERE key='config'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		t.Fatal(err)
	}
	custom, ok := config["custom"].(map[string]any)
	if !ok || custom["enabled"] != true {
		t.Fatalf("mode migration discarded unrelated config: %#v", config)
	}
	if !strings.Contains(raw, `"large_id":9007199254740993`) {
		t.Fatalf("mode migration changed an unrelated numeric value: %s", raw)
	}
	var epoch string
	if err := store.db.QueryRowContext(ctx, `SELECT updated_at FROM app_state WHERE key='routing-decision-epoch'`).Scan(&epoch); err != nil || epoch == "" {
		t.Fatalf("mode migration did not invalidate old routing decisions: epoch=%q err=%v", epoch, err)
	}
	if _, err := store.SetMode(ctx, "调度模式"); err == nil {
		t.Fatal("removed scheduling mode was still accepted")
	}
}

func TestOpenLeavesSupportedRuntimeModesUnchanged(t *testing.T) {
	for _, mode := range []string{"监控模式", "完全模式"} {
		t.Run(mode, func(t *testing.T) {
			path := createBusinessDatabase(t, `{"keys":[],"mode":"`+mode+`","custom":"keep"}`)
			store, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			var raw string
			var updatedAt string
			if err := store.db.QueryRow(`SELECT value_json,updated_at FROM app_state WHERE key='config'`).Scan(&raw, &updatedAt); err != nil {
				t.Fatal(err)
			}
			if updatedAt != "now" || raw != `{"keys":[],"mode":"`+mode+`","custom":"keep"}` {
				t.Fatalf("supported mode was unexpectedly rewritten: value=%s updated_at=%s", raw, updatedAt)
			}
		})
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
	if probe["retry_enabled"] != false || probe["retry_source"] != "fixed" || probe["retry_count"] != int64(0) {
		t.Fatalf("fresh probe retry policy is not disabled by default: %#v", probe)
	}
	if !reflect.DeepEqual(probe["retry_status_codes"], []any{int64(429), int64(500), int64(502), int64(503), int64(504)}) {
		t.Fatalf("fresh probe retry status codes are invalid: %#v", probe["retry_status_codes"])
	}
	pricing := policy["price_management"].(map[string]any)
	if pricing["enabled"] != false || pricing["profit_margin"] != 0.2 || pricing["interval_seconds"] != int64(120) || pricing["write_concurrency"] != int64(4) {
		t.Fatalf("fresh price management must be disabled with stable defaults: %#v", pricing)
	}
	if groups, ok := pricing["exchange_group_sets"].([]any); !ok || len(groups) != 0 {
		t.Fatalf("fresh pricing exchange groups must be empty: %#v", pricing["exchange_group_sets"])
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
