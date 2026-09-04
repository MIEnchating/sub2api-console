package routing

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

type Repository interface {
	ControlPolicy(context.Context) (map[string]any, error)
	RoutingAccounts(context.Context, *string, *string) ([]business.RoutingAccount, error)
	RoutingSamples(context.Context, *string, *string, string, int) ([]business.RoutingSample, error)
	PreviousRoutingDecisions(context.Context, *string, *string) ([]business.PreviousRoutingDecision, error)
	CleanupStates(context.Context, *string) (map[string]time.Time, error)
	PersistRoutingRound(context.Context, *string, *string, []business.RoutingEvaluationWrite, []business.RoutingDecisionWrite, []business.AccountRoutingTarget, []business.CleanupStateWrite, []business.RuntimeEventWrite, bool, time.Time) error
}

type Scope struct {
	AccountID *string
	GroupName *string
}

type Result struct {
	Source              string                                   `json:"source"`
	CalculationOnly     bool                                     `json:"calculation_only"`
	RemoteWrite         bool                                     `json:"remote_write"`
	Accounts            int                                      `json:"accounts"`
	Groups              int                                      `json:"groups"`
	Decisions           int                                      `json:"decisions"`
	HealthEvaluations   int                                      `json:"health_evaluations"`
	Fused               int                                      `json:"fused"`
	NewlyFused          int                                      `json:"newly_fused"`
	Recovered           int                                      `json:"recovered"`
	Degraded            int                                      `json:"degraded"`
	Survivors           int                                      `json:"survivors"`
	ConfigurationErrors []string                                 `json:"configuration_errors"`
	AccountTargets      map[string]business.AccountRoutingTarget `json:"account_targets"`
	AccountDecisions    map[string]Decision                      `json:"account_decisions"`
}

type Decision struct {
	AccountID             string   `json:"account_id"`
	GroupName             string   `json:"group_name"`
	GroupID               *string  `json:"group_id"`
	Priority              *int64   `json:"priority"`
	Schedulable           bool     `json:"schedulable"`
	Role                  string   `json:"role"`
	RoutingState          string   `json:"routing_state"`
	Rank                  *int     `json:"rank"`
	Reason                string   `json:"reason"`
	HealthScore           float64  `json:"health_score"`
	RoutingHealthScore    float64  `json:"routing_health_score"`
	ShortScore            float64  `json:"short_score"`
	LongScore             float64  `json:"long_score"`
	SampleCount           int      `json:"sample_count"`
	TTFBP50MS             *float64 `json:"ttfb_p50_ms"`
	TTFBP95MS             *float64 `json:"ttfb_p95_ms"`
	LatestEvent           Event    `json:"latest_event"`
	Strategy              string   `json:"strategy"`
	Rate                  *string  `json:"rate"`
	RateKnown             bool     `json:"rate_known"`
	Weight                float64  `json:"weight"`
	DesiredLoadFactor     *string  `json:"desired_load_factor"`
	DesiredConcurrency    *int64   `json:"desired_concurrency"`
	CostWall              *string  `json:"cost_wall"`
	CostTier              string   `json:"cost_tier"`
	RateReason            *string  `json:"rate_reason"`
	WriteCooldownActive   bool     `json:"write_cooldown_active"`
	ScalingCooldownActive bool     `json:"scaling_cooldown_active"`
	RecoveryTarget        float64  `json:"recovery_target"`
	StateSince            string   `json:"state_since"`
	FusedUntil            *string  `json:"fused_until,omitempty"`
	CleanupAction         *string  `json:"cleanup_action,omitempty"`
}

type Service struct {
	repository Repository
	now        func() time.Time
}

type engineConfig struct {
	strategy              string
	trafficEnabled        bool
	trafficMaxAge         time.Duration
	probeMaxAge           time.Duration
	shortWindow           int
	longWindow            int
	breakerEnabled        bool
	hardFatal             bool
	httpWindow            int
	httpFailures          int
	httpScoreBelow        float64
	transientFailures     int
	latencyWindow         int
	latencyOccurrences    int
	latencyTTFBMS         float64
	maxSwitch             int
	minPool               int
	minPoolScore          float64
	fusedCooldown         time.Duration
	instantCodes          map[int]struct{}
	httpDegradeOnly       bool
	latencyDegradeOnly    bool
	degradeEnabled        bool
	degradeThreshold      float64
	degradePriorityStep   int64
	degradeLoadRatio      float64
	degradeMinLoad        int64
	recoveryEnabled       bool
	recoveryTarget        float64
	recoverySuccesses     int
	recoveryHold          time.Duration
	weightsEnabled        bool
	weightBudget          int64
	manualPriorityMax     int64
	manageAllAccounts     bool
	gateFloor             float64
	priceExp              float64
	speedExp              float64
	balancedPriceRatio    float64
	performanceMinSamples int
	speedAdvantageCap     float64
	missingRateFallback   string
	changeThreshold       *big.Rat
	cooldown              time.Duration
	minLoadFactor         int64
	maxLoadFactor         int64
	scalingEnabled        bool
	scalingGlobalMax      int64
	scalingMin            int64
	scalingMax            int64
	scalingUpRatio        float64
	scalingStepUp         int64
	scalingStepDown       int64
	scalingCooldown       time.Duration
	excludedGroups        map[string]struct{}
	excludedAccounts      map[string]struct{}
	pausedAccounts        map[string]struct{}
	manualFusedAccounts   map[string]struct{}
	managedMode           string
	managedGroups         map[string]struct{}
	accountTypes          map[string]struct{}
	platforms             map[string]struct{}
	groupBindings         map[string]any
	cleanupEnabled        bool
	cleanupAction         string
	cleanupOccurrences    int
	cleanupWindow         int
	cleanupObservation    time.Duration
	cleanupMaxPerRound    int
	cleanupKeepLast       bool
	cleanupOnlyAuth       bool
	cleanupStatusCodes    map[int]struct{}
}

type candidate struct {
	account            business.RoutingAccount
	health             Health
	routingHealth      float64
	rows               []business.RoutingSample
	performanceP50MS   *float64
	performanceP95MS   *float64
	performanceSamples int
	performanceModel   string
	rankingLatencyMS   float64
	rate               *big.Rat
	rateText           *string
	rateKnown          bool
	rateReason         *string
	costWall           *big.Rat
	costWallText       *string
	costTier           string
	costTierRank       int
	state              string
	reason             string
	schedulable        bool
	fuseKind           string
	strategy           string
	quality            float64
	weight             float64
	rank               *int
	desiredPriority    *int64
	desiredLoad        *string
	desiredConcurrency *int64
	writeCooldown      bool
	scalingCooldown    bool
	stateSince         time.Time
	fusedUntil         time.Time
	cleanupAction      *string
}

type strategyScores struct {
	price float64
	speed float64
}

const (
	primaryStrategyRatio   = .80
	secondaryStrategyRatio = .20
)

func NewService(repository Repository) *Service {
	return &Service{repository: repository, now: time.Now}
}

func (s *Service) Calculate(ctx context.Context, scope Scope, persistDecisions bool) (Result, error) {
	policy, err := s.repository.ControlPolicy(ctx)
	if err != nil {
		return Result{}, err
	}
	config, err := parseEngineConfig(policy)
	if err != nil {
		return Result{}, err
	}
	now := s.now().UTC()
	accounts, err := s.repository.RoutingAccounts(ctx, scope.AccountID, scope.GroupName)
	if err != nil {
		return Result{}, err
	}
	sampleSource := "active_probe"
	if config.trafficEnabled {
		sampleSource = "traffic"
	}
	samples, err := s.repository.RoutingSamples(ctx, scope.AccountID, scope.GroupName, sampleSource, config.longWindow)
	if err != nil {
		return Result{}, err
	}
	previousRows, err := s.repository.PreviousRoutingDecisions(ctx, scope.AccountID, scope.GroupName)
	if err != nil {
		return Result{}, err
	}
	cleanupState, err := s.repository.CleanupStates(ctx, scope.AccountID)
	if err != nil {
		return Result{}, err
	}
	sampleMap := map[string][]business.RoutingSample{}
	for _, sample := range samples {
		sampleMap[sample.AccountID] = append(sampleMap[sample.AccountID], sample)
	}
	previous := map[string]business.PreviousRoutingDecision{}
	for _, item := range previousRows {
		previous[routingKey(item.AccountID, item.GroupName)] = item
		current, found := previous[item.AccountID]
		if !found || item.GroupName < current.GroupName {
			previous[item.AccountID] = item
		}
	}
	groups := map[string][]business.RoutingAccount{}
	allMemberships := map[string][]business.RoutingAccount{}
	manualAccounts := map[string]struct{}{}
	externallyManagedAccounts := map[string][]business.RoutingAccount{}
	externalReleaseAccounts := map[string][]business.RoutingAccount{}
	for _, account := range accounts {
		if account.ManualPriority != nil {
			manualAccounts[account.ID] = struct{}{}
			continue
		}
		allMemberships[account.ID] = append(allMemberships[account.ID], account)
		if !config.manageAllAccounts && account.ExternalControl {
			externallyManagedAccounts[account.ID] = append(externallyManagedAccounts[account.ID], account)
			continue
		}
		if !config.manageAllAccounts && accountExternallyModified(account) {
			externallyManagedAccounts[account.ID] = append(externallyManagedAccounts[account.ID], account)
			externalReleaseAccounts[account.ID] = append(externalReleaseAccounts[account.ID], account)
			continue
		}
		groups[account.GroupName] = append(groups[account.GroupName], account)
	}
	groupNames := make([]string, 0, len(groups))
	for name := range groups {
		groupNames = append(groupNames, name)
	}
	sort.Strings(groupNames)
	result := Result{
		Source: "console-domain-db", CalculationOnly: true, RemoteWrite: false,
		ConfigurationErrors: []string{}, AccountTargets: map[string]business.AccountRoutingTarget{},
		AccountDecisions: map[string]Decision{},
	}
	processedGroups := 0
	evaluations := []business.RoutingEvaluationWrite{}
	writes := []business.RoutingDecisionWrite{}
	byAccount := map[string][]*candidate{}
	candidatesByGroup := map[string][]*candidate{}
	configsByGroup := map[string]engineConfig{}
	cleanupWrites := []business.CleanupStateWrite{}
	runtimeEvents := []business.RuntimeEventWrite{}
	for _, groupName := range groupNames {
		members := groups[groupName]
		groupConfig, enabled, err := config.forGroup(members[0].GroupID)
		if err != nil {
			return Result{}, err
		}
		if !enabled || groupConfig.groupExcluded(groupName, members[0].GroupID) || !groupConfig.groupManaged(groupName, members[0].GroupID) {
			continue
		}
		processedGroups++
		candidates := make([]*candidate, 0, len(members))
		for _, account := range members {
			if !accountMetadataManaged(account, groupConfig) {
				continue
			}
			source := "active_probe"
			if groupConfig.trafficEnabled {
				source = "traffic"
			}
			rows := filterAndLimitSamples(sampleMap[account.ID], source, groupConfig.longWindow*2)
			healthRows, performanceRows := selectRoutingEvidence(
				rows, now, groupConfig.trafficMaxAge, groupConfig.probeMaxAge, groupConfig.longWindow,
			)
			healthRows = withCriticalProbeEvidence(healthRows, rows, now, groupConfig.probeMaxAge, policy)
			healthInput := make([]Sample, 0, len(healthRows))
			for _, row := range healthRows {
				healthInput = append(healthInput, Sample{
					Result: row.Result, FailureReason: row.FailureReason, Source: row.Source,
					LatencyP95: row.LatencyP95, StatusCode: routingSampleStatus(row), Payload: row.Payload,
				})
			}
			health, scoreErr := HealthScore(healthInput, policy)
			if scoreErr != nil {
				return Result{}, scoreErr
			}
			performanceP50, performanceP95, performanceCount, performanceModel := performanceLatencySummary(performanceRows)
			health.P50MS, health.P95MS = performanceP50, performanceP95
			current := &candidate{
				account: account, health: health, routingHealth: health.HealthScore, rows: healthRows,
				performanceP50MS: performanceP50, performanceP95MS: performanceP95, performanceSamples: performanceCount,
				performanceModel: performanceModel,
				state:            "healthy", reason: "已计算",
				schedulable: remoteSchedulable(account), strategy: groupConfig.strategy,
			}
			if health.SampleCount == 0 {
				current.state = "unknown"
			}
			current.costWall, current.costWallText, err = effectiveCostWall(account)
			if err != nil {
				return Result{}, fmt.Errorf("分组 %s 成本墙配置无效：%w", groupName, err)
			}
			current.rate, current.rateText, current.rateKnown, current.rateReason = resolveRate(account, groupConfig, current.costWall)
			current.costTier, current.costTierRank = costTier(current.rate, current.costWall)
			prior, _ := previousDecision(previous, account.ID, groupName)
			applyInitialState(current, groupConfig, prior, now)
			candidates = append(candidates, current)
			byAccount[account.ID] = append(byAccount[account.ID], current)
		}
		candidatesByGroup[groupName] = candidates
		configsByGroup[groupName] = groupConfig
	}
	alignAccountStateToPrimary(byAccount, configsByGroup, previous, now)
	applyFuseBudgets(candidatesByGroup, configsByGroup, byAccount, now)
	for groupName, candidates := range candidatesByGroup {
		calculateGroupWeights(candidates, configsByGroup[groupName])
	}
	assignAccountPlacements(candidatesByGroup, configsByGroup, byAccount)
	finalizeAccountStates(byAccount, configsByGroup, previous, now)
	for _, accountID := range sortedTargetIDsFromCandidates(byAccount) {
		primary := primaryMembership(byAccount[accountID])
		if primary == nil {
			continue
		}
		groupName := primary.account.GroupName
		decision := publicDecision(primary, groupName, configsByGroup[groupName])
		evaluations = append(evaluations, evaluationWrite(primary, groupName))
		if persistDecisions {
			result.AccountDecisions[accountID] = decision
			writes = append(writes, decisionWrite(decision))
		}
		prior, _ := previousDecision(previous, accountID, groupName)
		priorFused := fusedRoutingState(prior.State)
		currentFused := fusedRoutingState(primary.state)
		if currentFused && !priorFused {
			result.NewlyFused++
		}
		if !currentFused && priorFused {
			result.Recovered++
		}
		switch primary.state {
		case "fused":
			result.Fused++
		case "degraded":
			result.Degraded++
		case "survivor":
			result.Survivors++
		}
	}
	result.HealthEvaluations = len(evaluations)
	if persistDecisions {
		runtimeEvents = append(runtimeEvents, routingStateTransitionEvents(byAccount, previous)...)
		var cleanupEvents []business.RuntimeEventWrite
		cleanupWrites, cleanupEvents = applyCleanupPolicy(byAccount, config, cleanupState, now)
		runtimeEvents = append(runtimeEvents, cleanupEvents...)
		result.AccountTargets = aggregateTargets(byAccount)
		for accountID, memberships := range externalReleaseAccounts {
			groupNames := make([]string, 0, len(memberships))
			for _, membership := range memberships {
				groupNames = append(groupNames, membership.GroupName)
			}
			result.AccountTargets[accountID] = business.AccountRoutingTarget{
				AccountID: accountID, GroupNames: uniqueSorted(groupNames),
				DesiredHealth: "external_control", AbandonControl: true,
			}
		}
		// A group that leaves Guardian's scope is not merely skipped: accounts
		// with no remaining managed membership must have their captured baseline
		// restored. Group-scoped runs cannot safely make this account-level
		// determination because other memberships were not loaded.
		if scope.GroupName == nil {
			for accountID, memberships := range allMemberships {
				if _, manual := manualAccounts[accountID]; manual {
					continue
				}
				if _, managed := byAccount[accountID]; managed {
					continue
				}
				if _, external := externallyManagedAccounts[accountID]; external {
					continue
				}
				groupNames := make([]string, 0, len(memberships))
				for _, membership := range memberships {
					groupNames = append(groupNames, membership.GroupName)
				}
				result.AccountTargets[accountID] = business.AccountRoutingTarget{
					AccountID: accountID, GroupNames: uniqueSorted(groupNames),
					DesiredHealth: "excluded", ReleaseControl: true,
				}
			}
		}
		result.Decisions = len(writes)
	}
	result.Accounts = len(byAccount)
	result.Groups = processedGroups
	targets := make([]business.AccountRoutingTarget, 0, len(result.AccountTargets))
	for _, id := range sortedTargetIDs(result.AccountTargets) {
		targets = append(targets, result.AccountTargets[id])
	}
	if err := s.repository.PersistRoutingRound(ctx, scope.AccountID, scope.GroupName, evaluations, writes, targets, cleanupWrites, runtimeEvents, persistDecisions, now); err != nil {
		return Result{}, err
	}
	return result, nil
}

