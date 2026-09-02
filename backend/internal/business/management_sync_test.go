package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestManagementSnapshotUsesStableIDsAndPreservesLocalPolicyAndPartialFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "management.sqlite3")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	result, err := store.SyncManagementSnapshot(ctx, []map[string]any{
		{
			"id": json.Number("11"), "name": "alpha", "schedulable": true,
			"priority": json.Number("20"), "load_factor": json.Number("3"), "rate_multiplier": json.Number("0.25"),
			"groups": []any{json.Number("7")}, "group_rate_by_group": map[string]any{"codex": json.Number("4")},
			"credentials": map[string]any{"base_url": "https://api.openai.com/v1", "api_key": "must-not-persist"},
		},
		{"id": json.Number("12"), "name": "retained", "groups": []any{}},
	}, []map[string]any{
		{"id": json.Number("7"), "name": "codex", "platform": "openai", "rate_multiplier": json.Number("2"), "strategy": "price_first"},
	}, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if result.Accounts != 2 || result.Groups != 1 || result.GroupLinks != 1 || result.RemoteWrite || !result.ReadOnly || result.EventID >= 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE local_groups SET strategy='reliability',strategy_source='group_override' WHERE remote_id='7'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{
		{"id": json.Number("11"), "name": "alpha-renamed"},
	}, []map[string]any{
		{"id": json.Number("7"), "name": "codex", "strategy": "speed_first"},
	}, "tester"); err != nil {
		t.Fatal(err)
	}
	var name, loadFactor, multiplier string
	var priority int
	var schedulable bool
	if err := db.QueryRow(`SELECT name,schedulable,priority,load_factor,multiplier FROM accounts WHERE id='11'`).Scan(
		&name, &schedulable, &priority, &loadFactor, &multiplier,
	); err != nil {
		t.Fatal(err)
	}
	if name != "alpha-renamed" || !schedulable || priority != 20 || loadFactor != "3" || multiplier != "0.25" {
		t.Fatalf("partial snapshot erased fields: name=%q schedulable=%v priority=%d load=%q multiplier=%q", name, schedulable, priority, loadFactor, multiplier)
	}
	var retained int
	if err := db.QueryRow(`SELECT COUNT(*) FROM accounts WHERE id='12'`).Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("missing partial account was deleted: count=%d err=%v", retained, err)
	}
	var strategy, source, platform, groupRate string
	if err := db.QueryRow(`SELECT strategy,strategy_source,platform,rate_multiplier FROM local_groups WHERE remote_id='7'`).Scan(
		&strategy, &source, &platform, &groupRate,
	); err != nil {
		t.Fatal(err)
	}
	if strategy != "reliability" || source != "group_override" {
		t.Fatalf("remote catalog overwrote local policy: strategy=%q source=%q", strategy, source)
	}
	if platform != "openai" || groupRate != "2" {
		t.Fatalf("partial group snapshot erased metadata: platform=%q rate=%q", platform, groupRate)
	}
	if err := db.QueryRow(`SELECT group_rate FROM account_groups WHERE account_id='11' AND group_name='codex'`).Scan(&groupRate); err != nil {
		t.Fatal(err)
	}
	if groupRate != "0.25" {
		t.Fatalf("management per-group rate replaced the account cost: %q", groupRate)
	}
	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{
		{"id": json.Number("11"), "rate_multiplier": json.Number("0.17")},
	}, []map[string]any{}, "tester"); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT group_rate FROM account_groups WHERE account_id='11' AND group_name='codex'`).Scan(&groupRate); err != nil {
		t.Fatal(err)
	}
	if groupRate != "0.17" {
		t.Fatalf("account multiplier update left stale membership rate: %q", groupRate)
	}
	var metadataRaw string
	if err := db.QueryRow(`SELECT metadata_json FROM accounts WHERE id='11'`).Scan(&metadataRaw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(metadataRaw, `"base_url":"https://api.openai.com/v1"`) || strings.Contains(metadataRaw, "must-not-persist") {
		t.Fatalf("account Base URL was not safely projected: %s", metadataRaw)
	}
}

func TestManagementSnapshotNormalizesCompositeGroupPlatform(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "management-composite-group.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := store.SyncManagementSnapshot(ctx, nil, []map[string]any{{
		"id": json.Number("7"), "name": "复合分组", "platform": " Composite ",
	}}, "tester"); err != nil {
		t.Fatal(err)
	}

	groups, err := store.Groups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Platform == nil || *groups[0].Platform != "composite" {
		t.Fatalf("composite group platform was not normalized: %#v", groups)
	}
}

func TestManagementSnapshotClearsCurrentProjectionsWhenAccountBecomesUngrouped(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "management-account-ungrouped.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	groups := []map[string]any{{"id": json.Number("7"), "name": "codex-特价", "platform": "openai"}}
	grouped := []map[string]any{{"id": json.Number("101"), "name": "AI Gateway-0.12", "groups": []any{json.Number("7")}}}
	if _, err := store.SyncManagementSnapshot(ctx, grouped, groups, "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO routing_decisions(account_id,group_name,updated_at,payload_json) VALUES('101','codex-特价','now','{}');
		INSERT INTO account_health_evaluations(account_id,group_name,evaluated_at) VALUES('101','codex-特价','now');
		INSERT INTO health_samples(account_id,group_name,result,source,evidence_key,payload_json)
		VALUES('101','codex-特价','passed','active-probe','probe-ungrouped','{}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(ctx, grouped, groups, "tester"); err != nil {
		t.Fatal(err)
	}
	var unchangedCurrent int
	if err := store.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM routing_decisions WHERE account_id='101')+
		(SELECT COUNT(*) FROM account_health_evaluations WHERE account_id='101')`).Scan(&unchangedCurrent); err != nil {
		t.Fatal(err)
	}
	if unchangedCurrent != 2 {
		t.Fatalf("unchanged membership invalidated current projections: %d", unchangedCurrent)
	}

	ungrouped := []map[string]any{{"id": json.Number("101"), "name": "AI Gateway-0.12", "group_ids": []any{}}}
	if _, err := store.SyncManagementSnapshot(ctx, ungrouped, groups, "tester"); err != nil {
		t.Fatal(err)
	}
	var current, history int
	if err := store.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM account_groups WHERE account_id='101')+
		(SELECT COUNT(*) FROM routing_decisions WHERE account_id='101')+
		(SELECT COUNT(*) FROM account_health_evaluations WHERE account_id='101')`).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM health_samples WHERE account_id='101'`).Scan(&history); err != nil {
		t.Fatal(err)
	}
	if current != 0 || history != 1 {
		t.Fatalf("current=%d history=%d", current, history)
	}
}

