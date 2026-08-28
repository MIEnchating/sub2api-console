package business

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"time"
)

func (s *Store) migrateLegacySchema(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := rebuildLegacyHealthSamples(ctx, tx); err != nil {
		return err
	}
	migrations := map[string]map[string]string{
		"migration_runs": {
			"private_auth_imported": "INTEGER NOT NULL DEFAULT 0", "private_auth_status": "TEXT NOT NULL DEFAULT '未执行'", "private_auth_error": "TEXT",
		},
		"account_groups": {"group_id": "TEXT", "group_rate": "TEXT"},
		"accounts": {
			"target_priority": "INTEGER", "target_load_factor": "TEXT", "target_schedulable": "INTEGER", "target_concurrency": "INTEGER",
		},
		"upstreams": {"raw_balance": "TEXT", "mapped_balance": "TEXT"},
		"local_groups": {
			"remote_id": "TEXT", "strategy_source": "TEXT NOT NULL DEFAULT 'global_default'", "platform": "TEXT", "rate_multiplier": "TEXT",
			"profit_control_enabled": "INTEGER", "profit_min_margin": "TEXT", "profit_safety_buffer": "TEXT",
		},
		"operational_snapshots": {"origin": "TEXT NOT NULL DEFAULT 'console'"},
		"operation_audit": {
			"operation_id": "TEXT NOT NULL DEFAULT ''", "phase": "TEXT NOT NULL DEFAULT ''", "request_id": "TEXT",
			"actor": "TEXT", "source": "TEXT", "error": "TEXT", "remote_confirmed": "INTEGER", "readback_confirmed": "INTEGER",
			"object_type": "TEXT", "object_id": "TEXT", "object_name": "TEXT", "group_names_json": "TEXT NOT NULL DEFAULT '[]'",
			"field_name": "TEXT", "before_json": "TEXT", "after_json": "TEXT", "writeback": "INTEGER NOT NULL DEFAULT 1",
		},
		"routing_baselines": {
			"status": "TEXT", "ownership_version": "INTEGER NOT NULL DEFAULT 0", "managed_schedulable": "INTEGER",
			"managed_priority": "INTEGER", "managed_load_factor": "TEXT", "managed_concurrency": "INTEGER", "managed_status": "TEXT",
		},
	}
	for table, columns := range migrations {
		existing, err := tableColumns(ctx, tx, table)
		if err != nil {
			return err
		}
		for column, declaration := range columns {
			if _, present := existing[column]; present {
				continue
			}
			if _, err := tx.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, declaration)); err != nil {
				return fmt.Errorf("迁移 %s.%s 失败: %w", table, column, err)
			}
		}
	}
	snapshotColumns, err := tableColumns(ctx, tx, "operational_snapshots")
	if err != nil {
		return err
	}
	if _, oldPresent := snapshotColumns["source_updated_at"]; oldPresent {
		if _, newPresent := snapshotColumns["observed_at"]; !newPresent {
			if _, err := tx.ExecContext(ctx, `ALTER TABLE operational_snapshots RENAME COLUMN source_updated_at TO observed_at`); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS ux_health_samples_evidence ON health_samples(source,evidence_key,account_id,group_name)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS ix_health_samples_latest ON health_samples(account_id,group_name,observed_at DESC,id DESC)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS ix_health_samples_source_latest ON health_samples(source,account_id,group_name,observed_at DESC,id DESC)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE upstreams SET raw_balance=CAST(balance AS TEXT) WHERE raw_balance IS NULL AND balance IS NOT NULL`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE upstreams SET mapped_balance=CAST(balance AS TEXT) WHERE mapped_balance IS NULL AND balance IS NOT NULL`); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx, `INSERT INTO recharge_rates(host,recharge_rate,note,updated_at)
		SELECT u.host,'1','console-migration-default',?
		FROM upstreams u LEFT JOIN recharge_rates r ON r.host=u.host
		WHERE r.host IS NULL`, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE operation_audit SET writeback=0 WHERE phase='calculation' OR operation_type='upstream.rate_sync'
		OR (operation_type IN ('account.scheduling','routing.writeback') AND state='succeeded' AND remote_confirmed=0)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM health_samples
		WHERE LOWER(REPLACE(source,'-','_')) IN ('active_probe','probe')
		AND LOWER(TRIM(result)) IN ('跳过','skip','skipped')`); err != nil {
		return err
	}
	if err := s.migrateControlPolicyContract(ctx, tx); err != nil {
		return err
	}
	if err := initializeRoutingDecisionEpoch(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `PRAGMA user_version=9`); err != nil {
		return err
	}
	return tx.Commit()
}

type contextExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func initializeRoutingDecisionEpoch(ctx context.Context, executor contextExecer) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := executor.ExecContext(ctx, `INSERT INTO app_state(key,value_json,updated_at)
		SELECT 'routing-decision-epoch','{}',? WHERE
		EXISTS(SELECT 1 FROM routing_decisions) AND
		NOT EXISTS(SELECT 1 FROM app_state WHERE key='routing-decision-epoch')`, now)
	return err
}

func (s *Store) migrateControlPolicyContract(ctx context.Context, tx *sql.Tx) error {
	policy, err := s.readPolicyDocument(ctx, tx, "control-plane")
	if err != nil || policy == nil {
		return err
	}
	original := deepCopyPolicyValue(policy).(map[string]any)
	normalizeCurrentPolicyContract(policy)
	pruneDeprecatedPolicyFields(policy)
	policy["schema_version"] = int64(9)
	if reflect.DeepEqual(original, policy) {
		return nil
	}
	return s.writePolicyDocument(ctx, tx, "control-plane", policy, time.Now().UTC().Format(time.RFC3339Nano))
}

func normalizeCurrentPolicyContract(policy map[string]any) {
	traffic, _ := policy["traffic"].(map[string]any)
	if traffic == nil {
		traffic = map[string]any{}
		policy["traffic"] = traffic
	}
	if _, present := traffic["enabled"]; !present {
		enabled := true
		if health, ok := policy["health"].(map[string]any); ok {
			if source, ok := health["source"].(string); ok {
				enabled = strings.EqualFold(strings.TrimSpace(source), "traffic")
			}
		}
		traffic["enabled"] = enabled
	}
	delete(policy, "health")

	probe, _ := policy["probe"].(map[string]any)
	if probe == nil {
		probe = map[string]any{}
		policy["probe"] = probe
	}
	if _, present := probe["model"]; !present {
		model, _ := probe["default_model"].(string)
		probe["model"] = strings.TrimSpace(model)
	}
	delete(probe, "default_model")

	if autoApply, ok := policy["auto_apply"].(map[string]any); ok {
		for _, field := range []string{"schedulable", "priority", "load_factor", "concurrency"} {
			switch value := autoApply[field].(type) {
			case string:
				if value == "apply" {
					autoApply[field] = true
				} else if value == "shadow" {
					autoApply[field] = false
				}
			}
		}
	}
	fillMissingPolicyDefaults(policy, initialControlPolicy())
}

func fillMissingPolicyDefaults(target, defaults map[string]any) {
	for key, defaultValue := range defaults {
		current, present := target[key]
		if !present {
			target[key] = deepCopyPolicyValue(defaultValue)
			continue
		}
		currentObject, currentOK := current.(map[string]any)
		defaultObject, defaultOK := defaultValue.(map[string]any)
		if currentOK && defaultOK {
			fillMissingPolicyDefaults(currentObject, defaultObject)
		}
	}
}

func pruneDeprecatedPolicyFields(policy map[string]any) {
	delete(policy, "scheduling")
	if weights, ok := policy["weights"].(map[string]any); ok {
		delete(weights, "max_writes_per_group")
		delete(weights, "max_migration_ratio")
	}
	for section, coreFields := range advancedPolicyCoreFields {
		object, ok := policy[section].(map[string]any)
		if !ok {
			continue
		}
		allowed := advancedRules[section]
		for field := range object {
			if _, core := coreFields[field]; core {
				continue
			}
			if _, current := allowed[field]; !current {
				delete(object, field)
			}
		}
	}
}

func deepCopyPolicyValue(value any) any {
	switch item := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(item))
		for key, child := range item {
			result[key] = deepCopyPolicyValue(child)
		}
		return result
	case []any:
		result := make([]any, len(item))
		for index, child := range item {
			result[index] = deepCopyPolicyValue(child)
		}
		return result
	default:
		return item
	}
}

func rebuildLegacyHealthSamples(ctx context.Context, tx *sql.Tx) error {
	var createSQL sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='health_samples'`).Scan(&createSQL); err != nil {
		return err
	}
	if !createSQL.Valid || !strings.Contains(strings.ReplaceAll(createSQL.String, "\n", " "), "UNIQUE(account_id, group_name, observed_at)") {
		columns, err := tableColumns(ctx, tx, "health_samples")
		if err != nil {
			return err
		}
		if _, present := columns["evidence_key"]; !present {
			_, err = tx.ExecContext(ctx, `ALTER TABLE health_samples ADD COLUMN evidence_key TEXT`)
		}
		return err
	}
	statements := []string{
		`ALTER TABLE health_samples RENAME TO health_samples_legacy`,
		`CREATE TABLE health_samples (
			id INTEGER PRIMARY KEY AUTOINCREMENT,account_id TEXT NOT NULL,group_name TEXT NOT NULL,result TEXT,
			latency_p50 TEXT,latency_p95 TEXT,latency_p99 TEXT,sample_count INTEGER,attempts INTEGER,failure_reason TEXT,
			observed_at TEXT,source TEXT NOT NULL,evidence_key TEXT,payload_json TEXT NOT NULL DEFAULT '{}',
			UNIQUE(source,evidence_key,account_id,group_name)
		)`,
		`INSERT INTO health_samples(id,account_id,group_name,result,latency_p50,latency_p95,latency_p99,sample_count,attempts,
			failure_reason,observed_at,source,evidence_key,payload_json)
		 SELECT id,account_id,group_name,result,latency_p50,latency_p95,latency_p99,sample_count,attempts,failure_reason,
			observed_at,source,'legacy:'||id,payload_json FROM health_samples_legacy`,
		`DROP TABLE health_samples_legacy`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func tableColumns(ctx context.Context, queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, table string) (map[string]struct{}, error) {
	rows, err := queryer.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]struct{}{}
	for rows.Next() {
		var index int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&index, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, err
		}
		result[name] = struct{}{}
	}
	return result, rows.Err()
}