func routingStateTransitionEvents(
	byAccount map[string][]*candidate,
	previous map[string]business.PreviousRoutingDecision,
) []business.RuntimeEventWrite {
	result := []business.RuntimeEventWrite{}
	for _, accountID := range sortedTargetIDsFromCandidates(byAccount) {
		items := byAccount[accountID]
		if len(items) == 0 {
			continue
		}
		primary := items[0]
		for _, item := range items[1:] {
			if membershipLess(item, primary) {
				primary = item
			}
		}
		prior, found := previousDecision(previous, accountID, primary.account.GroupName)
		if found && prior.State == primary.state {
			continue
		}
		eventType, status, action := "", "succeeded", ""
		switch primary.state {
		case "fused":
			eventType, status, action = "routing.fused", "failed", "已停止调度"
		case "cost_blocked":
			eventType, status, action = "routing.cost_blocked", "warning", "已被成本墙拦截"
		case "binding_invalid":
			eventType, status, action = "routing.binding_invalid", "failed", "已因上游绑定失效而停止调度"
		case "degraded":
			eventType, status, action = "routing.degraded", "warning", "已降级"
		case "survivor":
			eventType, status, action = "routing.survivor", "warning", "已被保底强留"
		case "healthy", "unknown":
			if found && (prior.State == "fused" || prior.State == "cost_blocked" || prior.State == "binding_invalid" || prior.State == "degraded" || prior.State == "survivor") {
				eventType, action = "routing.recovered", "已恢复调度"
			}
		}
		if eventType == "" {
			continue
		}
		summary := fmt.Sprintf("账号 %s（%s）%s", primary.account.Name, accountID, action)
		if strings.TrimSpace(primary.reason) != "" {
			summary += "：" + primary.reason
		}
		payload := map[string]any{
			"account_id": accountID, "account_name": primary.account.Name,
			"group_name": primary.account.GroupName, "group_id": primary.account.GroupID,
			"previous_state": prior.State, "state": primary.state, "reason": primary.reason,
			"health_score": primary.health.HealthScore, "schedulable": primary.schedulable,
		}
		result = append(result, business.RuntimeEventWrite{EventType: eventType, Status: status, Summary: summary, Payload: payload})
	}
	return result
}

func accountMetadataManaged(account business.RoutingAccount, config engineConfig) bool {
	accountType := strings.ToLower(strings.TrimSpace(textMetadata(account.Metadata, "type", "account_type")))
	if accountType == "" {
		accountType = "apikey"
	}
	platform := strings.ToLower(strings.TrimSpace(textMetadata(account.Metadata, "platform")))
	if platform == "" && account.UpstreamType != nil {
		platform = strings.ToLower(strings.TrimSpace(*account.UpstreamType))
	}
	if len(config.accountTypes) > 0 {
		if _, found := config.accountTypes[accountType]; !found {
			return false
		}
	}
	if len(config.platforms) > 0 {
		if _, found := config.platforms[platform]; !found {
			return false
		}
	}
	return true
}

func alignAccountStateToPrimary(
	byAccount map[string][]*candidate,
	configs map[string]engineConfig,
	previous map[string]business.PreviousRoutingDecision,
	now time.Time,
) {
	for _, memberships := range byAccount {
		if len(memberships) == 0 {
			continue
		}
		primary := primaryMembership(memberships)
		for _, item := range memberships {
			if item == primary {
				continue
			}
			item.health = primary.health
			item.rows = primary.rows
			if primary.rate != nil {
				item.rate = new(big.Rat).Set(primary.rate)
			} else {
				item.rate = nil
			}
			item.rateText = cloneString(primary.rateText)
			item.rateKnown = primary.rateKnown
			item.rateReason = cloneString(primary.rateReason)
			item.costTier, item.costTierRank = costTier(item.rate, item.costWall)
		}
		primary.state, primary.reason, primary.schedulable, primary.fuseKind = "healthy", "已计算", remoteSchedulable(primary.account), ""
		primary.fusedUntil = time.Time{}
		prior, _ := previousDecision(previous, primary.account.ID, primary.account.GroupName)
		applyInitialState(primary, configs[primary.account.GroupName], prior, now)
		applyAccountCostWall(primary, memberships, now)
		for _, item := range memberships {
			if item == primary {
				continue
			}
			item.state, item.reason, item.schedulable, item.fuseKind = primary.state, primary.reason, primary.schedulable, primary.fuseKind
			item.fusedUntil = primary.fusedUntil
		}
	}
}

func applyAccountCostWall(primary *candidate, memberships []*candidate, now time.Time) {
	allAbove := len(memberships) > 0
	for _, item := range memberships {
		if item.costTier != "above" {
			allAbove = false
			break
		}
	}
	stateCanBeCostBlocked := primary.state == "healthy" || primary.state == "degraded" || primary.state == "unknown"
	previouslyCostBlocked := strings.EqualFold(strings.TrimSpace(primary.account.EffectiveState), "cost_blocked")
	if allAbove && stateCanBeCostBlocked && (remoteSchedulable(primary.account) || previouslyCostBlocked) {
		primary.state = "cost_blocked"
		primary.schedulable = false
		primary.reason = "所有受管分组的倍率均超过当前成本墙"
		return
	}
	if !allAbove && stateCanBeCostBlocked && previouslyCostBlocked && costWallRecoveryAllowed(primary.account, now) {
		primary.schedulable = true
		primary.reason = "至少一个受管分组的倍率已回到成本墙范围"
	}
}

func costWallRecoveryAllowed(account business.RoutingAccount, now time.Time) bool {
	schedulable := true
	return business.AccountUpstreamBlock(account.Metadata, &schedulable, now) == ""
}

func fusedRoutingState(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "fused", "hard_open", "soft_open":
		return true
	default:
		return false
	}
}

