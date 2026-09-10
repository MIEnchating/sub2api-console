package uptimekuma

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestNullableNativeMonitorOptionsUseEditableDefaults(t *testing.T) {
	o := monitorOptions(map[string]json.RawMessage{"timeout": json.RawMessage("null"), "maxredirects": json.RawMessage("null")})
	if o.Timeout != 16 || o.MaxRedirects != 10 {
		t.Fatalf("null native options block editing: %+v", o)
	}
}

func TestNativeMonthlyMaintenanceKeepsLastDaySelection(t *testing.T) {
	m := maintenanceConfig(map[string]json.RawMessage{"title": json.RawMessage(`"月末维护"`), "strategy": json.RawMessage(`"recurring-day-of-month"`), "daysOfMonth": json.RawMessage(`[15,"lastDay1"]`), "intervalDay": json.RawMessage("null"), "durationMinutes": json.RawMessage("null")})
	if !m.LastDay || !reflect.DeepEqual(m.DaysOfMonth, []int{15}) {
		t.Fatal("lost last day selection")
	}
	m.StartTime = "02:00"
	m.EndTime = "03:00"
	payload, err := maintenancePayload(m)
	if err != nil || string(payload["daysOfMonth"]) != `[15,"lastDay1"]` {
		t.Fatalf("monthly schedule: %v %s", err, payload["daysOfMonth"])
	}
}

func TestMaintenanceCreateSavesBothAssociationListsWithCreatedID(t *testing.T) {
	f := resourceFixture(t)
	c := f.configure(t, true)
	events := []string{}
	f.resourceHandler = func(event string, args []json.RawMessage) map[string]any {
		events = append(events, event)
		if event == "addMaintenance" {
			return map[string]any{"ok": true, "maintenanceID": 12}
		}
		if string(args[1]) != "12" {
			t.Error("association did not use created ID")
		}
		var refs []IDReference
		if err := json.Unmarshal(args[2], &refs); err != nil || len(refs) != 1 {
			t.Fatal("invalid associations")
		}
		if event == "addMonitorMaintenance" && refs[0].ID != 19 || event == "addMaintenanceStatusPage" && refs[0].ID != 9 {
			t.Error("wrong association target")
		}
		return map[string]any{"ok": true}
	}
	result, err := f.service.WriteResource(context.Background(), "maintenance", 0, ResourceInput{Action: "create", ConfigRevision: c.Revision, Maintenance: &MaintenanceConfig{Title: "发布维护", Strategy: "manual", Timezone: "UTC", Active: true, MonitorIDs: []int64{19}, StatusPageIDs: []int64{9}}})
	if err != nil || result["resource_id"] != int64(12) || !reflect.DeepEqual(events, []string{"addMaintenance", "addMonitorMaintenance", "addMaintenanceStatusPage"}) {
		t.Fatalf("create: %v %v", result, err)
	}
}

func TestStatusPageCreateLoadsAssignedIDBeforeSavingGroups(t *testing.T) {
	f := resourceFixture(t)
	c := f.configure(t, true)
	events := []string{}
	f.resourceHandler = func(event string, args []json.RawMessage) map[string]any {
		events = append(events, event)
		if event == "getStatusPage" {
			return map[string]any{"ok": true, "config": map[string]any{"id": 12, "slug": "new-status", "icon": "/icon.svg"}}
		}
		if event == "saveStatusPage" {
			var groups []PublicGroup
			if json.Unmarshal(args[4], &groups) != nil || len(groups) != 1 || groups[0].MonitorList[0].ID != 19 {
				t.Error("missing new groups")
			}
		}
		return map[string]any{"ok": true}
	}
	result, err := f.service.WriteResource(context.Background(), "status-pages", 0, ResourceInput{Action: "create", ConfigRevision: c.Revision, StatusPage: &StatusPageConfig{Title: "新状态页", Slug: "new-status", Theme: "auto", Groups: []PublicGroup{{Name: "服务", MonitorList: []PublicMonitor{{ID: 19}}}}}})
	if err != nil || result["resource_id"] != int64(12) || !reflect.DeepEqual(events, []string{"addStatusPage", "getStatusPage", "saveStatusPage"}) {
		t.Fatalf("create: %v %v", result, err)
	}
}

func TestStatusPageChangedAssociationsRejectWriteBeforeSaving(t *testing.T) {
	f := resourceFixture(t)
	c := f.configure(t, true)
	item, err := f.service.Resource(context.Background(), "status-pages", 9)
	if err != nil {
		t.Fatal(err)
	}
	wrote := false
	f.resourceHandler = func(string, []json.RawMessage) map[string]any { wrote = true; return map[string]any{"ok": true} }
	_, err = f.service.WriteResource(context.Background(), "status-pages", 9, ResourceInput{Action: "edit", ConfigRevision: c.Revision, Revision: item.Revision, AssociationRevision: "stale", StatusPage: item.StatusPage})
	if PublicError(err).Code != "kuma_resource_conflict" || wrote {
		t.Fatal("stale association overwrote current groups")
	}
}
