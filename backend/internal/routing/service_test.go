package routing

import (
	"context"
	"encoding/json"
	"math"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

type routingRepositoryStub struct {
	policy           map[string]any
	accounts         []business.RoutingAccount
	persistDecisions bool
	evaluations      []business.RoutingEvaluationWrite
	decisions        []business.RoutingDecisionWrite
	targets          []business.AccountRoutingTarget
	cleanupState     map[string]time.Time
	cleanupWrites    []business.CleanupStateWrite
	runtimeEvents    []business.RuntimeEventWrite
	samples          []business.RoutingSample
}

func (r *routingRepositoryStub) ControlPolicy(context.Context) (map[string]any, error) {
	return r.policy, nil
}

func (r *routingRepositoryStub) RoutingAccounts(context.Context, *string, *string) ([]business.RoutingAccount, error) {
	return r.accounts, nil
}

func (r *routingRepositoryStub) RoutingSamples(context.Context, *string, *string, string, int) ([]business.RoutingSample, error) {
	return r.samples, nil
}

func (r *routingRepositoryStub) PreviousRoutingDecisions(context.Context, *string, *string) ([]business.PreviousRoutingDecision, error) {
	return nil, nil
}

func (r *routingRepositoryStub) CleanupStates(context.Context, *string) (map[string]time.Time, error) {
	return r.cleanupState, nil
}

func (r *routingRepositoryStub) PersistRoutingRound(
	_ context.Context,
	_, _ *string,
	evaluations []business.RoutingEvaluationWrite,
	decisions []business.RoutingDecisionWrite,
	targets []business.AccountRoutingTarget,
	cleanupWrites []business.CleanupStateWrite,
	runtimeEvents []business.RuntimeEventWrite,
	persistDecisions bool,
	_ time.Time,
) error {
	r.persistDecisions = persistDecisions
	r.evaluations = evaluations
	r.decisions = decisions
	r.targets = targets
	r.cleanupWrites = cleanupWrites
	r.runtimeEvents = runtimeEvents
	return nil
}

func TestResolveRateUsesCurrentAccountMultiplierBeforeStaleMembershipRate(t *testing.T) {
	accountMultiplier, membershipRate := "0.17", "0.2"
	rate, text, known, reason := resolveRate(business.RoutingAccount{
		Multiplier: &accountMultiplier,
		GroupRate:  &membershipRate,
	}, engineConfig{}, nil)
	if rate == nil || rate.Cmp(big.NewRat(17, 100)) != 0 || text == nil || *text != "0.17" || !known || reason != nil {
		t.Fatalf("rate=%v text=%v known=%v reason=%v", rate, text, known, reason)
	}
}

func TestPausedAccountScopeKeepsScoringButStopsScheduling(t *testing.T) {
	policy := routingPolicy()
	policy["scope"].(map[string]any)["paused_account_ids"] = []any{"41"}
	schedulable := true
	multiplier := "1"
	repository := &routingRepositoryStub{
		policy: policy,
		accounts: []business.RoutingAccount{{
			ID: "41", Name: "upstream-0.1", GroupName: "codex", Schedulable: &schedulable,
			Multiplier: &multiplier, Metadata: map[string]any{},
		}},
	}
	service := NewService(repository)
	service.now = func() time.Time { return time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC) }

	result, err := service.Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.HealthEvaluations != 1 || len(repository.evaluations) != 1 {
		t.Fatalf("暂停账号仍应参与健康评分：%#v", result)
	}
	if len(repository.decisions) != 1 || repository.decisions[0].State != "paused" || repository.decisions[0].Schedulable == nil || *repository.decisions[0].Schedulable {
		t.Fatalf("暂停账号仍进入了流量池：%#v", repository.decisions)
	}
	if len(repository.targets) != 1 || repository.targets[0].Schedulable == nil || *repository.targets[0].Schedulable {
		t.Fatalf("暂停账号写回目标不正确：%#v", repository.targets)
	}
}

func TestUpstreamManagementAuthFailureDoesNotFuseHealthyAccount(t *testing.T) {
	policy := routingPolicy()
	schedulable := true
	multiplier := "1"
	authStatus := "认证过期"
	repository := &routingRepositoryStub{
		policy: policy,
		accounts: []business.RoutingAccount{{
			ID: "41", Name: "healthy", GroupName: "codex", Schedulable: &schedulable,
			Multiplier: &multiplier, UpstreamAuthStatus: &authStatus, Metadata: map[string]any{},
		}},
		samples: []business.RoutingSample{{
			AccountID: "41", GroupName: "codex", Result: "通过", Source: "traffic",
			ObservedAt: "2026-08-28T00:00:00Z", Payload: map[string]any{"status_code": int64(200)},
		}},
	}
	result, err := NewService(repository).Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(repository.decisions) != 1 || repository.decisions[0].State != "healthy" ||
		repository.decisions[0].Schedulable == nil || !*repository.decisions[0].Schedulable || result.Fused != 0 {
		t.Fatalf("management auth state incorrectly fused a healthy account: result=%#v decisions=%#v", result, repository.decisions)
	}
}

func TestConfirmedMissingUpstreamBindingStopsSchedulingWithoutSurvivorFallback(t *testing.T) {
	policy := routingPolicy()
	schedulable := true
	multiplier := "1"
	reason := "绑定的上游 Key key-1 已确认删除（连续 2 次完整同步未返回）"
	repository := &routingRepositoryStub{
		policy: policy,
		accounts: []business.RoutingAccount{{
			ID: "41", Name: "missing-key", GroupName: "codex", Schedulable: &schedulable,
			Multiplier: &multiplier, CatalogBindingState: "key_missing", CatalogBindingReason: &reason,
			Metadata: map[string]any{},
		}},
	}
	result, err := NewService(repository).Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	decision := result.AccountDecisions["41"]
	if decision.RoutingState != "binding_invalid" || decision.Schedulable || decision.Reason != reason {
		t.Fatalf("confirmed missing binding stayed schedulable: %#v", decision)
	}
	target := result.AccountTargets["41"]
	if target.Schedulable == nil || *target.Schedulable || target.DesiredHealth != "binding_invalid" {
		t.Fatalf("missing binding did not produce a stop target: %#v", target)
	}
}

func TestSuspectedMissingUpstreamBindingDoesNotStopScheduling(t *testing.T) {
	policy := routingPolicy()
	schedulable := true
	multiplier := "1"
	reason := "本轮完整同步未返回，等待下一轮复核"
	repository := &routingRepositoryStub{
		policy: policy,
		accounts: []business.RoutingAccount{{
			ID: "41", Name: "suspected-key", GroupName: "codex", Schedulable: &schedulable,
			Multiplier: &multiplier, CatalogBindingState: "suspected", CatalogBindingReason: &reason,
			Metadata: map[string]any{},
		}},
	}
	result, err := NewService(repository).Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	decision := result.AccountDecisions["41"]
	if !decision.Schedulable || decision.RoutingState == "binding_invalid" {
		t.Fatalf("one missing snapshot stopped scheduling: %#v", decision)
	}
}

func TestExcludedGroupReleasesAccountControl(t *testing.T) {
	policy := routingPolicy()
	policy["scope"].(map[string]any)["excluded_group_ids"] = []any{"7"}
	schedulable, multiplier := true, "1"
	groupID := "7"
	repository := &routingRepositoryStub{policy: policy, accounts: []business.RoutingAccount{{
		ID: "41", GroupName: "codex", GroupID: &groupID, Schedulable: &schedulable, Multiplier: &multiplier, Metadata: map[string]any{},
	}}}
	result, err := NewService(repository).Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	target, found := result.AccountTargets["41"]
	if !found || !target.ReleaseControl || target.DesiredHealth != "excluded" {
		t.Fatalf("账号所有分组退出管控后没有交还控制权：%#v", result.AccountTargets)
	}
}

func TestManualPriorityAccountIsProtectedFromAutomaticScheduling(t *testing.T) {
	policy := routingPolicy()
	policy["manual_priority"] = map[string]any{"reserved_max": int64(10)}
	schedulable, multiplier := true, "1"
	manualSlot := int64(3)
	repository := &routingRepositoryStub{policy: policy, accounts: []business.RoutingAccount{
		{ID: "41", Name: "manual", GroupName: "codex", Schedulable: &schedulable, Multiplier: &multiplier, ManualPriority: &manualSlot, Metadata: map[string]any{}},
		{ID: "42", Name: "automatic", GroupName: "codex", Schedulable: &schedulable, Multiplier: &multiplier, Metadata: map[string]any{}},
	}}
	result, err := NewService(repository).Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Accounts != 1 || len(repository.decisions) != 1 || repository.decisions[0].AccountID != "42" {
		t.Fatalf("manual account entered automatic decisions: result=%#v decisions=%#v", result, repository.decisions)
	}
	if _, found := result.AccountTargets["41"]; found {
		t.Fatalf("manual account received automatic target: %#v", result.AccountTargets["41"])
	}
	target := result.AccountTargets["42"]
	if target.Priority == nil || *target.Priority < 11 {
		t.Fatalf("automatic priority entered reserved range: %#v", target)
	}
}