func parseEngineConfig(policy map[string]any) (engineConfig, error) {
	selection, err := requiredObject(policy, "selection")
	if err != nil {
		return engineConfig{}, err
	}
	weights, err := requiredObject(policy, "weights")
	if err != nil {
		return engineConfig{}, err
	}
	traffic, err := requiredObject(policy, "traffic")
	if err != nil {
		return engineConfig{}, err
	}
	probe, err := optionalObject(policy, "probe")
	if err != nil {
		return engineConfig{}, err
	}
	breaker, err := requiredObject(policy, "breaker")
	if err != nil {
		return engineConfig{}, err
	}
	degrade, err := requiredObject(policy, "degrade")
	if err != nil {
		return engineConfig{}, err
	}
	recovery, err := requiredObject(policy, "recovery")
	if err != nil {
		return engineConfig{}, err
	}
	scaling, err := requiredObject(policy, "scaling")
	if err != nil {
		return engineConfig{}, err
	}
	cleanup, err := requiredObject(policy, "cleanup")
	if err != nil {
		return engineConfig{}, err
	}
	scope, err := requiredObject(policy, "scope")
	if err != nil {
		return engineConfig{}, err
	}
	manualPriority := map[string]any{}
	if raw, present := policy["manual_priority"]; present {
		manualPriority, _ = raw.(map[string]any)
		if manualPriority == nil {
			return engineConfig{}, errors.New("manual_priority 必须是对象")
		}
	}
	strategy, err := strategyField(selection, "strategy", "balanced")
	if err != nil {
		return engineConfig{}, err
	}
	change, err := decimalField(weights, "change_threshold", "0.1", false)
	if err != nil || change.Sign() <= 0 || change.Cmp(big.NewRat(1, 1)) > 0 {
		return engineConfig{}, errors.New("weights.change_threshold 必须大于 0 且不超过 1")
	}
	missing, ok := weights["scheduling_missing_rate_fallback"].(string)
	missing = strings.ToLower(strings.TrimSpace(missing))
	if !ok || (missing != "current_cost_wall" && missing != "fail_closed" && missing != "fail_open") {
		return engineConfig{}, errors.New("weights.scheduling_missing_rate_fallback 配置无效")
	}
	instantCodes, err := integerSet(breaker, "instant_status_codes", 100, 599)
	if err != nil {
		return engineConfig{}, err
	}
	cleanupCodes, err := integerSet(cleanup, "trigger_status_codes", 100, 599)
	if err != nil {
		return engineConfig{}, err
	}
	rawCleanupAction, present := cleanup["action"]
	if !present {
		rawCleanupAction = "pause"
	}
	cleanupAction, ok := rawCleanupAction.(string)
	cleanupAction = strings.ToLower(strings.TrimSpace(cleanupAction))
	if !ok || (cleanupAction != "none" && cleanupAction != "pause" && cleanupAction != "disable" && cleanupAction != "delete") {
		return engineConfig{}, errors.New("cleanup.action 配置无效")
	}
	bindings := map[string]any{}
	if raw, present := policy["group_policy_bindings"]; present {
		bindings, ok = raw.(map[string]any)
		if !ok {
			return engineConfig{}, errors.New("group_policy_bindings 必须是对象")
		}
	}
	managedMode, _ := scope["managed_group_mode"].(string)
	if managedMode == "" {
		managedMode = "all"
	}
	if managedMode != "all" && managedMode != "selected" {
		return engineConfig{}, errors.New("scope.managed_group_mode 配置无效")
	}
	reader := &policyReader{}
	probeInterval := reader.integer(probe, "probe.interval_seconds", "interval_seconds", 300, 1, 86400)
	config := engineConfig{
		strategy: strategy, trafficEnabled: reader.boolean(traffic, "traffic.enabled", "enabled", true),
		trafficMaxAge:  time.Duration(reader.integer(traffic, "traffic.lookback_minutes", "lookback_minutes", 120, 1, 10080)) * time.Minute,
		probeMaxAge:    time.Duration(max(probeInterval*3, 300)) * time.Second,
		shortWindow:    reader.nestedInteger(policy, "scoring", "short_window", 10, 1, 10000),
		longWindow:     reader.nestedInteger(policy, "scoring", "long_window", 60, 1, 100000),
		breakerEnabled: reader.boolean(breaker, "breaker.enabled", "enabled", true), hardFatal: reader.boolean(breaker, "breaker.hard_fatal", "hard_fatal", true),
		httpWindow: reader.integer(breaker, "breaker.http_window", "http_window", 5, 1, 10000), httpFailures: reader.integer(breaker, "breaker.http_failures", "http_failures", 3, 1, 10000),
		httpScoreBelow:    reader.number(breaker, "breaker.http_score_below", "http_score_below", 60, 0, 100),
		transientFailures: reader.integer(breaker, "breaker.transient_consecutive_failures", "transient_consecutive_failures", 2, 1, 10000),
		latencyWindow:     reader.integer(breaker, "breaker.latency_window", "latency_window", 10, 1, 10000), latencyOccurrences: reader.integer(breaker, "breaker.latency_occurrences", "latency_occurrences", 5, 1, 10000),
		latencyTTFBMS: reader.number(breaker, "breaker.latency_ttfb_ms", "latency_ttfb_ms", 15000, 1, 3_600_000),
		maxSwitch:     reader.integer(breaker, "breaker.max_switch_per_round", "max_switch_per_round", 1, 1, 10000), minPool: reader.integer(breaker, "breaker.min_pool_size", "min_pool_size", 1, 0, 10000),
		minPoolScore:  reader.number(breaker, "breaker.min_pool_score", "min_pool_score", 3, 0, 100),
		fusedCooldown: time.Duration(reader.integer(breaker, "breaker.fused_cooldown_seconds", "fused_cooldown_seconds", 180, 0, 86400)) * time.Second,
		instantCodes:  instantCodes, httpDegradeOnly: reader.boolean(breaker, "breaker.http_degrade_only", "http_degrade_only", true), latencyDegradeOnly: reader.boolean(breaker, "breaker.latency_degrade_only", "latency_degrade_only", true),
		degradeEnabled: reader.boolean(degrade, "degrade.enabled", "enabled", true), degradeThreshold: reader.number(degrade, "degrade.score_threshold", "score_threshold", 75, 0, 100),
		degradePriorityStep: int64(reader.integer(degrade, "degrade.priority_step", "priority_step", 10, 1, 100000)), degradeLoadRatio: reader.number(degrade, "degrade.load_factor_ratio", "load_factor_ratio", .5, math.SmallestNonzeroFloat64, 1),
		degradeMinLoad:  int64(reader.integer(degrade, "degrade.min_load_factor", "min_load_factor", 1, 1, 100000)),
		recoveryEnabled: reader.boolean(recovery, "recovery.enabled", "enabled", true), recoveryTarget: reader.number(recovery, "recovery.target_score", "target_score", 75, 0, 100),
		recoverySuccesses: reader.integer(recovery, "recovery.success_count", "success_count", 2, 1, 10000), recoveryHold: time.Duration(reader.integer(recovery, "recovery.hold_seconds", "hold_seconds", 60, 0, 86400)) * time.Second,
		weightsEnabled: reader.boolean(weights, "weights.enabled", "enabled", true), weightBudget: int64(reader.integer(weights, "weights.budget", "budget", 400, 1, 1_000_000)),
		manualPriorityMax: int64(reader.integer(manualPriority, "manual_priority.reserved_max", "reserved_max", 10, 1, 1000)),
		manageAllAccounts: reader.boolean(scope, "scope.manage_all_accounts", "manage_all_accounts", true),
		gateFloor:         reader.number(weights, "weights.gate_floor", "gate_floor", 40, 0, 100), priceExp: reader.number(weights, "weights.price_exp", "price_exp", 1, math.SmallestNonzeroFloat64, 100), speedExp: reader.number(weights, "weights.speed_exp", "speed_exp", 1, math.SmallestNonzeroFloat64, 100),
		balancedPriceRatio: reader.number(weights, "weights.balanced_price_ratio", "balanced_price_ratio", .5, 0, 1), missingRateFallback: missing, changeThreshold: change,
		performanceMinSamples: reader.integer(weights, "weights.performance_min_samples", "performance_min_samples", 5, 1, 200),
		speedAdvantageCap:     reader.number(weights, "weights.speed_advantage_cap", "speed_advantage_cap", 4, 1, 100),
		cooldown:              time.Duration(reader.integer(weights, "weights.cooldown_seconds", "cooldown_seconds", 60, 0, 86400)) * time.Second,
		minLoadFactor:         int64(reader.integer(weights, "weights.min_load_factor", "min_load_factor", 1, 1, 1_000_000)), maxLoadFactor: int64(reader.integer(weights, "weights.max_load_factor", "max_load_factor", 100, 1, 1_000_000)),
		scalingEnabled: reader.boolean(scaling, "scaling.enabled", "enabled", false), scalingGlobalMax: int64(reader.integer(scaling, "scaling.global_max_concurrency", "global_max_concurrency", 900, 1, 10_000_000)),
		scalingMin: int64(reader.integer(scaling, "scaling.min_per_account", "min_per_account", 3, 1, 1_000_000)), scalingMax: int64(reader.integer(scaling, "scaling.max_per_account", "max_per_account", 250, 1, 1_000_000)),
		scalingUpRatio: reader.number(scaling, "scaling.scale_up_ratio", "scale_up_ratio", .8, math.SmallestNonzeroFloat64, 1), scalingStepUp: int64(reader.integer(scaling, "scaling.step_up", "step_up", 5, 1, 1_000_000)),
		scalingStepDown:  int64(reader.integer(scaling, "scaling.step_down", "step_down", 5, 1, 1_000_000)),
		scalingCooldown:  time.Duration(reader.integer(scaling, "scaling.cooldown_seconds", "cooldown_seconds", 60, 0, 86400)) * time.Second,
		excludedGroups:   reader.stringSet(scope, "scope.excluded_group_ids", "excluded_group_ids"),
		excludedAccounts: reader.stringSet(scope, "scope.excluded_account_ids", "excluded_account_ids"),
		pausedAccounts:   reader.stringSet(scope, "scope.paused_account_ids", "paused_account_ids"), manualFusedAccounts: reader.stringSet(scope, "scope.manual_fused_account_ids", "manual_fused_account_ids"),
		managedMode: managedMode, managedGroups: reader.stringSet(scope, "scope.managed_group_ids", "managed_group_ids"), accountTypes: reader.stringSet(scope, "scope.account_types", "account_types"),
		platforms: reader.stringSet(scope, "scope.platforms", "platforms"), groupBindings: bindings,
		cleanupEnabled: reader.boolean(cleanup, "cleanup.enabled", "enabled", false), cleanupAction: cleanupAction,
		cleanupOccurrences: reader.integer(cleanup, "cleanup.occurrences", "occurrences", 3, 1, 10000),
		cleanupWindow:      reader.integer(cleanup, "cleanup.window", "window", 5, 1, 10000),
		cleanupObservation: time.Duration(reader.integer(cleanup, "cleanup.min_fused_minutes", "min_fused_minutes", 30, 0, 10080)) * time.Minute,
		cleanupMaxPerRound: reader.integer(cleanup, "cleanup.max_per_round", "max_per_round", 1, 1, 10000),
		cleanupKeepLast:    reader.boolean(cleanup, "cleanup.keep_last_in_group", "keep_last_in_group", true),
		cleanupOnlyAuth:    reader.boolean(cleanup, "cleanup.only_auth_errors", "only_auth_errors", true), cleanupStatusCodes: cleanupCodes,
	}
	if reader.err != nil {
		return engineConfig{}, reader.err
	}
	if config.httpFailures > config.httpWindow || config.latencyOccurrences > config.latencyWindow || config.maxLoadFactor < config.minLoadFactor || config.scalingMax < config.scalingMin || config.cleanupOccurrences > config.cleanupWindow {
		return engineConfig{}, errors.New("调度策略阈值关系无效")
	}
	return config, nil
}

func (c engineConfig) forGroup(groupID *string) (engineConfig, bool, error) {
	if groupID == nil {
		return c, true, nil
	}
	raw, present := c.groupBindings[*groupID]
	if !present {
		return c, true, nil
	}
	binding, ok := raw.(map[string]any)
	if !ok {
		return engineConfig{}, false, fmt.Errorf("group_policy_bindings.%s 必须是对象", *groupID)
	}
	result := c
	enabled := true
	if rawEnabled, present := binding["enabled"]; present {
		value, ok := rawEnabled.(bool)
		if !ok {
			return engineConfig{}, false, fmt.Errorf("group_policy_bindings.%s.enabled 必须是布尔值", *groupID)
		}
		enabled = value
	}
	if rawStrategy, present := binding["strategy"]; present {
		value, err := normalizeStrategyName(rawStrategy)
		if err != nil {
			return engineConfig{}, false, fmt.Errorf("group_policy_bindings.%s.strategy 配置无效", *groupID)
		}
		result.strategy = value
	}
	if rawInterval, present := binding["probe_interval_seconds"]; present {
		value, err := exactInteger(rawInterval)
		if err != nil || value < 30 || value > 86400 {
			return engineConfig{}, false, fmt.Errorf("group_policy_bindings.%s.probe_interval_seconds 必须在 30 到 86400 之间", *groupID)
		}
		result.probeMaxAge = time.Duration(max(value*3, 300)) * time.Second
	}
	for field, target := range map[string]*bool{
		"breaker_enabled": &result.breakerEnabled, "recovery_enabled": &result.recoveryEnabled,
		"weights_enabled": &result.weightsEnabled, "scaling_enabled": &result.scalingEnabled,
	} {
		if rawValue, present := binding[field]; present {
			value, ok := rawValue.(bool)
			if !ok {
				return engineConfig{}, false, fmt.Errorf("group_policy_bindings.%s.%s 必须是布尔值", *groupID, field)
			}
			*target = value
		}
	}
	for field, target := range map[string]*int{
		"min_pool_size": &result.minPool, "weight_budget": nil,
	} {
		if rawValue, present := binding[field]; present {
			value, err := exactInteger(rawValue)
			if err != nil || value < 0 {
				return engineConfig{}, false, fmt.Errorf("group_policy_bindings.%s.%s 必须是非负整数", *groupID, field)
			}
			if target == nil {
				if value < 1 {
					return engineConfig{}, false, fmt.Errorf("group_policy_bindings.%s.%s 必须是正整数", *groupID, field)
				}
				result.weightBudget = int64(value)
			} else {
				*target = value
			}
		}
	}
	if rawRatio, present := binding["balanced_price_ratio"]; present {
		value, err := strictNumber(rawRatio)
		if err != nil || value < 0 || value > 1 {
			return engineConfig{}, false, fmt.Errorf("group_policy_bindings.%s.balanced_price_ratio 必须在 0 到 1 之间", *groupID)
		}
		result.balancedPriceRatio = value
	}
	return result, enabled, nil
}

