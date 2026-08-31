package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/naming"
)

type UpstreamHost struct {
	UpstreamID    string   `json:"upstream_id"`
	Host          string   `json:"host"`
	Hosts         []string `json:"hosts"`
	BaseURL       string   `json:"base_url"`
	Name          string   `json:"name"`
	UpstreamType  string   `json:"upstream_type"`
	AccountCount  int64    `json:"account_count"`
	GroupCount    int64    `json:"group_count"`
	AuthStatus    string   `json:"auth_status"`
	RawBalance    *string  `json:"raw_balance"`
	Balance       *string  `json:"balance"`
	RechargeRate  string   `json:"recharge_rate"`
	BalanceStatus string   `json:"balance_status"`
	CheckedAt     *string  `json:"checked_at"`
}

type UpstreamSummary struct {
	Hosts              []UpstreamHost `json:"hosts"`
	TotalHosts         int            `json:"total_hosts"`
	AuthenticatedHosts int            `json:"authenticated_hosts"`
	RecoveryRequired   int            `json:"recovery_required"`
	Source             string         `json:"source"`
}

type UpstreamGroup struct {
	UpstreamID        string                 `json:"upstream_id"`
	Host              string                 `json:"host"`
	GroupID           *string                `json:"group_id"`
	Name              string                 `json:"name"`
	Description       *string                `json:"description"`
	Platform          *string                `json:"platform"`
	Status            *string                `json:"status"`
	RawRate           *string                `json:"raw_rate"`
	EffectiveRate     *string                `json:"effective_rate"`
	RechargeRate      *string                `json:"recharge_rate"`
	Bound             bool                   `json:"bound"`
	BoundAccounts     []UpstreamBoundAccount `json:"bound_accounts"`
	KeyPresent        bool                   `json:"key_present"`
	Bindable          bool                   `json:"bindable"`
	UnavailableReason *string                `json:"unavailable_reason"`
}

type UpstreamBoundAccount struct {
	BindingID       int64                  `json:"binding_id"`
	AccountID       string                 `json:"account_id"`
	AccountName     *string                `json:"account_name"`
	AccountExists   bool                   `json:"account_exists"`
	BindingStatus   *string                `json:"binding_status"`
	LocalGroup      string                 `json:"local_group"`
	LocalGroups     []LocalOnboardingGroup `json:"local_groups"`
	UpstreamKeyID   string                 `json:"upstream_key_id"`
	UpstreamKeyName string                 `json:"upstream_key_name"`
}

type GroupPolicyOverride struct {
	Enabled            *bool    `json:"enabled,omitempty"`
	Strategy           *string  `json:"strategy,omitempty"`
	MinPoolSize        *int64   `json:"min_pool_size,omitempty"`
	WeightBudget       *int64   `json:"weight_budget,omitempty"`
	BalancedPriceRatio *float64 `json:"balanced_price_ratio,omitempty"`
	BreakerEnabled     *bool    `json:"breaker_enabled,omitempty"`
	RecoveryEnabled    *bool    `json:"recovery_enabled,omitempty"`
	WeightsEnabled     *bool    `json:"weights_enabled,omitempty"`
	ScalingEnabled     *bool    `json:"scaling_enabled,omitempty"`
	ProbeEnabled       *bool    `json:"probe_enabled,omitempty"`
	ProbeInterval      *int64   `json:"probe_interval_seconds,omitempty"`
	ProbeModel         *string  `json:"probe_model,omitempty"`
}

type GroupStatus struct {
	Name                string               `json:"name"`
	ID                  *string              `json:"id"`
	Platform            *string              `json:"platform"`
	Platforms           []string             `json:"platforms"`
	RateMultiplier      *string              `json:"rate_multiplier"`
	ProbeInterval       int64                `json:"probe_interval_seconds"`
	WeightBudget        int64                `json:"weight_budget"`
	AccountCount        int64                `json:"account_count"`
	SchedulingOpen      int64                `json:"scheduling_open"`
	SchedulingClosed    int64                `json:"scheduling_closed"`
	SchedulingUnknown   int64                `json:"scheduling_unknown"`
	HealthyAccounts     int64                `json:"healthy_accounts"`
	DegradedAccounts    int64                `json:"degraded_accounts"`
	FusedAccounts       int64                `json:"fused_accounts"`
	PausedAccounts      int64                `json:"paused_accounts"`
	DisabledAccounts    int64                `json:"disabled_accounts"`
	ExcludedAccounts    int64                `json:"excluded_accounts"`
	RateLimitedAccounts int64                `json:"rate_limited_accounts"`
	PendingAccounts     int64                `json:"pending_accounts"`
	AvailableAccounts   int64                `json:"available_accounts"`
	NeedsAttention      int64                `json:"needs_attention"`
	ScoredAccounts      int64                `json:"scored_accounts"`
	AverageHealthScore  *float64             `json:"average_health_score"`
	Strategy            string               `json:"strategy"`
	StrategySource      string               `json:"strategy_source"`
	ParticipationStatus string               `json:"participation_status"`
	ParticipationReason *string              `json:"participation_reason"`
	Status              string               `json:"status"`
	Override            *GroupPolicyOverride `json:"override"`
	survivorAccounts    int64
}