func TestAutomaticPriorityStartsAfterConfiguredManualRange(t *testing.T) {
	priority := int64(2)
	item := &candidate{account: business.RoutingAccount{ID: "41", GroupName: "codex", Priority: &priority}, state: "healthy", schedulable: true, weight: 400}
	config := engineConfig{manualPriorityMax: 25, weightsEnabled: true, weightBudget: 400, minLoadFactor: 1, maxLoadFactor: 100}
	assignAccountPlacements(map[string][]*candidate{"codex": {item}}, map[string]engineConfig{"codex": config}, map[string][]*candidate{"41": {item}})
	if item.desiredPriority == nil || *item.desiredPriority != 26 {
		t.Fatalf("automatic priority did not start after reserved range: %#v", item.desiredPriority)
	}
}

func TestExcludedMembershipDoesNotReleaseStillManagedAccount(t *testing.T) {
	policy := routingPolicy()
	policy["scope"].(map[string]any)["excluded_group_ids"] = []any{"9"}
	schedulable, multiplier := true, "1"
	group7, group9 := "7", "9"
	repository := &routingRepositoryStub{policy: policy, accounts: []business.RoutingAccount{
		{ID: "41", GroupName: "group-a", GroupID: &group7, Schedulable: &schedulable, Multiplier: &multiplier, Metadata: map[string]any{}},
		{ID: "41", GroupName: "group-b", GroupID: &group9, Schedulable: &schedulable, Multiplier: &multiplier, Metadata: map[string]any{}},
	}}
	result, err := NewService(repository).Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if target := result.AccountTargets["41"]; target.ReleaseControl {
		t.Fatalf("账号仍有受管分组时错误交还控制权：%#v", target)
	}
}

func TestTypeMismatchIsUnmanagedAndReleasesControlWithoutDecision(t *testing.T) {
	policy := routingPolicy()
	policy["scope"].(map[string]any)["account_types"] = []any{"oauth"}
	schedulable, multiplier := true, "1"
	repository := &routingRepositoryStub{policy: policy, accounts: []business.RoutingAccount{{
		ID: "41", GroupName: "codex", Schedulable: &schedulable, Multiplier: &multiplier,
		Metadata: map[string]any{"account_type": "apikey", "platform": "openai"},
	}}}

	result, err := NewService(repository).Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Accounts != 0 || result.HealthEvaluations != 0 || len(repository.decisions) != 0 {
		t.Fatalf("类型不匹配账号不应参与评分或生成决策：result=%#v decisions=%#v", result, repository.decisions)
	}
	if target := result.AccountTargets["41"]; !target.ReleaseControl {
		t.Fatalf("退出守护范围的账号没有交还控制权：%#v", target)
	}
}

func TestMultiGroupAccountAveragesWeightAndUsesPrimaryPlacement(t *testing.T) {
	group1, group2 := "1", "2"
	priority, concurrency := int64(20), int64(10)
	first := &candidate{account: business.RoutingAccount{ID: "41", GroupName: "group-a", GroupID: &group1, Priority: &priority, Concurrency: &concurrency}, state: "healthy", schedulable: true, weight: 100}
	second := &candidate{account: business.RoutingAccount{ID: "41", GroupName: "group-b", GroupID: &group2, Priority: &priority, Concurrency: &concurrency}, state: "healthy", schedulable: true, weight: 300}
	config := engineConfig{weightsEnabled: true, weightBudget: 400, minLoadFactor: 1, maxLoadFactor: 100, scalingEnabled: true, scalingGlobalMax: 20, scalingMin: 3, scalingMax: 250, scalingUpRatio: .5, scalingStepUp: 5, scalingStepDown: 5}
	groups := map[string][]*candidate{"group-a": {first}, "group-b": {second}}
	byAccount := map[string][]*candidate{"41": {first, second}}
	assignAccountPlacements(groups, map[string]engineConfig{"group-a": config, "group-b": config}, byAccount)
	if first.weight != 200 || second.weight != 200 {
		t.Fatalf("多分组权重没有取平均：first=%v second=%v", first.weight, second.weight)
	}
	if first.desiredPriority == nil || second.desiredPriority == nil || *first.desiredPriority != *second.desiredPriority ||
		first.desiredLoad == nil || second.desiredLoad == nil || *first.desiredLoad != *second.desiredLoad {
		t.Fatalf("账号级调度字段没有由主分组统一生成：first=%#v second=%#v", first, second)
	}
	if first.desiredConcurrency == nil || second.desiredConcurrency == nil || *first.desiredConcurrency != 15 || *second.desiredConcurrency != 15 {
		t.Fatalf("扩缩容没有只由主分组计算后统一传播：first=%#v second=%#v", first.desiredConcurrency, second.desiredConcurrency)
	}
}

func TestCalculatePersistsOneCanonicalStateForMultiGroupAccount(t *testing.T) {
	group1, group2 := "1", "2"
	schedulable, multiplier := true, "0.2"
	repository := &routingRepositoryStub{policy: routingPolicy(), accounts: []business.RoutingAccount{
		{ID: "41", Name: "shared", GroupName: "group-a", GroupID: &group1, Schedulable: &schedulable, Multiplier: &multiplier, Metadata: map[string]any{}},
		{ID: "41", Name: "shared", GroupName: "group-b", GroupID: &group2, Schedulable: &schedulable, Multiplier: &multiplier, Metadata: map[string]any{}},
	}}

	result, err := NewService(repository).Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Decisions != 1 || len(repository.decisions) != 1 {
		t.Fatalf("多分组账号保存了多份最终决策：result=%d writes=%#v", result.Decisions, repository.decisions)
	}
	if len(repository.evaluations) != 1 {
		t.Fatalf("多分组账号保存了多份健康评估：%#v", repository.evaluations)
	}
	if repository.decisions[0].AccountID != "41" || repository.decisions[0].GroupName != "group-a" ||
		repository.evaluations[0].GroupName != "group-a" {
		t.Fatalf("最终状态没有归属稳定主分组：decisions=%#v evaluations=%#v", repository.decisions, repository.evaluations)
	}
}

func TestAlignAccountStateUsesPrimaryHealthAndMultiplierForEveryMembership(t *testing.T) {
	group1, group2 := "1", "2"
	schedulable := true
	primaryRate, secondaryRate := "0.2", "0.9"
	primary := &candidate{
		account: business.RoutingAccount{ID: "41", GroupName: "group-a", GroupID: &group1, Schedulable: &schedulable, Metadata: map[string]any{}},
		health:  Health{HealthScore: 91, ShortScore: 92, LongScore: 90, SampleCount: 3}, rateText: &primaryRate,
	}
	secondary := &candidate{
		account: business.RoutingAccount{ID: "41", GroupName: "group-b", GroupID: &group2, Schedulable: &schedulable, Metadata: map[string]any{}},
		health:  Health{HealthScore: 22, ShortScore: 20, LongScore: 24, SampleCount: 1}, rateText: &secondaryRate,
	}
	primary.rate, secondary.rate = big.NewRat(1, 5), big.NewRat(9, 10)

	alignAccountStateToPrimary(map[string][]*candidate{"41": {secondary, primary}}, map[string]engineConfig{
		"group-a": {degradeEnabled: false},
		"group-b": {degradeEnabled: false},
	}, map[string]business.PreviousRoutingDecision{}, time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC))

	if secondary.health.HealthScore != primary.health.HealthScore || secondary.health.ShortScore != primary.health.ShortScore ||
		secondary.health.LongScore != primary.health.LongScore || secondary.health.SampleCount != primary.health.SampleCount {
		t.Fatalf("同一账号仍有多份健康评估：primary=%#v secondary=%#v", primary.health, secondary.health)
	}
	if secondary.rate == nil || secondary.rate.Cmp(primary.rate) != 0 || secondary.rateText == nil || *secondary.rateText != primaryRate {
		t.Fatalf("同一账号仍有多份调度倍率：primary=%v secondary=%v", primary.rateText, secondary.rateText)
	}
}

func TestMultiGroupAccountStateUsesStablePrimaryPolicy(t *testing.T) {
	group1, group2 := "1", "2"
	schedulable, rate := true, "1"
	primary := &candidate{
		account: business.RoutingAccount{ID: "41", GroupName: "group-a", GroupID: &group1, Schedulable: &schedulable, Metadata: map[string]any{}},
		health:  Health{SampleCount: 1, Fatal: true}, state: "healthy", schedulable: true, rateText: &rate,
	}
	secondary := &candidate{
		account: business.RoutingAccount{ID: "41", GroupName: "group-b", GroupID: &group2, Schedulable: &schedulable, Metadata: map[string]any{}},
		health:  Health{SampleCount: 1, Fatal: true}, state: "fuse_pending", schedulable: false, fuseKind: "hard", rateText: &rate,
	}
	primary.rate, secondary.rate = big.NewRat(1, 1), big.NewRat(1, 1)
	alignAccountStateToPrimary(map[string][]*candidate{"41": {secondary, primary}}, map[string]engineConfig{
		"group-a": {breakerEnabled: false, degradeEnabled: false},
		"group-b": {breakerEnabled: true, hardFatal: true, degradeEnabled: false},
	}, map[string]business.PreviousRoutingDecision{}, time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC))
	if primary.state != "healthy" || secondary.state != "healthy" || !primary.schedulable || !secondary.schedulable {
		t.Fatalf("非主分组覆盖策略错误决定了账号级熔断：primary=%#v secondary=%#v", primary, secondary)
	}
}

