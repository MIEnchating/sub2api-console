package business

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestUpdatePolicyPreservesRoutingDecisionEpochAndRuntimeHistory(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	decisionAt := "2026-08-28T01:00:00Z"
	epochAt := "2026-08-28T00:00:00Z"
	fusedUntil := "2026-08-28T01:03:00Z"
	payload, err := json.Marshal(map[string]any{"fused_until": fusedUntil, "state_since": decisionAt})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO app_state(key,value_json,updated_at)
		VALUES('routing-decision-epoch','{}',?) ON CONFLICT(key) DO UPDATE SET updated_at=excluded.updated_at`, epochAt); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO routing_decisions(
		account_id,group_name,schedulable,role,routing_state,updated_at,payload_json
	) VALUES('41','codex',0,'fused','fused',?,?)`, decisionAt, string(payload)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO operation_audit(
		source_id,operation_id,operation_type,state,phase,remote_confirmed,readback_confirmed,
		object_type,object_id,group_names_json,field_name,writeback,created_at
	) VALUES(-1,'op-1','routing.writeback','succeeded','readback',1,1,
		'account','41','["codex"]','schedulable',1,?)`, decisionAt); err != nil {
		t.Fatal(err)
	}

	before, err := store.PreviousRoutingDecisions(ctx, nil, nil)
	if err != nil || len(before) != 1 || before[0].Payload["fused_until"] != fusedUntil || before[0].LastApplyAt.IsZero() {
		t.Fatalf("precondition history=%#v err=%v", before, err)
	}
	if _, err := store.UpdatePolicy(ctx, map[string]any{"global_strategy": "speed_first"}, "operator"); err != nil {
		t.Fatal(err)
	}
	after, err := store.PreviousRoutingDecisions(ctx, nil, nil)
	if err != nil || len(after) != 1 || after[0].Payload["fused_until"] != fusedUntil || after[0].LastApplyAt.IsZero() {
		t.Fatalf("policy update discarded cooldown/fuse history: history=%#v err=%v", after, err)
	}
	var epoch string
	if err := store.db.QueryRowContext(ctx, `SELECT updated_at FROM app_state WHERE key='routing-decision-epoch'`).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	if epoch != epochAt {
		t.Fatalf("policy update changed routing decision epoch: got=%s want=%s", epoch, epochAt)
	}
}

func TestUpdatePolicySavesRuntimeModeAndPolicyAtomically(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.SetMode(ctx, "监控模式"); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.UpdatePolicy(ctx, map[string]any{
		"mode": "完全模式", "global_strategy": "speed_first",
	}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Mode != "完全模式" || snapshot.GlobalStrategy == nil || *snapshot.GlobalStrategy != "speed_first" {
		t.Fatalf("combined policy was not returned: %#v", snapshot)
	}
	mode, err := store.Mode(ctx)
	if err != nil || mode != "完全模式" {
		t.Fatalf("runtime mode was not persisted: mode=%q err=%v", mode, err)
	}

	before, err := store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	for _, invalidMode := range []string{"调度模式", "错误模式"} {
		if _, err := store.UpdatePolicy(ctx, map[string]any{
			"mode": invalidMode, "global_strategy": "reliability",
		}, "operator"); err == nil {
			t.Fatalf("invalid runtime mode %q was accepted", invalidMode)
		}
	}
	after, err := store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("policy changed after the combined update was rejected")
	}
}

func TestPolicySnapshotUsesLightweightGroupSummary(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO local_groups(
		name,remote_id,platform,strategy,strategy_source,account_count,updated_at
	) VALUES('codex','6','openai','balanced','global_default',42,'now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `DROP TABLE health_samples`); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.PolicySnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.GroupStrategies) != 1 || snapshot.GroupStrategies[0].AccountCount != 42 ||
		!reflect.DeepEqual(snapshot.GroupStrategies[0].Platforms, []string{"openai"}) {
		t.Fatalf("unexpected lightweight group summary: %#v", snapshot.GroupStrategies)
	}
}

func TestUpdatePolicyPreservesOmittedFieldsAndClearsExplicitLists(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO local_groups(name,remote_id,strategy,strategy_source,updated_at)
		VALUES('codex','6','balanced','global_default','now'),('pro','8','balanced','global_default','now')`); err != nil {
		t.Fatal(err)
	}

	before, err := store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	probeBefore := before["probe"].(map[string]any)["timeout_seconds"]
	snapshot, err := store.UpdatePolicy(ctx, map[string]any{
		"global_strategy":  "price_first",
		"group_strategies": map[string]any{"6": "speed_first"},
	}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.GlobalStrategy == nil || *snapshot.GlobalStrategy != "price_first" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	rows := map[string]PolicyGroupStrategy{}
	for _, row := range snapshot.GroupStrategies {
		if row.ID != nil {
			rows[*row.ID] = row
		}
	}
	if rows["6"].Strategy != "speed_first" || rows["6"].StrategySource != "group_override" {
		t.Fatalf("stable-ID override missing: %#v", rows["6"])
	}
	if rows["8"].Strategy != "price_first" || rows["8"].StrategySource != "global_default" {
		t.Fatalf("global inheritance missing: %#v", rows["8"])
	}
	after, err := store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	if after["probe"].(map[string]any)["timeout_seconds"] != probeBefore {
		t.Fatal("omitted advanced policy field was changed")
	}
	var eventCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_events WHERE event_type='policy.updated'`).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 {
		t.Fatalf("eventCount=%d", eventCount)
	}
}

func TestUpdatePolicyKeepsExplicitManagedGroupSelection(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.UpdatePolicy(ctx, map[string]any{
		"advanced_policy": map[string]any{
			"scope": map[string]any{
				"managed_group_mode": "selected",
				"managed_group_ids":  []any{"6", "8"},
			},
		},
	}, "operator"); err != nil {
		t.Fatal(err)
	}
	document, err := store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	scope := document["scope"].(map[string]any)
	if scope["managed_group_mode"] != "selected" || !reflect.DeepEqual(scope["managed_group_ids"], []any{"6", "8"}) {
		t.Fatalf("explicit managed group selection was overwritten: %#v", scope)
	}
}

func TestPolicyDefaultsToManagingAllAccountsAndPersistsOptOut(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	snapshot, err := store.PolicySnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	scope, ok := snapshot.AdvancedPolicy["scope"].(map[string]any)
	if !ok || scope["manage_all_accounts"] != true {
		t.Fatalf("新策略必须默认托管全部账号：%#v", snapshot.AdvancedPolicy["scope"])
	}

	if _, err := store.UpdatePolicy(ctx, map[string]any{
		"advanced_policy": map[string]any{"scope": map[string]any{"manage_all_accounts": false}},
	}, "operator"); err != nil {
		t.Fatal(err)
	}
	document, err := store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	stored := document["scope"].(map[string]any)
	if stored["manage_all_accounts"] != false {
		t.Fatalf("关闭全部账号托管未持久化：%#v", stored)
	}
}

func TestLegacyPolicySnapshotExposesAndPersistsScalingDefaults(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	document, err := store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	delete(document, "scaling")
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.writePolicyDocument(ctx, tx, "control-plane", document, "2026-08-27T12:00:00Z"); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	snapshot, err := store.PolicySnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	scaling, ok := snapshot.AdvancedPolicy["scaling"].(map[string]any)
	want := map[string]any{
		"enabled": false, "global_max_concurrency": int64(900),
		"min_per_account": int64(3), "max_per_account": int64(250), "scale_up_ratio": 0.8,
		"step_up": int64(5), "step_down": int64(5), "cooldown_seconds": int64(60),
	}
	if !ok || !reflect.DeepEqual(scaling, want) {
		t.Fatalf("legacy scaling defaults are not explicit: got=%#v want=%#v", scaling, want)
	}

	if _, err := store.UpdatePolicy(ctx, map[string]any{
		"advanced_policy": snapshot.AdvancedPolicy,
	}, "operator"); err != nil {
		t.Fatal(err)
	}
	stored, err := store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored["scaling"], want) {
		t.Fatalf("legacy scaling defaults were not persisted: got=%#v want=%#v", stored["scaling"], want)
	}
}

func TestLegacyPolicySnapshotDefaultsWritebackVerificationToOff(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	document, err := store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	delete(document, "writeback")
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.writePolicyDocument(ctx, tx, "control-plane", document, "2026-08-28T00:00:00Z"); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	snapshot, err := store.PolicySnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	writeback, ok := snapshot.AdvancedPolicy["writeback"].(map[string]any)
	if !ok || writeback["concurrency"] != int64(4) || writeback["verification"] != false {
		t.Fatalf("writeback defaults are not explicit: %#v", writeback)
	}
}

func TestUpdatePolicyRejectsInvalidValuesWithoutPartialWrite(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO local_groups(name,remote_id,strategy,strategy_source,updated_at)
		VALUES('codex','6','balanced','global_default','now')`); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		patch map[string]any
		want  string
	}{
		{"unknown field", map[string]any{"mystery": true}, "未知字段"},
		{"null core", map[string]any{"global_strategy": nil}, "不能为 null"},
		{"unknown strategy", map[string]any{"global_strategy": "random"}, "无效"},
		{"retired strategy alias", map[string]any{"global_strategy": "price"}, "无效"},
		{"unstable group id", map[string]any{"group_strategies": map[string]any{"codex": "balanced"}}, "稳定数字 ID"},
		{"unknown group id", map[string]any{"group_strategies": map[string]any{"999": "balanced"}}, "已登记分组"},
		{"invalid interval", map[string]any{"advanced_policy": map[string]any{"upstream_multiplier": map[string]any{"interval_seconds": 1}}}, "不能小于 30"},
		{"core collision", map[string]any{"advanced_policy": map[string]any{"weights": map[string]any{"cooldown_seconds": 10}}}, "不能覆盖基础字段"},
		{"short window exceeds long window", map[string]any{"advanced_policy": map[string]any{"scoring": map[string]any{"short_window": 10, "long_window": 5}}}, "long_window 不能小于"},
		{"http failures exceed window", map[string]any{"advanced_policy": map[string]any{"breaker": map[string]any{"http_window": 5, "http_failures": 6}}}, "http_failures 不能大于"},
		{"latency occurrences exceed window", map[string]any{"advanced_policy": map[string]any{"breaker": map[string]any{"latency_window": 5, "latency_occurrences": 6}}}, "latency_occurrences 不能大于"},
		{"minimum load exceeds maximum", map[string]any{"advanced_policy": map[string]any{"weights": map[string]any{"min_load_factor": 101, "max_load_factor": 100}}}, "min_load_factor 不能大于"},
		{"zero change threshold", map[string]any{"change_threshold": "0"}, "大于 0 且不超过 1"},
		{"zero load floor", map[string]any{"advanced_policy": map[string]any{"weights": map[string]any{"min_load_factor": 0}}}, "不能小于 1"},
		{"zero price exponent", map[string]any{"advanced_policy": map[string]any{"weights": map[string]any{"price_exp": 0}}}, "必须大于 0"},
		{"zero speed exponent", map[string]any{"advanced_policy": map[string]any{"weights": map[string]any{"speed_exp": 0}}}, "必须大于 0"},
		{"zero performance samples", map[string]any{"advanced_policy": map[string]any{"weights": map[string]any{"performance_min_samples": 0}}}, "不能小于 1"},
		{"speed advantage below one", map[string]any{"advanced_policy": map[string]any{"weights": map[string]any{"speed_advantage_cap": 0.5}}}, "必须在 1 到 100 之间"},
		{"zero latest weight", map[string]any{"advanced_policy": map[string]any{"scoring": map[string]any{"latest_weight": 0}}}, "必须大于 0 且不超过 1"},
		{"zero short ratio", map[string]any{"advanced_policy": map[string]any{"scoring": map[string]any{"short_ratio": 0}}}, "必须大于 0 且不超过 1"},
		{"zero slow ttfb", map[string]any{"advanced_policy": map[string]any{"scoring": map[string]any{"slow_ttfb_ms": 0}}}, "不能小于 1"},
		{"zero breaker switch limit", map[string]any{"advanced_policy": map[string]any{"breaker": map[string]any{"max_switch_per_round": 0}}}, "不能小于 1"},
		{"zero breaker latency", map[string]any{"advanced_policy": map[string]any{"breaker": map[string]any{"latency_ttfb_ms": 0}}}, "不能小于 1"},
		{"zero transient streak", map[string]any{"advanced_policy": map[string]any{"breaker": map[string]any{"transient_consecutive_failures": 0}}}, "不能小于 1"},
		{"zero degrade ratio", map[string]any{"advanced_policy": map[string]any{"degrade": map[string]any{"load_factor_ratio": 0}}}, "必须大于 0 且不超过 1"},
		{"zero degrade load floor", map[string]any{"advanced_policy": map[string]any{"degrade": map[string]any{"min_load_factor": 0}}}, "不能小于 1"},
		{"zero probe timeout", map[string]any{"advanced_policy": map[string]any{"probe": map[string]any{"timeout_seconds": 0}}}, "不能小于 1"},
		{"invalid probe retry source", map[string]any{"advanced_policy": map[string]any{"probe": map[string]any{"retry_source": "other"}}}, "选项无效"},
		{"probe retry count exceeds limit", map[string]any{"advanced_policy": map[string]any{"probe": map[string]any{"retry_count": 11}}}, "不能大于 10"},
		{"invalid probe retry status", map[string]any{"advanced_policy": map[string]any{"probe": map[string]any{"retry_status_codes": []any{99}}}}, "不能小于 100"},
		{"invalid client error status", map[string]any{"advanced_policy": map[string]any{"classify": map[string]any{"client_error_status_codes": []any{399}}}}, "不能小于 400"},
		{"invalid price margin", map[string]any{"advanced_policy": map[string]any{"price_management": map[string]any{"profit_margin": 1}}}, "必须在 0 到 0.99 之间"},
		{"invalid price interval", map[string]any{"advanced_policy": map[string]any{"price_management": map[string]any{"interval_seconds": 29}}}, "不能小于 30"},
		{"invalid price concurrency", map[string]any{"advanced_policy": map[string]any{"price_management": map[string]any{"write_concurrency": 17}}}, "不能大于 16"},
		{"zero traffic freshness", map[string]any{"advanced_policy": map[string]any{"probe": map[string]any{"traffic_fresh_seconds": 0}}}, "不能小于 1"},
		{"minimum concurrency exceeds maximum", map[string]any{"advanced_policy": map[string]any{"scaling": map[string]any{"min_per_account": 251, "max_per_account": 250}}}, "min_per_account 不能大于"},
		{"zero scaling minimum", map[string]any{"advanced_policy": map[string]any{"scaling": map[string]any{"min_per_account": 0}}}, "不能小于 1"},
		{"zero scaling ratio", map[string]any{"advanced_policy": map[string]any{"scaling": map[string]any{"scale_up_ratio": 0}}}, "必须大于 0 且不超过 1"},
		{"cleanup occurrences exceed window", map[string]any{"advanced_policy": map[string]any{"cleanup": map[string]any{"occurrences": 6, "window": 5}}}, "cleanup.occurrences 不能大于"},
		{"quota score cannot be fatal", map[string]any{"advanced_policy": map[string]any{"scoring": map[string]any{"event_scores": map[string]any{"quota_exhausted": 0}}}}, "不能小于 1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := store.UpdatePolicy(ctx, test.patch, "operator"); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v want substring %q", err, test.want)
			}
		})
	}
	snapshot, err := store.PolicySnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.GlobalStrategy == nil || *snapshot.GlobalStrategy != "balanced" {
		t.Fatalf("invalid update changed policy: %#v", snapshot.GlobalStrategy)
	}
	var eventCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_events WHERE event_type='policy.updated'`).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 0 {
		t.Fatalf("invalid updates wrote %d events", eventCount)
	}
}

func TestUpdatePolicyAdvancedProjectionPreservesOmittedSectionsAndClearsExplicitSection(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	before, err := store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	breakerBefore := copyObject(before["breaker"].(map[string]any))
	_, err = store.UpdatePolicy(ctx, map[string]any{
		"advanced_policy": map[string]any{
			"probe": map[string]any{"enabled": false, "timeout_seconds": 15, "concurrency": 2, "prompt": "ping", "skip_when_traffic_fresh": false, "traffic_fresh_seconds": 90},
		},
	}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	document, err := store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	probe := document["probe"].(map[string]any)
	if probe["interval_seconds"] != int64(300) || probe["enabled"] != false || probe["timeout_seconds"] != int64(15) {
		t.Fatalf("unexpected probe replacement: %#v", probe)
	}
	if !reflect.DeepEqual(document["breaker"], breakerBefore) {
		t.Fatalf("omitted advanced section changed: before=%#v after=%#v", breakerBefore, document["breaker"])
	}
	if _, err := store.UpdatePolicy(ctx, map[string]any{
		"advanced_policy": map[string]any{"breaker": map[string]any{}},
	}, "operator"); err != nil {
		t.Fatal(err)
	}
	document, err = store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	if len(document["breaker"].(map[string]any)) != 0 {
		t.Fatalf("explicit empty advanced section was not cleared: %#v", document["breaker"])
	}
}

func TestUpdatePolicyAcceptsGuardianDecimalScores(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.UpdatePolicy(ctx, map[string]any{
		"advanced_policy": map[string]any{
			"weights": map[string]any{"gate_floor": 40.5},
			"scoring": map[string]any{"event_scores": map[string]any{
				"perfect": 99.5, "slow_ttfb": 64.5, "upstream_unknown": 39.5,
				"gateway_error": 24.5, "quota_exhausted": 14.5, "probe_fail": 9.5, "fatal": 0.0,
			}},
		},
	}, "operator"); err != nil {
		t.Fatal(err)
	}
	document, err := store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	if document["weights"].(map[string]any)["gate_floor"] != 40.5 {
		t.Fatalf("小数健康闸门没有保存：%#v", document["weights"])
	}
	scores := document["scoring"].(map[string]any)["event_scores"].(map[string]any)
	if scores["perfect"] != 99.5 || scores["quota_exhausted"] != 14.5 {
		t.Fatalf("小数事件分值没有保存：%#v", scores)
	}
}

func TestUpdatePolicyRestoresGuardianGatewayCodesWhenExplicitlyEmpty(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.UpdatePolicy(ctx, map[string]any{
		"advanced_policy": map[string]any{"classify": map[string]any{"gateway_status_codes": []any{}}},
	}, "operator"); err != nil {
		t.Fatal(err)
	}
	document, err := store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	codes := document["classify"].(map[string]any)["gateway_status_codes"]
	want := []any{int64(429), int64(500), int64(502), int64(503), int64(504)}
	if !reflect.DeepEqual(codes, want) {
		t.Fatalf("empty gateway codes did not normalize to Guardian defaults: %#v", codes)
	}
}

func TestUpdatePolicyDeduplicatesStatusCodesWithoutDuplicatingUniqueValues(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.UpdatePolicy(ctx, map[string]any{
		"advanced_policy": map[string]any{
			"breaker":  map[string]any{"instant_status_codes": []any{int64(401), int64(403), int64(401)}},
			"cleanup":  map[string]any{"trigger_status_codes": []any{int64(401), int64(403), int64(401)}},
			"classify": map[string]any{"gateway_status_codes": []any{int64(429), int64(500), int64(429)}},
		},
	}, "operator"); err != nil {
		t.Fatal(err)
	}
	document, err := store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	assertCodes := func(section, field string, want []any) {
		t.Helper()
		got := document[section].(map[string]any)[field]
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s.%s=%#v want %#v", section, field, got, want)
		}
	}
	assertCodes("breaker", "instant_status_codes", []any{int64(401), int64(403)})
	assertCodes("cleanup", "trigger_status_codes", []any{int64(401), int64(403)})
	assertCodes("classify", "gateway_status_codes", []any{int64(429), int64(500)})
}

func TestUpdatePolicyAutoApplyMergesPartialPatchAndRejectsUnknownField(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.UpdatePolicy(ctx, map[string]any{
		"auto_apply": map[string]any{"priority": false},
	}, "operator"); err != nil {
		t.Fatal(err)
	}
	document, err := store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	autoApply := document["auto_apply"].(map[string]any)
	if autoApply["priority"] != false || autoApply["schedulable"] != true || autoApply["load_factor"] != true || autoApply["concurrency"] != false {
		t.Fatalf("partial auto_apply patch replaced omitted fields: %#v", autoApply)
	}
	if _, err := store.UpdatePolicy(ctx, map[string]any{
		"auto_apply": map[string]any{"unknown": true},
	}, "operator"); err == nil || !strings.Contains(err.Error(), "未定义") {
		t.Fatalf("unknown auto_apply field was accepted: %v", err)
	}
}

func TestGroupPolicyAndExclusionMutationsUseStableGroupID(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO local_groups(name,remote_id,strategy,strategy_source,updated_at)
		VALUES('codex','6','balanced','global_default','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO routing_decisions(account_id,group_name,updated_at,payload_json)
		VALUES('41','codex','2026-08-27T01:00:00Z','{"weight":400}')`); err != nil {
		t.Fatal(err)
	}
	row, err := store.UpdateGroupPolicy(ctx, "6", map[string]any{
		"enabled": true, "strategy": "reliability", "min_pool_size": 2, "weight_budget": 500,
		"balanced_price_ratio": 0.4, "breaker_enabled": false, "recovery_enabled": true,
		"weights_enabled": true, "scaling_enabled": false, "probe_enabled": true,
		"probe_interval_seconds": 600, "probe_model": " gpt-5.1-codex ",
	}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if row.Strategy != "reliability" || row.StrategySource != "group_override" || row.Override == nil || row.Override.WeightBudget == nil || *row.Override.WeightBudget != 500 {
		t.Fatalf("unexpected group override: %#v", row)
	}
	var retainedDecisions int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM routing_decisions WHERE group_name='codex'`).Scan(&retainedDecisions); err != nil || retainedDecisions != 1 {
		t.Fatalf("group policy update discarded runtime continuity: count=%d err=%v", retainedDecisions, err)
	}
	cleared, err := store.ClearGroupPolicy(ctx, "6", "operator")
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Strategy != "balanced" || cleared.StrategySource != "global_default" || cleared.Override != nil {
		t.Fatalf("group policy did not fall back to the global strategy: %#v", cleared)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM routing_decisions WHERE group_name='codex'`).Scan(&retainedDecisions); err != nil || retainedDecisions != 1 {
		t.Fatalf("group policy clear discarded runtime continuity: count=%d err=%v", retainedDecisions, err)
	}
	excluded, err := store.SetGroupExcluded(ctx, "6", true, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if excluded.Status != "excluded" || excluded.ParticipationStatus != "out_of_scope" {
		t.Fatalf("group was not excluded: %#v", excluded)
	}
	restored, err := store.SetGroupExcluded(ctx, "6", false, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if restored.Status == "excluded" || restored.ParticipationStatus != "participating" {
		t.Fatalf("group was not restored: %#v", restored)
	}
	if _, err := store.UpdateGroupPolicy(ctx, "codex", map[string]any{}, "operator"); err == nil || !strings.Contains(err.Error(), "稳定数字 ID") {
		t.Fatalf("name-based mutation was not rejected: %v", err)
	}
	var eventCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_events WHERE event_type IN ('group.policy.updated','group.policy.cleared','group.scope.updated')`).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 4 {
		t.Fatalf("group mutation events = %d", eventCount)
	}
}

func TestGroupPolicyAndExclusionPreserveFuseAndCooldownHistoryUntilNextRound(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	epochAt := "2026-08-28T00:00:00Z"
	decisionAt := "2026-08-28T01:00:00Z"
	fusedUntil := "2026-08-28T01:03:00Z"
	if _, err := store.db.ExecContext(ctx, `INSERT INTO local_groups(name,remote_id,strategy,strategy_source,updated_at)
		VALUES('codex','6','balanced','global_default','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO app_state(key,value_json,updated_at) VALUES('routing-decision-epoch','{}',?)
		ON CONFLICT(key) DO UPDATE SET updated_at=excluded.updated_at`, epochAt); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{"fused_until": fusedUntil, "state_since": decisionAt})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO routing_decisions(account_id,group_name,routing_state,updated_at,payload_json)
		VALUES('41','codex','fused',?,?)`, decisionAt, string(payload)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO operation_audit(source_id,operation_id,operation_type,state,phase,
		remote_confirmed,readback_confirmed,object_type,object_id,group_names_json,field_name,writeback,created_at)
		VALUES(-1,'op-1','routing.writeback','succeeded','readback',1,1,
		'account','41','["codex"]','load_factor',1,?)`, decisionAt); err != nil {
		t.Fatal(err)
	}

	update := map[string]any{
		"enabled": true, "strategy": "speed_first", "min_pool_size": 1, "weight_budget": 400,
		"balanced_price_ratio": 0.5, "breaker_enabled": true, "recovery_enabled": true,
		"weights_enabled": true, "scaling_enabled": false, "probe_enabled": true,
		"probe_interval_seconds": 300, "probe_model": "gpt-5.1-codex",
	}
	if _, err := store.UpdateGroupPolicy(ctx, "6", update, "operator"); err != nil {
		t.Fatal(err)
	}
	assertGroupRuntimeHistory(t, store, fusedUntil, decisionAt)
	if _, err := store.SetGroupExcluded(ctx, "6", true, "operator"); err != nil {
		t.Fatal(err)
	}
	assertGroupRuntimeHistory(t, store, fusedUntil, decisionAt)
}

func assertGroupRuntimeHistory(t *testing.T, store *Store, fusedUntil, decisionAt string) {
	t.Helper()
	rows, err := store.PreviousRoutingDecisions(context.Background(), nil, nil)
	if err != nil || len(rows) != 1 {
		t.Fatalf("routing history=%#v err=%v", rows, err)
	}
	if rows[0].Payload["fused_until"] != fusedUntil || rows[0].LastWeightWriteAt.IsZero() ||
		rows[0].LastWeightWriteAt.UTC().Format(time.RFC3339) != decisionAt {
		t.Fatalf("fuse/cooldown history was discarded: %#v", rows[0])
	}
}

func openPolicyStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "business.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Bootstrap(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}
