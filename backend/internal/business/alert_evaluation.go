package business

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"
)

type AlertEvidenceResult struct {
	Findings           int  `json:"findings"`
	EvaluationDisabled bool `json:"evaluation_disabled"`
}

type AlertEvaluationRecord struct {
	RunKey  string `json:"run_key"`
	EventID int64  `json:"event_id"`
	Status  string `json:"status"`
	Summary string `json:"summary"`
}

type alertFinding struct {
	key, eventType, objectKind, objectID, causeCode string
}

func (s *Store) EvaluateAlertIncidents(ctx context.Context) (AlertEvidenceResult, error) {
	policy, err := s.AlertPolicy(ctx)
	if err != nil {
		return AlertEvidenceResult{}, err
	}
	if !policy.Enabled {
		if err := s.suppressFiringAlertIncidents(ctx); err != nil {
			return AlertEvidenceResult{}, err
		}
		return AlertEvidenceResult{EvaluationDisabled: true}, nil
	}
	findings, notEvaluated, err := s.alertFindings(ctx, policy)
	if err != nil {
		return AlertEvidenceResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AlertEvidenceResult{}, err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	current := make(map[string]struct{}, len(findings))
	currentScopes := make(map[string]struct{}, len(findings))
	for _, finding := range findings {
		current[finding.key] = struct{}{}
		currentScopes[alertIncidentScope(finding.key, finding.eventType, finding.objectKind, finding.objectID)] = struct{}{}
		var previous sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT status FROM alert_incidents WHERE incident_key=?`, finding.key).Scan(&previous)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return AlertEvidenceResult{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO alert_incidents(incident_key,event_type,object_kind,object_id,cause_code,status,first_seen_at,last_seen_at,delivery_status,last_error)
			VALUES(?,?,?,?,?,'firing',?,?,'未配置渠道',NULL) ON CONFLICT(incident_key) DO UPDATE SET
			status='firing',last_seen_at=excluded.last_seen_at,cause_code=excluded.cause_code`,
			finding.key, finding.eventType, finding.objectKind, finding.objectID, finding.causeCode, now, now); err != nil {
			return AlertEvidenceResult{}, err
		}
		if previous.Valid && previous.String != "firing" {
			if _, err := tx.ExecContext(ctx, `DELETE FROM alert_deliveries WHERE incident_key=?`, finding.key); err != nil {
				return AlertEvidenceResult{}, err
			}
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT incident_key,event_type,object_kind,object_id,status FROM alert_incidents WHERE status IN ('firing','suppressed')`)
	if err != nil {
		return AlertEvidenceResult{}, err
	}
	defer rows.Close()
	type activeIncident struct {
		key, eventType, objectKind, objectID, status string
	}
	active := []activeIncident{}
	for rows.Next() {
		var incident activeIncident
		if err := rows.Scan(&incident.key, &incident.eventType, &incident.objectKind, &incident.objectID, &incident.status); err != nil {
			return AlertEvidenceResult{}, err
		}
		active = append(active, incident)
	}
	if err := rows.Err(); err != nil {
		return AlertEvidenceResult{}, err
	}
	if err := rows.Close(); err != nil {
		return AlertEvidenceResult{}, err
	}
	for _, incident := range active {
		if _, stillFiring := current[incident.key]; stillFiring {
			continue
		}
		ruleEnabled := alertRuleEnabled(policy, incident.eventType)
		_, sameScopeStillFiring := currentScopes[alertIncidentScope(incident.key, incident.eventType, incident.objectKind, incident.objectID)]
		switch {
		case incident.status == "firing" && !ruleEnabled:
			if _, err := tx.ExecContext(ctx, `UPDATE alert_incidents SET status='suppressed',last_seen_at=?,delivery_status='规则已停用',last_error=NULL WHERE incident_key=?`, now, incident.key); err != nil {
				return AlertEvidenceResult{}, err
			}
		case incident.status == "firing" && sameScopeStillFiring:
			detail := "同一对象的其他异常仍存在"
			if incident.eventType == "upstream.balance" {
				detail = "余额告警阈值档位已变化"
			} else if strings.HasPrefix(incident.eventType, "account.routing_") || strings.HasPrefix(incident.eventType, "group.routing_") {
				detail = "同一调度对象的其他异常仍存在"
			}
			if _, err := tx.ExecContext(ctx, `UPDATE alert_incidents SET status='closed',last_seen_at=?,delivery_status=?,last_error=NULL WHERE incident_key=?`, now, detail, incident.key); err != nil {
				return AlertEvidenceResult{}, err
			}
		case incident.status == "firing" && notEvaluated[incident.key] != "":
			if _, err := tx.ExecContext(ctx, `UPDATE alert_incidents SET status='closed',last_seen_at=?,delivery_status=?,last_error=NULL WHERE incident_key=?`, now, notEvaluated[incident.key], incident.key); err != nil {
				return AlertEvidenceResult{}, err
			}
		case incident.status == "firing":
			if _, err := tx.ExecContext(ctx, `UPDATE alert_incidents SET status='recovered',last_seen_at=?,last_error=NULL WHERE incident_key=?`, now, incident.key); err != nil {
				return AlertEvidenceResult{}, err
			}
		case incident.status == "suppressed" && ruleEnabled:
			detail := "停用期间异常已消失"
			if notEvaluated[incident.key] != "" {
				detail = notEvaluated[incident.key]
			}
			if _, err := tx.ExecContext(ctx, `UPDATE alert_incidents SET status='closed',last_seen_at=?,delivery_status=?,last_error=NULL WHERE incident_key=?`, now, detail, incident.key); err != nil {
				return AlertEvidenceResult{}, err
			}
		default:
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM alert_deliveries WHERE incident_key=?`, incident.key); err != nil {
			return AlertEvidenceResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return AlertEvidenceResult{}, err
	}
	return AlertEvidenceResult{Findings: len(findings)}, nil
}

func alertIncidentScope(incidentKey, eventType, objectKind, objectID string) string {
	if eventType == "account.probe" {
		return incidentKey
	}
	if strings.HasPrefix(eventType, "account.routing_") {
		remainder := strings.TrimPrefix(incidentKey, "console:routing:")
		if separator := strings.IndexByte(remainder, ':'); separator >= 0 {
			return "account.routing\x00" + remainder[separator+1:]
		}
		return "account.routing\x00" + objectID
	}
	if strings.HasPrefix(eventType, "group.routing_") {
		return "group.routing\x00" + objectID
	}
	return eventType + "\x00" + objectKind + "\x00" + objectID
}

func (s *Store) suppressFiringAlertIncidents(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE alert_incidents SET status='suppressed',last_seen_at=?,delivery_status='告警总开关已关闭',last_error=NULL WHERE status='firing'`, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE alert_incidents SET status='closed',last_seen_at=?,delivery_status='告警总开关已关闭',last_error=NULL WHERE status='recovered'`, now); err != nil {
		return err
	}
	return tx.Commit()
}