func (s *Store) Upstreams(ctx context.Context) (UpstreamSummary, error) {
	// The upstream list is the ownership boundary where newly discovered host aliases
	// are reconciled, including aliases added after both hosts already had identities.
	if err := s.ensureUpstreamIdentities(ctx); err != nil {
		return UpstreamSummary{}, err
	}
	if err := s.ensureStableUpstreamRelations(ctx); err != nil {
		return UpstreamSummary{}, err
	}
	identityHosts, err := upstreamIdentityHostSets(ctx, s.db)
	if err != nil {
		return UpstreamSummary{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT h.upstream_id,u.host,u.base_url,u.upstream_type,u.auth_status,
		u.balance,u.raw_balance,u.mapped_balance,u.checked_at,u.metadata_json,
		COALESCE(r.recharge_rate,'1')
		FROM upstreams u JOIN upstream_identity_hosts h ON h.host=u.host
		LEFT JOIN recharge_rates r ON r.host=u.host WHERE h.is_primary=1 ORDER BY u.host`)
	if err != nil {
		return UpstreamSummary{}, err
	}
	defer rows.Close()
	accountCounts, err := countByString(ctx, s.db, `SELECT bi.upstream_id,COUNT(DISTINCT b.local_account_id)
		FROM binding_identities bi JOIN bindings b ON b.id=bi.binding_id GROUP BY bi.upstream_id`)
	if err != nil {
		return UpstreamSummary{}, err
	}
	groupCounts, err := countByString(ctx, s.db, `SELECT host,COUNT(*) FROM upstream_groups GROUP BY host`)
	if err != nil {
		return UpstreamSummary{}, err
	}
	derivedNames, err := s.derivedUpstreamNames(ctx)
	if err != nil {
		return UpstreamSummary{}, err
	}
	result := UpstreamSummary{Hosts: []UpstreamHost{}, Source: "Console 业务库"}
	for rows.Next() {
		var item UpstreamHost
		var legacyBalance sql.NullFloat64
		var rawBalance, mappedBalance, checkedAt sql.NullString
		var metadataRaw string
		if err := rows.Scan(
			&item.UpstreamID, &item.Host, &item.BaseURL, &item.UpstreamType, &item.AuthStatus, &legacyBalance,
			&rawBalance, &mappedBalance, &checkedAt, &metadataRaw, &item.RechargeRate,
		); err != nil {
			return UpstreamSummary{}, err
		}
		metadata, metadataErr := decodeObject(metadataRaw)
		item.Name = upstreamDisplayName(metadata, item.BaseURL, derivedNames[item.Host])
		item.Hosts = append([]string{}, identityHosts[item.UpstreamID]...)
		item.RawBalance = nullString(rawBalance)
		if item.RawBalance == nil && legacyBalance.Valid {
			legacy := normalizeDecimal(strconv.FormatFloat(legacyBalance.Float64, 'f', -1, 64))
			if legacy != nil && isNewAPI(item.UpstreamType) && stringValue(metadata["balance_unit"]) != "usd" {
				quota := stringValue(metadata["quota_per_unit"])
				if quota == "" {
					quota = "500000"
				}
				legacy = divideDecimalPointers(legacy, normalizePositiveDecimal(quota))
			}
			item.RawBalance = legacy
		}
		item.Balance = nullString(mappedBalance)
		if item.Balance == nil {
			item.Balance = divideDecimalPointers(item.RawBalance, normalizePositiveDecimal(item.RechargeRate))
		}
		item.CheckedAt = nullString(checkedAt)
		item.AccountCount = accountCounts[item.UpstreamID]
		item.GroupCount = groupCounts[item.Host]
		if metadataErr != nil {
			item.AuthStatus = "配置错误"
			item.BalanceStatus = "配置错误"
		} else {
			item.AuthStatus = effectiveAuthStatus(item.AuthStatus, item.RawBalance, item.CheckedAt, metadata)
			item.BalanceStatus = effectiveBalanceStatus(item.RawBalance, metadata)
		}
		result.Hosts = append(result.Hosts, item)
	}
	if err := rows.Err(); err != nil {
		return UpstreamSummary{}, err
	}
	result.TotalHosts = len(result.Hosts)
	for _, item := range result.Hosts {
		if authenticatedStatus(item.AuthStatus) {
			result.AuthenticatedHosts++
		}
	}
	result.RecoveryRequired = result.TotalHosts - result.AuthenticatedHosts
	return result, nil
}

func (s *Store) UpstreamGroups(ctx context.Context, host string, includeBound bool) ([]UpstreamGroup, error) {
	normalized := canonicalHost(host)
	upstreamID, err := s.upstreamIdentityID(ctx, normalized)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	var authStatus string
	var mappedBalance, legacyBalance sql.NullString
	var metadataRaw, upstreamUpdatedAt string
	err = s.db.QueryRowContext(ctx, `SELECT auth_status,mapped_balance,CAST(balance AS TEXT),metadata_json,updated_at
		FROM upstreams WHERE host=?`, normalized).Scan(&authStatus, &mappedBalance, &legacyBalance, &metadataRaw, &upstreamUpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	metadata, metadataErr := decodeObject(metadataRaw)
	hostReason := upstreamUnavailableReason(authStatus, mappedBalance, legacyBalance, metadata, metadataErr)
	if snapshotReason, snapshotErr := s.authRecoveryFailure(ctx, normalized, upstreamUpdatedAt); snapshotErr != nil {
		return nil, snapshotErr
	} else if hostReason == nil && snapshotReason != nil {
		hostReason = snapshotReason
	}
	var rechargeRaw sql.NullString
	err = s.db.QueryRowContext(ctx, `SELECT recharge_rate FROM recharge_rates WHERE host=?`, normalized).Scan(&rechargeRaw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	recharge := "1"
	if err == nil {
		if rechargeRaw.Valid {
			recharge = rechargeRaw.String
		} else {
			recharge = ""
		}
	}
	rechargeValue := normalizePositiveDecimal(recharge)
	if rechargeValue == nil && hostReason == nil {
		hostReason = stringPointer("倍率不可用")
	}
	boundAccounts, err := s.upstreamBoundAccounts(ctx, normalized)
	if err != nil {
		return nil, err
	}
	keyStates, err := s.upstreamKeyStates(ctx, normalized)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT g.host,g.group_id,g.name,g.description,g.platform,
		CASE WHEN e.lifecycle_state IN ('suspected','missing','retired') THEN e.lifecycle_state
			ELSE COALESCE(e.observed_status,g.status) END,g.raw_rate,g.effective_rate
		FROM upstream_groups g LEFT JOIN upstream_catalog_entities e ON e.upstream_id=? AND e.entity_kind='group' AND e.entity_id=g.group_id
		WHERE g.host=? ORDER BY g.name,g.group_id`, upstreamID, normalized)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]UpstreamGroup, 0)
	for rows.Next() {
		var item UpstreamGroup
		var groupID string
		var description, platform, status, rawRate, effectiveRate sql.NullString
		if err := rows.Scan(&item.Host, &groupID, &item.Name, &description, &platform, &status, &rawRate, &effectiveRate); err != nil {
			return nil, err
		}
		item.GroupID = stringPointer(groupID)
		item.UpstreamID = upstreamID
		item.Description = nullString(description)
		item.Platform = nullString(platform)
		item.Status = nullString(status)
		item.RawRate = nullString(rawRate)
		item.EffectiveRate = nullString(effectiveRate)
		if item.RawRate != nil && *item.RawRate != "" {
			if converted := divideMultiplierPointers(normalizeDecimal(*item.RawRate), rechargeValue); converted != nil {
				item.EffectiveRate = converted
			}
		}
		item.RechargeRate = stringPointer(recharge)
		item.BoundAccounts = append([]UpstreamBoundAccount{}, boundAccounts[groupID]...)
		item.Bound = len(item.BoundAccounts) > 0
		state, keyPresent := keyStates[groupID]
		item.KeyPresent = keyPresent
		if !includeBound && item.Bound {
			continue
		}
		var rowReason *string
		if item.Status == nil {
			rowReason = stringPointer("上游分组状态不可读")
		}
		item.UnavailableReason = hostReason
		if item.UnavailableReason == nil {
			item.UnavailableReason = rowReason
		}
		item.Bindable = activeStatus(pointerValue(item.Status)) && activeStatus(state) && !item.Bound && item.UnavailableReason == nil
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) upstreamBoundAccounts(ctx context.Context, host string) (map[string][]UpstreamBoundAccount, error) {
	if err := s.ensureStableUpstreamRelations(ctx); err != nil {
		return nil, err
	}
	upstreamID, err := s.upstreamIdentityID(ctx, host)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT b.id,b.upstream_group_id,b.local_account_id,a.name,
		a.id IS NOT NULL AND COALESCE(b.status,'')<>'missing',b.status,b.local_group,b.upstream_key_id,b.upstream_key_name
		FROM bindings b JOIN binding_identities bi ON bi.binding_id=b.id LEFT JOIN accounts a ON a.id=b.local_account_id
		WHERE bi.upstream_id=? AND b.upstream_group_id IS NOT NULL
		ORDER BY b.upstream_group_id,a.name,b.local_account_id,b.id`, upstreamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string][]UpstreamBoundAccount{}
	accountIDs := map[string]struct{}{}
	for rows.Next() {
		var groupID string
		var accountName, bindingStatus sql.NullString
		var accountExists int64
		var item UpstreamBoundAccount
		if err := rows.Scan(
			&item.BindingID, &groupID, &item.AccountID, &accountName, &accountExists, &bindingStatus,
			&item.LocalGroup, &item.UpstreamKeyID, &item.UpstreamKeyName,
		); err != nil {
			return nil, err
		}
		item.AccountName = nullString(accountName)
		item.AccountExists = accountExists == 1
		item.BindingStatus = nullString(bindingStatus)
		item.LocalGroups = []LocalOnboardingGroup{}
		result[groupID] = append(result[groupID], item)
		if item.AccountExists {
			accountIDs[item.AccountID] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(accountIDs) == 0 {
		return result, nil
	}
	ids := make([]string, 0, len(accountIDs))
	for accountID := range accountIDs {
		ids = append(ids, accountID)
	}
	sort.Strings(ids)
	groupPlaceholders, groupArguments := sqlStringArguments(ids)
	groupRows, err := s.db.QueryContext(ctx, `SELECT ag.account_id,COALESCE(ag.group_id,lg.remote_id),ag.group_name
		FROM account_groups ag LEFT JOIN local_groups lg ON lg.name=ag.group_name
		WHERE ag.account_id IN (`+groupPlaceholders+`) AND COALESCE(ag.group_id,lg.remote_id) IS NOT NULL
		ORDER BY ag.account_id,ag.group_name`, groupArguments...)
	if err != nil {
		return nil, err
	}
	defer groupRows.Close()
	memberships := map[string][]LocalOnboardingGroup{}
	for groupRows.Next() {
		var accountID, groupID, groupName string
		if err := groupRows.Scan(&accountID, &groupID, &groupName); err != nil {
			return nil, err
		}
		memberships[accountID] = append(memberships[accountID], LocalOnboardingGroup{ID: groupID, Name: groupName})
	}
	if err := groupRows.Err(); err != nil {
		return nil, err
	}
	for groupID, accounts := range result {
		for index := range accounts {
			accounts[index].LocalGroups = append([]LocalOnboardingGroup{}, memberships[accounts[index].AccountID]...)
		}
		result[groupID] = accounts
	}
	return result, nil
}

func (s *Store) equivalentUpstreamHosts(ctx context.Context, host string) ([]string, error) {
	if err := s.ensureUpstreamIdentities(ctx); err != nil {
		return nil, err
	}
	_, hosts, err := upstreamIdentityHostsForQueryer(ctx, s.db, host)
	if err != nil {
		return nil, err
	}
	return hosts, nil
}

func (s *Store) Groups(ctx context.Context) ([]GroupStatus, error) {
	control, err := s.readPolicyDocument(ctx, s.db, "control-plane")
	if err != nil {
		return nil, err
	}
	accounts, err := s.groupAccountProjections(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT name,remote_id,platform,rate_multiplier,strategy,strategy_source FROM local_groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]GroupStatus, 0)
	now := time.Now().UTC()
	for rows.Next() {
		var item GroupStatus
		var remoteID, platform, rateMultiplier, strategy, strategySource sql.NullString
		if err := rows.Scan(&item.Name, &remoteID, &platform, &rateMultiplier, &strategy, &strategySource); err != nil {
			return nil, err
		}
		item.ID = nullString(remoteID)
		item.Platform = nullString(platform)
		item.RateMultiplier = nullString(rateMultiplier)
		if item.Platform != nil {
			item.Platforms = []string{*item.Platform}
		} else {
			item.Platforms = []string{}
		}
		item.Strategy, item.StrategySource = groupStrategy(control, item.ID, nullString(strategy), nullString(strategySource))
		item.ParticipationStatus, item.ParticipationReason = groupParticipation(control, item.ID, item.Name)
		item.Override = groupOverride(control, item.ID)
		item.ProbeInterval = effectiveGroupProbeInterval(control, item.Override)
		item.WeightBudget = effectiveGroupWeightBudget(control, item.Override)
		minPoolSize := effectiveGroupMinPoolSize(control, item.Override)
		minPoolScore := effectiveGroupMinPoolScore(control)
		managedAccounts := make([]accountProjection, 0)
		for index := range accounts {
			if !containsString(accounts[index].Groups, item.Name) {
				continue
			}
			if !groupAccountMetadataManaged(accounts[index], control) {
				continue
			}
			managedAccounts = append(managedAccounts, accounts[index])
			item.AccountCount++
			switch {
			case accounts[index].Schedulable == nil:
				item.SchedulingUnknown++
			case *accounts[index].Schedulable:
				item.SchedulingOpen++
			default:
				item.SchedulingClosed++
			}
			classifyGroupAccount(&item, accounts[index], now)
		}
		item.AvailableAccounts = availableGroupAccounts(managedAccounts, item.Name, minPoolScore, now)
		item.NeedsAttention = max(int64(0), item.DegradedAccounts-item.RateLimitedAccounts) + item.FusedAccounts + item.PausedAccounts + item.DisabledAccounts + item.ExcludedAccounts
		item.AverageHealthScore = averageGroupScores(managedAccounts, item.Name, &item.ScoredAccounts)
		item.Status = groupRuntimeStatus(item, groupExplicitlyExcluded(control, item.ID, item.Name), minPoolSize)
		result = append(result, item)
	}
	return result, rows.Err()
}

func effectiveGroupMinPoolSize(control map[string]any, override *GroupPolicyOverride) int64 {
	if override != nil && override.MinPoolSize != nil && *override.MinPoolSize >= 0 {
		return *override.MinPoolSize
	}
	if raw, present := lookupPolicyPath(control, "breaker.min_pool_size"); present {
		if value, err := strictInteger(raw); err == nil && value >= 0 {
			return int64(value)
		}
	}
	return 1
}

func effectiveGroupMinPoolScore(control map[string]any) float64 {
	if raw, present := lookupPolicyPath(control, "breaker.min_pool_score"); present {
		if value := finiteFloat(raw); value != nil && *value >= 0 && *value <= 100 {
			return *value
		}
	}
	return 3
}

func effectiveGroupProbeInterval(control map[string]any, override *GroupPolicyOverride) int64 {
	if override != nil && override.ProbeInterval != nil && *override.ProbeInterval > 0 {
		return *override.ProbeInterval
	}
	if raw, present := lookupPolicyPath(control, "probe.interval_seconds"); present {
		if value, err := strictInteger(raw); err == nil && value > 0 {
			return int64(value)
		}
	}
	return 300
}

func effectiveGroupWeightBudget(control map[string]any, override *GroupPolicyOverride) int64 {
	if override != nil && override.WeightBudget != nil && *override.WeightBudget > 0 {
		return *override.WeightBudget
	}
	if raw, present := lookupPolicyPath(control, "weights.budget"); present {
		if value, err := strictInteger(raw); err == nil && value > 0 {
			return int64(value)
		}
	}
	return 400
}

func countByString(ctx context.Context, db *sql.DB, query string, arguments ...any) (map[string]int64, error) {
	rows, err := db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]int64{}
	for rows.Next() {
		var key string
		var count int64
		if err := rows.Scan(&key, &count); err != nil {
			return nil, err
		}
		result[key] = count
	}
	return result, rows.Err()
}

func (s *Store) derivedUpstreamNames(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT upstream_host,name FROM accounts
		WHERE upstream_host IS NOT NULL AND name<>''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pattern := regexp.MustCompile(`^(.+?)-\d+(?:\.\d+)?$`)
	counts := map[string]map[string]int{}
	for rows.Next() {
		var host, name string
		if err := rows.Scan(&host, &name); err != nil {
			return nil, err
		}
		match := pattern.FindStringSubmatch(strings.TrimSpace(name))
		if len(match) == 2 && strings.TrimSpace(match[1]) != "" {
			if counts[host] == nil {
				counts[host] = map[string]int{}
			}
			counts[host][strings.TrimSpace(match[1])]++
		}
	}
	result := map[string]string{}
	for host, names := range counts {
		candidates := make([]string, 0, len(names))
		for name := range names {
			candidates = append(candidates, name)
		}
		sort.Strings(candidates)
		for _, name := range candidates {
			if result[host] == "" || names[name] > names[result[host]] || (names[name] == names[result[host]] && name > result[host]) {
				result[host] = name
			}
		}
	}
	return result, rows.Err()
}

func upstreamDisplayName(metadata map[string]any, baseURL, derived string) string {
	for _, field := range []string{"site_name", "system_name"} {
		if value := strings.TrimSpace(stringValue(metadata[field])); value != "" && !naming.IsDefaultSiteName(value) {
			return value
		}
	}
	if brand := naming.DomainBrand(baseURL); brand != "上游" {
		return brand
	}
	if derived != "" {
		return derived
	}
	return "未读取"
}

func effectiveAuthStatus(current string, rawBalance, checkedAt *string, metadata map[string]any) string {
	if strings.TrimSpace(current) == "已鉴权" {
		_, verified := metadata["auth_verified_at"]
		rateSync := stringValue(metadata["rate_sync_status"]) == "succeeded"
		if !verified && !rateSync && (checkedAt == nil || rawBalance == nil) {
			return "待验证"
		}
	}
	return current
}

func effectiveBalanceStatus(rawBalance *string, metadata map[string]any) string {
	if value, present := metadata["balance_hard_closed"]; present {
		closed := strictAnyBool(value)
		if closed == nil {
			return "配置错误"
		}
		if *closed {
			return "余额硬关闭"
		}
	}
	if rawBalance != nil {
		return "已读取"
	}
	if value, present := metadata["balance_status"]; present {
		if value == nil {
			return "空值"
		}
		if value == "" {
			return "空字符串"
		}
		return fmt.Sprint(value)
	}
	return "未读取"
}

func upstreamUnavailableReason(authStatus string, mappedBalance, legacyBalance sql.NullString, metadata map[string]any, metadataErr error) *string {
	if metadataErr != nil {
		return stringPointer("上游配置记录损坏")
	}
	normalized := strings.ToLower(strings.TrimSpace(authStatus))
	if normalized == "" {
		return stringPointer("上游鉴权状态不可读")
	}
	for _, marker := range []string{"失效", "失败", "未鉴权", "过期", "unauthorized", "expired", "invalid"} {
		if strings.Contains(normalized, marker) {
			return stringPointer("上游鉴权不可用")
		}
	}
	if !authenticatedStatus(authStatus) {
		return stringPointer("上游鉴权状态不可识别")
	}
	if value, present := metadata["balance_hard_closed"]; present {
		closed := strictAnyBool(value)
		if closed == nil {
			return stringPointer("上游余额关闭状态配置无效")
		}
		if *closed {
			return stringPointer("上游余额已硬关闭")
		}
	}
	balance := mappedBalance
	if !balance.Valid {
		balance = legacyBalance
	}
	if balance.Valid && normalizeDecimal(balance.String) == nil {
		return stringPointer("上游余额不可读")
	}
	return nil
}

func (s *Store) authRecoveryFailure(ctx context.Context, host, upstreamUpdatedAt string) (*string, error) {
	var raw, snapshotUpdatedAt string
	err := s.db.QueryRowContext(ctx, `SELECT value_json,updated_at FROM operational_snapshots
		WHERE namespace='sub2api' AND state_key='auth-recovery-runtime-snapshot'
		ORDER BY updated_at DESC LIMIT 1`).Scan(&raw, &snapshotUpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !timestampAfter(snapshotUpdatedAt, upstreamUpdatedAt) {
		return nil, nil
	}
	var snapshot map[string]any
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return nil, nil
	}
	results, ok := snapshot["results"].([]any)
	if !ok {
		return nil, nil
	}
	for _, rawResult := range results {
		result, ok := rawResult.(map[string]any)
		if !ok {
			continue
		}
		success, present := result["success"]
		if !present || success == true {
			continue
		}
		resultHost := canonicalHost(stringValue(result["host"]))
		if resultHost == "" {
			resultHost = canonicalHost(stringValue(result["base_url"]))
		}
		if resultHost != host {
			continue
		}
		for _, field := range []string{"reason", "code"} {
			if value, present := result[field]; present {
				text := "空值"
				if value != nil {
					text = fmt.Sprint(value)
				}
				if len(text) > 300 {
					text = text[:300]
				}
				return stringPointer(text), nil
			}
		}
		return stringPointer("上游鉴权恢复失败"), nil
	}
	return nil, nil
}

func timestampAfter(candidate, baseline string) bool {
	candidateTime, candidateErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(candidate))
	baselineTime, baselineErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(baseline))
	if candidateErr != nil || baselineErr != nil {
		return true
	}
	return candidateTime.After(baselineTime)
}

func (s *Store) upstreamKeyStates(ctx context.Context, host string) (map[string]string, error) {
	upstreamID, err := s.upstreamIdentityID(ctx, host)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT parent_entity_id,
		CASE WHEN lifecycle_state='active' THEN observed_status ELSE lifecycle_state END
		FROM upstream_catalog_entities WHERE upstream_id=? AND entity_kind='key'`, upstreamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var group, status sql.NullString
		if err := rows.Scan(&group, &status); err != nil {
			return nil, err
		}
		if group.Valid && group.String != "" {
			result[group.String] = status.String
		}
	}
	return result, rows.Err()
}