func TestManagementSnapshotReconcilesGroupRenameByStableID(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "management-group-rename.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "groups": []any{json.Number("7")},
	}}, []map[string]any{{
		"id": json.Number("7"), "name": "old-name", "platform": "openai",
	}}, "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		UPDATE local_groups SET strategy='reliability',strategy_source='group_override' WHERE remote_id='7';
		INSERT INTO local_groups(name,remote_id,strategy,strategy_source,platform,updated_at)
		VALUES('new-name','7','balanced','global_default','openai','now');
		INSERT INTO routing_decisions(account_id,group_name,updated_at,payload_json) VALUES('11','old-name','now','{}');
		INSERT INTO account_health_evaluations(account_id,group_name,evaluated_at) VALUES('11','old-name','now');
		INSERT INTO health_samples(account_id,group_name,result,source,evidence_key,payload_json)
		VALUES('11','old-name','passed','active-probe','probe-1','{}');
		INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,status,metadata_json,updated_at)
		VALUES('11','upstream.example','91','key','old-name','active','{}','now')`); err != nil {
		t.Fatal(err)
	}

	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "groups": []any{json.Number("7")},
	}}, []map[string]any{{
		"id": json.Number("7"), "name": "new-name", "platform": "openai",
	}}, "tester"); err != nil {
		t.Fatal(err)
	}

	groups, err := store.Groups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].ID == nil || *groups[0].ID != "7" || groups[0].Name != "new-name" {
		t.Fatalf("renamed group was duplicated: %#v", groups)
	}
	if groups[0].Strategy != "reliability" || groups[0].StrategySource != "group_override" {
		t.Fatalf("renamed group lost its policy: %#v", groups[0])
	}
	for table := range map[string]struct{}{
		"account_groups": {}, "routing_decisions": {}, "account_health_evaluations": {}, "health_samples": {},
	} {
		var count int
		if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE group_name='new-name'`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%s did not migrate to the renamed group: count=%d", table, count)
		}
	}
	var bindingGroup string
	if err := store.db.QueryRowContext(ctx, `SELECT local_group FROM bindings WHERE local_account_id='11'`).Scan(&bindingGroup); err != nil {
		t.Fatal(err)
	}
	if bindingGroup != "new-name" {
		t.Fatalf("binding retained stale group name: %q", bindingGroup)
	}
	var stale int
	if err := store.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM local_groups WHERE name='old-name')+
		(SELECT COUNT(*) FROM account_groups WHERE group_name='old-name')+
		(SELECT COUNT(*) FROM routing_decisions WHERE group_name='old-name')+
		(SELECT COUNT(*) FROM account_health_evaluations WHERE group_name='old-name')+
		(SELECT COUNT(*) FROM health_samples WHERE group_name='old-name')+
		(SELECT COUNT(*) FROM bindings WHERE local_group='old-name')`).Scan(&stale); err != nil {
		t.Fatal(err)
	}
	if stale != 0 {
		t.Fatalf("rename left %d stale group references", stale)
	}
}

func TestManagementSnapshotReusesDeletedGroupNameForNewStableID(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "management-group-name-reuse.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "groups": []any{json.Number("13")},
	}}, []map[string]any{{
		"id": json.Number("13"), "name": "DeepSeek", "platform": "openai",
	}}, "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		UPDATE local_groups SET strategy='reliability',strategy_source='group_override' WHERE remote_id='13';
		INSERT INTO health_samples(account_id,group_name,result,source,evidence_key,payload_json)
		VALUES('11','DeepSeek','passed','active-probe','probe-reused-name','{}');
		INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,status,metadata_json,updated_at)
		VALUES('11','upstream.example','91','key','DeepSeek','active','{}','now')`); err != nil {
		t.Fatal(err)
	}
	control, err := store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	control["group_policy_bindings"] = map[string]any{
		"13": map[string]any{"strategy": "reliability"},
		"14": map[string]any{"strategy": "speed_first"},
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.writePolicyDocument(ctx, tx, "control-plane", control, "now"); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	result, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "groups": []any{json.Number("14")},
	}}, []map[string]any{{
		"id": json.Number("14"), "name": "DeepSeek", "platform": "openai",
	}}, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedGroups != 1 || result.Groups != 1 {
		t.Fatalf("result=%#v", result)
	}
	var remoteID, strategy, strategySource, membershipID string
	if err := store.db.QueryRowContext(ctx, `SELECT remote_id,strategy,strategy_source FROM local_groups WHERE name='DeepSeek'`).Scan(
		&remoteID, &strategy, &strategySource,
	); err != nil {
		t.Fatal(err)
	}
	if remoteID != "14" || strategy != "balanced" || strategySource != "global_default" {
		t.Fatalf("replacement group inherited deleted identity: id=%q strategy=%q source=%q", remoteID, strategy, strategySource)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT group_id FROM account_groups WHERE account_id='11'`).Scan(&membershipID); err != nil {
		t.Fatal(err)
	}
	if membershipID != "14" {
		t.Fatalf("replacement membership kept deleted ID: %q", membershipID)
	}
	var historicalGroup, bindingGroup string
	if err := store.db.QueryRowContext(ctx, `SELECT group_name FROM health_samples WHERE evidence_key='probe-reused-name'`).Scan(&historicalGroup); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT local_group FROM bindings WHERE local_account_id='11'`).Scan(&bindingGroup); err != nil {
		t.Fatal(err)
	}
	if historicalGroup == "DeepSeek" || bindingGroup == "DeepSeek" ||
		!strings.Contains(historicalGroup, "13") || !strings.Contains(bindingGroup, "13") {
		t.Fatalf("deleted identity was attributed to replacement: history=%q binding=%q", historicalGroup, bindingGroup)
	}
	control, err = store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	bindings := control["group_policy_bindings"].(map[string]any)
	if _, found := bindings["13"]; found {
		t.Fatalf("deleted stable ID policy remains: %#v", bindings)
	}
	if _, found := bindings["14"]; !found {
		t.Fatalf("replacement stable ID policy was removed: %#v", bindings)
	}
}

