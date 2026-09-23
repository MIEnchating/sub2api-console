package newapimanagement

import (
	"context"
	"encoding/hex"
	"net/http"
	"strconv"
)

func (s *Service) AvailableChannelModels(ctx context.Context, platformID, channelID, version string) ([]string, error) {
	id, err := strconv.ParseInt(channelID, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != channelID {
		return nil, serviceError(ErrorValidation, "渠道 ID 无效，请重新选择渠道")
	}
	if decoded, err := hex.DecodeString(version); err != nil || len(decoded) != 32 {
		return nil, serviceError(ErrorValidation, "渠道版本无效，请刷新列表后重试")
	}
	platform, err := s.requirePlatform(ctx, platformID)
	if err != nil {
		return nil, err
	}
	current, err := s.readChannel(ctx, *platform, channelID)
	if err != nil {
		return nil, serviceError(ErrorUpstream, "渠道读取失败，请刷新列表后重试")
	}
	if current.Version != version {
		return nil, serviceError(ErrorConflict, "渠道配置已变化，请刷新列表后重新获取模型")
	}
	// New API resolves the channel's credentials and adapter without exposing them to Console.
	payload, err := s.request(ctx, *platform, http.MethodGet, "/api/channel/fetch_models/"+channelID, nil)
	if err != nil {
		return nil, serviceError(ErrorUpstream, "获取渠道上游模型失败，请检查 New API 渠道连接、密钥和模型接口后重试")
	}
	rows, ok := payload.([]any)
	if !ok || len(rows) > 10000 {
		return nil, serviceError(ErrorUpstream, "渠道模型列表格式无效，请检查 New API 模型接口后重试")
	}
	models := make([]string, 0, len(rows))
	for _, row := range rows {
		model, ok := row.(string)
		if !ok || !validChannelModelName(model) {
			return nil, serviceError(ErrorUpstream, "渠道模型列表包含无效名称，请检查上游响应后重试")
		}
		models = append(models, model)
	}
	return normalizeModels(models), nil
}
