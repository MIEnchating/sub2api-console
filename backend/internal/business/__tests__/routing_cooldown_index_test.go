package business_test

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestRoutingCooldownQueryDoesNotRequireSpecificPartialIndex(t *testing.T) {
	for _, index := range []string{"readback-only", "absent"} {
		t.Run(index, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "routing-cooldown.sqlite3")
			store, err := business.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			if err := store.Bootstrap(t.Context()); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			if _, err := db.Exec(`DROP INDEX ix_operation_audit_routing_lookup`); err != nil {
				t.Fatal(err)
			}
			if index == "readback-only" {
				if _, err := db.Exec(`CREATE INDEX ix_operation_audit_routing_lookup ON operation_audit(object_id,created_at DESC)
					WHERE operation_type='routing.writeback' AND state='succeeded' AND remote_confirmed=1 AND readback_confirmed=1`); err != nil {
					t.Fatal(err)
				}
			}
			at := time.Now().UTC().Format(time.RFC3339Nano)
			if _, err := db.Exec(`INSERT INTO routing_decisions(account_id,group_name,routing_state,updated_at,payload_json)
				VALUES('41','codex','healthy',?,'{}')`, at); err != nil {
				t.Fatal(err)
			}
			field := "concurrency"
			if err := store.RecordAccountOperation(t.Context(), business.AccountOperation{
				OperationID: "successful-scaling", OperationType: "routing.writeback", State: "succeeded", Phase: "remote-write",
				ObjectID: "41", RemoteConfirmed: true, FieldName: &field,
			}); err != nil {
				t.Fatal(err)
			}
			rows, err := store.PreviousRoutingDecisions(t.Context(), nil, nil)
			if err != nil || len(rows) != 1 || rows[0].LastApplyAt.IsZero() || rows[0].LastScalingWriteAt.IsZero() {
				t.Fatalf("cooldown lookup must work without a matching dedicated index: rows=%+v err=%v", rows, err)
			}
		})
	}
}