func TestManagementSnapshotReusesNameAfterConfirmedDeletionWithoutReattributingReferences(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "management-group-name-reuse-after-delete.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "groups": []any{json.Number("13")},
	}}, []map[string]any{{
		"id": json.Number("13"), "name": "DeepSeek",
	}}, "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO health_samples(account_id,group_name,result,source,evidence_key,payload_json)
		VALUES('11','DeepSeek','passed','active-probe','probe-reused-after-delete','{}');
		INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,status,metadata_json,updated_at)
		VALUES('11','upstream.example','91','key','DeepSeek','active','{}','now');
		INSERT INTO usage_records(request_id,account_id,account_name,group_name,is_error,observed_at,source,payload_json)
		VALUES('req-reused-after-delete','11','alpha','DeepSeek',0,'2026-09-01T00:00:00Z','traffic','{}')`); err != nil {
		t.Fatal(err)
	}

	deleted, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "groups": []any{},
	}}, nil, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if deleted.DeletedGroups != 1 {
		t.Fatalf("deleted result=%#v", deleted)
	}
	var archivedHistory, archivedBinding, archivedUsage string
	if err := store.db.QueryRowContext(ctx, `SELECT group_name FROM health_samples
		WHERE evidence_key='probe-reused-after-delete'`).Scan(&archivedHistory); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT local_group FROM bindings WHERE local_account_id='11'`).Scan(&archivedBinding); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT group_name FROM usage_records
		WHERE request_id='req-reused-after-delete'`).Scan(&archivedUsage); err != nil {
		t.Fatal(err)
	}
	if archivedHistory != archivedBinding || archivedHistory != archivedUsage || archivedHistory == "DeepSeek" ||
		!strings.Contains(archivedHistory, "13") {
		t.Fatalf("deleted identity was not archived: history=%q binding=%q usage=%q", archivedHistory, archivedBinding, archivedUsage)
	}
	var deletedRows int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM local_groups WHERE remote_id='13'`).Scan(&deletedRows); err != nil {
		t.Fatal(err)
	}
	if deletedRows != 0 {
		t.Fatalf("deleted stable identity retained %d local rows", deletedRows)
	}

	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "groups": []any{json.Number("14")},
	}}, []map[string]any{{
		"id": json.Number("14"), "name": "DeepSeek",
	}}, "tester"); err != nil {
		t.Fatal(err)
	}
	var replacementID, membershipID, historyAfter, bindingAfter, usageAfter string
	if err := store.db.QueryRowContext(ctx, `SELECT remote_id FROM local_groups WHERE name='DeepSeek'`).Scan(&replacementID); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT group_id FROM account_groups
		WHERE account_id='11' AND group_name='DeepSeek'`).Scan(&membershipID); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT group_name FROM health_samples
		WHERE evidence_key='probe-reused-after-delete'`).Scan(&historyAfter); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT local_group FROM bindings WHERE local_account_id='11'`).Scan(&bindingAfter); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT group_name FROM usage_records
		WHERE request_id='req-reused-after-delete'`).Scan(&usageAfter); err != nil {
		t.Fatal(err)
	}
	if replacementID != "14" || membershipID != "14" || historyAfter != archivedHistory ||
		bindingAfter != archivedBinding || usageAfter != archivedUsage {
		t.Fatalf("replacement reused deleted identity: replacement=%q membership=%q history=%q binding=%q usage=%q",
			replacementID, membershipID, historyAfter, bindingAfter, usageAfter)
	}
}

