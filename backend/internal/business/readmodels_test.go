package business

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAccountProjectionUsesStableIDsAndPreservesInvalidBooleanAsUnknown(t *testing.T) {
	store := openReadModelFixture(t)
	ctx := context.Background()

	accounts, err := store.Accounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 2 {
		t.Fatalf("accounts = %#v", accounts)
	}
	first := accounts[0]
	if first.ID != "41" || len(first.Groups) != 1 || first.Groups[0] != "codex" {
		t.Fatalf("stable account/group projection lost: %#v", first)
	}
	if first.Platform == nil || *first.Platform != "openai" || first.AccountType == nil || *first.AccountType != "apikey" {
		t.Fatalf("account platform/type metadata not projected: %#v", first)
	}
	if first.KeyStatus == nil || *first.KeyStatus != "active" {
		t.Fatalf("upstream Key status not projected: %#v", first.KeyStatus)
	}
	if first.KeyStatusReason == nil || !strings.Contains(*first.KeyStatusReason, "所属分组仍存在") {
		t.Fatalf("upstream Key status reason not projected: %#v", first.KeyStatusReason)
	}
	if first.Sub2APIStatus == nil || *first.Sub2APIStatus != "active" || first.Sub2APIError != nil {
		t.Fatalf("Sub2API account status not projected: %#v", first)
	}
	if first.BaseURL == nil || *first.BaseURL != "https://api.openai.com/v1" || first.UpstreamBaseURL == nil ||
		first.BaseURLCheck != "official_mismatch" || first.BaseURLCheckReason == nil {
		t.Fatalf("account Base URL check not projected: %#v", first)
	}
	if first.Schedulable != nil {
		t.Fatalf("invalid persisted boolean must remain unknown: %#v", first.Schedulable)
	}
	if first.Health != "healthy" || first.DesiredHealth == nil || *first.DesiredHealth != "fused" || !first.ApplyPending {
		t.Fatalf("current and desired routing states were conflated: %#v", first)
	}
	if first.HealthScore == nil || *first.HealthScore != 82.5 || first.SampleCount != 4 {
		t.Fatalf("health evaluation not projected: %#v", first)
	}
	if len(first.RecentResults) != 1 || first.RecentResults[0].LatencyMS == nil || *first.RecentResults[0].LatencyMS != 120 {
		t.Fatalf("recent evidence not projected: %#v", first.RecentResults)
	}
}

