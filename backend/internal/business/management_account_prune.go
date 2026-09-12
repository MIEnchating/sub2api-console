package business

import (
	"context"
	"database/sql"
)

func (s *Store) pruneManagementDeletedAccounts(
	ctx context.Context,
	tx *sql.Tx,
	remoteIDs map[string]struct{},
	now string,
) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM accounts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	deletedIDs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if _, exists := remoteIDs[id]; !exists {
			deletedIDs = append(deletedIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(deletedIDs) == 0 {
		return deletedIDs, nil
	}
	control, err := s.readPolicyDocument(ctx, tx, "control-plane")
	if err != nil {
		return nil, err
	}
	for _, id := range deletedIDs {
		// Remove current state and bindings; retain health, usage and audit history.
		for _, table := range []string{
			"account_groups", "routing_decisions", "account_health_evaluations", "paused_accounts",
			"manual_priority_accounts", "routing_baselines", "cleanup_states",
		} {
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE account_id=?", id); err != nil {
				return nil, err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM bindings WHERE local_account_id=?`, id); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM accounts WHERE id=?`, id); err != nil {
			return nil, err
		}
		if control != nil {
			removeAccountPolicyReferences(control, id)
		}
	}
	if control != nil {
		if err := s.writePolicyDocument(ctx, tx, "control-plane", control, now); err != nil {
			return nil, err
		}
	}
	return deletedIDs, nil
}