func TestManagementSnapshotSwapsGroupNamesByStableID(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "management-group-name-swap.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{
		{"id": json.Number("11"), "name": "first", "groups": []any{json.Number("7")}},
		{"id": json.Number("12"), "name": "second", "groups": []any{json.Number("8")}},
	}, []map[string]any{
		{"id": json.Number("7"), "name": "alpha", "platform": "openai"},
		{"id": json.Number("8"), "name": "beta", "platform": "anthropic"},
	}, "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		UPDATE local_groups SET strategy='reliability',strategy_source='group_override' WHERE remote_id='7';
		UPDATE local_groups SET strategy='speed_first',strategy_source='group_override' WHERE remote_id='8';
		INSERT INTO health_samples(account_id,group_name,result,source,evidence_key,payload_json)
		VALUES('11','alpha','passed','active-probe','probe-swap-7','{}');
		INSERT INTO health_samples(account_id,group_name,result,source,evidence_key,payload_json)
		VALUES('12','beta','passed','active-probe','probe-swap-8','{}')`); err != nil {
		t.Fatal(err)
	}

	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{
		{"id": json.Number("11"), "name": "first", "groups": []any{json.Number("7")}},
		{"id": json.Number("12"), "name": "second", "groups": []any{json.Number("8")}},
	}, []map[string]any{
		{"id": json.Number("7"), "name": "beta", "platform": "openai"},
		{"id": json.Number("8"), "name": "alpha", "platform": "anthropic"},
	}, "tester"); err != nil {
		t.Fatal(err)
	}

	for _, expected := range []struct {
		groupID, name, strategy, accountID, evidenceKey string
	}{
		{groupID: "7", name: "beta", strategy: "reliability", accountID: "11", evidenceKey: "probe-swap-7"},
		{groupID: "8", name: "alpha", strategy: "speed_first", accountID: "12", evidenceKey: "probe-swap-8"},
	} {
		var name, strategy, membershipName, sampleName string
		if err := store.db.QueryRowContext(ctx, `SELECT name,strategy FROM local_groups WHERE remote_id=?`, expected.groupID).Scan(
			&name, &strategy,
		); err != nil {
			t.Fatal(err)
		}
		if err := store.db.QueryRowContext(ctx, `SELECT group_name FROM account_groups WHERE account_id=? AND group_id=?`,
			expected.accountID, expected.groupID,
		).Scan(&membershipName); err != nil {
			t.Fatal(err)
		}
		if err := store.db.QueryRowContext(ctx, `SELECT group_name FROM health_samples WHERE evidence_key=?`, expected.evidenceKey).Scan(&sampleName); err != nil {
			t.Fatal(err)
		}
		if name != expected.name || strategy != expected.strategy || membershipName != expected.name || sampleName != expected.name {
			t.Fatalf("group %s lost identity: name=%q strategy=%q membership=%q sample=%q", expected.groupID, name, strategy, membershipName, sampleName)
		}
	}
}

func TestManagementSnapshotGroupRenameKeepsNewestConflictingHealthSample(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "management-group-health-merge.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "groups": []any{json.Number("7")},
	}}, []map[string]any{{
		"id": json.Number("7"), "name": "old-name",
	}}, "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO local_groups(name,remote_id,strategy,strategy_source,updated_at)
		VALUES('new-name','7','balanced','global_default','now');
		INSERT INTO health_samples(account_id,group_name,result,observed_at,source,evidence_key,payload_json)
		VALUES('11','old-name','newer','2026-09-01T08:00:00Z','active-probe','source-newer','{"winner":"source"}');
		INSERT INTO health_samples(account_id,group_name,result,observed_at,source,evidence_key,payload_json)
		VALUES('11','new-name','older','2026-09-01T07:00:00Z','active-probe','source-newer','{"winner":"destination"}');
		INSERT INTO health_samples(account_id,group_name,result,observed_at,source,evidence_key,payload_json)
		VALUES('11','old-name','older','2026-09-01T07:00:00Z','active-probe','destination-newer','{"winner":"source"}');
		INSERT INTO health_samples(account_id,group_name,result,observed_at,source,evidence_key,payload_json)
		VALUES('11','new-name','newer','2026-09-01T08:00:00Z','active-probe','destination-newer','{"winner":"destination"}')`); err != nil {
		t.Fatal(err)
	}

	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "groups": []any{json.Number("7")},
	}}, []map[string]any{{
		"id": json.Number("7"), "name": "new-name",
	}}, "tester"); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []struct {
		evidenceKey string
		winner      string
	}{
		{evidenceKey: "source-newer", winner: "source"},
		{evidenceKey: "destination-newer", winner: "destination"},
	} {
		var count int
		var groupName, observedAt, payload string
		if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*),MAX(group_name),MAX(observed_at),MAX(payload_json)
			FROM health_samples WHERE account_id='11' AND source='active-probe' AND evidence_key=?`, expected.evidenceKey,
		).Scan(&count, &groupName, &observedAt, &payload); err != nil {
			t.Fatal(err)
		}
		if count != 1 || groupName != "new-name" || observedAt != "2026-09-01T08:00:00Z" ||
			!strings.Contains(payload, `"winner":"`+expected.winner+`"`) {
			t.Fatalf("evidence %q merged incorrectly: count=%d group=%q observed=%q payload=%s", expected.evidenceKey, count, groupName, observedAt, payload)
		}
	}
}

func TestManagementSnapshotGroupRenameKeepsNewestConflictingUsageRecord(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "management-group-usage-merge.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "groups": []any{json.Number("7")},
	}}, []map[string]any{{
		"id": json.Number("7"), "name": "old-name",
	}}, "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO usage_records(request_id,account_id,account_name,group_name,observed_at,source,payload_json)
		VALUES('source-newer','11','alpha','new-name','2026-09-01T08:00:00Z','traffic','{"winner":"destination"}');
		INSERT INTO usage_records(request_id,account_id,account_name,group_name,observed_at,source,payload_json)
		VALUES('source-newer','11','alpha','old-name','2026-09-01T08:00:00Z','traffic','{"winner":"source"}');
		INSERT INTO usage_records(request_id,account_id,account_name,group_name,observed_at,source,payload_json)
		VALUES('destination-newer','11','alpha','old-name','2026-09-01T08:00:00Z','traffic','{"winner":"source"}');
		INSERT INTO usage_records(request_id,account_id,account_name,group_name,observed_at,source,payload_json)
		VALUES('destination-newer','11','alpha','new-name','2026-09-01T08:00:00Z','traffic','{"winner":"destination"}')`); err != nil {
		t.Fatal(err)
	}

	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "groups": []any{json.Number("7")},
	}}, []map[string]any{{
		"id": json.Number("7"), "name": "new-name",
	}}, "tester"); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []struct {
		requestID string
		winner    string
	}{
		{requestID: "source-newer", winner: "source"},
		{requestID: "destination-newer", winner: "destination"},
	} {
		var count int
		var groupName, payload string
		if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*),MAX(group_name),MAX(payload_json)
			FROM usage_records WHERE request_id=?`, expected.requestID,
		).Scan(&count, &groupName, &payload); err != nil {
			t.Fatal(err)
		}
		if count != 1 || groupName != "new-name" || !strings.Contains(payload, `"winner":"`+expected.winner+`"`) {
			t.Fatalf("usage %q merged incorrectly: count=%d group=%q payload=%s", expected.requestID, count, groupName, payload)
		}
	}
}

func TestManagementSnapshotGroupStagingPrefersCurrentNameMetadata(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "management-group-staging-canonical.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "groups": []any{json.Number("7")},
	}}, []map[string]any{{
		"id": json.Number("7"), "name": "a-old", "rate_multiplier": json.Number("1"),
	}}, "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO local_groups(
		name,remote_id,strategy,strategy_source,rate_multiplier,updated_at
	) VALUES('z-new','7','balanced','global_default','2','now')`); err != nil {
		t.Fatal(err)
	}

	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "groups": []any{json.Number("7")},
	}}, []map[string]any{{
		"id": json.Number("7"), "name": "z-new",
	}}, "tester"); err != nil {
		t.Fatal(err)
	}
	var rate string
	if err := store.db.QueryRowContext(ctx, `SELECT rate_multiplier FROM local_groups WHERE remote_id='7'`).Scan(&rate); err != nil {
		t.Fatal(err)
	}
	if rate != "2" {
		t.Fatalf("stale-name metadata replaced current-name metadata: rate=%q", rate)
	}
}

