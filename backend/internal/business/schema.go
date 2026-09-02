package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

const businessSchema = `
CREATE TABLE IF NOT EXISTS app_state (key TEXT PRIMARY KEY,value_json TEXT NOT NULL,updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS upstream_identities (
 upstream_id TEXT PRIMARY KEY,created_at TEXT NOT NULL,updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS upstream_identity_hosts (
 host TEXT PRIMARY KEY,upstream_id TEXT NOT NULL,is_primary INTEGER NOT NULL DEFAULT 0,updated_at TEXT NOT NULL,
 FOREIGN KEY(upstream_id) REFERENCES upstream_identities(upstream_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS ix_upstream_identity_hosts_identity ON upstream_identity_hosts(upstream_id,is_primary DESC,host);
CREATE TABLE IF NOT EXISTS upstream_catalog_entities (
 upstream_id TEXT NOT NULL,entity_kind TEXT NOT NULL CHECK(entity_kind IN ('group','key')),entity_id TEXT NOT NULL,
 parent_entity_id TEXT,name TEXT NOT NULL,observed_status TEXT,lifecycle_state TEXT NOT NULL
 CHECK(lifecycle_state IN ('active','suspected','missing','retired')),missing_observations INTEGER NOT NULL DEFAULT 0,
 last_seen_at TEXT,missing_since TEXT,confirmed_missing_at TEXT,updated_at TEXT NOT NULL,
 PRIMARY KEY(upstream_id,entity_kind,entity_id),
 FOREIGN KEY(upstream_id) REFERENCES upstream_identities(upstream_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS ix_upstream_catalog_entities_lifecycle ON upstream_catalog_entities(
 upstream_id,entity_kind,lifecycle_state,entity_id
);
CREATE TABLE IF NOT EXISTS upstream_group_change_events (
 id INTEGER PRIMARY KEY AUTOINCREMENT,upstream_id TEXT NOT NULL,group_id TEXT NOT NULL,group_name TEXT NOT NULL,
 change_type TEXT NOT NULL CHECK(change_type IN ('added','removed')),changed_at TEXT NOT NULL,
 FOREIGN KEY(upstream_id) REFERENCES upstream_identities(upstream_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS ix_upstream_group_change_events_recent ON upstream_group_change_events(
 upstream_id,changed_at DESC,id DESC
);
CREATE TABLE IF NOT EXISTS upstreams (
 host TEXT PRIMARY KEY,base_url TEXT NOT NULL,upstream_type TEXT NOT NULL,auth_mode TEXT,enabled INTEGER NOT NULL DEFAULT 1,
 auth_status TEXT NOT NULL,balance REAL,raw_balance TEXT,mapped_balance TEXT,checked_at TEXT,
 account_count INTEGER NOT NULL DEFAULT 0,metadata_json TEXT NOT NULL DEFAULT '{}',updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS upstream_keys (
 host TEXT NOT NULL,key_id TEXT NOT NULL,name TEXT NOT NULL,upstream_group TEXT,rate TEXT,status TEXT,
 metadata_json TEXT NOT NULL DEFAULT '{}',updated_at TEXT NOT NULL,PRIMARY KEY(host,key_id)
);
CREATE TABLE IF NOT EXISTS upstream_groups (
 host TEXT NOT NULL,group_id TEXT NOT NULL,name TEXT NOT NULL,description TEXT,platform TEXT,status TEXT,
 raw_rate TEXT,effective_rate TEXT,rate_source TEXT,updated_at TEXT NOT NULL,PRIMARY KEY(host,group_id)
);
CREATE TABLE IF NOT EXISTS accounts (
 id TEXT PRIMARY KEY,name TEXT NOT NULL,upstream_host TEXT,upstream_type TEXT,schedulable INTEGER,priority INTEGER,
 load_factor TEXT,concurrency INTEGER,multiplier TEXT,balance TEXT,paused INTEGER NOT NULL DEFAULT 0,paused_reason TEXT,
 routing_state TEXT,routing_tier TEXT,health_status TEXT,failure_streak INTEGER,recovery_pass_streak INTEGER,
 target_priority INTEGER,target_load_factor TEXT,target_schedulable INTEGER,target_concurrency INTEGER,
 metadata_json TEXT NOT NULL DEFAULT '{}',updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS account_groups (
 account_id TEXT NOT NULL,group_name TEXT NOT NULL,group_id TEXT,group_rate TEXT,PRIMARY KEY(account_id,group_name),
 FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS ix_account_groups_group_account ON account_groups(group_name,account_id);
CREATE TRIGGER IF NOT EXISTS trg_account_groups_use_account_cost_after_insert
AFTER INSERT ON account_groups
BEGIN
 UPDATE account_groups SET group_rate=(SELECT multiplier FROM accounts WHERE id=NEW.account_id)
 WHERE account_id=NEW.account_id AND group_name=NEW.group_name;
END;
CREATE TRIGGER IF NOT EXISTS trg_account_groups_use_account_cost_after_update
AFTER UPDATE OF group_rate ON account_groups
WHEN NEW.group_rate IS NOT (SELECT multiplier FROM accounts WHERE id=NEW.account_id)
BEGIN
 UPDATE account_groups SET group_rate=(SELECT multiplier FROM accounts WHERE id=NEW.account_id)
 WHERE account_id=NEW.account_id AND group_name=NEW.group_name;
END;
CREATE TRIGGER IF NOT EXISTS trg_accounts_cascade_cost_to_groups
AFTER UPDATE OF multiplier ON accounts
BEGIN
 UPDATE account_groups SET group_rate=NEW.multiplier WHERE account_id=NEW.id;
END;
CREATE TABLE IF NOT EXISTS health_samples (
 id INTEGER PRIMARY KEY AUTOINCREMENT,account_id TEXT NOT NULL,group_name TEXT NOT NULL,result TEXT,
 latency_p50 TEXT,latency_p95 TEXT,latency_p99 TEXT,sample_count INTEGER,attempts INTEGER,failure_reason TEXT,
 observed_at TEXT,source TEXT NOT NULL,evidence_key TEXT,payload_json TEXT NOT NULL DEFAULT '{}',
 UNIQUE(source,evidence_key,account_id,group_name)
);
CREATE INDEX IF NOT EXISTS ix_health_samples_latest ON health_samples(account_id,group_name,observed_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS ix_health_samples_source_latest ON health_samples(source,account_id,group_name,observed_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS ix_health_samples_normalized_source_latest ON health_samples(
 account_id,LOWER(REPLACE(source,'_','-')),observed_at DESC,id DESC
);
CREATE INDEX IF NOT EXISTS ix_health_samples_probe_recent ON health_samples(
 LOWER(REPLACE(source,'_','-')),account_id,group_name,observed_at DESC,id DESC,result,failure_reason
);
CREATE INDEX IF NOT EXISTS ix_health_samples_account_recent ON health_samples(account_id,observed_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS ix_health_samples_recent ON health_samples(COALESCE(observed_at,'') DESC,id DESC);
CREATE TABLE IF NOT EXISTS routing_decisions (
 account_id TEXT NOT NULL,group_name TEXT NOT NULL,priority INTEGER,schedulable INTEGER,role TEXT,routing_state TEXT,
 rank INTEGER,reason TEXT,updated_at TEXT NOT NULL,payload_json TEXT NOT NULL DEFAULT '{}',PRIMARY KEY(account_id)
);
CREATE TABLE IF NOT EXISTS account_health_evaluations (
 account_id TEXT NOT NULL,group_name TEXT NOT NULL,health_score REAL,short_score REAL,long_score REAL,
 sample_count INTEGER NOT NULL DEFAULT 0,ttfb_p50_ms REAL,ttfb_p95_ms REAL,latest_event TEXT,evaluated_at TEXT NOT NULL,
 PRIMARY KEY(account_id),FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS bindings (
 id INTEGER PRIMARY KEY AUTOINCREMENT,local_account_id TEXT NOT NULL,upstream_host TEXT NOT NULL,
 upstream_key_id TEXT NOT NULL,upstream_key_name TEXT NOT NULL,upstream_group TEXT,upstream_group_id TEXT,
 local_group TEXT NOT NULL,local_rate TEXT,upstream_rate TEXT,source_auth_host TEXT,binding_host_alias TEXT,
 description TEXT,status TEXT,metadata_json TEXT NOT NULL DEFAULT '{}',updated_at TEXT NOT NULL,
 UNIQUE(local_account_id,upstream_host,upstream_key_id)
);
CREATE INDEX IF NOT EXISTS ix_bindings_upstream_lookup ON bindings(
 upstream_host,upstream_group_id,upstream_key_id,local_account_id
);
CREATE TRIGGER IF NOT EXISTS trg_bindings_use_account_cost_after_insert
AFTER INSERT ON bindings
BEGIN
 UPDATE bindings SET local_rate=(SELECT multiplier FROM accounts WHERE id=NEW.local_account_id)
 WHERE id=NEW.id AND EXISTS(SELECT 1 FROM accounts WHERE id=NEW.local_account_id);
END;
CREATE TRIGGER IF NOT EXISTS trg_bindings_use_account_cost_after_update
AFTER UPDATE OF local_rate ON bindings
WHEN EXISTS(SELECT 1 FROM accounts WHERE id=NEW.local_account_id)
 AND NEW.local_rate IS NOT (SELECT multiplier FROM accounts WHERE id=NEW.local_account_id)
BEGIN
 UPDATE bindings SET local_rate=(SELECT multiplier FROM accounts WHERE id=NEW.local_account_id) WHERE id=NEW.id;
END;
CREATE TRIGGER IF NOT EXISTS trg_accounts_cascade_cost_to_bindings
AFTER UPDATE OF multiplier ON accounts
BEGIN
 UPDATE bindings SET local_rate=NEW.multiplier WHERE local_account_id=NEW.id;
END;
CREATE TABLE IF NOT EXISTS binding_identities (
 binding_id INTEGER PRIMARY KEY,upstream_id TEXT NOT NULL,upstream_key_id TEXT NOT NULL,upstream_group_id TEXT,updated_at TEXT NOT NULL,
 FOREIGN KEY(binding_id) REFERENCES bindings(id) ON DELETE CASCADE,
 FOREIGN KEY(upstream_id) REFERENCES upstream_identities(upstream_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS ix_binding_identities_upstream_key ON binding_identities(
 upstream_id,upstream_key_id,upstream_group_id,binding_id
);
CREATE TABLE IF NOT EXISTS local_groups (
 name TEXT PRIMARY KEY,remote_id TEXT,strategy TEXT NOT NULL DEFAULT 'balanced',strategy_source TEXT NOT NULL DEFAULT 'global_default',
 platform TEXT,rate_multiplier TEXT,profit_control_enabled INTEGER,profit_min_margin TEXT,profit_safety_buffer TEXT,
 account_count INTEGER NOT NULL DEFAULT 0,updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS newapi_group_bindings (
 platform_id TEXT NOT NULL,newapi_group_id TEXT NOT NULL,newapi_group_name TEXT NOT NULL,
 sub2api_group_id TEXT NOT NULL,sync_ratio INTEGER NOT NULL DEFAULT 0 CHECK(sync_ratio IN (0,1)),
 updated_at TEXT NOT NULL,PRIMARY KEY(platform_id,newapi_group_id)
);
CREATE INDEX IF NOT EXISTS ix_newapi_group_bindings_local ON newapi_group_bindings(sub2api_group_id,platform_id);
CREATE TABLE IF NOT EXISTS recharge_rates (host TEXT PRIMARY KEY,recharge_rate TEXT NOT NULL,note TEXT,updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS billing_quota_unit_observations (
 host TEXT NOT NULL,observed_at TEXT NOT NULL,quota_per_unit TEXT NOT NULL,PRIMARY KEY(host,observed_at)
);
CREATE INDEX IF NOT EXISTS ix_billing_quota_unit_host_time ON billing_quota_unit_observations(host,observed_at DESC);
CREATE TABLE IF NOT EXISTS pricing_backups (
 id TEXT PRIMARY KEY,name TEXT NOT NULL COLLATE NOCASE UNIQUE,actor TEXT NOT NULL,account_count INTEGER NOT NULL,created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS pricing_backup_accounts (
 backup_id TEXT NOT NULL,account_id TEXT NOT NULL,account_name TEXT NOT NULL,group_ids_json TEXT NOT NULL,group_names_json TEXT NOT NULL,
 PRIMARY KEY(backup_id,account_id),FOREIGN KEY(backup_id) REFERENCES pricing_backups(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS ix_pricing_backups_created ON pricing_backups(created_at DESC,id DESC);
CREATE TABLE IF NOT EXISTS policy_nodes (
 id INTEGER PRIMARY KEY AUTOINCREMENT,policy_key TEXT NOT NULL,parent_id INTEGER,key_name TEXT,list_index INTEGER,
 node_type TEXT NOT NULL CHECK(node_type IN ('object','array','string','integer','real','boolean','null')),
 scalar_value TEXT,updated_at TEXT NOT NULL,FOREIGN KEY(parent_id) REFERENCES policy_nodes(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_policy_nodes_root ON policy_nodes(policy_key) WHERE parent_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ux_policy_nodes_object_child ON policy_nodes(policy_key,parent_id,key_name) WHERE key_name IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ux_policy_nodes_array_child ON policy_nodes(policy_key,parent_id,list_index) WHERE list_index IS NOT NULL;
CREATE TABLE IF NOT EXISTS paused_accounts (account_id TEXT PRIMARY KEY,reason TEXT,enabled INTEGER NOT NULL DEFAULT 0,updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS manual_priority_accounts (
 account_id TEXT PRIMARY KEY,priority INTEGER NOT NULL,previous_priority INTEGER,previous_load_factor TEXT,
 previous_concurrency INTEGER,sync_balance_multiplier INTEGER NOT NULL DEFAULT 0 CHECK(sync_balance_multiplier IN (0,1)),
 created_at TEXT NOT NULL,updated_at TEXT NOT NULL,
 FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS routing_baselines (
 account_id TEXT PRIMARY KEY,target_fingerprint TEXT NOT NULL DEFAULT '',schedulable INTEGER,priority INTEGER,load_factor TEXT,concurrency INTEGER,status TEXT,captured_at TEXT NOT NULL,
 ownership_version INTEGER NOT NULL DEFAULT 1,managed_schedulable INTEGER,managed_priority INTEGER,
 managed_load_factor TEXT,managed_concurrency INTEGER,managed_status TEXT
);
CREATE TABLE IF NOT EXISTS cleanup_states (account_id TEXT PRIMARY KEY,eligible_since TEXT NOT NULL,updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS runtime_events (source_id INTEGER PRIMARY KEY,event_type TEXT NOT NULL,created_at TEXT NOT NULL,status TEXT NOT NULL,summary TEXT NOT NULL,payload_json TEXT NOT NULL DEFAULT '{}');
CREATE INDEX IF NOT EXISTS ix_runtime_events_recent ON runtime_events(created_at DESC,source_id);
CREATE INDEX IF NOT EXISTS ix_runtime_events_log_order ON runtime_events(
 created_at DESC,
 CASE WHEN source_id < 0 THEN 0 ELSE 1 END,
 CASE WHEN source_id < 0 THEN source_id END ASC,
 CASE WHEN source_id >= 0 THEN source_id END DESC
);
CREATE TABLE IF NOT EXISTS alert_incidents (
 incident_key TEXT PRIMARY KEY,event_type TEXT NOT NULL,object_kind TEXT NOT NULL,object_id TEXT NOT NULL,cause_code TEXT NOT NULL,
 status TEXT NOT NULL,first_seen_at TEXT NOT NULL,last_seen_at TEXT NOT NULL,delivery_status TEXT,last_error TEXT
);
CREATE INDEX IF NOT EXISTS ix_alert_incidents_status ON alert_incidents(status,incident_key);
CREATE TABLE IF NOT EXISTS alert_deliveries (
 incident_key TEXT NOT NULL,channel_key TEXT NOT NULL,status TEXT NOT NULL,attempts INTEGER NOT NULL DEFAULT 0,
 last_error TEXT,delivered_at TEXT,updated_at TEXT NOT NULL,PRIMARY KEY(incident_key,channel_key),
 FOREIGN KEY(incident_key) REFERENCES alert_incidents(incident_key) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS ix_alert_deliveries_channel_incident ON alert_deliveries(channel_key,incident_key);
CREATE TABLE IF NOT EXISTS scheduler_leases (
 lease_name TEXT PRIMARY KEY,owner_id TEXT NOT NULL,owner_pid INTEGER NOT NULL,owner_host TEXT NOT NULL,
 checked_at TEXT NOT NULL,acquired_at TEXT NOT NULL,renewed_at TEXT NOT NULL,expires_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS operation_audit (
 source_id INTEGER PRIMARY KEY,operation_id TEXT NOT NULL,operation_type TEXT NOT NULL,state TEXT NOT NULL,phase TEXT NOT NULL,
 request_id TEXT,actor TEXT,source TEXT,error TEXT,remote_confirmed INTEGER,readback_confirmed INTEGER,
 object_type TEXT,object_id TEXT,object_name TEXT,group_names_json TEXT NOT NULL DEFAULT '[]',field_name TEXT,
 before_json TEXT,after_json TEXT,writeback INTEGER NOT NULL DEFAULT 1,created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS ix_operation_audit_routing_lookup ON operation_audit(object_id,created_at DESC)
 WHERE operation_type='routing.writeback' AND state='succeeded' AND remote_confirmed=1 AND readback_confirmed=1;
CREATE INDEX IF NOT EXISTS ix_operation_audit_type_object_recent ON operation_audit(
 operation_type,object_id,created_at DESC,source_id
);
CREATE INDEX IF NOT EXISTS ix_operation_audit_apply_error_recent ON operation_audit(
 object_id,created_at DESC,
 CASE WHEN source_id < 0 THEN 0 ELSE 1 END,
 CASE WHEN source_id < 0 THEN source_id END ASC,
 CASE WHEN source_id >= 0 THEN source_id END DESC,
 state,error
) WHERE operation_type IN ('routing.writeback','cleanup.delete') AND object_id IS NOT NULL
 AND (state='failed' OR readback_confirmed=1);
CREATE INDEX IF NOT EXISTS ix_operation_audit_recent ON operation_audit(created_at DESC,source_id);
CREATE INDEX IF NOT EXISTS ix_operation_audit_log_recent ON operation_audit(created_at DESC,source_id)
	 WHERE phase<>'calculation' AND operation_type<>'upstream.rate_sync' AND (
	  writeback=1 OR (operation_type='account.delete' AND state='failed') OR
	  (operation_type IN ('account.scheduling','routing.writeback')
   AND state='succeeded' AND remote_confirmed=0 AND readback_confirmed=1
   AND before_json IS NOT NULL AND after_json IS NOT NULL)
 );
CREATE TABLE IF NOT EXISTS run_records (
 run_key TEXT PRIMARY KEY,task_name TEXT NOT NULL,status TEXT,stage TEXT,started_at TEXT,ended_at TEXT,
 duration_seconds TEXT,summary TEXT,payload_json TEXT NOT NULL DEFAULT '{}',updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS ix_run_records_display_time ON run_records(
 COALESCE(ended_at,started_at,updated_at) DESC,run_key
);
CREATE INDEX IF NOT EXISTS ix_run_records_updated_at ON run_records(updated_at);
CREATE TABLE IF NOT EXISTS usage_records (
 id INTEGER PRIMARY KEY AUTOINCREMENT,request_id TEXT NOT NULL,account_id TEXT,account_name TEXT,group_name TEXT,
 is_error INTEGER,error_reason TEXT,first_token_ms TEXT,observed_at TEXT,source TEXT NOT NULL,payload_json TEXT NOT NULL DEFAULT '{}',
 UNIQUE(request_id,account_id,group_name,observed_at)
);
CREATE INDEX IF NOT EXISTS ix_usage_records_recent ON usage_records(COALESCE(observed_at,'') DESC,id DESC);
CREATE INDEX IF NOT EXISTS ix_usage_records_request_recent ON usage_records(
 request_id,COALESCE(observed_at,'') DESC,id DESC
);
CREATE INDEX IF NOT EXISTS ix_usage_records_account_recent ON usage_records(
 account_id,COALESCE(observed_at,'') DESC,id DESC
);
CREATE INDEX IF NOT EXISTS ix_usage_records_source_account_recent ON usage_records(
 LOWER(REPLACE(source,'_','-')),account_id,COALESCE(observed_at,'') DESC,id DESC
);
CREATE TABLE IF NOT EXISTS operational_snapshots (
 namespace TEXT NOT NULL,state_key TEXT NOT NULL,value_json TEXT NOT NULL,observed_at TEXT,updated_at TEXT NOT NULL,
 origin TEXT NOT NULL DEFAULT 'console',PRIMARY KEY(namespace,state_key)
);
CREATE INDEX IF NOT EXISTS ix_operational_snapshots_state_recent ON operational_snapshots(state_key,updated_at DESC);
CREATE TABLE IF NOT EXISTS onboarding_pending (
	 operation_id TEXT PRIMARY KEY,upstream_id TEXT NOT NULL DEFAULT '',upstream_host TEXT NOT NULL,upstream_type TEXT NOT NULL,upstream_key_id TEXT NOT NULL,
	 upstream_key_name TEXT,upstream_account_id TEXT NOT NULL DEFAULT '',upstream_group_id TEXT NOT NULL,
	 upstream_group_name TEXT NOT NULL,local_group_id TEXT NOT NULL,local_group_name TEXT NOT NULL,
	 local_group_ids_json TEXT NOT NULL DEFAULT '',multiplier TEXT NOT NULL,intent_hash TEXT NOT NULL DEFAULT '',reason TEXT NOT NULL,
	 key_commit_unknown INTEGER NOT NULL DEFAULT 0,account_commit_unknown INTEGER NOT NULL DEFAULT 0,
	 created_at TEXT NOT NULL,updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_onboarding_pending_identity ON onboarding_pending(
 upstream_id,upstream_group_id,local_group_ids_json
) WHERE upstream_id<>'' AND local_group_ids_json<>'';
`

