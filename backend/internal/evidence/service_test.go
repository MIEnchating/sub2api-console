package evidence

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

type trafficConcurrencyAdmin struct {
	active  atomic.Int32
	maximum atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (a *trafficConcurrencyAdmin) RequestDetails(ctx context.Context, _ string, _, _ int) ([]map[string]any, error) {
	current := a.active.Add(1)
	for {
		previous := a.maximum.Load()
		if current <= previous || a.maximum.CompareAndSwap(previous, current) {
			break
		}
	}
	a.started <- struct{}{}
	select {
	case <-a.release:
	case <-ctx.Done():
	}
	a.active.Add(-1)
	return []map[string]any{}, nil
}

type trafficConcurrencyRepository struct {
	targets []business.EvidenceTarget
	mu      sync.Mutex
	fetched []string
}

func (r *trafficConcurrencyRepository) EvidenceTargets(context.Context, *string, *string) ([]business.EvidenceTarget, error) {
	return r.targets, nil
}

func (r *trafficConcurrencyRepository) PersistTrafficSamples(context.Context, []business.TrafficSample) (int, error) {
	return 0, nil
}

func (r *trafficConcurrencyRepository) PersistTrafficFetches(_ context.Context, ids []string, _ time.Time) error {
	r.mu.Lock()
	r.fetched = append([]string{}, ids...)
	r.mu.Unlock()
	return nil
}

func TestCollectTrafficUsesConfiguredBoundedConcurrency(t *testing.T) {
	repository := &trafficConcurrencyRepository{}
	for _, accountID := range []string{"41", "42", "43", "44"} {
		repository.targets = append(repository.targets, business.EvidenceTarget{AccountID: accountID, GroupName: "codex"})
	}
	admin := &trafficConcurrencyAdmin{started: make(chan struct{}, 4), release: make(chan struct{}, 4)}
	done := make(chan error, 1)
	go func() {
		_, err := New(repository, nil).Collect(context.Background(), map[string]any{
			"traffic":  map[string]any{"enabled": true},
			"probe":    map[string]any{"enabled": false, "concurrency": int64(2)},
			"recovery": map[string]any{"enabled": false},
		}, admin, Options{FetchTraffic: true, Now: time.Now().UTC()})
		done <- err
	}()
	for range 2 {
		<-admin.started
	}
	select {
	case <-admin.started:
		t.Fatal("traffic fetch exceeded configured concurrency")
	case <-time.After(50 * time.Millisecond):
	}
	for range 4 {
		admin.release <- struct{}{}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if admin.maximum.Load() != 2 || len(repository.fetched) != 4 {
		t.Fatalf("maximum=%d fetched=%v", admin.maximum.Load(), repository.fetched)
	}
}

func TestConvertTrafficRowsUsesTotalDurationForCombinedLatency(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	rows := []map[string]any{{
		"account_id": "41", "request_id": "request-1", "kind": "success",
		"created_at": now.Format(time.RFC3339Nano), "duration_ms": json.Number("195843"),
	}}

	samples, malformed := convertTrafficRows(
		"41",
		[]business.EvidenceTarget{{AccountID: "41", GroupName: "codex"}},
		rows,
		now.Add(-time.Minute),
		10,
	)

	if malformed != 0 || len(samples) != 1 {
		t.Fatalf("converted=%#v malformed=%d", samples, malformed)
	}
	if samples[0].LatencyP50 == nil || *samples[0].LatencyP50 != "195843" ||
		samples[0].LatencyP95 == nil || *samples[0].LatencyP95 != "195843" ||
		samples[0].LatencyP99 == nil || *samples[0].LatencyP99 != "195843" {
		t.Fatalf("真实流量总耗时没有进入综合延迟：%#v", samples[0])
	}
	if samples[0].Payload["duration_ms"] != "195843" || samples[0].Payload["duration_unit"] != "ms" {
		t.Fatalf("整体请求耗时没有按原语义保存：%#v", samples[0].Payload)
	}
	if samples[0].Payload["latency_metric"] != "request_duration" {
		t.Fatalf("真实流量综合延迟指标错误：%#v", samples[0].Payload)
	}
	if samples[0].Payload["latency_source"] != "operations.duration_ms" {
		t.Fatalf("真实流量综合延迟来源错误：%#v", samples[0].Payload)
	}
}

func TestConvertTrafficRowsStoresOneSampleForMultiGroupAccount(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	group7, group9 := "7", "9"
	samples, malformed := convertTrafficRows("41", []business.EvidenceTarget{
		{AccountID: "41", GroupName: "pro", GroupID: &group9},
		{AccountID: "41", GroupName: "codex", GroupID: &group7},
	}, []map[string]any{{
		"account_id": "41", "request_id": "request-1", "kind": "success",
		"created_at": now.Format(time.RFC3339Nano), "duration_ms": json.Number("100"),
	}}, now.Add(-time.Minute), 10)
	if malformed != 0 || len(samples) != 1 || samples[0].GroupName != "codex" {
		t.Fatalf("多分组账号必须只保存一份账号级样本并使用主分组标识：samples=%#v malformed=%d", samples, malformed)
	}
}

func TestConvertTrafficRowsKeepsExplicitFirstTokenSeparateFromCombinedLatency(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	rows := []map[string]any{{
		"account_id": "41", "request_id": "request-1", "kind": "success",
		"created_at": now.Format(time.RFC3339Nano), "duration_ms": json.Number("195843"),
		"first_token_ms": json.Number("1250"), "model": "gpt-test",
	}}

	samples, malformed := convertTrafficRows(
		"41",
		[]business.EvidenceTarget{{AccountID: "41", GroupName: "codex"}},
		rows,
		now.Add(-time.Minute),
		10,
	)

	if malformed != 0 || len(samples) != 1 || samples[0].LatencyP95 == nil || *samples[0].LatencyP95 != "195843" {
		t.Fatalf("converted=%#v malformed=%d", samples, malformed)
	}
	if samples[0].Payload["latency_metric"] != "request_duration" || samples[0].Payload["first_token_ms"] != "1250" {
		t.Fatalf("真实流量总耗时和首字没有分开保存：%#v", samples[0].Payload)
	}
	if samples[0].Payload["model"] != "gpt-test" {
		t.Fatalf("真实流量模型维度丢失：%#v", samples[0].Payload)
	}
}

func TestConvertTrafficRowsAcceptsModelNameField(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	samples, malformed := convertTrafficRows("41", []business.EvidenceTarget{{AccountID: "41", GroupName: "codex"}}, []map[string]any{{
		"account_id": "41", "request_id": "request-1", "kind": "success",
		"created_at": now.Format(time.RFC3339Nano), "first_token_ms": json.Number("1250"), "model_name": "gpt-test",
	}}, now.Add(-time.Minute), 10)
	if malformed != 0 || len(samples) != 1 || samples[0].Payload["model"] != "gpt-test" {
		t.Fatalf("model_name was not preserved: samples=%#v malformed=%d", samples, malformed)
	}
}

func TestDueProbeAccountsHonorsGroupProbeInterval(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	lastProbe := now.Add(-2 * time.Minute)
	groupID := "7"
	policy, err := parsePolicy(map[string]any{
		"traffic": map[string]any{"enabled": false},
		"probe":   map[string]any{"enabled": true, "interval_seconds": int64(600)},
		"recovery": map[string]any{
			"enabled": true, "probe_interval_seconds": int64(180),
		},
		"group_policy_bindings": map[string]any{
			"7": map[string]any{"probe_enabled": true, "probe_interval_seconds": int64(60)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	targets := []business.EvidenceTarget{{AccountID: "41", GroupName: "codex", GroupID: &groupID, ProbeAt: &lastProbe}}

	due := dueProbeAccounts(targets, policy, now, false, map[string]struct{}{})
	if len(due) != 1 || due[0] != "41" {
		t.Fatalf("分组 60 秒探测周期没有生效：%v", due)
	}
}

func TestParsePolicyRejectsRegularProbeIntervalBelowGuardianMinimum(t *testing.T) {
	_, err := parsePolicy(map[string]any{
		"traffic":  map[string]any{"enabled": false},
		"probe":    map[string]any{"enabled": true, "interval_seconds": int64(29)},
		"recovery": map[string]any{"enabled": true, "probe_interval_seconds": int64(1)},
	})
	if err == nil || err.Error() != "probe.interval_seconds 配置无效" {
		t.Fatalf("regular probe interval below 30 seconds was accepted: %v", err)
	}
}

func TestParsePolicyRejectsValuesOutsidePersistedPolicyBounds(t *testing.T) {
	valid := func() map[string]any {
		return map[string]any{
			"traffic": map[string]any{
				"enabled": true, "lookback_minutes": int64(120), "max_samples_per_account": int64(60), "refresh_seconds": int64(60),
			},
			"probe": map[string]any{
				"enabled": true, "interval_seconds": int64(300), "concurrency": int64(4), "traffic_fresh_seconds": int64(180),
			},
			"recovery": map[string]any{"enabled": true, "probe_interval_seconds": int64(180)},
		}
	}
	tests := []struct {
		name    string
		section string
		field   string
		value   int64
	}{
		{"traffic lookback maximum", "traffic", "lookback_minutes", 10081},
		{"traffic refresh maximum", "traffic", "refresh_seconds", 86401},
		{"probe interval maximum", "probe", "interval_seconds", 86401},
		{"traffic freshness maximum", "probe", "traffic_fresh_seconds", 86401},
		{"recovery interval maximum", "recovery", "probe_interval_seconds", 86401},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := valid()
			policy[test.section].(map[string]any)[test.field] = test.value
			if _, err := parsePolicy(policy); err == nil {
				t.Fatalf("%s.%s=%d bypassed execution bounds", test.section, test.field, test.value)
			}
		})
	}
}

func TestDueProbeAccountsUsesRecoveryProbeWhenRegularProbeDisabled(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	lastProbe := now.Add(-4 * time.Minute)
	policy, err := parsePolicy(map[string]any{
		"traffic": map[string]any{"enabled": true},
		"probe": map[string]any{
			"enabled": false, "interval_seconds": int64(600), "skip_when_traffic_fresh": true,
		},
		"recovery": map[string]any{
			"enabled": true, "probe_interval_seconds": int64(180),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	targets := []business.EvidenceTarget{{
		AccountID: "41", GroupName: "codex", EffectiveState: "fused", ProbeAt: &lastProbe,
	}}

	due := dueProbeAccounts(targets, policy, now, false, map[string]struct{}{})
	if len(due) != 1 || due[0] != "41" {
		t.Fatalf("普通探针关闭后没有独立安排熔断回池探测：%v", due)
	}
}

func TestDueProbeAccountsIgnoresUnappliedDesiredFuse(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	lastProbe := now.Add(-4 * time.Minute)
	decision := "fused"
	policy, err := parsePolicy(map[string]any{
		"traffic":  map[string]any{"enabled": true},
		"probe":    map[string]any{"enabled": false, "interval_seconds": int64(600)},
		"recovery": map[string]any{"enabled": true, "probe_interval_seconds": int64(180)},
	})
	if err != nil {
		t.Fatal(err)
	}
	targets := []business.EvidenceTarget{{
		AccountID: "41", GroupName: "codex", EffectiveState: "healthy",
		DecisionState: &decision, ProbeAt: &lastProbe,
	}}
	if due := dueProbeAccounts(targets, policy, now, false, map[string]struct{}{}); len(due) != 0 {
		t.Fatalf("未落地的期望熔断不应触发恢复探针：%v", due)
	}
}

func TestDueProbeAccountsRespectsDisabledGroupProbe(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	groupID := "7"
	policy, err := parsePolicy(map[string]any{
		"traffic": map[string]any{"enabled": false},
		"probe":   map[string]any{"enabled": true, "interval_seconds": int64(60)},
		"recovery": map[string]any{
			"enabled": true, "probe_interval_seconds": int64(180),
		},
		"group_policy_bindings": map[string]any{
			"7": map[string]any{"probe_enabled": false, "probe_interval_seconds": int64(60)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	targets := []business.EvidenceTarget{{AccountID: "41", GroupName: "codex", GroupID: &groupID}}

	if due := dueProbeAccounts(targets, policy, now, false, map[string]struct{}{}); len(due) != 0 {
		t.Fatalf("关闭分组定时测试后仍安排了探测：%v", due)
	}
}

func TestDueProbeAccountsUsesStablePrimaryGroupPolicy(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	lastProbe := now.Add(-2 * time.Minute)
	primaryID, secondaryID := "7", "9"
	policy, err := parsePolicy(map[string]any{
		"traffic":  map[string]any{"enabled": false},
		"probe":    map[string]any{"enabled": true, "interval_seconds": int64(600)},
		"recovery": map[string]any{"enabled": true, "probe_interval_seconds": int64(180)},
		"group_policy_bindings": map[string]any{
			"7": map[string]any{"probe_enabled": true, "probe_interval_seconds": int64(600)},
			"9": map[string]any{"probe_enabled": true, "probe_interval_seconds": int64(60)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	targets := []business.EvidenceTarget{
		{AccountID: "41", GroupName: "secondary", GroupID: &secondaryID, ProbeAt: &lastProbe},
		{AccountID: "41", GroupName: "primary", GroupID: &primaryID, ProbeAt: &lastProbe},
	}
	if due := dueProbeAccounts(targets, policy, now, false, map[string]struct{}{}); len(due) != 0 {
		t.Fatalf("多分组账号不应由次分组的较短周期重复探测：%v", due)
	}
}

func TestDueProbeAccountsTreatsRecentProbeAsFreshSample(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	lastProbe := now.Add(-90 * time.Second)
	policy, err := parsePolicy(map[string]any{
		"traffic": map[string]any{"enabled": true},
		"probe": map[string]any{
			"enabled": true, "interval_seconds": int64(60), "skip_when_traffic_fresh": true,
			"traffic_fresh_seconds": int64(180),
		},
		"recovery": map[string]any{"enabled": true, "probe_interval_seconds": int64(180)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if due := dueProbeAccounts([]business.EvidenceTarget{{AccountID: "41", ProbeAt: &lastProbe}}, policy, now, false, map[string]struct{}{}); len(due) != 0 {
		t.Fatalf("最近探针样本仍新鲜时不应再次探测：%v", due)
	}
}

func TestTrafficFetchDueHonorsSuccessfulFetchInterval(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-30 * time.Second)
	if trafficFetchDue([]business.EvidenceTarget{{AccountID: "41", TrafficFetchAt: &recent}}, now, time.Minute) {
		t.Fatal("真实流量刚拉取成功，不应在间隔内重复请求运维接口")
	}
	old := now.Add(-61 * time.Second)
	if !trafficFetchDue([]business.EvidenceTarget{{AccountID: "41", TrafficFetchAt: &old}}, now, time.Minute) {
		t.Fatal("真实流量拉取间隔到期后应再次读取")
	}
}

func TestFilterEvidenceTargetsMatchesGuardianScope(t *testing.T) {
	group7, group9, group11 := "7", "9", "11"
	policy, err := parsePolicy(map[string]any{
		"traffic": map[string]any{"enabled": true}, "probe": map[string]any{},
		"recovery": map[string]any{},
		"scope": map[string]any{
			"managed_group_mode": "selected", "managed_group_ids": []any{"7", "9"},
			"excluded_group_ids": []any{"9"}, "excluded_account_ids": []any{"44"},
			"account_types": []any{"apikey"}, "platforms": []any{"openai"},
		},
		"group_policy_bindings": map[string]any{"11": map[string]any{"enabled": false}},
	})
	if err != nil {
		t.Fatal(err)
	}
	targets := []business.EvidenceTarget{
		{AccountID: "41", GroupName: "managed", GroupID: &group7, AccountType: "apikey", Platform: "openai"},
		{AccountID: "41", GroupName: "excluded-membership", GroupID: &group9, AccountType: "apikey", Platform: "openai"},
		{AccountID: "42", GroupName: "wrong-type", GroupID: &group7, AccountType: "oauth", Platform: "openai"},
		{AccountID: "43", GroupName: "wrong-platform", GroupID: &group7, AccountType: "apikey", Platform: "claude"},
		{AccountID: "44", GroupName: "excluded-account", GroupID: &group7, AccountType: "apikey", Platform: "openai"},
		{AccountID: "45", GroupName: "disabled-binding", GroupID: &group11, AccountType: "apikey", Platform: "openai"},
	}

	filtered := filterEvidenceTargets(targets, policy)
	if len(filtered) != 1 || filtered[0].AccountID != "41" || filtered[0].GroupID == nil || *filtered[0].GroupID != "7" {
		t.Fatalf("采集范围没有按 Guardian 过滤：%#v", filtered)
	}
}

func TestFilterEvidenceTargetsKeepsPausedAccountsForMonitoring(t *testing.T) {
	policy, err := parsePolicy(map[string]any{
		"traffic": map[string]any{"enabled": true}, "probe": map[string]any{},
		"recovery": map[string]any{}, "scope": map[string]any{"paused_account_ids": []any{"41"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	target := business.EvidenceTarget{AccountID: "41", GroupName: "codex", AccountType: "apikey"}
	filtered := filterEvidenceTargets([]business.EvidenceTarget{target}, policy)
	if len(filtered) != 1 {
		t.Fatalf("暂停账号仍应采样计分：%#v", filtered)
	}
}
