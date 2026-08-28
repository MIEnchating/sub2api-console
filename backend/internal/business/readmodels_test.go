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
	if _, err := store.db.Exec(`INSERT INTO operation_audit(source_id,operation_type,state,object_id,error,created_at)
		VALUES(-1,'routing.writeback','failed',NULL,'缺少稳定账号 ID','2026-08-26T10:00:00Z')`); err != nil {
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

	upstreams, err := store.Upstreams(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if upstreams.TotalHosts != 1 || upstreams.AuthenticatedHosts != 1 || upstreams.RecoveryRequired != 0 {
		t.Fatalf("unexpected upstream summary: %#v", upstreams)
	}
	host := upstreams.Hosts[0]
	if host.Name != "Example API" || host.RawBalance == nil || *host.RawBalance != "10" || host.Balance == nil || *host.Balance != "5" {
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

func openReadModelFixture(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "business.sqlite3")
	database, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE migration_runs (id INTEGER PRIMARY KEY,status TEXT NOT NULL)`,
		`INSERT INTO migration_runs VALUES(1,'succeeded')`,
		`CREATE TABLE app_state (key TEXT PRIMARY KEY,value_json TEXT NOT NULL,updated_at TEXT NOT NULL)`,
		`INSERT INTO app_state VALUES('config','{"keys":[],"mode":"完全模式"}','now')`,
		`INSERT INTO app_state VALUES('routing-decision-epoch','{}','2026-08-26T09:00:00Z')`,
		`CREATE TABLE policies (key TEXT PRIMARY KEY,value_json TEXT NOT NULL,updated_at TEXT NOT NULL)`,
		`INSERT INTO policies VALUES('control-plane','{"selection":{"strategy":"balanced"},"scope":{"managed_group_mode":"all","managed_group_ids":[],"excluded_group_ids":[]},"group_policy_bindings":{"1":{"strategy":"reliability"}}}','now')`,
		`CREATE TABLE policy_nodes (id INTEGER PRIMARY KEY AUTOINCREMENT,policy_key TEXT NOT NULL,parent_id INTEGER,key_name TEXT,list_index INTEGER,node_type TEXT NOT NULL,scalar_value TEXT,updated_at TEXT NOT NULL)`,
		`CREATE TABLE accounts (
			id TEXT PRIMARY KEY,name TEXT NOT NULL,upstream_host TEXT,upstream_type TEXT,schedulable INTEGER,
			priority INTEGER,load_factor TEXT,concurrency INTEGER,multiplier TEXT,balance TEXT,paused INTEGER,
			paused_reason TEXT,routing_state TEXT,health_status TEXT,failure_streak INTEGER,recovery_pass_streak INTEGER,
			target_priority INTEGER,target_load_factor TEXT,target_schedulable INTEGER,target_concurrency INTEGER,
			metadata_json TEXT NOT NULL,updated_at TEXT NOT NULL
		)`,
		`INSERT INTO accounts VALUES
			('41','example-0.1','api.example','newapi',2,10,'1',5,'0.1','20',0,NULL,'active','healthy',0,1,20,'2',NULL,8,'{"status":"active","platform":"openai","type":"apikey"}','now'),
			('42','example-0.2','api.example','newapi',1,20,'2',5,'0.2',NULL,0,NULL,'active','healthy',0,0,NULL,NULL,NULL,NULL,'{}','now')`,
		`CREATE TABLE account_groups (account_id TEXT NOT NULL,group_name TEXT NOT NULL,group_id TEXT,group_rate TEXT,PRIMARY KEY(account_id,group_name))`,
		`INSERT INTO account_groups VALUES('41','codex','1','0.1'),('42','pro','2','0.2')`,
		`CREATE TABLE routing_decisions (account_id TEXT NOT NULL,group_name TEXT NOT NULL,priority INTEGER,schedulable INTEGER,role TEXT,routing_state TEXT,rank INTEGER,reason TEXT,updated_at TEXT NOT NULL,payload_json TEXT NOT NULL,PRIMARY KEY(account_id,group_name))`,
		`INSERT INTO routing_decisions VALUES('41','codex',1,0,'primary','hard_open',1,'致命错误','2026-08-26T10:00:00Z','{"weight":60}')`,
		`CREATE TABLE account_health_evaluations (account_id TEXT NOT NULL,group_name TEXT NOT NULL,health_score REAL,short_score REAL,long_score REAL,sample_count INTEGER NOT NULL,ttfb_p50_ms REAL,ttfb_p95_ms REAL,latest_event TEXT,evaluated_at TEXT NOT NULL,PRIMARY KEY(account_id,group_name))`,
		`INSERT INTO account_health_evaluations VALUES('41','codex',82.5,80,85,4,80,120,'success','2026-08-26T10:00:00Z')`,
		`CREATE TABLE health_samples (id INTEGER PRIMARY KEY AUTOINCREMENT,account_id TEXT NOT NULL,group_name TEXT NOT NULL,result TEXT,latency_p50 TEXT,latency_p95 TEXT,latency_p99 TEXT,sample_count INTEGER,attempts INTEGER,failure_reason TEXT,observed_at TEXT,source TEXT NOT NULL,evidence_key TEXT,payload_json TEXT NOT NULL)`,
		`INSERT INTO health_samples(account_id,group_name,result,latency_p50,latency_p95,failure_reason,observed_at,source,evidence_key,payload_json) VALUES('41','codex','success','80','120',NULL,'2026-08-26T10:00:00Z','traffic','request-1','{"latency_metric":"first_token","latency_unit":"ms"}')`,
		`CREATE TABLE operation_audit (source_id INTEGER PRIMARY KEY,operation_type TEXT NOT NULL,state TEXT NOT NULL,object_id TEXT,error TEXT,created_at TEXT NOT NULL)`,
		`CREATE TABLE bindings (id INTEGER PRIMARY KEY AUTOINCREMENT,local_account_id TEXT NOT NULL,upstream_host TEXT NOT NULL,upstream_key_id TEXT NOT NULL,upstream_key_name TEXT NOT NULL,upstream_group TEXT,upstream_group_id TEXT,local_group TEXT NOT NULL,local_rate TEXT,upstream_rate TEXT,source_auth_host TEXT,binding_host_alias TEXT,description TEXT,status TEXT,updated_at TEXT NOT NULL)`,
		`INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,upstream_group,upstream_group_id,local_group,local_rate,upstream_rate,description,status,updated_at) VALUES('41','api.example','key-1','codex-key','codex','1','codex','0.1','1','test','active','now')`,
		`CREATE TABLE local_groups (name TEXT PRIMARY KEY,remote_id TEXT,platform TEXT,rate_multiplier TEXT,strategy TEXT,strategy_source TEXT,account_count INTEGER,updated_at TEXT NOT NULL)`,
		`INSERT INTO local_groups VALUES('codex','1','openai',NULL,'balanced','global_default',1,'now'),('pro','2','openai',NULL,'balanced','global_default',1,'now')`,
		`CREATE TABLE upstreams (host TEXT PRIMARY KEY,base_url TEXT NOT NULL,upstream_type TEXT NOT NULL,auth_mode TEXT,enabled INTEGER,auth_status TEXT NOT NULL,balance REAL,raw_balance TEXT,mapped_balance TEXT,checked_at TEXT,account_count INTEGER,metadata_json TEXT NOT NULL,updated_at TEXT NOT NULL)`,
		`INSERT INTO upstreams VALUES('api.example','https://api.example','newapi','newapi_admin',1,'已鉴权',NULL,'10',NULL,'2026-08-26T10:00:00Z',2,'{"site_name":"Example API","auth_verified_at":"2026-08-26T10:00:00Z"}','now')`,
		`CREATE TABLE recharge_rates (host TEXT PRIMARY KEY,recharge_rate TEXT,note TEXT,updated_at TEXT NOT NULL)`,
		`INSERT INTO recharge_rates VALUES('api.example','2',NULL,'now')`,
		`CREATE TABLE upstream_groups (host TEXT NOT NULL,group_id TEXT NOT NULL,name TEXT NOT NULL,description TEXT,platform TEXT,status TEXT,raw_rate TEXT,effective_rate TEXT,rate_source TEXT,updated_at TEXT NOT NULL,PRIMARY KEY(host,group_id))`,
		`INSERT INTO upstream_groups VALUES('api.example','1','codex','Codex','openai','active','1',NULL,'live','now'),('api.example','2','pro','Pro','openai','active','2',NULL,'live','now')`,
		`CREATE TABLE upstream_keys (host TEXT NOT NULL,key_id TEXT NOT NULL,name TEXT NOT NULL,upstream_group TEXT,rate TEXT,status TEXT,metadata_json TEXT NOT NULL,updated_at TEXT NOT NULL,PRIMARY KEY(host,key_id))`,
		`INSERT INTO upstream_keys VALUES('api.example','key-1','codex-key','1','1','active','{}','now'),('api.example','key-2','pro-key','2','2','active','{}','now')`,
		`CREATE TABLE operational_snapshots (namespace TEXT NOT NULL,state_key TEXT NOT NULL,value_json TEXT NOT NULL,observed_at TEXT,updated_at TEXT NOT NULL,origin TEXT NOT NULL,PRIMARY KEY(namespace,state_key))`,
	}
	for _, statement := range statements {
		if _, err := database.Exec(statement); err != nil {
			database.Close()
			t.Fatalf("fixture statement failed: %v\n%s", err, statement)
		}
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}