func applyInitialState(item *candidate, config engineConfig, previous business.PreviousRoutingDecision, now time.Time) {
	item.schedulable = remoteSchedulable(item.account)
	neutralOnly := item.health.SampleCount == 0 && item.health.NeutralCount > 0
	confirmedUnhealthy, transientPending := routingEvidenceConfirmation(item, config)
	item.routingHealth = confirmedRoutingHealth(item, config, previous, confirmedUnhealthy, transientPending)
	allowed, reason := eligibleScope(item.account, config)
	_, accountPaused := config.pausedAccounts[strings.ToLower(strings.TrimSpace(item.account.ID))]
	_, manuallyFused := config.manualFusedAccounts[strings.ToLower(strings.TrimSpace(item.account.ID))]
	switch {
	case !allowed:
		item.state, item.schedulable, item.reason = "excluded", false, reason
	case item.account.Paused:
		item.state, item.schedulable, item.reason = "paused", false, "人工暂停"
	case accountPaused:
		item.state, item.schedulable, item.reason = "paused", false, "账号已在策略中暂停"
	case accountDisabled(item.account):
		item.state, item.schedulable, item.reason = "disabled", false, "账号已停用"
	case catalogBindingInvalid(item.account.CatalogBindingState):
		item.state, item.schedulable = "binding_invalid", false
		item.reason = pointerText(item.account.CatalogBindingReason, "上游 Key 或分组已确认删除，绑定失效")
	case item.account.Schedulable == nil:
		item.state, item.schedulable, item.reason = "unknown", false, "账号调度状态未知"
	case manuallyFused:
		item.state, item.schedulable, item.reason = "fused", false, "人工熔断"
	case item.rate == nil:
		item.state, item.schedulable, item.reason, item.fuseKind = "fuse_pending", false, pointerText(item.rateReason, "倍率不可用"), "soft"
	case strings.EqualFold(strings.TrimSpace(item.account.EffectiveState), "binding_invalid") &&
		strings.EqualFold(strings.TrimSpace(item.account.CatalogBindingState), "active"):
		item.state, item.schedulable, item.reason = "healthy", true, "上游 Key 与分组已重新出现，稳定 ID 绑定已恢复"
	case fusedRoutingState(item.account.EffectiveState):
		item.fusedUntil = previousFusedUntil(previous)
		if item.fusedUntil.IsZero() {
			stateSince := previousStateSince(previous)
			if !stateSince.IsZero() {
				item.fusedUntil = stateSince.UTC().Add(config.fusedCooldown)
			}
		}
		healthySpan := recoverySpan(item.rows, item.health.RecoveryPassStreak, now)
		if config.recoveryEnabled && (item.fusedUntil.IsZero() || !now.Before(item.fusedUntil)) && item.health.HealthScore >= config.recoveryTarget &&
			item.health.RecoveryPassStreak >= config.recoverySuccesses && healthySpan >= config.recoveryHold && !item.health.Fatal {
			item.state, item.schedulable, item.reason = "healthy", true, "健康分与连续成功已达到回池条件"
			item.fusedUntil = time.Time{}
		} else {
			item.state, item.schedulable, item.reason = "fused", false, "熔断恢复条件未满足"
		}
	case config.breakerEnabled && config.hardFatal && item.health.Fatal:
		item.state, item.schedulable, item.reason, item.fuseKind = "fuse_pending", false, "致命错误：凭据失效", "hard"
	case config.breakerEnabled && !candidateRateLimited(item, now) && latestStatusIn(item.rows, config.instantCodes):
		item.state, item.schedulable, item.reason, item.fuseKind = "fuse_pending", false, "命中立即熔断状态码", "soft"
	case config.breakerEnabled && !candidateRateLimited(item, now) && recentHTTPFailures(item.health, config.httpWindow) >= config.httpFailures && item.health.HealthScore < config.httpScoreBelow:
		if config.httpDegradeOnly {
			item.state, item.reason = "degraded", "网关错误率达到阈值，仅降级"
		} else {
			item.state, item.schedulable, item.reason, item.fuseKind = "fuse_pending", false, "网关错误率达到熔断阈值", "soft"
		}
	case config.breakerEnabled && config.latencyOccurrences > 0 && !candidateRateLimited(item, now) && slowOccurrences(item.rows, config.latencyWindow, config.latencyTTFBMS) >= config.latencyOccurrences:
		if config.latencyDegradeOnly {
			item.state, item.reason = "degraded", "延迟超标达到阈值，仅降级"
		} else {
			item.state, item.schedulable, item.reason, item.fuseKind = "fuse_pending", false, "延迟超标达到熔断阈值", "soft"
		}
	case config.breakerEnabled && config.httpFailures > 0 && retryRecoveredOccurrences(item.rows, config.httpWindow) >= config.httpFailures:
		item.state, item.reason = "degraded", "近期多次依赖重试成功"
	case neutralOnly:
		item.state, item.reason = preservedRoutingState(item.account.EffectiveState, previous.State), "客户端错误仅记录，保持上一调度状态"
	case item.health.SampleCount == 0:
		// Guardian keeps an unprobed account's health unknown while still
		// counting the remotely schedulable account in the minimum pool. Health
		// evidence and current traffic eligibility are separate facts.
		item.state, item.reason = "unknown", "尚无样本，等待首次探测"
	case degradedRoutingState(item.account.EffectiveState, previous.State) && !degradedRecoveryAllowed(item, config, now):
		item.state, item.reason = "degraded", "降级恢复条件未满足"
	case config.degradeEnabled && confirmedUnhealthy && item.health.SampleCount > 0 && item.health.HealthScore < config.degradeThreshold:
		item.state, item.reason = "degraded", fmt.Sprintf("健康分低于降级线 %.4g", config.degradeThreshold)
	case transientPending:
		item.reason = "短暂异常待确认，保持当前调度位置"
	}
	if config.manageAllAccounts && managedAccountCanReceiveTraffic(item, now) {
		item.schedulable = true
	}
}

func routingEvidenceConfirmation(item *candidate, config engineConfig) (bool, bool) {
	recent := recentTransientFailures(item.health, config.httpWindow)
	retryRecovered := retryRecoveredOccurrences(item.rows, config.httpWindow)
	consecutiveThreshold := config.transientFailures
	if consecutiveThreshold < 1 {
		consecutiveThreshold = 2
	}
	confirmed := item.health.FailureStreak >= consecutiveThreshold
	if config.httpFailures > 0 {
		confirmed = confirmed || recent >= config.httpFailures || retryRecovered >= config.httpFailures
	}
	slowConfirmed := config.latencyOccurrences > 0 && slowOccurrences(item.rows, config.latencyWindow, config.latencyTTFBMS) >= config.latencyOccurrences
	rateLimited := item.health.LatestEvent == EventRateLimited
	hasPending := (recent > 0 || retryRecovered > 0) && !confirmed
	return confirmed || slowConfirmed || rateLimited, hasPending
}

func confirmedRoutingHealth(
	item *candidate,
	config engineConfig,
	previous business.PreviousRoutingDecision,
	confirmed bool,
	pending bool,
) float64 {
	neutralOnly := item.health.SampleCount == 0 && item.health.NeutralCount > 0
	if confirmed || !pending && !neutralOnly {
		return item.health.HealthScore
	}
	if raw, present := previous.Payload["routing_health_score"]; present {
		if value, err := strictNumber(raw); err == nil {
			return value
		}
	}
	if raw, present := previous.Payload["health_score"]; present {
		if value, err := strictNumber(raw); err == nil {
			return value
		}
	}
	if strings.EqualFold(strings.TrimSpace(previous.State), "healthy") || strings.EqualFold(strings.TrimSpace(item.account.EffectiveState), "healthy") {
		return 100
	}
	return config.degradeThreshold
}

func preservedRoutingState(values ...string) string {
	if degradedRoutingState(values...) {
		return "degraded"
	}
	for _, value := range values {
		switch normalized := strings.ToLower(strings.TrimSpace(value)); normalized {
		case "healthy", "unknown", "survivor":
			return normalized
		}
	}
	return "unknown"
}

func degradedRoutingState(values ...string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), "degraded") {
			return true
		}
	}
	return false
}

func degradedRecoveryAllowed(item *candidate, config engineConfig, now time.Time) bool {
	if !config.recoveryEnabled || item.health.Fatal || item.health.HealthScore < config.recoveryTarget || item.health.RecoveryPassStreak < config.recoverySuccesses {
		return false
	}
	return recoverySpan(item.rows, item.health.RecoveryPassStreak, now) >= config.recoveryHold
}

func managedAccountCanReceiveTraffic(item *candidate, now time.Time) bool {
	if item.account.Schedulable == nil {
		return false
	}
	switch item.state {
	case "healthy", "degraded", "unknown", "survivor":
		remoteEnabled := true
		return business.AccountUpstreamBlock(item.account.Metadata, &remoteEnabled, now) == ""
	default:
		return false
	}
}

func accountExternallyModified(account business.RoutingAccount) bool {
	if account.ManagedSchedulable != nil && !sameOptionalBool(account.Schedulable, account.ManagedSchedulable) {
		return true
	}
	if account.ManagedPriority != nil && !sameOptionalInt64(account.Priority, account.ManagedPriority) {
		return true
	}
	if account.ManagedLoadFactor != nil && !sameOptionalText(account.LoadFactor, account.ManagedLoadFactor) {
		return true
	}
	return account.ManagedConcurrency != nil && !sameOptionalInt64(account.Concurrency, account.ManagedConcurrency)
}

func sameOptionalBool(left, right *bool) bool {
	return left != nil && right != nil && *left == *right
}

func sameOptionalInt64(left, right *int64) bool {
	return left != nil && right != nil && *left == *right
}

func sameOptionalText(left, right *string) bool {
	return left != nil && right != nil && strings.TrimSpace(*left) == strings.TrimSpace(*right)
}

func catalogBindingInvalid(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "key_missing", "group_missing", "key_and_group_missing":
		return true
	default:
		return false
	}
}

func remoteSchedulable(account business.RoutingAccount) bool {
	return account.Schedulable != nil && *account.Schedulable
}

func accountDisabled(account business.RoutingAccount) bool {
	status := strings.ToLower(strings.TrimSpace(textMetadata(account.Metadata, "status")))
	return status == "inactive" || status == "disabled" || status == "停用" || status == "已停用"
}

func candidateRateLimited(item *candidate, now time.Time) bool {
	return accountUpstreamBlock(item.account, now) == "rate_limited" || item.health.LatestEvent == EventRateLimited
}

// accountUpstreamBlock mirrors the fields used by Sub2API's routing query.
// An empty value means the account is currently eligible from Sub2API's point
// of view; Guardian health state is evaluated separately.
func accountUpstreamBlock(account business.RoutingAccount, now time.Time) string {
	return business.AccountUpstreamBlock(account.Metadata, account.Schedulable, now)
}

type accountFuseRequest struct {
	accountID    string
	kind         string
	healthScore  float64
	triggerGroup string
	reason       string
}

func applyFuseBudgets(groups map[string][]*candidate, configs map[string]engineConfig, byAccount map[string][]*candidate, now time.Time) {
	requestsByAccount := map[string]accountFuseRequest{}
	for groupName, items := range groups {
		for _, item := range items {
			if item.state != "fuse_pending" {
				continue
			}
			request, found := requestsByAccount[item.account.ID]
			if !found || item.fuseKind == "hard" && request.kind != "hard" ||
				item.fuseKind == request.kind && item.health.HealthScore < request.healthScore {
				requestsByAccount[item.account.ID] = accountFuseRequest{
					accountID: item.account.ID, kind: item.fuseKind, healthScore: item.health.HealthScore,
					triggerGroup: groupName, reason: item.reason,
				}
			}
		}
	}
	requests := make([]accountFuseRequest, 0, len(requestsByAccount))
	for _, request := range requestsByAccount {
		requests = append(requests, request)
	}
	sort.SliceStable(requests, func(left, right int) bool {
		if requests[left].kind != requests[right].kind {
			return requests[left].kind == "hard"
		}
		if requests[left].healthScore != requests[right].healthScore {
			return requests[left].healthScore < requests[right].healthScore
		}
		return stableIDLess(requests[right].accountID, requests[left].accountID)
	})
	softFusedByGroup := map[string]int{}
	for _, request := range requests {
		memberships := byAccount[request.accountID]
		if accountAlreadyFused(memberships) {
			resolveAccountFuse(memberships, request, "", configs, now)
			continue
		}
		if request.kind == "soft" {
			if blockedGroup := softFuseLimitGroup(memberships, configs, softFusedByGroup, now); blockedGroup != "" {
				resolvePendingFallback(memberships, configs, "分组 "+blockedGroup+" 本轮熔断上限已用完，下轮继续评估")
				continue
			}
		}
		if blockedGroup := minimumPoolBlockedGroup(request.accountID, memberships, groups, configs, now); blockedGroup != "" {
			for _, item := range memberships {
				if item.state == "fuse_pending" {
					item.state, item.schedulable = "survivor", true
					item.reason = "保底强留：" + item.reason + "（会使分组 " + blockedGroup + " 低于保底容量）"
				}
			}
			continue
		}
		affectedGroups := fuseAffectedGroupNames(memberships, now)
		resolveAccountFuse(memberships, request, request.triggerGroup, configs, now)
		if request.kind == "soft" {
			for _, groupName := range affectedGroups {
				softFusedByGroup[groupName]++
			}
		}
	}
}

