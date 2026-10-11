package business

import (
	"context"
	"database/sql"
)

// Removing an upstream ends monitoring; it is not evidence of recovery.
// Run inside the writer transaction so a stale evaluation cannot reopen alerts
// after deletion, including alerts left by older versions.
func closeDeletedUpstreamAlerts(ctx context.Context, tx *sql.Tx, host, now string) error {
	query := `UPDATE alert_incidents SET status='closed',last_seen_at=?,
		delivery_status='上游已删除，告警已关闭',last_error=NULL
		WHERE object_kind='host' AND event_type IN (
			'upstream.balance','upstream.auth','upstream.configuration','upstream.rate_sync'
		) AND status IN ('firing','suppressed','recovered')
		AND NOT EXISTS (SELECT 1 FROM upstreams WHERE host=alert_incidents.object_id)`
	args := []any{now}
	if host != "" {
		query += " AND object_id=?"
		args = append(args, host)
	}
	_, err := tx.ExecContext(ctx, query, args...)
	return err
}