func (s *Store) ensureSchema(ctx context.Context) error {
	if err := s.ensureLegacyOnboardingPendingColumns(ctx); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, businessSchema); err != nil {
		return err
	}
	if err := s.ensureRoutingBaselineColumns(ctx); err != nil {
		return err
	}
	var accountMultiplierColumns int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('accounts') WHERE name='multiplier'`).Scan(&accountMultiplierColumns); err != nil {
		return err
	}
	if accountMultiplierColumns == 1 {
		if _, err := s.db.ExecContext(ctx, `UPDATE account_groups
			SET group_rate=(SELECT a.multiplier FROM accounts a WHERE a.id=account_groups.account_id)
			WHERE group_rate IS NOT (SELECT a.multiplier FROM accounts a WHERE a.id=account_groups.account_id)`); err != nil {
			return fmt.Errorf("归一账号分组成本失败: %w", err)
		}
		var bindingLocalRateColumns int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('bindings') WHERE name='local_rate'`).Scan(&bindingLocalRateColumns); err != nil {
			return err
		}
		if bindingLocalRateColumns == 1 {
			if _, err := s.db.ExecContext(ctx, `UPDATE bindings
				SET local_rate=(SELECT a.multiplier FROM accounts a WHERE a.id=bindings.local_account_id)
				WHERE EXISTS(SELECT 1 FROM accounts a WHERE a.id=bindings.local_account_id)
				 AND local_rate IS NOT (SELECT a.multiplier FROM accounts a WHERE a.id=bindings.local_account_id)`); err != nil {
				return fmt.Errorf("归一账号绑定成本失败: %w", err)
			}
		}
	}
	if err := s.ensureManualPriorityBalanceSyncColumn(ctx); err != nil {
		return err
	}
	if err := s.migrateRemovedRuntimeModes(ctx); err != nil {
		return err
	}
	if err := s.ensureUpstreamIdentities(ctx); err != nil {
		return err
	}
	return s.ensureStableUpstreamRelations(ctx)
}