func accountAlreadyFused(items []*candidate) bool {
	for _, item := range items {
		if item.state == "fused" {
			return true
		}
	}
	return false
}

func resolveAccountFuse(items []*candidate, request accountFuseRequest, triggerGroup string, configs map[string]engineConfig, now time.Time) {
	primary := primaryMembership(items)
	fusedUntil := time.Time{}
	if primary != nil {
		fusedUntil = now.UTC().Add(configs[primary.account.GroupName].fusedCooldown)
	}
	for _, item := range items {
		if item.state == "excluded" || item.state == "paused" || item.state == "disabled" {
			continue
		}
		item.state, item.schedulable = "fused", false
		item.fusedUntil = fusedUntil
		item.reason = request.reason
	}
}

func resolvePendingFallback(items []*candidate, configs map[string]engineConfig, reason string) {
	for _, item := range items {
		if item.state != "fuse_pending" {
			continue
		}
		item.state, item.schedulable, item.reason = fallbackState(item, configs[item.account.GroupName]), remoteSchedulable(item.account), reason
	}
}

func softFuseLimitGroup(items []*candidate, configs map[string]engineConfig, used map[string]int, now time.Time) string {
	for _, groupName := range fuseAffectedGroupNames(items, now) {
		if used[groupName] >= configs[groupName].maxSwitch {
			return groupName
		}
	}
	return ""
}

func minimumPoolBlockedGroup(accountID string, memberships []*candidate, groups map[string][]*candidate, configs map[string]engineConfig, now time.Time) string {
	for _, groupName := range fuseAffectedGroupNames(memberships, now) {
		config := configs[groupName]
		available := 0
		for _, other := range groups[groupName] {
			if other.account.ID == accountID || !availableForMinimumPool(other, now) {
				continue
			}
			if other.state == "survivor" {
				available++
				continue
			}
			if other.health.SampleCount == 0 || other.health.HealthScore >= config.minPoolScore {
				available++
			}
		}
		if available < config.minPool {
			return groupName
		}
	}
	return ""
}

func fuseAffectedGroupNames(items []*candidate, now time.Time) []string {
	groups := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		if _, found := seen[item.account.GroupName]; found {
			continue
		}
		seen[item.account.GroupName] = struct{}{}
		groups = append(groups, item.account.GroupName)
	}
	sort.Strings(groups)
	return groups
}

func availableForMinimumPool(item *candidate, now time.Time) bool {
	if !item.schedulable {
		return false
	}
	switch item.state {
	case "excluded", "paused", "disabled", "fused", "binding_invalid", "cost_blocked":
		return false
	}
	kind := accountUpstreamBlock(item.account, now)
	if kind != "" && kind != "unschedulable" {
		return false
	}
	if item.state == "survivor" {
		return true
	}
	return item.account.Schedulable != nil && *item.account.Schedulable
}

func calculateGroupWeights(items []*candidate, config engineConfig) {
	eligible := make([]*candidate, 0)
	for _, item := range items {
		item.weight = 0
		if !item.schedulable || item.state == "excluded" || item.state == "paused" || item.state == "disabled" || item.state == "fused" || item.state == "cost_blocked" {
			continue
		}
		eligible = append(eligible, item)
	}
	applyPerformanceConfidence(eligible, config.performanceMinSamples, config.speedAdvantageCap)
	benchmark := strategyScoreBenchmark(eligible, config)
	qualityTotal := 0.0
	for _, item := range eligible {
		item.quality = strategyQuality(item, config, benchmark)
		qualityTotal += item.quality
	}
	for _, item := range eligible {
		if qualityTotal > 0 {
			item.weight = item.quality / qualityTotal * float64(config.weightBudget)
		} else if len(eligible) > 0 {
			item.weight = float64(config.weightBudget) / float64(len(eligible))
		}
	}
}

// assignAccountPlacements mirrors Guardian's account-level ownership model:
// group policies each contribute one weight, those weights are averaged, and
// only the stable primary group writes fields that Sub2API stores per account.
func assignAccountPlacements(
	groups map[string][]*candidate,
	configs map[string]engineConfig,
	byAccount map[string][]*candidate,
) {
	primary := map[string]*candidate{}
	for accountID, memberships := range byAccount {
		if len(memberships) == 0 {
			continue
		}
		chosen := memberships[0]
		total := 0.0
		for _, item := range memberships {
			total += item.weight
			if membershipLess(item, chosen) {
				chosen = item
			}
		}
		average := total / float64(len(memberships))
		for _, item := range memberships {
			item.weight = average
			item.rank, item.desiredPriority, item.desiredLoad, item.desiredConcurrency = nil, nil, nil, nil
		}
		primary[accountID] = chosen
	}

	for groupName, members := range groups {
		owned := make([]*candidate, 0, len(members))
		for _, item := range members {
			if primary[item.account.ID] == item {
				owned = append(owned, item)
			}
		}
		if len(owned) == 0 {
			continue
		}
		config := configs[groupName]
		sortOwnedWithHysteresis(owned, config.changeThreshold)
		priorities := make([]int64, 0, len(owned))
		for _, item := range owned {
			if item.account.BaselinePriority != nil && *item.account.BaselinePriority > 0 {
				priorities = append(priorities, *item.account.BaselinePriority)
			} else if item.account.Priority != nil && *item.account.Priority > 0 {
				priorities = append(priorities, *item.account.Priority)
			} else {
				priorities = append(priorities, 1)
			}
		}
		sort.Slice(priorities, func(left, right int) bool { return priorities[left] < priorities[right] })
		base := max(config.manualPriorityMax+1, priorities[len(priorities)/2]-int64(len(owned)/2))
		for index, item := range owned {
			rank := index + 1
			item.rank = &rank
			priority := base + int64(index)
			if item.state == "degraded" {
				priority += config.degradePriorityStep * int64(max(1, item.health.FailureStreak))
			} else if item.state == "survivor" {
				priority += config.degradePriorityStep
			}
			priority = max(config.manualPriorityMax+1, priority)
			item.desiredPriority = &priority
			if config.weightsEnabled && placementLoadFactorEligible(item) {
				average := float64(config.weightBudget) / float64(len(owned))
				scale := 1.0
				if average > 0 {
					scale = item.weight / average
				}
				midpoint := max(1.0, float64(config.minLoadFactor+config.maxLoadFactor)/2)
				desired := int64(math.Round(scale * midpoint))
				if item.state == "degraded" {
					desired = max(config.degradeMinLoad, int64(math.Round(float64(desired)*config.degradeLoadRatio)))
				} else if item.state == "survivor" {
					desired = config.minLoadFactor
				}
				desired = min(config.maxLoadFactor, max(config.minLoadFactor, desired))
				text := strconv.FormatInt(desired, 10)
				item.desiredLoad = &text
			}
		}
		// Scaling is also account-level and is therefore evaluated only once,
		// by the same primary group that owns priority and load factor.
		applyScaling(owned, config)
	}

	for accountID, memberships := range byAccount {
		owner := primary[accountID]
		if owner == nil {
			continue
		}
		for _, item := range memberships {
			if item == owner {
				continue
			}
			item.rank = cloneInt(owner.rank)
			item.desiredPriority = cloneInt64(owner.desiredPriority)
			item.desiredLoad = cloneString(owner.desiredLoad)
			item.desiredConcurrency = cloneInt64(owner.desiredConcurrency)
		}
	}
}

func placementLoadFactorEligible(item *candidate) bool {
	switch item.state {
	case "excluded", "paused", "disabled", "fused", "cost_blocked", "binding_invalid":
		return false
	default:
		return true
	}
}

func membershipLess(left, right *candidate) bool {
	leftKey, rightKey := left.account.GroupName, right.account.GroupName
	if left.account.GroupID != nil && strings.TrimSpace(*left.account.GroupID) != "" {
		leftKey = strings.TrimSpace(*left.account.GroupID)
	}
	if right.account.GroupID != nil && strings.TrimSpace(*right.account.GroupID) != "" {
		rightKey = strings.TrimSpace(*right.account.GroupID)
	}
	if leftKey != rightKey {
		return stableIDLess(leftKey, rightKey)
	}
	return left.account.GroupName < right.account.GroupName
}

func primaryMembership(items []*candidate) *candidate {
	var primary *candidate
	for _, item := range items {
		if primary == nil || membershipLess(item, primary) {
			primary = item
		}
	}
	return primary
}

