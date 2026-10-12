package business_test

import (
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
)

func TestDatabaseContractInspectionRoutingPending(t *testing.T) {
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
			calculatedAt := time.Date(2099, 1, 1, 12, 0, 0, 0, time.UTC)
			for _, tc := range []struct {
				name      string
				newPolicy bool
				decision  bool
				stale     bool
				attempts  []string
				pending   bool
			}{
				{name: "no decisions after calculation stays idle"},
				{name: "new policy requires calculation", newPolicy: true, pending: true},
				{name: "decision without attempt requires writeback", decision: true, pending: true},
				{name: "failed writeback remains pending", decision: true, attempts: []string{"failed"}, pending: true},
				{name: "successful retry clears pending", decision: true, attempts: []string{"failed", "succeeded"}},
				{name: "latest failure supersedes success", decision: true, attempts: []string{"succeeded", "failed"}, pending: true},
				{name: "new policy supersedes successful writeback", newPolicy: true, decision: true, attempts: []string{"succeeded"}, pending: true},
				{name: "old decision before current epoch is ignored", decision: true, stale: true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					if err := store.PersistRoutingRound(t.Context(), nil, nil, nil, nil, nil, nil, nil, true, calculatedAt); err != nil {
						t.Fatal(err)
					}
					if _, err := db.ExecContext(t.Context(), db.Dialect(`INSERT INTO app_state("key",value_json,updated_at) VALUES('routing-decision-epoch','{}',?) ON CONFLICT("key") DO UPDATE SET updated_at=excluded.updated_at`, `INSERT INTO app_state("key",value_json,updated_at) VALUES('routing-decision-epoch','{}',?) ON DUPLICATE KEY UPDATE updated_at=VALUES(updated_at)`), calculatedAt.Format(time.RFC3339Nano)); err != nil {
						t.Fatal(err)
					}
					policyAt := calculatedAt.Add(-time.Minute)
					if tc.newPolicy {
						policyAt = calculatedAt.Add(time.Minute)
					}
					if _, err := db.ExecContext(t.Context(), `UPDATE policy_nodes SET updated_at=? WHERE policy_key='control-plane'`, policyAt.Format(time.RFC3339Nano)); err != nil {
						t.Fatal(err)
					}
					if tc.decision {
						decidedAt := calculatedAt
						if tc.stale {
							decidedAt = calculatedAt.Add(-time.Minute)
						}
						if _, err := db.ExecContext(t.Context(), `INSERT INTO routing_decisions(account_id,group_name,updated_at,payload_json) VALUES('inspection-contract','contract',?,'{}')`, decidedAt.Format(time.RFC3339Nano)); err != nil {
							t.Fatal(err)
						}
						defer db.ExecContext(t.Context(), `DELETE FROM routing_decisions WHERE account_id='inspection-contract'`)
					}
					for i, state := range tc.attempts {
						if _, err := db.ExecContext(t.Context(), `INSERT INTO operation_audit(source_id,operation_id,operation_type,state,phase,object_id,created_at) VALUES(?,'inspection-contract','routing.writeback',?,'remote-write','inspection-contract',?)`, -9000001-i, state, calculatedAt.Add(time.Duration(i+1)*time.Minute).Format(time.RFC3339Nano)); err != nil {
							t.Fatal(err)
						}
					}
					defer db.ExecContext(t.Context(), `DELETE FROM operation_audit WHERE operation_id='inspection-contract'`)
					pending, err := store.RoutingWritebackPending(t.Context())
					if err != nil || pending != tc.pending {
						t.Fatalf("pending=%v, want %v; error=%v", pending, tc.pending, err)
					}
				})
			}
		})
	}
}
