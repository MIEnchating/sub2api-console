package notification

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func splitUpstreamNotificationGroups(groups []notificationGroup) []notificationGroup {
	result := make([]notificationGroup, 0, len(groups))
	for _, group := range groups {
		if !group.upstream || len(group.incidents) < 2 {
			result = append(result, group)
			continue
		}
		current := make([]business.AlertIncident, 0)
		for _, incident := range group.incidents {
			candidate := append(append([]business.AlertIncident{}, current...), incident)
			if len(current) > 0 && utf8.RuneCountInString(batchMessage(candidate)) >= batchLimit {
				result = append(result, notificationGroup{incidents: current, upstream: true})
				current = []business.AlertIncident{incident}
			} else {
				current = candidate
			}
		}
		result = append(result, notificationGroup{incidents: current, upstream: true})
	}
	return result
}

func upstreamNotificationMessage(title string, incidents []business.AlertIncident) string {
	fields := notificationIncidentFields(incidents[0])
	lines := []string{
		fmt.Sprintf("## Sub2API · %s", title),
		"",
		markdownTableValue(fields.object),
		"",
		"| 告警类型 | 原因 | 状态 | 时间（北京时间） |",
		"| --- | --- | --- | --- |",
	}
	for _, incident := range incidents {
		fields := notificationIncidentFields(incident)
		lines = append(lines, fmt.Sprintf("| %s | %s | %s | %s |",
			markdownTableValue(fields.event), markdownTableValue(fields.cause),
			fields.status, markdownTableValue(fields.observedAt),
		))
	}
	return strings.Join(lines, "\n")
}

func upstreamNotificationRows(incidents []business.AlertIncident) []string {
	rows := make([]string, 0, len(incidents))
	for index, incident := range incidents {
		fields := notificationIncidentFields(incident)
		object := ""
		if index == 0 {
			object = fields.object
		}
		rows = append(rows, fmt.Sprintf("| %s | %s | %s | %s | %s |",
			markdownTableValue(fields.event), markdownTableValue(object),
			markdownTableValue(fields.cause), fields.status, markdownTableValue(fields.observedAt),
		))
	}
	return rows
}
