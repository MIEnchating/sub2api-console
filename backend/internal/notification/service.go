package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/redact"
)

const (
	tokenEndpoint            = "https://bots.qq.com/app/getAppAccessToken"
	messageLimit             = 4000
	batchLimit               = 3900
	maximumQQBotResponseSize = 1 << 20
)

type Repository interface {
	PrepareAlertDelivery(context.Context, string, bool) (business.AlertDeliveryPlan, error)
	BeginAlertDelivery(context.Context, string, []business.AlertIncident) error
	FinalizeAlertDelivery(context.Context, string, []business.AlertDeliveryOutcome) error
	RecordRuntimeEvent(context.Context, string, string, string, map[string]any) (int64, error)
}

type SettingsStore interface {
	NotificationSettings(context.Context) (configstore.NotificationSettings, error)
}

type BatchSender interface {
	Send(context.Context, configstore.NotificationSettings, []string) []SendOutcome
}

type SendOutcome struct {
	Success       bool
	CommitUnknown bool
	Detail        string
	MessageID     *string
}

type Service struct {
	repository     Repository
	settings       SettingsStore
	sender         BatchSender
	deliveryMu     sync.Mutex
	stateMu        sync.RWMutex
	consumerActive bool
}

type QueueStatus struct {
	ConsumerActive bool `json:"consumer_active"`
}

type TestResult struct {
	Sent           bool    `json:"sent"`
	Simulated      bool    `json:"simulated"`
	Detail         string  `json:"detail"`
	MessageID      *string `json:"message_id"`
	RuntimeEventID int64   `json:"runtime_event_id"`
	Persisted      bool    `json:"persisted"`
}

func New(repository Repository, settings SettingsStore, sender BatchSender) *Service {
	return &Service{repository: repository, settings: settings, sender: sender}
}

func (s *Service) Status() QueueStatus {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return QueueStatus{ConsumerActive: s.consumerActive}
}

func (s *Service) Test(ctx context.Context, message string, dryRun bool) (TestResult, error) {
	message = strings.TrimSpace(message)
	if message == "" || utf8.RuneCountInString(message) > messageLimit {
		return TestResult{}, errors.New("通知测试消息长度必须在 1 到 4000 之间")
	}
	settings, err := s.settings.NotificationSettings(ctx)
	if err != nil {
		return TestResult{}, err
	}
	if !dryRun && (settings.AppID == "" || settings.ClientSecret == "" || settings.HomeChannel == "") {
		return TestResult{}, errors.New("通知渠道尚未配置")
	}
	outcome := SendOutcome{Success: true, Detail: "模拟发送成功"}
	if !dryRun {
		outcomes := s.sender.Send(ctx, settings, []string{message})
		if len(outcomes) != 1 {
			outcome = SendOutcome{Detail: "QQBot 批次响应数量不一致"}
		} else {
			outcome = outcomes[0]
		}
	}
	detail := strings.TrimSpace(outcome.Detail)
	if detail == "" {
		if outcome.Success {
			detail = "发送成功"
		} else {
			detail = "发送失败"
		}
	}
	status, summary := "failed", "通知测试失败："+detail
	if outcome.Success {
		status, summary = "succeeded", "通知测试成功："+detail
	}
	eventID, err := s.repository.RecordRuntimeEvent(ctx, "notifications.test", status, summary, map[string]any{
		"sent": outcome.Success, "simulated": dryRun, "message_id": outcome.MessageID, "detail": detail,
	})
	if err != nil {
		return TestResult{}, err
	}
	return TestResult{
		Sent: outcome.Success, Simulated: dryRun, Detail: detail, MessageID: outcome.MessageID,
		RuntimeEventID: eventID, Persisted: true,
	}, nil
}

