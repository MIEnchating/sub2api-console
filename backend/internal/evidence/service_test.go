package evidence

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/probe"
	"github.com/MIEnchating/sub2api-console/backend/internal/routing"
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

type lateTrafficRepository struct {
	targets []business.EvidenceTarget
	samples []business.TrafficSample
	fetched []string
}

func (r *lateTrafficRepository) EvidenceTargets(context.Context, *string, *string) ([]business.EvidenceTarget, error) {
	return r.targets, nil
}

func (r *lateTrafficRepository) PersistTrafficSamples(_ context.Context, samples []business.TrafficSample) (int, error) {
	r.samples = append(r.samples, samples...)
	return len(samples), nil
}

func (r *lateTrafficRepository) PersistTrafficFetches(_ context.Context, accountIDs []string, _ time.Time) error {
	r.fetched = append(r.fetched, accountIDs...)
	return nil
}

type lateTrafficAdmin struct {
	calls      int
	recheckErr error
}

func (a *lateTrafficAdmin) RequestDetails(_ context.Context, accountID string, _, _ int) ([]map[string]any, error) {
	a.calls++
	if a.calls == 1 {
		return []map[string]any{}, nil
	}
	if a.recheckErr != nil {
		return nil, a.recheckErr
	}
	return []map[string]any{{
		"account_id": accountID, "request_id": "late-request", "kind": "success",
		"created_at": time.Now().UTC().Format(time.RFC3339Nano), "duration_ms": json.Number("1200"),
	}}, nil
}

type lateTrafficProbeRunner struct {
	request probe.Request
}

func (r *lateTrafficProbeRunner) RunNow(ctx context.Context, request probe.Request) (probe.RunSummary, error) {
	r.request = request
	if request.FreshTrafficCheck == nil {
		return probe.RunSummary{}, errors.New("missing fresh traffic check")
	}
	fresh, err := request.FreshTrafficCheck(ctx, "41")
	if err != nil {
		return probe.RunSummary{Targets: 1, Persisted: 1, Passed: 1}, nil
	}
	if !fresh {
		return probe.RunSummary{Targets: 1, Persisted: 1, Passed: 1}, nil
	}
	return probe.RunSummary{Targets: 1, Skipped: 1}, nil
}

func TestCollectSkipsQueuedProbeWhenTrafficAppearsAfterInitialFetch(t *testing.T) {
	now := time.Now().UTC()
	old := now.Add(-10 * time.Minute)
	repository := &lateTrafficRepository{targets: []business.EvidenceTarget{{
		AccountID: "41", GroupName: "codex", TrafficAt: &old, ProbeAt: &old,
	}}}
	admin := &lateTrafficAdmin{}
	probes := &lateTrafficProbeRunner{}

	result, err := New(repository, probes).Collect(
		context.Background(), testTrafficProbePolicy(), admin,
		Options{FetchTraffic: true, ProbesAllowed: true, Now: now},
	)

	if err != nil {
		t.Fatal(err)
	}
	if admin.calls != 2 || len(repository.samples) != 1 || repository.samples[0].EvidenceKey != "late-request" {
		t.Fatalf("calls=%d samples=%#v", admin.calls, repository.samples)
	}
	if result.TrafficPersisted != 1 || result.ProbesPersisted != 0 || result.EffectiveSource != "traffic" {
		t.Fatalf("result=%#v", result)
	}
}

func TestCollectFallsBackToProbeAndReportsLateTrafficCheckFailure(t *testing.T) {
	now := time.Now().UTC()
	old := now.Add(-10 * time.Minute)
	repository := &lateTrafficRepository{targets: []business.EvidenceTarget{{
		AccountID: "41", GroupName: "codex", TrafficAt: &old, ProbeAt: &old,
	}}}
	admin := &lateTrafficAdmin{recheckErr: errors.New("monitoring unavailable")}
	probes := &lateTrafficProbeRunner{}

	result, err := New(repository, probes).Collect(context.Background(), testTrafficProbePolicy(), admin, Options{
		FetchTraffic: true, ProbesAllowed: true, Now: now,
	})

	if err != nil {
		t.Fatal(err)
	}
	if admin.calls != 2 || result.ProbesPersisted != 1 || len(result.SourceErrors) != 1 ||
		!strings.Contains(result.SourceErrors[0], "执行前流量复核") {
		t.Fatalf("calls=%d result=%#v", admin.calls, result)
	}
}