func finalizeAccountStates(
	byAccount map[string][]*candidate,
	configs map[string]engineConfig,
	previous map[string]business.PreviousRoutingDecision,
	now time.Time,
) {
	for _, accountID := range sortedTargetIDsFromCandidates(byAccount) {
		memberships := byAccount[accountID]
		primary := primaryMembership(memberships)
		if primary == nil {
			continue
		}
		applyStateSince([]*candidate{primary}, previous, now)
		applyDeadband([]*candidate{primary}, previous, configs[primary.account.GroupName], now)
		for _, item := range memberships {
			if item == primary {
				continue
			}
			item.health = primary.health
			item.state = primary.state
			item.reason = primary.reason
			item.schedulable = primary.schedulable
			item.fuseKind = primary.fuseKind
			item.fusedUntil = primary.fusedUntil
			item.stateSince = primary.stateSince
			item.rank = cloneInt(primary.rank)
			item.desiredPriority = cloneInt64(primary.desiredPriority)
			item.desiredLoad = cloneString(primary.desiredLoad)
			item.desiredConcurrency = cloneInt64(primary.desiredConcurrency)
			item.writeCooldown = primary.writeCooldown
			item.scalingCooldown = primary.scalingCooldown
		}
	}
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func applyScaling(items []*candidate, config engineConfig) {
	if !config.scalingEnabled {
		return
	}
	allocated := int64(0)
	for _, item := range items {
		if item.rank != nil && item.account.Concurrency != nil && *item.account.Concurrency > 0 {
			allocated += *item.account.Concurrency
		}
	}
	headroom := max(int64(0), config.scalingGlobalMax-allocated)
	scaleUp := float64(allocated)/float64(config.scalingGlobalMax) >= config.scalingUpRatio && headroom > 0
	for _, item := range items {
		if item.rank == nil {
			continue
		}
		current := config.scalingMin
		if item.account.Concurrency != nil && *item.account.Concurrency > 0 {
			current = *item.account.Concurrency
		}
		desired := current
		if item.state == "excluded" || item.state == "paused" || item.state == "disabled" || item.state == "fused" || item.state == "cost_blocked" {
			continue
		}
		if item.state == "degraded" || item.state == "survivor" {
			desired -= config.scalingStepDown
		} else if scaleUp {
			increase := min(config.scalingStepUp, headroom)
			desired += increase
		}
		desired = min(config.scalingMax, max(config.scalingMin, desired))
		if desired > current {
			headroom -= desired - current
		}
		if desired != current {
			item.desiredConcurrency = &desired
		}
	}
}

func applyDeadband(items []*candidate, previous map[string]business.PreviousRoutingDecision, config engineConfig, now time.Time) {
	for _, item := range items {
		if item.desiredLoad != nil && loadFactorWithinThreshold(item.account, *item.desiredLoad, config.changeThreshold) {
			item.desiredLoad = cloneString(item.account.LoadFactor)
		}
		prior, found := previousDecision(previous, item.account.ID, item.account.GroupName)
		if !found {
			continue
		}
		if !prior.LastApplyAt.IsZero() && now.Sub(prior.LastApplyAt) < config.cooldown {
			item.writeCooldown = true
			item.desiredLoad = cloneString(item.account.LoadFactor)
			if item.account.Priority != nil && strings.EqualFold(strings.TrimSpace(prior.State), strings.TrimSpace(item.state)) {
				item.desiredPriority = cloneInt64(item.account.Priority)
			}
		}
		if !prior.LastApplyAt.IsZero() && now.Sub(prior.LastApplyAt) < config.scalingCooldown {
			item.scalingCooldown = true
			item.desiredConcurrency = nil
		}
	}
}

func loadFactorWithinThreshold(account business.RoutingAccount, desired string, threshold *big.Rat) bool {
	target, ok := new(big.Rat).SetString(strings.TrimSpace(desired))
	if !ok || target.Sign() < 0 || threshold == nil {
		return false
	}
	current := big.NewRat(1, 1)
	if account.LoadFactor != nil {
		if value, valid := new(big.Rat).SetString(strings.TrimSpace(*account.LoadFactor)); valid && value.Sign() > 0 {
			current = value
		} else if account.Concurrency != nil && *account.Concurrency > 0 {
			current = big.NewRat(*account.Concurrency, 1)
		}
	} else if account.Concurrency != nil && *account.Concurrency > 0 {
		current = big.NewRat(*account.Concurrency, 1)
	}
	difference := new(big.Rat).Abs(new(big.Rat).Sub(target, current))
	ratio := new(big.Rat).Quo(difference, new(big.Rat).Abs(current))
	return ratio.Cmp(threshold) < 0
}

func applyStateSince(items []*candidate, previous map[string]business.PreviousRoutingDecision, now time.Time) {
	for _, item := range items {
		prior, found := previousDecision(previous, item.account.ID, item.account.GroupName)
		if found && prior.State == item.state {
			item.stateSince = previousStateSince(prior)
		}
		if item.stateSince.IsZero() {
			item.stateSince = now.UTC()
		}
	}
}

func applyCleanupPolicy(
	byAccount map[string][]*candidate,
	config engineConfig,
	states map[string]time.Time,
	now time.Time,
) ([]business.CleanupStateWrite, []business.RuntimeEventWrite) {
	writes := make([]business.CleanupStateWrite, 0, len(byAccount))
	events := []business.RuntimeEventWrite{}
	groupMembers := map[string]map[string]struct{}{}
	for accountID, items := range byAccount {
		for _, item := range items {
			if item.state == "excluded" {
				continue
			}
			if groupMembers[item.account.GroupName] == nil {
				groupMembers[item.account.GroupName] = map[string]struct{}{}
			}
			groupMembers[item.account.GroupName][accountID] = struct{}{}
		}
	}
	selected := map[string]struct{}{}
	done := 0
	for _, accountID := range sortedTargetIDsFromCandidates(byAccount) {
		items := byAccount[accountID]
		if !config.cleanupEnabled || config.cleanupAction == "none" {
			if _, found := states[accountID]; found {
				writes = append(writes, business.CleanupStateWrite{AccountID: accountID})
			}
			continue
		}
		currentlyRateLimited := false
		for _, item := range items {
			if candidateRateLimited(item, now) {
				currentlyRateLimited = true
				break
			}
		}
		if currentlyRateLimited {
			if _, found := states[accountID]; found {
				writes = append(writes, business.CleanupStateWrite{AccountID: accountID})
			}
			continue
		}
		hits, match := cleanupAccountHits(items, config)
		if hits < config.cleanupOccurrences || cleanupAccountProtected(items, config.cleanupAction) {
			if _, found := states[accountID]; found {
				writes = append(writes, business.CleanupStateWrite{AccountID: accountID})
			}
			if hits > 0 {
				reason := fmt.Sprintf("最近 %d 条样本中%s %d 次，未达到处置阈值 %d 次", config.cleanupWindow, match, hits, config.cleanupOccurrences)
				if cleanupAccountProtected(items, config.cleanupAction) {
					reason = "账号命中自动处置条件，但当前为保底强留、暂停或排除状态"
				}
				events = append(events, cleanupRuntimeEvent("cleanup_skipped", "succeeded", accountID, reason, items))
			}
			continue
		}
		eligibleSince, observed := states[accountID]
		if !observed {
			eligibleSince = now.UTC()
			copy := eligibleSince
			writes = append(writes, business.CleanupStateWrite{AccountID: accountID, EligibleSince: &copy})
		}
		if now.Sub(eligibleSince) < config.cleanupObservation {
			remaining := config.cleanupObservation - now.Sub(eligibleSince)
			events = append(events, cleanupRuntimeEvent("cleanup_skipped", "succeeded", accountID,
				fmt.Sprintf("账号已满足自动处置条件，仍需观察 %d 分钟", max(1, int(math.Ceil(remaining.Minutes())))), items))
			continue
		}
		if done >= config.cleanupMaxPerRound {
			events = append(events, cleanupRuntimeEvent("cleanup_deferred", "succeeded", accountID, "本轮自动处置数量已达到上限，下轮继续评估", items))
			continue
		}
		if config.cleanupKeepLast && cleanupWouldEmptyGroup(accountID, items, groupMembers, selected) {
			events = append(events, cleanupRuntimeEvent("cleanup_skipped", "succeeded", accountID, "账号是分组内最后一个可保留账号，已跳过自动处置", items))
			continue
		}
		action := config.cleanupAction
		for _, item := range items {
			item.cleanupAction = &action
		}
		selected[accountID] = struct{}{}
		done++
		events = append(events, cleanupRuntimeEvent("cleanup_queued", "succeeded", accountID,
			fmt.Sprintf("账号已进入自动处置队列：%s；最近样本%s %d 次", cleanupActionLabel(action), match, hits), items))
	}
	return writes, events
}

func cleanupAccountHits(items []*candidate, config engineConfig) (int, string) {
	best := 0
	label := "命中致命错误"
	if len(config.cleanupStatusCodes) > 0 {
		codes := make([]int, 0, len(config.cleanupStatusCodes))
		for code := range config.cleanupStatusCodes {
			codes = append(codes, code)
		}
		sort.Ints(codes)
		label = fmt.Sprintf("命中状态码 %v", codes)
	} else if config.cleanupOnlyAuth {
		label = "命中认证失效"
	}
	for _, item := range items {
		hits := 0
		limit := min(config.cleanupWindow, len(item.rows))
		for index, row := range item.rows[:limit] {
			status := routingSampleStatus(row)
			text := strings.ToLower(strings.TrimSpace(row.Result + " " + row.FailureReason))
			switch {
			case len(config.cleanupStatusCodes) > 0:
				if status != nil {
					_, matched := config.cleanupStatusCodes[*status]
					if matched {
						hits++
					}
				}
			case config.cleanupOnlyAuth:
				if cleanupAuthFailure(status, text) {
					hits++
				}
			case index < len(item.health.Events) && item.health.Events[index] == EventCredentialBad:
				hits++
			}
		}
		best = max(best, hits)
	}
	return best, label
}

func cleanupAuthFailure(status *int, text string) bool {
	if containsAnyText(text, "quota", "balance", "insufficient", "usage limit", "credit", "额度", "余额", "欠费") {
		return false
	}
	if status != nil && (*status == 401 || *status == 403) {
		return true
	}
	return containsAnyText(text, "unauthorized", "forbidden", "invalid api key", "authentication", "account not found", "no api key", "no access token", "expired", "鉴权", "认证", "凭据", "密钥失效")
}

func cleanupAccountProtected(items []*candidate, action string) bool {
	if len(items) == 0 {
		return true
	}
	for _, item := range items {
		if item.state == "survivor" || item.state == "excluded" ||
			action == "pause" && item.state == "paused" ||
			action == "disable" && item.state == "disabled" {
			return true
		}
	}
	return false
}

func cleanupWouldEmptyGroup(accountID string, items []*candidate, members map[string]map[string]struct{}, selected map[string]struct{}) bool {
	for _, item := range items {
		remaining := 0
		for memberID := range members[item.account.GroupName] {
			if memberID == accountID {
				continue
			}
			if _, removed := selected[memberID]; !removed {
				remaining++
			}
		}
		if remaining == 0 {
			return true
		}
	}
	return false
}

func cleanupRuntimeEvent(eventType, status, accountID, summary string, items []*candidate) business.RuntimeEventWrite {
	groups := make([]string, 0, len(items))
	name := accountID
	if len(items) > 0 {
		name = items[0].account.Name
	}
	for _, item := range items {
		groups = append(groups, item.account.GroupName)
	}
	return business.RuntimeEventWrite{
		EventType: eventType, Status: status, Summary: summary,
		Payload: map[string]any{"account_id": accountID, "account_name": name, "groups": uniqueSorted(groups)},
	}
}

func cleanupActionLabel(action string) string {
	switch action {
	case "pause":
		return "暂停"
	case "disable":
		return "停用"
	case "delete":
		return "删除"
	default:
		return action
	}
}

func sortedTargetIDsFromCandidates(values map[string][]*candidate) []string {
	result := make([]string, 0, len(values))
	for accountID := range values {
		result = append(result, accountID)
	}
	sort.Slice(result, func(left, right int) bool { return stableIDLess(result[left], result[right]) })
	return result
}

func routingSampleStatus(row business.RoutingSample) *int {
	raw, present := row.Payload["status_code"]
	if present && raw != nil {
		value, err := exactInteger(raw)
		if err == nil && value >= 100 && value <= 599 {
			return &value
		}
	}
	match := statusPattern.FindStringSubmatch(strings.ToLower(row.Result + " " + row.FailureReason))
	if len(match) != 2 {
		return nil
	}
	value, err := strconv.Atoi(match[1])
	if err != nil {
		return nil
	}
	return &value
}

func containsAnyText(value string, markers ...string) bool {
	for _, marker := range markers {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func previousStateSince(previous business.PreviousRoutingDecision) time.Time {
	if raw, present := previous.Payload["state_since"]; present {
		if parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(fmt.Sprint(raw))); err == nil {
			return parsed.UTC()
		}
	}
	if parsed, err := time.Parse(time.RFC3339Nano, previous.UpdatedAt); err == nil {
		return parsed.UTC()
	}
	return time.Time{}
}

func previousFusedUntil(previous business.PreviousRoutingDecision) time.Time {
	raw, present := previous.Payload["fused_until"]
	if !present || raw == nil {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(fmt.Sprint(raw)))
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func aggregateTargets(values map[string][]*candidate) map[string]business.AccountRoutingTarget {
	result := map[string]business.AccountRoutingTarget{}
	for accountID, items := range values {
		primary := primaryMembership(items)
		if primary == nil {
			continue
		}
		groups := make([]string, 0, len(items))
		for _, item := range items {
			groups = append(groups, item.account.GroupName)
		}
		target := business.AccountRoutingTarget{
			AccountID: accountID, GroupNames: uniqueSorted(groups), DesiredHealth: primary.state,
			Priority: cloneInt64(primary.desiredPriority), LoadFactor: cloneString(primary.desiredLoad),
			Concurrency: cloneInt64(primary.desiredConcurrency), WriteCooldown: primary.writeCooldown,
			ScalingCooldown: primary.scalingCooldown, CleanupAction: cloneString(primary.cleanupAction),
		}
		if primary.state != "excluded" {
			schedulable := primary.schedulable
			target.Schedulable = &schedulable
		} else {
			target.ReleaseControl = true
		}
		result[accountID] = target
	}
	return result
}

func publicDecision(item *candidate, groupName string, config engineConfig) Decision {
	var fusedUntil *string
	if !item.fusedUntil.IsZero() {
		value := item.fusedUntil.UTC().Format(time.RFC3339Nano)
		fusedUntil = &value
	}
	return Decision{
		AccountID: item.account.ID, GroupName: groupName, GroupID: item.account.GroupID,
		Priority: item.desiredPriority, Schedulable: item.schedulable, Role: item.state, RoutingState: item.state,
		Rank: item.rank, Reason: item.reason, HealthScore: item.health.HealthScore,
		RoutingHealthScore: item.routingHealth, ShortScore: item.health.ShortScore,
		LongScore: item.health.LongScore, SampleCount: item.health.SampleCount, TTFBP50MS: item.health.P50MS,
		TTFBP95MS: item.health.P95MS, LatestEvent: item.health.LatestEvent, Strategy: item.strategy,
		Rate: item.rateText, RateKnown: item.rateKnown, Weight: round4(item.weight), DesiredLoadFactor: item.desiredLoad,
		DesiredConcurrency: item.desiredConcurrency, CostWall: item.costWallText, CostTier: item.costTier,
		RateReason: item.rateReason, WriteCooldownActive: item.writeCooldown, ScalingCooldownActive: item.scalingCooldown,
		RecoveryTarget: config.recoveryTarget, StateSince: item.stateSince.UTC().Format(time.RFC3339Nano), FusedUntil: fusedUntil,
		CleanupAction: item.cleanupAction,
	}
}

func evaluationWrite(item *candidate, groupName string) business.RoutingEvaluationWrite {
	result := business.RoutingEvaluationWrite{
		AccountID: item.account.ID, GroupName: groupName, SampleCount: item.health.SampleCount,
		TTFBP50MS: item.health.P50MS, TTFBP95MS: item.health.P95MS, LatestEvent: string(item.health.LatestEvent),
	}
	if item.health.SampleCount > 0 {
		result.HealthScore, result.ShortScore, result.LongScore = floatPointer(item.health.HealthScore), floatPointer(item.health.ShortScore), floatPointer(item.health.LongScore)
	}
	return result
}

func decisionWrite(item Decision) business.RoutingDecisionWrite {
	payload := map[string]any{
		"health_score": item.HealthScore, "routing_health_score": item.RoutingHealthScore,
		"short_score": item.ShortScore, "long_score": item.LongScore,
		"sample_count": item.SampleCount, "ttfb_p50_ms": item.TTFBP50MS, "ttfb_p95_ms": item.TTFBP95MS,
		"strategy": item.Strategy, "rate": item.Rate, "rate_known": item.RateKnown, "weight": item.Weight,
		"desired_load_factor": item.DesiredLoadFactor, "desired_concurrency": item.DesiredConcurrency,
		"cost_wall": item.CostWall, "cost_tier": item.CostTier, "rate_reason": item.RateReason,
		"write_cooldown_active": item.WriteCooldownActive, "scaling_cooldown_active": item.ScalingCooldownActive,
		"recovery_target": item.RecoveryTarget, "state_since": item.StateSince,
		"fused_until": item.FusedUntil,
	}
	schedulable := item.Schedulable
	return business.RoutingDecisionWrite{
		AccountID: item.AccountID, GroupName: item.GroupName, Priority: item.Priority, Schedulable: &schedulable,
		Role: item.Role, State: item.RoutingState, Rank: item.Rank, Reason: item.Reason, Payload: payload,
	}
}

func resolveRate(account business.RoutingAccount, config engineConfig, wall *big.Rat) (*big.Rat, *string, bool, *string) {
	raw := account.Multiplier
	if value, text := nonnegativeDecimal(raw); value != nil {
		return value, text, true, nil
	}
	switch config.missingRateFallback {
	case "current_cost_wall":
		if wall != nil {
			text, reason := decimalText(wall), "倍率缺失，回退当前成本墙"
			return new(big.Rat).Set(wall), &text, false, &reason
		}
		reason := "倍率缺失且当前成本墙未同步，严格关闭"
		return nil, nil, false, &reason
	case "fail_open":
		text, reason := "1", "倍率缺失，按允许继续处理"
		return big.NewRat(1, 1), &text, false, &reason
	default:
		reason := "倍率缺失，严格关闭"
		return nil, nil, false, &reason
	}
}

func effectiveCostWall(account business.RoutingAccount) (*big.Rat, *string, error) {
	wall, text := nonnegativeDecimal(account.GroupCostWall)
	if wall == nil {
		return nil, nil, nil
	}
	if account.ProfitEnabled != nil && *account.ProfitEnabled {
		margin, _ := nonnegativeDecimal(account.ProfitMinMargin)
		buffer, _ := nonnegativeDecimal(account.ProfitSafetyBuffer)
		if margin == nil || buffer == nil {
			return nil, nil, errors.New("利润控制缺少利润率或安全缓冲")
		}
		deduction := new(big.Rat).Add(margin, buffer)
		if deduction.Cmp(big.NewRat(1, 1)) >= 0 {
			return nil, nil, errors.New("利润率与安全缓冲之和必须小于 1")
		}
		wall.Mul(wall, new(big.Rat).Sub(big.NewRat(1, 1), deduction))
		value := decimalText(wall)
		text = &value
	}
	return wall, text, nil
}

func costTier(rate, wall *big.Rat) (string, int) {
	if rate == nil || wall == nil {
		return "unknown", 1
	}
	switch rate.Cmp(wall) {
	case -1:
		return "below", 0
	case 1:
		return "above", 2
	default:
		return "equal", 1
	}
}

func candidateStrategyScores(item *candidate, config engineConfig) strategyScores {
	multiplier := 1.0
	if item.rate != nil && item.rate.Sign() > 0 {
		multiplier, _ = item.rate.Float64()
	}
	priceScore := 1 / max(multiplier, math.SmallestNonzeroFloat64)
	latencySeconds := 1.0
	if item.rankingLatencyMS > 0 {
		latencySeconds = max(item.rankingLatencyMS/1000, .05)
	} else if item.health.P95MS != nil && *item.health.P95MS > 0 {
		latencySeconds = max(*item.health.P95MS/1000, .05)
	} else if item.health.P50MS != nil && *item.health.P50MS > 0 {
		latencySeconds = max(*item.health.P50MS/1000, .05)
	}
	speedScore := 1 / latencySeconds
	return strategyScores{
		price: math.Pow(priceScore, config.priceExp),
		speed: math.Pow(speedScore, config.speedExp),
	}
}

func strategyScoreBenchmark(items []*candidate, config engineConfig) strategyScores {
	// Price and latency use different units, so compare each account with the
	// best eligible account in its group before mixing the two dimensions.
	benchmark := strategyScores{}
	for _, item := range items {
		scores := candidateStrategyScores(item, config)
		benchmark.price = max(benchmark.price, scores.price)
		benchmark.speed = max(benchmark.speed, scores.speed)
	}
	return benchmark
}

func relativeStrategyScore(score, benchmark float64) float64 {
	if benchmark <= 0 {
		return 0
	}
	return min(1, max(0, score/benchmark))
}

func strategyQuality(item *candidate, config engineConfig, benchmark strategyScores) float64 {
	scores := candidateStrategyScores(item, config)
	priceScore := relativeStrategyScore(scores.price, benchmark.price)
	speedScore := relativeStrategyScore(scores.speed, benchmark.speed)
	routingHealth := item.routingHealth
	if routingHealth == 0 && item.health.HealthScore > 0 {
		routingHealth = item.health.HealthScore
	}
	healthValue := min(100.0, max(0.0, routingHealth))
	if healthValue <= config.gateFloor && item.state != "survivor" {
		return 0
	}
	healthGate := 1.0
	if config.gateFloor < 100 {
		healthGate = max(0.0, (healthValue-config.gateFloor)/(100-config.gateFloor))
	}
	if item.state == "survivor" {
		healthGate = max(.01, healthGate)
	}
	stability := healthValue / 100
	quality := 0.0
	switch item.strategy {
	case "price_first":
		quality = (priceScore*primaryStrategyRatio + speedScore*secondaryStrategyRatio) * healthGate
	case "speed_first":
		quality = (priceScore*secondaryStrategyRatio + speedScore*primaryStrategyRatio) * healthGate
	case "reliability":
		quality = (priceScore*.10 + speedScore*.15 + stability*.75) * healthGate
	default:
		quality = (priceScore*config.balancedPriceRatio + speedScore*(1-config.balancedPriceRatio)) * healthGate
	}
	if item.costTier == "above" {
		return 0
	}
	return max(0.0, quality)
}

func eligibleScope(account business.RoutingAccount, config engineConfig) (bool, string) {
	if _, found := config.excludedAccounts[account.ID]; found {
		return false, "账号被排除"
	}
	if config.groupExcluded(account.GroupName, account.GroupID) {
		return false, "分组被排除"
	}
	if !config.groupManaged(account.GroupName, account.GroupID) {
		return false, "分组未纳入托管范围"
	}
	return true, ""
}

func (c engineConfig) groupExcluded(name string, id *string) bool {
	return setsOverlap(groupKeys(name, id), c.excludedGroups)
}

func (c engineConfig) groupManaged(name string, id *string) bool {
	if c.managedMode != "selected" {
		return true
	}
	return setsOverlap(groupKeys(name, id), c.managedGroups)
}

func filterSource(samples []business.RoutingSample, source string) []business.RoutingSample {
	result := []business.RoutingSample{}
	for _, sample := range samples {
		normalized := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(sample.Source)), "_", "-")
		if source == "active_probe" && normalized != "active-probe" {
			continue
		}
		if source == "traffic" && normalized != "traffic" && normalized != "active-probe" {
			continue
		}
		result = append(result, sample)
	}
	return result
}

func filterAndLimitSamples(samples []business.RoutingSample, source string, limit int) []business.RoutingSample {
	filtered := filterSource(samples, source)
	if limit > 0 && len(filtered) > limit {
		return filtered[:limit]
	}
	return filtered
}

func selectRoutingEvidence(
	samples []business.RoutingSample,
	now time.Time,
	trafficMaxAge time.Duration,
	probeMaxAge time.Duration,
	limit int,
) ([]business.RoutingSample, []business.RoutingSample) {
	traffic := freshSourceSamples(samples, "traffic", now, trafficMaxAge, limit)
	probes := freshSourceSamples(samples, "active-probe", now, probeMaxAge, limit)
	performance := append([]business.RoutingSample{}, traffic...)
	if len(traffic) > 0 {
		return traffic, performance
	}
	return probes, performance
}

func withCriticalProbeEvidence(
	healthRows []business.RoutingSample,
	allRows []business.RoutingSample,
	now time.Time,
	probeMaxAge time.Duration,
	policy map[string]any,
) []business.RoutingSample {
	probes := freshSourceSamples(allRows, "active-probe", now, probeMaxAge, len(allRows))
	if len(probes) == 0 {
		return healthRows
	}
	latestProbe := probes[0]
	classified, err := ClassifySample(Sample{
		Result: latestProbe.Result, FailureReason: latestProbe.FailureReason, Source: latestProbe.Source,
		LatencyP95: latestProbe.LatencyP95, StatusCode: routingSampleStatus(latestProbe), Payload: latestProbe.Payload,
	}, policy)
	if err != nil || !classified.Fatal {
		return healthRows
	}
	probeObserved, err := time.Parse(time.RFC3339Nano, latestProbe.ObservedAt)
	if err != nil {
		return healthRows
	}
	if len(healthRows) > 0 {
		healthObserved, parseErr := time.Parse(time.RFC3339Nano, healthRows[0].ObservedAt)
		if parseErr == nil && !probeObserved.After(healthObserved) {
			return healthRows
		}
	}
	return append([]business.RoutingSample{latestProbe}, healthRows...)
}

func freshSourceSamples(
	samples []business.RoutingSample,
	source string,
	now time.Time,
	maxAge time.Duration,
	limit int,
) []business.RoutingSample {
	result := make([]business.RoutingSample, 0, min(limit, len(samples)))
	for _, sample := range samples {
		normalized := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(sample.Source)), "_", "-")
		if normalized != source {
			continue
		}
		observed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(sample.ObservedAt))
		if err != nil || observed.After(now.Add(time.Minute)) || now.Sub(observed) > maxAge {
			continue
		}
		result = append(result, sample)
	}
	sort.SliceStable(result, func(left, right int) bool {
		return result[left].ObservedAt > result[right].ObservedAt
	})
	if limit > 0 && len(result) > limit {
		return result[:limit]
	}
	return result
}

