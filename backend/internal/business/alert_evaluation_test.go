package business

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestAlertRuleDisableSuppressesIncidentWithoutFalseRecovery(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at)
		VALUES('api.example','https://api.example','sub2api','认证过期','{}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, "console:auth:api.example", "firing")

	policy := DefaultAlertPolicy()
	policy.AuthEnabled = false
	if _, err := store.UpdateAlertPolicy(ctx, alertPolicyDocument(policy)); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, "console:auth:api.example", "suppressed")
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, "console:auth:api.example", "suppressed")

	if _, err := store.db.ExecContext(ctx, `UPDATE upstreams SET auth_status='已鉴权' WHERE host='api.example'`); err != nil {
		t.Fatal(err)
	}
	policy.AuthEnabled = true
	if _, err := store.UpdateAlertPolicy(ctx, alertPolicyDocument(policy)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, "console:auth:api.example", "closed")

	if _, err := store.db.ExecContext(ctx, `UPDATE upstreams SET auth_status='认证过期' WHERE host='api.example'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, "console:auth:api.example", "firing")
}

func TestRoutingRuleDisableSuppressesIncidentWithoutFalseRecovery(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','primary','{}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO routing_decisions(account_id,group_name,schedulable,role,routing_state,reason,updated_at,payload_json)
		VALUES('41','codex',0,'fused','fused','连续网关错误','2026-08-27T01:00:00Z','{}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	const incidentKey = "console:routing:breaker:41:codex"
	assertAlertStatus(t, store, incidentKey, "firing")

	policy := DefaultAlertPolicy()
	policy.RoutingBreakerEnabled = false
	if _, err := store.UpdateAlertPolicy(ctx, alertPolicyDocument(policy)); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, incidentKey, "suppressed")
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, incidentKey, "suppressed")

	if _, err := store.db.ExecContext(ctx, `UPDATE routing_decisions SET schedulable=1,role='healthy',routing_state='healthy' WHERE account_id='41'`); err != nil {
		t.Fatal(err)
	}
	policy.RoutingBreakerEnabled = true
	if _, err := store.UpdateAlertPolicy(ctx, alertPolicyDocument(policy)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, incidentKey, "closed")
}

func TestAlertMasterSwitchSuppressesFiringIncidents(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at)
		VALUES('api.example','https://api.example','sub2api','认证过期','{}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	policy := DefaultAlertPolicy()
	policy.Enabled = false
	if _, err := store.UpdateAlertPolicy(ctx, alertPolicyDocument(policy)); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, "console:auth:api.example", "suppressed")
	result, err := store.EvaluateAlertIncidents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !result.EvaluationDisabled {
		t.Fatalf("disabled policy result=%#v", result)
	}
	assertAlertStatus(t, store, "console:auth:api.example", "suppressed")
}

func TestDisablingAlertsClosesPendingRecoveryNotification(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at)
		VALUES('api.example','https://api.example','sub2api','认证过期','{}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE upstreams SET auth_status='已鉴权' WHERE host='api.example'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	const incidentKey = "console:auth:api.example"
	assertAlertStatus(t, store, incidentKey, "recovered")
	policy := DefaultAlertPolicy()
	policy.Enabled = false
	if _, err := store.UpdateAlertPolicy(ctx, alertPolicyDocument(policy)); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, incidentKey, "closed")
	var deliveryStatus string
	if err := store.db.QueryRow(`SELECT delivery_status FROM alert_incidents WHERE incident_key=?`, incidentKey).Scan(&deliveryStatus); err != nil {
		t.Fatal(err)
	}
	if deliveryStatus != "告警总开关已关闭" {
		t.Fatalf("closed recovery detail=%q", deliveryStatus)
	}
}

func TestBalanceThresholdChangeDoesNotCreateFalseRecovery(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO upstreams(host,base_url,upstream_type,auth_status,balance,metadata_json,updated_at)
		VALUES('api.example','https://api.example','sub2api','已鉴权',9,'{}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, "console:balance:api.example:10", "firing")

	if _, err := store.db.ExecContext(ctx, `UPDATE upstreams SET balance=4 WHERE host='api.example'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, "console:balance:api.example:10", "closed")
	assertAlertStatus(t, store, "console:balance:api.example:5", "firing")

	if _, err := store.db.ExecContext(ctx, `UPDATE upstreams SET balance=30 WHERE host='api.example'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, "console:balance:api.example:5", "recovered")
}

func TestAlertEvaluationStatusDistinguishesExpectedSkipAndNotificationFailure(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	evidence := AlertEvidenceResult{Findings: 1}

	expectedSkip, err := store.RecordAlertEvaluation(ctx, "2026-08-27T00:00:00Z", evidence, AlertDeliveryResult{
		Skipped: 1, Configured: true, MessageIDs: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if expectedSkip.Status != "succeeded" {
		t.Fatalf("repeat-window skip status=%q", expectedSkip.Status)
	}

	missingChannel, err := store.RecordAlertEvaluation(ctx, "2026-08-27T00:01:00Z", evidence, AlertDeliveryResult{
		Skipped: 1, Configured: false, MessageIDs: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if missingChannel.Status != "failed" {
		t.Fatalf("missing-channel status=%q", missingChannel.Status)
	}

	sendFailure, err := store.RecordAlertEvaluation(ctx, "2026-08-27T00:02:00Z", evidence, AlertDeliveryResult{
		Failed: 1, Configured: true, MessageIDs: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if sendFailure.Status != "failed" {
		t.Fatalf("send-failure status=%q", sendFailure.Status)
	}
}

func TestProbeAlertsKeepAccountGroupsIndependent(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	reason := "timeout"
	if _, err := store.PersistProbeSamples(ctx, []ProbeSample{
		{AccountID: "41", GroupName: "codex", Result: "失败", FailureReason: &reason, ObservedAt: now.Format(time.RFC3339Nano)},
		{AccountID: "41", GroupName: "pro", Result: "失败", FailureReason: &reason, ObservedAt: now.Add(time.Second).Format(time.RFC3339Nano)},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, "console:probe:41:codex", "firing")
	assertAlertStatus(t, store, "console:probe:41:pro", "firing")

	if _, err := store.PersistProbeSamples(ctx, []ProbeSample{{
		AccountID: "41", GroupName: "codex", Result: "通过", ObservedAt: now.Add(2 * time.Second).Format(time.RFC3339Nano),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, "console:probe:41:codex", "recovered")
	assertAlertStatus(t, store, "console:probe:41:pro", "firing")
}

func TestProbeAlertsIgnoreUnevaluatedSamples(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	reason := "缺少探测模型"
	if _, err := store.PersistProbeSamples(ctx, []ProbeSample{
		{AccountID: "41", GroupName: "codex", Result: "失败", FailureReason: &reason, ObservedAt: now.Add(-time.Minute).Format(time.RFC3339Nano)},
		{AccountID: "41", GroupName: "codex", Result: "跳过", FailureReason: &reason, ObservedAt: now.Format(time.RFC3339Nano)},
	}); err != nil {
		t.Fatal(err)
	}
	policy := DefaultAlertPolicy()
	policy.ProbeFailureStreak = 2
	if _, err := store.UpdateAlertPolicy(ctx, alertPolicyDocument(policy)); err != nil {
		t.Fatal(err)
	}
	result, err := store.EvaluateAlertIncidents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Findings != 0 {
		t.Fatalf("unevaluated probe sample triggered an alert: %#v", result)
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM alert_incidents WHERE incident_key='console:probe:41:codex' AND status='firing'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("skipped probe was counted as a consecutive failure")
	}
}

func TestProbeAlertsTreatEveryAcceptedSuccessSpellingAsSuccess(t *testing.T) {
	for _, result := range []string{"pass", "healthy", "ok"} {
		t.Run(result, func(t *testing.T) {
			store := openPolicyStore(t)
			ctx := context.Background()
			if _, err := store.PersistProbeSamples(ctx, []ProbeSample{{
				AccountID: "41", GroupName: "codex", Result: result,
				ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
			}}); err != nil {
				t.Fatal(err)
			}
			evaluation, err := store.EvaluateAlertIncidents(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if evaluation.Findings != 0 {
				t.Fatalf("accepted success result %q triggered an alert", result)
			}
		})
	}
}

func TestExpiredProbeEvidenceClosesWithoutFalseRecovery(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	reason := "timeout"
	if _, err := store.PersistProbeSamples(ctx, []ProbeSample{{
		AccountID: "41", GroupName: "codex", Result: "失败", FailureReason: &reason,
		ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	const incidentKey = "console:probe:41:codex"
	assertAlertStatus(t, store, incidentKey, "firing")
	old := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339Nano)
	if _, err := store.db.ExecContext(ctx, `UPDATE health_samples SET observed_at=? WHERE account_id='41' AND group_name='codex'`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, incidentKey, "closed")
	var deliveryStatus string
	if err := store.db.QueryRow(`SELECT delivery_status FROM alert_incidents WHERE incident_key=?`, incidentKey).Scan(&deliveryStatus); err != nil {
		t.Fatal(err)
	}
	if deliveryStatus != "主动探测证据不足或已过期" {
		t.Fatalf("expired evidence detail=%q", deliveryStatus)
	}
}

func TestRemovingProbeGroupFromScopeClosesWithoutFalseRecovery(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	reason := "timeout"
	if _, err := store.PersistProbeSamples(ctx, []ProbeSample{{
		AccountID: "41", GroupName: "codex", Result: "失败", FailureReason: &reason,
		ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	const incidentKey = "console:probe:41:codex"
	assertAlertStatus(t, store, incidentKey, "firing")
	policy := DefaultAlertPolicy()
	policy.ProbeGroups = []string{"other-group"}
	if _, err := store.UpdateAlertPolicy(ctx, alertPolicyDocument(policy)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, incidentKey, "closed")
	var deliveryStatus string
	if err := store.db.QueryRow(`SELECT delivery_status FROM alert_incidents WHERE incident_key=?`, incidentKey).Scan(&deliveryStatus); err != nil {
		t.Fatal(err)
	}
	if deliveryStatus != "主动探测分组已移出告警范围" {
		t.Fatalf("scope-removal detail=%q", deliveryStatus)
	}
}

func TestRemovedProbeEvidenceClosesWithoutFalseRecovery(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	reason := "timeout"
	if _, err := store.PersistProbeSamples(ctx, []ProbeSample{{
		AccountID: "41", GroupName: "codex", Result: "失败", FailureReason: &reason,
		ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	const incidentKey = "console:probe:41:codex"
	if _, err := store.db.ExecContext(ctx, `DELETE FROM health_samples WHERE account_id='41' AND group_name='codex'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, incidentKey, "closed")
	var deliveryStatus string
	if err := store.db.QueryRow(`SELECT delivery_status FROM alert_incidents WHERE incident_key=?`, incidentKey).Scan(&deliveryStatus); err != nil {
		t.Fatal(err)
	}
	if deliveryStatus != "主动探测证据不足或已过期" {
		t.Fatalf("removed-evidence detail=%q", deliveryStatus)
	}
}

