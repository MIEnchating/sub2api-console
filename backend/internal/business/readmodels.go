package business

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/runtimepolicy"
)

type AccountRecentResult struct {
	Result                *string        `json:"result"`
	EventType             *string        `json:"event_type"`
	Score                 *float64       `json:"score"`
	ObservedAt            *string        `json:"observed_at"`
	LatencyMS             *float64       `json:"latency_ms"`
	DurationMS            *float64       `json:"duration_ms"`
	FailureReason         *string        `json:"failure_reason"`
	Source                string         `json:"source"`
	ClassificationLatency *string        `json:"-"`
	ClassificationPayload map[string]any `json:"-"`
}

type AccountStatus struct {
	ID                          string                `json:"id"`
	Name                        string                `json:"name"`
	Groups                      []string              `json:"groups"`
	UpstreamID                  *string               `json:"upstream_id"`
	UpstreamHost                *string               `json:"upstream_host"`
	RecordedUpstreamHost        *string               `json:"recorded_upstream_host"`
	UpstreamHostRepairable      bool                  `json:"upstream_host_repairable"`
	UpstreamType                *string               `json:"upstream_type"`
	BaseURL                     *string               `json:"base_url"`
	BaseURLCheckedAt            *string               `json:"base_url_checked_at"`
	BaseURLSource               *string               `json:"base_url_source"`
	UpstreamBaseURL             *string               `json:"upstream_base_url"`
	BaseURLCheck                string                `json:"base_url_check"`
	BaseURLCheckReason          *string               `json:"base_url_check_reason"`
	KeyStatus                   *string               `json:"key_status"`
	KeyStatusReason             *string               `json:"key_status_reason"`
	Sub2APIStatus               *string               `json:"sub2api_status"`
	Sub2APIError                *string               `json:"sub2api_error"`
	Platform                    *string               `json:"platform"`
	AccountType                 *string               `json:"account_type"`
	Schedulable                 *bool                 `json:"schedulable"`
	Priority                    *int64                `json:"priority"`
	ManualPriority              *int64                `json:"manual_priority"`
	ManualSyncBalanceMultiplier bool                  `json:"manual_sync_balance_multiplier"`
	LoadFactor                  *string               `json:"load_factor"`
	Concurrency                 *int64                `json:"concurrency"`
	Multiplier                  *string               `json:"multiplier"`
	Balance                     *string               `json:"balance"`
	Paused                      *bool                 `json:"paused"`
	PausedReason                *string               `json:"paused_reason"`
	RoutingState                *string               `json:"routing_state"`
	HealthStatus                *string               `json:"health_status"`
	Health                      string                `json:"health"`
	DesiredHealth               *string               `json:"desired_health"`
	ApplyPending                bool                  `json:"apply_pending"`
	ApplyError                  *string               `json:"apply_error"`
	DecisionState               *string               `json:"decision_state"`
	DecisionReason              *string               `json:"decision_reason"`
	LastError                   *string               `json:"last_error"`
	UpstreamBlock               *string               `json:"upstream_block"`
	UpstreamBlockReason         *string               `json:"upstream_block_reason"`
	FailureStreak               *int64                `json:"failure_streak"`
	RecoveryPassStreak          *int64                `json:"recovery_pass_streak"`
	TargetPriority              *int64                `json:"target_priority"`
	TargetLoadFactor            *string               `json:"target_load_factor"`
	TargetSchedulable           *bool                 `json:"target_schedulable"`
	TargetConcurrency           *int64                `json:"target_concurrency"`
	HealthScore                 *float64              `json:"health_score"`
	ShortScore                  *float64              `json:"short_score"`
	LongScore                   *float64              `json:"long_score"`
	SampleCount                 int64                 `json:"sample_count"`
	RecentResults               []AccountRecentResult `json:"recent_results"`
	TTFBP50MS                   *float64              `json:"ttfb_p50_ms"`
	TTFBP95MS                   *float64              `json:"ttfb_p95_ms"`
	Weight                      *float64              `json:"weight"`
}

type AccountBinding struct {
	ID               int64   `json:"id"`
	LocalAccountID   string  `json:"local_account_id"`
	UpstreamID       string  `json:"upstream_id"`
	UpstreamHost     string  `json:"upstream_host"`
	UpstreamKeyID    string  `json:"upstream_key_id"`
	UpstreamKeyName  string  `json:"upstream_key_name"`
	UpstreamGroup    *string `json:"upstream_group"`
	UpstreamGroupID  *string `json:"upstream_group_id"`
	LocalGroup       string  `json:"local_group"`
	LocalRate        *string `json:"local_rate"`
	UpstreamRate     *string `json:"upstream_rate"`
	SourceAuthHost   *string `json:"source_auth_host,omitempty"`
	BindingHostAlias *string `json:"binding_host_alias,omitempty"`
	Description      *string `json:"description"`
	Status           *string `json:"status"`
	UpdatedAt        string  `json:"updated_at"`
}

type AccountDetail struct {
	AccountStatus
	Metadata   map[string]any     `json:"metadata"`
	GroupRates map[string]*string `json:"group_rates"`
	GroupIDs   map[string]*string `json:"group_ids"`
	Bindings   []AccountBinding   `json:"bindings"`
	TestModel  *string            `json:"test_model"`
}

type accountProjection struct {
	AccountStatus
	metadataRaw  string
	groupIDs     map[string]*string
	groupRates   map[string]*string
	latestEvents map[string]string
}

type decisionProjection struct {
	state     string
	reason    *string
	updatedAt *string
	weight    *float64
}