func groupStrategy(control map[string]any, groupID, current, currentSource *string) (string, string) {
	if control == nil {
		return "配置错误", "configuration_error"
	}
	if groupID != nil {
		if bindings, ok := control["group_policy_bindings"].(map[string]any); ok {
			if binding, ok := bindings[*groupID].(map[string]any); ok {
				if value, present := binding["strategy"]; present {
					return visibleStrategy(value), "group_override"
				}
			}
		}
	}
	if currentSource != nil && *currentSource == "group_override" {
		if current == nil {
			return "空值", "group_override"
		}
		return visibleStrategy(*current), "group_override"
	}
	if selection, ok := control["selection"].(map[string]any); ok {
		if value, present := selection["strategy"]; present {
			return visibleStrategy(value), "global_default"
		}
	}
	if value, present := control["strategy"]; present {
		return visibleStrategy(value), "global_default"
	}
	return "balanced", "global_default"
}

func visibleStrategy(value any) string {
	if value == nil {
		return "空值"
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" {
		return "空字符串"
	}
	return text
}

func groupParticipation(control map[string]any, groupID *string, groupName string) (string, *string) {
	if control == nil {
		return "configuration_error", stringPointer("控制面策略不可用，无法判断参与范围")
	}
	scope, ok := control["scope"].(map[string]any)
	if !ok {
		if _, present := control["scope"]; present {
			return "configuration_error", stringPointer("参与守护范围配置无效")
		}
		scope = map[string]any{}
	}
	mode := stringValue(scope["managed_group_mode"])
	if mode == "" {
		mode = "all"
	}
	if mode != "all" && mode != "selected" {
		return "configuration_error", stringPointer("参与守护模式无效")
	}
	keys := map[string]struct{}{strings.ToLower(strings.TrimSpace(groupName)): {}}
	if groupID != nil {
		keys[strings.ToLower(strings.TrimSpace(*groupID))] = struct{}{}
	}
	excluded, valid := scopeStringSet(scope, "excluded_group_ids")
	if !valid {
		return "configuration_error", stringPointer("排除分组列表配置无效")
	}
	if setsIntersect(keys, excluded) {
		return "out_of_scope", stringPointer("分组位于排除分组列表中")
	}
	managed, valid := scopeStringSet(scope, "managed_group_ids")
	if !valid {
		return "configuration_error", stringPointer("参与分组列表配置无效")
	}
	if mode == "selected" && !setsIntersect(keys, managed) {
		return "out_of_scope", stringPointer("当前仅守护指定分组，该分组未加入参与分组列表")
	}
	return "participating", nil
}