func performanceLatencySummary(samples []business.RoutingSample) (*float64, *float64, int, string) {
	latenciesByModel := map[string][]float64{}
	models := map[string]int{}
	for _, row := range samples {
		if !successfulRoutingSample(row) {
			continue
		}
		latency := latencyMS(Sample{Source: row.Source, LatencyP95: row.LatencyP95, Payload: row.Payload})
		if latency != nil {
			model := performanceModel(row.Payload)
			latenciesByModel[model] = append(latenciesByModel[model], *latency)
			models[model]++
		}
	}
	if len(latenciesByModel) == 0 {
		return nil, nil, 0, ""
	}
	model := dominantModel(models)
	latencies := latenciesByModel[model]
	sort.Float64s(latencies)
	p50 := latencies[max(0, int(math.Ceil(.50*float64(len(latencies))))-1)]
	p95 := latencies[max(0, int(math.Ceil(.95*float64(len(latencies))))-1)]
	return floatPointer(round4(p50)), floatPointer(round4(p95)), len(latencies), model
}

func applyPerformanceConfidence(items []*candidate, minimumSamples int, advantageCap float64) {
	modelValues := map[string][]float64{}
	for _, item := range items {
		latency, samples := candidatePerformanceEvidence(item)
		if latency != nil && samples >= minimumSamples {
			modelValues[item.performanceModel] = append(modelValues[item.performanceModel], *latency)
		}
	}
	if len(modelValues) == 0 {
		for _, item := range items {
			latency, _ := candidatePerformanceEvidence(item)
			if latency != nil {
				modelValues[item.performanceModel] = append(modelValues[item.performanceModel], *latency)
			}
		}
	}
	modelBaselines := map[string]float64{}
	for model, values := range modelValues {
		modelBaselines[model] = medianFloat64(values)
	}
	const neutralLatency = 1000.0
	minimumLatency := neutralLatency / max(1.0, advantageCap)
	for _, item := range items {
		latency := neutralLatency
		performanceLatency, performanceSamples := candidatePerformanceEvidence(item)
		baseline := modelBaselines[item.performanceModel]
		if performanceLatency != nil && baseline > 0 {
			latency = *performanceLatency / baseline * neutralLatency
			if performanceSamples < minimumSamples {
				confidence := float64(performanceSamples) / float64(max(1, minimumSamples))
				latency = latency*confidence + neutralLatency*(1-confidence)
			}
		}
		item.rankingLatencyMS = max(minimumLatency, latency)
	}
}