type evaluationProjection struct {
	healthScore *float64
	shortScore  *float64
	longScore   *float64
	sampleCount int64
	p50         *float64
	p95         *float64
}

type routingApplyView struct {
	fields    map[string]bool
	automatic bool
}

type accountProjectionOptions struct {
	includeRecentEvidence bool
	includeApplyState     bool
	accountID             string
}

func (s *Store) Accounts(ctx context.Context) ([]AccountStatus, error) {
	projections, err := s.accountProjections(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]AccountStatus, len(projections))
	for index := range projections {
		result[index] = projections[index].AccountStatus
	}
	return result, nil
}

func (s *Store) Account(ctx context.Context, accountID string) (*AccountDetail, error) {
	normalized := strings.TrimSpace(accountID)
	if !positiveNumericID(normalized) {
		return nil, errors.New("账号必须使用有效的稳定 ID")
	}
	projections, err := s.accountProjectionsWithOptions(ctx, accountProjectionOptions{
		includeRecentEvidence: true,
		includeApplyState:     true,
		accountID:             normalized,
	})
	if err != nil {
		return nil, err
	}
	var selected *accountProjection
	for index := range projections {
		if projections[index].ID == normalized {
			selected = &projections[index]
			break
		}
	}
	if selected == nil {
		return nil, sql.ErrNoRows
	}
	metadata, err := decodeObject(selected.metadataRaw)
	if err != nil {
		metadata = map[string]any{"_invalid_configuration": []string{"account.metadata_json"}}
	}
	bindings, err := s.accountBindings(ctx, normalized)
	if err != nil {
		return nil, err
	}
	var testModel *string
	if policy, policyErr := s.readPolicyDocument(ctx, s.db, "control-plane"); policyErr == nil && policy != nil {
		if models, ok := policy["account_test_models"].(map[string]any); ok {
			if value, ok := models[normalized].(string); ok && strings.TrimSpace(value) != "" {
				normalizedModel := strings.TrimSpace(value)
				testModel = &normalizedModel
			}
		}
	}
	return &AccountDetail{
		AccountStatus: selected.AccountStatus,
		Metadata:      metadata,
		GroupRates:    selected.groupRates,
		GroupIDs:      selected.groupIDs,
		Bindings:      bindings,
		TestModel:     testModel,
	}, nil
}

