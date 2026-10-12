package business

import (
	"context"
	"errors"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

func deletePolicyNodes(ctx context.Context, tx *dbconn.Tx, policyKey string) error {
	if tx.Driver() != dbconn.MySQL {
		_, err := tx.ExecContext(ctx, `DELETE FROM policy_nodes WHERE policy_key=?`, policyKey)
		return err
	}
	// InnoDB cascades stop at depth 15. Delete leaves in materialized batches
	// so valid older policy trees can be replaced without disabling constraints.
	for {
		result, err := tx.ExecContext(ctx, `DELETE FROM policy_nodes WHERE id IN (
		 SELECT id FROM (
		  SELECT p.id FROM policy_nodes p LEFT JOIN policy_nodes child ON child.parent_id=p.id
		  WHERE p.policy_key=? AND child.id IS NULL LIMIT 1000
		 ) AS leaves
		)`, policyKey)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count == 0 {
			var remaining bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM policy_nodes WHERE policy_key=?)`, policyKey).Scan(&remaining); err != nil {
				return err
			}
			if remaining {
				return errors.New("策略节点存在循环或跨策略引用，无法更新")
			}
			return nil
		}
	}
}