func performanceModel(payload map[string]any) string {
	for _, key := range []string{"model", "model_name", "actual_model"} {
		if value := strings.ToLower(strings.TrimSpace(fmt.Sprint(payload[key]))); value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func dominantModel(counts map[string]int) string {
	result, best := "", 0
	for model, count := range counts {
		if count > best || count == best && model < result {
			result, best = model, count
		}
	}
	return result
}

func successfulRoutingSample(row business.RoutingSample) bool {
	if strings.TrimSpace(row.FailureReason) != "" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(row.Result)) {
	case "通过", "passed", "pass", "success", "succeeded", "healthy", "ok":
		return true
	default:
		return false
	}
}

func sortOwnedWithHysteresis(items []*candidate, threshold *big.Rat) {
	sort.SliceStable(items, func(left, right int) bool {
		leftPriority, leftHasPriority := positivePriority(items[left].account.Priority)
		rightPriority, rightHasPriority := positivePriority(items[right].account.Priority)
		if leftHasPriority != rightHasPriority {
			return leftHasPriority
		}
		if leftHasPriority && leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		return stableIDLess(items[left].account.ID, items[right].account.ID)
	})
	for index := 1; index < len(items); index++ {
		for current := index; current > 0 && weightAdvantageExceeds(items[current].weight, items[current-1].weight, threshold); current-- {
			items[current], items[current-1] = items[current-1], items[current]
		}
	}
}

func positivePriority(priority *int64) (int64, bool) {
	if priority == nil || *priority <= 0 {
		return 0, false
	}
	return *priority, true
}

func weightAdvantageExceeds(challenger, incumbent float64, threshold *big.Rat) bool {
	if challenger <= incumbent {
		return false
	}
	if threshold == nil {
		return true
	}
	limit, _ := threshold.Float64()
	base := max(math.Abs(challenger), math.Abs(incumbent))
	return base > 0 && (challenger-incumbent)/base > limit
}

func candidatePerformanceEvidence(item *candidate) (*float64, int) {
	if item.performanceP95MS != nil {
		return item.performanceP95MS, item.performanceSamples
	}
	if item.health.P95MS != nil {
		return item.health.P95MS, max(1, item.health.SampleCount)
	}
	return nil, 0
}

func medianFloat64(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	ordered := append([]float64{}, values...)
	sort.Float64s(ordered)
	middle := len(ordered) / 2
	if len(ordered)%2 == 0 {
		return (ordered[middle-1] + ordered[middle]) / 2
	}
	return ordered[middle]
}

func requiredObject(parent map[string]any, key string) (map[string]any, error) {
	raw, present := parent[key]
	if !present {
		return map[string]any{}, nil
	}
	value, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s 必须是对象", key)
	}
	return value, nil
}

func strategyField(source map[string]any, key, fallback string) (string, error) {
	raw, present := source[key]
	if !present {
		raw = fallback
	}
	return normalizeStrategyName(raw)
}

func normalizeStrategyName(raw any) (string, error) {
	value, ok := raw.(string)
	if !ok {
		return "", errors.New("策略必须是字符串")
	}
	aliases := map[string]string{
		"balanced": "balanced", "price_first": "price_first", "speed_first": "speed_first", "reliability": "reliability",
	}
	result, found := aliases[strings.ToLower(strings.TrimSpace(value))]
	if !found {
		return "", errors.New("策略值无效")
	}
	return result, nil
}

type policyReader struct{ err error }

func (r *policyReader) integer(source map[string]any, field, key string, fallback, minimum, maximum int) int {
	if r.err != nil {
		return fallback
	}
	raw, present := source[key]
	if !present {
		return fallback
	}
	value, err := exactInteger(raw)
	if err != nil || value < minimum || value > maximum {
		r.err = fmt.Errorf("策略字段 %s 必须是 %d 到 %d 之间的整数", field, minimum, maximum)
		return fallback
	}
	return value
}

func (r *policyReader) nestedInteger(policy map[string]any, section, key string, fallback, minimum, maximum int) int {
	value, err := requiredObject(policy, section)
	if err != nil {
		if r.err == nil {
			r.err = err
		}
		return fallback
	}
	return r.integer(value, section+"."+key, key, fallback, minimum, maximum)
}

func (r *policyReader) boolean(source map[string]any, field, key string, fallback bool) bool {
	if r.err != nil {
		return fallback
	}
	raw, present := source[key]
	if !present {
		return fallback
	}
	value, ok := raw.(bool)
	if !ok {
		r.err = fmt.Errorf("策略字段 %s 必须是布尔值", field)
		return fallback
	}
	return value
}

func (r *policyReader) number(source map[string]any, field, key string, fallback, minimum, maximum float64) float64 {
	if r.err != nil {
		return fallback
	}
	raw, present := source[key]
	if !present {
		return fallback
	}
	value, err := strictNumber(raw)
	if err != nil || value < minimum || value > maximum {
		r.err = fmt.Errorf("策略字段 %s 必须是 %.4g 到 %.4g 之间的有限数值", field, minimum, maximum)
		return fallback
	}
	return value
}

func exactInteger(raw any) (int, error) {
	switch value := raw.(type) {
	case int:
		return value, nil
	case int64:
		return int(value), nil
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) || value != math.Trunc(value) {
			return 0, errors.New("not integer")
		}
		return int(value), nil
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || strconv.Itoa(parsed) != strings.TrimSpace(value) {
			return 0, errors.New("not integer")
		}
		return parsed, nil
	default:
		text := strings.TrimSpace(fmt.Sprint(raw))
		parsed, err := strconv.Atoi(text)
		if err != nil || strconv.Itoa(parsed) != text {
			return 0, errors.New("not integer")
		}
		return parsed, nil
	}
}

func decimalField(source map[string]any, key, fallback string, positive bool) (*big.Rat, error) {
	raw, present := source[key]
	if !present {
		raw = fallback
	}
	if _, ok := raw.(bool); ok {
		return nil, errors.New("invalid decimal")
	}
	value, ok := new(big.Rat).SetString(strings.TrimSpace(fmt.Sprint(raw)))
	if !ok || (positive && value.Sign() <= 0) || (!positive && value.Sign() < 0) {
		return nil, errors.New("invalid decimal")
	}
	return value, nil
}

func (r *policyReader) stringSet(source map[string]any, field, key string) map[string]struct{} {
	if r.err != nil {
		return map[string]struct{}{}
	}
	raw, present := source[key]
	if !present {
		return map[string]struct{}{}
	}
	list, ok := raw.([]any)
	if !ok {
		r.err = fmt.Errorf("策略字段 %s 必须是字符串数组", field)
		return map[string]struct{}{}
	}
	result := map[string]struct{}{}
	for _, item := range list {
		value, ok := item.(string)
		value = strings.ToLower(strings.TrimSpace(value))
		if !ok || value == "" {
			r.err = fmt.Errorf("策略字段 %s 只能包含非空字符串", field)
			return map[string]struct{}{}
		}
		result[value] = struct{}{}
	}
	return result
}

func integerSet(source map[string]any, key string, minimum, maximum int) (map[int]struct{}, error) {
	raw, present := source[key]
	if !present {
		return map[int]struct{}{}, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s 必须是数组", key)
	}
	result := map[int]struct{}{}
	for _, item := range list {
		value, err := exactInteger(item)
		if err != nil || value < minimum || value > maximum {
			return nil, fmt.Errorf("%s 包含无效状态码", key)
		}
		result[value] = struct{}{}
	}
	return result, nil
}

func nonnegativeDecimal(raw *string) (*big.Rat, *string) {
	if raw == nil {
		return nil, nil
	}
	value, ok := new(big.Rat).SetString(strings.TrimSpace(*raw))
	if !ok || value.Sign() < 0 {
		return nil, nil
	}
	text := decimalText(value)
	return value, &text
}

func decimalText(value *big.Rat) string {
	text := value.FloatString(12)
	text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	if text == "" || text == "-0" {
		return "0"
	}
	return text
}

func recoverySpan(rows []business.RoutingSample, successes int, now time.Time) time.Duration {
	if successes < 1 || len(rows) == 0 {
		return 0
	}
	index := min(successes, len(rows)) - 1
	observed, err := time.Parse(time.RFC3339Nano, rows[index].ObservedAt)
	if err != nil {
		return 0
	}
	return max(time.Duration(0), now.Sub(observed.UTC()))
}

func latestStatusIn(rows []business.RoutingSample, allowed map[int]struct{}) bool {
	if len(rows) == 0 || len(allowed) == 0 {
		return false
	}
	status := routingSampleStatus(rows[0])
	if status == nil {
		return false
	}
	_, found := allowed[*status]
	return found
}

func recentHTTPFailures(health Health, window int) int {
	count := 0
	for _, event := range health.Events[:min(window, len(health.Events))] {
		if event == EventGateway || event == EventProbeFailed {
			count++
		}
	}
	return count
}

func recentTransientFailures(health Health, window int) int {
	count := 0
	for _, event := range health.Events[:min(window, len(health.Events))] {
		switch event {
		case EventUnknown, EventGateway, EventProbeFailed:
			count++
		}
	}
	return count
}

func retryRecoveredOccurrences(rows []business.RoutingSample, window int) int {
	count := 0
	for _, row := range rows[:min(window, len(rows))] {
		if recovered, _ := row.Payload["retry_recovered"].(bool); recovered {
			count++
		}
	}
	return count
}

func slowOccurrences(rows []business.RoutingSample, window int, threshold float64) int {
	count := 0
	for _, row := range rows[:min(window, len(rows))] {
		if !successfulRoutingSample(row) {
			continue
		}
		value := latencyMS(Sample{Source: row.Source, LatencyP95: row.LatencyP95, Payload: row.Payload})
		if value != nil && *value > threshold {
			count++
		}
	}
	return count
}

func fallbackState(item *candidate, config engineConfig) string {
	if config.degradeEnabled {
		return "degraded"
	}
	if item.health.SampleCount > 0 {
		return "healthy"
	}
	return "unknown"
}

func routingKey(accountID, groupName string) string { return accountID + "\x00" + groupName }

func previousDecision(
	values map[string]business.PreviousRoutingDecision,
	accountID string,
	groupName string,
) (business.PreviousRoutingDecision, bool) {
	if item, found := values[routingKey(accountID, groupName)]; found {
		return item, true
	}
	item, found := values[accountID]
	return item, found
}

func stableIDLess(left, right string) bool {
	leftInt, leftOK := new(big.Int).SetString(left, 10)
	rightInt, rightOK := new(big.Int).SetString(right, 10)
	if leftOK && rightOK {
		return leftInt.Cmp(rightInt) < 0
	}
	return left < right
}

func groupKeys(name string, id *string) map[string]struct{} {
	result := map[string]struct{}{strings.ToLower(strings.TrimSpace(name)): {}}
	if id != nil {
		result[strings.ToLower(strings.TrimSpace(*id))] = struct{}{}
	}
	return result
}

func setsOverlap(left, right map[string]struct{}) bool {
	for value := range left {
		if _, found := right[value]; found {
			return true
		}
	}
	return false
}

func textMetadata(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if raw, present := value[key]; present && raw != nil {
			if text, ok := raw.(string); ok {
				return text
			}
		}
	}
	return ""
}

func pointerText(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return *value
}

func floatPointer(value float64) *float64 { return &value }

func uniqueSorted(values []string) []string {
	set := map[string]struct{}{}
	for _, value := range values {
		set[value] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func sortedTargetIDs(values map[string]business.AccountRoutingTarget) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool { return stableIDLess(result[left], result[right]) })
	return result
}
