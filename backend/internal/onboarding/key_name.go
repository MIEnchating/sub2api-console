package onboarding

import (
	"strings"
	"unicode/utf8"
)

func newAPITokenNameLimit(upstreamType string) int {
	switch strings.ToLower(strings.TrimSpace(upstreamType)) {
	case "newapi", "oneapi":
		// New API validates len(token.Name), i.e. UTF-8 bytes, not runes.
		return 50
	default:
		return 0
	}
}

func upstreamOnboardingKeyMarker(operationID, groupName, upstreamType string) string {
	marker := onboardingKeyMarker(operationID, groupName)
	limit := newAPITokenNameLimit(upstreamType)
	if limit == 0 || len(marker) <= limit {
		return marker
	}
	// Keep the entire operation identity for collision resistance and recovery.
	// Current UUIDs use 36 bytes; legacy operation suffixes use 16.
	suffix := strings.TrimPrefix(strings.TrimSpace(operationID), legacyOnboardingKeyMarkerPrefix)
	budget := limit - len(suffix) - 1
	if budget <= 0 {
		return suffix
	}
	groupName = strings.TrimSpace(groupName)
	end := 0
	for _, char := range groupName {
		next := end + utf8.RuneLen(char)
		if next > budget {
			break
		}
		end = next
	}
	return strings.TrimSpace(groupName[:end]) + "-" + suffix
}
