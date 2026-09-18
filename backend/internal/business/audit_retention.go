package business

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Keep one unchanged automatic confirmation per hour. Actual writes, manual
// operations and the first confirmation after a failure remain separate audits.
func repeatedAutomaticReadback(ctx context.Context, tx *sql.Tx, op AccountOperation, before, after any, groups string) (bool, error) {
	if op.OperationType != "routing.writeback" || strings.TrimSpace(op.Actor) != "自动巡检" ||
		op.State != "succeeded" || op.Phase != "readback" || op.RemoteConfirmed ||
		!op.ReadbackConfirmed || op.Writeback || op.Error != nil || op.FieldName != nil ||
		before == nil || before != after || before == "null" {
		return false, nil
	}
	var createdAt string
	err := tx.QueryRowContext(ctx, `SELECT created_at FROM operation_audit WHERE source_id=(
		SELECT (`+latestRoutingOutcomeSQL+`) FROM (SELECT ? AS id) a
	) AND operation_type='routing.writeback' AND state='succeeded' AND phase='readback'
		AND actor=? AND source='console' AND remote_confirmed=0 AND readback_confirmed=1
		AND writeback=0 AND error IS NULL AND field_name IS NULL
		AND before_json=? AND after_json=? AND group_names_json=? AND object_name IS ?`,
		op.ObjectID, strings.TrimSpace(op.Actor), before, after, groups, managementNullableString(op.ObjectName)).Scan(&createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	last, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return false, nil
	}
	age := time.Since(last)
	return age >= 0 && age < time.Hour, nil
}
