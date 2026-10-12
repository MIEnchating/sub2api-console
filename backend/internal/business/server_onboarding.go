package business

import (
	"context"
	"strings"
)

// MySQL UPSERT handles every unique key, including the separate frozen-intent
// identity. It must never turn a competing operation into an update of the
// existing owner's record. Serialize one operation ID and use explicit writes.
func (s *Store) saveMySQLPendingOnboarding(ctx context.Context, value PendingOnboarding, upstreamID, selection, now string) error {
	tx, err := s.beginResourceTx(ctx, []string{"onboarding-pending/" + value.OperationID})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM onboarding_pending WHERE operation_id=?)`, value.OperationID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		_, err = tx.ExecContext(ctx, `UPDATE onboarding_pending SET upstream_key_id=?,upstream_key_name=?,upstream_account_id=?,reason=?,
		 local_group_ids_json=CASE WHEN local_group_ids_json='' THEN ? ELSE local_group_ids_json END,
		 frozen_intent_json=CASE WHEN frozen_intent_json='' THEN ? ELSE frozen_intent_json END,
		 key_commit_unknown=?,account_commit_unknown=?,updated_at=? WHERE operation_id=?`,
			value.UpstreamKeyID, managementNullableString(value.UpstreamKeyName), value.UpstreamAccountID, safeOnboardingReason(value.Reason), selection, strings.TrimSpace(value.FrozenIntentJSON), boolDatabaseValue(value.KeyCommitUnknown), boolDatabaseValue(value.AccountCommitUnknown), now, value.OperationID)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT INTO onboarding_pending(operation_id,upstream_id,upstream_host,upstream_type,
		 upstream_key_id,upstream_key_name,upstream_account_id,upstream_group_id,upstream_group_name,local_group_id,local_group_name,
		 local_group_ids_json,multiplier,intent_hash,frozen_intent_json,reason,key_commit_unknown,account_commit_unknown,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			value.OperationID, upstreamID, canonicalHost(value.UpstreamHost), strings.TrimSpace(value.UpstreamType), value.UpstreamKeyID, managementNullableString(value.UpstreamKeyName), value.UpstreamAccountID, value.UpstreamGroupID, value.UpstreamGroupName, value.LocalGroupID, value.LocalGroupName, selection, value.Multiplier, value.IntentHash, strings.TrimSpace(value.FrozenIntentJSON), safeOnboardingReason(value.Reason), boolDatabaseValue(value.KeyCommitUnknown), boolDatabaseValue(value.AccountCommitUnknown), value.CreatedAt, now)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