func TestProbeEvidenceMaxAgeIncludesGroupProbeInterval(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO local_groups(name,remote_id,strategy,strategy_source,updated_at)
		VALUES('slow-group','6','balanced','global_default','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateGroupPolicy(ctx, "6", map[string]any{
		"enabled": true, "strategy": "balanced", "min_pool_size": 1, "weight_budget": 100,
		"balanced_price_ratio": 0.5, "breaker_enabled": true, "recovery_enabled": true,
		"weights_enabled": true, "scaling_enabled": false, "probe_enabled": true,
		"probe_interval_seconds": 1200, "probe_model": "gpt-5.1-codex",
	}, "operator"); err != nil {
		t.Fatal(err)
	}

	maxAge, err := store.probeAlertEvidenceMaxAge(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if maxAge != 40*time.Minute {
		t.Fatalf("probe evidence max age=%s want=40m", maxAge)
	}
}

func TestRateSyncAlertKeepsTheRedactedFailureReason(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at)
		VALUES('api.example','https://api.example','sub2api','已鉴权','{"rate_sync_status":"failed","rate_sync_error":"上游分组 auto 倍率不是有限数值"}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	var cause string
	if err := store.db.QueryRow(`SELECT cause_code FROM alert_incidents WHERE incident_key='console:rate-sync:api.example'`).Scan(&cause); err != nil {
		t.Fatal(err)
	}
	if cause != "RATE_SYNC:上游分组 auto 倍率不是有限数值" {
		t.Fatalf("rate-sync cause=%q", cause)
	}
}

