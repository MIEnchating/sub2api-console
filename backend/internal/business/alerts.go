package business

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type AlertIncident struct {
	IncidentKey    string  `json:"incident_key"`
	EventType      string  `json:"event_type"`
	ObjectKind     string  `json:"object_kind"`
	ObjectID       string  `json:"object_id"`
	ObjectName     *string `json:"object_name,omitempty"`
	CauseCode      string  `json:"cause_code"`
	Status         string  `json:"status"`
	FirstSeenAt    string  `json:"first_seen_at"`
	LastSeenAt     string  `json:"last_seen_at"`
	DeliveryStatus *string `json:"delivery_status"`
	LastError      *string `json:"last_error"`
}

type AlertDeliveryPlan struct {
	Pending        []AlertIncident
	Skipped        int
	Suppressed     int
	Configured     bool
	Disabled       bool
	ChannelKey     string
	MergeThreshold int
}

type AlertDeliveryOutcome struct {
	IncidentKey string
	Success     bool
	Detail      string
	MessageID   *string
}

type AlertDeliveryResult struct {
	Attempted  int      `json:"attempted"`
	Sent       int      `json:"sent"`
	Failed     int      `json:"failed"`
	Skipped    int      `json:"skipped"`
	Suppressed int      `json:"suppressed"`
	Configured bool     `json:"configured"`
	Disabled   bool     `json:"disabled,omitempty"`
	DryRun     bool     `json:"dry_run"`
	MessageIDs []string `json:"message_ids"`
	Batches    int      `json:"batches"`
}

type NotificationQueueSnapshot struct {
	ProducerFiring    int `json:"producer_firing"`
	ProducerRecovered int `json:"producer_recovered"`
	ConsumerPending   int `json:"consumer_pending"`
	ConsumerFailed    int `json:"consumer_failed"`
}

type NotificationQueueItem struct {
	AlertIncident
	DeliveryAttempts int     `json:"delivery_attempts"`
	DeliveredAt      *string `json:"delivered_at"`
	QueueStatus      string  `json:"queue_status,omitempty"`
	QueueReason      string  `json:"queue_reason,omitempty"`
}

type NotificationQueueDetails struct {
	ProducerFiring    []NotificationQueueItem `json:"producer_firing"`
	ProducerRecovered []NotificationQueueItem `json:"producer_recovered"`
	ConsumerPending   []NotificationQueueItem `json:"consumer_pending"`
	ConsumerFailed    []NotificationQueueItem `json:"consumer_failed"`
	ConsumerItems     []NotificationQueueItem `json:"consumer_items"`
}

type alertPolicy struct {
	DeliveryEnabled           bool
	NotifyRecovery            bool
	RecoveryNotificationTypes map[string]struct{}
	RepeatIntervalMinute      int
	StateChangeCooldownMinute int
	MergeThreshold            int
}

func NotificationChannelKey(channelType, destination string) string {
	digest := sha256.Sum256([]byte(channelType + "\x00" + destination))
	return channelType + ":" + hex.EncodeToString(digest[:])[:16]
}