func TestFusedAccountKeepsPriorityPlacementButNotLoadOrScaling(t *testing.T) {
	priority, concurrency := int64(20), int64(10)
	item := &candidate{account: business.RoutingAccount{ID: "41", GroupName: "codex", Priority: &priority, Concurrency: &concurrency}, state: "fused", weight: 0}
	config := engineConfig{weightsEnabled: true, weightBudget: 400, minLoadFactor: 1, maxLoadFactor: 100, scalingEnabled: true, scalingGlobalMax: 20, scalingMin: 3, scalingMax: 250, scalingUpRatio: .5, scalingStepUp: 5, scalingStepDown: 5}
	assignAccountPlacements(map[string][]*candidate{"codex": {item}}, map[string]engineConfig{"codex": config}, map[string][]*candidate{"41": {item}})
	if item.desiredPriority == nil || item.desiredLoad != nil || item.desiredConcurrency != nil {
		t.Fatalf("熔断账号 placement 与 Guardian 不一致：%#v", item)
	}
}

func TestStrategyQualityUsesNormalizedGroupTerms(t *testing.T) {
	cheapRate, _ := new(big.Rat).SetString("0.2")
	expensiveRate, _ := new(big.Rat).SetString("1")
	slow, fast := 10_000.0, 500.0
	config := engineConfig{priceExp: 1, speedExp: 1, balancedPriceRatio: .5, gateFloor: 40}
	cheap := &candidate{rate: cheapRate, health: Health{HealthScore: 100, P95MS: &slow}, strategy: "balanced", state: "healthy"}
	fastCandidate := &candidate{rate: expensiveRate, health: Health{HealthScore: 100, P95MS: &fast}, strategy: "balanced", state: "healthy"}
	benchmark := strategyScoreBenchmark([]*candidate{cheap, fastCandidate}, config)
	cheapQuality := strategyQuality(cheap, config, benchmark)
	fastQuality := strategyQuality(fastCandidate, config, benchmark)
	if math.Abs(cheapQuality-.525) > 0.0001 || math.Abs(fastQuality-.6) > 0.0001 || cheapQuality >= fastQuality {
		t.Fatalf("权重没有使用组内相对价格与速度：cheap=%v fast=%v", cheapQuality, fastQuality)
	}
}

func TestStrategyQualityFallsBackFromP95ToP50(t *testing.T) {
	rate := big.NewRat(1, 1)
	p50 := 250.0
	config := engineConfig{priceExp: 1, speedExp: 1, balancedPriceRatio: 0, gateFloor: 40}
	item := &candidate{
		rate: rate, health: Health{HealthScore: 100, P50MS: &p50}, strategy: "speed_first", state: "healthy",
	}
	benchmark := strategyScoreBenchmark([]*candidate{item}, config)
	if quality := strategyQuality(item, config, benchmark); math.Abs(quality-1) > 0.0001 {
		t.Fatalf("speed score did not use P50 when P95 was absent: %v", quality)
	}
}

func TestCostWallStopsSchedulingWhenEveryManagedMembershipIsAboveWall(t *testing.T) {
	policy := routingPolicy()
	schedulable := true
	groupRate, costWall := "2", "1"
	repository := &routingRepositoryStub{
		policy: policy,
		accounts: []business.RoutingAccount{{
			ID: "41", Name: "above-wall", GroupName: "codex", GroupRate: &groupRate,
			GroupCostWall: &costWall, Schedulable: &schedulable, Metadata: map[string]any{},
		}},
		samples: []business.RoutingSample{{
			AccountID: "41", GroupName: "codex", Result: "通过", Source: "traffic",
			ObservedAt: "2026-08-28T00:00:00Z", Payload: map[string]any{"status_code": int64(200)},
		}},
	}

	result, err := NewService(repository).Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	decision := repository.decisions[0]
	if decision.State != "cost_blocked" || decision.Schedulable == nil || *decision.Schedulable {
		t.Fatalf("above-wall account was not removed from scheduling: %#v", decision)
	}
	desiredLoad, desiredLoadOK := decision.Payload["desired_load_factor"].(*string)
	if decision.Payload["weight"] != float64(0) || !desiredLoadOK || desiredLoad != nil {
		t.Fatalf("above-wall account retained routing allocation: %#v", decision.Payload)
	}
	target := result.AccountTargets["41"]
	if target.Schedulable == nil || *target.Schedulable || target.DesiredHealth != "cost_blocked" {
		t.Fatalf("above-wall account target=%#v", target)
	}
}

func TestCostWallKeepsMultiGroupAccountWhenAnyManagedMembershipIsWithinWall(t *testing.T) {
	policy := routingPolicy()
	schedulable := false
	rate, lowWall, highWall := "1", "0.5", "2"
	group1, group2 := "1", "2"
	repository := &routingRepositoryStub{
		policy: policy,
		accounts: []business.RoutingAccount{
			{ID: "41", Name: "mixed-wall", GroupName: "group-a", GroupID: &group1, GroupRate: &rate, GroupCostWall: &lowWall, Schedulable: &schedulable, EffectiveState: "cost_blocked", Metadata: map[string]any{}},
			{ID: "41", Name: "mixed-wall", GroupName: "group-b", GroupID: &group2, GroupRate: &rate, GroupCostWall: &highWall, Schedulable: &schedulable, EffectiveState: "cost_blocked", Metadata: map[string]any{}},
		},
		samples: []business.RoutingSample{{
			AccountID: "41", GroupName: "group-a", Result: "通过", Source: "traffic",
			ObservedAt: "2026-08-28T00:00:00Z", Payload: map[string]any{"status_code": int64(200)},
		}},
	}

	result, err := NewService(repository).Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, decision := range repository.decisions {
		if decision.State != "healthy" || decision.Schedulable == nil || !*decision.Schedulable {
			t.Fatalf("safe membership did not restore account scheduling: %#v", repository.decisions)
		}
	}
	if target := result.AccountTargets["41"]; target.Schedulable == nil || !*target.Schedulable || target.DesiredHealth != "healthy" {
		t.Fatalf("recovered cost-wall target=%#v", target)
	}
}

func TestUnknownAccountsShareBudgetAndReceivePlacement(t *testing.T) {
	priority, concurrency := int64(20), int64(10)
	first := &candidate{account: business.RoutingAccount{ID: "41", GroupName: "codex", Priority: &priority, Concurrency: &concurrency}, state: "unknown", schedulable: true}
	second := &candidate{account: business.RoutingAccount{ID: "42", GroupName: "codex", Priority: &priority, Concurrency: &concurrency}, state: "unknown", schedulable: true}
	config := engineConfig{weightBudget: 400, gateFloor: 40, priceExp: 1, speedExp: 1, balancedPriceRatio: .5, weightsEnabled: true, minLoadFactor: 1, maxLoadFactor: 100}
	calculateGroupWeights([]*candidate{first, second}, config)
	if first.weight != 200 || second.weight != 200 {
		t.Fatalf("全组无样本时没有平均分配权重：first=%v second=%v", first.weight, second.weight)
	}
	groups := map[string][]*candidate{"codex": {first, second}}
	assignAccountPlacements(groups, map[string]engineConfig{"codex": config}, map[string][]*candidate{"41": {first}, "42": {second}})
	if first.desiredPriority == nil || second.desiredPriority == nil {
		t.Fatalf("无样本账号被错误排除在 placement 外：first=%#v second=%#v", first, second)
	}
}

func TestGroupWeightBudgetIsSharedAndConserved(t *testing.T) {
	rate := big.NewRat(1, 1)
	fastLatency, slowLatency := 500.0, 2_000.0
	fast := &candidate{
		account: business.RoutingAccount{ID: "41"}, rate: rate, schedulable: true,
		health: Health{HealthScore: 100, P95MS: &fastLatency}, state: "healthy", strategy: "speed_first",
	}
	slow := &candidate{
		account: business.RoutingAccount{ID: "42"}, rate: rate, schedulable: true,
		health: Health{HealthScore: 100, P95MS: &slowLatency}, state: "healthy", strategy: "speed_first",
	}
	config := engineConfig{weightBudget: 400, gateFloor: 40, priceExp: 1, speedExp: 1, balancedPriceRatio: .5}

	calculateGroupWeights([]*candidate{fast, slow}, config)

	if math.Abs(fast.weight+slow.weight-400) > 0.0001 {
		t.Fatalf("组内分配后总权重没有守恒：fast=%v slow=%v", fast.weight, slow.weight)
	}
	if fast.weight <= slow.weight || fast.weight >= 400 || slow.weight <= 0 {
		t.Fatalf("总预算没有按组内账号质量共享：fast=%v slow=%v", fast.weight, slow.weight)
	}
}

