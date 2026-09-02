package business

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyUpstreamSyncAtomicallyReplacesCatalogAndMapsDecimalBalance(t *testing.T) {
	store := upstreamSyncTestStore(t)
	ctx := context.Background()
	active, old := "active", "0.2"
	if _, err := store.db.Exec(`INSERT INTO upstream_groups(host,group_id,name,status,raw_rate,updated_at) VALUES
		('api.example','bound-old','bound','active','9','now'),('api.example','unused-old','unused','active','9','now');
		INSERT INTO upstream_keys(host,key_id,name,upstream_group,status,metadata_json,updated_at) VALUES
		('api.example','bound-key','bound','bound-old','active','{}','now'),('api.example','unused-key','unused','unused-old','active','{}','now');
		INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group_id,local_group,metadata_json,updated_at)
		VALUES('12','api.example','bound-key','bound','bound-old','codex','{}','now'),
		('13','api.example','17','pro-key','7','codex','{}','now')`); err != nil {
		t.Fatal(err)
	}
	result, err := store.ApplyUpstreamSync(ctx, UpstreamSyncWrite{
		Host: "HTTPS://API.EXAMPLE/", AuthenticationOK: true, AuthRecovered: true,
		Catalog: &UpstreamCatalogSnapshot{
			Groups: []UpstreamCatalogGroup{{GroupID: "7", Name: "pro", Status: &active, RawRate: &old}},
			Keys:   []UpstreamCatalogKey{{KeyID: "17", Name: "pro-key", UpstreamGroup: stringPointer("7"), Status: &active, Rate: &old}},
		},
		Balance: &UpstreamBalanceObservation{
			RawBalance: stringPointer("1"), DisplayBalance: stringPointer("7.3"), BalanceUnit: stringPointer("cny"),
			Status: "已读取", SiteName: stringPointer("Example"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.GroupCount != 1 || result.KeyCount != 1 || result.AccountTotal != 2 ||
		result.AccountRateSucceeded != 1 || result.AccountRateFailed != 1 ||
		result.Balance == nil || *result.Balance != "3.3333333333333333333333333333" ||
		result.DisplayBalance == nil || *result.DisplayBalance != "7.3" || result.BalanceUnit == nil || *result.BalanceUnit != "cny" {
		t.Fatalf("unexpected result: %#v", result)
	}
	var effective, authStatus string
	if err := store.db.QueryRow(`SELECT effective_rate FROM upstream_groups WHERE host='api.example' AND group_id='7'`).Scan(&effective); err != nil {
		t.Fatal(err)
	}
	if effective != "0.666667" {
		t.Fatalf("wrong effective rate: %s", effective)
	}
	if err := store.db.QueryRow(`SELECT auth_status FROM upstreams WHERE host='api.example'`).Scan(&authStatus); err != nil || authStatus != "已恢复" {
		t.Fatalf("auth status=%q err=%v", authStatus, err)
	}
	for kind, id := range map[string]string{"group": "bound-old", "key": "bound-key", "group-unused": "unused-old", "key-unused": "unused-key"} {
		entityKind := strings.TrimSuffix(kind, "-unused")
		var lifecycle string
		if err := store.db.QueryRow(`SELECT lifecycle_state FROM upstream_catalog_entities
			WHERE entity_kind=? AND entity_id=?`, entityKind, id).Scan(&lifecycle); err != nil || lifecycle != "suspected" {
			t.Fatalf("stale entity %s/%s was not retained for confirmation: lifecycle=%q err=%v", entityKind, id, lifecycle, err)
		}
	}
}

func TestApplyUpstreamSyncRoundsConvertedGroupMultiplier(t *testing.T) {
	store := upstreamSyncTestStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `UPDATE recharge_rates SET recharge_rate='10' WHERE host='api.example'`); err != nil {
		t.Fatal(err)
	}
	active, rate := "active", "1.1000000000000001"
	if _, err := store.ApplyUpstreamSync(ctx, UpstreamSyncWrite{
		Host: "api.example", AuthenticationOK: true,
		Catalog: &UpstreamCatalogSnapshot{Groups: []UpstreamCatalogGroup{{
			GroupID: "7", Name: "pro", Status: &active, RawRate: &rate,
		}}},
	}); err != nil {
		t.Fatal(err)
	}
	var effective string
	if err := store.db.QueryRowContext(ctx, `SELECT effective_rate FROM upstream_groups
		WHERE host='api.example' AND group_id='7'`).Scan(&effective); err != nil {
		t.Fatal(err)
	}
	if effective != "0.11" {
		t.Fatalf("converted multiplier was not rounded: %q", effective)
	}
}

func TestCatalogSyncDoesNotRewriteManualPriorityBinding(t *testing.T) {
	store := openReadModelFixture(t)
	ctx := context.Background()
	if _, err := store.AssignManualPriority(ctx, "41", 3, "100", 100, false, "operator"); err != nil {
		t.Fatal(err)
	}
	active, rate := "active", "0.2"
	_, err := store.ApplyUpstreamSync(ctx, UpstreamSyncWrite{
		Host: "api.example", AuthenticationOK: true,
		Catalog: &UpstreamCatalogSnapshot{
			Groups: []UpstreamCatalogGroup{{GroupID: "1", Name: "renamed-group", Status: &active, RawRate: &rate}},
			Keys: []UpstreamCatalogKey{{
				KeyID: "key-1", Name: "renamed-key", UpstreamGroup: stringPointer("1"), Status: &active, Rate: &rate,
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var keyName, groupName string
	if err := store.db.QueryRow(`SELECT upstream_key_name,upstream_group FROM bindings WHERE local_account_id='41'`).Scan(&keyName, &groupName); err != nil {
		t.Fatal(err)
	}
	if keyName != "codex-key" || groupName != "codex" {
		t.Fatalf("manual binding was rewritten: key=%q group=%q", keyName, groupName)
	}
}

func TestApplyUpstreamSyncConfirmsMissingCatalogObjectsAfterTwoCompleteSnapshots(t *testing.T) {
	store := upstreamSyncTestStore(t)
	ctx := context.Background()
	active, rate := "active", "0.2"
	if _, err := store.db.Exec(`INSERT INTO accounts(id,name,schedulable,metadata_json,updated_at)
		VALUES('41','example-0.2',1,'{}','now');
		INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group,upstream_group_id,local_group,metadata_json,updated_at)
		VALUES('41','api.example','key-1','key-1','pro','group-1','codex','{}','now')`); err != nil {
		t.Fatal(err)
	}
	live := &UpstreamCatalogSnapshot{
		Groups: []UpstreamCatalogGroup{{GroupID: "group-1", Name: "pro", Status: &active, RawRate: &rate}},
		Keys:   []UpstreamCatalogKey{{KeyID: "key-1", Name: "key-1", UpstreamGroup: stringPointer("group-1"), Status: &active}},
	}
	if _, err := store.ApplyUpstreamSync(ctx, UpstreamSyncWrite{Host: "api.example", Catalog: live, AuthenticationOK: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyUpstreamSync(ctx, UpstreamSyncWrite{Host: "api.example", Catalog: &UpstreamCatalogSnapshot{}, AuthenticationOK: true}); err != nil {
		t.Fatal(err)
	}
	first, err := store.Accounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].KeyStatus == nil || *first[0].KeyStatus != "suspected" {
		t.Fatalf("first absent snapshot must wait for confirmation: %#v", first)
	}
	if _, err := store.ApplyUpstreamSync(ctx, UpstreamSyncWrite{Host: "api.example", Catalog: &UpstreamCatalogSnapshot{}, AuthenticationOK: true}); err != nil {
		t.Fatal(err)
	}
	second, err := store.Accounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 || second[0].KeyStatus == nil || *second[0].KeyStatus != "key_and_group_missing" ||
		second[0].KeyStatusReason == nil || !strings.Contains(*second[0].KeyStatusReason, "连续 2 次完整同步") {
		t.Fatalf("second absent snapshot must confirm deletion: %#v", second)
	}
	if _, err := store.ApplyUpstreamSync(ctx, UpstreamSyncWrite{Host: "api.example", Catalog: live, AuthenticationOK: true}); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.Accounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 1 || recovered[0].KeyStatus == nil || *recovered[0].KeyStatus != "active" {
		t.Fatalf("returned stable IDs must restore the binding: %#v", recovered)
	}
}

func TestApplyNameOnlySyncPreservesBalanceAndAuthenticationState(t *testing.T) {
	store := upstreamSyncTestStore(t)
	result, err := store.ApplyUpstreamSync(context.Background(), UpstreamSyncWrite{
		Host: "api.example", NameOnly: true,
		Balance: &UpstreamBalanceObservation{SiteName: stringPointer("New API"), Status: "未读取"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RawBalance == nil || *result.RawBalance != "1" {
		t.Fatalf("result=%#v", result)
	}
	var rawBalance, authStatus, metadata string
	if err := store.db.QueryRow(`SELECT raw_balance,auth_status,metadata_json FROM upstreams WHERE host='api.example'`).Scan(&rawBalance, &authStatus, &metadata); err != nil {
		t.Fatal(err)
	}
	if rawBalance != "1" || authStatus != "已鉴权" || !strings.Contains(metadata, `"site_name":"New API"`) {
		t.Fatalf("raw=%q auth=%q metadata=%s", rawBalance, authStatus, metadata)
	}
}

func TestApplyUpstreamBalanceKeepsBalanceAtUpstreamScope(t *testing.T) {
	store := upstreamSyncTestStore(t)
	if _, err := store.db.Exec(`INSERT INTO accounts(id,name,balance,metadata_json,updated_at) VALUES
		('11','Example-0.1',NULL,'{}','now'),('12','Example-0.2',NULL,'{}','now')`); err != nil {
		t.Fatal(err)
	}
	result, err := store.ApplyUpstreamSync(context.Background(), UpstreamSyncWrite{
		Host: "api.example", AuthenticationOK: true,
		Balance: &UpstreamBalanceObservation{RawBalance: stringPointer("6"), Status: "已读取"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Balance == nil || *result.Balance != "20" {
		t.Fatalf("result=%#v", result)
	}
	var accountBalances int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM accounts WHERE balance IS NOT NULL`).Scan(&accountBalances); err != nil {
		t.Fatal(err)
	}
	if accountBalances != 0 {
		t.Fatalf("upstream balance was copied into %d accounts", accountBalances)
	}
}

func TestAccountRateObservationAndConfirmedRateUseSeparateBindingFields(t *testing.T) {
	store := upstreamSyncTestStore(t)
	if _, err := store.db.Exec(`INSERT INTO accounts(id,name,multiplier,metadata_json,updated_at) VALUES('11','Example-0.1','0.1','{}','now');
		INSERT INTO account_groups(account_id,group_name,group_rate) VALUES('11','codex','0.1');
		INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,local_rate,upstream_rate,metadata_json,updated_at)
		VALUES('11','api.example','key-11','key-11','codex','0.1','0.1','{}','now')`); err != nil {
		t.Fatal(err)
	}
	if err := store.CommitAccountRateObservations(context.Background(), []AccountRateObservation{{AccountID: "11", Rate: "0.2"}}); err != nil {
		t.Fatal(err)
	}
	var localRate, upstreamRate string
	if err := store.db.QueryRow(`SELECT local_rate,upstream_rate FROM bindings WHERE local_account_id='11'`).Scan(&localRate, &upstreamRate); err != nil {
		t.Fatal(err)
	}
	var observedGroupRate string
	if err := store.db.QueryRow(`SELECT group_rate FROM account_groups WHERE account_id='11'`).Scan(&observedGroupRate); err != nil {
		t.Fatal(err)
	}
	if localRate != "0.1" || upstreamRate != "0.2" || observedGroupRate != "0.1" {
		t.Fatalf("observed rates local=%q upstream=%q group=%q", localRate, upstreamRate, observedGroupRate)
	}
	name := "Example-0.2"
	multiplier := "0.2"
	if err := store.CommitAccountFieldsReadback(context.Background(), "11", &name, nil, nil, nil, &multiplier, nil, nil, false, nil, AccountOperation{
		OperationID: "rate-sync-11", OperationType: "account.sync", State: "succeeded", Phase: "readback", Actor: "auto-inspection",
		RemoteConfirmed: true, ReadbackConfirmed: true, ObjectID: "11", ObjectName: &name, Before: map[string]any{}, After: map[string]any{}, Writeback: true,
	}); err != nil {
		t.Fatal(err)
	}
	var accountName, accountRate, groupRate string
	if err := store.db.QueryRow(`SELECT name,multiplier FROM accounts WHERE id='11'`).Scan(&accountName, &accountRate); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT group_rate FROM account_groups WHERE account_id='11'`).Scan(&groupRate); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT local_rate,upstream_rate FROM bindings WHERE local_account_id='11'`).Scan(&localRate, &upstreamRate); err != nil {
		t.Fatal(err)
	}
	if accountName != "Example-0.2" || accountRate != "0.2" || groupRate != "0.2" || localRate != "0.2" || upstreamRate != "0.2" {
		t.Fatalf("account=%q rate=%q group=%q local=%q upstream=%q", accountName, accountRate, groupRate, localRate, upstreamRate)
	}
}

func TestApplyUpstreamSyncRejectsInvalidCatalogWithoutPartialWrites(t *testing.T) {
	store := upstreamSyncTestStore(t)
	ctx := context.Background()
	rate := "0.1"
	_, err := store.ApplyUpstreamSync(ctx, UpstreamSyncWrite{
		Host: "api.example", AuthenticationOK: true,
		Catalog: &UpstreamCatalogSnapshot{Groups: []UpstreamCatalogGroup{
			{GroupID: "7", Name: "one", RawRate: &rate}, {GroupID: "7", Name: "duplicate", RawRate: &rate},
		}},
		Balance: &UpstreamBalanceObservation{RawBalance: stringPointer("2"), Status: "已读取"},
	})
	if err == nil || !strings.Contains(err.Error(), "重复 ID") {
		t.Fatalf("err=%v", err)
	}
	var rawBalance string
	if err := store.db.QueryRow(`SELECT raw_balance FROM upstreams WHERE host='api.example'`).Scan(&rawBalance); err != nil || rawBalance != "1" {
		t.Fatalf("invalid batch changed balance: raw=%q err=%v", rawBalance, err)
	}
	var groups int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM upstream_groups WHERE host='api.example'`).Scan(&groups); err != nil || groups != 0 {
		t.Fatalf("invalid batch partially wrote groups: count=%d err=%v", groups, err)
	}
}

func TestApplyUpstreamSyncRepairsMissingRechargeRate(t *testing.T) {
	store := upstreamSyncTestStore(t)
	ctx := context.Background()
	if _, err := store.db.Exec(`DELETE FROM recharge_rates WHERE host='api.example'`); err != nil {
		t.Fatal(err)
	}
	active, rate := "active", "0.5"
	result, err := store.ApplyUpstreamSync(ctx, UpstreamSyncWrite{
		Host: "api.example",
		Catalog: &UpstreamCatalogSnapshot{Groups: []UpstreamCatalogGroup{{
			GroupID: "7", Name: "pro", Status: &active, RawRate: &rate,
		}}},
		AuthenticationOK: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.GroupCount != 1 {
		t.Fatalf("result=%#v", result)
	}
	var persisted string
	if err := store.db.QueryRow(`SELECT recharge_rate FROM recharge_rates WHERE host='api.example'`).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != "1" {
		t.Fatalf("recharge rate=%q", persisted)
	}
}

func TestApplyUpstreamSyncInheritsRechargeRateFromStableUpstreamHost(t *testing.T) {
	store := upstreamSyncTestStore(t)
	ctx := context.Background()
	var upstreamID string
	if err := store.db.QueryRowContext(ctx, `SELECT upstream_id FROM upstream_identity_hosts WHERE host='api.example'`).Scan(&upstreamID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE recharge_rates SET recharge_rate='10' WHERE host='api.example';
		INSERT INTO upstreams(host,base_url,upstream_type,auth_mode,enabled,auth_status,metadata_json,updated_at)
		VALUES('auth.example','https://auth.example','sub2api','sub2api_user_token',1,'已鉴权','{}','now');
		INSERT INTO upstream_identity_hosts(upstream_id,host,is_primary,updated_at) VALUES(?,'auth.example',0,'now')`, upstreamID); err != nil {
		t.Fatal(err)
	}
	active, rate := "active", "1.5"
	if _, err := store.ApplyUpstreamSync(ctx, UpstreamSyncWrite{
		Host: "auth.example", AuthenticationOK: true,
		Catalog: &UpstreamCatalogSnapshot{Groups: []UpstreamCatalogGroup{{
			GroupID: "7", Name: "pro", Status: &active, RawRate: &rate,
		}}},
	}); err != nil {
		t.Fatal(err)
	}
	var recharge, effective string
	if err := store.db.QueryRowContext(ctx, `SELECT
		(SELECT recharge_rate FROM recharge_rates WHERE host='auth.example'),
		(SELECT effective_rate FROM upstream_groups WHERE host='auth.example' AND group_id='7')`).Scan(
		&recharge, &effective,
	); err != nil {
		t.Fatal(err)
	}
	if recharge != "10" || effective != "0.15" {
		t.Fatalf("recharge=%q effective=%q", recharge, effective)
	}
}

func TestApplyUpstreamSyncForOneKeyDoesNotReconcileUnselectedCatalogRows(t *testing.T) {
	store := upstreamSyncTestStore(t)
	ctx := context.Background()
	if _, err := store.db.Exec(`INSERT INTO upstream_groups(host,group_id,name,status,raw_rate,updated_at) VALUES
		('api.example','old-group','old group','active','9','now');
		INSERT INTO upstream_keys(host,key_id,name,upstream_group,rate,status,metadata_json,updated_at) VALUES
		('api.example','old-key','old key','old-group','9','active','{}','now');
		UPDATE upstreams SET metadata_json='{"catalog_status":"同步失败","rate_sync_status":"failed","rate_sync_error":"完整目录读取失败","rate_sync_at":"before"}'
		WHERE host='api.example'`); err != nil {
		t.Fatal(err)
	}
	active, rate, selected := "active", "0.6", "selected-key"
	result, err := store.ApplyUpstreamSync(ctx, UpstreamSyncWrite{
		Host: "api.example", KeyID: &selected, AuthenticationOK: true,
		Catalog: &UpstreamCatalogSnapshot{
			Groups: []UpstreamCatalogGroup{
				{GroupID: "selected-group", Name: "selected", Status: &active, RawRate: &rate},
				{GroupID: "other-group", Name: "other", Status: &active, RawRate: &rate},
			},
			Keys: []UpstreamCatalogKey{
				{KeyID: selected, Name: "selected", UpstreamGroup: stringPointer("selected-group"), Status: &active, Rate: &rate},
				{KeyID: "other-key", Name: "other", UpstreamGroup: stringPointer("other-group"), Status: &active, Rate: &rate},
			},
		},
		Balance: &UpstreamBalanceObservation{RawBalance: stringPointer("3"), Status: "已读取"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.KeyCount != 1 || result.GroupCount != 1 {
		t.Fatalf("partial result counted the full catalog: %#v", result)
	}
	for table, id := range map[string]string{"upstream_groups": "old-group", "upstream_keys": "old-key"} {
		idColumn := map[string]string{"upstream_groups": "group_id", "upstream_keys": "key_id"}[table]
		var status string
		if err := store.db.QueryRow(`SELECT status FROM `+table+` WHERE host='api.example' AND `+idColumn+`=?`, id).Scan(&status); err != nil || status != "active" {
			t.Fatalf("unselected row was reconciled: table=%s id=%s status=%q err=%v", table, id, status, err)
		}
	}
	var otherCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM upstream_keys WHERE host='api.example' AND key_id='other-key'`).Scan(&otherCount); err != nil || otherCount != 0 {
		t.Fatalf("partial sync wrote an unselected key: count=%d err=%v", otherCount, err)
	}
	var metadataRaw string
	if err := store.db.QueryRow(`SELECT metadata_json FROM upstreams WHERE host='api.example'`).Scan(&metadataRaw); err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(metadataRaw), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["rate_sync_status"] != "failed" || metadata["rate_sync_error"] != "完整目录读取失败" || metadata["rate_sync_at"] != "before" {
		t.Fatalf("partial sync overwrote Host-wide rate status: %#v", metadata)
	}
}

func TestApplyUpstreamSyncRepairsBindingGroupFromAuthoritativeKeyCatalog(t *testing.T) {
	store := upstreamSyncTestStore(t)
	if _, err := store.db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('28','pro-0.2','{}','now');
		INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group,upstream_group_id,local_group,metadata_json,updated_at)
		VALUES('28','api.example','4924','pro','gptproo','4924','pro','{}','now');
		INSERT INTO upstream_groups(host,group_id,name,status,raw_rate,updated_at)
		VALUES('api.example','4924','stale','missing','0.2','now')`); err != nil {
		t.Fatal(err)
	}
	active, rate := "active", "0.2"
	if _, err := store.ApplyUpstreamSync(context.Background(), UpstreamSyncWrite{
		Host: "api.example", AuthenticationOK: true,
		Catalog: &UpstreamCatalogSnapshot{
			Groups: []UpstreamCatalogGroup{{GroupID: "gptproo", Name: "gptproo", Status: &active, RawRate: &rate}},
			Keys: []UpstreamCatalogKey{{
				KeyID: "4924", Name: "pro", UpstreamGroup: stringPointer("gptproo"), Status: &active,
			}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	var groupID, groupName string
	if err := store.db.QueryRow(`SELECT upstream_group_id,upstream_group FROM bindings
		WHERE upstream_host='api.example' AND upstream_key_id='4924'`).Scan(&groupID, &groupName); err != nil {
		t.Fatal(err)
	}
	if groupID != "gptproo" || groupName != "gptproo" {
		t.Fatalf("binding group was not repaired: id=%q name=%q", groupID, groupName)
	}
	var lifecycle string
	if err := store.db.QueryRow(`SELECT lifecycle_state FROM upstream_catalog_entities
		WHERE entity_kind='group' AND entity_id='4924'`).Scan(&lifecycle); err != nil {
		t.Fatal(err)
	}
	if lifecycle != "missing" {
		t.Fatalf("stale group identity must remain as a tombstone: lifecycle=%q", lifecycle)
	}
}

func TestRecordUpstreamSyncFailurePreservesLastKnownValues(t *testing.T) {
	store := upstreamSyncTestStore(t)
	if err := store.RecordUpstreamSyncFailure(context.Background(), "api.example", "all", "HTTP 401", true); err != nil {
		t.Fatal(err)
	}
	var authStatus, rawBalance, mappedBalance, metadataRaw string
	if err := store.db.QueryRow(`SELECT auth_status,raw_balance,mapped_balance,metadata_json FROM upstreams WHERE host='api.example'`).Scan(
		&authStatus, &rawBalance, &mappedBalance, &metadataRaw,
	); err != nil {
		t.Fatal(err)
	}
	if authStatus != "鉴权失效" || rawBalance != "1" || mappedBalance != "3.3333333333333333333333333333" {
		t.Fatalf("last values were not preserved: auth=%q raw=%q mapped=%q", authStatus, rawBalance, mappedBalance)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(metadataRaw), &metadata); err != nil || metadata["auth_error"] != "HTTP 401" {
		t.Fatalf("failure metadata=%#v err=%v", metadata, err)
	}
}

func TestRecordBalanceFailurePreservesAuthenticationAndRateStatus(t *testing.T) {
	store := upstreamSyncTestStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `UPDATE upstreams SET auth_status='已鉴权',
		metadata_json='{"rate_sync_status":"succeeded","rate_sync_error":null,"rate_sync_at":"before","balance_status":"已读取"}'
		WHERE host='api.example'`); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordUpstreamSyncFailure(ctx, "api.example", "balance", "balance timeout", false); err != nil {
		t.Fatal(err)
	}
	var authStatus, metadataRaw string
	if err := store.db.QueryRowContext(ctx, `SELECT auth_status,metadata_json FROM upstreams WHERE host='api.example'`).Scan(&authStatus, &metadataRaw); err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(metadataRaw), &metadata); err != nil {
		t.Fatal(err)
	}
	if authStatus != "已鉴权" || metadata["rate_sync_status"] != "succeeded" || metadata["rate_sync_at"] != "before" ||
		metadata["balance_status"] != "读取失败" || metadata["balance_error"] != "balance timeout" {
		t.Fatalf("balance failure changed unrelated Host state: auth=%q metadata=%#v", authStatus, metadata)
	}
}

func TestRecordCatalogFailureMarksRateFailedWithoutChangingAuthentication(t *testing.T) {
	store := upstreamSyncTestStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `UPDATE upstreams SET auth_status='已鉴权',
		metadata_json='{"rate_sync_status":"succeeded","rate_sync_error":null,"rate_sync_at":"before","balance_status":"已读取"}'
		WHERE host='api.example'`); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordUpstreamSyncFailure(ctx, "api.example", "catalog", "catalog timeout", false); err != nil {
		t.Fatal(err)
	}
	var authStatus, metadataRaw string
	if err := store.db.QueryRowContext(ctx, `SELECT auth_status,metadata_json FROM upstreams WHERE host='api.example'`).Scan(&authStatus, &metadataRaw); err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(metadataRaw), &metadata); err != nil {
		t.Fatal(err)
	}
	if authStatus != "已鉴权" || metadata["rate_sync_status"] != "failed" || metadata["rate_sync_error"] != "catalog timeout" ||
		metadata["balance_status"] != "已读取" {
		t.Fatalf("catalog failure state: auth=%q metadata=%#v", authStatus, metadata)
	}
}

func TestRecordSingleKeyFailurePreservesHostRateStatus(t *testing.T) {
	store := upstreamSyncTestStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `UPDATE upstreams SET auth_status='已鉴权',
		metadata_json='{"catalog_status":"已同步","rate_sync_status":"succeeded","rate_sync_error":null,"rate_sync_at":"before"}'
		WHERE host='api.example'`); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordUpstreamSyncFailure(ctx, "api.example", "key", "single key timeout", false); err != nil {
		t.Fatal(err)
	}
	var authStatus, metadataRaw string
	if err := store.db.QueryRowContext(ctx, `SELECT auth_status,metadata_json FROM upstreams WHERE host='api.example'`).Scan(&authStatus, &metadataRaw); err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(metadataRaw), &metadata); err != nil {
		t.Fatal(err)
	}
	if authStatus != "已鉴权" || metadata["catalog_status"] != "已同步" || metadata["rate_sync_status"] != "succeeded" ||
		metadata["rate_sync_at"] != "before" {
		t.Fatalf("single-Key failure changed Host-wide state: auth=%q metadata=%#v", authStatus, metadata)
	}
}

func TestUpstreamGroupHistoryRecordsChangesAfterInitialCatalogBaseline(t *testing.T) {
	store := upstreamSyncTestStore(t)
	ctx := context.Background()
	active, rate := "active", "0.2"
	baseline := &UpstreamCatalogSnapshot{Groups: []UpstreamCatalogGroup{{
		GroupID: "group-1", Name: "标准组", Status: &active, RawRate: &rate,
	}}}
	if _, err := store.ApplyUpstreamSync(ctx, UpstreamSyncWrite{Host: "api.example", Catalog: baseline, AuthenticationOK: true}); err != nil {
		t.Fatal(err)
	}
	rows, err := store.UpstreamGroupHistory(ctx, "api.example", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("initial catalog was recorded as a change: %#v", rows)
	}

	changed := &UpstreamCatalogSnapshot{Groups: []UpstreamCatalogGroup{
		{GroupID: "group-1", Name: "标准组", Status: &active, RawRate: &rate},
		{GroupID: "group-2", Name: "新增组", Status: &active, RawRate: &rate},
	}}
	if _, err := store.ApplyUpstreamSync(ctx, UpstreamSyncWrite{Host: "api.example", Catalog: changed, AuthenticationOK: true}); err != nil {
		t.Fatal(err)
	}
	rows, err = store.UpstreamGroupHistory(ctx, "api.example", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ChangeType != "added" || rows[0].GroupID != "group-2" || rows[0].GroupName != "新增组" {
		t.Fatalf("added history=%#v", rows)
	}

	if _, err := store.ApplyUpstreamSync(ctx, UpstreamSyncWrite{Host: "api.example", Catalog: baseline, AuthenticationOK: true}); err != nil {
		t.Fatal(err)
	}
	rows, err = store.UpstreamGroupHistory(ctx, "api.example", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("suspected missing group was recorded too early: %#v", rows)
	}
	if _, err := store.ApplyUpstreamSync(ctx, UpstreamSyncWrite{Host: "api.example", Catalog: baseline, AuthenticationOK: true}); err != nil {
		t.Fatal(err)
	}
	rows, err = store.UpstreamGroupHistory(ctx, "api.example", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ChangeType != "removed" || rows[0].GroupID != "group-2" {
		t.Fatalf("removed history=%#v", rows)
	}
}

func TestUpstreamGroupHistoryTreatsAnEmptyCompleteCatalogAsBaseline(t *testing.T) {
	store := upstreamSyncTestStore(t)
	ctx := context.Background()
	if _, err := store.ApplyUpstreamSync(ctx, UpstreamSyncWrite{
		Host: "api.example", Catalog: &UpstreamCatalogSnapshot{}, AuthenticationOK: true,
	}); err != nil {
		t.Fatal(err)
	}
	active, rate := "active", "0.2"
	if _, err := store.ApplyUpstreamSync(ctx, UpstreamSyncWrite{
		Host: "api.example", AuthenticationOK: true,
		Catalog: &UpstreamCatalogSnapshot{Groups: []UpstreamCatalogGroup{{
			GroupID: "first-group", Name: "首个分组", Status: &active, RawRate: &rate,
		}}},
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := store.UpstreamGroupHistory(ctx, "api.example", 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ChangeType != "added" || rows[0].GroupID != "first-group" {
		t.Fatalf("history=%#v", rows)
	}
}

func upstreamSyncTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	name := "Example"
	if _, err := store.CreateUpstreamConfiguration(ctx, UpstreamConfigurationWrite{
		Host: "api.example", Name: &name, BaseURL: "https://api.example", UpstreamType: "sub2api",
		AuthMode: "sub2api_user_token", RechargeRate: "0.3",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE upstreams SET raw_balance='1',mapped_balance='3.3333333333333333333333333333' WHERE host='api.example'`); err != nil {
		t.Fatal(err)
	}
	return store
}
