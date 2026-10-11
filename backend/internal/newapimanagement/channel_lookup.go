package newapimanagement

import (
	"context"
	"strconv"
	"sync"
	"time"
)

type ChannelLookupFailure struct {
	ID      string `json:"id"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ChannelLookupResult struct {
	Items    []Channel              `json:"items"`
	Failures []ChannelLookupFailure `json:"failures"`
}

// LookupChannels restores a bounded selection without scanning the directory.
// An unavailable member remains explicit; callers must not silently drop it.
func (s *Service) LookupChannels(ctx context.Context, platformID string, ids []string) (ChannelLookupResult, error) {
	result := ChannelLookupResult{Items: []Channel{}, Failures: []ChannelLookupFailure{}}
	if len(ids) == 0 || len(ids) > 50 {
		return result, serviceError(ErrorValidation, "请选择 1 到 50 个渠道")
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		value, err := strconv.ParseInt(id, 10, 64)
		if err != nil || value <= 0 || strconv.FormatInt(value, 10) != id || seen[id] {
			return result, serviceError(ErrorValidation, "渠道必须使用不重复的有效 ID")
		}
		seen[id] = true
	}
	platform, err := s.requirePlatform(ctx, platformID)
	if err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	rows := make([]Channel, len(ids))
	failures := make([]error, len(ids))
	var workers sync.WaitGroup
	for worker := 0; worker < min(4, len(ids)); worker++ {
		workers.Go(func() {
			for i := worker; i < len(ids); i += 4 {
				if ctx.Err() != nil {
					return
				}
				rows[i], failures[i] = s.readChannel(ctx, *platform, ids[i])
			}
		})
	}
	workers.Wait()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	for i, id := range ids {
		if failures[i] == nil {
			result.Items = append(result.Items, rows[i])
			continue
		}
		failure := ChannelLookupFailure{ID: id, Code: "channel_unavailable", Message: "渠道读取失败，请重试；若渠道已删除，请手动移除"}
		if KindOf(failures[i]) == ErrorConflict {
			failure.Code = "channel_identity_changed"
			failure.Message = "返回的渠道身份不一致，请重新读取"
		}
		result.Failures = append(result.Failures, failure)
	}
	return result, nil
}