func TestSpeedFirstUsesPriceAsSecondaryFactor(t *testing.T) {
	cheapRate, expensiveRate := big.NewRat(1, 10), big.NewRat(1, 5)
	latency := 1_000.0
	cheap := &candidate{
		account: business.RoutingAccount{ID: "41"}, rate: cheapRate, schedulable: true,
		health: Health{HealthScore: 100, P95MS: &latency}, state: "healthy", strategy: "speed_first",
	}
	expensive := &candidate{
		account: business.RoutingAccount{ID: "42"}, rate: expensiveRate, schedulable: true,
		health: Health{HealthScore: 100, P95MS: &latency}, state: "healthy", strategy: "speed_first",
	}
	config := engineConfig{weightBudget: 400, gateFloor: 40, priceExp: 1, speedExp: 1, balancedPriceRatio: .5}

	calculateGroupWeights([]*candidate{cheap, expensive}, config)

	if cheap.weight <= expensive.weight {
		t.Fatalf("速度相同时速度优先没有把价格作为次要因素：cheap=%v expensive=%v", cheap.weight, expensive.weight)
	}
}

func TestSpeedFirstStillPrefersMeaningfullyFasterAccount(t *testing.T) {
	cheapRate, expensiveRate := big.NewRat(1, 10), big.NewRat(1, 5)
	fastLatency, slowLatency := 1_000.0, 2_000.0
	fast := &candidate{
		account: business.RoutingAccount{ID: "41"}, rate: expensiveRate, schedulable: true,
		health: Health{HealthScore: 100, P95MS: &fastLatency}, state: "healthy", strategy: "speed_first",
	}
	cheap := &candidate{
		account: business.RoutingAccount{ID: "42"}, rate: cheapRate, schedulable: true,
		health: Health{HealthScore: 100, P95MS: &slowLatency}, state: "healthy", strategy: "speed_first",
	}
	config := engineConfig{weightBudget: 400, gateFloor: 40, priceExp: 1, speedExp: 1, balancedPriceRatio: .5}

	calculateGroupWeights([]*candidate{fast, cheap}, config)

	if fast.weight <= cheap.weight {
		t.Fatalf("速度优先没有保持速度主导：fast=%v cheap=%v", fast.weight, cheap.weight)
	}
}

func TestUnschedulableAccountDoesNotConsumeGroupWeightBudget(t *testing.T) {
	rate := big.NewRat(1, 1)
	latency := 1_000.0
	available := &candidate{
		account: business.RoutingAccount{ID: "41"}, rate: rate, schedulable: true,
		health: Health{HealthScore: 100, P95MS: &latency}, state: "healthy", strategy: "balanced",
	}
	unavailable := &candidate{
		account: business.RoutingAccount{ID: "42"}, rate: rate, schedulable: false,
		health: Health{HealthScore: 100, P95MS: &latency}, state: "healthy", strategy: "balanced",
	}
	config := engineConfig{weightBudget: 400, gateFloor: 40, priceExp: 1, speedExp: 1, balancedPriceRatio: .5}

	calculateGroupWeights([]*candidate{available, unavailable}, config)

	if available.weight != 400 || unavailable.weight != 0 {
		t.Fatalf("不可调度账号占用了组内权重预算：available=%v unavailable=%v", available.weight, unavailable.weight)
	}
}

func TestZeroQualityBudgetIsSharedOnlyBySchedulableMembers(t *testing.T) {
	first := &candidate{account: business.RoutingAccount{ID: "41"}, state: "unknown", schedulable: true}
	second := &candidate{account: business.RoutingAccount{ID: "42"}, state: "unknown", schedulable: true}
	fused := &candidate{account: business.RoutingAccount{ID: "43"}, state: "fused"}
	config := engineConfig{weightBudget: 300, gateFloor: 40, priceExp: 1, speedExp: 1, balancedPriceRatio: .5}

	calculateGroupWeights([]*candidate{first, second, fused}, config)

	if first.weight != 150 || second.weight != 150 || fused.weight != 0 {
		t.Fatalf("全组原始权重为零时没有只在可调度成员间分配：first=%v second=%v fused=%v", first.weight, second.weight, fused.weight)
	}
}

func TestSurvivorPlacementUsesMinimumLoadFactor(t *testing.T) {
	priority := int64(20)
	item := &candidate{account: business.RoutingAccount{ID: "41", GroupName: "codex", Priority: &priority}, state: "survivor", weight: 400}
	config := engineConfig{weightsEnabled: true, weightBudget: 400, minLoadFactor: 3, maxLoadFactor: 100, degradePriorityStep: 10}

	assignAccountPlacements(map[string][]*candidate{"codex": {item}}, map[string]engineConfig{"codex": config}, map[string][]*candidate{"41": {item}})

	if item.desiredLoad == nil || *item.desiredLoad != "3" {
		t.Fatalf("保底账号没有压到最低负载因子：%#v", item.desiredLoad)
	}
}

func TestScalingConsumesOnlyActualIncreaseAfterPerAccountClamp(t *testing.T) {
	firstCurrent, secondCurrent := int64(99), int64(99)
	firstRank, secondRank := 1, 2
	first := &candidate{account: business.RoutingAccount{ID: "41", Concurrency: &firstCurrent}, state: "healthy", rank: &firstRank}
	second := &candidate{account: business.RoutingAccount{ID: "42", Concurrency: &secondCurrent}, state: "healthy", rank: &secondRank}
	config := engineConfig{
		scalingEnabled: true, scalingGlobalMax: 200, scalingMin: 3, scalingMax: 100,
		scalingUpRatio: .8, scalingStepUp: 5, scalingStepDown: 5,
	}

	applyScaling([]*candidate{first, second}, config)

	if first.desiredConcurrency == nil || second.desiredConcurrency == nil ||
		*first.desiredConcurrency != 100 || *second.desiredConcurrency != 100 {
		t.Fatalf("单账号上限截断后虚耗了全局余量：first=%v second=%v", first.desiredConcurrency, second.desiredConcurrency)
	}
}

func TestScalingKeepsPositiveConcurrencyBelowConfiguredMinimumAsCurrent(t *testing.T) {
	current := int64(1)
	rank := 1
	item := &candidate{
		account: business.RoutingAccount{ID: "41", Concurrency: &current},
		state:   "degraded", rank: &rank,
	}
	config := engineConfig{
		scalingEnabled: true, scalingGlobalMax: 100, scalingMin: 3, scalingMax: 50,
		scalingUpRatio: .8, scalingStepUp: 5, scalingStepDown: 5,
	}

	applyScaling([]*candidate{item}, config)

	if item.desiredConcurrency == nil || *item.desiredConcurrency != 3 {
		t.Fatalf("参考项目以远端正并发为当前值后再做边界截断，得到 %v", item.desiredConcurrency)
	}
}

func TestPlacementPriorityUsesCapturedBaseline(t *testing.T) {
	currentA, currentB := int64(1000), int64(2000)
	baselineA, baselineB := int64(10), int64(20)
	concurrency := int64(10)
	first := &candidate{account: business.RoutingAccount{ID: "41", GroupName: "codex", Priority: &currentA, BaselinePriority: &baselineA, Concurrency: &concurrency}, state: "healthy", schedulable: true, weight: 200}
	second := &candidate{account: business.RoutingAccount{ID: "42", GroupName: "codex", Priority: &currentB, BaselinePriority: &baselineB, Concurrency: &concurrency}, state: "healthy", schedulable: true, weight: 100}
	config := engineConfig{weightBudget: 400, weightsEnabled: true, minLoadFactor: 1, maxLoadFactor: 100}
	groups := map[string][]*candidate{"codex": {first, second}}
	assignAccountPlacements(groups, map[string]engineConfig{"codex": config}, map[string][]*candidate{"41": {first}, "42": {second}})
	if first.desiredPriority == nil || second.desiredPriority == nil || *first.desiredPriority != 19 || *second.desiredPriority != 20 {
		t.Fatalf("优先级基准没有使用接管前基线：first=%v second=%v", first.desiredPriority, second.desiredPriority)
	}
}

func TestInitialStateSeparatesUnknownSchedulingFromDisabledAccount(t *testing.T) {
	configured, err := parseEngineConfig(routingPolicy())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	unknown := &candidate{account: business.RoutingAccount{ID: "41", GroupName: "codex", Metadata: map[string]any{}}}
	applyInitialState(unknown, configured, business.PreviousRoutingDecision{}, now)
	if unknown.state != "unknown" || unknown.schedulable {
		t.Fatalf("unknown scheduling state was misclassified: %#v", unknown)
	}
	disabled := &candidate{account: business.RoutingAccount{ID: "42", GroupName: "codex", Metadata: map[string]any{"status": "inactive"}}}
	applyInitialState(disabled, configured, business.PreviousRoutingDecision{}, now)
	if disabled.state != "disabled" || disabled.schedulable {
		t.Fatalf("disabled account was misclassified: %#v", disabled)
	}
}