func (s *Service) Deliver(ctx context.Context, dryRun bool) (business.AlertDeliveryResult, error) {
	s.deliveryMu.Lock()
	s.setConsumerActive(true)
	defer func() {
		s.setConsumerActive(false)
		s.deliveryMu.Unlock()
	}()
	settings, err := s.settings.NotificationSettings(ctx)
	if err != nil {
		return business.AlertDeliveryResult{}, err
	}
	privateConfigured := settings.AppID != "" && settings.ClientSecret != "" && settings.HomeChannel != ""
	channelKey := business.NotificationChannelKey("qqbot", settings.HomeChannel)
	plan, err := s.repository.PrepareAlertDelivery(ctx, channelKey, privateConfigured)
	if err != nil {
		return business.AlertDeliveryResult{}, err
	}
	result := business.AlertDeliveryResult{
		Skipped: plan.Skipped, Suppressed: plan.Suppressed, Configured: plan.Configured,
		Disabled: plan.Disabled, DryRun: dryRun, MessageIDs: []string{},
	}
	if len(plan.Pending) == 0 {
		return result, nil
	}
	batches := NotificationBatches(plan.Pending, plan.MergeThreshold)
	if err := s.repository.BeginAlertDelivery(ctx, channelKey, plan.Pending); err != nil {
		return business.AlertDeliveryResult{}, err
	}
	messages := make([]string, len(batches))
	for index := range batches {
		messages[index] = batches[index].Message
	}
	batchOutcomes := make([]SendOutcome, len(batches))
	if dryRun {
		for index := range batchOutcomes {
			batchOutcomes[index] = SendOutcome{Success: true, Detail: "模拟发送成功"}
		}
	} else {
		batchOutcomes = s.sender.Send(ctx, settings, messages)
	}
	if len(batchOutcomes) != len(batches) {
		batchOutcomes = make([]SendOutcome, len(batches))
		for index := range batchOutcomes {
			batchOutcomes[index] = SendOutcome{CommitUnknown: true, Detail: "QQBot 批次响应数量不一致，发送结果待人工确认"}
		}
	}
	outcomes := make([]business.AlertDeliveryOutcome, 0, len(plan.Pending))
	for index, batch := range batches {
		outcome := batchOutcomes[index]
		if outcome.MessageID != nil {
			result.MessageIDs = append(result.MessageIDs, *outcome.MessageID)
		}
		for _, incident := range batch.Incidents {
			outcomes = append(outcomes, business.AlertDeliveryOutcome{
				IncidentKey: incident.IncidentKey, Success: outcome.Success,
				CommitUnknown: outcome.CommitUnknown, Detail: outcome.Detail, MessageID: outcome.MessageID,
			})
			if outcome.Success {
				result.Sent++
			} else if outcome.CommitUnknown {
				result.Uncertain++
			} else {
				result.Failed++
			}
		}
	}
	result.Attempted = result.Sent + result.Failed + result.Uncertain
	result.Batches = len(batches)
	if err := s.repository.FinalizeAlertDelivery(ctx, channelKey, outcomes); err != nil {
		return business.AlertDeliveryResult{}, err
	}
	return result, nil
}

func (s *Service) setConsumerActive(active bool) {
	s.stateMu.Lock()
	s.consumerActive = active
	s.stateMu.Unlock()
}

type NotificationBatch struct {
	Incidents []business.AlertIncident
	Message   string
}

func NotificationBatches(incidents []business.AlertIncident, mergeThreshold int) []NotificationBatch {
	if mergeThreshold < 2 {
		mergeThreshold = 10
	}
	groups := notificationGroups(incidents)
	if len(incidents) < mergeThreshold {
		degraded := make([]business.AlertIncident, 0)
		for _, group := range groups {
			if len(group.incidents) == 1 && group.incidents[0].EventType == "account.routing_degraded" {
				degraded = append(degraded, group.incidents[0])
			}
		}
		result := make([]NotificationBatch, 0, len(groups))
		degradedAdded := false
		for _, group := range groups {
			if len(degraded) >= 2 && len(group.incidents) == 1 && group.incidents[0].EventType == "account.routing_degraded" {
				if !degradedAdded {
					result = append(result, NotificationBatch{Incidents: degraded, Message: BatchMessage(degraded)})
					degradedAdded = true
				}
				continue
			}
			result = append(result, NotificationBatch{Incidents: group.incidents, Message: BatchMessage(group.incidents)})
		}
		return result
	}
	result := make([]NotificationBatch, 0)
	current := make([]business.AlertIncident, 0)
	for _, group := range groups {
		candidate := append(append([]business.AlertIncident{}, current...), group.incidents...)
		message := BatchMessage(candidate)
		if len(current) > 0 && utf8.RuneCountInString(message) >= batchLimit {
			result = append(result, NotificationBatch{Incidents: current, Message: BatchMessage(current)})
			current = append([]business.AlertIncident{}, group.incidents...)
		} else {
			current = candidate
		}
	}
	if len(current) > 0 {
		result = append(result, NotificationBatch{Incidents: current, Message: BatchMessage(current)})
	}
	return result
}

type notificationGroup struct {
	incidents []business.AlertIncident
	parent    *business.AlertIncident
}

func notificationGroups(incidents []business.AlertIncident) []notificationGroup {
	parentByRelation := map[string]int{}
	for index, incident := range incidents {
		childTypes := relatedRoutingChildTypes(incident.EventType)
		if len(childTypes) == 0 || strings.TrimSpace(incident.ObjectID) == "" {
			continue
		}
		for _, childType := range childTypes {
			parentByRelation[routingRelationKey(incident.Status, incident.ObjectID, childType)] = index
		}
	}
	membersByParent := map[int][]int{}
	for index, incident := range incidents {
		groupName := routingIncidentGroup(incident)
		if groupName == "" {
			continue
		}
		parentIndex, found := parentByRelation[routingRelationKey(incident.Status, groupName, incident.EventType)]
		if !found {
			continue
		}
		membersByParent[parentIndex] = append(membersByParent[parentIndex], index)
	}

	consumed := make(map[int]struct{})
	groupByRoot := map[int]notificationGroup{}
	for parentIndex, childIndexes := range membersByParent {
		if len(childIndexes) == 0 {
			continue
		}
		indexes := append([]int{parentIndex}, childIndexes...)
		sort.Ints(indexes)
		members := make([]business.AlertIncident, 0, len(indexes))
		for _, index := range indexes {
			consumed[index] = struct{}{}
			members = append(members, incidents[index])
		}
		parent := incidents[parentIndex]
		groupByRoot[indexes[0]] = notificationGroup{incidents: members, parent: &parent}
	}

	result := make([]notificationGroup, 0, len(incidents))
	for index, incident := range incidents {
		if group, found := groupByRoot[index]; found {
			result = append(result, group)
			continue
		}
		if _, found := consumed[index]; found {
			continue
		}
		result = append(result, notificationGroup{incidents: []business.AlertIncident{incident}})
	}
	return result
}

