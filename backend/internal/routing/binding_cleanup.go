package routing

import (
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

type bindingCleanupConfig struct {
	enabled     bool
	maxPerRound int
}

func parseBindingCleanup(policy map[string]any) (bindingCleanupConfig, error) {
	section := map[string]any{}
	if raw, present := policy["binding_cleanup"]; present {
		var ok bool
		section, ok = raw.(map[string]any)
		if !ok {
			return bindingCleanupConfig{}, errors.New("binding_cleanup 必须是对象")
		}
	}
	reader := policyReader{}
	result := bindingCleanupConfig{
		enabled:     reader.boolean(section, "binding_cleanup.enabled", "enabled", false),
		maxPerRound: reader.integer(section, "binding_cleanup.max_per_round", "max_per_round", 1, 1, 10000),
	}
	return result, reader.err
}

func applyBindingCleanup(byAccount map[string][]*candidate, config engineConfig) []business.RuntimeEventWrite {
	events := []business.RuntimeEventWrite{}
	if !config.bindingCleanup.enabled {
		return events
	}
	done := 0
	for _, id := range sortedTargetIDsFromCandidates(byAccount) {
		items := byAccount[id]
		if !missingBindingCleanupEligible(items, config) {
			continue
		}
		if done >= config.bindingCleanup.maxPerRound {
			events = append(events, cleanupRuntimeEvent("cleanup_deferred", "succeeded", id, "失效绑定账号删除达到本轮数量上限，下轮重新评估", items))
			continue
		}
		for _, item := range items {
			action := "delete"
			item.cleanupAction = &action
			item.cleanupReason = business.MissingBindingCleanupReason
		}
		done++
		events = append(events, cleanupRuntimeEvent("cleanup_queued", "succeeded", id, "上游 Key 或分组已确认不存在，进入账号自动删除队列", items))
	}
	return events
}

func missingBindingCleanupEligible(items []*candidate, config engineConfig) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if _, manual := config.manualFusedAccounts[item.account.ID]; manual {
			return false
		}
		if item.account.Paused || item.account.ManualPriority != nil || item.account.UpstreamExcluded ||
			item.state != "binding_invalid" || item.account.MissingBindingFingerprint == "" || item.cleanupAction != nil {
			return false
		}
	}
	return true
}
