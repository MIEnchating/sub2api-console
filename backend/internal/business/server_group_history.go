package business

import (
	"context"

	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

// Move history while retaining the same winning evidence as the SQLite path.
// All names here are constants; group names remain bound parameters.
func moveServerGroupHistory(ctx context.Context, tx *dbconn.Tx, currentName, staleName string) error {
	for _, spec := range []struct{ table, identity, newer string }{
		{"health_samples", `stale.account_id=current.account_id AND stale.source=current.source AND stale.evidence_key=current.evidence_key`, `(COALESCE(stale.observed_at,'')>COALESCE(current.observed_at,'') OR (COALESCE(stale.observed_at,'')=COALESCE(current.observed_at,'') AND stale.id>current.id))`},
		{"usage_records", `stale.request_id=current.request_id AND stale.account_id=current.account_id AND stale.observed_at=current.observed_at`, `stale.id>current.id`},
	} {
		removeOlder := tx.Dialect(`DELETE FROM `+spec.table+` AS current USING `+spec.table+` AS stale WHERE `,
			`DELETE current FROM `+spec.table+` AS current JOIN `+spec.table+` AS stale ON `) + spec.identity + ` AND current.group_name=? AND stale.group_name=? AND ` + spec.newer
		if _, err := tx.ExecContext(ctx, removeOlder, currentName, staleName); err != nil {
			return err
		}
		removeDuplicate := tx.Dialect(`DELETE FROM `+spec.table+` AS stale USING `+spec.table+` AS current WHERE `,
			`DELETE stale FROM `+spec.table+` AS stale JOIN `+spec.table+` AS current ON `) + spec.identity + ` AND current.group_name=? AND stale.group_name=?`
		if _, err := tx.ExecContext(ctx, removeDuplicate, currentName, staleName); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE `+spec.table+` SET group_name=? WHERE group_name=?`, currentName, staleName); err != nil {
			return err
		}
	}
	return nil
}