func TestConfigurationAlertChecksInvalidBalanceWhenBalanceRuleIsDisabled(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO upstreams(host,base_url,upstream_type,auth_status,mapped_balance,metadata_json,updated_at)
		VALUES('api.example','https://api.example','sub2api','已鉴权','not-a-number','{}','now')`); err != nil {
		t.Fatal(err)
	}
	policy := DefaultAlertPolicy()
	policy.BalanceEnabled = false
	if _, err := store.UpdateAlertPolicy(ctx, alertPolicyDocument(policy)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, "console:configuration:upstream-balance:api.example", "firing")
	var cause string
	if err := store.db.QueryRow(`SELECT cause_code FROM alert_incidents WHERE incident_key='console:configuration:upstream-balance:api.example'`).Scan(&cause); err != nil {
		t.Fatal(err)
	}
	if cause != "CONFIG_BALANCE_INVALID:not-a-number" {
		t.Fatalf("invalid balance cause=%q", cause)
	}
}

func TestRoutingAlertsTrackDecisionTransitionsAndApplyRecovery(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,schedulable,routing_state,metadata_json,updated_at)
		VALUES('41','primary',0,'fused','{}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','codex','7')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO routing_decisions(account_id,group_name,schedulable,role,routing_state,reason,updated_at,payload_json)
		VALUES('41','codex',0,'fused','fused','连续网关错误','2026-08-27T01:00:00Z','{}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO operation_audit(source_id,operation_id,operation_type,state,phase,actor,source,error,
		remote_confirmed,readback_confirmed,object_type,object_id,group_names_json,writeback,created_at)
		VALUES(-1,'failed-1','routing.writeback','failed','remote-write','scheduler','console','network timeout',0,0,'account','41','["codex"]',1,'2026-08-27T01:00:01Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, "console:routing:breaker:41:codex", "firing")
	assertAlertStatus(t, store, "console:routing:group-unavailable:codex", "firing")
	assertAlertStatus(t, store, "console:routing:apply:41", "firing")

	if _, err := store.db.ExecContext(ctx, `UPDATE routing_decisions SET schedulable=1,role='survivor',routing_state='survivor',reason='保底强留',updated_at='2026-08-27T01:01:00Z'
		WHERE account_id='41' AND group_name='codex'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE accounts SET schedulable=1,routing_state='survivor' WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO operation_audit(source_id,operation_id,operation_type,state,phase,actor,source,
		remote_confirmed,readback_confirmed,object_type,object_id,group_names_json,writeback,created_at)
		VALUES(-2,'success-1','routing.writeback','succeeded','readback','scheduler','console',1,1,'account','41','["codex"]',1,'2026-08-27T01:01:01Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, "console:routing:breaker:41:codex", "closed")
	assertAlertStatus(t, store, "console:routing:group-unavailable:codex", "closed")
	assertAlertStatus(t, store, "console:routing:apply:41", "recovered")
	assertAlertStatus(t, store, "console:routing:survivor:41:codex", "firing")
	assertAlertStatus(t, store, "console:routing:group-survivor:codex", "firing")
	for _, incidentKey := range []string{"console:routing:breaker:41:codex", "console:routing:group-unavailable:codex"} {
		var deliveryStatus string
		if err := store.db.QueryRow(`SELECT delivery_status FROM alert_incidents WHERE incident_key=?`, incidentKey).Scan(&deliveryStatus); err != nil {
			t.Fatal(err)
		}
		if deliveryStatus != "同一调度对象的其他异常仍存在" {
			t.Fatalf("routing transition detail=%q", deliveryStatus)
		}
	}

	if _, err := store.db.ExecContext(ctx, `UPDATE routing_decisions SET role='healthy',routing_state='healthy',reason='已恢复',updated_at='2026-08-27T01:02:00Z'
		WHERE account_id='41' AND group_name='codex'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, "console:routing:survivor:41:codex", "recovered")
	assertAlertStatus(t, store, "console:routing:group-survivor:codex", "recovered")
}

func TestBindingInvalidAlertNamesCauseAndRecoversWithStableIncident(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,schedulable,routing_state,metadata_json,updated_at)
		VALUES('41','primary',0,'binding_invalid','{}','now');
		INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','codex','7');
		INSERT INTO routing_decisions(account_id,group_name,schedulable,role,routing_state,reason,updated_at,payload_json)
		VALUES('41','codex',0,'excluded','binding_invalid','上游 Key key-1 已确认删除（连续 2 次完整同步未返回）','2026-08-27T01:00:00Z','{}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	const incidentKey = "console:routing:binding-invalid:41:codex"
	assertAlertStatus(t, store, incidentKey, "firing")
	assertAlertStatus(t, store, "console:routing:group-unavailable:codex", "firing")
	var eventType, cause string
	if err := store.db.QueryRow(`SELECT event_type,cause_code FROM alert_incidents WHERE incident_key=?`, incidentKey).Scan(&eventType, &cause); err != nil {
		t.Fatal(err)
	}
	if eventType != "account.binding_invalid" || !strings.Contains(cause, "Key key-1 已确认删除") {
		t.Fatalf("event=%q cause=%q", eventType, cause)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE routing_decisions SET schedulable=1,role='healthy',routing_state='healthy',
		reason='稳定绑定已恢复',updated_at='2026-08-27T01:01:00Z' WHERE account_id='41'`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, incidentKey, "recovered")
}

func TestModeSwitchClosesStaleRoutingAlertWithoutFalseRecovery(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','primary','{}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO routing_decisions(account_id,group_name,schedulable,role,routing_state,reason,updated_at,payload_json)
		VALUES('41','codex',0,'fused','fused','连续网关错误','2026-08-27T01:00:00Z','{}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	const incidentKey = "console:routing:breaker:41:codex"
	assertAlertStatus(t, store, incidentKey, "firing")
	epoch := time.Now().UTC().Add(time.Second).Format(time.RFC3339Nano)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO app_state(key,value_json,updated_at)
		VALUES('routing-decision-epoch','{}',?)`, epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, incidentKey, "closed")
	var deliveryStatus string
	if err := store.db.QueryRow(`SELECT delivery_status FROM alert_incidents WHERE incident_key=?`, incidentKey).Scan(&deliveryStatus); err != nil {
		t.Fatal(err)
	}
	if deliveryStatus != "运行模式已切换，旧调度判定已失效" {
		t.Fatalf("stale routing alert detail=%q", deliveryStatus)
	}
}

func TestModeSwitchClosesStaleRoutingAlertsWhenSomeNewDecisionsExist(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES
		('41','old-primary','{}','now'),('42','new-primary','{}','now')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO routing_decisions(account_id,group_name,schedulable,role,routing_state,reason,updated_at,payload_json) VALUES
		('41','codex',0,'fused','fused','旧熔断','2026-08-27T01:00:00Z','{}'),
		('42','gemini',0,'fused','fused','旧熔断','2026-08-27T01:00:00Z','{}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	const staleKey = "console:routing:breaker:41:codex"
	assertAlertStatus(t, store, staleKey, "firing")
	epoch := time.Now().UTC().Add(time.Second).Format(time.RFC3339Nano)
	if _, err := store.db.ExecContext(ctx, `INSERT INTO app_state(key,value_json,updated_at)
		VALUES('routing-decision-epoch','{}',?)`, epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE routing_decisions SET schedulable=1,role='healthy',routing_state='healthy',
		reason='新模式已评估',updated_at=? WHERE account_id='42'`, epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := store.EvaluateAlertIncidents(ctx); err != nil {
		t.Fatal(err)
	}
	assertAlertStatus(t, store, staleKey, "closed")
	var deliveryStatus string
	if err := store.db.QueryRow(`SELECT delivery_status FROM alert_incidents WHERE incident_key=?`, staleKey).Scan(&deliveryStatus); err != nil {
		t.Fatal(err)
	}
	if deliveryStatus != "运行模式已切换，旧调度判定已失效" {
		t.Fatalf("stale routing alert detail=%q", deliveryStatus)
	}
}

func TestApplyFailureUsesNewestPositiveSourceIDAtSameTimestamp(t *testing.T) {
	store := openPolicyStore(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `INSERT INTO operation_audit(source_id,operation_id,operation_type,state,phase,actor,source,error,
		remote_confirmed,readback_confirmed,object_type,object_id,group_names_json,writeback,created_at) VALUES
		(1,'older-failure','routing.writeback','failed','remote-write','scheduler','imported','timeout',0,0,'account','41','["codex"]',1,'2026-08-27T01:00:00Z'),
		(2,'newer-success','routing.writeback','succeeded','readback','scheduler','imported',NULL,1,1,'account','41','["codex"]',1,'2026-08-27T01:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	findings, err := store.routingApplyFailureFindings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("同时间戳下较新的成功读回不应留下执行失败告警：%#v", findings)
	}
}

func assertAlertStatus(t *testing.T, store *Store, key, want string) {
	t.Helper()
	var status string
	if err := store.db.QueryRow(`SELECT status FROM alert_incidents WHERE incident_key=?`, key).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != want {
		t.Fatalf("incident %s status=%q want=%q", key, status, want)
	}
}