func (s *Store) PrepareAlertDelivery(
	ctx context.Context,
	channelKey string,
	privateConfigured bool,
) (AlertDeliveryPlan, error) {
	policy, err := s.readAlertPolicy(ctx)
	if err != nil {
		return AlertDeliveryPlan{}, err
	}
	enabled, qqbotEnabled, configurationErrors, err := s.notificationRules(ctx, privateConfigured)
	if err != nil {
		return AlertDeliveryPlan{}, err
	}
	incidents, err := s.deliveryIncidents(ctx)
	if err != nil {
		return AlertDeliveryPlan{}, err
	}
	plan := AlertDeliveryPlan{
		Pending: []AlertIncident{}, Configured: enabled && qqbotEnabled && privateConfigured, ChannelKey: channelKey,
		MergeThreshold: policy.MergeThreshold,
	}
	if !policy.DeliveryEnabled {
		if err := s.markIncidentDeliveryState(ctx, incidents, "通知发送已关闭", nil); err != nil {
			return AlertDeliveryPlan{}, err
		}
		plan.Suppressed = len(incidents)
		plan.Disabled = true
		return plan, nil
	}
	recovered := make([]AlertIncident, 0)
	active := make([]AlertIncident, 0, len(incidents))
	for _, incident := range incidents {
		if incident.Status == "recovered" && !recoveryNotificationEnabled(policy, incident.EventType) {
			recovered = append(recovered, incident)
		} else {
			active = append(active, incident)
		}
	}
	if err := s.markIncidentDeliveryState(ctx, recovered, "恢复通知已关闭", nil); err != nil {
		return AlertDeliveryPlan{}, err
	}
	plan.Suppressed = len(recovered)
	incidents = active
	// A recovery deliberately suppressed by policy is final for that incident.
	// Re-enabling recovery notifications must not replay stale recoveries.
	if policy.NotifyRecovery {
		active := make([]AlertIncident, 0, len(incidents))
		for _, incident := range incidents {
			if incident.Status == "recovered" && pointerTextValue(incident.DeliveryStatus) == "恢复通知已关闭" {
				plan.Suppressed++
				continue
			}
			active = append(active, incident)
		}
		incidents = active
	}
	if len(incidents) == 0 {
		return plan, nil
	}
	if len(configurationErrors) > 0 {
		detail := "通知配置无效：" + strings.Join(configurationErrors, "、")
		if err := s.markIncidentDeliveryState(ctx, incidents, "通知配置无效", &detail); err != nil {
			return AlertDeliveryPlan{}, err
		}
		plan.Skipped = len(incidents)
		plan.Configured = false
		return plan, nil
	}
	if !privateConfigured {
		detail := "QQBot 通知凭据或目标未配置完整"
		if err := s.markIncidentDeliveryState(ctx, incidents, "未配置渠道", &detail); err != nil {
			return AlertDeliveryPlan{}, err
		}
		plan.Skipped = len(incidents)
		plan.Configured = false
		return plan, nil
	}
	if !enabled || !qqbotEnabled {
		detail := "未配置通知渠道"
		if err := s.markIncidentDeliveryState(ctx, incidents, "未配置渠道", &detail); err != nil {
			return AlertDeliveryPlan{}, err
		}
		plan.Skipped = len(incidents)
		plan.Configured = false
		return plan, nil
	}
	prior, err := s.priorDeliveries(ctx, channelKey)
	if err != nil {
		return AlertDeliveryPlan{}, err
	}
	now := time.Now().UTC()
	degradedDigestCooling := !deliveryCooldownDue(
		latestDegradedDelivery(prior),
		policy.StateChangeCooldownMinute,
		now,
	)
	for _, incident := range incidents {
		previous, found := prior[incident.IncidentKey]
		if !found && incident.EventType == "account.routing_degraded" && incident.Status == "recovered" {
			plan.Skipped++
			continue
		}
		if !found && incident.EventType == "account.routing_degraded" &&
			!incidentObservationDue(incident.FirstSeenAt, policy.StateChangeCooldownMinute, now) {
			plan.Skipped++
			continue
		}
		coolingDown := found && previous.status == "transition" &&
			!deliveryCooldownDue(previous.updatedAt, policy.StateChangeCooldownMinute, now)
		if coolingDown {
			plan.Skipped++
			continue
		}
		if incident.EventType == "account.routing_degraded" && degradedDigestCooling {
			plan.Skipped++
			continue
		}
		repeatDue := incident.Status == "firing" && deliveryRepeatDue(previous.deliveredAt, policy.RepeatIntervalMinute, now)
		if found && previous.status == "sent" && !repeatDue {
			plan.Skipped++
			continue
		}
		plan.Pending = append(plan.Pending, incident)
	}
	return plan, nil
}

func (s *Store) NotificationQueueSnapshot(ctx context.Context, channelKey string, privateConfigured bool) (NotificationQueueSnapshot, error) {
	details, err := s.NotificationQueueDetails(ctx, channelKey, privateConfigured)
	if err != nil {
		return NotificationQueueSnapshot{}, err
	}
	return NotificationQueueSnapshot{
		ProducerFiring:    len(details.ProducerFiring),
		ProducerRecovered: len(details.ProducerRecovered),
		ConsumerPending:   len(details.ConsumerPending),
		ConsumerFailed:    len(details.ConsumerFailed),
	}, nil
}