func TestManagementSnapshotGroupStagingFailureRollsBack(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "management-group-staging-rollback.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{
		{"id": json.Number("11"), "name": "first", "groups": []any{json.Number("7")}},
		{"id": json.Number("12"), "name": "second", "groups": []any{json.Number("8")}},
	}, []map[string]any{
		{"id": json.Number("7"), "name": "alpha"},
		{"id": json.Number("8"), "name": "beta"},
	}, "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,status,metadata_json,updated_at)
		VALUES('11','upstream.example','91','key','alpha','active','{}','now');
		CREATE TRIGGER reject_management_staging BEFORE UPDATE OF local_group ON bindings
		WHEN NEW.local_group GLOB '__sub2api_management_sync_*'
		BEGIN SELECT RAISE(ABORT,'forced staging failure'); END`); err != nil {
		t.Fatal(err)
	}

	_, err = store.SyncManagementSnapshot(ctx, []map[string]any{
		{"id": json.Number("11"), "name": "first", "groups": []any{json.Number("7")}},
		{"id": json.Number("12"), "name": "second", "groups": []any{json.Number("8")}},
	}, []map[string]any{
		{"id": json.Number("7"), "name": "beta"},
		{"id": json.Number("8"), "name": "alpha"},
	}, "tester")
	if err == nil || !strings.Contains(err.Error(), "forced staging failure") {
		t.Fatalf("err=%v", err)
	}
	var groups, memberships, bindings, staged int
	if err := store.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM local_groups WHERE (remote_id='7' AND name='alpha') OR (remote_id='8' AND name='beta')),
		(SELECT COUNT(*) FROM account_groups WHERE (group_id='7' AND group_name='alpha') OR (group_id='8' AND group_name='beta')),
		(SELECT COUNT(*) FROM bindings WHERE local_group='alpha'),
		(SELECT COUNT(*) FROM local_groups WHERE name GLOB '__sub2api_management_sync_*')+
		(SELECT COUNT(*) FROM account_groups WHERE group_name GLOB '__sub2api_management_sync_*')+
		(SELECT COUNT(*) FROM bindings WHERE local_group GLOB '__sub2api_management_sync_*')`).Scan(
		&groups, &memberships, &bindings, &staged,
	); err != nil {
		t.Fatal(err)
	}
	if groups != 2 || memberships != 2 || bindings != 1 || staged != 0 {
		t.Fatalf("failed staging partially committed: groups=%d memberships=%d bindings=%d staged=%d", groups, memberships, bindings, staged)
	}
}