func TestReadModelsDoNotRequireSQLiteWrites(t *testing.T) {
	store := openReadModelFixture(t)
	ctx := context.Background()
	if err := store.ensureStableUpstreamRelations(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.db.Close(); err != nil {
		t.Fatal(err)
	}
	readOnly, err := sql.Open("sqlite", "file:"+store.path+"?mode=ro&_pragma=query_only%281%29&_pragma=foreign_keys%28ON%29")
	if err != nil {
		t.Fatal(err)
	}
	readOnly.SetMaxOpenConns(8)
	store.db = readOnly
	if err := store.db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	reads := []struct {
		name string
		run  func() error
	}{
		{name: "accounts", run: func() error { _, err := store.Accounts(ctx); return err }},
		{name: "account detail", run: func() error { _, err := store.Account(ctx, "41"); return err }},
		{name: "upstreams", run: func() error { _, err := store.Upstreams(ctx); return err }},
		{name: "upstream groups", run: func() error { _, err := store.UpstreamGroups(ctx, "api.example", true); return err }},
		{name: "routing accounts", run: func() error { _, err := store.RoutingAccounts(ctx, nil, nil); return err }},
		{name: "balance policy", run: func() error { _, err := store.HostBalanceSyncAllowed(ctx, "api.example"); return err }},
		{name: "maintenance accounts", run: func() error { _, err := store.BoundAccountsForMaintenance(ctx, []string{"41"}); return err }},
	}
	for _, read := range reads {
		if err := read.run(); err != nil {
			t.Fatalf("%s attempted a SQLite write: %v", read.name, err)
		}
	}
}

func TestAccountBaseURLCheckAllowsDifferentCustomEndpoints(t *testing.T) {
	accountURL := "http://10.0.0.8:8080/v1"
	upstreamURL := "https://portal.example.test"
	platform := "openai"
	status, reason := accountBaseURLCheck(&accountURL, &upstreamURL, nil, nil, &platform)
	if status != "different_allowed" || reason == nil || !strings.Contains(*reason, "不自动判错") {
		t.Fatalf("status=%q reason=%v", status, reason)
	}
}

func TestAccountBaseURLCheckRecognizesSameHostAcrossSchemes(t *testing.T) {
	accountURL := "http://api.example.test:8080/v1"
	upstreamURL := "https://api.example.test:8080"
	status, _ := accountBaseURLCheck(&accountURL, &upstreamURL, nil, nil, nil)
	if status != "matched" {
		t.Fatalf("status=%q", status)
	}
}

func TestAccountBaseURLCheckDistinguishesUncheckedFromMissingDetailValue(t *testing.T) {
	status, reason := accountBaseURLCheck(nil, nil, nil, nil, nil)
	if status != "unchecked" || reason == nil || !strings.Contains(*reason, "尚未读取") {
		t.Fatalf("unchecked status=%q reason=%v", status, reason)
	}
	checkedAt := "2026-08-30T12:00:00Z"
	status, reason = accountBaseURLCheck(nil, nil, nil, &checkedAt, nil)
	if status != "unknown" || reason == nil || !strings.Contains(*reason, "接口未提供") {
		t.Fatalf("missing status=%q reason=%v", status, reason)
	}
}

func TestAccountBaseURLCheckExplainsPlatformDefaultMismatch(t *testing.T) {
	accountURL := "https://api.openai.com"
	upstreamURL := "https://relay.example.test"
	source := "platform_default"
	status, reason := accountBaseURLCheck(&accountURL, &upstreamURL, nil, nil, &source)
	if status != "official_mismatch" || reason == nil || !strings.Contains(*reason, "未显式配置") {
		t.Fatalf("status=%q reason=%v", status, reason)
	}
}

func TestAccountBaseURLCheckFallsBackToUpstreamHost(t *testing.T) {
	accountURL := "http://172.18.0.1:8317"
	upstreamHost := "172.18.0.1:8317"
	status, reason := accountBaseURLCheck(&accountURL, nil, &upstreamHost, nil, nil)
	if status != "matched" || reason == nil {
		t.Fatalf("status=%q reason=%v", status, reason)
	}
}

func TestAccountProjectionPrefersBindingOwnershipAndMarksStoredHostRepairable(t *testing.T) {
	store := openReadModelFixture(t)
	if _, err := store.db.Exec(`UPDATE accounts SET upstream_host='relay.example' WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	accounts, err := store.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if accounts[0].UpstreamHost == nil || *accounts[0].UpstreamHost != "api.example" ||
		accounts[0].RecordedUpstreamHost == nil || *accounts[0].RecordedUpstreamHost != "relay.example" ||
		!accounts[0].UpstreamHostRepairable {
		t.Fatalf("account=%#v", accounts[0])
	}
}

func TestAccountProjectionReportsDeletedUpstreamKeyGroup(t *testing.T) {
	store := openReadModelFixture(t)
	if _, err := store.Accounts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE upstream_catalog_entities SET lifecycle_state='missing',missing_observations=2,
		confirmed_missing_at='now' WHERE entity_kind='group' AND entity_id='1'`); err != nil {
		t.Fatal(err)
	}
	accounts, err := store.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if accounts[0].KeyStatus == nil || *accounts[0].KeyStatus != "group_missing" ||
		accounts[0].KeyStatusReason == nil || !strings.Contains(*accounts[0].KeyStatusReason, "所属分组 1 已确认删除") {
		t.Fatalf("deleted upstream group not projected: %#v", accounts[0])
	}
}

func TestAccountProjectionUsesLiveKeyGroupWhenBindingGroupIDIsStale(t *testing.T) {
	store := openReadModelFixture(t)
	if _, err := store.db.Exec(`INSERT INTO upstream_groups(host,group_id,name,status,raw_rate,updated_at)
		VALUES('api.example','gptproo','gptproo','active','0.2','now');
		UPDATE upstream_keys SET upstream_group='gptproo' WHERE host='api.example' AND key_id='key-1';
		UPDATE bindings SET upstream_group='gptproo',upstream_group_id='4924'
		WHERE upstream_host='api.example' AND upstream_key_id='key-1'`); err != nil {
		t.Fatal(err)
	}
	accounts, err := store.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if accounts[0].KeyStatus == nil || *accounts[0].KeyStatus != "active" ||
		accounts[0].KeyStatusReason == nil || !strings.Contains(*accounts[0].KeyStatusReason, "所属分组仍存在") ||
		strings.Contains(*accounts[0].KeyStatusReason, "4924") {
		t.Fatalf("live Key group was not preferred over stale binding: %#v", accounts[0])
	}
}

func TestAccountProjectionPreservesSub2APIErrorStatusAndMessage(t *testing.T) {
	store := openReadModelFixture(t)
	if _, err := store.db.Exec(`UPDATE accounts SET schedulable=0,
		metadata_json='{"status":"error","error_message":"Access forbidden (403): quota exceeded"}'
		WHERE id='41'`); err != nil {
		t.Fatal(err)
	}
	accounts, err := store.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if accounts[0].Sub2APIStatus == nil || *accounts[0].Sub2APIStatus != "error" ||
		accounts[0].Sub2APIError == nil || *accounts[0].Sub2APIError != "Access forbidden (403): quota exceeded" {
		t.Fatalf("Sub2API error status was lost: %#v", accounts[0])
	}
}

func TestAccountProjectionExcludesAccountStatePlaceholdersFromRecentResults(t *testing.T) {
	store := openReadModelFixture(t)
	if _, err := store.db.Exec(`INSERT INTO health_samples(
		account_id,group_name,result,latency_p50,latency_p95,failure_reason,observed_at,source,evidence_key,payload_json
	) VALUES('41','codex','未取到日志','15.4','15.4','最近1分钟无账号使用日志，未调用官方测试接口',
		'2026-08-26T11:00:00Z','account-state','state-1','{}')`); err != nil {
		t.Fatal(err)
	}

	accounts, err := store.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts[0].RecentResults) != 1 || accounts[0].RecentResults[0].Source != "traffic" {
		t.Fatalf("account-state placeholder must not be exposed as a recent result: %#v", accounts[0].RecentResults)
	}
	if accounts[0].RecentResults[0].LatencyMS == nil || *accounts[0].RecentResults[0].LatencyMS != 120 {
		t.Fatalf("explicit traffic first-token latency was lost: %#v", accounts[0].RecentResults)
	}
}

