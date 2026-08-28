package business

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

const businessSchema = `
CREATE TABLE IF NOT EXISTS migration_runs (
 id INTEGER PRIMARY KEY AUTOINCREMENT,status TEXT NOT NULL,row_count INTEGER NOT NULL DEFAULT 0,error TEXT,
 started_at TEXT NOT NULL,completed_at TEXT,private_auth_imported INTEGER NOT NULL DEFAULT 0,
 private_auth_status TEXT NOT NULL DEFAULT '未执行',private_auth_error TEXT
);
CREATE TABLE IF NOT EXISTS app_state (key TEXT PRIMARY KEY,value_json TEXT NOT NULL,updated_at TEXT NOT NULL);
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
CREATE TABLE IF NOT EXISTS health_samples (
 id INTEGER PRIMARY KEY AUTOINCREMENT,account_id TEXT NOT NULL,group_name TEXT NOT NULL,result TEXT,
 latency_p50 TEXT,latency_p95 TEXT,latency_p99 TEXT,sample_count INTEGER,attempts INTEGER,failure_reason TEXT,
 observed_at TEXT,source TEXT NOT NULL,evidence_key TEXT,payload_json TEXT NOT NULL DEFAULT '{}',
 UNIQUE(source,evidence_key,account_id,group_name)
);
CREATE INDEX IF NOT EXISTS ix_health_samples_latest ON health_samples(account_id,group_name,observed_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS ix_health_samples_source_latest ON health_samples(source,account_id,group_name,observed_at DESC,id DESC);
CREATE TABLE IF NOT EXISTS routing_decisions (
 account_id TEXT NOT NULL,group_name TEXT NOT NULL,priority INTEGER,schedulable INTEGER,role TEXT,routing_state TEXT,
 rank INTEGER,reason TEXT,updated_at TEXT NOT NULL,payload_json TEXT NOT NULL DEFAULT '{}',PRIMARY KEY(account_id,group_name)
);
CREATE TABLE IF NOT EXISTS account_health_evaluations (
 account_id TEXT NOT NULL,group_name TEXT NOT NULL,health_score REAL,short_score REAL,long_score REAL,
 sample_count INTEGER NOT NULL DEFAULT 0,ttfb_p50_ms REAL,ttfb_p95_ms REAL,latest_event TEXT,evaluated_at TEXT NOT NULL,
 PRIMARY KEY(account_id,group_name),FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE
);
CREATE TABLE IF NOT EXISTS bindings (
 id INTEGER PRIMARY KEY AUTOINCREMENT,local_account_id TEXT NOT NULL,upstream_host TEXT NOT NULL,
 upstream_key_id TEXT NOT NULL,upstream_key_name TEXT NOT NULL,upstream_group TEXT,upstream_group_id TEXT,
 local_group TEXT NOT NULL,local_rate TEXT,upstream_rate TEXT,source_auth_host TEXT,binding_host_alias TEXT,
 description TEXT,status TEXT,metadata_json TEXT NOT NULL DEFAULT '{}',updated_at TEXT NOT NULL,
 UNIQUE(local_account_id,upstream_host,upstream_key_id)
);
CREATE TABLE IF NOT EXISTS local_groups (
 name TEXT PRIMARY KEY,remote_id TEXT,strategy TEXT NOT NULL DEFAULT 'balanced',strategy_source TEXT NOT NULL DEFAULT 'global_default',
 platform TEXT,rate_multiplier TEXT,profit_control_enabled INTEGER,profit_min_margin TEXT,profit_safety_buffer TEXT,
 account_count INTEGER NOT NULL DEFAULT 0,updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS recharge_rates (host TEXT PRIMARY KEY,recharge_rate TEXT NOT NULL,note TEXT,updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS policies (key TEXT PRIMARY KEY,value_json TEXT NOT NULL,updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS policy_nodes (
 id INTEGER PRIMARY KEY AUTOINCREMENT,policy_key TEXT NOT NULL,parent_id INTEGER,key_name TEXT,list_index INTEGER,
 node_type TEXT NOT NULL CHECK(node_type IN ('object','array','string','integer','real','boolean','null')),
 scalar_value TEXT,updated_at TEXT NOT NULL,FOREIGN KEY(parent_id) REFERENCES policy_nodes(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_policy_nodes_root ON policy_nodes(policy_key) WHERE parent_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ux_policy_nodes_object_child ON policy_nodes(policy_key,parent_id,key_name) WHERE key_name IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ux_policy_nodes_array_child ON policy_nodes(policy_key,parent_id,list_index) WHERE list_index IS NOT NULL;
CREATE TABLE IF NOT EXISTS paused_accounts (account_id TEXT PRIMARY KEY,reason TEXT,enabled INTEGER NOT NULL DEFAULT 0,updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS routing_baselines (
 account_id TEXT PRIMARY KEY,schedulable INTEGER,priority INTEGER,load_factor TEXT,concurrency INTEGER,status TEXT,captured_at TEXT NOT NULL,
 ownership_version INTEGER NOT NULL DEFAULT 1,managed_schedulable INTEGER,managed_priority INTEGER,
 managed_load_factor TEXT,managed_concurrency INTEGER,managed_status TEXT
);
CREATE TABLE IF NOT EXISTS cleanup_states (account_id TEXT PRIMARY KEY,eligible_since TEXT NOT NULL,updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS runtime_events (source_id INTEGER PRIMARY KEY,event_type TEXT NOT NULL,created_at TEXT NOT NULL,status TEXT NOT NULL,summary TEXT NOT NULL,payload_json TEXT NOT NULL DEFAULT '{}');
CREATE TABLE IF NOT EXISTS alert_incidents (
 incident_key TEXT PRIMARY KEY,event_type TEXT NOT NULL,object_kind TEXT NOT NULL,object_id TEXT NOT NULL,cause_code TEXT NOT NULL,
 status TEXT NOT NULL,first_seen_at TEXT NOT NULL,last_seen_at TEXT NOT NULL,delivery_status TEXT,last_error TEXT
);
CREATE TABLE IF NOT EXISTS alert_deliveries (
 incident_key TEXT NOT NULL,channel_key TEXT NOT NULL,status TEXT NOT NULL,attempts INTEGER NOT NULL DEFAULT 0,
 last_error TEXT,delivered_at TEXT,updated_at TEXT NOT NULL,PRIMARY KEY(incident_key,channel_key),
 FOREIGN KEY(incident_key) REFERENCES alert_incidents(incident_key) ON DELETE CASCADE
);
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
CREATE TABLE IF NOT EXISTS run_records (
 run_key TEXT PRIMARY KEY,task_name TEXT NOT NULL,status TEXT,stage TEXT,started_at TEXT,ended_at TEXT,
 duration_seconds TEXT,summary TEXT,payload_json TEXT NOT NULL DEFAULT '{}',updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS usage_records (
 id INTEGER PRIMARY KEY AUTOINCREMENT,request_id TEXT NOT NULL,account_id TEXT,account_name TEXT,group_name TEXT,
 is_error INTEGER,error_reason TEXT,first_token_ms TEXT,observed_at TEXT,source TEXT NOT NULL,payload_json TEXT NOT NULL DEFAULT '{}',
 UNIQUE(request_id,account_id,group_name,observed_at)
);
CREATE TABLE IF NOT EXISTS operational_snapshots (
 namespace TEXT NOT NULL,state_key TEXT NOT NULL,value_json TEXT NOT NULL,observed_at TEXT,updated_at TEXT NOT NULL,
 origin TEXT NOT NULL DEFAULT 'console',PRIMARY KEY(namespace,state_key)
);
CREATE TABLE IF NOT EXISTS imported_records (
 record_table TEXT NOT NULL,record_key TEXT NOT NULL,value_json TEXT NOT NULL,observed_at TEXT,updated_at TEXT NOT NULL,
 PRIMARY KEY(record_table,record_key)
);
CREATE TABLE IF NOT EXISTS onboarding_pending (
 operation_id TEXT PRIMARY KEY,upstream_host TEXT NOT NULL,upstream_type TEXT NOT NULL,upstream_key_id TEXT NOT NULL,
 upstream_key_name TEXT,upstream_group_id TEXT NOT NULL,upstream_group_name TEXT NOT NULL,local_group_id TEXT NOT NULL,
 local_group_name TEXT NOT NULL,multiplier TEXT NOT NULL,reason TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL
);
`

