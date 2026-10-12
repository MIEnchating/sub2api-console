package configstore

import (
	"context"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
)

// The current workbench uses versioned documents. Only remove known retired
// tables when empty; populated legacy stores must remain recoverable.
func removeEmptyRetiredTables(ctx context.Context, tx *dbconn.Tx) error {
	for _, table := range []string{
		"workbench_login_profiles", "workbench_oauth_checkpoints", "workbench_queues",
		"workbench_sms_receipts", "workbench_source_profiles",
	} {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_schema WHERE type='table' AND name=?)`, table).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			continue
		}
		var populated bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM `+table+`)`).Scan(&populated); err != nil {
			return err
		}
		if populated {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DROP TABLE `+table); err != nil {
			return err
		}
	}
	return nil
}
