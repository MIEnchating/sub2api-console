package uptimekuma

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type remoteMonitor struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	URL      string `json:"url"`
	Active   bool   `json:"active"`
	Parent   *int64 `json:"parent"`
	Interval int    `json:"interval"`
}

func decodeMonitor(data json.RawMessage) (Monitor, map[string]json.RawMessage, error) {
	var remote remoteMonitor
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &remote) != nil || json.Unmarshal(data, &raw) != nil || remote.ID <= 0 || remote.Name == "" || remote.Type == "" {
		return Monitor{}, nil, protocolError()
	}
	u, redacted := publicURL(remote.URL)
	result := Monitor{Options: monitorOptions(raw), ID: remote.ID, Key: "id:" + strconv.FormatInt(remote.ID, 10), Name: remote.Name, Type: remote.Type, URL: u, URLRedacted: redacted, Active: remote.Active, Parent: remote.Parent, Interval: remote.Interval, Revision: monitorRevision(raw)}
	result.Target = displayTarget(result)
	return result, raw, nil
}
func decodeMonitors(raw map[string]json.RawMessage, statuses map[int64]int) ([]Monitor, error) {
	result := make([]Monitor, 0, len(raw))
	for key, data := range raw {
		m, _, err := decodeMonitor(data)
		if err != nil {
			return nil, err
		}
		if strconv.FormatInt(m.ID, 10) != key {
			return nil, protocolError()
		}
		if status, ok := statuses[m.ID]; ok && status >= 0 && status <= 3 {
			m.Status = &status
		}
		result = append(result, m)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}
func validateMonitor(in MonitorInput) error {
	if strings.TrimSpace(in.Name) == "" || utf8.RuneCountInString(in.Name) > 150 {
		return failure("kuma_invalid_monitor", "监控项名称必须为 1 到 150 个字符", 422)
	}
	if in.Type == "" || len(in.Type) > 64 {
		return failure("kuma_unsupported_type", "监控类型无效", 422)
	}
	if in.Interval < 20 || in.Interval > 86400 {
		return failure("kuma_invalid_interval", "检测间隔必须为 20 到 86400 秒", 422)
	}
	if in.Parent != nil && *in.Parent <= 0 {
		return failure("kuma_invalid_parent", "请选择有效的监控分组", 422)
	}
	if isHTTP(in.Type) && in.URL != "" && !validateHTTPURL(in.URL) {
		return failure("kuma_invalid_monitor_url", "监控地址必须是完整的 HTTP(S) URL，不能包含用户名密码或片段", 422)
	}
	return validateOptions(in.Type, in.Options)
}
func (s *Service) Write(ctx context.Context, id int64, in WriteInput) (int64, error) {
	if in.Action != "create" && in.Action != "edit" && in.Action != "pause" && in.Action != "resume" && in.Action != "delete" {
		return 0, failure("kuma_invalid_action", "不支持的监控操作", 422)
	}
	if (in.Action == "create" && id != 0) || (in.Action != "create" && id <= 0) {
		return 0, failure("kuma_invalid_id", "监控项 ID 无效，请刷新列表", 422)
	}
	if in.Action == "create" || in.Action == "edit" {
		if in.Monitor.TemplateAuthOverride && (in.Monitor.TemplateID == "" || in.Monitor.Options == nil) {
			return 0, failure("kuma_invalid_auth", "请选择模板并填写独立鉴权设置", 422)
		}
		if in.Action == "create" || in.Monitor.Options != nil {
			if err := s.resolveTemplate(ctx, &in.Monitor); err != nil {
				return 0, err
			}
		}
		if err := validateMonitor(in.Monitor); err != nil {
			return 0, err
		}
	}
	if in.Action == "create" && isHTTP(in.Monitor.Type) && in.Monitor.URL == "" {
		return 0, failure("kuma_invalid_monitor_url", "请输入监控地址", 422)
	}
	if in.Action == "create" && !supportedMonitorTypes[in.Monitor.Type] {
		return 0, failure("kuma_unsupported_type", "不支持新增此监控类型", 422)
	}
	if in.Action == "create" && (in.Monitor.Type == "port" || in.Monitor.Type == "ping" || in.Monitor.Type == "dns" || in.Monitor.Type == "keyword") && in.Monitor.Options == nil {
		return 0, failure("kuma_invalid_options", "请填写该监控类型的设置", 422)
	}
	ctx, release, err := s.enter(ctx)
	if err != nil {
		return 0, err
	}
	defer release()
	c, err := s.store.UptimeKuma(ctx)
	if err != nil {
		return 0, err
	}
	if c.Revision != in.ConfigRevision {
		return 0, configstore.ErrKumaConfigConflict
	}
	conn, err := s.connect(ctx, c)
	if err != nil {
		return 0, err
	}
	defer conn.conn.CloseNow()
	var current Monitor
	var raw map[string]json.RawMessage
	if id > 0 {
		r, err := conn.call(ctx, "getMonitor", id)
		if err != nil {
			return 0, err
		}
		current, raw, err = decodeMonitor(r.Monitor)
		if err != nil {
			return 0, err
		}
		if current.ID != id {
			return 0, protocolError()
		}
		if in.Revision == "" || current.Revision != in.Revision {
			return 0, failure("kuma_monitor_conflict", "监控项已更改，请刷新后重试", 409)
		}
	}
	if in.Action == "create" || in.Action == "edit" {
		if in.Action == "edit" && in.Monitor.TemplateID != "" && in.Monitor.Options == nil {
			in.Monitor.Options = current.Options
			if err := s.resolveTemplate(ctx, &in.Monitor); err != nil {
				return 0, err
			}
		}
		if o := in.Monitor.Options; o != nil && isHTTP(in.Monitor.Type) && o.AuthMethod != "none" && o.AuthMethod != "basic" && o.AuthMethod != "bearer" {
			if in.Action == "create" || o.AuthMethod != rawString(raw, "authMethod") {
				return 0, failure("kuma_invalid_options", "不支持切换到该鉴权方式", 422)
			}
		}
		if in.Action == "edit" && in.Monitor.Type != current.Type {
			return 0, failure("kuma_monitor_structure", "编辑时不能更改监控类型，请新建对应类型的监控项", 422)
		}
		if in.Monitor.Parent != nil && *in.Monitor.Parent == id {
			return 0, failure("kuma_invalid_parent", "监控项不能归入自身", 422)
		}
		if in.Action == "edit" && !sameParent(in.Monitor.Parent, current.Parent) {
			if _, err = conn.call(ctx, "getMonitorList"); err != nil {
				return 0, err
			}
			if err = validateParentChain(conn.monitors, id, in.Monitor.Parent); err != nil {
				return 0, err
			}
		}
		if in.Monitor.Parent != nil {
			r, err := conn.call(ctx, "getMonitor", *in.Monitor.Parent)
			if err != nil {
				return 0, err
			}
			p, _, err := decodeMonitor(r.Monitor)
			if err != nil {
				return 0, err
			}
			if p.ID != *in.Monitor.Parent || p.Type != "group" {
				return 0, failure("kuma_invalid_parent", "目标监控分组不存在，请刷新后重试", 422)
			}
		}
	}
	if in.Action == "delete" && current.Type == "group" {
		if _, err = conn.call(ctx, "getMonitorList"); err != nil {
			return 0, err
		}
		if conn.monitors == nil {
			return 0, protocolError()
		}
		list, err := decodeMonitors(conn.monitors, nil)
		if err != nil {
			return 0, err
		}
		for _, m := range list {
			if m.Parent != nil && *m.Parent == id {
				return 0, failure("kuma_group_not_empty", "该分组仍有监控项，请先移出或删除子项", 409)
			}
		}
	}
	if in.Monitor.Options != nil && (in.Action == "create" || in.Action == "edit") {
		if err = validateNotificationIDs(conn, in.Monitor.Options.NotificationIDs); err != nil {
			return 0, err
		}
	}
	var r reply
	switch in.Action {
	case "create":
		payload := map[string]any{"name": strings.TrimSpace(in.Monitor.Name), "type": in.Monitor.Type, "url": in.Monitor.URL, "interval": in.Monitor.Interval, "parent": in.Monitor.Parent, "active": true, "method": "GET", "timeout": 16, "retryInterval": in.Monitor.Interval, "maxretries": 0, "maxredirects": 10, "accepted_statuscodes": []string{"200-299"}, "notificationIDList": map[string]bool{}, "conditions": []any{}, "kafkaProducerBrokers": []string{}, "kafkaProducerSaslOptions": map[string]any{}}
		data, _ := json.Marshal(payload)
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(data, &fields)
		applyOptions(fields, in.Monitor.Type, in.Monitor.Options)
		if in.Monitor.Type == "push" {
			token, e := randomToken()
			if e != nil {
				return 0, e
			}
			fields["pushToken"], _ = json.Marshal(token)
		}
		r, err = conn.call(ctx, "add", fields)
		if err == nil && r.MonitorID <= 0 {
			return 0, protocolError()
		}
		id = r.MonitorID
	case "edit":
		// Round trip the server's full monitor object privately so headers, bodies,
		// authentication, notifications and unrelated options are never overwritten.
		raw["name"], _ = json.Marshal(strings.TrimSpace(in.Monitor.Name))
		raw["interval"], _ = json.Marshal(in.Monitor.Interval)
		raw["parent"], _ = json.Marshal(in.Monitor.Parent)
		applyOptions(raw, current.Type, in.Monitor.Options)
		if in.Monitor.URL != "" && isHTTP(current.Type) {
			raw["url"], _ = json.Marshal(in.Monitor.URL)
		}
		_, err = conn.call(ctx, "editMonitor", raw)
	case "pause":
		_, err = conn.call(ctx, "pauseMonitor", id)
	case "resume":
		_, err = conn.call(ctx, "resumeMonitor", id)
	case "delete":
		_, err = conn.call(ctx, "deleteMonitor", id)
	}
	return id, err
}
func sameParent(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func validateParentChain(monitors map[string]json.RawMessage, id int64, parent *int64) error {
	if monitors == nil {
		return protocolError()
	}
	seen := map[int64]bool{id: true}
	for parent != nil {
		if seen[*parent] {
			return failure("kuma_invalid_parent", "不能将分组移入自身或子分组", 422)
		}
		seen[*parent] = true
		data, ok := monitors[strconv.FormatInt(*parent, 10)]
		if !ok {
			return failure("kuma_invalid_parent", "目标分组不存在，请刷新后重试", 422)
		}
		m, _, err := decodeMonitor(data)
		if err != nil {
			return err
		}
		if m.Type != "group" {
			return failure("kuma_invalid_parent", "目标必须是监控分组", 422)
		}
		parent = m.Parent
	}
	return nil
}