func TestCalculateKeepsUnprobedAccountUnknownButSchedulable(t *testing.T) {
	policy := routingPolicy()
	schedulable, multiplier := true, "1"
	repository := &routingRepositoryStub{
		policy: policy,
		accounts: []business.RoutingAccount{{
			ID: "41", Name: "unprobed", GroupName: "codex", Schedulable: &schedulable,
			Multiplier: &multiplier, Metadata: map[string]any{},
		}},
	}

	result, err := NewService(repository).Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(repository.decisions) != 1 || repository.decisions[0].State != "unknown" ||
		repository.decisions[0].Schedulable == nil || !*repository.decisions[0].Schedulable {
		t.Fatalf("unprobed account must stay unknown without being removed from traffic: %#v", repository.decisions)
	}
	if result.AccountTargets["41"].Schedulable == nil || !*result.AccountTargets["41"].Schedulable {
		t.Fatalf("unprobed schedulable account was removed from the available pool: %#v", result.AccountTargets["41"])
	}
}

func TestHealthyButManuallyUnschedulableAccountStaysClosed(t *testing.T) {
	configured, err := parseEngineConfig(routingPolicy())
	if err != nil {
		t.Fatal(err)
	}
	configured.manageAllAccounts = false
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	schedulable := false
	rate := "1"
	item := &candidate{
		account: business.RoutingAccount{ID: "41", GroupName: "codex", Schedulable: &schedulable, Metadata: map[string]any{}},
		health:  Health{SampleCount: 1, HealthScore: 100}, state: "healthy", rateText: &rate,
	}
	item.rate = big.NewRat(1, 1)
	applyInitialState(item, configured, business.PreviousRoutingDecision{}, now)
	if item.state != "healthy" || item.schedulable {
		t.Fatalf("人工关闭调度的健康账号被自动重开：%#v", item)
	}

	item.health.HealthScore = 50
	applyInitialState(item, configured, business.PreviousRoutingDecision{}, now)
	if item.state != "degraded" || item.schedulable {
		t.Fatalf("降级分支不应覆盖人工关闭状态：%#v", item)
	}
}

