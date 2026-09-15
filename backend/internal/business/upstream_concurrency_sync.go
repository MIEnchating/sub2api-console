package business

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

const (
	UpstreamConcurrencyKnown     = "known"
	UpstreamConcurrencyUnlimited = "unlimited"
	UpstreamConcurrencyUnknown   = "unknown"
	UpstreamConcurrencyStale     = "stale"
)

func normalizeUpstreamConcurrencyObservation(value *UpstreamBalanceObservation) error {
	value.ProfileUserID = strings.TrimSpace(value.ProfileUserID)
	if value.ProfileUserID != "" {
		id, err := strconv.ParseInt(value.ProfileUserID, 10, 64)
		if err != nil || id <= 0 {
			return errors.New("上游并发信息缺少有效用户 ID，请重新同步")
		}
		value.ProfileUserID = strconv.FormatInt(id, 10)
	}
	if value.ConcurrencyLimit == nil {
		if value.ConcurrencyStatus != "" && value.ConcurrencyStatus != UpstreamConcurrencyUnknown {
			return errors.New("上游并发信息缺少可验证的上限，请重新同步")
		}
		return nil
	}
	if *value.ConcurrencyLimit < 0 {
		return errors.New("上游用户并发上限必须是非负整数，请重新同步")
	}
	value.ConcurrencyStatus = UpstreamConcurrencyKnown
	if *value.ConcurrencyLimit == 0 {
		value.ConcurrencyStatus = UpstreamConcurrencyUnlimited
	}
	return nil
}

func applyUpstreamConcurrencyObservation(metadata map[string]any, observation *UpstreamBalanceObservation, now string) {
	if observation.ConcurrencyStatus == "" && observation.ConcurrencyLimit == nil {
		return
	}
	previousUser := stringValue(metadata["concurrency_user_id"])
	if observation.ProfileUserID != "" {
		if previousUser != "" && previousUser != observation.ProfileUserID {
			delete(metadata, "concurrency_limit")
			delete(metadata, "concurrency_checked_at")
		}
		metadata["concurrency_user_id"] = observation.ProfileUserID
	}
	if observation.ConcurrencyLimit == nil {
		markUpstreamConcurrencyStale(metadata, "上游未返回用户并发上限，请重新同步确认")
		return
	}
	metadata["concurrency_limit"] = *observation.ConcurrencyLimit
	metadata["concurrency_status"] = observation.ConcurrencyStatus
	metadata["concurrency_checked_at"] = now
	delete(metadata, "concurrency_error")
}

func markUpstreamConcurrencyStale(metadata map[string]any, reason string) {
	limit, _ := readUpstreamConcurrencyMetadata(metadata)
	metadata["concurrency_status"] = UpstreamConcurrencyUnknown
	if limit != nil {
		metadata["concurrency_status"] = UpstreamConcurrencyStale
	}
	metadata["concurrency_error"] = reason
}

func readUpstreamConcurrencyMetadata(metadata map[string]any) (*int64, string) {
	status := stringValue(metadata["concurrency_status"])
	if status == "" {
		status = UpstreamConcurrencyUnknown
	}
	var text string
	switch value := metadata["concurrency_limit"].(type) {
	case int64:
		text = strconv.FormatInt(value, 10)
	case json.Number:
		text = value.String()
	default:
		return nil, status
	}
	limit, err := strconv.ParseInt(text, 10, 64)
	if err != nil || limit < 0 {
		return nil, UpstreamConcurrencyUnknown
	}
	return &limit, status
}
