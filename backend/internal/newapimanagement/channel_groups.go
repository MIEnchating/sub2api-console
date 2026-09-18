package newapimanagement

import (
	"context"
	"strconv"
	"sync"
	"time"
)

// ChannelsByID reads saved group members independently of the upstream list page.
func (s *Service) ChannelsByID(ctx context.Context, platformID string, ids []string, page, pageSize int) (ChannelPage, error) {
	result := ChannelPage{Items: []Channel{}, Total: len(ids)}
	if page < 0 || page > 999 || pageSize < 1 || pageSize > 100 || len(ids) > 1000 {
		return ChannelPage{}, serviceError(ErrorValidation, "渠道分组分页无效")
	}
	for _, id := range ids {
		value, err := strconv.ParseInt(id, 10, 64)
		if err != nil || value <= 0 || strconv.FormatInt(value, 10) != id {
			return ChannelPage{}, serviceError(ErrorValidation, "渠道 ID 无效")
		}
	}
	platform, err := s.requirePlatform(ctx, platformID)
	if err != nil {
		return ChannelPage{}, err
	}
	start := page * pageSize
	if start >= len(ids) {
		return result, nil
	}
	ids = ids[start:min(start+pageSize, len(ids))]
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	result.Items = make([]Channel, len(ids))
	errors := make([]error, len(ids))
	var workers sync.WaitGroup
	for worker := 0; worker < min(4, len(ids)); worker++ {
		workers.Go(func() {
			for i := worker; i < len(ids); i += 4 {
				result.Items[i], errors[i] = s.readChannel(ctx, *platform, ids[i])
			}
		})
	}
	workers.Wait()
	for _, err := range errors {
		if err != nil {
			return ChannelPage{}, serviceError(ErrorUpstream, "分组渠道读取失败，请重试；已删除的渠道可在渠道分组中移除")
		}
	}
	return result, nil
}