func (s *Store) accountBindings(ctx context.Context, accountID string) ([]AccountBinding, error) {
	if err := s.ensureStableUpstreamRelations(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT b.id,b.local_account_id,COALESCE(bi.upstream_id,''),b.upstream_host,b.upstream_key_id,
		b.upstream_key_name,b.upstream_group,b.upstream_group_id,b.local_group,b.local_rate,b.upstream_rate,
		b.source_auth_host,b.binding_host_alias,b.description,b.status,b.updated_at
		FROM bindings b LEFT JOIN binding_identities bi ON bi.binding_id=b.id
		WHERE b.local_account_id=? ORDER BY b.id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AccountBinding, 0)
	for rows.Next() {
		var item AccountBinding
		var upstreamGroup, upstreamGroupID, localRate, upstreamRate sql.NullString
		var sourceAuthHost, bindingHostAlias, description, status sql.NullString
		if err := rows.Scan(
			&item.ID, &item.LocalAccountID, &item.UpstreamID, &item.UpstreamHost, &item.UpstreamKeyID, &item.UpstreamKeyName,
			&upstreamGroup, &upstreamGroupID, &item.LocalGroup, &localRate, &upstreamRate,
			&sourceAuthHost, &bindingHostAlias, &description, &status, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.UpstreamGroup = nullString(upstreamGroup)
		item.UpstreamGroupID = nullString(upstreamGroupID)
		item.LocalRate = nullString(localRate)
		item.UpstreamRate = nullString(upstreamRate)
		item.SourceAuthHost = nullString(sourceAuthHost)
		item.BindingHostAlias = nullString(bindingHostAlias)
		item.Description = nullString(description)
		item.Status = nullString(status)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) accountProjections(ctx context.Context) ([]accountProjection, error) {
	return s.accountProjectionsWithOptions(ctx, accountProjectionOptions{
		includeRecentEvidence: true,
		includeApplyState:     true,
	})
}

func (s *Store) groupAccountProjections(ctx context.Context) ([]accountProjection, error) {
	return s.accountProjectionsWithOptions(ctx, accountProjectionOptions{})
}

func (s *Store) accountProjectionsWithOptions(ctx context.Context, options accountProjectionOptions) ([]accountProjection, error) {
	if err := s.ensureStableUpstreamRelations(ctx); err != nil {
		return nil, err
	}
	query := `SELECT a.id,a.name,COALESCE(ownership.upstream_host,a.upstream_host),a.upstream_host,
		ownership.upstream_host,ownership.upstream_id,COALESCE(ownership.identity_count,0),COALESCE(u.upstream_type,a.upstream_type),u.base_url,a.schedulable,a.priority,
		a.load_factor,a.concurrency,a.multiplier,a.balance,a.paused,a.paused_reason,a.routing_state,a.health_status,
		a.failure_streak,a.recovery_pass_streak,a.target_priority,a.target_load_factor,a.target_schedulable,
		a.target_concurrency,a.metadata_json,m.priority,m.sync_balance_multiplier FROM accounts a
		LEFT JOIN (
			SELECT b.local_account_id,MIN(COALESCE(primary_host.host,b.upstream_host)) AS upstream_host,
				MIN(bi.upstream_id) AS upstream_id,
				COUNT(DISTINCT bi.upstream_id) AS identity_count
			FROM bindings b JOIN binding_identities bi ON bi.binding_id=b.id
			LEFT JOIN upstream_identity_hosts primary_host ON primary_host.upstream_id=bi.upstream_id AND primary_host.is_primary=1
			WHERE TRIM(b.upstream_host)<>'' GROUP BY b.local_account_id
		) ownership ON ownership.local_account_id=a.id
		LEFT JOIN upstreams u ON u.host=COALESCE(ownership.upstream_host,a.upstream_host)
		LEFT JOIN manual_priority_accounts m ON m.account_id=a.id`
	arguments := []any{}
	if options.accountID != "" {
		query += ` WHERE a.id=?`
		arguments = append(arguments, options.accountID)
	}
	query += ` ORDER BY CASE WHEN a.id GLOB '[0-9]*' THEN CAST(a.id AS INTEGER) ELSE 0 END,a.id`
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	projections := make([]accountProjection, 0)
	for rows.Next() {
		var item accountProjection
		var upstreamID, upstreamHost, recordedUpstreamHost, bindingUpstreamHost sql.NullString
		var upstreamType, upstreamBaseURL, loadFactor, multiplier, balance sql.NullString
		var pausedReason, routingState, healthStatus, targetLoadFactor sql.NullString
		var schedulable, paused, targetSchedulable sql.NullInt64
		var priority, concurrency, failureStreak, recoveryPassStreak sql.NullInt64
		var targetPriority, targetConcurrency, manualPriority, manualSyncBalanceMultiplier, bindingHostCount sql.NullInt64
		if err := rows.Scan(
			&item.ID, &item.Name, &upstreamHost, &recordedUpstreamHost, &bindingUpstreamHost, &upstreamID, &bindingHostCount,
			&upstreamType, &upstreamBaseURL, &schedulable, &priority,
			&loadFactor, &concurrency, &multiplier, &balance, &paused, &pausedReason, &routingState,
			&healthStatus, &failureStreak, &recoveryPassStreak, &targetPriority, &targetLoadFactor,
			&targetSchedulable, &targetConcurrency, &item.metadataRaw, &manualPriority, &manualSyncBalanceMultiplier,
		); err != nil {
			return nil, err
		}
		item.Groups = []string{}
		item.RecentResults = []AccountRecentResult{}
		item.groupIDs = map[string]*string{}
		item.groupRates = map[string]*string{}
		item.latestEvents = map[string]string{}
		item.UpstreamHost = normalizedHost(nullString(upstreamHost))
		item.UpstreamID = nullString(upstreamID)
		item.RecordedUpstreamHost = normalizedHost(nullString(recordedUpstreamHost))
		bindingHost := normalizedHost(nullString(bindingUpstreamHost))
		item.UpstreamHostRepairable = bindingHostCount.Valid && bindingHostCount.Int64 == 1 && bindingHost != nil &&
			(item.RecordedUpstreamHost == nil || !strings.EqualFold(*item.RecordedUpstreamHost, *bindingHost))
		item.UpstreamType = nullString(upstreamType)
		item.Platform = accountMetadataText(item.metadataRaw, "platform")
		item.AccountType = accountMetadataText(item.metadataRaw, "account_type", "type")
		item.BaseURL = accountMetadataText(item.metadataRaw, "base_url")
		item.BaseURLCheckedAt = accountMetadataText(item.metadataRaw, "base_url_checked_at")
		item.BaseURLSource = accountMetadataText(item.metadataRaw, "base_url_source")
		item.Sub2APIStatus = accountMetadataText(item.metadataRaw, "status")
		item.Sub2APIError = accountMetadataText(item.metadataRaw, "error_message")
		item.UpstreamBaseURL = nullString(upstreamBaseURL)
		item.BaseURLCheck, item.BaseURLCheckReason = accountBaseURLCheck(
			item.BaseURL, item.UpstreamBaseURL, item.UpstreamHost, item.BaseURLCheckedAt, item.BaseURLSource,
		)
		if item.AccountType == nil {
			item.AccountType = item.UpstreamType
		}
		item.Schedulable = strictBool(schedulable)
		item.LastError = accountMetadataText(item.metadataRaw, "error_message", "last_error")
		if metadata, decodeErr := decodeObject(item.metadataRaw); decodeErr == nil {
			block, reason := AccountUpstreamBlockDetails(metadata, item.Schedulable, time.Now())
			if block != "" {
				item.UpstreamBlock = stringPointer(block)
				item.UpstreamBlockReason = stringPointer(reason)
			}
		}
		item.Priority = nullInt(priority)
		item.ManualPriority = nullInt(manualPriority)
		item.ManualSyncBalanceMultiplier = manualSyncBalanceMultiplier.Valid && manualSyncBalanceMultiplier.Int64 == 1
		item.LoadFactor = nullString(loadFactor)
		item.Concurrency = nullInt(concurrency)
		item.Multiplier = nullString(multiplier)
		item.Balance = nullString(balance)
		item.Paused = strictBool(paused)
		item.PausedReason = nullString(pausedReason)
		item.RoutingState = nullString(routingState)
		item.HealthStatus = nullString(healthStatus)
		item.FailureStreak = nullInt(failureStreak)
		item.RecoveryPassStreak = nullInt(recoveryPassStreak)
		item.TargetPriority = nullInt(targetPriority)
		item.TargetLoadFactor = nullString(targetLoadFactor)
		item.TargetSchedulable = strictBool(targetSchedulable)
		item.TargetConcurrency = nullInt(targetConcurrency)
		projections = append(projections, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	byID := make(map[string]*accountProjection, len(projections))
	for index := range projections {
		byID[projections[index].ID] = &projections[index]
	}
	if err := s.loadAccountGroups(ctx, byID); err != nil {
		return nil, err
	}
	if err := s.loadAccountKeyStatuses(ctx, byID); err != nil {
		return nil, err
	}
	decisions, err := s.loadAccountDecisions(ctx, byID)
	if err != nil {
		return nil, err
	}
	evaluations, err := s.loadAccountEvaluations(ctx, byID)
	if err != nil {
		return nil, err
	}
	if options.includeRecentEvidence {
		if err := s.loadRecentEvidence(ctx, byID); err != nil {
			return nil, err
		}
		for index := range projections {
			item := &projections[index]
			if item.LastError != nil {
				continue
			}
			for _, result := range item.RecentResults {
				if result.FailureReason != nil {
					item.LastError = result.FailureReason
					break
				}
			}
		}
	}
	mode, err := s.Mode(ctx)
	if err != nil {
		return nil, err
	}
	applyErrors := map[string]struct {
		message string
		at      *string
	}{}
	applyView := routingApplyView{fields: map[string]bool{}}
	if options.includeApplyState {
		applyErrors, err = s.loadApplyErrors(ctx, options.accountID)
		if err != nil {
			return nil, err
		}
		applyView, err = s.routingApplyView(ctx, mode)
		if err != nil {
			return nil, err
		}
	}
	excludedIDs, degradeThreshold, err := s.monitorPolicy(ctx, mode)
	if err != nil {
		return nil, err
	}
	for index := range projections {
		item := &projections[index]
		applyAccountCalculations(item, decisions[item.ID], evaluations[item.ID], applyErrors[item.ID], applyView)
		if mode == runtimepolicy.Monitoring {
			applyMonitoringHealth(item, excludedIDs, degradeThreshold)
		}
	}
	return projections, nil
}

func (s *Store) loadAccountKeyStatuses(ctx context.Context, accounts map[string]*accountProjection) error {
	for _, account := range accounts {
		status, reason := "unbound", "账号没有上游 Key 绑定记录"
		account.KeyStatus, account.KeyStatusReason = &status, &reason
	}
	states, err := s.accountCatalogBindingStates(ctx)
	if err != nil {
		return err
	}
	for accountID, state := range states {
		if accounts[accountID] == nil {
			continue
		}
		status, reason := state.Status, state.Reason
		accounts[accountID].KeyStatus, accounts[accountID].KeyStatusReason = &status, &reason
	}
	return nil
}

func boundUpstreamKeyState(
	keyID string,
	groupID, groupName *string,
	keyPresent bool,
	keyStatus *string,
	groupPresent bool,
	groupStatus *string,
) (string, string) {
	groupReference := ""
	if groupID != nil {
		groupReference = strings.TrimSpace(*groupID)
	}
	if groupReference == "" && groupName != nil {
		groupReference = strings.TrimSpace(*groupName)
	}
	keyState := normalizedCatalogStatus(keyStatus)
	groupState := normalizedCatalogStatus(groupStatus)
	keyMissing := !keyPresent || keyState == "missing" || keyState == "deleted"
	groupMissing := groupReference != "" && (!groupPresent || groupState == "missing" || groupState == "deleted")
	if keyMissing && groupMissing {
		return "key_and_group_missing", fmt.Sprintf("上游 Key %s 和所属分组 %s 均已删除或不存在", keyID, groupReference)
	}
	if groupMissing {
		return "group_missing", fmt.Sprintf("上游 Key %s 仍有绑定，但所属分组 %s 已删除或不存在", keyID, groupReference)
	}
	if keyMissing {
		return "key_missing", fmt.Sprintf("绑定的上游 Key %s 已删除或不存在", keyID)
	}
	if groupState == "inactive" || groupState == "disabled" || groupState == "2" {
		return "group_inactive", fmt.Sprintf("上游 Key %s 所属分组 %s 已停用", keyID, groupReference)
	}
	if keyState == "" {
		return "unknown", fmt.Sprintf("上游 Key %s 存在，但未返回状态", keyID)
	}
	return keyState, fmt.Sprintf("上游 Key %s 状态为 %s，所属分组仍存在", keyID, keyState)
}

func normalizedCatalogStatus(value *string) string {
	if value == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(*value))
}

func accountBaseURLCheck(baseURL, upstreamBaseURL, upstreamHost, checkedAt, source *string) (string, *string) {
	if baseURL == nil || strings.TrimSpace(*baseURL) == "" {
		if checkedAt == nil || strings.TrimSpace(*checkedAt) == "" {
			reason := "尚未读取管理平台账号详情，请执行 Base URL 校验"
			return "unchecked", &reason
		}
		reason := "已读取管理平台账号详情，但接口未提供 Base URL，无法与上游地址比较"
		return "unknown", &reason
	}
	accountURL, accountOK := parseHTTPBaseURL(*baseURL)
	if !accountOK {
		reason := "账号 Base URL 不是有效的 HTTP/HTTPS 地址"
		return "invalid", &reason
	}
	comparisonURL := upstreamBaseURL
	if comparisonURL == nil || strings.TrimSpace(*comparisonURL) == "" {
		comparisonURL = upstreamHostURL(upstreamHost)
	}
	if comparisonURL == nil {
		reason := "账号 Base URL 已读取，但账号没有可用的归属上游 Host"
		return "unknown", &reason
	}
	upstreamURL, upstreamOK := parseHTTPBaseURL(*comparisonURL)
	if !upstreamOK {
		reason := "归属上游 Host 或访问地址不是有效的 HTTP/HTTPS 地址"
		return "invalid", &reason
	}
	if strings.EqualFold(accountURL.Host, upstreamURL.Host) {
		reason := "账号 Base URL 与上游地址使用同一 Host"
		if source != nil && strings.TrimSpace(*source) == "platform_default" {
			reason = "账号未显式配置 Base URL，Sub2API 使用的平台默认地址与上游使用同一 Host"
		}
		return "matched", &reason
	}
	if knownOfficialAPIHost(accountURL.Hostname()) && !knownOfficialAPIHost(upstreamURL.Hostname()) {
		reason := "上游地址不是官方服务，但账号 Base URL 指向官方地址，请检查添加账号时的 Base URL"
		if source != nil && strings.TrimSpace(*source) == "platform_default" {
			reason = "账号未显式配置 Base URL，Sub2API 将使用平台默认官方地址；该地址与账号归属上游不一致"
		}
		return "official_mismatch", &reason
	}
	reason := "账号 Base URL 与归属上游 Host 不同；允许独立 API 域名、IP 直连或 CDN 加速，不自动判错"
	return "different_allowed", &reason
}

func normalizedHost(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" || normalized == "-" || normalized == "—" || strings.EqualFold(normalized, "null") {
		return nil
	}
	return &normalized
}

func upstreamHostURL(host *string) *string {
	host = normalizedHost(host)
	if host == nil {
		return nil
	}
	value := *host
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	return &value
}

func parseHTTPBaseURL(value string) (*url.URL, bool) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	validScheme := parsed != nil && (strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https"))
	return parsed, err == nil && validScheme && parsed.Host != "" && parsed.User == nil
}

func knownOfficialAPIHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	suffixes := []string{
		"openai.com", "chatgpt.com", "anthropic.com", "googleapis.com", "google.com", "x.ai",
		"deepseek.com", "moonshot.cn", "bigmodel.cn",
	}
	for _, suffix := range suffixes {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

func (s *Store) routingApplyView(ctx context.Context, mode string) (routingApplyView, error) {
	result := routingApplyView{fields: map[string]bool{}, automatic: mode == runtimepolicy.Full}
	if mode == runtimepolicy.Monitoring {
		return result, nil
	}
	policy, err := s.readPolicyDocument(ctx, s.db, "control-plane")
	if err != nil {
		return routingApplyView{}, err
	}
	rawAutoApply, present := policy["auto_apply"]
	if !present {
		for _, field := range []string{"schedulable", "priority", "load_factor", "concurrency"} {
			result.fields[field] = true
		}
		return result, nil
	}
	autoApply, ok := rawAutoApply.(map[string]any)
	if !ok {
		return routingApplyView{}, errors.New("调度策略 auto_apply 配置无效")
	}
	for _, field := range []string{"schedulable", "priority", "load_factor", "concurrency"} {
		raw, present := autoApply[field]
		if !present {
			continue
		}
		value, ok := raw.(bool)
		if !ok {
			return routingApplyView{}, fmt.Errorf("调度策略 auto_apply.%s 配置无效", field)
		}
		result.fields[field] = value
	}
	return result, nil
}

func (s *Store) loadAccountGroups(ctx context.Context, accounts map[string]*accountProjection) error {
	query := `SELECT account_id,group_name,group_id,group_rate FROM account_groups`
	arguments := []any{}
	if accountID, ok := soleProjectionAccountID(accounts); ok {
		query += ` WHERE account_id=?`
		arguments = append(arguments, accountID)
	}
	query += ` ORDER BY account_id,group_name`
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var accountID, groupName string
		var groupID, groupRate sql.NullString
		if err := rows.Scan(&accountID, &groupName, &groupID, &groupRate); err != nil {
			return err
		}
		if item := accounts[accountID]; item != nil {
			item.Groups = append(item.Groups, groupName)
			item.groupIDs[groupName] = nullString(groupID)
			item.groupRates[groupName] = nullString(groupRate)
		}
	}
	return rows.Err()
}

func (s *Store) loadAccountDecisions(ctx context.Context, accounts map[string]*accountProjection) (map[string][]decisionProjection, error) {
	query := `SELECT account_id,group_name,routing_state,role,reason,updated_at,payload_json FROM routing_decisions`
	args := []any{}
	clauses := []string{}
	var epoch string
	err := s.db.QueryRowContext(ctx, `SELECT updated_at FROM app_state WHERE key='routing-decision-epoch'`).Scan(&epoch)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		clauses = append(clauses, `updated_at>=?`)
		args = append(args, epoch)
	}
	if accountID, ok := soleProjectionAccountID(accounts); ok {
		clauses = append(clauses, `account_id=?`)
		args = append(args, accountID)
	}
	if len(clauses) > 0 {
		query += ` WHERE ` + strings.Join(clauses, ` AND `)
	}
	query += ` ORDER BY updated_at DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string][]decisionProjection)
	for rows.Next() {
		var accountID, groupName, payloadRaw string
		var routingState, role, reason, updatedAt sql.NullString
		if err := rows.Scan(&accountID, &groupName, &routingState, &role, &reason, &updatedAt, &payloadRaw); err != nil {
			return nil, err
		}
		account := accounts[accountID]
		if account == nil {
			continue
		}
		state := ""
		if routingState.Valid && routingState.String != "" {
			state = routingState.String
		} else if role.Valid {
			state = role.String
		}
		var weight *float64
		if payload, decodeErr := decodeObject(payloadRaw); decodeErr == nil {
			weight = finiteFloat(payload["weight"])
		}
		result[accountID] = append(result[accountID], decisionProjection{
			state: state, reason: nullString(reason), updatedAt: nullString(updatedAt), weight: weight,
		})
	}
	return result, rows.Err()
}

func (s *Store) loadAccountEvaluations(ctx context.Context, accounts map[string]*accountProjection) (map[string][]evaluationProjection, error) {
	query := `SELECT account_id,group_name,health_score,short_score,long_score,
		sample_count,ttfb_p50_ms,ttfb_p95_ms,latest_event
		FROM account_health_evaluations`
	arguments := []any{}
	if accountID, ok := soleProjectionAccountID(accounts); ok {
		query += ` WHERE account_id=?`
		arguments = append(arguments, accountID)
	}
	query += ` ORDER BY evaluated_at DESC`
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string][]evaluationProjection)
	for rows.Next() {
		var accountID, groupName string
		var healthScore, shortScore, longScore, p50, p95 sql.NullFloat64
		var sampleCount int64
		var latestEvent sql.NullString
		if err := rows.Scan(&accountID, &groupName, &healthScore, &shortScore, &longScore, &sampleCount, &p50, &p95, &latestEvent); err != nil {
			return nil, err
		}
		account := accounts[accountID]
		if account == nil {
			continue
		}
		if latest := nullString(latestEvent); latest != nil {
			for _, membership := range account.Groups {
				account.latestEvents[membership] = *latest
			}
		}
		result[accountID] = append(result[accountID], evaluationProjection{
			healthScore: nullFiniteFloat(healthScore), shortScore: nullFiniteFloat(shortScore),
			longScore: nullFiniteFloat(longScore), sampleCount: sampleCount,
			p50: nullFiniteFloat(p50), p95: nullFiniteFloat(p95),
		})
	}
	return result, rows.Err()
}

func (s *Store) loadRecentEvidence(ctx context.Context, accounts map[string]*accountProjection) error {
	clauses := []string{`LOWER(REPLACE(source,'_','-'))<>'account-state'`}
	arguments := []any{}
	if accountID, ok := soleProjectionAccountID(accounts); ok {
		clauses = append(clauses, `account_id=?`)
		arguments = append(arguments, accountID)
	}
	selections, err := s.selectHealthSampleWindow(ctx, clauses, arguments, 10, false, true)
	if err != nil {
		return err
	}
	samples, err := s.selectedHealthSamples(ctx, selections)
	if err != nil {
		return err
	}
	for _, selection := range selections {
		sample, present := samples[selection.id]
		if !present {
			continue
		}
		if account := accounts[sample.accountID]; account != nil {
			payload, decodeErr := decodeObject(sample.payloadJSON)
			if decodeErr != nil {
				payload = map[string]any{}
			}
			payloadRaw := sql.NullString{String: sample.payloadJSON, Valid: true}
			latency := recentFirstTokenLatency(sample.source, sample.latencyP50, sample.latencyP95, payloadRaw)
			duration := recentRequestDuration(sample.source, payload)
			classificationLatency := nullString(sample.latencyP95)
			if classificationLatency == nil {
				classificationLatency = nullString(sample.latencyP50)
			}
			account.RecentResults = append(account.RecentResults, AccountRecentResult{
				Result: nullString(sample.result), ObservedAt: nullString(sample.observedAt), LatencyMS: latency, DurationMS: duration,
				FailureReason: nullString(sample.failureReason), Source: sample.source,
				ClassificationLatency: classificationLatency, ClassificationPayload: payload,
			})
		}
	}
	return nil
}

func recentRequestDuration(source string, payload map[string]any) *float64 {
	normalizedSource := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(source)), "_", "-")
	if normalizedSource != "traffic" && normalizedSource != "ops" {
		return nil
	}
	duration := finiteFloat(payload["duration_ms"])
	if duration == nil || *duration < 0 {
		return nil
	}
	return duration
}

func recentFirstTokenLatency(source string, p50Raw, p95Raw, payloadRaw sql.NullString) *float64 {
	normalizedSource := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(source)), "_", "-")
	trustedProbe := normalizedSource == "active-probe" || normalizedSource == "probe"
	payload, err := decodeObject(payloadRaw.String)
	if normalizedSource == "traffic" || normalizedSource == "ops" {
		if firstToken := finiteFloat(payload["first_token_ms"]); firstToken != nil && *firstToken >= 0 {
			return firstToken
		}
	}
	metric, _ := payload["latency_metric"].(string)
	metric = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(metric)), "-", "_")
	latencySource, _ := payload["latency_source"].(string)
	if trustedProbe && (metric == "total_duration" || metric == "request_duration" ||
		strings.EqualFold(strings.TrimSpace(latencySource), "account_test.complete_response")) {
		return nil
	}
	if !trustedProbe && (err != nil || (metric != "first_token" && metric != "ttfb")) {
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(latencySource), "operations.duration_ms") {
		return nil
	}
	latency := finiteFloatFromNullString(p95Raw)
	if latency == nil {
		latency = finiteFloatFromNullString(p50Raw)
	}
	return latency
}

func (s *Store) loadApplyErrors(ctx context.Context, accountID string) (map[string]struct {
	message string
	at      *string
}, error) {
	query := `SELECT a.id,latest.error,latest.created_at
		FROM accounts a JOIN operation_audit latest ON latest.source_id=(
			SELECT recent.source_id FROM operation_audit recent INDEXED BY ix_operation_audit_apply_error_recent
			WHERE recent.operation_type IN ('routing.writeback','cleanup.delete') AND recent.object_id=a.id
			AND (recent.state='failed' OR recent.readback_confirmed=1)
			ORDER BY recent.created_at DESC,
			CASE WHEN recent.source_id < 0 THEN 0 ELSE 1 END,
			CASE WHEN recent.source_id < 0 THEN recent.source_id END ASC,
			CASE WHEN recent.source_id >= 0 THEN recent.source_id END DESC LIMIT 1
		) WHERE latest.state='failed'`
	arguments := []any{}
	if accountID != "" {
		query += ` AND a.id=?`
		arguments = append(arguments, accountID)
	}
	query += ` ORDER BY a.id`
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]struct {
		message string
		at      *string
	})
	for rows.Next() {
		var objectID, message, createdAt sql.NullString
		if err := rows.Scan(&objectID, &message, &createdAt); err != nil {
			return nil, err
		}
		if !objectID.Valid || strings.TrimSpace(objectID.String) == "" {
			continue
		}
		accountID := strings.TrimSpace(objectID.String)
		if _, exists := result[accountID]; exists {
			continue
		}
		text := "自动执行失败"
		if message.Valid && message.String != "" {
			text = message.String
		}
		result[accountID] = struct {
			message string
			at      *string
		}{text, nullString(createdAt)}
	}
	return result, rows.Err()
}

func soleProjectionAccountID(accounts map[string]*accountProjection) (string, bool) {
	if len(accounts) != 1 {
		return "", false
	}
	for accountID := range accounts {
		return accountID, true
	}
	return "", false
}

func (s *Store) monitorPolicy(ctx context.Context, mode string) (map[string]struct{}, float64, error) {
	if mode != runtimepolicy.Monitoring {
		return map[string]struct{}{}, 75, nil
	}
	control, err := s.readPolicyDocument(ctx, s.db, "control-plane")
	if err != nil {
		return nil, 0, err
	}
	excluded := map[string]struct{}{}
	threshold := 75.0
	if scope, ok := control["scope"].(map[string]any); ok {
		if values, ok := scope["excluded_account_ids"].([]any); ok {
			for _, value := range values {
				if normalized := strings.TrimSpace(fmt.Sprint(value)); normalized != "" {
					excluded[normalized] = struct{}{}
				}
			}
		}
	}
	if degrade, ok := control["degrade"].(map[string]any); ok {
		if value := finiteFloat(degrade["score_threshold"]); value != nil && *value != 0 {
			threshold = *value
		}
	}
	return excluded, threshold, nil
}

func applyAccountCalculations(
	item *accountProjection,
	decisions []decisionProjection,
	evaluations []evaluationProjection,
	applyError struct {
		message string
		at      *string
	},
	applyView routingApplyView,
) {
	var selected *decisionProjection
	selectedState := ""
	for index := range decisions {
		state := NormalizeAccountState(decisions[index].state)
		if selected == nil || accountStatePriority(state) > accountStatePriority(selectedState) {
			selected = &decisions[index]
			selectedState = state
		}
	}
	if selected != nil {
		item.DecisionState = stringPointer(selectedState)
		item.DecisionReason = selected.reason
		item.DesiredHealth = stringPointer(selectedState)
	}
	currentHealth := AccountStateUnknown
	if item.RoutingState != nil {
		currentHealth = NormalizeAccountState(*item.RoutingState)
	}
	if currentHealth == AccountStateUnknown && item.HealthStatus != nil {
		currentHealth = NormalizeAccountState(*item.HealthStatus)
	}
	if metadataState := accountMetadataState(item.metadataRaw); metadataState == AccountStateDisabled {
		currentHealth = metadataState
	}
	effectiveHealth := currentHealth
	if item.Paused != nil && *item.Paused {
		effectiveHealth = "paused"
	} else if currentHealth == "disabled" {
		effectiveHealth = "disabled"
	} else if selectedState == "excluded" {
		effectiveHealth = "excluded"
	}
	item.Health = effectiveHealth
	statePending := selectedState != "" && selectedState != effectiveHealth && applyView.fields["schedulable"] &&
		effectiveHealth != "paused" && effectiveHealth != "disabled" && effectiveHealth != "excluded"
	item.ApplyPending = selectedState != "" && selectedState != "excluded" &&
		(statePending || routingTargetMismatch(item, applyView.fields))
	if item.ApplyPending {
		if applyView.automatic {
			item.ApplyError = stringPointer("尚未应用到 Sub2API")
		} else {
			item.ApplyError = stringPointer("当前运行模式只保存调度目标，不会自动执行")
		}
		if applyError.message != "" && (selected == nil || selected.updatedAt == nil || applyError.at == nil || *applyError.at >= *selected.updatedAt) {
			item.ApplyError = stringPointer(applyError.message)
		}
	}
	scored := make([]evaluationProjection, 0, len(evaluations))
	for _, evaluation := range evaluations {
		if evaluation.sampleCount > 0 {
			scored = append(scored, evaluation)
		}
	}
	item.HealthScore = averageEvaluation(scored, func(value evaluationProjection) *float64 { return value.healthScore })
	item.ShortScore = averageEvaluation(scored, func(value evaluationProjection) *float64 { return value.shortScore })
	item.LongScore = averageEvaluation(scored, func(value evaluationProjection) *float64 { return value.longScore })
	item.TTFBP50MS = averageEvaluation(scored, func(value evaluationProjection) *float64 { return value.p50 })
	item.TTFBP95MS = averageEvaluation(scored, func(value evaluationProjection) *float64 { return value.p95 })
	weights := make([]float64, 0, len(decisions))
	for _, decision := range decisions {
		if decision.weight != nil {
			weights = append(weights, *decision.weight)
		}
	}
	item.Weight = roundedAverage(weights)
	for _, evaluation := range scored {
		if evaluation.sampleCount > item.SampleCount {
			item.SampleCount = evaluation.sampleCount
		}
	}
}

func accountMetadataState(raw string) string {
	metadata, err := decodeObject(raw)
	if err != nil {
		return AccountStateUnknown
	}
	status, _ := metadata["status"].(string)
	return NormalizeAccountState(status)
}

func accountMetadataText(raw string, keys ...string) *string {
	metadata, err := decodeObject(raw)
	if err != nil {
		return nil
	}
	for _, key := range keys {
		value, ok := metadata[key].(string)
		value = strings.TrimSpace(value)
		if ok && value != "" {
			return stringPointer(value)
		}
	}
	return nil
}

func routingTargetMismatch(item *accountProjection, fields map[string]bool) bool {
	if fields["schedulable"] && item.TargetSchedulable != nil && !boolPointersEqual(item.Schedulable, item.TargetSchedulable) {
		return true
	}
	if fields["priority"] && item.TargetPriority != nil && !intPointersEqual(item.Priority, item.TargetPriority) {
		return true
	}
	if fields["load_factor"] && item.TargetLoadFactor != nil && !decimalPointersEqual(item.LoadFactor, item.TargetLoadFactor) {
		return true
	}
	return fields["concurrency"] && item.TargetConcurrency != nil && !intPointersEqual(item.Concurrency, item.TargetConcurrency)
}

func intPointersEqual(left, right *int64) bool {
	return left != nil && right != nil && *left == *right
}

func decimalPointersEqual(left, right *string) bool {
	if left == nil || right == nil {
		return false
	}
	leftValue, leftOK := new(big.Rat).SetString(strings.TrimSpace(*left))
	rightValue, rightOK := new(big.Rat).SetString(strings.TrimSpace(*right))
	return leftOK && rightOK && leftValue.Cmp(rightValue) == 0
}

func applyMonitoringHealth(item *accountProjection, excluded map[string]struct{}, degradeThreshold float64) {
	current := AccountStateUnknown
	if item.RoutingState != nil {
		current = NormalizeAccountState(*item.RoutingState)
	}
	if current == AccountStateUnknown && item.HealthStatus != nil {
		current = NormalizeAccountState(*item.HealthStatus)
	}
	reason := (*string)(nil)
	if item.Paused != nil && *item.Paused {
		current = "paused"
		reason = item.PausedReason
	} else if _, found := excluded[item.ID]; found {
		current = "excluded"
		reason = stringPointer("账号被排除")
	} else if current == "disabled" {
		reason = stringPointer("账号已停用")
	} else if current == "fused" || current == "cost_blocked" || current == "survivor" {
		reason = item.DecisionReason
	} else if item.SampleCount == 0 || item.HealthScore == nil {
		current = "unknown"
		reason = stringPointer("最近评估窗口没有有效样本")
	} else if *item.HealthScore < degradeThreshold {
		current = "degraded"
		reason = stringPointer("健康分低于降级线 " + strconv.FormatFloat(degradeThreshold, 'g', -1, 64))
	} else {
		current = "healthy"
	}
	item.Health = current
	item.DesiredHealth = nil
	item.DecisionState = stringPointer(current)
	item.DecisionReason = reason
	item.ApplyPending = false
	item.ApplyError = nil
}

func averageEvaluation(values []evaluationProjection, field func(evaluationProjection) *float64) *float64 {
	numbers := make([]float64, 0, len(values))
	for _, value := range values {
		if number := field(value); number != nil {
			numbers = append(numbers, *number)
		}
	}
	return roundedAverage(numbers)
}

func roundedAverage(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	total := 0.0
	for _, value := range values {
		total += value
	}
	result := math.Round(total/float64(len(values))*10000) / 10000
	return &result
}

func decodeObject(raw string) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil || value == nil {
		return nil, errors.New("JSON 对象无效")
	}
	return value, nil
}

func finiteFloat(value any) *float64 {
	if value == nil {
		return nil
	}
	var parsed float64
	var err error
	switch item := value.(type) {
	case bool:
		return nil
	case float64:
		parsed = item
	case json.Number:
		parsed, err = item.Float64()
	case string:
		parsed, err = strconv.ParseFloat(strings.TrimSpace(item), 64)
	default:
		parsed, err = strconv.ParseFloat(fmt.Sprint(item), 64)
	}
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return nil
	}
	return &parsed
}

func finiteFloatFromNullString(value sql.NullString) *float64 {
	if !value.Valid {
		return nil
	}
	return finiteFloat(value.String)
}

func nullFiniteFloat(value sql.NullFloat64) *float64 {
	if !value.Valid || math.IsNaN(value.Float64) || math.IsInf(value.Float64, 0) {
		return nil
	}
	result := value.Float64
	return &result
}

func strictBool(value sql.NullInt64) *bool {
	if !value.Valid || (value.Int64 != 0 && value.Int64 != 1) {
		return nil
	}
	result := value.Int64 == 1
	return &result
}

func nullString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func nullInt(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func stringPointer(value string) *string {
	result := value
	return &result
}

func positiveNumericID(value string) bool {
	number, err := strconv.ParseUint(value, 10, 64)
	return err == nil && number > 0
}

func canonicalHost(value string) string {
	normalized := strings.TrimSpace(strings.TrimRight(value, "/"))
	if separator := strings.Index(normalized, "://"); separator >= 0 {
		normalized = normalized[separator+3:]
		if slash := strings.IndexByte(normalized, '/'); slash >= 0 {
			normalized = normalized[:slash]
		}
	}
	return strings.ToLower(strings.TrimRight(normalized, "/"))
}

func containsString(values []string, expected string) bool {
	index := sort.SearchStrings(values, expected)
	return index < len(values) && values[index] == expected
}

func boolPointersEqual(left, right *bool) bool {
	return left != nil && right != nil && *left == *right
}
