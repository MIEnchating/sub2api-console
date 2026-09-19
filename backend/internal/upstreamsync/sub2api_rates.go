package upstreamsync

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/decimalutil"
)

// Sub2API exposes user-specific group rates separately from the group catalog.
// They replace the group multiplier; they are not an additional discount factor.
func (r *Reader) sub2APIUserGroupRates(ctx context.Context, record configstore.AuthRecord) (map[string]string, error) {
	if !strings.EqualFold(strings.TrimSpace(record.UpstreamType), "sub2api") {
		return nil, nil
	}
	payload, status, err := r.request(ctx, record, "/api/v1/groups/rates", nil, true)
	if err != nil {
		// Older versions do not expose this endpoint. Other failures must not
		// overwrite a previously known exclusive rate with the base multiplier.
		if status == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("Sub2API 专属倍率读取失败：%w", err)
	}
	envelope, ok := payload.(map[string]any)
	if !ok {
		return nil, errors.New("Sub2API 专属倍率返回格式无效")
	}
	raw, present := envelope["data"]
	if !present {
		return nil, errors.New("Sub2API 专属倍率返回缺少 data")
	}
	if raw == nil {
		return nil, nil
	}
	values, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("Sub2API 专属倍率必须按分组 ID 返回")
	}
	rates := make(map[string]string, len(values))
	for groupID, value := range values {
		id, err := strconv.ParseInt(groupID, 10, 64)
		if err != nil || id <= 0 || strconv.FormatInt(id, 10) != groupID {
			return nil, errors.New("Sub2API 专属倍率包含无效分组 ID")
		}
		rate, ok := decimalutil.Parse(textValue(value))
		if !ok || rate.Sign() < 0 {
			return nil, fmt.Errorf("Sub2API 分组 %s 专属倍率必须为非负十进制数", groupID)
		}
		rates[groupID] = ratText(rate)
	}
	return rates, nil
}