func relatedRoutingChildTypes(eventType string) []string {
	switch eventType {
	case "group.routing_survivor":
		return []string{"account.routing_survivor"}
	case "group.routing_unavailable":
		return []string{"account.binding_invalid", "account.routing_breaker"}
	default:
		return nil
	}
}

func routingRelationKey(status, groupName, childType string) string {
	return status + "\x00" + groupName + "\x00" + childType
}

func routingIncidentGroup(incident business.AlertIncident) string {
	if incident.ObjectKind != "account" {
		return ""
	}
	prefix := accountGroupIncidentPrefix(incident.EventType, incident.ObjectID)
	if prefix == "" || !strings.HasPrefix(incident.IncidentKey, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(incident.IncidentKey, prefix))
}

var eventLabels = map[string]string{
	"upstream.configuration":       "上游配置有问题",
	"upstream.auth":                "上游鉴权失败",
	"upstream.rate_sync":           "上游倍率同步失败",
	"upstream.balance":             "上游余额不足",
	"account.multiplier_increased": "账号倍率上涨",
	"account.multiplier_decreased": "账号倍率下降",
	"account.probe":                "账号连接探测失败",
	"account.routing_breaker":      "账号已停止调度",
	"account.binding_invalid":      "账号绑定已失效",
	"account.routing_degraded":     "账号已降级调度",
	"account.routing_survivor":     "账号被临时保留",
	"group.routing_unavailable":    "分组没有可用账号",
	"group.routing_survivor":       "分组只剩保底账号",
	"routing.apply_failure":        "自动处理失败",
}

var causeLabels = map[string]string{
	"CONFIG":                              "上游配置有问题",
	"CONFIG_METADATA_INVALID":             "上游返回信息无法识别",
	"CONFIG_AUTH_STATUS_MISSING":          "上游没有返回鉴权状态",
	"CONFIG_AUTH_STATUS_UNKNOWN":          "上游鉴权状态无法识别",
	"CONFIG_BALANCE_CLOSED_INVALID":       "上游余额关闭状态无效",
	"CONFIG_BALANCE_INVALID":              "上游余额不是有效数字",
	"AUTH":                                "上游鉴权失败",
	"RATE_SYNC":                           "倍率同步失败",
	"BALANCE_HARD_CLOSED":                 "上游因余额不足停止服务",
	"PROBE":                               "连续连接探测失败",
	"ROUTING_BREAKER":                     "连续失败，已停止调度",
	"BINDING_INVALID":                     "绑定的上游或分组已不存在",
	"ROUTING_DEGRADED":                    "调度规则判定账号需要降级",
	"ROUTING_DEGRADED_HEALTH_SCORE":       "健康分低于降级标准",
	"ROUTING_DEGRADED_GATEWAY_ERROR_RATE": "网关错误率过高，达到降级标准",
	"ROUTING_DEGRADED_LATENCY":            "响应太慢，超过降级标准",
	"ROUTING_DEGRADED_OTHER":              "其他调度规则触发降级",
	"ROUTING_SURVIVOR":                    "为保证分组可用，暂时保留账号",
	"GROUP_UNAVAILABLE":                   "分组中没有可调度账号",
	"GROUP_SURVIVOR_ONLY":                 "分组中只剩保底账号",
	"APPLY_FAILED":                        "自动处理未成功，请查看执行记录",
	"TEST":                                "测试通知",
}

var objectLabels = map[string]string{"host": "上游", "account": "账号", "group": "分组"}

