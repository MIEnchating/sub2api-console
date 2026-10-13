package business_test

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbconn"
	"github.com/MIEnchating/sub2api-console/backend/internal/dbtest"
)

func TestDatabaseContractBalanceRecoveryPreservesOriginalSchedulingAfterReopenAndCommit(t *testing.T) {
	for _, config := range dbtest.Configs(t) {
		for _, enabled := range []bool{false, true} {
			t.Run(string(config.Driver)+"/schedulable="+strconv.FormatBool(enabled), func(t *testing.T) {
				store, err := business.OpenDatabase(t.Context(), config)
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if store != nil {
						_ = store.Close()
					}
				}()
				db, err := dbconn.Open(t.Context(), config)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				defer func() {
					for _, query := range []string{
						`DELETE FROM operation_audit WHERE object_id='770041'`,
						`DELETE FROM bindings WHERE id=770041`,
						`DELETE FROM accounts WHERE id='770041'`,
						`DELETE FROM upstreams WHERE host='balance.contract.test'`,
						`DELETE FROM upstream_identities WHERE upstream_id='balance-contract'`,
					} {
						if _, err := db.ExecContext(t.Context(), query); err != nil {
							t.Error(err)
						}
					}
				}()
				for _, query := range []string{
					`INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES('balance-contract','now','now')`,
					`INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES('balance.contract.test','balance-contract',1,'now')`,
					`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,raw_balance,updated_at) VALUES('balance.contract.test','https://balance.contract.test','sub2api','ok','10','now')`,
					`INSERT INTO accounts(id,name,schedulable,metadata_json,updated_at) VALUES('770041','balance-contract',0,'{"base_url":"https://balance.contract.test","status":"error","error_message":"quota exceeded"}','now')`,
					`INSERT INTO bindings(id,local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group_id,local_group,updated_at) VALUES(770041,'770041','balance.contract.test','10','key','7','group','now')`,
					`INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,upstream_group_id,updated_at) VALUES(770041,'balance-contract','10','7','now')`,
				} {
					if _, err := db.ExecContext(t.Context(), query); err != nil {
						t.Fatal(err)
					}
				}
				balanceMetadata, err := json.Marshal(map[string]string{"balance_checked_at": time.Now().UTC().Format(time.RFC3339Nano)})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := db.ExecContext(t.Context(), `UPDATE upstreams SET metadata_json=? WHERE host='balance.contract.test'`, string(balanceMetadata)); err != nil {
					t.Fatal(err)
				}
				state := business.BalanceRecoveryState{AccountID: "770041", TargetFingerprint: strings.Repeat("a", 64), BindingFingerprint: strings.Repeat("b", 64), Schedulable: enabled, Depleted: true}
				if err := store.SaveBalanceRecoveryState(t.Context(), state); err != nil {
					t.Fatal(err)
				}
				state.Started = true
				if err := store.SaveBalanceRecoveryState(t.Context(), state); err != nil {
					t.Fatal(err)
				}
				if err := store.Close(); err != nil {
					t.Fatal(err)
				}
				store, err = business.OpenDatabase(t.Context(), config)
				if err != nil {
					t.Fatal(err)
				}
				items, err := store.BalanceRecoveryAccounts(t.Context(), "balance.contract.test")
				if err != nil || len(items) != 1 || !items[0].BalanceKnown || items[0].Depleted || items[0].State == nil || *items[0].State != state {
					t.Fatalf("reopened recovery intent: %+v %v", items, err)
				}
				operation := business.AccountOperation{OperationID: "balance-contract", OperationType: "account.balance_recovery", ObjectID: "770041", State: "succeeded", Phase: "readback", RemoteConfirmed: true, ReadbackConfirmed: true}
				if err := store.CommitBalanceRecovery(t.Context(), state, operation); err != nil {
					t.Fatal(err)
				}
				var schedulable int
				var metadataRaw string
				if err := db.QueryRowContext(t.Context(), `SELECT schedulable,metadata_json FROM accounts WHERE id='770041'`).Scan(&schedulable, &metadataRaw); err != nil {
					t.Fatal(err)
				}
				var metadata map[string]any
				if err := json.Unmarshal([]byte(metadataRaw), &metadata); err != nil {
					t.Fatal(err)
				}
				if (schedulable == 1) != enabled || metadata["status"] != "active" || metadata["error_message"] != nil || metadata["base_url"] != "https://balance.contract.test" {
					t.Fatalf("confirmed recovery changed intent or unrelated metadata: schedulable=%d metadata=%s", schedulable, metadataRaw)
				}
				items, err = store.BalanceRecoveryAccounts(t.Context(), "balance.contract.test")
				if err != nil || len(items) != 1 || items[0].State == nil || items[0].State.Depleted || items[0].State.Started || items[0].State.Schedulable != enabled {
					t.Fatalf("committed recovery state: %+v %v", items, err)
				}
			})
		}
	}
}