func (s *Store) ensureRoutingBaselineColumns(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(routing_baselines)`)
	if err != nil {
		return err
	}
	columns := map[string]struct{}{}
	for rows.Next() {
		var id, notNull, primaryKey int
		var name, dataType string
		var defaultValue sql.NullString
		if err := rows.Scan(&id, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return err
		}
		columns[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	definitions := []struct {
		name string
		sql  string
	}{
		{"target_fingerprint", `ALTER TABLE routing_baselines ADD COLUMN target_fingerprint TEXT NOT NULL DEFAULT ''`},
		{"ownership_version", `ALTER TABLE routing_baselines ADD COLUMN ownership_version INTEGER NOT NULL DEFAULT 1`},
		{"managed_schedulable", `ALTER TABLE routing_baselines ADD COLUMN managed_schedulable INTEGER`},
		{"managed_priority", `ALTER TABLE routing_baselines ADD COLUMN managed_priority INTEGER`},
		{"managed_load_factor", `ALTER TABLE routing_baselines ADD COLUMN managed_load_factor TEXT`},
		{"managed_concurrency", `ALTER TABLE routing_baselines ADD COLUMN managed_concurrency INTEGER`},
		{"managed_status", `ALTER TABLE routing_baselines ADD COLUMN managed_status TEXT`},
	}
	for _, definition := range definitions {
		if _, found := columns[definition.name]; found {
			continue
		}
		if _, err := tx.ExecContext(ctx, definition.sql); err != nil {
			return fmt.Errorf("补充调度基线字段 %s 失败: %w", definition.name, err)
		}
	}
	var alertRecoveryColumns int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('alert_incidents')
		WHERE name IN ('event_type','cause_code','last_seen_at','last_error')`).Scan(&alertRecoveryColumns); err != nil {
		return err
	}
	if alertRecoveryColumns != 4 {
		return tx.Commit()
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE alert_deliveries SET status='transition',updated_at=?
		WHERE incident_key IN (
			SELECT incident_key FROM alert_incidents WHERE status='firing'
			AND event_type='routing.apply_failure'
			AND cause_code LIKE 'APPLY_FAILED:%table routing_baselines has no column named target_fingerprint%'
		)`, now); err != nil {
		return fmt.Errorf("更新调度基线迁移恢复通知失败: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE alert_incidents SET status='recovered',last_seen_at=?,last_error=NULL
		WHERE status='firing' AND event_type='routing.apply_failure'
		AND cause_code LIKE 'APPLY_FAILED:%table routing_baselines has no column named target_fingerprint%'`, now); err != nil {
		return fmt.Errorf("恢复调度基线缺列告警失败: %w", err)
	}
	return tx.Commit()
}