func BatchMessage(incidents []business.AlertIncident) string {
	groups := notificationGroups(incidents)
	degradedDigests := routingDegradedDigests(groups)
	statuses := map[string]struct{}{}
	for _, incident := range incidents {
		statuses[incident.Status] = struct{}{}
	}
	title := "告警与恢复汇总"
	if len(groups) == 1 {
		title = "状态通知"
		if incidents[0].Status == "recovered" {
			title = "恢复通知"
		} else if incidents[0].Status == "firing" {
			title = "告警通知"
		}
	}
	if len(statuses) == 1 {
		if _, found := statuses["recovered"]; found && len(groups) > 1 {
			title = "恢复汇总"
		} else if _, found := statuses["firing"]; found && len(groups) > 1 {
			title = "告警汇总"
		}
	}
	if len(groups) == 1 && groups[0].parent != nil {
		return relatedRoutingMessage(title, groups[0])
	}
	if len(groups) == 1 {
		fields := notificationIncidentFields(incidents[0])
		lines := []string{
			fmt.Sprintf("## Sub2API · %s", title),
			"",
			"| 项目 | 内容 |",
			"| --- | --- |",
			fmt.Sprintf("| 告警类型 | %s |", markdownTableValue(fields.event)),
			fmt.Sprintf("| 告警对象 | %s |", markdownTableValue(fields.object)),
			fmt.Sprintf("| 原因 | %s |", markdownTableValue(fields.cause)),
			fmt.Sprintf("| 状态 | %s |", fields.status),
			fmt.Sprintf("| 时间（北京时间） | %s |", markdownTableValue(fields.observedAt)),
		}
		return truncateRunes(strings.Join(lines, "\n"), messageLimit)
	}
	displayCount := len(groups)
	for _, digest := range degradedDigests {
		displayCount -= len(digest) - 1
	}
	lines := []string{
		fmt.Sprintf("## Sub2API · %s（%d项）", title, displayCount),
		"",
		"| 类型 | 对象 | 原因 | 状态 | 时间（北京时间） |",
		"| --- | --- | --- | --- | --- |",
	}
	renderedDigests := map[string]struct{}{}
	for _, group := range groups {
		if len(group.incidents) == 1 && group.incidents[0].EventType == "account.routing_degraded" {
			status := group.incidents[0].Status
			if digest := degradedDigests[status]; len(digest) > 0 {
				if _, rendered := renderedDigests[status]; !rendered {
					lines = append(lines, routingDegradedDigestRow(digest))
					renderedDigests[status] = struct{}{}
				}
				continue
			}
		}
		if group.parent != nil {
			lines = append(lines, relatedRoutingTableRow(group))
			continue
		}
		lines = append(lines, incidentTableRow(group.incidents[0]))
	}
	return truncateRunes(strings.Join(lines, "\n"), messageLimit)
}

func routingDegradedDigests(groups []notificationGroup) map[string][]business.AlertIncident {
	result := map[string][]business.AlertIncident{}
	for _, group := range groups {
		if len(group.incidents) != 1 || group.incidents[0].EventType != "account.routing_degraded" {
			continue
		}
		incident := group.incidents[0]
		result[incident.Status] = append(result[incident.Status], incident)
	}
	for status, incidents := range result {
		if len(incidents) < 2 {
			delete(result, status)
		}
	}
	return result
}

func routingDegradedDigestRow(incidents []business.AlertIncident) string {
	groups := map[string]struct{}{}
	latest := incidents[0]
	for _, incident := range incidents {
		group := routingIncidentGroup(incident)
		if group != "" {
			groups[group] = struct{}{}
		}
		if incident.LastSeenAt > latest.LastSeenAt {
			latest = incident
		}
	}
	groupNames := make([]string, 0, len(groups))
	for group := range groups {
		groupNames = append(groupNames, group)
	}
	sort.Strings(groupNames)
	groupSummary := strings.Join(groupNames, "、")
	if len(groupNames) > 8 {
		groupSummary = strings.Join(groupNames[:8], "、") + fmt.Sprintf(" 等 %d 个分组", len(groupNames))
	}
	if groupSummary == "" {
		groupSummary = "分组未记录"
	}
	status := "告警中"
	if latest.Status == "recovered" {
		status = "已恢复"
	}
	object := fmt.Sprintf("%d 个账号 · %d 个分组", len(incidents), len(groups))
	cause := routingDegradedReasonSummary(incidents) + "；分组：" + groupSummary + "。账号明细请在账号管理中查看"
	return fmt.Sprintf("| %s | %s | %s | %s | %s |",
		markdownTableValue(eventLabels["account.routing_degraded"]), markdownTableValue(object),
		markdownTableValue(cause), status, markdownTableValue(notificationTime(latest.LastSeenAt)),
	)
}

func routingDegradedReasonSummary(incidents []business.AlertIncident) string {
	type reasonCount struct {
		reason string
		count  int
	}
	counts := map[string]int{}
	for _, incident := range incidents {
		reason := strings.TrimSpace(relatedAccountCause(incident))
		if reason == "" {
			reason = "未分类原因"
		}
		counts[reason]++
	}
	reasons := make([]reasonCount, 0, len(counts))
	for reason, count := range counts {
		reasons = append(reasons, reasonCount{reason: reason, count: count})
	}
	sort.Slice(reasons, func(i, j int) bool {
		if reasons[i].count != reasons[j].count {
			return reasons[i].count > reasons[j].count
		}
		return reasons[i].reason < reasons[j].reason
	})
	parts := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		parts = append(parts, fmt.Sprintf("%s（%d 个账号）", reason.reason, reason.count))
	}
	return strings.Join(parts, "；")
}