func TestManageAllAccountsReopensHealthyAndFirstSeenAccounts(t *testing.T) {
	configured, err := parseEngineConfig(routingPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if !configured.manageAllAccounts {
		t.Fatal("托管全部账号必须默认开启")
	}
	schedulable := false
	rate := "1"
	for _, sampleCount := range []int{0, 1} {
		item := &candidate{
			account: business.RoutingAccount{ID: "41", GroupName: "codex", Schedulable: &schedulable, Metadata: map[string]any{}},
			health:  Health{SampleCount: sampleCount, HealthScore: 100}, state: "healthy", rateText: &rate,
		}
		item.rate = big.NewRat(1, 1)
		applyInitialState(item, configured, business.PreviousRoutingDecision{}, time.Now().UTC())
		if !item.schedulable {
			t.Fatalf("托管账号没有进入调度：sample_count=%d item=%#v", sampleCount, item)
		}
	}
}

func TestDisabledManageAllAccountsAbandonsExternallyModifiedAccount(t *testing.T) {
	policy := routingPolicy()
	policy["scope"].(map[string]any)["manage_all_accounts"] = false
	schedulable, managedSchedulable, multiplier := false, true, "1"
	repository := &routingRepositoryStub{policy: policy, accounts: []business.RoutingAccount{{
		ID: "41", Name: "externally-closed", GroupName: "codex", Schedulable: &schedulable,
		ManagedSchedulable: &managedSchedulable, Multiplier: &multiplier, Metadata: map[string]any{},
	}}}
	result, err := NewService(repository).Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	target, found := result.AccountTargets["41"]
	if !found || !target.AbandonControl || target.ReleaseControl || target.DesiredHealth != "external_control" {
		t.Fatalf("人工改动后没有停止托管：%#v", result.AccountTargets)
	}
	if len(repository.decisions) != 0 {
		t.Fatalf("停止托管账号仍生成自动调度决策：%#v", repository.decisions)
	}
}

func TestFusedRecoveryUsesStableStateSinceInsteadOfDecisionRefreshTime(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	configured, err := parseEngineConfig(routingPolicy())
	if err != nil {
		t.Fatal(err)
	}
	configured.fusedCooldown = 3 * time.Minute
	configured.recoveryTarget = 75
	configured.recoverySuccesses = 1
	configured.recoveryHold = time.Minute
	rate := "1"
	schedulable := false
	item := &candidate{
		account:  business.RoutingAccount{ID: "41", GroupName: "codex", Schedulable: &schedulable, EffectiveState: "fused", Metadata: map[string]any{}},
		health:   Health{HealthScore: 100, RecoveryPassStreak: 1},
		rows:     []business.RoutingSample{{ObservedAt: now.Add(-2 * time.Minute).Format(time.RFC3339Nano)}},
		rateText: &rate,
	}
	item.rate, _ = new(big.Rat).SetString(rate)
	previous := business.PreviousRoutingDecision{
		State: " FUSED ", UpdatedAt: now.Add(-10 * time.Second).Format(time.RFC3339Nano),
		Payload: map[string]any{"state_since": now.Add(-10 * time.Minute).Format(time.RFC3339Nano)},
	}

	applyInitialState(item, configured, previous, now)
	if item.state != "healthy" || !item.schedulable {
		t.Fatalf("每轮刷新决策时间不应阻止已满足条件的账号回池：%#v", item)
	}
}

func TestFusedRecoveryUsesPersistedCooldownDeadline(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	configured, err := parseEngineConfig(routingPolicy())
	if err != nil {
		t.Fatal(err)
	}
	configured.fusedCooldown = time.Second
	configured.recoveryTarget = 75
	configured.recoverySuccesses = 1
	configured.recoveryHold = 0
	rate, schedulable := "1", false
	item := &candidate{
		account: business.RoutingAccount{ID: "41", GroupName: "codex", Schedulable: &schedulable, EffectiveState: "fused", Metadata: map[string]any{}},
		health:  Health{HealthScore: 100, RecoveryPassStreak: 1},
		rows:    []business.RoutingSample{{ObservedAt: now.Add(-time.Minute).Format(time.RFC3339Nano)}}, rateText: &rate,
	}
	item.rate, _ = new(big.Rat).SetString(rate)
	deadline := now.Add(time.Minute)
	previous := business.PreviousRoutingDecision{State: "fused", Payload: map[string]any{
		"state_since": now.Add(-time.Hour).Format(time.RFC3339Nano),
		"fused_until": deadline.Format(time.RFC3339Nano),
	}}

	applyInitialState(item, configured, previous, now)

	if item.state != "fused" || !item.fusedUntil.Equal(deadline) {
		t.Fatalf("修改策略不能重算已经生效的熔断截止时间：state=%s deadline=%s", item.state, item.fusedUntil)
	}
}

func TestDesiredFuseDoesNotEnterRecoveryBeforeItIsEffective(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	configured, err := parseEngineConfig(routingPolicy())
	if err != nil {
		t.Fatal(err)
	}
	rate, schedulable := "1", true
	item := &candidate{
		account: business.RoutingAccount{ID: "41", GroupName: "codex", Schedulable: &schedulable, EffectiveState: "healthy", Metadata: map[string]any{}},
		health:  Health{HealthScore: 100, RecoveryPassStreak: 10}, rateText: &rate,
		state: "healthy", schedulable: true,
	}
	item.rate, _ = new(big.Rat).SetString(rate)
	previous := business.PreviousRoutingDecision{State: "fused", Payload: map[string]any{"state_since": now.Add(-time.Hour).Format(time.RFC3339Nano)}}
	applyInitialState(item, configured, previous, now)
	if item.state == "fused" || !item.schedulable {
		t.Fatalf("未生效的期望熔断被当成实际熔断：%#v", item)
	}
}

func TestFusedRoutingStateRecognizesOpenBreakerStates(t *testing.T) {
	for _, state := range []string{"fused", "hard_open", "soft_open", " FUSED "} {
		if !fusedRoutingState(state) {
			t.Fatalf("state %q should count as fused", state)
		}
	}
	for _, state := range []string{"", "healthy", "degraded", "survivor"} {
		if fusedRoutingState(state) {
			t.Fatalf("state %q should not count as fused", state)
		}
	}
}

func TestHardFatalStillPreservesMinimumPool(t *testing.T) {
	config := engineConfig{minPool: 1, minPoolScore: 0, maxSwitch: 10, degradeEnabled: true}
	first := fuseTestCandidate("1", "codex", "hard", 0)
	second := fuseTestCandidate("2", "codex", "hard", 0)
	groups := map[string][]*candidate{"codex": {first, second}}
	byAccount := map[string][]*candidate{"1": {first}, "2": {second}}

	applyFuseBudgets(groups, map[string]engineConfig{"codex": config}, byAccount, time.Time{})

	states := map[string]string{first.state: first.account.ID, second.state: second.account.ID}
	if states["fused"] == "" || states["survivor"] == "" {
		t.Fatalf("hard failures must leave one protected account: first=%#v second=%#v", first, second)
	}
}

func TestRateLimitedAccountIsNeverSoftFused(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	schedulable, rate := true, "1"
	item := &candidate{
		account: business.RoutingAccount{
			ID: "41", GroupName: "codex", Schedulable: &schedulable,
			Metadata: map[string]any{"rate_limit_reset_at": now.Add(time.Hour).Format(time.RFC3339Nano)},
		},
		health: Health{HealthScore: 20, SampleCount: 5, GatewayFailures: 5, LatestEvent: EventGateway},
		rows:   []business.RoutingSample{{Result: "失败", Payload: map[string]any{"status_code": 503}}},
		state:  "healthy", schedulable: true, rateText: &rate,
	}
	item.rate = big.NewRat(1, 1)
	config := engineConfig{breakerEnabled: true, httpWindow: 5, httpFailures: 3, httpScoreBelow: 60, latencyWindow: 10, latencyOccurrences: 5, instantCodes: map[int]struct{}{503: {}}, degradeEnabled: false}
	applyInitialState(item, config, business.PreviousRoutingDecision{}, now)
	if item.state == "fuse_pending" || item.state == "fused" || !item.schedulable {
		t.Fatalf("Sub2API 限流窗口内的账号被错误软熔断：%#v", item)
	}
}

func TestInstantStatusCodeFallsBackToErrorText(t *testing.T) {
	rows := []business.RoutingSample{{
		Result: "失败", FailureReason: "upstream returned 401: Unauthorized", Payload: map[string]any{},
	}}
	if !latestStatusIn(rows, map[int]struct{}{401: {}}) {
		t.Fatal("错误文本中的状态码没有触发立即熔断")
	}
}

func TestBlockedAccountDoesNotCountTowardMinimumPool(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	config := engineConfig{minPool: 1, minPoolScore: 0, maxSwitch: 10, degradeEnabled: true}
	trigger := fuseTestCandidate("1", "codex", "soft", 10)
	blocked := healthyTestCandidate("2", "codex", 100)
	schedulable := true
	blocked.account.Schedulable = &schedulable
	blocked.account.Metadata = map[string]any{"overload_until": now.Add(time.Hour).Format(time.RFC3339Nano)}
	groups := map[string][]*candidate{"codex": {trigger, blocked}}
	byAccount := map[string][]*candidate{"1": {trigger}, "2": {blocked}}
	applyFuseBudgets(groups, map[string]engineConfig{"codex": config}, byAccount, now)
	if trigger.state != "survivor" || !trigger.schedulable {
		t.Fatalf("处于 Sub2API 阻塞窗口的账号错误充当了保底容量：trigger=%#v blocked=%#v", trigger, blocked)
	}
}

func TestUnknownSchedulableAccountCountsTowardMinimumPool(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	config := engineConfig{minPool: 1, minPoolScore: 3, maxSwitch: 10, degradeEnabled: true}
	trigger := fuseTestCandidate("1", "codex", "soft", 10)
	unknown := healthyTestCandidate("2", "codex", 0)
	unknown.state = "unknown"
	unknown.health = Health{}
	schedulable := true
	unknown.account.Schedulable = &schedulable
	groups := map[string][]*candidate{"codex": {trigger, unknown}}
	byAccount := map[string][]*candidate{"1": {trigger}, "2": {unknown}}
	applyFuseBudgets(groups, map[string]engineConfig{"codex": config}, byAccount, now)
	if trigger.state != "fused" || trigger.schedulable {
		t.Fatalf("网站实际可调度的无样本账号没有计入保底池：trigger=%#v unknown=%#v", trigger, unknown)
	}
}

func TestAccountLevelFuseChecksEveryMembershipMinimumPool(t *testing.T) {
	config := engineConfig{minPool: 1, minPoolScore: 0, maxSwitch: 10, degradeEnabled: true}
	trigger := fuseTestCandidate("1", "group-a", "soft", 10)
	spare := healthyTestCandidate("2", "group-a", 100)
	otherMembership := healthyTestCandidate("1", "group-b", 10)
	groups := map[string][]*candidate{
		"group-a": {trigger, spare},
		"group-b": {otherMembership},
	}
	byAccount := map[string][]*candidate{
		"1": {trigger, otherMembership},
		"2": {spare},
	}

	applyFuseBudgets(groups, map[string]engineConfig{"group-a": config, "group-b": config}, byAccount, time.Time{})

	if trigger.state != "survivor" || !trigger.schedulable || otherMembership.state != "healthy" {
		t.Fatalf("cross-group minimum pool was not preserved: trigger=%#v membership=%#v", trigger, otherMembership)
	}
}

func TestAccountLevelSoftFuseConsumesEveryMembershipSwitchBudget(t *testing.T) {
	config := engineConfig{minPool: 1, minPoolScore: 0, maxSwitch: 1, degradeEnabled: true}
	firstTrigger := fuseTestCandidate("1", "group-a", "soft", 10)
	firstSpare := healthyTestCandidate("3", "group-a", 100)
	firstMembership := healthyTestCandidate("1", "group-b", 10)
	secondTrigger := fuseTestCandidate("2", "group-b", "soft", 20)
	secondSpare := healthyTestCandidate("4", "group-b", 100)
	groups := map[string][]*candidate{
		"group-a": {firstTrigger, firstSpare},
		"group-b": {firstMembership, secondTrigger, secondSpare},
	}
	byAccount := map[string][]*candidate{
		"1": {firstTrigger, firstMembership},
		"2": {secondTrigger},
		"3": {firstSpare},
		"4": {secondSpare},
	}

	applyFuseBudgets(groups, map[string]engineConfig{"group-a": config, "group-b": config}, byAccount, time.Time{})

	if firstTrigger.state != "fused" || firstMembership.state != "fused" {
		t.Fatalf("first account was not fused across memberships: %#v %#v", firstTrigger, firstMembership)
	}
	if secondTrigger.state != "degraded" || !secondTrigger.schedulable || !strings.Contains(secondTrigger.reason, "group-b") {
		t.Fatalf("second fuse ignored shared switch budget: %#v", secondTrigger)
	}
}

func TestFuseBudgetChecksMembershipEvenWhenAccountIsAlreadyBlockedThere(t *testing.T) {
	config := engineConfig{minPool: 1, minPoolScore: 0, maxSwitch: 1, degradeEnabled: true}
	trigger := fuseTestCandidate("1", "group-a", "soft", 10)
	spare := healthyTestCandidate("2", "group-a", 100)
	blockedMembership := fuseTestCandidate("1", "group-b", "soft", 10)
	blockedMembership.account.Metadata = map[string]any{"overload_until": "2999-01-01T00:00:00Z"}
	groups := map[string][]*candidate{
		"group-a": {trigger, spare},
		"group-b": {blockedMembership},
	}
	byAccount := map[string][]*candidate{
		"1": {trigger, blockedMembership},
		"2": {spare},
	}

	applyFuseBudgets(groups, map[string]engineConfig{"group-a": config, "group-b": config}, byAccount, time.Time{})

	if trigger.state != "survivor" || !trigger.schedulable {
		t.Fatalf("账号级熔断没有检查每个受管分组的保底容量：%#v", trigger)
	}
	if blockedMembership.state != "survivor" || !blockedMembership.schedulable {
		t.Fatalf("账号级保底状态没有同步到所有成员关系：%#v", blockedMembership)
	}
}

func TestCalculateAppliesAccountLevelFuseAcrossManagedGroups(t *testing.T) {
	policy := routingPolicy()
	policy["breaker"].(map[string]any)["min_pool_size"] = int64(1)
	schedulable := true
	multiplier := "1"
	accounts := []business.RoutingAccount{
		{ID: "1", Name: "failed", GroupName: "group-a", Schedulable: &schedulable, Multiplier: &multiplier, Metadata: map[string]any{}},
		{ID: "1", Name: "failed", GroupName: "group-b", Schedulable: &schedulable, Multiplier: &multiplier, Metadata: map[string]any{}},
		{ID: "2", Name: "spare-a", GroupName: "group-a", Schedulable: &schedulable, Multiplier: &multiplier, Metadata: map[string]any{}},
		{ID: "3", Name: "spare-b", GroupName: "group-b", Schedulable: &schedulable, Multiplier: &multiplier, Metadata: map[string]any{}},
	}
	samples := []business.RoutingSample{
		{AccountID: "1", GroupName: "group-a", Result: "失败", FailureReason: "unauthorized", Source: "traffic", ObservedAt: "2026-08-27T11:59:00Z", Payload: map[string]any{"status_code": int64(401)}},
		{AccountID: "1", GroupName: "group-b", Result: "通过", Source: "traffic", ObservedAt: "2026-08-27T11:59:00Z", Payload: map[string]any{"status_code": int64(200)}},
		{AccountID: "2", GroupName: "group-a", Result: "通过", Source: "traffic", ObservedAt: "2026-08-27T11:59:00Z", Payload: map[string]any{"status_code": int64(200)}},
		{AccountID: "3", GroupName: "group-b", Result: "通过", Source: "traffic", ObservedAt: "2026-08-27T11:59:00Z", Payload: map[string]any{"status_code": int64(200)}},
	}
	repository := &routingRepositoryStub{policy: policy, accounts: accounts, samples: samples}
	service := NewService(repository)
	service.now = func() time.Time { return time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC) }

	result, err := service.Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	decision, found := result.AccountDecisions["1"]
	if !found || decision.RoutingState != "fused" || decision.Schedulable {
		t.Fatalf("account-level fuse missing: %#v", result.AccountDecisions)
	}
	target := result.AccountTargets["1"]
	if target.Schedulable == nil || *target.Schedulable || target.DesiredHealth != "fused" {
		t.Fatalf("aggregated account target conflicts with group decisions: %#v", target)
	}
}