func TestRecentFirstTokenLatencyRequiresExplicitFirstTokenSemantics(t *testing.T) {
	latency := sql.NullString{String: "195843", Valid: true}
	tests := []struct {
		name    string
		source  string
		payload string
		want    *float64
	}{
		{name: "active probe first content", source: "active-probe", payload: `{"latency_metric":"first_token","latency_source":"account_test.first_content","latency_unit":"ms"}`, want: float64Pointer(195843)},
		{name: "legacy active probe", source: "active-probe", payload: `{}`, want: float64Pointer(195843)},
		{name: "legacy duration fallback", source: "traffic", payload: `{"latency_metric":"ttfb","latency_source":"operations.duration_ms","latency_unit":"ms"}`},
		{name: "combined traffic sample with separate first token", source: "traffic", payload: `{"latency_metric":"request_duration","latency_source":"operations.duration_ms","latency_unit":"ms","first_token_ms":"1250"}`, want: float64Pointer(1250)},
		{name: "complete probe response", source: "active-probe", payload: `{"latency_metric":"total_duration","latency_source":"account_test.complete_response","latency_unit":"ms"}`},
		{name: "missing metric", source: "traffic", payload: `{"duration_ms":"195843","latency_unit":"ms"}`},
		{name: "explicit first token", source: "traffic", payload: `{"latency_metric":"first_token","latency_source":"operations.first_token_ms","latency_unit":"ms"}`, want: float64Pointer(195843)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := recentFirstTokenLatency(test.source, latency, latency, sql.NullString{String: test.payload, Valid: true})
			if test.want == nil && got != nil {
				t.Fatalf("latency=%v, want nil", *got)
			}
			if test.want != nil && (got == nil || *got != *test.want) {
				t.Fatalf("latency=%v, want %v", got, *test.want)
			}
		})
	}
}