func relatedRoutingMessage(title string, group notificationGroup) string {
	fields := notificationIncidentFields(*group.parent)
	lines := []string{
		fmt.Sprintf("## Sub2API · %s", title),
		"",
		"| 项目 | 内容 |",
		"| --- | --- |",
		fmt.Sprintf("| 告警类型 | %s |", markdownTableValue(fields.event)),
		fmt.Sprintf("| 告警对象 | %s |", markdownTableValue(fields.object)),
		fmt.Sprintf("| 关联账号 | %s |", markdownTableValue(relatedAccountDetails(group))),
		fmt.Sprintf("| 原因 | %s |", markdownTableValue(fields.cause)),
		fmt.Sprintf("| 状态 | %s |", fields.status),
		fmt.Sprintf("| 时间（北京时间） | %s |", markdownTableValue(fields.observedAt)),
	}
	return truncateRunes(strings.Join(lines, "\n"), messageLimit)
}

func relatedRoutingTableRow(group notificationGroup) string {
	fields := notificationIncidentFields(*group.parent)
	object := fields.object + " · 关联账号：" + relatedAccountSummary(group)
	cause := fields.cause + "；账号原因：" + relatedAccountReasons(group)
	return fmt.Sprintf("| %s | %s | %s | %s | %s |",
		markdownTableValue(fields.event), markdownTableValue(object), markdownTableValue(cause),
		fields.status, markdownTableValue(fields.observedAt),
	)
}

func relatedAccountDetails(group notificationGroup) string {
	details := make([]string, 0, len(group.incidents)-1)
	for _, incident := range group.incidents {
		if incident.IncidentKey == group.parent.IncidentKey {
			continue
		}
		details = append(details, notificationAccountIdentity(incident)+"："+relatedAccountCause(incident))
	}
	return strings.Join(details, "\n")
}

func relatedAccountSummary(group notificationGroup) string {
	accounts := make([]string, 0, len(group.incidents)-1)
	for _, incident := range group.incidents {
		if incident.IncidentKey != group.parent.IncidentKey {
			accounts = append(accounts, notificationAccountIdentity(incident))
		}
	}
	return strings.Join(accounts, "、")
}

func relatedAccountReasons(group notificationGroup) string {
	reasons := make([]string, 0, len(group.incidents)-1)
	for _, incident := range group.incidents {
		if incident.IncidentKey == group.parent.IncidentKey {
			continue
		}
		reasons = append(reasons, notificationAccountIdentity(incident)+"："+relatedAccountCause(incident))
	}
	return strings.Join(reasons, "；")
}

func notificationAccountIdentity(incident business.AlertIncident) string {
	if incident.ObjectName != nil && strings.TrimSpace(*incident.ObjectName) != "" {
		return strings.TrimSpace(*incident.ObjectName) + "（#" + incident.ObjectID + "）"
	}
	return "账号 #" + incident.ObjectID
}

func relatedAccountCause(incident business.AlertIncident) string {
	if incident.Status == "recovered" {
		return notificationIncidentFields(incident).cause
	}
	if _, reason, found := dynamicAlertCause(incident.CauseCode); found && reason != "" {
		if incident.EventType == "account.routing_survivor" {
			reason = strings.TrimSpace(strings.TrimPrefix(reason, "保底强留："))
		}
		if reason != "" {
			return reason
		}
	}
	return notificationIncidentFields(incident).cause
}

type notificationIncidentDisplay struct {
	event      string
	object     string
	cause      string
	status     string
	observedAt string
}

