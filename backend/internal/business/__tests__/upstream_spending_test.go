package business_test

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func TestUpstreamSpendingAccumulatesOnlyObservedDrops(t *testing.T) {
	store, db := openTrafficRankingStore(t)
	if _, err := db.Exec(`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at) VALUES('balance.example','https://balance.example','sub2api','已鉴权','{}','now');
 INSERT INTO recharge_rates(host,recharge_rate,updated_at) VALUES('balance.example','2','now')`); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct{ balance, want string }{
		{"100.123456789", "0"}, {"90.023456789", "5.05"},
		{"90.023456789", "5.05"}, {"120.023456789", "5.05"},
		{"100.023456789", "15.05"},
	} {
		_, err := store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: "balance.example", Balance: &business.UpstreamBalanceObservation{RawBalance: &step.balance, Status: "已读取", ProfileUserID: "17"}})
		if err != nil {
			t.Fatal(err)
		}
		assertUpstreamSpending(t, store, step.want)
	}
}

func TestUpstreamSpendingPreservesBaselineAcrossMissingBalanceAndFailure(t *testing.T) {
	store, db := openTrafficRankingStore(t)
	if _, err := db.Exec(`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at) VALUES('balance.example','https://balance.example','sub2api','已鉴权','{}','now')`); err != nil {
		t.Fatal(err)
	}
	for _, value := range []business.UpstreamBalanceObservation{
		{RawBalance: spendingPointer("10"), Status: "已读取"},
		{Status: "未返回余额"},
		{RawBalance: spendingPointer("0"), Status: "读取失败"},
		{RawBalance: spendingPointer("7"), Status: "已读取"},
	} {
		if _, err := store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: "balance.example", Balance: &value}); err != nil {
			t.Fatal(err)
		}
	}
	assertUpstreamSpending(t, store, "3")
}

func TestUpstreamSpendingSharesAliasBaselineAndIgnoresIdentityAndUnitChanges(t *testing.T) {
	store, db := openTrafficRankingStore(t)
	if _, err := db.Exec(`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at) VALUES
 ('balance.example','https://balance.example','newapi','已鉴权','{"alias_hosts":["old.example"]}','now'),
 ('old.example','https://old.example','newapi','已鉴权','{}','now')`); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct{ host, balance, user, quota, want string }{
		{"balance.example", "100", "17", "500000", "0"},
		{"old.example", "80", "17", "500000", "20"},
		{"balance.example", "80", "17", "500000", "20"},
		{"balance.example", "5", "18", "500000", "20"},
		{"balance.example", "4", "18", "500000", "21"},
		{"balance.example", "2", "18", "1000000", "21"},
		{"balance.example", "1", "18", "1000000", "22"},
	} {
		if _, err := store.ApplyUpstreamSync(t.Context(), business.UpstreamSyncWrite{Host: step.host, Balance: &business.UpstreamBalanceObservation{RawBalance: &step.balance, Status: "已读取", ProfileUserID: step.user, QuotaPerUnit: &step.quota}}); err != nil {
			t.Fatal(err)
		}
		assertUpstreamSpending(t, store, step.want)
	}
}

func TestUpstreamRequestUsageDeduplicatesRequestsAndCalculatesAccountShare(t *testing.T) {
	store, db := openTrafficRankingStore(t)
	now := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO upstreams(host,base_url,upstream_type,auth_status,metadata_json,updated_at) VALUES
 ('usage.example','https://usage.example','sub2api','已鉴权','{}','now');
 INSERT INTO upstream_identities(upstream_id,created_at,updated_at) VALUES('usage-id','now','now');
 INSERT INTO upstream_identity_hosts(host,upstream_id,is_primary,updated_at) VALUES('usage.example','usage-id',1,'now');
 INSERT INTO accounts(id,name,upstream_host,metadata_json,updated_at) VALUES
 ('41','账号 A','usage.example','{}','now'),('42','账号 B','usage.example','{}','now');
 INSERT INTO bindings(local_account_id,upstream_host,upstream_key_id,upstream_key_name,local_group,updated_at) VALUES
	 ('41','usage.example','key-a','key-a','codex','now'),
	 ('42','usage.example','key-b','key-b','codex','now');
 INSERT INTO binding_identities(binding_id,upstream_id,upstream_key_id,updated_at)
 SELECT id,'usage-id',upstream_key_id,'now' FROM bindings;`); err != nil {
		t.Fatal(err)
	}
	for _, request := range []struct {
		id, account, group, source string
		age                        time.Duration
	}{
		{"a-1", "41", "codex", "traffic", time.Hour},
		{"a-1", "41", "claude", "traffic", time.Hour},
		{"a-2", "41", "codex", "traffic", 2 * time.Hour},
		{"b-1", "42", "codex", "traffic", 3 * time.Hour},
		{"probe", "42", "codex", "active-probe", time.Hour},
		{"old", "42", "codex", "traffic", 31 * 24 * time.Hour},
	} {
		observed := now.Add(-request.age).Format("2006-01-02T15:04:05.000000000Z")
		if _, err := db.Exec(`INSERT INTO usage_records(request_id,account_id,account_name,group_name,observed_at,source,payload_json)
 VALUES(?,?,?,?,?,?, '{}')`, request.id, request.account, request.account, request.group, observed, request.source); err != nil {
			t.Fatal(err)
		}
	}
	result, err := store.Upstreams(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hosts) != 1 {
		t.Fatalf("unexpected upstream list: %#v", result.Hosts)
	}
	host := result.Hosts[0]
	if host.RequestCount != 3 || host.RequestWindowDays != 30 || len(host.AccountRequestUsage) != 2 {
		t.Fatalf("unexpected request summary: %#v", host)
	}
	want := map[string]struct {
		requests int64
		share    float64
	}{"41": {requests: 2, share: 200.0 / 3}, "42": {requests: 1, share: 100.0 / 3}}
	for _, usage := range host.AccountRequestUsage {
		expected, ok := want[usage.AccountID]
		if !ok || usage.RequestCount != expected.requests || math.Abs(usage.Share-expected.share) > 0.001 {
			t.Fatalf("unexpected account usage: %#v, expected %#v", usage, expected)
		}
		delete(want, usage.AccountID)
	}
	if len(want) > 0 {
		t.Fatalf("missing accounts: %#v", want)
	}
}

func assertUpstreamSpending(t *testing.T, store *business.Store, want string) {
	t.Helper()
	result, err := store.Upstreams(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(result.Hosts)
	if err != nil {
		t.Fatal(err)
	}
	var hosts []struct {
		Spending *struct {
			Total     string `json:"total"`
			StartedAt string `json:"started_at"`
		} `json:"spending"`
	}
	if err := json.Unmarshal(raw, &hosts); err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 1 || hosts[0].Spending == nil || hosts[0].Spending.Total != want || hosts[0].Spending.StartedAt == "" {
		t.Fatalf("want cumulative spending %s, got %s", want, raw)
	}
}

func spendingPointer(value string) *string { return &value }