func alertRuleEnabled(policy AlertPolicy, eventType string) bool {
	switch eventType {
	case "upstream.configuration":
		return policy.ConfigurationEnabled
	case "upstream.auth":
		return policy.AuthEnabled
	case "upstream.rate_sync":
		return policy.RateSyncEnabled
	case "upstream.balance":
		return policy.BalanceEnabled
	case "account.probe":
		return policy.ProbeEnabled
	case "account.routing_breaker":
		return policy.RoutingBreakerEnabled
	case "account.routing_degraded":
		return policy.RoutingDegradedEnabled
	case "account.routing_survivor":
		return policy.RoutingSurvivorEnabled
	case "group.routing_unavailable":
		return policy.GroupUnavailableEnabled
	case "group.routing_survivor":
		return policy.GroupSurvivorEnabled
	case "routing.apply_failure":
		return policy.ApplyFailureEnabled
	default:
		return true
	}
}

func (s *Store) RecordAlertEvaluation(ctx context.Context, startedAt string, evidence AlertEvidenceResult, delivery AlertDeliveryResult) (AlertEvaluationRecord, error) {
	ended := time.Now().UTC().Format(time.RFC3339Nano)
	runKey, err := randomRunKey("console:alerts:")
	if err != nil {
		return AlertEvaluationRecord{}, err
	}
	status := "succeeded"
	summary := fmt.Sprintf("告警检测完成：当前异常 %d 项，发送 %d 项，跳过 %d 项", evidence.Findings, delivery.Sent, delivery.Skipped+delivery.Suppressed)
	if evidence.EvaluationDisabled {
		summary = "告警检测已跳过：告警策略总开关已关闭"
	}
	if delivery.Failed > 0 || (delivery.Skipped > 0 && !delivery.Configured && !delivery.Disabled) {
		status = "failed"
		summary = fmt.Sprintf("告警检测完成，通知发送未完成：当前异常 %d 项，发送 %d 项，失败 %d 项，跳过 %d 项", evidence.Findings, delivery.Sent, delivery.Failed, delivery.Skipped+delivery.Suppressed)
	}
	payload := map[string]any{
		"source": "console-domain-db", "findings": evidence.Findings, "delivery": delivery,
		"remote_write": false, "evaluation_disabled": evidence.EvaluationDisabled,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return AlertEvaluationRecord{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AlertEvaluationRecord{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO run_records(run_key,task_name,status,stage,started_at,ended_at,summary,payload_json,updated_at)
		VALUES(?,'alerts',?,'evaluate',?,?,?,?,?)`, runKey, status, startedAt, ended, summary, string(encoded), ended); err != nil {
		return AlertEvaluationRecord{}, err
	}
	eventPayload := copyObject(payload)
	eventPayload["run_key"] = runKey
	if err := insertRuntimeEventWithStatus(ctx, tx, "alerts.evaluate", status, summary, eventPayload, ended); err != nil {
		return AlertEvaluationRecord{}, err
	}
	var eventID int64
	if err := tx.QueryRowContext(ctx, `SELECT MIN(source_id) FROM runtime_events WHERE source_id < 0`).Scan(&eventID); err != nil {
		return AlertEvaluationRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return AlertEvaluationRecord{}, err
	}
	return AlertEvaluationRecord{RunKey: runKey, EventID: eventID, Status: status, Summary: summary}, nil
}

func (s *Store) alertFindings(ctx context.Context, policy AlertPolicy) ([]alertFinding, map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT host,auth_status,COALESCE(mapped_balance,CAST(balance AS TEXT)),metadata_json FROM upstreams ORDER BY host`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	findings := []alertFinding{}
	for rows.Next() {
		var host, authStatus, metadataRaw string
		var balance sql.NullString
		if err := rows.Scan(&host, &authStatus, &balance, &metadataRaw); err != nil {
			return nil, nil, err
		}
		metadata, metadataErr := decodeObject(metadataRaw)
		if metadataErr != nil {
			metadata = map[string]any{}
			if policy.ConfigurationEnabled {
				findings = append(findings, alertFinding{"console:configuration:upstream:" + host, "upstream.configuration", "host", host, "CONFIG_METADATA_INVALID"})
			}
		}
		authText := authStatus
		if strings.TrimSpace(authText) == "" {
			authText = firstMetadataText(metadata, "auth_status", "auth_error")
		}
		normalizedAuth := strings.ToLower(strings.TrimSpace(authText))
		authFailure := containsAny(normalizedAuth, "失效", "失败", "未鉴权", "过期", "unauthorized", "expired", "invalid")
		authHealthy := valueIn(normalizedAuth, "已鉴权", "已恢复", "已认证", "authenticated", "authorized", "healthy", "valid", "ok", "succeeded")
		if policy.ConfigurationEnabled && normalizedAuth == "" {
			findings = append(findings, alertFinding{"console:configuration:upstream-auth:" + host, "upstream.configuration", "host", host, "CONFIG_AUTH_STATUS_MISSING"})
		} else if policy.ConfigurationEnabled && !authHealthy && !authFailure {
			findings = append(findings, alertFinding{"console:configuration:upstream-auth:" + host, "upstream.configuration", "host", host, alertCause("CONFIG_AUTH_STATUS_UNKNOWN", authText)})
		} else if policy.AuthEnabled && authFailure {
			findings = append(findings, alertFinding{"console:auth:" + host, "upstream.auth", "host", host, alertCause("AUTH", authText)})
		}
		if policy.RateSyncEnabled && stringValue(metadata["rate_sync_status"]) == "failed" {
			cause := "RATE_SYNC"
			if reason := strings.TrimSpace(stringValue(metadata["rate_sync_error"])); reason != "" {
				cause += ":" + truncateText(reason, 300)
			}
			findings = append(findings, alertFinding{"console:rate-sync:" + host, "upstream.rate_sync", "host", host, cause})
		}
		if rawClosed, present := metadata["balance_hard_closed"]; present {
			closed := strictAnyBool(rawClosed)
			if policy.ConfigurationEnabled && closed == nil {
				findings = append(findings, alertFinding{"console:configuration:upstream-balance-closed:" + host, "upstream.configuration", "host", host, "CONFIG_BALANCE_CLOSED_INVALID"})
			} else if policy.BalanceEnabled && closed != nil && *closed {
				findings = append(findings, alertFinding{"console:balance:" + host + ":hard-closed", "upstream.balance", "host", host, "BALANCE_HARD_CLOSED"})
				continue
			}
		}
		if balance.Valid && (policy.ConfigurationEnabled || policy.BalanceEnabled) {
			value, ok := new(big.Rat).SetString(strings.TrimSpace(balance.String))
			if !ok {
				if policy.ConfigurationEnabled {
					findings = append(findings, alertFinding{"console:configuration:upstream-balance:" + host, "upstream.configuration", "host", host, alertCause("CONFIG_BALANCE_INVALID", balance.String)})
				}
			} else if policy.BalanceEnabled {
				if threshold := triggeredBalanceThreshold(value, policy.BalanceThresholds); threshold != "" {
					findings = append(findings, alertFinding{"console:balance:" + host + ":" + threshold, "upstream.balance", "host", host, "BALANCE:" + threshold})
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	routingFindings, routingNotEvaluated, err := s.routingAlertFindings(ctx, policy)
	if err != nil {
		return nil, nil, err
	}
	findings = append(findings, routingFindings...)
	notEvaluated := routingNotEvaluated
	if policy.ProbeEnabled {
		probeFindings, probeNotEvaluated, err := s.probeFailureFindings(ctx, policy)
		if err != nil {
			return nil, nil, err
		}
		findings = append(findings, probeFindings...)
		for key, detail := range probeNotEvaluated {
			notEvaluated[key] = detail
		}
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].key < findings[j].key })
	return findings, notEvaluated, nil
}

type routingAlertGroup struct {
	total, schedulable int
	hasSurvivor        bool
}

func (s *Store) routingAlertFindings(ctx context.Context, policy AlertPolicy) ([]alertFinding, map[string]string, error) {
	findings := []alertFinding{}
	groups := map[string]*routingAlertGroup{}
	epoch, err := s.routingDecisionEpoch(ctx)
	if err != nil {
		return nil, nil, err
	}
	query := `WITH ranked AS (
		SELECT rd.*,ROW_NUMBER() OVER(PARTITION BY rd.account_id ORDER BY julianday(rd.updated_at) DESC,rd.group_name) AS decision_rank
		FROM routing_decisions rd`
	arguments := []any{}
	if epoch != nil {
		query += ` WHERE julianday(rd.updated_at)>=julianday(?)`
		arguments = append(arguments, *epoch)
	}
	query += `) SELECT rd.account_id,rd.group_name,COALESCE(ag.group_name,rd.group_name),
		COALESCE(NULLIF(TRIM(rd.routing_state),''),NULLIF(TRIM(rd.role),''),''),COALESCE(rd.reason,''),rd.schedulable
		FROM ranked rd LEFT JOIN account_groups ag ON ag.account_id=rd.account_id
		WHERE rd.decision_rank=1 ORDER BY rd.account_id,ag.group_name`
	rows, err := s.db.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	accountFindings := map[string]struct{}{}
	for rows.Next() {
		var accountID, primaryGroupName, groupName, state, reason string
		var schedulable sql.NullInt64
		if err := rows.Scan(&accountID, &primaryGroupName, &groupName, &state, &reason, &schedulable); err != nil {
			return nil, nil, err
		}
		state = strings.ToLower(strings.TrimSpace(state))
		group := groups[groupName]
		if group == nil {
			group = &routingAlertGroup{}
			groups[groupName] = group
		}
		group.total++
		if schedulable.Valid && schedulable.Int64 == 1 {
			group.schedulable++
		}
		group.hasSurvivor = group.hasSurvivor || state == "survivor"
		if _, found := accountFindings[accountID]; found {
			continue
		}
		switch {
		case policy.RoutingBreakerEnabled && fusedRoutingDecisionState(state):
			findings = append(findings, alertFinding{
				"console:routing:breaker:" + accountID + ":" + primaryGroupName,
				"account.routing_breaker", "account", accountID, alertCause("ROUTING_BREAKER", reason),
			})
			accountFindings[accountID] = struct{}{}
		case policy.RoutingDegradedEnabled && state == "degraded":
			findings = append(findings, alertFinding{
				"console:routing:degraded:" + accountID + ":" + primaryGroupName,
				"account.routing_degraded", "account", accountID, alertCause("ROUTING_DEGRADED", reason),
			})
			accountFindings[accountID] = struct{}{}
		case policy.RoutingSurvivorEnabled && state == "survivor":
			findings = append(findings, alertFinding{
				"console:routing:survivor:" + accountID + ":" + primaryGroupName,
				"account.routing_survivor", "account", accountID, alertCause("ROUTING_SURVIVOR", reason),
			})
			accountFindings[accountID] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, nil, err
	}
	for groupName, group := range groups {
		if policy.GroupUnavailableEnabled && group.total > 0 && group.schedulable == 0 {
			findings = append(findings, alertFinding{
				"console:routing:group-unavailable:" + groupName,
				"group.routing_unavailable", "group", groupName, "GROUP_UNAVAILABLE",
			})
		} else if policy.GroupSurvivorEnabled && group.hasSurvivor && group.schedulable <= 1 {
			findings = append(findings, alertFinding{
				"console:routing:group-survivor:" + groupName,
				"group.routing_survivor", "group", groupName, "GROUP_SURVIVOR_ONLY",
			})
		}
	}
	if policy.ApplyFailureEnabled {
		applyFindings, err := s.routingApplyFailureFindings(ctx)
		if err != nil {
			return nil, nil, err
		}
		findings = append(findings, applyFindings...)
	}
	notEvaluated, err := s.invalidatedRoutingAlertIncidents(ctx, epoch)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	return findings, notEvaluated, nil
}

func (s *Store) routingDecisionEpoch(ctx context.Context) (*string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT updated_at FROM app_state WHERE key='routing-decision-epoch'`).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func (s *Store) invalidatedRoutingAlertIncidents(ctx context.Context, epoch *string) (map[string]string, error) {
	result := map[string]string{}
	if epoch == nil {
		return result, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT incident_key FROM alert_incidents WHERE status IN ('firing','suppressed')
		AND event_type IN ('account.routing_breaker','account.routing_degraded','account.routing_survivor',
		'group.routing_unavailable','group.routing_survivor') AND julianday(last_seen_at)<julianday(?)`, *epoch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var incidentKey string
		if err := rows.Scan(&incidentKey); err != nil {
			return nil, err
		}
		result[incidentKey] = "运行模式已切换，旧调度判定已失效"
	}
	return result, rows.Err()
}

func (s *Store) routingApplyFailureFindings(ctx context.Context) ([]alertFinding, error) {
	rows, err := s.db.QueryContext(ctx, `WITH ranked AS (
		SELECT object_id,error,state,ROW_NUMBER() OVER(PARTITION BY object_id ORDER BY created_at DESC,
		CASE WHEN source_id < 0 THEN 0 ELSE 1 END,CASE WHEN source_id < 0 THEN source_id END ASC,
		CASE WHEN source_id >= 0 THEN source_id END DESC) AS position
		FROM operation_audit WHERE operation_type IN ('routing.writeback','cleanup.delete') AND object_id IS NOT NULL
		AND (state='failed' OR readback_confirmed=1)
	) SELECT object_id,COALESCE(error,'') FROM ranked WHERE position=1 AND state='failed' ORDER BY object_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	findings := []alertFinding{}
	for rows.Next() {
		var accountID, reason string
		if err := rows.Scan(&accountID, &reason); err != nil {
			return nil, err
		}
		findings = append(findings, alertFinding{
			"console:routing:apply:" + accountID, "routing.apply_failure", "account", accountID,
			alertCause("APPLY_FAILED", reason),
		})
	}
	return findings, rows.Err()
}

func fusedRoutingDecisionState(state string) bool {
	return valueIn(state, "fused", "hard_open", "soft_open")
}

func alertCause(code, detail string) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return code
	}
	return code + ":" + truncateText(detail, 300)
}

func truncateText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func (s *Store) probeFailureFindings(ctx context.Context, policy AlertPolicy) ([]alertFinding, map[string]string, error) {
	selected := map[string]struct{}{}
	for _, group := range policy.ProbeGroups {
		selected[strings.ToLower(strings.TrimSpace(group))] = struct{}{}
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT account_id,group_name FROM health_samples
		WHERE LOWER(REPLACE(source,'-','_')) IN ('active_probe','probe') ORDER BY account_id,group_name`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	type probeKey struct {
		accountID, groupName, incidentKey string
	}
	keys := []probeKey{}
	seen := map[string]struct{}{}
	for rows.Next() {
		var accountID, groupName string
		if err := rows.Scan(&accountID, &groupName); err != nil {
			return nil, nil, err
		}
		incidentKey := "console:probe:" + accountID + ":" + groupName
		keys = append(keys, probeKey{accountID: accountID, groupName: groupName, incidentKey: incidentKey})
		seen[incidentKey] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, nil, err
	}
	activeRows, err := s.db.QueryContext(ctx, `SELECT incident_key,object_id FROM alert_incidents
		WHERE event_type='account.probe' AND status IN ('firing','suppressed') ORDER BY incident_key`)
	if err != nil {
		return nil, nil, err
	}
	defer activeRows.Close()
	for activeRows.Next() {
		var incidentKey, accountID string
		if err := activeRows.Scan(&incidentKey, &accountID); err != nil {
			return nil, nil, err
		}
		if _, found := seen[incidentKey]; found {
			continue
		}
		prefix := "console:probe:" + accountID + ":"
		if !strings.HasPrefix(incidentKey, prefix) {
			continue
		}
		keys = append(keys, probeKey{accountID: accountID, groupName: strings.TrimPrefix(incidentKey, prefix), incidentKey: incidentKey})
		seen[incidentKey] = struct{}{}
	}
	if err := activeRows.Err(); err != nil {
		return nil, nil, err
	}
	if err := activeRows.Close(); err != nil {
		return nil, nil, err
	}
	maxAge, err := s.probeAlertEvidenceMaxAge(ctx)
	if err != nil {
		return nil, nil, err
	}
	cutoff := time.Now().UTC().Add(-maxAge)
	findings := []alertFinding{}
	notEvaluated := map[string]string{}
	for _, key := range keys {
		if len(selected) > 0 {
			if _, allowed := selected[strings.ToLower(strings.TrimSpace(key.groupName))]; !allowed {
				notEvaluated[key.incidentKey] = "主动探测分组已移出告警范围"
				continue
			}
		}
		count, failed, latestReason, err := s.probeFailureEvidence(ctx, key.accountID, key.groupName, policy.ProbeFailureStreak, cutoff)
		if err != nil {
			return nil, nil, err
		}
		if count >= policy.ProbeFailureStreak && failed {
			findings = append(findings, alertFinding{key.incidentKey, "account.probe", "account", key.accountID, alertCause("PROBE", latestReason)})
		} else if count == 0 || failed {
			notEvaluated[key.incidentKey] = "主动探测证据不足或已过期"
		}
	}
	return findings, notEvaluated, nil
}

func (s *Store) probeFailureEvidence(
	ctx context.Context,
	accountID string,
	groupName string,
	limit int,
	cutoff time.Time,
) (int, bool, string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT result,failure_reason,observed_at FROM health_samples
		WHERE account_id=? AND group_name=? AND LOWER(REPLACE(source,'-','_')) IN ('active_probe','probe')
		AND LOWER(TRIM(result)) IN ('通过','passed','pass','success','succeeded','healthy','ok',
			'失败','failed','error','timeout','超时','probe failed','unhealthy','管理 api 异常')
		ORDER BY observed_at DESC,id DESC LIMIT ?`, accountID, groupName, limit)
	if err != nil {
		return 0, false, "", err
	}
	defer rows.Close()
	count, failed, latestReason := 0, true, ""
	for rows.Next() {
		var result, failureReason, observedAt sql.NullString
		if err := rows.Scan(&result, &failureReason, &observedAt); err != nil {
			return 0, false, "", err
		}
		observed, parseErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(observedAt.String))
		if parseErr != nil || observed.Before(cutoff) {
			continue
		}
		count++
		if valueIn(strings.ToLower(strings.TrimSpace(result.String)), "通过", "passed", "pass", "success", "succeeded", "healthy", "ok") {
			failed = false
		} else if latestReason == "" && failureReason.Valid {
			latestReason = strings.TrimSpace(failureReason.String)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, false, "", err
	}
	return count, failed, latestReason, nil
}

func (s *Store) probeAlertEvidenceMaxAge(ctx context.Context) (time.Duration, error) {
	policy, err := s.readPolicyDocument(ctx, s.db, "control-plane")
	if err != nil {
		return 0, err
	}
	maximumSeconds := 300
	for _, sectionName := range []string{"probe", "recovery"} {
		section, _ := policy[sectionName].(map[string]any)
		field := "interval_seconds"
		if sectionName == "recovery" {
			field = "probe_interval_seconds"
		}
		if raw, present := section[field]; present {
			if value, parseErr := strictInteger(raw); parseErr == nil && value > maximumSeconds {
				maximumSeconds = value
			}
		}
	}
	if bindings, ok := policy["group_policy_bindings"].(map[string]any); ok {
		for _, rawBinding := range bindings {
			binding, _ := rawBinding.(map[string]any)
			if raw, present := binding["probe_interval_seconds"]; present {
				if value, parseErr := strictInteger(raw); parseErr == nil && value > maximumSeconds {
					maximumSeconds = value
				}
			}
		}
	}
	return max(10*time.Minute, 2*time.Duration(maximumSeconds)*time.Second), nil
}

func triggeredBalanceThreshold(balance *big.Rat, thresholds []string) string {
	var triggered *big.Rat
	for _, text := range thresholds {
		threshold, ok := new(big.Rat).SetString(text)
		if ok && balance.Cmp(threshold) <= 0 && (triggered == nil || threshold.Cmp(triggered) < 0) {
			triggered = threshold
		}
	}
	if triggered == nil {
		return ""
	}
	return decimalRatText(triggered)
}

func firstMetadataText(metadata map[string]any, fields ...string) string {
	for _, field := range fields {
		if value, present := metadata[field]; present {
			if value == nil {
				return ""
			}
			return fmt.Sprint(value)
		}
	}
	return ""
}

func containsAny(value string, markers ...string) bool {
	for _, marker := range markers {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func valueIn(value string, options ...string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}

func randomRunKey(prefix string) (string, error) {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(buffer), nil
}