func (s *Store) ensureLegacyOnboardingPendingColumns(ctx context.Context) error {
	var tableExists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(
		SELECT 1 FROM sqlite_master WHERE type='table' AND name='onboarding_pending'
	)`).Scan(&tableExists); err != nil {
		return err
	}
	if !tableExists {
		return nil
	}
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(onboarding_pending)`)
	if err != nil {
		return err
	}
	columns := map[string]struct{}{}
	for rows.Next() {
		var id, notNull, primaryKey int
		var name, dataType string
		var defaultValue sql.NullString
		if err := rows.Scan(&id, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return err
		}
		columns[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	definitions := []struct {
		name string
		sql  string
	}{
		{"upstream_id", `ALTER TABLE onboarding_pending ADD COLUMN upstream_id TEXT NOT NULL DEFAULT ''`},
		{"upstream_account_id", `ALTER TABLE onboarding_pending ADD COLUMN upstream_account_id TEXT NOT NULL DEFAULT ''`},
		{"local_group_ids_json", `ALTER TABLE onboarding_pending ADD COLUMN local_group_ids_json TEXT NOT NULL DEFAULT ''`},
		{"intent_hash", `ALTER TABLE onboarding_pending ADD COLUMN intent_hash TEXT NOT NULL DEFAULT ''`},
		{"key_commit_unknown", `ALTER TABLE onboarding_pending ADD COLUMN key_commit_unknown INTEGER NOT NULL DEFAULT 0`},
		{"account_commit_unknown", `ALTER TABLE onboarding_pending ADD COLUMN account_commit_unknown INTEGER NOT NULL DEFAULT 0`},
	}
	for _, definition := range definitions {
		if _, found := columns[definition.name]; found {
			continue
		}
		if _, err := s.db.ExecContext(ctx, definition.sql); err != nil {
			return fmt.Errorf("补充待处理账号字段 %s 失败: %w", definition.name, err)
		}
	}
	return nil
}

func (s *Store) ensureManualPriorityBalanceSyncColumn(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `PRAGMA table_info(manual_priority_accounts)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var id, notNull, primaryKey int
		var name, dataType string
		var defaultValue sql.NullString
		if err := rows.Scan(&id, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return err
		}
		if name == "sync_balance_multiplier" {
			found = true
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if !found {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE manual_priority_accounts
			ADD COLUMN sync_balance_multiplier INTEGER NOT NULL DEFAULT 0
			CHECK(sync_balance_multiplier IN (0,1))`); err != nil {
			return fmt.Errorf("补充人工优先位余额与倍率同步字段失败: %w", err)
		}
	}
	return tx.Commit()
}

