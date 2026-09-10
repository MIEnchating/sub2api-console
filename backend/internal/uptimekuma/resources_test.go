package uptimekuma

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func resourceFixture(t *testing.T) *kumaFixture {
	f := newFixture(t)
	f.resourceEvents = map[string]any{
		"notificationList": []any{map[string]any{"id": 7, "name": "通知", "active": true, "isDefault": false, "config": `{"name":"通知","type":"webhook","webhookURL":"https://hooks.example/private-token","webhookAdditionalHeaders":"private-header"}`}},
		"maintenanceList":  map[string]any{"8": map[string]any{"id": 8, "title": "维护", "description": "test", "strategy": "manual", "active": true, "timezoneOption": "UTC", "dateRange": []any{nil, nil}, "durationMinutes": 60, "intervalDay": 1}},
		"statusPageList":   map[string]any{"9": map[string]any{"id": 9, "title": "状态", "slug": "status", "theme": "auto", "domainNameList": []string{}, "icon": "/icon.svg", "customCSS": "private-css", "analyticsType": nil}},
		"monitorList":      f.monitors,
	}
	f.publicGroups = []PublicGroup{{ID: 10, Name: "原分组", MonitorList: []PublicMonitor{{ID: 19, URL: "https://service.example/private-link", SendURL: true}}}}
	return f
}
func TestResourceListsHideSecretsAndKeepStableIDs(t *testing.T) {
	f := resourceFixture(t)
	f.configure(t, true)
	for _, kind := range []string{"notifications", "maintenance", "status-pages"} {
		list, err := f.service.Resources(context.Background(), kind)
		if err != nil {
			t.Fatal(err)
		}
		if len(list.Items) != 1 || list.Items[0].ID <= 0 || list.Items[0].Revision == "" {
			t.Fatal("missing identity")
		}
		data, _ := json.Marshal(list)
		for _, secret := range []string{"private-token", "private-header", "private-css", "private-body", "private-url-key"} {
			if strings.Contains(string(data), secret) {
				t.Fatalf("leaked %s", secret)
			}
		}
	}
}
func TestNotificationEditPreservesSecretAndRejectsConflictingVersion(t *testing.T) {
	f := resourceFixture(t)
	c := f.configure(t, true)
	list, e := f.service.Resources(context.Background(), "notifications")
	if e != nil {
		t.Fatal(e)
	}
	var payload map[string]json.RawMessage
	calls := 0
	f.resourceHandler = func(event string, args []json.RawMessage) map[string]any {
		if event != "addNotification" {
			t.Errorf("unexpected %s", event)
		}
		calls++
		_ = json.Unmarshal(args[1], &payload)
		return map[string]any{"ok": true, "id": 7}
	}
	n := list.Items[0].Notification
	n.Name = "更名"
	in := ResourceInput{Action: "edit", ConfigRevision: c.Revision, Revision: "old", Notification: n}
	if _, e = f.service.WriteResource(context.Background(), "notifications", 7, in); PublicError(e).Code != "kuma_resource_conflict" || calls != 0 {
		t.Fatal("stale version wrote")
	}
	in.Revision = list.Items[0].Revision
	if _, e = f.service.WriteResource(context.Background(), "notifications", 7, in); e != nil {
		t.Fatal(e)
	}
	if rawString(payload, "webhookURL") != "https://hooks.example/private-token" || rawString(payload, "webhookAdditionalHeaders") != "private-header" || rawString(payload, "name") != "更名" {
		t.Fatal("lost saved credentials")
	}
}
func TestMaintenancePartialSaveReturnsCreatedIDAndDoesNotReplay(t *testing.T) {
	f := resourceFixture(t)
	c := f.configure(t, true)
	calls := []string{}
	f.resourceHandler = func(event string, args []json.RawMessage) map[string]any {
		calls = append(calls, event)
		if event == "addMaintenance" {
			return map[string]any{"ok": true, "maintenanceID": 12}
		}
		return map[string]any{"ok": false}
	}
	in := ResourceInput{Action: "create", ConfigRevision: c.Revision, Maintenance: &MaintenanceConfig{Title: "维护", Strategy: "manual", Timezone: "UTC", Active: true, MonitorIDs: []int64{19}}}
	result, e := f.service.WriteResource(context.Background(), "maintenance", 0, in)
	if PublicError(e).Code != "kuma_resource_partial" || result["resource_id"] != int64(12) || len(calls) != 2 || calls[0] != "addMaintenance" || calls[1] != "addMonitorMaintenance" {
		t.Fatalf("unexpected partial result %v %v %v", result, e, calls)
	}
}
func TestMaintenanceRejectsMissingTargetsBeforeRemoteWrite(t *testing.T) {
	f := resourceFixture(t)
	c := f.configure(t, true)
	called := false
	f.resourceHandler = func(string, []json.RawMessage) map[string]any { called = true; return map[string]any{"ok": true} }
	_, e := f.service.WriteResource(context.Background(), "maintenance", 0, ResourceInput{Action: "create", ConfigRevision: c.Revision, Maintenance: &MaintenanceConfig{Title: "维护", Strategy: "manual", Timezone: "UTC", MonitorIDs: []int64{999}}})
	if PublicError(e).Code != "kuma_invalid_resource_ids" || called {
		t.Fatal("invalid association accepted")
	}
}
func TestStatusPageEditPreservesCustomSettingsAndHiddenLinks(t *testing.T) {
	f := resourceFixture(t)
	c := f.configure(t, true)
	item, e := f.service.Resource(context.Background(), "status-pages", 9)
	if e != nil {
		t.Fatal(e)
	}
	if item.StatusPage.Groups[0].MonitorList[0].URL != "" {
		t.Fatal("private link leaked")
	}
	var payload map[string]json.RawMessage
	var groups []PublicGroup
	f.resourceHandler = func(event string, args []json.RawMessage) map[string]any {
		if event != "saveStatusPage" {
			t.Errorf("unexpected event %s", event)
		}
		_ = json.Unmarshal(args[2], &payload)
		_ = json.Unmarshal(args[4], &groups)
		return map[string]any{"ok": true}
	}
	item.StatusPage.Title = "更新状态页"
	_, e = f.service.WriteResource(context.Background(), "status-pages", 9, ResourceInput{Action: "edit", ConfigRevision: c.Revision, Revision: item.Revision, AssociationRevision: item.AssociationRevision, StatusPage: item.StatusPage})
	if e != nil {
		t.Fatal(e)
	}
	if rawString(payload, "customCSS") != "private-css" || groups[0].MonitorList[0].URL != "https://service.example/private-link" {
		t.Fatal("lost unchanged config")
	}
}
func TestMaintenanceRejectsInvalidDatesCronAndWeekdays(t *testing.T) {
	for _, m := range []MaintenanceConfig{
		{Title: "维护", Strategy: "single", Timezone: "UTC", Start: "2026-01-02 12:00", End: "2026-01-01 12:00"},
		{Title: "维护", Strategy: "cron", Timezone: "UTC", Cron: "bad", DurationMinutes: 60},
		{Title: "维护", Strategy: "recurring-weekday", Timezone: "UTC", StartTime: "02:00", EndTime: "03:00", Weekdays: []int{8}},
	} {
		if _, e := maintenancePayload(&m); e == nil {
			t.Fatal("accepted invalid schedule")
		}
	}
}
func TestResourceTestAndDeleteUseStableNotificationID(t *testing.T) {
	for _, action := range []string{"test", "delete"} {
		t.Run(action, func(t *testing.T) {
			f := resourceFixture(t)
			c := f.configure(t, true)
			list, e := f.service.Resources(context.Background(), "notifications")
			if e != nil {
				t.Fatal(e)
			}
			count := 0
			f.resourceHandler = func(event string, args []json.RawMessage) map[string]any {
				count++
				if action == "delete" && string(args[1]) != "7" {
					t.Error("wrong ID")
				}
				if action == "test" {
					var raw map[string]json.RawMessage
					_ = json.Unmarshal(args[1], &raw)
					if rawString(raw, "webhookURL") != "https://hooks.example/private-token" {
						t.Error("test missing credential")
					}
				}
				return map[string]any{"ok": true}
			}
			_, e = f.service.WriteResource(context.Background(), "notifications", 7, ResourceInput{Action: action, ConfigRevision: c.Revision, Revision: list.Items[0].Revision})
			if e != nil || count != 1 {
				t.Fatalf("write %v", e)
			}
		})
	}
}