func groupExplicitlyExcluded(control map[string]any, groupID *string, groupName string) bool {
	if control == nil {
		return false
	}
	scope, ok := control["scope"].(map[string]any)
	if !ok {
		return false
	}
	excluded, valid := scopeStringSet(scope, "excluded_group_ids")
	if !valid {
		return false
	}
	keys := map[string]struct{}{strings.ToLower(strings.TrimSpace(groupName)): {}}
	if groupID != nil {
		keys[strings.ToLower(strings.TrimSpace(*groupID))] = struct{}{}
	}
	return setsIntersect(keys, excluded)
}

func scopeStringSet(scope map[string]any, fields ...string) (map[string]struct{}, bool) {
	result := map[string]struct{}{}
	for _, field := range fields {
		raw, present := scope[field]
		if !present {
			continue
		}
		values, ok := raw.([]any)
		if !ok {
			return nil, false
		}
		for _, value := range values {
			text, ok := value.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return nil, false
			}
			result[strings.ToLower(strings.TrimSpace(text))] = struct{}{}
		}
	}
	return result, true
}

func setsIntersect(left, right map[string]struct{}) bool {
	for value := range left {
		if _, found := right[value]; found {
			return true
		}
	}
	return false
}

func groupOverride(control map[string]any, groupID *string) *GroupPolicyOverride {
	if control == nil || groupID == nil {
		return nil
	}
	bindings, ok := control["group_policy_bindings"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := bindings[*groupID].(map[string]any)
	if !ok {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var result GroupPolicyOverride
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil
	}
	return &result
}

func classifyGroupAccount(group *GroupStatus, account accountProjection, now time.Time) {
	switch account.Health {
	case "fused":
		group.FusedAccounts++
		return
	case "cost_blocked":
		group.DegradedAccounts++
		return
	case "paused":
		group.PausedAccounts++
		return
	case "disabled":
		group.DisabledAccounts++
		return
	case "excluded":
		group.ExcludedAccounts++
		return
	case "survivor":
		group.DegradedAccounts++
		group.survivorAccounts++
		return
	}
	metadata, err := decodeObject(account.metadataRaw)
	if err == nil {
		switch AccountUpstreamBlock(metadata, account.Schedulable, now) {
		case AccountBlockRateLimited:
			group.DegradedAccounts++
			group.RateLimitedAccounts++
			return
		case AccountBlockDisabled:
			group.DisabledAccounts++
			return
		case "":
		default:
			group.ExcludedAccounts++
			return
		}
	}
	sampleRateLimited := strings.EqualFold(strings.TrimSpace(account.latestEvents[group.Name]), "rate_limited_or_exhausted")
	switch account.Health {
	case "degraded":
		group.DegradedAccounts++
		if sampleRateLimited {
			group.RateLimitedAccounts++
		}
	case "healthy", "active", "available", "normal":
		group.HealthyAccounts++
	default:
		group.PendingAccounts++
	}
}

func availableGroupAccounts(accounts []accountProjection, groupName string, minScore float64, now time.Time) int64 {
	var count int64
	for _, account := range accounts {
		if !containsString(account.Groups, groupName) {
			continue
		}
		switch account.Health {
		case "fused", "cost_blocked", "paused", "disabled", "excluded":
			continue
		}
		metadata, err := decodeObject(account.metadataRaw)
		if err != nil || AccountUpstreamBlock(metadata, account.Schedulable, now) != "" {
			continue
		}
		if account.Health == "survivor" {
			count++
			continue
		}
		if account.Schedulable == nil || !*account.Schedulable {
			continue
		}
		if account.HealthScore == nil || *account.HealthScore >= minScore {
			count++
		}
	}
	return count
}

func groupAccountMetadataManaged(account accountProjection, control map[string]any) bool {
	if control == nil {
		return true
	}
	scope, ok := control["scope"].(map[string]any)
	if !ok {
		return true
	}
	accountTypes, typesValid := scopeStringSet(scope, "account_types")
	platforms, platformsValid := scopeStringSet(scope, "platforms")
	if !typesValid || !platformsValid {
		return true
	}
	metadata, err := decodeObject(account.metadataRaw)
	if err != nil {
		return false
	}
	accountType := strings.ToLower(strings.TrimSpace(evidenceMetadataText(metadata, "type", "account_type")))
	if accountType == "" {
		accountType = "apikey"
	}
	platform := strings.ToLower(strings.TrimSpace(evidenceMetadataText(metadata, "platform")))
	if platform == "" && account.UpstreamType != nil {
		platform = strings.ToLower(strings.TrimSpace(*account.UpstreamType))
	}
	if len(accountTypes) > 0 {
		if _, found := accountTypes[accountType]; !found {
			return false
		}
	}
	if len(platforms) > 0 {
		if _, found := platforms[platform]; !found {
			return false
		}
	}
	return true
}

func averageGroupScores(accounts []accountProjection, groupName string, count *int64) *float64 {
	values := []float64{}
	for _, account := range accounts {
		if containsString(account.Groups, groupName) && account.HealthScore != nil {
			values = append(values, *account.HealthScore)
		}
	}
	*count = int64(len(values))
	if len(values) == 0 {
		return nil
	}
	total := 0.0
	for _, value := range values {
		total += value
	}
	result := mathRoundOne(total / float64(len(values)))
	return &result
}

func groupRuntimeStatus(group GroupStatus, excluded bool, minPoolSize int64) string {
	if excluded {
		return "excluded"
	}
	if group.ParticipationStatus == "configuration_error" {
		return "configuration_error"
	}
	if group.ParticipationStatus != "participating" {
		return "skipped"
	}
	if group.AccountCount == 0 {
		return "empty"
	}
	unavailable := group.FusedAccounts + group.PausedAccounts + group.DisabledAccounts + group.ExcludedAccounts
	if unavailable >= group.AccountCount {
		return "all_fused"
	}
	if group.survivorAccounts > 0 && group.AvailableAccounts <= minPoolSize {
		return "survivor_only"
	}
	if unavailable > 0 || group.DegradedAccounts > group.RateLimitedAccounts {
		return "partial_degraded"
	}
	if group.RateLimitedAccounts > 0 {
		return "rate_limited"
	}
	return "healthy"
}

func normalizeDecimal(raw string) *string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return stringPointer("")
	}
	rational, ok := new(big.Rat).SetString(text)
	if !ok {
		return nil
	}
	return stringPointer(decimalRatText(rational))
}

