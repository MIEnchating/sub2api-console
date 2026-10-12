package business_test

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
)

func TestDatabaseContractUpdatingPolicyPreservesDeepLegacyTree(t *testing.T) {
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
			defer func() {
				rows, err := db.QueryContext(t.Context(), `SELECT id FROM policy_nodes WHERE policy_key='control-plane' AND key_name IN ('contract_nested','contract_legacy_extension') ORDER BY id DESC`)
				if err != nil {
					t.Error(err)
					return
				}
				var ids []int64
				for rows.Next() {
					var id int64
					if err := rows.Scan(&id); err != nil {
						t.Error(err)
						break
					}
					ids = append(ids, id)
				}
				if err := rows.Err(); err != nil {
					t.Error(err)
				}
				rows.Close()
				for _, id := range ids {
					if _, err := db.ExecContext(t.Context(), `DELETE FROM policy_nodes WHERE id=?`, id); err != nil {
						t.Error(err)
						return
					}
				}
			}()
			tx, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			var parent int64
			if err := tx.QueryRowContext(t.Context(), `SELECT id FROM policy_nodes WHERE policy_key='control-plane' AND parent_id IS NULL`).Scan(&parent); err != nil {
				t.Fatal(err)
			}
			for index := range 20 {
				key := "contract_nested"
				if index == 0 {
					key = "contract_legacy_extension"
				}
				parent, err = tx.InsertID(t.Context(), `INSERT INTO policy_nodes(policy_key,parent_id,key_name,node_type,updated_at) VALUES('control-plane',?,?,'object','2026-10-12T00:00:00Z')`, parent, key)
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if _, err := store.UpdatePolicy(t.Context(), map[string]any{"cooldown_seconds": 105}, "contract"); err != nil {
				t.Fatalf("deep legacy policy blocked update: %v", err)
			}
			var count int
			if err := db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM policy_nodes WHERE policy_key='control-plane' AND key_name='contract_nested'`).Scan(&count); err != nil || count != 19 {
				t.Fatalf("legacy fields dropped: %d %v", count, err)
			}
		})
	}
}

func TestDatabaseContractPolicyRevisionAllowsOneConcurrentEditor(t *testing.T) {
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
			original, err := store.PolicySnapshot(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if original.CooldownSeconds == nil {
				t.Fatal("initial policy has no cooldown")
			}
			var workers sync.WaitGroup
			var winners atomic.Int32
			start := make(chan struct{})
			for i := 0; i < 6; i++ {
				workers.Go(func() {
					<-start
					// Revisions identify content; a no-op correctly retains its
					// revision. Every competitor must change the initial value.
					cooldown := (*original.CooldownSeconds + i + 1) % 86401
					_, err := store.UpdatePolicy(t.Context(), map[string]any{"expected_revision": original.Revision, "cooldown_seconds": cooldown}, "contract")
					if err == nil {
						winners.Add(1)
					} else if !errors.Is(err, business.ErrPolicyRevisionConflict) {
						t.Errorf("edit: %v", err)
					}
				})
			}
			close(start)
			workers.Wait()
			if winners.Load() != 1 {
				t.Fatalf("concurrent policy winners=%d", winners.Load())
			}
		})
	}
}