func TestCollectKeepsRecoveryProbeForFusedAccountWithoutLateTrafficCheck(t *testing.T) {
	now := time.Now().UTC()
	old := now.Add(-10 * time.Minute)
	repository := &lateTrafficRepository{targets: []business.EvidenceTarget{{
		AccountID: "41", GroupName: "codex", EffectiveState: "fused", TrafficAt: &old, ProbeAt: &old,
	}}}
	admin := &lateTrafficAdmin{}
	probes := &lateTrafficProbeRunner{}

	result, err := New(repository, probes).Collect(context.Background(), testTrafficProbePolicy(), admin, Options{
		FetchTraffic: true, ProbesAllowed: true, Now: now,
	})

	if err != nil {
		t.Fatal(err)
	}
	if admin.calls != 1 || result.ProbesPersisted != 1 || len(repository.samples) != 0 {
		t.Fatalf("calls=%d samples=%#v result=%#v", admin.calls, repository.samples, result)
	}
}

func TestFreshTrafficCheckRevalidatesLaterTargetForSameAccount(t *testing.T) {
	targets := []business.EvidenceTarget{{AccountID: "41", GroupName: "codex"}}
	repository := &lateTrafficRepository{targets: targets}
	admin := &lateTrafficAdmin{}
	result := Result{SourceErrors: []string{}}
	trafficForScope := false
	policy, err := parsePolicy(testTrafficProbePolicy())
	if err != nil {
		t.Fatal(err)
	}
	check := New(repository, nil).freshTrafficCheck(
		admin, groupTargets(targets), policy, &result, &trafficForScope, targets,
	)

	firstFresh, err := check(context.Background(), "41")
	if err != nil {
		t.Fatal(err)
	}
	secondFresh, err := check(context.Background(), "41")
	if err != nil {
		t.Fatal(err)
	}

	if firstFresh || !secondFresh || admin.calls != 2 || len(repository.samples) != 1 {
		t.Fatalf("first=%t second=%t calls=%d samples=%#v", firstFresh, secondFresh, admin.calls, repository.samples)
	}
}

func testTrafficProbePolicy() map[string]any {
	return map[string]any{
		"traffic": map[string]any{
			"enabled": true, "lookback_minutes": int64(120), "max_samples_per_account": int64(60), "refresh_seconds": int64(60),
		},
		"probe": map[string]any{
			"enabled": true, "interval_seconds": int64(300), "concurrency": int64(4),
			"skip_when_traffic_fresh": true, "traffic_fresh_seconds": int64(180),
		},
		"recovery": map[string]any{"enabled": true, "probe_interval_seconds": int64(180)},
	}
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
		now,
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
	}}, now.Add(-time.Minute), now, 10)
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
		now,
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
	}}, now.Add(-time.Minute), now, 10)
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

func TestDueProbeAccountsDoesNotTreatRecentProbeAsFreshTraffic(t *testing.T) {
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
	if due := dueProbeAccounts([]business.EvidenceTarget{{AccountID: "41", ProbeAt: &lastProbe}}, policy, now, false, map[string]struct{}{}); len(due) != 1 {
		t.Fatalf("最近探针不能冒充新鲜真实流量：%v", due)
	}
}