func normalizePositiveDecimal(raw string) *string {
	value := normalizeDecimal(raw)
	if value == nil || *value == "" {
		return nil
	}
	rational, ok := new(big.Rat).SetString(*value)
	if !ok || rational.Sign() <= 0 {
		return nil
	}
	return value
}

func divideDecimalPointers(numerator, denominator *string) *string {
	if numerator == nil || denominator == nil || *numerator == "" || *denominator == "" {
		return nil
	}
	left, leftOK := new(big.Rat).SetString(*numerator)
	right, rightOK := new(big.Rat).SetString(*denominator)
	if !leftOK || !rightOK || right.Sign() == 0 {
		return nil
	}
	return stringPointer(decimalRatText(new(big.Rat).Quo(left, right)))
}

// ConvertMultiplier applies the recharge ratio and rounds the resulting
// multiplier to six decimal places so upstream floating-point noise is not
// persisted or written to managed accounts.
func ConvertMultiplier(rawRate, rechargeRate string) (string, error) {
	raw, rawOK := new(big.Rat).SetString(strings.TrimSpace(rawRate))
	recharge, rechargeOK := new(big.Rat).SetString(strings.TrimSpace(rechargeRate))
	if !rawOK || !rechargeOK || raw.Sign() <= 0 || recharge.Sign() <= 0 {
		return "", errors.New("倍率换算参数必须是有限正数")
	}
	text := strings.TrimRight(strings.TrimRight(new(big.Rat).Quo(raw, recharge).FloatString(6), "0"), ".")
	if text == "" || text == "0" {
		return "", errors.New("换算后的倍率小于可用精度")
	}
	return text, nil
}