func TestManagementSnapshotGroupMergeBackfillsStableMembershipID(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "management-group-membership-id.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "groups": []any{json.Number("7")},
	}}, []map[string]any{{
		"id": json.Number("7"), "name": "old-name",
	}}, "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('42','legacy','{}','now');
		INSERT INTO local_groups(name,remote_id,updated_at) VALUES('new-name',NULL,'now');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('42','new-name',NULL)`); err != nil {
		t.Fatal(err)
	}

	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "groups": []any{json.Number("7")},
	}}, []map[string]any{{
		"id": json.Number("7"), "name": "new-name",
	}}, "tester"); err != nil {
		t.Fatal(err)
	}
	var groupID sql.NullString
	if err := store.db.QueryRowContext(ctx, `SELECT group_id FROM account_groups WHERE account_id='42' AND group_name='new-name'`).Scan(&groupID); err != nil {
		t.Fatal(err)
	}
	if !groupID.Valid || groupID.String != "7" {
		t.Fatalf("merged name-only membership lacks stable ID: %#v", groupID)
	}
}

func TestManagementSnapshotCountsDeletedStableGroupOnce(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "management-group-deleted-duplicates.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO local_groups(name,remote_id,updated_at) VALUES
		('old-name','13','now'),('duplicate-old-name','13','now')`); err != nil {
		t.Fatal(err)
	}

	result, err := store.SyncManagementSnapshot(ctx, nil, nil, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedGroups != 1 {
		t.Fatalf("duplicate rows inflated deleted stable groups: %#v", result)
	}
	var remaining int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM local_groups WHERE remote_id='13'`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("deleted stable group retained %d duplicate rows", remaining)
	}
}

func TestManagementSnapshotKeepsWholeDeletedIdentityWhileAnyNameIsReferenced(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "management-group-partially-referenced.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('11','alpha','{}','now');
		INSERT INTO local_groups(name,remote_id,updated_at) VALUES('A','13','now'),('B','13','now');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('11','A',NULL)`); err != nil {
		t.Fatal(err)
	}

	result, err := store.SyncManagementSnapshot(ctx, nil, nil, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedGroups != 0 {
		t.Fatalf("partially referenced identity was reported deleted: %#v", result)
	}
	var remaining int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM local_groups WHERE remote_id='13'`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 2 {
		t.Fatalf("partially referenced identity lost rows: %d", remaining)
	}
}

func TestManagementSnapshotRejectsMismatchedMembershipIdentity(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "management-membership-identity.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}

	_, err = store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "groups": []any{
			map[string]any{"id": json.Number("7"), "name": "beta"},
		},
	}}, []map[string]any{
		{"id": json.Number("7"), "name": "alpha"},
		{"id": json.Number("8"), "name": "beta"},
	}, "tester")
	if err == nil || !strings.Contains(err.Error(), "稳定 ID 与名称不一致") {
		t.Fatalf("err=%v", err)
	}
	var accounts, groups int
	if err := store.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM accounts),(SELECT COUNT(*) FROM local_groups)`).Scan(&accounts, &groups); err != nil {
		t.Fatal(err)
	}
	if accounts != 0 || groups != 0 {
		t.Fatalf("invalid membership partially committed: accounts=%d groups=%d", accounts, groups)
	}
}

