package business

import (
	"context"
	"database/sql"
	"strings"
)

// Older writers recorded a pre-write capacity deferral as a failed mutation.
// Recognize only those emitted messages; readback and remote API failures remain alerts.
func legacyCapacityWaitReason(reason string) bool {
	reason = strings.TrimSpace(reason)
	return reason == "上游可用并发已变化，本轮扩容或恢复会超过用户并发上限；请重新计算调度" ||
		strings.HasPrefix(reason, "共享并发额度不足，已拦截本轮扩容或恢复（未执行）：上游用户上限 ")
}

func legacyCapacityWaitAlert(eventType, causeCode string) bool {
	return eventType == "routing.apply_failure" && strings.HasPrefix(causeCode, "APPLY_FAILED:") &&
		legacyCapacityWaitReason(strings.TrimPrefix(causeCode, "APPLY_FAILED:"))
}

func closeLegacyCapacityWaitAlerts(ctx context.Context, tx *sql.Tx, now string) error {
	rows, err := tx.QueryContext(ctx, `SELECT incident_key,cause_code FROM alert_incidents
		WHERE event_type='routing.apply_failure' AND status IN ('firing','recovered','suppressed')`)
	if err != nil {
		return err
	}
	defer rows.Close()
	keys := []string{}
	for rows.Next() {
		var key, cause string
		if err := rows.Scan(&key, &cause); err != nil {
			return err
		}
		if legacyCapacityWaitAlert("routing.apply_failure", cause) {
			keys = append(keys, key)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, key := range keys {
		if _, err := tx.ExecContext(ctx, `UPDATE alert_incidents SET status='closed',last_seen_at=?,
			delivery_status='等待并发额度，已转为调度等待记录',last_error=NULL WHERE incident_key=?`, now, key); err != nil {
			return err
		}
	}
	return nil
}