func TestUnknownErrorsDoNotCountAsGatewayFailures(t *testing.T) {
	health := Health{Events: []Event{EventUnknown, EventGateway, EventProbeFailed, EventRateLimited}}
	if got := recentHTTPFailures(health, 4); got != 2 {
		t.Fatalf("gateway failures=%d want=2", got)
	}
}

func fuseTestCandidate(accountID, groupName, kind string, score float64) *candidate {
	schedulable := true
	return &candidate{
		account: business.RoutingAccount{ID: accountID, GroupName: groupName, Schedulable: &schedulable},
		health:  Health{HealthScore: score, SampleCount: 1},
		state:   "fuse_pending", reason: "触发熔断", fuseKind: kind,
	}
}

func healthyTestCandidate(accountID, groupName string, score float64) *candidate {
	schedulable := true
	return &candidate{
		account: business.RoutingAccount{ID: accountID, GroupName: groupName, Schedulable: &schedulable},
		health:  Health{HealthScore: score, SampleCount: 1},
		state:   "healthy", reason: "健康", schedulable: true,
	}
}

func TestDeadbandUsesGuardianSharedWriteCooldown(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	load := "20"
	concurrency := int64(10)
	item := &candidate{
		account:     business.RoutingAccount{ID: "41", GroupName: "codex"},
		desiredLoad: &load, desiredConcurrency: &concurrency,
	}
	previous := map[string]business.PreviousRoutingDecision{
		routingKey("41", "codex"): {
			AccountID: "41", GroupName: "codex", UpdatedAt: now.Format(time.RFC3339Nano), Payload: map[string]any{},
			LastApplyAt: now.Add(-10 * time.Second),
		},
	}
	configured := engineConfig{changeThreshold: big.NewRat(0, 1), cooldown: time.Minute, scalingCooldown: 5 * time.Minute}

	applyDeadband([]*candidate{item}, previous, configured, now)
	if !item.writeCooldown || !item.scalingCooldown {
		t.Fatalf("任意成功写回应同时触发负载与并发冷却：%#v", item)
	}
	if item.desiredLoad != nil || item.desiredConcurrency != nil {
		t.Fatalf("共享冷却期内不应继续发布负载或并发目标：%#v", item)
	}
}

func TestSharedWriteTimeUsesIndependentCooldownDurations(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	load := "20"
	concurrency := int64(10)
	item := &candidate{
		account:     business.RoutingAccount{ID: "41", GroupName: "codex"},
		desiredLoad: &load, desiredConcurrency: &concurrency,
	}
	previous := map[string]business.PreviousRoutingDecision{
		routingKey("41", "codex"): {
			AccountID: "41", GroupName: "codex", UpdatedAt: now.Format(time.RFC3339Nano), Payload: map[string]any{},
			LastApplyAt: now.Add(-2 * time.Minute),
		},
	}
	configured := engineConfig{changeThreshold: big.NewRat(0, 1), cooldown: time.Minute, scalingCooldown: 5 * time.Minute}

	applyDeadband([]*candidate{item}, previous, configured, now)
	if item.writeCooldown || !item.scalingCooldown {
		t.Fatalf("cooldown durations must be evaluated independently: %#v", item)
	}
	if item.desiredLoad == nil || *item.desiredLoad != "20" || item.desiredConcurrency != nil {
		t.Fatalf("only the scaling target must be suppressed: %#v", item)
	}
}

func TestLoadFactorDeadbandIsAppliedDuringCalculation(t *testing.T) {
	threshold := big.NewRat(1, 10)
	current := "100"
	account := business.RoutingAccount{LoadFactor: &current}
	if !loadFactorWithinThreshold(account, "109", threshold) {
		t.Fatal("9% change must remain inside Guardian deadband")
	}
	if loadFactorWithinThreshold(account, "110", threshold) {
		t.Fatal("change exactly at the threshold must be eligible for writeback")
	}
	concurrency := int64(20)
	account = business.RoutingAccount{Concurrency: &concurrency}
	if !loadFactorWithinThreshold(account, "21", threshold) {
		t.Fatal("missing load_factor must fall back to concurrency like Sub2API")
	}
	item := &candidate{account: business.RoutingAccount{ID: "41", GroupName: "codex", LoadFactor: &current}}
	desired := "105"
	item.desiredLoad = &desired
	applyDeadband([]*candidate{item}, map[string]business.PreviousRoutingDecision{}, engineConfig{changeThreshold: threshold}, time.Now())
	if item.desiredLoad == nil || *item.desiredLoad != "100" {
		t.Fatalf("suppressed target must retain current load factor: %#v", item.desiredLoad)
	}
}

func TestRoutingCalculationDoesNotCompareAgainstPreviousDesiredLoad(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	load := "105"
	item := &candidate{
		account: business.RoutingAccount{ID: "41", GroupName: "codex"}, desiredLoad: &load,
	}
	previous := map[string]business.PreviousRoutingDecision{
		routingKey("41", "codex"): {
			AccountID: "41", GroupName: "codex", Payload: map[string]any{"desired_load_factor": "100"},
		},
	}
	applyDeadband([]*candidate{item}, previous, engineConfig{
		changeThreshold: big.NewRat(1, 10), cooldown: time.Minute, scalingCooldown: time.Minute,
	}, now)
	if item.desiredLoad == nil || *item.desiredLoad != "105" {
		t.Fatalf("上一轮期望值不应覆盖本轮目标：%v", item.desiredLoad)
	}
}

func TestCleanupQueues401DeleteAfterAllGuardsPass(t *testing.T) {
	policy := cleanupRoutingPolicy("delete", 0, true)
	repository := cleanupRoutingRepository(policy, true)
	service := NewService(repository)
	service.now = func() time.Time { return time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC) }

	result, err := service.Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	first := result.AccountTargets["41"]
	second := result.AccountTargets["42"]
	if first.CleanupAction == nil || *first.CleanupAction != "delete" {
		t.Fatalf("第一个 401 账号没有进入删除队列：%#v", first)
	}
	if second.CleanupAction != nil {
		t.Fatalf("每轮上限为 1 时不应同时处置第二个账号：%#v", second)
	}
	if len(repository.runtimeEvents) == 0 {
		t.Fatal("自动处置判定必须留下可诊断事件")
	}
}

func TestCleanupStartsIndependentObservationWindow(t *testing.T) {
	policy := cleanupRoutingPolicy("delete", 30, false)
	repository := cleanupRoutingRepository(policy, true)
	repository.samples[1].Result = "通过"
	repository.samples[1].FailureReason = ""
	repository.samples[1].Payload = map[string]any{"status_code": int64(200)}
	service := NewService(repository)
	service.now = func() time.Time { return time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC) }

	result, err := service.Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.AccountTargets["41"].CleanupAction != nil {
		t.Fatal("首次满足条件时不能绕过 30 分钟观察期")
	}
	if len(repository.cleanupWrites) != 1 || repository.cleanupWrites[0].EligibleSince == nil {
		t.Fatalf("首次满足条件时没有保存独立观察起点：%#v", repository.cleanupWrites)
	}
}

