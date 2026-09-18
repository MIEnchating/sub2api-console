package business

import (
	"context"
	"database/sql"
)

const legacyPolicyChangeReason = "等待写回期间策略已变化，请重新计算调度"

// Exclude the old pre-write deferral before selecting the newest outcome so an
// earlier real failure remains active until a confirmed readback supersedes it.
const excludeLegacyPolicyChangeOutcomeSQL = ` AND NOT (
	recent.operation_type='routing.writeback' AND recent.state='failed'
	AND COALESCE(recent.remote_confirmed,0)=0 AND COALESCE(recent.readback_confirmed,0)=0
	AND COALESCE(recent.error,'')='` + legacyPolicyChangeReason + `')`

func legacyPolicyChangeAlertKeys(ctx context.Context, queryer policyQueryer) (map[string]struct{}, error) {
	// Historical incidents did not retain their source audit ID. If this exact
	// cause also belongs to a real failure, retain its notification rather than
	// guessing that a later deferral replaced it (including after recovery).
	rows, err := queryer.QueryContext(ctx, `SELECT i.incident_key FROM alert_incidents i
		WHERE i.event_type='routing.apply_failure' AND i.object_kind='account'
		AND i.status IN ('firing','recovered','suppressed') AND i.cause_code=?
		AND EXISTS(SELECT 1 FROM operation_audit deferred
			WHERE deferred.object_id=i.object_id AND deferred.operation_type='routing.writeback'
			AND deferred.state='failed' AND deferred.error=?
			AND COALESCE(deferred.remote_confirmed,0)=0 AND COALESCE(deferred.readback_confirmed,0)=0)
		AND NOT EXISTS(SELECT 1 FROM operation_audit confirmed
			WHERE confirmed.object_id=i.object_id AND confirmed.operation_type IN ('routing.writeback','cleanup.delete')
			AND confirmed.state='failed' AND confirmed.error=?
			AND (confirmed.operation_type='cleanup.delete' OR COALESCE(confirmed.remote_confirmed,0)<>0 OR COALESCE(confirmed.readback_confirmed,0)<>0))`,
		"APPLY_FAILED:"+legacyPolicyChangeReason, legacyPolicyChangeReason, legacyPolicyChangeReason)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := map[string]struct{}{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys[key] = struct{}{}
	}
	return keys, rows.Err()
}

func closeLegacyPolicyChangeAlerts(ctx context.Context, tx *sql.Tx, now string) error {
	keys, err := legacyPolicyChangeAlertKeys(ctx, tx)
	if err != nil {
		return err
	}
	for key := range keys {
		if _, err := tx.ExecContext(ctx, `UPDATE alert_incidents SET status='closed',last_seen_at=?,
			delivery_status='策略已变化，本轮未执行，已转为调度跳过记录',last_error=NULL WHERE incident_key=?`, now, key); err != nil {
			return err
		}
	}
	return nil
}
