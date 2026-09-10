package uptimekuma

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func validateObjectIDs(ids []int64, objects map[string]json.RawMessage, unique bool) error {
	seen := map[int64]bool{}
	for _, id := range ids {
		if id <= 0 || unique && seen[id] {
			return failure("kuma_invalid_resource_ids", "目标 ID 无效或重复，请刷新后选择", 422)
		}
		seen[id] = true
		found := false
		for _, raw := range objects {
			var v IDReference
			if json.Unmarshal(raw, &v) != nil {
				return protocolError()
			}
			if v.ID == id {
				found = true
				break
			}
		}
		if !found {
			return failure("kuma_invalid_resource_ids", "目标已被删除，请刷新后重新选择", 409)
		}
	}
	return nil
}
func decodeResource(id int64, kind string, raw map[string]json.RawMessage) ResourceItem {
	item := ResourceItem{ID: id, Revision: stableResourceRevision(raw), Active: true}
	switch kind {
	case "notifications":
		item.Notification = decodeNotification(raw)
		item.Name = item.Notification.Name
		item.Type = item.Notification.Type
	case "maintenance":
		item.Maintenance = maintenanceConfig(raw)
		item.Name = item.Maintenance.Title
		item.Type = item.Maintenance.Strategy
		item.Active = item.Maintenance.Active
	case "status-pages":
		item.StatusPage = statusPageConfig(raw)
		item.Name = item.StatusPage.Title
		item.Type = item.StatusPage.Slug
	}
	return item
}
func (s *Service) resourceConnection(ctx context.Context) (configstore.UptimeKumaConfig, *socket, error) {
	c, e := s.store.UptimeKuma(ctx)
	if e != nil {
		return c, nil, e
	}
	conn, e := s.connect(ctx, c)
	return c, conn, e
}
func (s *Service) Resources(ctx context.Context, kind string) (ResourceList, error) {
	result := ResourceList{Items: []ResourceItem{}, Monitors: []Monitor{}, StatusPages: []IDReference{}}
	if !resourceKind(kind) {
		return result, failure("kuma_invalid_resource", "资源类型无效", 422)
	}
	ctx, release, err := s.enter(ctx)
	if err != nil {
		return result, err
	}
	defer release()
	c, conn, err := s.resourceConnection(ctx)
	result.Config = summary(c)
	if err != nil {
		return result, err
	}
	defer conn.conn.CloseNow()
	raw, err := resourceRaw(conn, kind)
	if err != nil {
		return result, err
	}
	for _, id := range sortedResourceIDs(raw) {
		result.Items = append(result.Items, decodeResource(id, kind, raw[id]))
	}
	if conn.monitors != nil {
		result.Monitors, err = decodeMonitors(conn.monitors, conn.statuses)
		if err != nil {
			return result, err
		}
	}
	for _, raw := range conn.statusPages {
		var v IDReference
		if json.Unmarshal(raw, &v) != nil || v.ID <= 0 {
			return result, protocolError()
		}
		result.StatusPages = append(result.StatusPages, v)
	}
	return result, nil
}
func (s *Service) resourceDetail(ctx context.Context, c configstore.UptimeKumaConfig, conn *socket, kind string, id int64, associations bool) (ResourceItem, map[string]json.RawMessage, []PublicGroup, error) {
	all, err := resourceRaw(conn, kind)
	if err != nil {
		return ResourceItem{}, nil, nil, err
	}
	raw, ok := all[id]
	if !ok {
		return ResourceItem{}, nil, nil, failure("kuma_resource_missing", "目标不存在，请刷新列表", 404)
	}
	item := decodeResource(id, kind, raw)
	var groups []PublicGroup
	if kind == "maintenance" && associations {
		r, e := conn.call(ctx, "getMonitorMaintenance", id)
		if e != nil {
			return item, nil, nil, e
		}
		for _, m := range r.Monitors {
			item.Maintenance.MonitorIDs = append(item.Maintenance.MonitorIDs, m.ID)
		}
		r, e = conn.call(ctx, "getMaintenanceStatusPage", id)
		if e != nil {
			return item, nil, nil, e
		}
		for _, p := range r.StatusPages {
			item.Maintenance.StatusPageIDs = append(item.Maintenance.StatusPageIDs, p.ID)
		}
		item.AssociationRevision = hashValue([]any{item.Maintenance.MonitorIDs, item.Maintenance.StatusPageIDs})
	}
	if kind == "status-pages" && associations {
		groups, err = s.publicGroups(ctx, c.BaseURL, item.StatusPage.Slug)
		if err != nil {
			return item, nil, nil, err
		}
		item.AssociationRevision = hashValue(groups)
		safe := make([]PublicGroup, 0, len(groups))
		for _, g := range groups {
			next := g
			next.MonitorList = append([]PublicMonitor{}, g.MonitorList...)
			for i := range next.MonitorList {
				next.MonitorList[i].URL = ""
			}
			safe = append(safe, next)
		}
		item.StatusPage.Groups = safe
	}
	return item, raw, groups, nil
}
func (s *Service) Resource(ctx context.Context, kind string, id int64) (ResourceItem, error) {
	if !resourceKind(kind) || id <= 0 {
		return ResourceItem{}, failure("kuma_invalid_resource", "资源类型或 ID 无效", 422)
	}
	ctx, release, err := s.enter(ctx)
	if err != nil {
		return ResourceItem{}, err
	}
	defer release()
	c, conn, err := s.resourceConnection(ctx)
	if err != nil {
		return ResourceItem{}, err
	}
	defer conn.conn.CloseNow()
	item, _, _, err := s.resourceDetail(ctx, c, conn, kind, id, true)
	return item, err
}
func (s *Service) WriteResource(ctx context.Context, kind string, id int64, in ResourceInput) (map[string]any, error) {
	result := map[string]any{"resource_id": id, "kind": kind, "action": in.Action}
	allowed := in.Action == "create" || in.Action == "edit" || in.Action == "delete" || kind == "notifications" && in.Action == "test" || kind == "maintenance" && (in.Action == "pause" || in.Action == "resume")
	if !resourceKind(kind) || !allowed || (in.Action == "create" && id != 0) || (in.Action != "create" && id <= 0) {
		return result, failure("kuma_invalid_resource", "资源类型、操作或 ID 无效", 422)
	}
	ctx, release, err := s.enter(ctx)
	if err != nil {
		return result, err
	}
	defer release()
	c, conn, err := s.resourceConnection(ctx)
	if err != nil {
		return result, err
	}
	defer conn.conn.CloseNow()
	if c.Revision != in.ConfigRevision {
		return result, configstore.ErrKumaConfigConflict
	}
	var current ResourceItem
	var raw map[string]json.RawMessage
	var oldGroups []PublicGroup
	if id > 0 {
		current, raw, oldGroups, err = s.resourceDetail(ctx, c, conn, kind, id, in.Action == "edit")
		if err != nil {
			return result, err
		}
		if in.Revision == "" || in.Revision != current.Revision {
			return result, failure("kuma_resource_conflict", "资源已被修改，请刷新后重试", 409)
		}
		if in.Action == "edit" && kind != "notifications" && (in.AssociationRevision == "" || in.AssociationRevision != current.AssociationRevision) {
			return result, failure("kuma_resource_conflict", "关联目标已被修改，请重新打开编辑窗口", 409)
		}
	}
	switch kind {
	case "notifications":
		switch in.Action {
		case "delete":
			_, err = conn.call(ctx, "deleteNotification", id)
		case "test":
			_, err = conn.call(ctx, "testNotification", raw)
		default:
			payload, e := notificationPayload(in.Notification, raw)
			if e != nil {
				return result, e
			}
			var r reply
			r, err = conn.call(ctx, "addNotification", payload, id)
			if err == nil && r.ID <= 0 {
				return result, protocolError()
			}
			if id > 0 && err == nil && r.ID != id {
				return result, protocolError()
			}
			result["resource_id"] = r.ID
		}
	case "maintenance":
		switch in.Action {
		case "delete":
			_, err = conn.call(ctx, "deleteMaintenance", id)
		case "pause":
			_, err = conn.call(ctx, "pauseMaintenance", id)
		case "resume":
			_, err = conn.call(ctx, "resumeMaintenance", id)
		default:
			payload, e := maintenancePayload(in.Maintenance)
			if e != nil {
				return result, e
			}
			if e = validateObjectIDs(in.Maintenance.MonitorIDs, conn.monitors, true); e != nil {
				return result, e
			}
			if e = validateObjectIDs(in.Maintenance.StatusPageIDs, conn.statusPages, true); e != nil {
				return result, e
			}
			event := "addMaintenance"
			if id > 0 {
				event = "editMaintenance"
				payload["id"], _ = json.Marshal(id)
			}
			var r reply
			r, err = conn.call(ctx, event, payload)
			if err != nil {
				return result, err
			}
			if r.MaintenanceID <= 0 || id > 0 && r.MaintenanceID != id {
				return result, protocolError()
			}
			id = r.MaintenanceID
			result["resource_id"] = id
			result["saved"] = true
			if _, err = conn.call(ctx, "addMonitorMaintenance", id, idReferences(in.Maintenance.MonitorIDs)); err != nil {
				return result, partialResourceError()
			}
			if _, err = conn.call(ctx, "addMaintenanceStatusPage", id, idReferences(in.Maintenance.StatusPageIDs)); err != nil {
				return result, partialResourceError()
			}
		}
	case "status-pages":
		if in.Action == "delete" {
			_, err = conn.call(ctx, "deleteStatusPage", current.StatusPage.Slug)
			break
		}
		if err = validateStatusPage(in.StatusPage, raw, conn.monitors); err != nil {
			return result, err
		}
		groups, e := preservePublicLinks(in.StatusPage.Groups, oldGroups)
		if e != nil {
			return result, e
		}
		if id == 0 {
			for _, item := range conn.statusPages {
				var p map[string]json.RawMessage
				if json.Unmarshal(item, &p) != nil {
					return result, protocolError()
				}
				if rawString(p, "slug") == in.StatusPage.Slug {
					return result, failure("kuma_invalid_status_page", "状态页路径已存在，请使用其他路径", 409)
				}
			}
			if _, err = conn.call(ctx, "addStatusPage", in.StatusPage.Title, in.StatusPage.Slug); err != nil {
				return result, err
			}
			result["saved"] = true
			result["slug"] = in.StatusPage.Slug
			r, e := conn.call(ctx, "getStatusPage", in.StatusPage.Slug)
			if e != nil {
				return result, partialResourceError()
			}
			if json.Unmarshal(r.Config, &raw) != nil || raw == nil {
				return result, partialResourceError()
			}
			id = int64(rawInt(raw, "id", 0))
			if id <= 0 {
				return result, partialResourceError()
			}
			result["resource_id"] = id
		}
		payload := statusPagePayload(in.StatusPage, raw)
		icon := rawString(raw, "icon")
		if _, err = conn.call(ctx, "saveStatusPage", in.StatusPage.Slug, payload, icon, groups); err != nil && in.Action == "create" {
			return result, partialResourceError()
		}
	}
	return result, err
}
func partialResourceError() error {
	return failure("kuma_resource_partial", "主体已保存，但部分关联或展示设置未完成。请刷新列表后编辑该项补全，不要重复新建", 502)
}
func resourceID(value string) (int64, error) {
	n, e := strconv.ParseInt(value, 10, 64)
	if e != nil || n < 0 {
		return 0, failure("kuma_invalid_resource", "资源 ID 无效", 422)
	}
	return n, nil
}