func notificationIncidentFields(incident business.AlertIncident) notificationIncidentDisplay {
	eventLabel := eventLabels[incident.EventType]
	if eventLabel == "" {
		eventLabel = "运行异常"
	}
	if incident.Status == "recovered" {
		recoveredLabels := map[string]string{
			"upstream.configuration":    "上游配置已恢复",
			"upstream.auth":             "上游鉴权已恢复",
			"upstream.rate_sync":        "上游倍率同步已恢复",
			"upstream.balance":          "上游余额已恢复",
			"account.probe":             "账号连接探测已恢复",
			"account.routing_breaker":   "账号已恢复调度",
			"account.binding_invalid":   "账号绑定已恢复",
			"account.routing_degraded":  "账号已恢复正常调度",
			"account.routing_survivor":  "账号已退出临时保留",
			"group.routing_unavailable": "分组已恢复可用",
			"group.routing_survivor":    "分组已恢复正常",
			"routing.apply_failure":     "自动处理已恢复",
		}
		if recoveredLabel := recoveredLabels[incident.EventType]; recoveredLabel != "" {
			eventLabel = recoveredLabel
		}
	}
	objectLabel := objectLabels[incident.ObjectKind]
	if objectLabel == "" {
		objectLabel = "对象"
	}
	causeLabel := causeLabels[incident.CauseCode]
	if multiplierCause, found := multiplierChangeCause(incident.CauseCode); found {
		causeLabel = multiplierCause
	} else if strings.HasPrefix(incident.CauseCode, "BALANCE:") {
		threshold := strings.TrimSpace(strings.TrimPrefix(incident.CauseCode, "BALANCE:"))
		if incident.Status == "recovered" {
			causeLabel = "余额已恢复至告警阈值以上"
			if threshold != "" {
				causeLabel = "余额已高于告警阈值 " + threshold
			}
		} else {
			causeLabel = "余额不足"
			if threshold != "" {
				causeLabel = "余额达到或低于 " + threshold
			}
		}
	} else if incident.Status == "recovered" {
		recoveredCauses := map[string]string{
			"CONFIG":                              "上游配置已恢复正常",
			"AUTH":                                "上游鉴权已恢复",
			"RATE_SYNC":                           "倍率同步已恢复",
			"BALANCE_HARD_CLOSED":                 "余额不足导致的停服已解除",
			"PROBE":                               "账号连接探测已恢复",
			"ROUTING_BREAKER":                     "账号已恢复调度",
			"BINDING_INVALID":                     "账号绑定已恢复",
			"ROUTING_DEGRADED":                    "账号已恢复正常调度",
			"ROUTING_DEGRADED_HEALTH_SCORE":       "账号已恢复正常调度",
			"ROUTING_DEGRADED_GATEWAY_ERROR_RATE": "账号已恢复正常调度",
			"ROUTING_DEGRADED_LATENCY":            "账号已恢复正常调度",
			"ROUTING_DEGRADED_OTHER":              "账号已恢复正常调度",
			"ROUTING_SURVIVOR":                    "账号已退出临时保留",
			"GROUP_UNAVAILABLE":                   "分组已恢复可用",
			"GROUP_SURVIVOR_ONLY":                 "分组已恢复正常",
			"APPLY_FAILED":                        "自动处理已恢复",
		}
		if recoveredCause := recoveredCauses[incident.CauseCode]; recoveredCause != "" {
			causeLabel = recoveredCause
		} else if strings.HasPrefix(incident.CauseCode, "RATE_SYNC:") {
			causeLabel = recoveredCauses["RATE_SYNC"]
		} else if code, _, found := dynamicAlertCause(incident.CauseCode); found {
			causeLabel = recoveredCauses[code]
			if causeLabel == "" {
				causeLabel = "相关问题已恢复"
			}
		} else if causeLabel == "" {
			causeLabel = "相关问题已恢复"
		}
	} else if strings.HasPrefix(incident.CauseCode, "RATE_SYNC:") {
		reason := strings.TrimSpace(strings.TrimPrefix(incident.CauseCode, "RATE_SYNC:"))
		causeLabel = compactRateSyncReason(reason)
	} else if code, reason, found := dynamicAlertCause(incident.CauseCode); found {
		causeLabel = causeLabels[code]
		if reason != "" {
			causeLabel += "：" + reason
		}
	} else if causeLabel == "" {
		causeLabel = "未分类原因"
	}
	statusLabel := "告警中"
	if incident.Status == "recovered" {
		statusLabel = "已恢复"
	}
	objectValue := objectLabel + "：" + incident.ObjectID
	if incident.ObjectName != nil && strings.TrimSpace(*incident.ObjectName) != "" {
		objectValue = objectLabel + "：" + strings.TrimSpace(*incident.ObjectName) + "（#" + incident.ObjectID + "）"
	}
	if prefix := accountGroupIncidentPrefix(incident.EventType, incident.ObjectID); prefix != "" {
		if group := strings.TrimSpace(strings.TrimPrefix(incident.IncidentKey, prefix)); strings.HasPrefix(incident.IncidentKey, prefix) && group != "" {
			objectValue += " · 分组：" + group
		}
	}
	return notificationIncidentDisplay{
		event: eventLabel, object: objectValue, cause: causeLabel,
		status: statusLabel, observedAt: notificationTime(incident.LastSeenAt),
	}
}

func multiplierChangeCause(causeCode string) (string, bool) {
	for _, change := range []struct {
		prefix, verb string
	}{
		{prefix: "MULTIPLIER_INCREASED:", verb: "上涨"},
		{prefix: "MULTIPLIER_DECREASED:", verb: "下降"},
	} {
		if !strings.HasPrefix(causeCode, change.prefix) {
			continue
		}
		values := strings.SplitN(strings.TrimSpace(strings.TrimPrefix(causeCode, change.prefix)), " -> ", 2)
		if len(values) != 2 || strings.TrimSpace(values[0]) == "" || strings.TrimSpace(values[1]) == "" {
			return "倍率发生变化", true
		}
		return fmt.Sprintf("倍率从 %s %s至 %s", strings.TrimSpace(values[0]), change.verb, strings.TrimSpace(values[1])), true
	}
	return "", false
}

var rateSyncHTTPStatusPattern = regexp.MustCompile(`HTTP\s+([1-5][0-9]{2})`)

