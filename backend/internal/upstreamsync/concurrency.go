package upstreamsync

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func profileConcurrency(record configstore.AuthRecord, data map[string]any) (*int64, string, string, error) {
	if !strings.EqualFold(strings.TrimSpace(record.UpstreamType), "sub2api") {
		return nil, "", "", nil
	}
	var userID string
	if value, exists := data["id"]; exists && value != nil {
		id, err := profileInteger(value)
		if err != nil || id <= 0 {
			return nil, "", "", errors.New("上游用户信息的用户 ID 无效，请检查上游后重试同步")
		}
		userID = strconv.FormatInt(id, 10)
	}
	value, exists := data["concurrency"]
	if !exists || value == nil {
		return nil, business.UpstreamConcurrencyUnknown, userID, nil
	}
	limit, err := profileInteger(value)
	if err != nil || limit < 0 {
		return nil, "", "", errors.New("上游用户并发上限不是有效的非负整数，请检查上游后重试同步")
	}
	status := business.UpstreamConcurrencyKnown
	if limit == 0 {
		status = business.UpstreamConcurrencyUnlimited
	}
	return &limit, status, userID, nil
}

func profileInteger(value any) (int64, error) {
	var text string
	switch number := value.(type) {
	case json.Number:
		text = number.String()
	case string:
		text = strings.TrimSpace(number)
	default:
		return 0, errors.New("整数格式无效")
	}
	return strconv.ParseInt(text, 10, 64)
}