func TestAccountProjectionKeepsTrafficDurationSeparateFromFirstTokenLatency(t *testing.T) {
	store := openReadModelFixture(t)
	if _, err := store.db.Exec(`INSERT INTO health_samples(
		account_id,group_name,result,latency_p50,latency_p95,observed_at,source,evidence_key,payload_json
	) VALUES('41','codex','success','2795','2795','2026-08-29T18:39:18Z','traffic','request-duration',
		'{"duration_ms":"2795","duration_unit":"ms"}')`); err != nil {
		t.Fatal(err)
	}

	accounts, err := store.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	result := accounts[0].RecentResults[0]
	if result.LatencyMS != nil {
		t.Fatalf("traffic request duration was exposed as first-token latency: %#v", result)
	}
	if result.DurationMS == nil || *result.DurationMS != 2795 {
		t.Fatalf("traffic request duration was not projected separately: %#v", result)
	}
}

func float64Pointer(value float64) *float64 {
	return &value
}

func TestAccountProjectionLimitsRecentResultsWithoutChangingScoringCount(t *testing.T) {
	store := openReadModelFixture(t)
	for index := range 12 {
		observedAt := fmt.Sprintf("2026-08-26T10:%02d:00Z", index)
		evidenceKey := fmt.Sprintf("request-extra-%02d", index)
		if _, err := store.db.Exec(`INSERT INTO health_samples(
			account_id,group_name,result,latency_p50,latency_p95,observed_at,source,evidence_key,payload_json
		) VALUES('41','codex','success','80','120',?,'traffic',?,'{"latency_metric":"first_token","latency_unit":"ms"}')`, observedAt, evidenceKey); err != nil {
			t.Fatal(err)
		}
	}

	accounts, err := store.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	first := accounts[0]
	if len(first.RecentResults) != 10 {
		t.Fatalf("recent results=%d, want display limit 10", len(first.RecentResults))
	}
	if first.RecentResults[0].ObservedAt == nil || *first.RecentResults[0].ObservedAt != "2026-08-26T10:11:00Z" {
		t.Fatalf("latest recent result was not kept first: %#v", first.RecentResults[0])
	}
	if first.SampleCount != 4 {
		t.Fatalf("display limit changed persisted scoring count: %d", first.SampleCount)
	}
}