func compactRateSyncReason(reason string) string {
	reason = strings.TrimSpace(reason)
	for _, prefix := range []string{"上游倍率同步失败：", "倍率同步失败："} {
		for strings.HasPrefix(reason, prefix) {
			reason = strings.TrimSpace(strings.TrimPrefix(reason, prefix))
		}
	}
	if reason == "" {
		return "未能读取上游倍率信息"
	}
	statuses := map[string]struct{}{}
	for _, match := range rateSyncHTTPStatusPattern.FindAllStringSubmatch(reason, -1) {
		statuses[match[1]] = struct{}{}
	}
	scopes := make([]string, 0, 2)
	if strings.Contains(reason, "分组目录读取失败") {
		scopes = append(scopes, "分组目录")
	}
	if strings.Contains(reason, "余额读取失败") {
		scopes = append(scopes, "余额")
	}
	if len(statuses) == 0 && len(scopes) > 0 && strings.Contains(reason, "上游网络请求失败") {
		return "上游网络请求失败，" + strings.Join(scopes, "和") + "读取失败"
	}
	if len(statuses) == 1 && len(scopes) > 0 {
		status := ""
		for value := range statuses {
			status = value
		}
		return "上游返回 HTTP " + status + "，" + strings.Join(scopes, "和") + "读取失败"
	}
	if len(statuses) > 0 && len(scopes) > 0 {
		details := make([]string, 0, len(scopes))
		for _, part := range strings.Split(reason, "；") {
			scope := ""
			if strings.Contains(part, "分组目录读取失败") {
				scope = "分组目录"
			} else if strings.Contains(part, "余额读取失败") {
				scope = "余额"
			}
			if scope == "" {
				continue
			}
			match := rateSyncHTTPStatusPattern.FindStringSubmatch(part)
			if len(match) > 1 {
				scope += "（HTTP " + match[1] + "）"
			}
			details = append(details, scope)
		}
		if len(details) > 0 {
			return strings.Join(details, "、") + "读取失败"
		}
	}
	if len(statuses) == 1 {
		status := ""
		for value := range statuses {
			status = value
		}
		return "上游返回 HTTP " + status + "，倍率同步未完成"
	}
	return truncateWithEllipsis(reason, 120)
}

func truncateWithEllipsis(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit-1]) + "…"
}

func incidentTableRow(incident business.AlertIncident) string {
	fields := notificationIncidentFields(incident)
	return fmt.Sprintf("| %s | %s | %s | %s | %s |",
		markdownTableValue(fields.event), markdownTableValue(fields.object),
		markdownTableValue(fields.cause), fields.status, markdownTableValue(fields.observedAt),
	)
}

func dynamicAlertCause(cause string) (string, string, bool) {
	for _, code := range []string{
		"AUTH", "PROBE", "CONFIG_AUTH_STATUS_UNKNOWN", "CONFIG_BALANCE_INVALID",
		"ROUTING_BREAKER", "ROUTING_DEGRADED_HEALTH_SCORE", "ROUTING_DEGRADED_GATEWAY_ERROR_RATE",
		"ROUTING_DEGRADED_LATENCY", "ROUTING_DEGRADED_OTHER", "ROUTING_DEGRADED",
		"ROUTING_SURVIVOR", "BINDING_INVALID", "APPLY_FAILED",
	} {
		prefix := code + ":"
		if strings.HasPrefix(cause, prefix) {
			return code, strings.TrimSpace(strings.TrimPrefix(cause, prefix)), true
		}
	}
	return "", "", false
}

func accountGroupIncidentPrefix(eventType, accountID string) string {
	switch eventType {
	case "account.probe":
		return "console:probe:" + accountID + ":"
	case "account.routing_breaker":
		return "console:routing:breaker:" + accountID + ":"
	case "account.routing_degraded":
		return "console:routing:degraded:" + accountID + ":"
	case "account.routing_survivor":
		return "console:routing:survivor:" + accountID + ":"
	case "account.binding_invalid":
		return "console:routing:binding-invalid:" + accountID + ":"
	default:
		return ""
	}
}

var markdownSpecial = regexp.MustCompile("([`*_{}\\[\\]()#+!|>])")

func markdownTableValue(value string) string {
	text := strings.ReplaceAll(value, "\\", "\\\\")
	text = markdownSpecial.ReplaceAllString(text, `\$1`)
	text = truncateRunes(text, 1000)
	text = strings.ReplaceAll(text, "\r\n", "<br>")
	text = strings.ReplaceAll(text, "\r", "<br>")
	return strings.ReplaceAll(text, "\n", "<br>")
}

func notificationTime(value string) string {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	if err != nil {
		return value
	}
	beijing := time.FixedZone("CST", 8*60*60)
	return parsed.In(beijing).Format("2006-01-02 15:04:05")
}

func truncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}

type QQBotSender struct {
	client *http.Client
}

type qqBotCommitUnknownError struct {
	detail string
}

func (err *qqBotCommitUnknownError) Error() string { return err.detail }

func NewQQBotSender(client *http.Client) *QQBotSender {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	copy := *client
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &QQBotSender{client: &copy}
}

