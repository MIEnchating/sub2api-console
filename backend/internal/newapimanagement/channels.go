package newapimanagement

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type Channel struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Type    int      `json:"type"`
	Status  int      `json:"status"`
	Models  []string `json:"models"`
	Groups  []string `json:"groups"`
	Version string   `json:"version"`
}

type ChannelPage struct {
	Items []Channel `json:"items"`
	Total int       `json:"total"`
}

type ChannelModelChange struct {
	Action  string   `json:"action" binding:"required,oneof=add remove"`
	Models  []string `json:"models" binding:"required,min=1,max=1000,dive,required,max=255"`
	Version string   `json:"version" binding:"required,len=64"`
}

func (s *Service) Channels(ctx context.Context, platformID string, page, pageSize int) (ChannelPage, error) {
	result := ChannelPage{Items: []Channel{}}
	if page < 0 || page > 999 || pageSize < 1 || pageSize > 100 {
		return result, serviceError(ErrorValidation, "渠道页码或每页行数无效")
	}
	platform, err := s.requirePlatform(ctx, platformID)
	if err != nil {
		return result, err
	}
	payload, err := s.request(ctx, *platform, http.MethodGet, fmt.Sprintf("/api/channel/?p=%d&page_size=%d&id_sort=true", page, pageSize), nil)
	if err != nil {
		return result, err
	}
	rows, total, err := newAPIChannelRows(payload)
	if err != nil {
		return result, wrapServiceError(ErrorUpstream, err)
	}
	result.Total = total
	for _, row := range rows {
		channel, err := publicChannel(row, *platform)
		if err != nil {
			return ChannelPage{}, err
		}
		result.Items = append(result.Items, channel)
	}
	return result, nil
}

func publicChannel(row map[string]any, platform configstore.NewAPIPlatform) (Channel, error) {
	channel := Channel{ID: firstDecimal(row, "id"), Name: firstText(row, "name")}
	id, err := strconv.ParseInt(channel.ID, 10, 64)
	if err != nil || id <= 0 || channel.Name == "" {
		return Channel{}, serviceError(ErrorUpstream, "远端渠道缺少有效 ID 或名称")
	}
	channel.Type, err = strconv.Atoi(fmt.Sprint(row["type"]))
	if err != nil || channel.Type <= 0 {
		return Channel{}, serviceError(ErrorUpstream, "远端渠道类型无效")
	}
	channel.Status, err = strconv.Atoi(fmt.Sprint(row["status"]))
	if err != nil || channel.Status < 1 || channel.Status > 3 {
		return Channel{}, serviceError(ErrorUpstream, "远端渠道状态无效")
	}
	models, ok := row["models"].(string)
	if !ok {
		return Channel{}, serviceError(ErrorUpstream, "远端渠道模型列表无效")
	}
	groups, ok := row["group"].(string)
	if !ok {
		return Channel{}, serviceError(ErrorUpstream, "远端渠道分组无效")
	}
	channel.Models = normalizeModels(strings.Split(models, ","))
	channel.Groups = normalizeModels(strings.Split(groups, ","))
	raw, _ := json.Marshal(struct {
		Channel    Channel
		PlatformID string
		BaseURL    string
		UserID     string
		UpdatedAt  string
	}{channel, platform.ID, platform.BaseURL, platform.UserID, platform.UpdatedAt})
	digest := sha256.Sum256(raw)
	channel.Version = hex.EncodeToString(digest[:])
	return channel, nil
}

func (s *Service) readChannel(ctx context.Context, platform configstore.NewAPIPlatform, id string) (Channel, error) {
	payload, err := s.request(ctx, platform, http.MethodGet, "/api/channel/"+id, nil)
	if err != nil {
		return Channel{}, err
	}
	row, ok := payload.(map[string]any)
	if !ok {
		return Channel{}, serviceError(ErrorUpstream, "远端渠道详情无效")
	}
	channel, err := publicChannel(row, platform)
	if err == nil && channel.ID != id {
		return Channel{}, serviceError(ErrorConflict, "远端渠道 ID 不一致，请刷新后重试")
	}
	return channel, err
}

func (s *Service) ChangeChannelModels(ctx context.Context, platformID, channelID string, input ChannelModelChange) (Channel, error) {
	s.managementMu.Lock()
	defer s.managementMu.Unlock()
	if err := validateChannelModelChange(channelID, input); err != nil {
		return Channel{}, err
	}
	id, _ := strconv.ParseInt(channelID, 10, 64)
	platform, err := s.requirePlatform(ctx, platformID)
	if err != nil {
		return Channel{}, err
	}
	current, err := s.readChannel(ctx, *platform, channelID)
	if err != nil {
		return Channel{}, err
	}
	if current.Version != input.Version {
		return Channel{}, serviceError(ErrorConflict, "渠道配置已变化，请刷新列表后重新选择模型")
	}
	selected := normalizeModels(input.Models)
	models := append([]string{}, current.Models...)
	if input.Action == "add" {
		models = normalizeModels(append(models, selected...))
	} else {
		removing := map[string]bool{}
		for _, model := range selected {
			removing[model] = true
		}
		models = []string{}
		for _, model := range current.Models {
			if !removing[model] {
				models = append(models, model)
			}
		}
	}
	if strings.Join(models, ",") == strings.Join(current.Models, ",") {
		return current, nil
	}
	// New API's channel update skips empty string fields. Reject a complete removal
	// instead of claiming a successful update that leaves the original models active.
	if len(models) == 0 {
		return Channel{}, serviceError(ErrorValidation, "渠道至少保留一个模型；如需全部停止服务，请在 New API 停用该渠道")
	}
	_, err = s.request(ctx, *platform, http.MethodPut, "/api/channel/", map[string]any{"id": id, "models": strings.Join(models, ",")})
	if err != nil {
		return Channel{}, serviceError(ErrorUpstream, "模型变更未确认，请刷新渠道核对结果后再操作（不会自动重试）")
	}
	updated, err := s.readChannel(ctx, *platform, channelID)
	if err != nil {
		return Channel{}, serviceError(ErrorUpstream, "模型已提交但读取确认失败，请刷新渠道核对结果")
	}
	if strings.Join(updated.Models, ",") != strings.Join(models, ",") {
		return Channel{}, serviceError(ErrorConflict, "模型已提交但远端结果不一致，请刷新渠道核对结果")
	}
	return updated, nil
}

func validChannelModelName(model string) bool {
	return strings.TrimSpace(model) != "" && utf8.RuneCountInString(model) <= 255 && !strings.ContainsAny(model, ",\r\n")
}
