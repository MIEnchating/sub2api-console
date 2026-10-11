package business

import (
	"context"
	"database/sql"
)

// Confirmed manual controls still override stale automatic routing decisions,
// but do not produce account breaker alerts.
func (s *Store) manualFusedAlertAccounts(ctx context.Context) (map[string]struct{}, error) {
	policy, err := s.readPolicyDocument(ctx, s.db, "control-plane")
	if err != nil {
		return nil, err
	}
	scope, _ := policy["scope"].(map[string]any)
	return controlAccountIDs(scope["manual_fused_account_ids"]), nil
}

func manualFuseAlert(eventType, causeCode string) bool {
	return eventType == "account.routing_breaker" &&
		(causeCode == "MANUAL_FUSE" || causeCode == "ROUTING_BREAKER:人工熔断")
}

// Retain historical records without announcing a recovery for a retired alert.
func closeLegacyManualFuseAlerts(ctx context.Context, tx *sql.Tx, now string) error {
	_, err := tx.ExecContext(ctx, `UPDATE alert_incidents SET status='closed',last_seen_at=?,
		delivery_status='人工熔断不再告警',last_error=NULL
		WHERE event_type='account.routing_breaker' AND cause_code IN ('MANUAL_FUSE','ROUTING_BREAKER:人工熔断')
		AND status IN ('firing','recovered','suppressed')`, now)
	return err
}