func (s *Store) migrateRemovedRuntimeModes(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var raw string
	err = tx.QueryRowContext(ctx, `SELECT value_json FROM app_state WHERE key='config'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return tx.Commit()
	}
	if err != nil {
		return err
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &value); err != nil || value == nil {
		return tx.Commit()
	}
	var mode string
	if err := json.Unmarshal(value["mode"], &mode); err != nil || mode != "调度模式" {
		return tx.Commit()
	}
	value["mode"] = json.RawMessage(`"` + runtimepolicy.Monitoring + `"`)
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `UPDATE app_state SET value_json=?,updated_at=? WHERE key='config'`, string(encoded), now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO app_state(key,value_json,updated_at)
		VALUES('routing-decision-epoch','{}',?) ON CONFLICT(key) DO UPDATE SET updated_at=excluded.updated_at`, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) bootstrapFresh(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ready, err := readyOnConnection(ctx, tx)
	if err != nil {
		return err
	}
	if ready {
		return tx.Commit()
	}
	populated, err := populatedBusinessTables(ctx, tx)
	if err != nil {
		return err
	}
	if len(populated) > 0 {
		return fmt.Errorf("控制台业务库初始化状态不完整，拒绝覆盖已有数据：%s", strings.Join(populated, ","))
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO app_state(key,value_json,updated_at) VALUES('config',?,?)`,
		`{"mode":"`+runtimepolicy.Full+`","keys":[]}`, now); err != nil {
		return err
	}
	if err := s.writePolicyDocument(ctx, tx, "control-plane", initialControlPolicy(), now); err != nil {
		return err
	}
	return tx.Commit()
}

type connectionQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func readyOnConnection(ctx context.Context, queryer connectionQueryer) (bool, error) {
	var marker int
	err := queryer.QueryRowContext(ctx, `SELECT 1 FROM app_state WHERE key='config'`).Scan(&marker)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func populatedBusinessTables(ctx context.Context, queryer connectionQueryer) ([]string, error) {
	tables := []string{
		"upstream_identities", "upstream_identity_hosts", "upstream_catalog_entities", "upstreams", "upstream_keys", "upstream_groups", "accounts", "account_groups", "health_samples",
		"routing_decisions", "account_health_evaluations", "bindings", "local_groups", "recharge_rates", "billing_quota_unit_observations", "pricing_backups", "pricing_backup_accounts",
		"policy_nodes", "paused_accounts", "manual_priority_accounts", "routing_baselines", "cleanup_states", "runtime_events",
		"alert_incidents", "alert_deliveries", "operation_audit", "run_records", "usage_records",
		"operational_snapshots", "onboarding_pending",
	}
	result := make([]string, 0)
	for _, table := range tables {
		var count int
		if err := queryer.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			return nil, err
		}
		if count > 0 {
			result = append(result, table)
		}
	}
	var stateCount int
	if err := queryer.QueryRowContext(ctx, `SELECT COUNT(*) FROM app_state`).Scan(&stateCount); err != nil {
		return nil, err
	}
	if stateCount > 0 {
		result = append(result, "app_state")
	}
	return result, nil
}

func initialControlPolicy() map[string]any {
	return map[string]any{
		"strategy": "balanced", "selection": map[string]any{"strategy": "balanced"},
		"weights": map[string]any{
			"scheduling_missing_rate_fallback": "current_cost_wall", "enabled": true, "budget": int64(400),
			"gate_floor": int64(40), "price_exp": 1.0, "speed_exp": 1.0, "balanced_price_ratio": 0.5,
			"change_threshold": "0.1", "cooldown_seconds": int64(60), "min_load_factor": int64(1), "max_load_factor": int64(100),
		},
		"manual_priority":     map[string]any{"reserved_max": int64(10)},
		"probe":               map[string]any{"enabled": true, "interval_seconds": int64(300), "timeout_seconds": int64(60), "concurrency": int64(4), "model": "", "prompt": "hi", "skip_when_traffic_fresh": true, "traffic_fresh_seconds": int64(180), "retry_enabled": false, "retry_source": "fixed", "retry_count": int64(0), "retry_status_codes": []any{int64(429), int64(500), int64(502), int64(503), int64(504)}},
		"traffic":             map[string]any{"enabled": true, "refresh_seconds": int64(60), "lookback_minutes": int64(120), "max_samples_per_account": int64(60)},
		"upstream_multiplier": map[string]any{"interval_seconds": int64(120)},
		"price_management":    map[string]any{"enabled": false, "profit_margin": 0.2, "exchange_group_sets": []any{}, "exchange_group_set_names": []any{}, "interval_seconds": int64(120), "write_concurrency": int64(4)},
		"writeback":           map[string]any{"concurrency": int64(4), "verification": false},
		"scoring": map[string]any{
			"event_scores": map[string]any{"perfect": int64(100), "slow_ttfb": int64(65), "upstream_unknown": int64(40), "gateway_error": int64(25), "quota_exhausted": int64(15), "probe_fail": int64(10), "fatal": int64(0)},
			"short_window": int64(10), "long_window": int64(60), "latest_weight": 0.5, "short_ratio": 0.7, "slow_ttfb_ms": int64(5000),
		},
		"breaker":  map[string]any{"enabled": true, "hard_fatal": true, "http_window": int64(5), "http_failures": int64(3), "http_score_below": int64(60), "latency_window": int64(10), "latency_occurrences": int64(5), "latency_ttfb_ms": int64(15000), "max_switch_per_round": int64(1), "min_pool_size": int64(1), "min_pool_score": int64(3), "fused_cooldown_seconds": int64(180), "instant_status_codes": []any{}, "http_degrade_only": true, "latency_degrade_only": true},
		"degrade":  map[string]any{"enabled": true, "score_threshold": int64(75), "priority_step": int64(10), "load_factor_ratio": 0.5, "min_load_factor": int64(1)},
		"recovery": map[string]any{"enabled": true, "probe_interval_seconds": int64(180), "target_score": int64(75), "success_count": int64(2), "hold_seconds": int64(60)},
		"scaling":  map[string]any{"enabled": false, "global_max_concurrency": int64(900), "min_per_account": int64(3), "max_per_account": int64(250), "scale_up_ratio": 0.8, "step_up": int64(5), "step_down": int64(5), "cooldown_seconds": int64(60)},
		"cleanup":  map[string]any{"enabled": false, "action": "pause", "occurrences": int64(3), "window": int64(5), "min_fused_minutes": int64(30), "max_per_round": int64(1), "keep_last_in_group": true, "only_auth_errors": true, "trigger_status_codes": []any{int64(401), int64(403)}},
		"classify": map[string]any{"fatal_patterns": []any{
			"invalid api key", "unauthorized", "forbidden", "authentication", "account not found", "no api key", "no access token",
			"insufficient", "balance", "quota exceeded", "usage limit", "credit", "expired",
		}, "gateway_status_codes": []any{int64(429), int64(500), int64(502), int64(503), int64(504)}},
		"scope": map[string]any{
			"manage_all_accounts": true, "managed_group_mode": "all", "managed_group_ids": []any{}, "excluded_group_ids": []any{},
			"account_types": []any{}, "platforms": []any{}, "paused_account_ids": []any{},
			"excluded_account_ids": []any{}, "manual_fused_account_ids": []any{},
		},
		"group_policy_bindings": map[string]any{},
		"account_test_models":   map[string]any{},
		"auto_apply":            map[string]any{"schedulable": true, "priority": true, "load_factor": true, "concurrency": false},
	}
}
