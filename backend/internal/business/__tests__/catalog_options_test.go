package business_test

import (
	"context"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"testing"
)

func TestSyncTargetsDoNotReadTrafficAndRestrictSingleHost(t *testing.T) {
	store, db := concurrencyStore(t)
	if _, err := store.CreateUpstreamConfiguration(t.Context(), business.UpstreamConfigurationWrite{Host: "second.test", BaseURL: "https://second.test", UpstreamType: "sub2api", AuthMode: "sub2api_user_token", RechargeRate: "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE usage_records`); err != nil {
		t.Fatal(err)
	}
	host := " HTTPS://FIXTURE.EXAMPLE/ "
	result, err := store.UpstreamSyncTargets(t.Context(), &host)
	if err != nil || len(result.Hosts) != 1 || result.Hosts[0].Host != "fixture.example" {
		t.Fatalf("scoped directory: %+v %v", result, err)
	}
	all, err := store.UpstreamSyncTargets(t.Context(), nil)
	if err != nil || len(all.Hosts) != 2 {
		t.Fatalf("all directory: %+v %v", all, err)
	}
	host = "missing.test"
	empty, err := store.UpstreamSyncTargets(t.Context(), &host)
	if err != nil || len(empty.Hosts) != 0 {
		t.Fatalf("missing host expanded scope: %+v %v", empty, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.UpstreamSyncTargets(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestOptionsPreserveIdentityAndMembershipWithoutStatistics(t *testing.T) {
	store, db := concurrencyStore(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','选项账号','{}','now');
 INSERT INTO local_groups(name,remote_id,platform,rate_multiplier,updated_at) VALUES('多平台组','7','composite','0.2','now');
 INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','多平台组','7');
 DROP TABLE usage_records;DROP TABLE health_samples;DROP TABLE account_health_evaluations;DROP TABLE routing_decisions`); err != nil {
		t.Fatal(err)
	}
	accounts, err := store.AccountOptions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 || accounts[0].ID != "41" || len(accounts[0].Groups) != 1 || accounts[0].Groups[0] != "多平台组" {
		t.Fatalf("accounts: %+v", accounts)
	}
	groups, err := store.GroupOptions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].ID == nil || *groups[0].ID != "7" || groups[0].Platform == nil || *groups[0].Platform != "composite" {
		t.Fatalf("groups: %+v", groups)
	}
	upstreams, err := store.UpstreamOptions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(upstreams) != 1 || upstreams[0].Host != "fixture.example" || upstreams[0].UpstreamID == "" {
		t.Fatalf("upstreams: %+v", upstreams)
	}
}

func TestGroupMembershipOptionsPreserveManagedCountsWithoutHistory(t *testing.T) {
	store, db := concurrencyStore(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','成员','{}','now');
 INSERT INTO local_groups(name,remote_id,updated_at) VALUES('已绑定','7','now'),('空分组','8','now');
 INSERT INTO account_groups(account_id,group_name,group_id) VALUES('41','已绑定','7');
 DROP TABLE usage_records; DROP TABLE health_samples; DROP TABLE account_health_evaluations; DROP TABLE routing_decisions`); err != nil {
		t.Fatal(err)
	}
	groups, err := store.GroupMembershipOptions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups: %+v", groups)
	}
	for _, group := range groups {
		expected := int64(0)
		if group.Name == "已绑定" {
			expected = 1
		}
		if group.AccountCount != expected {
			t.Fatalf("count: %+v", group)
		}
	}
}

func TestUpstreamOptionsPreserveLegacyAuthenticationEvidence(t *testing.T) {
	store, db := concurrencyStore(t)
	if _, err := db.Exec(`UPDATE upstreams SET auth_status='已鉴权',balance=10,raw_balance=NULL,checked_at='2026-10-12T00:00:00Z',metadata_json='{}'`); err != nil {
		t.Fatal(err)
	}
	options, err := store.UpstreamOptions(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 1 || options[0].AuthStatus != business.UpstreamAuthStatusAuthenticated {
		t.Fatalf("legacy auth: %+v", options)
	}
}
