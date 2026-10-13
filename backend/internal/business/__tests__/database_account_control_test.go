package business_test

import (
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
)

func TestDatabaseContractAccountControlCommitsConfirmedScheduling(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		t.Run(string(config.Driver), func(t *testing.T) {
			store, err := business.OpenDatabase(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			if err := store.Bootstrap(t.Context()); err != nil {
				t.Fatal(err)
			}
			db, err := dbconn.Open(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.ExecContext(t.Context(), `INSERT INTO accounts(id,name,schedulable,metadata_json,updated_at) VALUES('780041','control-contract',1,'{}','now')`); err != nil {
				t.Fatal(err)
			}
			defer func() {
				for _, query := range []string{
					`DELETE FROM operation_audit WHERE object_id='780041'`,
					`DELETE FROM accounts WHERE id='780041'`,
				} {
					if _, err := db.ExecContext(t.Context(), query); err != nil {
						t.Error(err)
					}
				}
			}()
			for _, scenario := range []struct {
				action  string
				enabled bool
				paused  int
				state   string
			}{
				{"pause", false, 1, "paused"},
				{"resume", true, 0, "unknown"},
				{"fuse", false, 0, "fused"},
				{"recover", true, 0, "unknown"},
			} {
				t.Run(scenario.action, func(t *testing.T) {
					operation := business.AccountOperation{OperationID: "control-contract-" + scenario.action, OperationType: "account.control", State: "succeeded", Phase: "readback", RemoteConfirmed: true, ReadbackConfirmed: true}
					if err := store.CommitAccountControlReadback(t.Context(), "780041", scenario.action, "contract", scenario.enabled, operation); err != nil {
						t.Fatal(err)
					}
					var enabled, paused bool
					var state string
					if err := db.QueryRowContext(t.Context(), `SELECT schedulable,paused,routing_state FROM accounts WHERE id='780041'`).Scan(&enabled, &paused, &state); err != nil {
						t.Fatal(err)
					}
					if enabled != scenario.enabled || paused != (scenario.paused == 1) || state != scenario.state {
						t.Fatalf("confirmed state: enabled=%v paused=%v state=%s", enabled, paused, state)
					}
					var auditCount int
					if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM operation_audit WHERE operation_id=? AND object_id='780041' AND remote_confirmed=1 AND readback_confirmed=1`, operation.OperationID).Scan(&auditCount); err != nil || auditCount != 1 {
						t.Fatalf("confirmed audit: count=%d err=%v", auditCount, err)
					}
				})
			}
		})
	}
}
