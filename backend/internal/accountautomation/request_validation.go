package accountautomation

import (
	"encoding/hex"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Creation, saved schedules and recovery accept the same normalized scope.
func validateRequest(request Request) error {
	if err := validateSelection(request); err != nil {
		return err
	}
	if len(request.GroupIDs) == 0 || len(request.GroupIDs) > 50 {
		return errors.New("至少选择一个分组，且不能超过 50 个")
	}
	if strings.TrimSpace(request.PlatformID) == "" {
		return errors.New("请选择 New API 平台")
	}
	if len(request.Channels) > 50 {
		return errors.New("渠道不能超过 50 个")
	}
	seenGroups := make(map[string]bool, len(request.GroupIDs))
	for _, id := range request.GroupIDs {
		if !stableID(id) || seenGroups[id] {
			return errors.New("分组必须使用不重复的稳定 ID")
		}
		seenGroups[id] = true
	}
	seenChannels := make(map[string]bool, len(request.Channels))
	for _, channel := range request.Channels {
		if !stableID(channel.ID) || !validChannelVersion(channel.Version) || seenChannels[channel.ID] {
			return errors.New("渠道必须使用不重复的稳定 ID 和最新版本")
		}
		seenChannels[channel.ID] = true
	}
	if request.PricingSource != "official" && request.PricingSource != "cached" {
		return errors.New("价格来源无效")
	}
	return nil
}

func validChannelVersion(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func stableID(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value[0] != '0'
}

func normalizeRequest(request Request) Request {
	if request.PricingSource == "" {
		request.PricingSource = "official"
	}
	if len(request.Channels) == 0 {
		request.Channels = []ChannelTarget{}
		request.PriceEnabled = false
		request.PublishMissing = false
	}
	if !request.PublishMissing {
		request.PriceEnabled = false
	}
	return request
}

func validateSelection(request Request) error {
	if request.PublishMissing && len(request.Channels) > 0 && len(request.SelectedModels) == 0 {
		return errors.New("请先读取并选择要上架的模型；未选择模型不会自动上架")
	}
	if len(request.SelectedModels) > 1000 {
		return errors.New("所选模型不能超过 1000 个")
	}
	seen := map[string]bool{}
	for _, model := range request.SelectedModels {
		key := modelKey(model)
		if key == "" || model != strings.TrimSpace(model) || utf8.RuneCountInString(model) > 255 ||
			strings.IndexFunc(model, unicode.IsControl) >= 0 || seen[key] {
			return errors.New("请选择不重复的有效模型名称，每项不能超过 255 个字符")
		}
		seen[key] = true
	}
	return nil
}