func (s *Store) NotificationQueueDetails(ctx context.Context, channelKey string, privateConfigured bool) (NotificationQueueDetails, error) {
	policy, err := s.readAlertPolicy(ctx)
	if err != nil {
		return NotificationQueueDetails{}, err
	}
	incidents, err := s.deliveryIncidents(ctx)
	if err != nil {
		return NotificationQueueDetails{}, err
	}
	prior, err := s.priorDeliveries(ctx, channelKey)
	if err != nil {
		return NotificationQueueDetails{}, err
	}
	enabled, qqbotEnabled, configurationErrors, err := s.notificationRules(ctx, privateConfigured)
	if err != nil {
		return NotificationQueueDetails{}, err
	}
	result := NotificationQueueDetails{
		ProducerFiring:    []NotificationQueueItem{},
		ProducerRecovered: []NotificationQueueItem{},
		ConsumerPending:   []NotificationQueueItem{},
		ConsumerFailed:    []NotificationQueueItem{},
		ConsumerItems:     []NotificationQueueItem{},
	}
	now := time.Now().UTC()
	degradedDigestCooling := !deliveryCooldownDue(
		latestDegradedDelivery(prior),
		policy.StateChangeCooldownMinute,
		now,
	)
	for _, incident := range incidents {
		previous, found := prior[incident.IncidentKey]
		item := NotificationQueueItem{AlertIncident: incident}
		if found {
			item.DeliveryAttempts = previous.attempts
			item.DeliveredAt = previous.deliveredAt
		}
		if incident.Status == "firing" {
			result.ProducerFiring = append(result.ProducerFiring, item)
		} else if incident.Status == "recovered" {
			result.ProducerRecovered = append(result.ProducerRecovered, item)
		}
		if !found && incident.EventType == "account.routing_degraded" && incident.Status == "recovered" {
			item.QueueStatus = "无需发送"
			item.QueueReason = "异常在首次通知前已恢复"
			result.ConsumerItems = append(result.ConsumerItems, item)
			continue
		}
		if !found && incident.EventType == "account.routing_degraded" &&
			!incidentObservationDue(incident.FirstSeenAt, policy.StateChangeCooldownMinute, now) {
			item.QueueStatus = "状态观察中"
			item.QueueReason = fmt.Sprintf("持续 %d 分钟后才通知，避免瞬时降级刷屏", policy.StateChangeCooldownMinute)
			result.ConsumerItems = append(result.ConsumerItems, item)
			continue
		}
		if !policy.DeliveryEnabled {
			item.QueueStatus = "已抑制"
			item.QueueReason = "告警通知发送已关闭"
			result.ConsumerItems = append(result.ConsumerItems, item)
			continue
		}
		if incident.Status == "recovered" && (!recoveryNotificationEnabled(policy, incident.EventType) || pointerTextValue(incident.DeliveryStatus) == "恢复通知已关闭") {
			item.QueueStatus = "已抑制"
			item.QueueReason = "恢复通知已关闭"
			result.ConsumerItems = append(result.ConsumerItems, item)
			continue
		}
		if len(configurationErrors) > 0 {
			item.QueueStatus = "已抑制"
			item.QueueReason = "通知配置无效：" + strings.Join(configurationErrors, "、")
			result.ConsumerItems = append(result.ConsumerItems, item)
			continue
		}
		if !privateConfigured {
			item.QueueStatus = "已抑制"
			item.QueueReason = "QQBot 通知凭据或目标未配置完整"
			result.ConsumerItems = append(result.ConsumerItems, item)
			continue
		}
		if !enabled {
			item.QueueStatus = "已抑制"
			item.QueueReason = "通知规则已关闭"
			result.ConsumerItems = append(result.ConsumerItems, item)
			continue
		}
		if !qqbotEnabled {
			item.QueueStatus = "已抑制"
			item.QueueReason = "QQBot 通知渠道未启用"
			result.ConsumerItems = append(result.ConsumerItems, item)
			continue
		}
		if found && previous.status == "failed" {
			item.QueueStatus = "发送失败，等待重试"
			item.QueueReason = pointerTextValue(incident.LastError)
			if item.QueueReason == "" {
				item.QueueReason = "上次通知发送失败"
			}
			result.ConsumerFailed = append(result.ConsumerFailed, item)
			result.ConsumerPending = append(result.ConsumerPending, item)
			result.ConsumerItems = append(result.ConsumerItems, item)
			continue
		}
		if found && previous.status == "transition" &&
			!deliveryCooldownDue(previous.updatedAt, policy.StateChangeCooldownMinute, now) {
			item.QueueStatus = "状态变化冷却中"
			item.QueueReason = fmt.Sprintf("距离上次通知不足 %d 分钟", policy.StateChangeCooldownMinute)
			result.ConsumerItems = append(result.ConsumerItems, item)
			continue
		}
		if incident.EventType == "account.routing_degraded" && degradedDigestCooling {
			item.QueueStatus = "降级告警汇总冷却中"
			item.QueueReason = fmt.Sprintf("本渠道 %d 分钟内的降级变化将在冷却结束后统一汇总发送", policy.StateChangeCooldownMinute)
			result.ConsumerItems = append(result.ConsumerItems, item)
			continue
		}
		repeatDue := incident.Status == "firing" && deliveryRepeatDue(previous.deliveredAt, policy.RepeatIntervalMinute, now)
		if !found || previous.status != "sent" || repeatDue {
			item.QueueStatus = "待发送"
			switch {
			case !found:
				item.QueueReason = "尚未发送通知"
			case repeatDue:
				item.QueueReason = "已到再次通知时间"
			default:
				item.QueueReason = "等待通知服务处理"
			}
			result.ConsumerPending = append(result.ConsumerPending, item)
			result.ConsumerItems = append(result.ConsumerItems, item)
			continue
		}
		item.QueueStatus = "本轮不发送"
		switch {
		case incident.Status == "recovered":
			item.QueueReason = "恢复通知已发送"
		case policy.RepeatIntervalMinute <= 0:
			item.QueueReason = "持续告警已通知，策略设置为只发送一次"
		default:
			item.QueueReason = "尚未到再次通知时间"
		}
		result.ConsumerItems = append(result.ConsumerItems, item)
	}
	return result, nil
}

func pointerTextValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func (s *Store) FinalizeAlertDelivery(ctx context.Context, channelKey string, outcomes []AlertDeliveryOutcome) error {
	if len(outcomes) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, outcome := range outcomes {
		status := "failed"
		deliveryStatus := "发送失败"
		var lastError any = outcome.Detail
		var deliveredAt any
		if outcome.Success {
			status = "sent"
			deliveryStatus = "已发送"
			lastError = nil
			deliveredAt = now
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO alert_deliveries(
			incident_key,channel_key,status,attempts,last_error,delivered_at,updated_at
		) VALUES(?,?,?,?,?,?,?) ON CONFLICT(incident_key,channel_key) DO UPDATE SET
			status=excluded.status,attempts=alert_deliveries.attempts+1,last_error=excluded.last_error,
			delivered_at=excluded.delivered_at,updated_at=excluded.updated_at`,
			outcome.IncidentKey, channelKey, status, 1, lastError, deliveredAt, now,
		); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE alert_incidents SET delivery_status=?,last_error=?
			WHERE incident_key=?`, deliveryStatus, lastError, outcome.IncidentKey); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) readAlertPolicy(ctx context.Context) (alertPolicy, error) {
	policy, err := s.AlertPolicy(ctx)
	if err != nil {
		return alertPolicy{}, err
	}
	return alertPolicy{
		DeliveryEnabled:           policy.DeliveryEnabled,
		NotifyRecovery:            policy.NotifyRecovery,
		RecoveryNotificationTypes: valueStringSet(policy.RecoveryNotificationTypes...),
		RepeatIntervalMinute:      policy.RepeatIntervalMinutes,
		StateChangeCooldownMinute: policy.StateChangeCooldown,
		MergeThreshold:            policy.MergeThreshold,
	}, nil
}

func recoveryNotificationEnabled(policy alertPolicy, eventType string) bool {
	if !policy.NotifyRecovery {
		return false
	}
	category := recoveryNotificationType(eventType)
	if category == "" {
		return false
	}
	_, enabled := policy.RecoveryNotificationTypes[category]
	return enabled
}

func recoveryNotificationType(eventType string) string {
	switch eventType {
	case "upstream.configuration":
		return "configuration"
	case "upstream.auth":
		return "auth"
	case "upstream.rate_sync":
		return "rate_sync"
	case "upstream.balance":
		return "balance"
	case "account.probe":
		return "probe"
	case "account.routing_breaker", "account.binding_invalid":
		return "routing_breaker"
	case "account.routing_degraded":
		return "routing_degraded"
	case "account.routing_survivor":
		return "routing_survivor"
	case "group.routing_unavailable":
		return "group_unavailable"
	case "group.routing_survivor":
		return "group_survivor"
	case "routing.apply_failure":
		return "apply_failure"
	default:
		return ""
	}
}

func (s *Store) notificationRules(ctx context.Context, privateConfigured bool) (bool, bool, []string, error) {
	var source map[string]any
	configurationErrors := make([]string, 0)
	var rawSnapshot string
	err := s.db.QueryRowContext(ctx, `SELECT value_json FROM operational_snapshots
		WHERE state_key='sub2api-notify-rules.json' ORDER BY updated_at DESC LIMIT 1`).Scan(&rawSnapshot)
	if err == nil {
		decoded, decodeErr := decodeObject(rawSnapshot)
		if decodeErr != nil {
			configurationErrors = append(configurationErrors, "notifications.snapshot")
			source = map[string]any{}
		} else {
			source = decoded
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return false, false, nil, err
	} else {
		control, readErr := s.readPolicyDocument(ctx, s.db, "control-plane")
		if readErr != nil {
			return false, false, nil, readErr
		}
		if control == nil {
			if privateConfigured {
				source = map[string]any{"enabled": true, "channels": []any{map[string]any{"type": "qqbot", "enabled": true}}}
			} else {
				source = map[string]any{}
			}
		} else if raw, present := control["notifications"]; present {
			var ok bool
			source, ok = raw.(map[string]any)
			if !ok {
				configurationErrors = append(configurationErrors, "notifications")
				source = map[string]any{}
			}
		} else {
			source = map[string]any{}
		}
	}
	enabled, valid := notificationEnabled(source, "enabled", "delivery_enabled")
	if !valid {
		configurationErrors = append(configurationErrors, "notifications.enabled")
	}
	channelsRaw, present := source["channels"]
	if !present {
		channelsRaw = []any{}
	}
	channels, ok := channelsRaw.([]any)
	if !ok {
		configurationErrors = append(configurationErrors, "notifications.channels")
		channels = []any{}
	}
	qqbotEnabled := false
	for index, raw := range channels {
		channel, ok := raw.(map[string]any)
		if !ok {
			configurationErrors = append(configurationErrors, fmt.Sprintf("notifications.channels[%d]", index))
			continue
		}
		channelEnabled, valid := notificationEnabled(channel, "enabled")
		if !valid {
			configurationErrors = append(configurationErrors, fmt.Sprintf("notifications.channels[%d].enabled", index))
		}
		if channelEnabled && strings.EqualFold(strings.TrimSpace(stringValue(channel["type"])), "qqbot") {
			qqbotEnabled = true
		}
	}
	sort.Strings(configurationErrors)
	configurationErrors = uniqueText(configurationErrors)
	return enabled, qqbotEnabled, configurationErrors, nil
}

func notificationEnabled(source map[string]any, fields ...string) (bool, bool) {
	for _, field := range fields {
		value, present := source[field]
		if !present {
			continue
		}
		parsed := strictAnyBool(value)
		if parsed == nil {
			return false, false
		}
		return *parsed, true
	}
	return true, true
}

func (s *Store) deliveryIncidents(ctx context.Context) ([]AlertIncident, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT i.incident_key,i.event_type,i.object_kind,i.object_id,a.name,i.cause_code,
		i.status,i.first_seen_at,i.last_seen_at,i.delivery_status,i.last_error FROM alert_incidents i
		LEFT JOIN accounts a ON i.object_kind='account' AND a.id=i.object_id
		WHERE i.status IN ('firing','recovered') ORDER BY i.last_seen_at,i.incident_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]AlertIncident, 0)
	for rows.Next() {
		var item AlertIncident
		var objectName, deliveryStatus, lastError sql.NullString
		if err := rows.Scan(&item.IncidentKey, &item.EventType, &item.ObjectKind, &item.ObjectID,
			&objectName, &item.CauseCode, &item.Status, &item.FirstSeenAt, &item.LastSeenAt, &deliveryStatus, &lastError); err != nil {
			return nil, err
		}
		item.ObjectName = nullString(objectName)
		item.DeliveryStatus = nullString(deliveryStatus)
		item.LastError = nullString(lastError)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) markIncidentDeliveryState(ctx context.Context, incidents []AlertIncident, status string, detail *string) error {
	if len(incidents) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, incident := range incidents {
		if _, err := tx.ExecContext(ctx, `UPDATE alert_incidents SET delivery_status=?,last_error=?
			WHERE incident_key=?`, status, detail, incident.IncidentKey); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type priorDelivery struct {
	eventType   string
	status      string
	attempts    int
	deliveredAt *string
	updatedAt   *string
}

func (s *Store) priorDeliveries(ctx context.Context, channelKey string) (map[string]priorDelivery, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT d.incident_key,i.event_type,d.status,d.attempts,d.delivered_at,d.updated_at
		FROM alert_deliveries d JOIN alert_incidents i ON i.incident_key=d.incident_key
		WHERE d.channel_key=?`, channelKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]priorDelivery{}
	for rows.Next() {
		var incidentKey, eventType, status string
		var attempts int
		var deliveredAt, updatedAt sql.NullString
		if err := rows.Scan(&incidentKey, &eventType, &status, &attempts, &deliveredAt, &updatedAt); err != nil {
			return nil, err
		}
		result[incidentKey] = priorDelivery{
			eventType: eventType, status: status, attempts: attempts,
			deliveredAt: nullString(deliveredAt), updatedAt: nullString(updatedAt),
		}
	}
	return result, rows.Err()
}

func deliveryRepeatDue(deliveredAt *string, intervalMinutes int, now time.Time) bool {
	if intervalMinutes <= 0 {
		return false
	}
	if deliveredAt == nil || strings.TrimSpace(*deliveredAt) == "" {
		return true
	}
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(*deliveredAt))
	if err != nil {
		return true
	}
	return now.Sub(parsed) >= time.Duration(intervalMinutes)*time.Minute
}

func deliveryCooldownDue(deliveredAt *string, intervalMinutes int, now time.Time) bool {
	if intervalMinutes <= 0 {
		return true
	}
	return deliveryRepeatDue(deliveredAt, intervalMinutes, now)
}

func latestDegradedDelivery(prior map[string]priorDelivery) *string {
	var latest time.Time
	var latestText string
	for _, previous := range prior {
		if previous.eventType != "account.routing_degraded" {
			continue
		}
		if previous.deliveredAt == nil {
			continue
		}
		text := strings.TrimSpace(*previous.deliveredAt)
		parsed, err := time.Parse(time.RFC3339Nano, text)
		if err != nil || (!latest.IsZero() && !parsed.After(latest)) {
			continue
		}
		latest = parsed
		latestText = text
	}
	if latestText == "" {
		return nil
	}
	return &latestText
}

func incidentObservationDue(firstSeenAt string, intervalMinutes int, now time.Time) bool {
	if intervalMinutes <= 0 {
		return true
	}
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(firstSeenAt))
	if err != nil {
		return true
	}
	return now.Sub(parsed) >= time.Duration(intervalMinutes)*time.Minute
}

func strictInteger(value any) (int, error) {
	switch item := value.(type) {
	case int64:
		return int(item), nil
	case int:
		return item, nil
	case float64:
		if item != float64(int(item)) {
			return 0, errors.New("not integer")
		}
		return int(item), nil
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(item))
		if err != nil || strconv.Itoa(parsed) != strings.TrimSpace(item) {
			return 0, errors.New("not integer")
		}
		return parsed, nil
	default:
		return 0, errors.New("not integer")
	}
}

func uniqueText(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}
