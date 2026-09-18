package accountworkbench

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type MaintenanceCondition struct {
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

var statusText = regexp.MustCompile(`(?i)(?:http(?:/\d(?:\.\d)?)?(?:\s+error)?|(?:http[_ -]?)?status(?:[_ -]?code)?|(?:upstream\s+)?error)\s*[:=]?\s*([45]\d{2})\b|(?m)^\s*([45]\d{2})\s*:`)
var permanentSignal = regexp.MustCompile(`(?i)deactivat|account[_ -]?deleted|deleted account|permanent(?:ly)?[_ -]?(?:disabled|suspend)|account[_ -]?removed|\bbanned\b`)
var authSignal = regexp.MustCompile(`(?i)invalid[_ -]?grant|invalid[_ -]?token|refresh[_ -]?token[^\n]{0,60}(?:invalid|revok|expir|missing)|(?:invalid|revok|expir)[^\n]{0,60}refresh[_ -]?token|token[^\n]{0,40}(?:revok|expir)|expired[^\n]{0,20}token|authorization[^\n]{0,30}expired|unauthori[sz]ed`)
var transientSignal = regexp.MustCompile(`(?i)network|econn|enotfound|etimedout|timeout|timed out|socket|connection reset|connection refused|fetch failed|tls|temporary unavailable`)
var signalField = regexp.MustCompile(`(?i)status|code|message|error|reason|detail|health|state|remark|result|response|cause`)

// ClassifyMaintenanceSignal only examines error/status fields. Credential and
// profile contents cannot change a maintenance decision.
func ClassifyMaintenanceSignal(value map[string]any) MaintenanceCondition {
	parts := []string{}
	statuses := map[int]bool{}
	collectMaintenanceSignal(value, 0, &parts, statuses)
	message := strings.ToLower(strings.Join(parts, "\n"))
	for _, match := range statusText.FindAllStringSubmatch(message, -1) {
		for _, part := range match[1:] {
			if n, err := strconv.Atoi(part); err == nil {
				statuses[n] = true
			}
		}
	}
	if permanentSignal.MatchString(message) {
		return MaintenanceCondition{"permanent", "account_deactivated"}
	}
	if statuses[429] || strings.Contains(message, "rate_limit") || strings.Contains(message, "rate limit") || strings.Contains(message, "too many requests") {
		return MaintenanceCondition{"transient", "rate_limited"}
	}
	if statuses[401] || authSignal.MatchString(message) {
		return MaintenanceCondition{"auth", "authorization_expired"}
	}
	for status := range statuses {
		if status >= 500 && status <= 599 {
			return MaintenanceCondition{"transient", "upstream_unavailable"}
		}
	}
	if transientSignal.MatchString(message) {
		return MaintenanceCondition{"transient", "upstream_unavailable"}
	}
	if statuses[403] || strings.Contains(message, "forbidden") || strings.Contains(message, "permission denied") {
		return MaintenanceCondition{"manual", "permission_denied"}
	}
	if value["ok"] == false || value["success"] == false || text(value["status"]) == "error" || text(value["status"]) == "failed" {
		return MaintenanceCondition{"manual", "account_error_requires_review"}
	}
	for status := range statuses {
		if status >= 400 {
			return MaintenanceCondition{"manual", "account_error_requires_review"}
		}
	}
	return MaintenanceCondition{"healthy", "account_healthy"}
}

func collectMaintenanceSignal(value any, depth int, parts *[]string, statuses map[int]bool) {
	if depth > 4 {
		return
	}
	switch v := value.(type) {
	case map[string]any:
		for key, entry := range v {
			if !signalField.MatchString(key) {
				continue
			}
			lower := strings.ToLower(key)
			if strings.Contains(lower, "status") {
				if code, err := strconv.Atoi(text(entry)); err == nil && code >= 100 && code <= 599 {
					statuses[code] = true
				}
			}
			collectMaintenanceSignal(entry, depth+1, parts, statuses)
		}
	case []any:
		for _, entry := range v {
			collectMaintenanceSignal(entry, depth+1, parts, statuses)
		}
	case string:
		if len(v) > 4096 {
			v = v[:4096]
		}
		*parts = append(*parts, v)
	case json.Number:
		*parts = append(*parts, v.String())
	}
}

func maintenanceCondition(account map[string]any, now time.Time, interval int) MaintenanceCondition {
	condition := ClassifyMaintenanceSignal(account)
	if condition.Kind != "healthy" {
		return condition
	}
	for _, value := range []any{object(account["credentials"])["expires_at"], account["token_expires_at"]} {
		expires := time.Time{}
		if seconds, err := strconv.ParseInt(text(value), 10, 64); err == nil {
			if seconds > 1e12 {
				seconds /= 1000
			}
			expires = time.Unix(seconds, 0)
		} else {
			expires, _ = time.Parse(time.RFC3339, text(value))
		}
		if !expires.IsZero() && !expires.After(now.Add(time.Duration(interval)*time.Minute)) {
			return MaintenanceCondition{"auth", "token_near_expiry"}
		}
	}
	return condition
}

func maintenanceEligible(account map[string]any, groups []string) bool {
	if text(account["platform"]) != "openai" || text(account["type"]) != "oauth" {
		return false
	}
	for _, field := range []string{"disabled", "is_disabled", "intentionally_disabled"} {
		if account[field] == true {
			return false
		}
	}
	for _, field := range []string{"enabled", "is_enabled"} {
		if account[field] == false {
			return false
		}
	}
	switch strings.ToLower(text(account["status"])) {
	case "inactive", "disabled", "manually_disabled", "intentionally_disabled":
		return false
	}
	if len(groups) == 0 {
		return true
	}
	for _, group := range publicAccount(account).Groups {
		for _, id := range groups {
			if group.ID == id {
				return true
			}
		}
	}
	return false
}