func TestDueProbeAccountsSkipsRegularProbeForFreshTraffic(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	lastProbe := now.Add(-10 * time.Minute)
	lastTraffic := now.Add(-90 * time.Second)
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
	target := business.EvidenceTarget{AccountID: "41", TrafficAt: &lastTraffic, ProbeAt: &lastProbe}
	if due := dueProbeAccounts([]business.EvidenceTarget{target}, policy, now, false, map[string]struct{}{}); len(due) != 0 {
		t.Fatalf("新鲜真实流量未抑制普通探针：%v", due)
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

func TestOfficialUsageCollectionPersistsFirstTokenAndKeepsErrorsOnPartialFailure(t *testing.T) {
	for _, partial := range []bool{false, true} {
		for _, late := range []bool{false, true} {
			name := "initial"
			if late {
				name = "before probe"
			}
			if partial {
				name += " usage unavailable"
			}
			t.Run(name, func(t *testing.T) {
				now := time.Now().UTC()
				ctx := context.Background()
				store, client := officialUsageFixture(t, now, partial)
				service := New(store, nil)
				result := Result{}
				if late {
					targets, err := store.EvidenceTargets(ctx, nil, nil)
					if err != nil {
						t.Fatal(err)
					}
					policy, err := parsePolicy(testTrafficProbePolicy())
					if err != nil {
						t.Fatal(err)
					}
					trafficForScope := false
					fresh, err := service.freshTrafficCheck(client, groupTargets(targets), policy, &result, &trafficForScope, targets)(ctx, "41")
					if err != nil || !fresh {
						t.Fatalf("fresh=%v err=%v", fresh, err)
					}
				} else {
					var err error
					result, err = service.Collect(ctx, testTrafficProbePolicy(), client, Options{FetchTraffic: true, Now: now})
					if err != nil {
						t.Fatal(err)
					}
				}
				if partial != (len(result.SourceErrors) > 0) {
					t.Fatalf("source errors=%v", result.SourceErrors)
				}
				rows, err := store.RoutingSamples(ctx, nil, nil, "traffic", 60)
				if err != nil || len(rows) != 2 {
					t.Fatalf("rows=%v err=%v", rows, err)
				}
				byRequest := map[string]business.RoutingSample{}
				for _, row := range rows {
					byRequest[row.Payload["request_id"].(string)] = row
				}
				if byRequest["failure"].Result != "失败" {
					t.Fatalf("failure lost: %v", byRequest)
				}
				success := byRequest["success"]
				if partial {
					if success.Payload["first_token_ms"] != nil {
						t.Fatalf("duration used as first token: %v", success.Payload)
					}
				} else {
					if success.Payload["first_token_ms"] != "6000" || success.Payload["first_token_source"] != "usage.first_token_ms" {
						t.Fatalf("first token not persisted: %v", success.Payload)
					}
					health, err := routing.HealthScore([]routing.Sample{{Result: success.Result, Source: success.Source, LatencyP95: success.LatencyP95, Payload: success.Payload}}, nil)
					if err != nil || health.HealthScore != 65 || health.P95MS == nil || *health.P95MS != 6000 {
						t.Fatalf("health=%#v err=%v", health, err)
					}
				}
				targets, err := store.EvidenceTargets(ctx, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				if partial && targets[0].TrafficFetchAt != nil {
					t.Fatal("partial read postponed retry")
				}
			})
		}
	}
}

func officialUsageFixture(t *testing.T, now time.Time, partial bool) (*business.Store, *adminclient.Client) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "evidence.sqlite3")
	store, err := business.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, seedErr := db.Exec(`INSERT INTO accounts(id,name,metadata_json,updated_at) VALUES('41','test','{}','now'); INSERT INTO account_groups(account_id,group_name) VALUES('41','codex')`)
	_ = db.Close()
	if seedErr != nil {
		t.Fatal(seedErr)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var rows []map[string]any
		switch r.URL.Path {
		case "/api/v1/admin/ops/requests":
			rows = []map[string]any{
				{"account_id": 41, "request_id": "success", "kind": "success", "created_at": now.Format(time.RFC3339Nano), "duration_ms": 30000},
				{"account_id": 41, "request_id": "failure", "kind": "error", "created_at": now.Add(-time.Second).Format(time.RFC3339Nano), "status_code": 502, "message": "gateway failed"},
			}
		case "/api/v1/admin/usage":
			if partial {
				http.Error(w, `{"message":"usage unavailable"}`, http.StatusServiceUnavailable)
				return
			}
			rows = []map[string]any{{"id": 1, "account_id": 41, "request_id": "success", "first_token_ms": 6000}}
		default:
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"items": rows, "total": len(rows)}})
	}))
	t.Cleanup(server.Close)
	client, err := adminclient.New(adminclient.Config{BaseURL: server.URL, AdminKey: "isolated-test", Attempts: 1, Timeout: time.Second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return store, client
}
