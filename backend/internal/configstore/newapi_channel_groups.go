package configstore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const newAPIChannelGroupsPrefix = "newapi.channel_groups."

var channelGroupPlatformPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)
var channelGroupIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,19}$`)
var ErrChannelGroupsConflict = errors.New("渠道分组已在其他页面修改，请关闭弹窗并重新打开")

type NewAPIChannelGroup struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	ChannelIDs []string `json:"channel_ids"`
}
type NewAPIChannelGroups struct {
	Groups  []NewAPIChannelGroup `json:"groups"`
	Version string               `json:"version"`
}

func channelGroupsKey(platformID string) string { return newAPIChannelGroupsPrefix + platformID }
func normalizeNewAPIChannelGroups(platformID string, groups []NewAPIChannelGroup) ([]NewAPIChannelGroup, error) {
	if !channelGroupPlatformPattern.MatchString(platformID) {
		return nil, errors.New("平台标识无效")
	}
	if len(groups) > 100 {
		return nil, errors.New("渠道分组最多 100 个")
	}
	seenGroups := map[string]bool{}
	seenNames := map[string]bool{}
	out := make([]NewAPIChannelGroup, 0, len(groups))
	for _, group := range groups {
		id := strings.TrimSpace(group.ID)
		if !channelGroupPlatformPattern.MatchString(id) || id == "all" || seenGroups[id] {
			return nil, errors.New("渠道分组标识重复或无效")
		}
		name := strings.TrimSpace(group.Name)
		if name == "" || utf8.RuneCountInString(name) > 80 || seenNames[name] {
			return nil, errors.New("渠道分组名称必须唯一且不超过 80 个字符")
		}
		seenGroups[id], seenNames[name] = true, true
		if len(group.ChannelIDs) > 1000 {
			return nil, errors.New("每组最多 1000 个渠道")
		}
		ids := make([]string, 0, len(group.ChannelIDs))
		seen := map[string]bool{}
		for _, raw := range group.ChannelIDs {
			id := strings.TrimSpace(raw)
			_, parseErr := strconv.ParseInt(id, 10, 64)
			if !channelGroupIDPattern.MatchString(id) || parseErr != nil || seen[id] {
				return nil, errors.New("渠道分组包含无效或重复渠道 ID")
			}
			seen[id] = true
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool {
			left, _ := strconv.ParseInt(ids[i], 10, 64)
			right, _ := strconv.ParseInt(ids[j], 10, 64)
			return left < right
		})
		out = append(out, NewAPIChannelGroup{ID: id, Name: name, ChannelIDs: ids})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
func channelGroupsVersion(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
func (s *Store) NewAPIChannelGroups(ctx context.Context, platformID string) (NewAPIChannelGroups, error) {
	result := NewAPIChannelGroups{Groups: []NewAPIChannelGroup{}}
	if !channelGroupPlatformPattern.MatchString(platformID) {
		return result, errors.New("平台标识无效")
	}
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, channelGroupsKey(platformID)).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal([]byte(raw), &result.Groups); err != nil {
		return result, errors.New("渠道分组配置不可读")
	}
	groups, err := normalizeNewAPIChannelGroups(platformID, result.Groups)
	if err != nil {
		return NewAPIChannelGroups{}, err
	}
	result.Groups, result.Version = groups, channelGroupsVersion(raw)
	return result, nil
}
func (s *Store) SaveNewAPIChannelGroups(ctx context.Context, platformID string, groups []NewAPIChannelGroup, version string) (NewAPIChannelGroups, error) {
	normalized, err := normalizeNewAPIChannelGroups(platformID, groups)
	if err != nil {
		return NewAPIChannelGroups{}, err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return NewAPIChannelGroups{}, err
	}
	key := channelGroupsKey(platformID)
	current, err := s.NewAPIChannelGroups(ctx, platformID)
	if err != nil {
		return NewAPIChannelGroups{}, err
	}
	if current.Version != version {
		return NewAPIChannelGroups{}, ErrChannelGroupsConflict
	}
	if version == "" {
		var result sql.Result
		result, err = s.db.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO NOTHING`, key, string(encoded))
		if err == nil {
			var n int64
			n, err = result.RowsAffected()
			if err == nil && n != 1 {
				err = ErrChannelGroupsConflict
			}
		}
	} else {
		old, _ := json.Marshal(current.Groups)
		var result sql.Result
		result, err = s.db.ExecContext(ctx, `UPDATE settings SET value=? WHERE key=? AND value=?`, string(encoded), key, string(old))
		if err == nil {
			var n int64
			n, err = result.RowsAffected()
			if err == nil && n != 1 {
				err = ErrChannelGroupsConflict
			}
		}
	}
	if err != nil {
		return NewAPIChannelGroups{}, err
	}
	return NewAPIChannelGroups{Groups: normalized, Version: channelGroupsVersion(string(encoded))}, nil
}