func TestRoutingStateTransitionsCreateDiagnosticEventsOnlyWhenStateChanges(t *testing.T) {
	groupID := "7"
	now := time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC)
	item := &candidate{
		account: business.RoutingAccount{ID: "41", Name: "alpha", GroupName: "codex", GroupID: &groupID},
		state:   "fused", reason: "连续认证失败", schedulable: false,
		health: Health{HealthScore: 12.5}, stateSince: now,
	}
	previous := map[string]business.PreviousRoutingDecision{
		routingKey("41", "codex"): {AccountID: "41", GroupName: "codex", State: "healthy"},
	}
	events := routingStateTransitionEvents(map[string][]*candidate{"41": {item}}, previous)
	if len(events) != 1 || events[0].EventType != "routing.fused" || events[0].Status != "failed" ||
		!strings.Contains(events[0].Summary, "连续认证失败") || events[0].Payload["group_name"] != "codex" {
		t.Fatalf("熔断状态变化没有形成可诊断事件：%#v", events)
	}
	if events := routingStateTransitionEvents(map[string][]*candidate{"41": {item}}, map[string]business.PreviousRoutingDecision{}); len(events) != 1 {
		t.Fatalf("首次计算直接熔断也必须生成事件：%#v", events)
	}
	previous[routingKey("41", "codex")] = business.PreviousRoutingDecision{AccountID: "41", GroupName: "codex", State: "fused"}
	if events := routingStateTransitionEvents(map[string][]*candidate{"41": {item}}, previous); len(events) != 0 {
		t.Fatalf("相同状态不应重复生成事件：%#v", events)
	}
}

func TestCleanupDoesNotTreatQuotaFailureAsAuthenticationFailure(t *testing.T) {
	policy := cleanupRoutingPolicy("delete", 0, false)
	policy["cleanup"].(map[string]any)["trigger_status_codes"] = []any{}
	repository := cleanupRoutingRepository(policy, false)
	repository.samples[0].Payload = map[string]any{}
	repository.samples[0].FailureReason = "insufficient balance, please recharge"
	service := NewService(repository)

	result, err := service.Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.AccountTargets["41"].CleanupAction != nil {
		t.Fatalf("额度不足可恢复，不应触发自动删除：%#v", result.AccountTargets["41"])
	}
}

func TestCleanupDoesNotRemoveMinimumPoolSurvivor(t *testing.T) {
	policy := cleanupRoutingPolicy("delete", 0, true)
	repository := cleanupRoutingRepository(policy, false)
	service := NewService(repository)

	result, err := service.Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.AccountTargets["41"].CleanupAction != nil {
		t.Fatal("分组内最后一个账号不应被自动处置")
	}
	found := false
	for _, event := range repository.runtimeEvents {
		if strings.Contains(event.Summary, "保底强留") {
			found = true
		}
	}
	if !found {
		t.Fatalf("保底账号保护没有留下原因：%#v", repository.runtimeEvents)
	}
}

func cleanupRoutingPolicy(action string, observationMinutes int, maxOne bool) map[string]any {
	policy := routingPolicy()
	maxPerRound := int64(5)
	if maxOne {
		maxPerRound = 1
	}
	policy["cleanup"] = map[string]any{
		"enabled": true, "action": action, "occurrences": int64(1), "window": int64(5),
		"min_fused_minutes": int64(observationMinutes), "max_per_round": maxPerRound,
		"keep_last_in_group": true, "only_auth_errors": true, "trigger_status_codes": []any{int64(401)},
	}
	return policy
}

func cleanupRoutingRepository(policy map[string]any, includeSpare bool) *routingRepositoryStub {
	schedulable := true
	multiplier := "1"
	accounts := []business.RoutingAccount{{
		ID: "41", Name: "expired", GroupName: "codex", Schedulable: &schedulable,
		Multiplier: &multiplier, Metadata: map[string]any{},
	}}
	samples := []business.RoutingSample{{
		AccountID: "41", GroupName: "codex", Result: "失败", FailureReason: "unauthorized",
		Source: "traffic", ObservedAt: "2026-08-27T11:59:00Z", Payload: map[string]any{"status_code": int64(401)},
	}}
	if includeSpare {
		accounts = append(accounts, business.RoutingAccount{
			ID: "42", Name: "expired-too", GroupName: "codex", Schedulable: &schedulable,
			Multiplier: &multiplier, Metadata: map[string]any{},
		})
		samples = append(samples, business.RoutingSample{
			AccountID: "42", GroupName: "codex", Result: "失败", FailureReason: "unauthorized",
			Source: "traffic", ObservedAt: "2026-08-27T11:59:00Z", Payload: map[string]any{"status_code": int64(401)},
		})
	}
	return &routingRepositoryStub{policy: policy, accounts: accounts, samples: samples, cleanupState: map[string]time.Time{}}
}

func routingPolicy() map[string]any {
	policy := testPolicy()
	policy["selection"] = map[string]any{"strategy": "balanced"}
	policy["weights"] = map[string]any{"scheduling_missing_rate_fallback": "fail_open"}
	policy["traffic"] = map[string]any{"enabled": true}
	policy["breaker"] = map[string]any{}
	policy["degrade"] = map[string]any{}
	policy["recovery"] = map[string]any{}
	policy["scaling"] = map[string]any{}
	policy["cleanup"] = map[string]any{"action": "pause"}
	policy["scope"] = map[string]any{}
	return policy
}

func TestInvalidEnginePolicyReturnsErrorWithoutPanic(t *testing.T) {
	policy := routingPolicy()
	policy["breaker"].(map[string]any)["http_window"] = "not-an-integer"

	deferredRan := false
	func() {
		defer func() {
			deferredRan = true
			if recovered := recover(); recovered != nil {
				t.Fatalf("invalid policy panicked: %v", recovered)
			}
		}()
		_, err := parseEngineConfig(policy)
		if err == nil || !strings.Contains(err.Error(), "breaker.http_window") {
			t.Fatalf("unexpected validation error: %v", err)
		}
	}()
	if !deferredRan {
		t.Fatal("panic guard was not executed")
	}
}

func TestEngineRejectsRetiredStrategyAliases(t *testing.T) {
	for _, strategy := range []string{"price", "cost_first", "latency_first", "稳定优先"} {
		policy := routingPolicy()
		policy["selection"].(map[string]any)["strategy"] = strategy
		if _, err := parseEngineConfig(policy); err == nil {
			t.Fatalf("retired strategy alias %q was accepted", strategy)
		}
	}
}

func TestManualFuseScopeOverridesHealthyAccount(t *testing.T) {
	policy := routingPolicy()
	policy["scope"].(map[string]any)["manual_fused_account_ids"] = []any{"41"}
	schedulable := true
	multiplier := "1"
	repository := &routingRepositoryStub{
		policy: policy,
		accounts: []business.RoutingAccount{{
			ID: "41", Name: "healthy", GroupName: "codex", Schedulable: &schedulable,
			Multiplier: &multiplier, Metadata: map[string]any{},
		}},
	}
	service := NewService(repository)
	service.now = func() time.Time { return time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC) }
	result, err := service.Calculate(context.Background(), Scope{}, true)
	if err != nil {
		t.Fatal(err)
	}
	decision := result.AccountDecisions["41"]
	if decision.RoutingState != "fused" || decision.Schedulable || decision.Reason != "人工熔断" {
		t.Fatalf("manual fuse decision=%#v", decision)
	}
}

func TestCalculationOnlyRetainsGroupAndAccountCounts(t *testing.T) {
	repository := &routingRepositoryStub{
		policy: routingPolicy(),
		accounts: []business.RoutingAccount{{
			ID: "41", Name: "upstream-0.1", GroupName: "codex", Metadata: map[string]any{},
		}},
	}
	service := NewService(repository)
	service.now = func() time.Time { return time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC) }

	result, err := service.Calculate(context.Background(), Scope{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Accounts != 1 || result.Groups != 1 || result.Decisions != 0 || len(result.AccountDecisions) != 0 {
		t.Fatalf("calculation-only summary lost its scope: %#v", result)
	}
	if repository.persistDecisions || len(repository.evaluations) != 1 {
		t.Fatalf("unexpected persistence contract: persist=%v evaluations=%d", repository.persistDecisions, len(repository.evaluations))
	}
	if _, err := json.Marshal(result); err != nil {
		t.Fatalf("result is not strict JSON: %v", err)
	}
}

func TestSlowOccurrencesCountsOnlyFirstTokenLatency(t *testing.T) {
	totalDuration := "195843"
	firstToken := "16000"
	rows := []business.RoutingSample{
		{
			Source: "traffic", LatencyP95: &totalDuration,
			Payload: map[string]any{"latency_unit": "ms", "duration_ms": totalDuration},
		},
		{
			Source: "traffic", LatencyP95: &firstToken,
			Payload: map[string]any{"latency_unit": "ms", "latency_metric": "first_token"},
		},
	}

	if count := slowOccurrences(rows, 10, 15000); count != 1 {
		t.Fatalf("slow occurrences=%d, want 1", count)
	}
}

func TestFilterAndLimitSamplesAppliesOneWindowAfterSourceSelection(t *testing.T) {
	rows := make([]business.RoutingSample, 63)
	for index := range rows {
		rows[index].Source = "traffic"
	}
	for index := 0; index < 8; index++ {
		rows[index].Source = "active-probe"
	}

	mixed := filterAndLimitSamples(rows, "traffic", 60)
	if len(mixed) != 60 {
		t.Fatalf("mixed source window=%d, want 60", len(mixed))
	}
	probeOnly := filterAndLimitSamples(rows, "active_probe", 60)
	if len(probeOnly) != 8 {
		t.Fatalf("short probe history=%d, want actual 8", len(probeOnly))
	}
}