func (s *Store) ensureSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, businessSchema)
	return err
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
	if _, err := tx.ExecContext(ctx, `INSERT INTO migration_runs(status,row_count,started_at,completed_at,private_auth_status)
		VALUES('succeeded',0,?,?,?)`, now, now, "无需迁移"); err != nil {
		return err
	}
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
	var status string
	err := queryer.QueryRowContext(ctx, `SELECT status FROM migration_runs ORDER BY id DESC LIMIT 1`).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return status == "succeeded", err
}

func populatedBusinessTables(ctx context.Context, queryer connectionQueryer) ([]string, error) {
	tables := []string{
		"upstreams", "upstream_keys", "upstream_groups", "accounts", "account_groups", "health_samples",
		"routing_decisions", "account_health_evaluations", "bindings", "local_groups", "recharge_rates",
		"policies", "policy_nodes", "paused_accounts", "routing_baselines", "cleanup_states", "runtime_events",
		"alert_incidents", "alert_deliveries", "operation_audit", "run_records", "usage_records",
		"operational_snapshots", "imported_records", "onboarding_pending",
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
		"schema_version": int64(9), "strategy": "balanced", "selection": map[string]any{"strategy": "balanced"},
		"weights": map[string]any{
			"scheduling_missing_rate_fallback": "current_cost_wall", "enabled": true, "budget": int64(400),
			"gate_floor": int64(40), "price_exp": 1.0, "speed_exp": 1.0, "balanced_price_ratio": 0.5,
			"change_threshold": "0.1", "cooldown_seconds": int64(60), "min_load_factor": int64(1), "max_load_factor": int64(100),
		},
		"probe":               map[string]any{"enabled": true, "interval_seconds": int64(300), "timeout_seconds": int64(60), "concurrency": int64(4), "model": "", "prompt": "hi", "skip_when_traffic_fresh": true, "traffic_fresh_seconds": int64(180)},
		"traffic":             map[string]any{"enabled": true, "refresh_seconds": int64(60), "lookback_minutes": int64(120), "max_samples_per_account": int64(60)},
		"upstream_multiplier": map[string]any{"interval_seconds": int64(120)},
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
			"managed_group_mode": "all", "managed_group_ids": []any{}, "excluded_group_ids": []any{},
			"account_types": []any{}, "platforms": []any{}, "paused_account_ids": []any{},
			"excluded_account_ids": []any{}, "manual_fused_account_ids": []any{},
		},
		"group_policy_bindings": map[string]any{},
		"account_test_models":   map[string]any{},
		"auto_apply":            map[string]any{"schedulable": true, "priority": true, "load_factor": true, "concurrency": false},
	}
}