func TestManagementSnapshotRejectsMembershipStableIDOutsideCatalog(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "management-membership-unknown-id.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}

	_, err = store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "groups": []any{json.Number("9")},
	}}, []map[string]any{{
		"id": json.Number("7"), "name": "alpha",
	}}, "tester")
	if err == nil || !strings.Contains(err.Error(), "目录外稳定 ID") {
		t.Fatalf("err=%v", err)
	}
	var accounts, groups int
	if err := store.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM accounts),(SELECT COUNT(*) FROM local_groups)`).Scan(&accounts, &groups); err != nil {
		t.Fatal(err)
	}
	if accounts != 0 || groups != 0 {
		t.Fatalf("unknown membership ID partially committed: accounts=%d groups=%d", accounts, groups)
	}
}

func TestManagementSnapshotRemovesGroupDeletedFromRemoteCatalog(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "management-group-deleted.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "groups": []any{json.Number("13")},
	}}, []map[string]any{{
		"id": json.Number("13"), "name": "DeepSeek", "platform": "openai",
	}}, "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `
		INSERT INTO routing_decisions(account_id,group_name,updated_at,payload_json) VALUES('11','DeepSeek','now','{}');
		INSERT INTO account_health_evaluations(account_id,group_name,evaluated_at) VALUES('11','DeepSeek','now');
		INSERT INTO health_samples(account_id,group_name,result,source,evidence_key,payload_json)
		VALUES('11','DeepSeek','passed','active-probe','probe-deleted-group','{}');
		INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,status,metadata_json,updated_at)
		VALUES('11','upstream.example','91','key','DeepSeek','active','{}','now')`); err != nil {
		t.Fatal(err)
	}
	control, err := store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	control["group_policy_bindings"] = map[string]any{"13": map[string]any{"strategy": "reliability"}}
	control["scope"].(map[string]any)["managed_group_ids"] = []any{"13", "7"}
	control["scope"].(map[string]any)["excluded_group_ids"] = []any{"13"}
	control["price_management"].(map[string]any)["exchange_group_sets"] = []any{[]any{"13", "7"}}
	control["price_management"].(map[string]any)["exchange_group_set_names"] = []any{"已删除规则"}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.writePolicyDocument(ctx, tx, "control-plane", control, "now"); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	result, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "groups": []any{},
	}}, []map[string]any{}, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedGroups != 1 || result.Groups != 0 {
		t.Fatalf("result=%#v", result)
	}
	var current, history int
	if err := store.db.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM local_groups WHERE remote_id='13')+
		(SELECT COUNT(*) FROM account_groups WHERE group_id='13' OR group_name='DeepSeek')+
		(SELECT COUNT(*) FROM routing_decisions WHERE group_name='DeepSeek')+
		(SELECT COUNT(*) FROM account_health_evaluations WHERE group_name='DeepSeek')`).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM health_samples WHERE group_name='DeepSeek'`).Scan(&history); err != nil {
		t.Fatal(err)
	}
	var historicalGroup, bindingGroup string
	if err := store.db.QueryRowContext(ctx, `SELECT group_name FROM health_samples
		WHERE evidence_key='probe-deleted-group'`).Scan(&historicalGroup); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT local_group FROM bindings WHERE local_account_id='11'`).Scan(&bindingGroup); err != nil {
		t.Fatal(err)
	}
	if current != 0 || history != 0 || historicalGroup != bindingGroup || historicalGroup == "DeepSeek" ||
		!strings.Contains(historicalGroup, "13") {
		t.Fatalf("current=%d history=%d historical_group=%q binding_group=%q", current, history, historicalGroup, bindingGroup)
	}
	control, err = store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	if _, found := control["group_policy_bindings"].(map[string]any)["13"]; found {
		t.Fatalf("deleted group policy remains: %#v", control["group_policy_bindings"])
	}
	if containsPolicyGroupID(control["scope"].(map[string]any)["managed_group_ids"], "13") ||
		containsPolicyGroupID(control["scope"].(map[string]any)["excluded_group_ids"], "13") ||
		containsPolicyGroupID(control["price_management"].(map[string]any)["exchange_group_sets"], "13") {
		t.Fatalf("deleted group remains in policy: %#v", control)
	}
	if names, ok := control["price_management"].(map[string]any)["exchange_group_set_names"].([]any); !ok || len(names) != 0 {
		t.Fatalf("deleted exchange set name remains in policy: %#v", control["price_management"])
	}
}

func containsPolicyGroupID(value any, groupID string) bool {
	switch item := value.(type) {
	case []any:
		for _, child := range item {
			if containsPolicyGroupID(child, groupID) {
				return true
			}
		}
	case string:
		return item == groupID
	}
	return false
}

