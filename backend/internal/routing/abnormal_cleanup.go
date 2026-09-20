package routing

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

type abnormalCleanupConfig struct {
	enabled     bool
	action      string
	duration    time.Duration
	maxPerRound int
	keepLast    bool
}

func parseAbnormalCleanup(policy map[string]any) (abnormalCleanupConfig, error) {
	section := map[string]any{}
	if raw, present := policy["abnormal_cleanup"]; present {
		var ok bool
		section, ok = raw.(map[string]any)
		if !ok {
			return abnormalCleanupConfig{}, errors.New("abnormal_cleanup 必须是对象")
		}
	}
	action := "pause"
	if raw, present := section["action"]; present {
		var ok bool
		action, ok = raw.(string)
		if !ok || (action != "pause" && action != "disable" && action != "delete") {
			return abnormalCleanupConfig{}, errors.New("abnormal_cleanup.action 配置无效")
		}
	}
	reader := policyReader{}
	result := abnormalCleanupConfig{
		enabled: reader.boolean(section, "abnormal_cleanup.enabled", "enabled", false), action: action,
		duration:    time.Duration(reader.integer(section, "abnormal_cleanup.duration_minutes", "duration_minutes", 1440, 1, 525600)) * time.Minute,
		maxPerRound: reader.integer(section, "abnormal_cleanup.max_per_round", "max_per_round", 1, 1, 10000),
		keepLast:    reader.boolean(section, "abnormal_cleanup.keep_last_in_group", "keep_last_in_group", true),
	}
	return result, reader.err
}

func applyAbnormalCleanup(byAccount map[string][]*candidate, config engineConfig, states map[string]time.Time, now time.Time) ([]business.CleanupStateWrite, []business.RuntimeEventWrite) {
	settings := config.abnormalCleanup
	writes := []business.CleanupStateWrite{}
	events := []business.RuntimeEventWrite{}
	members := map[string]map[string]struct{}{}
	selected := map[string]struct{}{}
	for id, items := range byAccount {
		for _, item := range items {
			if item.state == "excluded" {
				continue
			}
			if members[item.account.GroupName] == nil {
				members[item.account.GroupName] = map[string]struct{}{}
			}
			members[item.account.GroupName][id] = struct{}{}
			if item.cleanupAction != nil {
				selected[id] = struct{}{}
			}
		}
	}
	done := 0
	for _, id := range sortedTargetIDsFromCandidates(byAccount) {
		items := byAccount[id]
		since, observed := states[id]
		if !settings.enabled || !persistentAbnormal(items, config, since, now) {
			if observed {
				writes = append(writes, business.CleanupStateWrite{AccountID: id, PersistentAbnormal: true})
			}
			continue
		}
		if !observed || since.After(now) {
			since = now.UTC()
			writes = append(writes, business.CleanupStateWrite{AccountID: id, PersistentAbnormal: true, EligibleSince: &since})
		}
		if now.Sub(since) < settings.duration {
			continue
		}
		if _, queued := selected[id]; queued {
			continue
		}
		if done >= settings.maxPerRound {
			events = append(events, cleanupRuntimeEvent("cleanup_deferred", "succeeded", id, "长期异常处置达到本轮数量上限，下轮重新评估", items))
			continue
		}
		if settings.keepLast && cleanupWouldEmptyGroup(id, items, members, selected) {
			events = append(events, cleanupRuntimeEvent("cleanup_skipped", "succeeded", id, "长期异常账号是分组内最后一个可保留账号，已跳过处置", items))
			continue
		}
		for _, item := range items {
			action := settings.action
			item.cleanupAction = &action
			item.cleanupReason = "长期异常"
		}
		selected[id] = struct{}{}
		done++
		events = append(events, cleanupRuntimeEvent("cleanup_queued", "succeeded", id,
			fmt.Sprintf("长期异常已持续至少 %d 分钟，进入自动处置队列：%s", int(settings.duration.Minutes()), cleanupActionLabel(settings.action)), items))
	}
	return writes, events
}

func persistentAbnormal(items []*candidate, config engineConfig, since, now time.Time) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if _, manual := config.manualFusedAccounts[item.account.ID]; manual {
			return false
		}
		if item.account.Paused || item.account.ManualPriority != nil {
			return false
		}
		if (item.state != "fused" && item.state != "degraded") || item.evidencePending || candidateRateLimited(item, now) || len(item.rows) == 0 {
			return false
		}
		// Only current health evidence qualifies. A fresh successful request breaks the
		// failure streak even while recovery is still waiting for more successes.
		if !since.IsZero() {
			for _, row := range item.rows {
				at, err := time.Parse(time.RFC3339Nano, row.ObservedAt)
				if err != nil || at.Before(since) {
					continue
				}
				classification := classify(Sample{Result: row.Result, FailureReason: row.FailureReason, Source: row.Source, LatencyP95: row.LatencyP95, StatusCode: routingSampleStatus(row), Payload: row.Payload}, config.sampleClassification)
				if !classification.Failure || classification.Neutral || classification.RateLimited {
					return false
				}
			}
		}
		row := item.rows[0]
		observed, err := time.Parse(time.RFC3339Nano, row.ObservedAt)
		maxAge := config.trafficMaxAge
		if strings.ReplaceAll(strings.ToLower(strings.TrimSpace(row.Source)), "_", "-") == "active-probe" {
			maxAge = config.probeMaxAge
		}
		if err != nil || observed.After(now) || now.Sub(observed) > maxAge {
			return false
		}
		classified := classify(Sample{Result: row.Result, FailureReason: row.FailureReason, Source: row.Source, LatencyP95: row.LatencyP95, StatusCode: routingSampleStatus(row), Payload: row.Payload}, config.sampleClassification)
		if classified.Neutral || classified.RateLimited || !classified.Failure {
			return false
		}
	}
	return true
}