func divideMultiplierPointers(numerator, denominator *string) *string {
	if numerator == nil || denominator == nil || *numerator == "" || *denominator == "" {
		return nil
	}
	text, err := ConvertMultiplier(*numerator, *denominator)
	if err != nil {
		return nil
	}
	return stringPointer(text)
}

func decimalRatText(value *big.Rat) string {
	text := value.FloatString(28)
	if strings.Contains(text, ".") {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}
	if text == "-0" || text == "" {
		return "0"
	}
	return text
}

func strictAnyBool(value any) *bool {
	switch item := value.(type) {
	case bool:
		result := item
		return &result
	case json.Number:
		if item.String() == "0" || item.String() == "1" {
			result := item.String() == "1"
			return &result
		}
	case float64:
		if item == 0 || item == 1 {
			result := item == 1
			return &result
		}
	case string:
		normalized := strings.ToLower(strings.TrimSpace(item))
		truthy := map[string]bool{"1": true, "true": true, "yes": true, "on": true, "enabled": true, "开启": true, "启用": true}
		falsy := map[string]bool{"0": true, "false": true, "no": true, "off": true, "disabled": true, "关闭": true, "禁用": true}
		if truthy[normalized] {
			result := true
			return &result
		}
		if falsy[normalized] {
			result := false
			return &result
		}
	}
	return nil
}

func authenticatedStatus(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	for _, healthy := range []string{"已鉴权", "已发现鉴权记录", "已恢复", "已认证", "authenticated", "authorized", "healthy", "valid", "ok", "succeeded"} {
		if normalized == strings.ToLower(healthy) {
			return true
		}
	}
	return false
}

func activeStatus(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized == "active" || normalized == "enabled" || normalized == "available" || normalized == "ok" || normalized == "1"
}

func isNewAPI(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized == "newapi" || normalized == "oneapi"
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func pointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func mathRoundOne(value float64) float64 {
	parsed, _ := strconv.ParseFloat(strconv.FormatFloat(value, 'f', 1, 64), 64)
	return parsed
}