func TestManagementSnapshotKeepsBaseURLWhenListCredentialsOmitIt(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "management-base-url.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "credentials": map[string]any{"base_url": "https://api.openai.com/v1"},
	}}, nil, "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "credentials": map[string]any{},
	}}, nil, "tester"); err != nil {
		t.Fatal(err)
	}
	var metadataRaw string
	if err := store.db.QueryRow(`SELECT metadata_json FROM accounts WHERE id='11'`).Scan(&metadataRaw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(metadataRaw, `"base_url":"https://api.openai.com/v1"`) {
		t.Fatalf("partial account list erased Base URL: %s", metadataRaw)
	}
	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "base_url": nil,
	}}, nil, "tester"); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT metadata_json FROM accounts WHERE id='11'`).Scan(&metadataRaw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(metadataRaw, "base_url") {
		t.Fatalf("explicit Base URL removal was ignored: %s", metadataRaw)
	}
}

func TestManagementSnapshotCannotOverwriteBoundAccountConvertedCostOrName(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "management-bound-rate.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,multiplier,metadata_json,updated_at)
		VALUES('83','HX｜Relay-0.15','0.15','{}','now');
		INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,local_rate,status,metadata_json,updated_at)
		VALUES('83','api.example','91','relay','codex','0.15','active','{}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("83"), "name": "HX｜Relay-1.5", "rate_multiplier": json.Number("1.5"),
	}}, nil, "tester"); err != nil {
		t.Fatal(err)
	}
	var name, multiplier, localRate string
	if err := store.db.QueryRowContext(ctx, `SELECT a.name,a.multiplier,b.local_rate FROM accounts a
		JOIN bindings b ON b.local_account_id=a.id WHERE a.id='83'`).Scan(&name, &multiplier, &localRate); err != nil {
		t.Fatal(err)
	}
	if name != "HX｜Relay-0.15" || multiplier != "0.15" || localRate != "0.15" {
		t.Fatalf("bound converted rate was overwritten: name=%q multiplier=%q local_rate=%q", name, multiplier, localRate)
	}
}

func TestManagementSnapshotCannotPopulateBoundAccountMissingCostFromRemoteRawRate(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "management-bound-empty-rate.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,multiplier,metadata_json,updated_at)
		VALUES('83','HX｜Relay',NULL,'{}','now');
		INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,local_rate,status,metadata_json,updated_at)
		VALUES('83','api.example','91','relay','codex',NULL,'active','{}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("83"), "name": "HX｜Relay-1.5", "rate_multiplier": json.Number("1.5"),
	}}, nil, "tester"); err != nil {
		t.Fatal(err)
	}
	var name string
	var multiplier, localRate sql.NullString
	if err := store.db.QueryRowContext(ctx, `SELECT a.name,a.multiplier,b.local_rate FROM accounts a
		JOIN bindings b ON b.local_account_id=a.id WHERE a.id='83'`).Scan(&name, &multiplier, &localRate); err != nil {
		t.Fatal(err)
	}
	if name != "HX｜Relay" || multiplier.Valid || localRate.Valid {
		t.Fatalf("remote raw rate leaked into bound account: name=%q multiplier=%#v local_rate=%#v", name, multiplier, localRate)
	}
}

func TestCommitAccountBaseURLObservationsOnlyUpdatesBaseURLMetadata(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "base-url-observations.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO accounts(
		id,name,schedulable,priority,load_factor,concurrency,metadata_json,updated_at
	) VALUES('11','alpha',1,20,'80',40,'{"platform":"openai","base_url":"https://old.example.test"}','before')`); err != nil {
		t.Fatal(err)
	}
	baseURL := " https://relay.example.test/v1 "
	if err := store.CommitAccountBaseURLObservations(ctx, []AccountBaseURLObservation{{
		AccountID: "11", BaseURL: &baseURL, Source: "platform_default",
	}}); err != nil {
		t.Fatal(err)
	}
	var name, loadFactor, metadataRaw string
	var schedulable bool
	var priority, concurrency int
	if err := store.db.QueryRow(`SELECT name,schedulable,priority,load_factor,concurrency,metadata_json
		FROM accounts WHERE id='11'`).Scan(&name, &schedulable, &priority, &loadFactor, &concurrency, &metadataRaw); err != nil {
		t.Fatal(err)
	}
	if name != "alpha" || !schedulable || priority != 20 || loadFactor != "80" || concurrency != 40 {
		t.Fatalf("unrelated account fields changed: name=%q schedulable=%v priority=%d load=%q concurrency=%d", name, schedulable, priority, loadFactor, concurrency)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(metadataRaw), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["platform"] != "openai" || metadata["base_url"] != "https://relay.example.test/v1" ||
		metadata["base_url_source"] != "platform_default" || metadata["base_url_checked_at"] == nil {
		t.Fatalf("metadata=%#v", metadata)
	}
	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "credentials": map[string]any{},
	}}, nil, "tester"); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT metadata_json FROM accounts WHERE id='11'`).Scan(&metadataRaw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(metadataRaw, `"base_url":"https://relay.example.test/v1"`) ||
		!strings.Contains(metadataRaw, `"base_url_source":"platform_default"`) {
		t.Fatalf("partial management sync erased dedicated validation: %s", metadataRaw)
	}
	if err := store.CommitAccountBaseURLObservations(ctx, []AccountBaseURLObservation{{AccountID: "11"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT metadata_json FROM accounts WHERE id='11'`).Scan(&metadataRaw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(metadataRaw, `"base_url":`) || strings.Contains(metadataRaw, `"base_url_source":`) ||
		!strings.Contains(metadataRaw, `"base_url_checked_at":`) || !strings.Contains(metadataRaw, `"platform":"openai"`) {
		t.Fatalf("cleared metadata=%s", metadataRaw)
	}
}

func TestManagementSnapshotKeepsCurrentAccountErrorAndClearsStaleError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "management-error.sqlite3")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha", "error_message": "API returned 503",
	}}, []map[string]any{}, "tester"); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := store.db.QueryRow(`SELECT metadata_json FROM accounts WHERE id='11'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, `"error_message":"API returned 503"`) {
		t.Fatalf("current account error was not retained: %s", raw)
	}

	if _, err := store.SyncManagementSnapshot(ctx, []map[string]any{{
		"id": json.Number("11"), "name": "alpha",
	}}, []map[string]any{}, "tester"); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT metadata_json FROM accounts WHERE id='11'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "error_message") {
		t.Fatalf("stale account error was not cleared: %s", raw)
	}
}

func TestManagementSnapshotRollsBackOnInvalidStableIDOrField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "management-invalid.sqlite3")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = store.SyncManagementSnapshot(ctx, []map[string]any{
		{"id": json.Number("1"), "name": "valid"},
		{"id": "account-name", "name": "invalid"},
	}, []map[string]any{}, "tester")
	if err == nil || !strings.Contains(err.Error(), "稳定 ID") {
		t.Fatalf("err=%v", err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var accounts int
	if err := db.QueryRow(`SELECT COUNT(*) FROM accounts`).Scan(&accounts); err != nil || accounts != 0 {
		t.Fatalf("invalid batch partially committed: accounts=%d err=%v", accounts, err)
	}
	_, err = store.SyncManagementSnapshot(ctx, []map[string]any{
		{"id": json.Number("1"), "name": "valid", "schedulable": "unknown"},
	}, []map[string]any{}, "tester")
	if err == nil || !strings.Contains(err.Error(), "schedulable") {
		t.Fatalf("err=%v", err)
	}
}