func (s *QQBotSender) Send(
	ctx context.Context,
	settings configstore.NotificationSettings,
	messages []string,
) []SendOutcome {
	if len(messages) == 0 {
		return []SendOutcome{}
	}
	validationError := validateSettings(settings)
	if validationError != "" {
		return repeatedFailure(len(messages), validationError)
	}
	endpoint := qqbotMessageEndpoint(settings.HomeChannelType, settings.HomeChannel)
	if endpoint == "" {
		return repeatedFailure(len(messages), "QQBot 目标类型无效")
	}
	accessToken, detail := s.accessToken(ctx, settings)
	if detail != "" {
		return repeatedFailure(len(messages), detail)
	}
	result := make([]SendOutcome, 0, len(messages))
	baseSequence := time.Now().UnixMilli()
	for index, message := range messages {
		payload := map[string]any{
			"markdown": map[string]string{"content": message},
			"msg_type": 2,
			"msg_seq":  (baseSequence + int64(index)) % 1_000_000_000,
		}
		response, err := s.postJSON(ctx, endpoint, payload, map[string]string{"Authorization": "QQBot " + accessToken})
		if err != nil {
			var unknown *qqBotCommitUnknownError
			result = append(result, SendOutcome{CommitUnknown: errors.As(err, &unknown), Detail: err.Error()})
			continue
		}
		messageID, idPresent := response["id"]
		if !idPresent {
			messageID = response["message_id"]
		}
		if messageID == nil || strings.TrimSpace(fmt.Sprint(messageID)) == "" {
			result = append(result, SendOutcome{CommitUnknown: true, Detail: "QQBot 发送响应缺少消息 ID"})
			continue
		}
		id := fmt.Sprint(messageID)
		result = append(result, SendOutcome{Success: true, Detail: "QQBot 已确认发送", MessageID: &id})
	}
	return result
}

func (s *QQBotSender) accessToken(ctx context.Context, settings configstore.NotificationSettings) (string, string) {
	response, err := s.postJSON(ctx, tokenEndpoint, map[string]string{
		"appId": settings.AppID, "clientSecret": settings.ClientSecret,
	}, nil)
	if err != nil {
		return "", err.Error()
	}
	accessToken, found := response["access_token"]
	if !found || accessToken == nil || strings.TrimSpace(fmt.Sprint(accessToken)) == "" {
		return "", "QQBot 鉴权响应缺少 access_token"
	}
	return fmt.Sprint(accessToken), ""
}

func (s *QQBotSender) postJSON(ctx context.Context, endpoint string, payload any, headers map[string]string) (map[string]any, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.New("QQBot 请求编码失败")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("QQBot 请求创建失败")
	}
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := s.client.Do(request)
	if err != nil {
		return nil, &qqBotCommitUnknownError{detail: fmt.Sprintf("QQBot 网络请求失败：%T", err)}
	}
	defer response.Body.Close()
	limited, readErr := io.ReadAll(io.LimitReader(response.Body, maximumQQBotResponseSize+1))
	if readErr != nil {
		return nil, &qqBotCommitUnknownError{detail: "QQBot 响应读取失败"}
	}
	if len(limited) > maximumQQBotResponseSize {
		return nil, &qqBotCommitUnknownError{detail: "QQBot 响应过大"}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail := redactSecrets(strings.ReplaceAll(string(limited), "\n", " "))
		if strings.TrimSpace(detail) == "" {
			detail = "上游未返回错误详情"
		}
		return nil, fmt.Errorf("QQBot 请求失败（HTTP %d）：%s", response.StatusCode, truncateRunes(detail, 300))
	}
	var decoded map[string]any
	if err := json.Unmarshal(limited, &decoded); err != nil || decoded == nil {
		return nil, &qqBotCommitUnknownError{detail: "QQBot 响应不可解析"}
	}
	return decoded, nil
}

func validateSettings(settings configstore.NotificationSettings) string {
	if settings.AppID == "" || settings.ClientSecret == "" || settings.HomeChannel == "" {
		return "QQBot 配置不完整"
	}
	if len(settings.HomeChannel) > 256 || strings.ContainsAny(settings.HomeChannel, "/\\\r\n") {
		return "QQBot 目标格式无效"
	}
	return ""
}

func qqbotMessageEndpoint(channelType, destination string) string {
	escaped := url.PathEscape(destination)
	switch channelType {
	case "channel":
		return "https://api.sgroup.qq.com/channels/" + escaped + "/messages"
	case "c2c":
		return "https://api.sgroup.qq.com/v2/users/" + escaped + "/messages"
	case "group":
		return "https://api.sgroup.qq.com/v2/groups/" + escaped + "/messages"
	default:
		return ""
	}
}

func repeatedFailure(count int, detail string) []SendOutcome {
	result := make([]SendOutcome, count)
	for index := range result {
		result[index] = SendOutcome{Detail: detail}
	}
	return result
}

func redactSecrets(value string) string {
	return redact.Secrets(value)
}