func TestRecentEvidenceSelectionUsesBoundedPerAccountWindows(t *testing.T) {
	store := openReadModelFixture(t)
	for index := range 12 {
		observedAt := fmt.Sprintf("2026-08-30T10:%02d:00Z", index)
		evidenceKey := fmt.Sprintf("account-42-%02d", index)
		if _, err := store.db.Exec(`INSERT INTO health_samples(
			account_id,group_name,result,observed_at,source,evidence_key,payload_json
		) VALUES('42','codex','success',?,'traffic',?,'{}')`, observedAt, evidenceKey); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.Exec(`INSERT INTO health_samples(
		account_id,group_name,result,observed_at,source,evidence_key,payload_json
	) VALUES
		('41','codex','success','2026-08-30T11:03:00Z','traffic','duplicate','{}'),
		('41','pro','success','2026-08-30T11:02:00Z','traffic','duplicate','{}'),
		('41','codex','success','2026-08-30T11:01:00Z','traffic','unique','{}')`); err != nil {
		t.Fatal(err)
	}

	selections, err := store.selectHealthSampleWindowsForAccounts(
		context.Background(),
		[]string{"42", "41"},
		[]string{`LOWER(REPLACE(source,'_','-'))<>'account-state'`},
		nil,
		2,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	samples, err := store.selectedHealthSamples(context.Background(), selections)
	if err != nil {
		t.Fatal(err)
	}
	observed := map[string][]string{}
	for _, selection := range selections {
		sample := samples[selection.id]
		observed[sample.accountID] = append(observed[sample.accountID], sample.observedAt.String)
	}
	if !reflect.DeepEqual(observed["41"], []string{"2026-08-30T11:03:00Z", "2026-08-30T11:01:00Z"}) {
		t.Fatalf("account 41 selection=%#v", observed["41"])
	}
	if !reflect.DeepEqual(observed["42"], []string{"2026-08-30T10:11:00Z", "2026-08-30T10:10:00Z"}) {
		t.Fatalf("account 42 selection=%#v", observed["42"])
	}
}

func TestAccountProjectionIncludesLatestErrorAndUpstreamSchedulingReason(t *testing.T) {
	store := openReadModelFixture(t)
	if _, err := store.db.Exec(`UPDATE accounts SET schedulable=1,metadata_json=? WHERE id='41'`,
		`{"status":"active","platform":"openai","type":"apikey","error_message":"API returned 503","temp_unschedulable_until":"2099-08-27T13:00:00Z","temp_unschedulable_reason":"令牌刷新失败"}`,
	); err != nil {
		t.Fatal(err)
	}

	accounts, err := store.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	first := accounts[0]
	if first.LastError == nil || *first.LastError != "API returned 503" {
		t.Fatalf("latest error not projected: %#v", first.LastError)
	}
	if first.UpstreamBlock == nil || *first.UpstreamBlock != AccountBlockTempUnschedulable {
		t.Fatalf("upstream block not projected: %#v", first.UpstreamBlock)
	}
	if first.UpstreamBlockReason == nil || !strings.Contains(*first.UpstreamBlockReason, "令牌刷新失败") {
		t.Fatalf("upstream block reason not projected: %#v", first.UpstreamBlockReason)
	}
}

func TestAccountProjectionSeparatesEffectiveStateFromPendingParameters(t *testing.T) {
	allApply := routingApplyView{automatic: true, fields: map[string]bool{
		"schedulable": true, "priority": true, "load_factor": true, "concurrency": true,
	}}
	currentSchedulable := true
	targetSchedulable := true
	currentPriority, targetPriority := int64(10), int64(20)
	currentLoad, targetLoad := "10", "2"
	degraded := accountProjection{AccountStatus: AccountStatus{
		RoutingState: stringPointer("healthy"), Schedulable: &currentSchedulable, TargetSchedulable: &targetSchedulable,
		Priority: &currentPriority, TargetPriority: &targetPriority, LoadFactor: &currentLoad, TargetLoadFactor: &targetLoad,
	}, metadataRaw: `{}`}
	applyAccountCalculations(&degraded, []decisionProjection{{state: "degraded"}}, nil, struct {
		message string
		at      *string
	}{}, allApply)
	if degraded.Health != "healthy" || !degraded.ApplyPending || degraded.ApplyError == nil {
		t.Fatalf("尚未执行的降级目标覆盖了当前状态：%#v", degraded.AccountStatus)
	}

	pausedTarget := false
	paused := accountProjection{AccountStatus: AccountStatus{
		RoutingState: stringPointer("healthy"), Schedulable: &currentSchedulable, TargetSchedulable: &pausedTarget,
	}, metadataRaw: `{}`}
	applyAccountCalculations(&paused, []decisionProjection{{state: "paused"}}, nil, struct {
		message string
		at      *string
	}{}, allApply)
	if paused.Health != "healthy" || !paused.ApplyPending || paused.DesiredHealth == nil || *paused.DesiredHealth != "paused" {
		t.Fatalf("尚未摘流量的分组暂停被误报为已生效：%#v", paused.AccountStatus)
	}

	currentFused := false
	confirmed := accountProjection{AccountStatus: AccountStatus{
		RoutingState: stringPointer("healthy"), Schedulable: &currentFused, TargetSchedulable: &pausedTarget,
	}, metadataRaw: `{}`}
	applyAccountCalculations(&confirmed, []decisionProjection{{state: "fused"}}, nil, struct {
		message string
		at      *string
	}{}, allApply)
	if confirmed.Health != "healthy" || !confirmed.ApplyPending {
		t.Fatalf("仅字段值一致但尚未提交生效状态时不应显示为已熔断：%#v", confirmed.AccountStatus)
	}

	shadow := routingApplyView{automatic: true, fields: map[string]bool{}}
	preview := accountProjection{AccountStatus: AccountStatus{
		RoutingState: stringPointer("healthy"), Schedulable: &currentSchedulable, TargetSchedulable: &pausedTarget,
	}, metadataRaw: `{}`}
	applyAccountCalculations(&preview, []decisionProjection{{state: "fused"}}, nil, struct {
		message string
		at      *string
	}{}, shadow)
	if preview.Health != "healthy" || preview.ApplyPending {
		t.Fatalf("影子字段不应被当作已生效或待自动执行：%#v", preview.AccountStatus)
	}
}

func TestAccountProjectionIgnoresUnscopedWritebackAudit(t *testing.T) {
	store := openReadModelFixture(t)
	if _, err := store.db.Exec(`INSERT INTO operation_audit(
		source_id,operation_id,operation_type,state,phase,object_id,error,group_names_json,writeback,created_at)
		VALUES(-1,'unscoped','routing.writeback','failed','writeback',NULL,'缺少稳定账号 ID','[]',1,'2026-08-26T10:00:00Z')`); err != nil {
		t.Fatal(err)
	}

	accounts, err := store.Accounts(context.Background())
	if err != nil {
		t.Fatalf("nullable audit object_id must not break the whole account list: %v", err)
	}
	if len(accounts) != 2 {
		t.Fatalf("accounts = %#v", accounts)
	}
}

func TestAccountProjectionDoesNotExposeDecisionsFromPreviousRuntimeMode(t *testing.T) {
	store := openReadModelFixture(t)
	if _, err := store.db.Exec(`UPDATE app_state SET updated_at='2026-08-26T11:00:00Z'
		WHERE key='routing-decision-epoch'`); err != nil {
		t.Fatal(err)
	}

	accounts, err := store.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	first := accounts[0]
	if first.DesiredHealth != nil || first.DecisionState != nil || first.ApplyPending {
		t.Fatalf("decision from previous runtime mode leaked into current projection: %#v", first)
	}
}

func TestAccountDetailExposesTypedJSONAndBindingFields(t *testing.T) {
	store := openReadModelFixture(t)
	ctx := context.Background()

	detail, err := store.Account(ctx, "41")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Metadata["status"] != "active" || detail.GroupIDs["codex"] == nil || *detail.GroupIDs["codex"] != "1" {
		t.Fatalf("unexpected detail: %#v", detail)
	}
	if len(detail.Bindings) != 1 || detail.Bindings[0].LocalAccountID != "41" || detail.Bindings[0].UpstreamGroupID == nil {
		t.Fatalf("unexpected account bindings: %#v", detail.Bindings)
	}
	if _, err := store.Account(ctx, "account-name"); err == nil {
		t.Fatal("non-stable account identifier must be rejected")
	}
}

func TestUpstreamAndGroupCatalogsUseTypedRatesAndPolicyInheritance(t *testing.T) {
	store := openReadModelFixture(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `UPDATE account_groups SET group_id=NULL WHERE account_id='41'`); err != nil {
		t.Fatal(err)
	}

	upstreams, err := store.Upstreams(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if upstreams.TotalHosts != 1 || upstreams.AuthenticatedHosts != 1 || upstreams.RecoveryRequired != 0 {
		t.Fatalf("unexpected upstream summary: %#v", upstreams)
	}
	host := upstreams.Hosts[0]
	if host.Name != "Example API" || host.RawBalance == nil || *host.RawBalance != "10" || host.Balance == nil || *host.Balance != "5" ||
		host.DisplayBalance == nil || *host.DisplayBalance != "5" || host.BalanceUnit == nil || *host.BalanceUnit != "usd" {
		t.Fatalf("unexpected decimal-safe upstream projection: %#v", host)
	}
	groups, err := store.UpstreamGroups(ctx, "https://API.EXAMPLE/", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[0].EffectiveRate == nil || *groups[0].EffectiveRate != "0.5" {
		t.Fatalf("unexpected upstream groups: %#v", groups)
	}
	if groups[0].Bindable || !groups[0].Bound {
		t.Fatalf("bound group must not be offered again: %#v", groups[0])
	}
	if len(groups[0].BoundAccounts) != 1 || groups[0].BoundAccounts[0].AccountID != "41" ||
		groups[0].BoundAccounts[0].AccountName == nil || *groups[0].BoundAccounts[0].AccountName != "example-0.1" {
		t.Fatalf("bound group must expose its concrete account: %#v", groups[0].BoundAccounts)
	}
	if len(groups[0].BoundAccounts[0].LocalGroups) != 1 || groups[0].BoundAccounts[0].LocalGroups[0].ID != "1" ||
		groups[0].BoundAccounts[0].LocalGroups[0].Name != "codex" {
		t.Fatalf("bound account must expose current local groups: %#v", groups[0].BoundAccounts[0].LocalGroups)
	}
	unbound, err := store.UpstreamGroups(ctx, "api.example", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(unbound) != 1 || !unbound[0].Bindable {
		t.Fatalf("unbound active group with key should be bindable: %#v", unbound)
	}
	localGroups, err := store.Groups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(localGroups) != 2 || localGroups[0].Strategy != "reliability" || localGroups[0].StrategySource != "group_override" {
		t.Fatalf("group policy inheritance not projected: %#v", localGroups)
	}
	if !reflect.DeepEqual(localGroups[0].Platforms, []string{"openai"}) {
		t.Fatalf("group platforms not projected: %#v", localGroups[0].Platforms)
	}
}

func TestUpstreamGroupsRecognizesBindingsThroughExplicitHostAlias(t *testing.T) {
	store := openReadModelFixture(t)
	ctx := context.Background()
	if _, err := store.db.ExecContext(ctx, `UPDATE accounts
		SET name='renamed-account',upstream_host='moved.example',multiplier='9.9' WHERE id='41';
		UPDATE upstreams
		SET metadata_json='{"site_name":"Example API","alias_hosts":["edge.example:8080"]}'
		WHERE host='api.example';
		INSERT INTO upstreams(host,base_url,upstream_type,auth_mode,enabled,auth_status,metadata_json,updated_at)
		VALUES('edge.example:8080','https://edge.example','newapi','newapi_admin',1,'已鉴权','{"site_name":"Example Edge"}','now');
		INSERT INTO recharge_rates(host,recharge_rate,updated_at) VALUES('edge.example:8080','1','now');
		INSERT INTO upstream_groups(host,group_id,name,status,raw_rate,effective_rate,updated_at)
		VALUES('edge.example:8080','1','codex','active','1','1','now');
		INSERT INTO upstream_keys(host,key_id,name,upstream_group,rate,status,updated_at)
		VALUES('edge.example:8080','edge-key','codex-key','1','1','active','now')`); err != nil {
		t.Fatal(err)
	}
	if err := store.ensureStableUpstreamRelations(ctx); err != nil {
		t.Fatal(err)
	}

	groups, err := store.UpstreamGroups(ctx, "edge.example:8080", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || !groups[0].Bound || len(groups[0].BoundAccounts) != 1 ||
		groups[0].BoundAccounts[0].AccountID != "41" || groups[0].BoundAccounts[0].AccountName == nil ||
		*groups[0].BoundAccounts[0].AccountName != "renamed-account" {
		t.Fatalf("explicit Host alias did not preserve the stable binding: %#v", groups)
	}
	summary, err := store.Upstreams(ctx)
	if err != nil {
		t.Fatal(err)
	}
	foundIdentity := false
	for _, upstream := range summary.Hosts {
		if upstream.Host == "api.example" {
			foundIdentity = true
			if upstream.UpstreamID == "" || upstream.AccountCount != 1 || len(upstream.Hosts) != 2 {
				t.Fatalf("explicit Host alias summary is incomplete: %#v", upstream)
			}
		}
	}
	if !foundIdentity {
		t.Fatalf("primary upstream identity missing from summary: %#v", summary.Hosts)
	}
}

func openReadModelFixture(t *testing.T) *Store {
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
	policy, err := store.readPolicyDocument(ctx, store.db, "control-plane")
	if err != nil {
		t.Fatal(err)
	}
	policy["group_policy_bindings"] = map[string]any{"1": map[string]any{"strategy": "reliability"}}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.writePolicyDocument(ctx, tx, "control-plane", policy, "now"); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`INSERT INTO app_state VALUES('routing-decision-epoch','{}','2026-08-26T09:00:00Z')`,
		`INSERT INTO accounts(id,name,upstream_host,upstream_type,schedulable,priority,load_factor,concurrency,multiplier,balance,
			paused,routing_state,health_status,failure_streak,recovery_pass_streak,target_priority,target_load_factor,target_concurrency,
			metadata_json,updated_at) VALUES
			('41','example-0.1','api.example','newapi',2,10,'1',5,'0.1','20',0,'active','healthy',0,1,20,'2',8,'{"status":"active","platform":"openai","type":"apikey","base_url":"https://api.openai.com/v1"}','now'),
			('42','example-0.2','api.example','newapi',1,20,'2',5,'0.2',NULL,0,'active','healthy',0,0,NULL,NULL,NULL,'{}','now')`,
		`INSERT INTO account_groups VALUES('41','codex','1','0.1'),('42','pro','2','0.2')`,
		`INSERT INTO routing_decisions VALUES('41','codex',1,0,'primary','hard_open',1,'致命错误','2026-08-26T10:00:00Z','{"weight":60}')`,
		`INSERT INTO account_health_evaluations VALUES('41','codex',82.5,80,85,4,80,120,'success','2026-08-26T10:00:00Z')`,
		`INSERT INTO health_samples(account_id,group_name,result,latency_p50,latency_p95,failure_reason,observed_at,source,evidence_key,payload_json) VALUES('41','codex','success','80','120',NULL,'2026-08-26T10:00:00Z','traffic','request-1','{"latency_metric":"first_token","latency_unit":"ms"}')`,
		`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group,upstream_group_id,local_group,local_rate,upstream_rate,description,status,updated_at) VALUES('41','api.example','key-1','codex-key','codex','1','codex','0.1','1','test','active','now')`,
		`INSERT INTO local_groups(name,remote_id,platform,rate_multiplier,strategy,strategy_source,account_count,updated_at)
			VALUES('codex','1','openai',NULL,'balanced','global_default',1,'now'),('pro','2','openai',NULL,'balanced','global_default',1,'now')`,
		`INSERT INTO upstreams VALUES('api.example','https://api.example','newapi','newapi_admin',1,'已鉴权',NULL,'10',NULL,'2026-08-26T10:00:00Z',2,'{"site_name":"Example API","auth_verified_at":"2026-08-26T10:00:00Z"}','now')`,
		`INSERT INTO recharge_rates VALUES('api.example','2',NULL,'now')`,
		`INSERT INTO upstream_groups VALUES('api.example','1','codex','Codex','openai','active','1',NULL,'live','now'),('api.example','2','pro','Pro','openai','active','2',NULL,'live','now')`,
		`INSERT INTO upstream_keys VALUES('api.example','key-1','codex-key','1','1','active','{}','now'),('api.example','key-2','pro-key','2','2','active','{}','now')`,
	}
	for _, statement := range statements {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("fixture statement failed: %v\n%s", err, statement)
		}
	}
	if err := store.ensureStableUpstreamRelations(ctx); err != nil {
		t.Fatal(err)
	}
	return store
}
