package accountautomation

import (
	"errors"

	"github.com/MIEnchating/sub2api-console/backend/internal/newapimanagement"
)

// Validate the entire remote snapshot before accepting any refreshed version
// or publishing to a channel. Preserve the user's selected target order.
func channelTargets(targets []ChannelTarget, channels []newapimanagement.Channel) ([]ChannelTarget, error) {
	if len(channels) != len(targets) {
		return nil, errors.New("部分目标渠道不存在，请刷新后重新预览")
	}
	versions := make(map[string]string, len(targets))
	for _, target := range targets {
		versions[target.ID] = ""
	}
	for _, channel := range channels {
		previous, selected := versions[channel.ID]
		if !selected || previous != "" {
			return nil, errors.New("远端返回了未选择或重复的渠道，请刷新后重新预览")
		}
		if !validChannelVersion(channel.Version) {
			return nil, errors.New("远端渠道版本无效，请刷新后重新预览")
		}
		versions[channel.ID] = channel.Version
	}
	refreshed := make([]ChannelTarget, 0, len(targets))
	for _, target := range targets {
		refreshed = append(refreshed, ChannelTarget{ID: target.ID, Version: versions[target.ID]})
	}
	return refreshed, nil
}
