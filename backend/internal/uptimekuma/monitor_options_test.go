package uptimekuma

import (
	"context"
	"encoding/json"
	"testing"
)

func TestMonitorMoveRejectsCyclesAndPreservesSensitiveSettings(t *testing.T) {
	f := resourceFixture(t)
	f.monitors["7"] = json.RawMessage(`{"id":7,"name":"分组","type":"group","parent":null,"active":true,"interval":60}`)
	c := f.configure(t, true)
	snapshot, e := f.service.Snapshot(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	var m Monitor
	for _, item := range snapshot.Monitors {
		if item.ID == 19 {
			m = item
		}
	}
	parent := int64(7)
	_, e = f.service.Write(context.Background(), 19, WriteInput{Action: "edit", ConfigRevision: c.Revision, Revision: m.Revision, Monitor: MonitorInput{Name: m.Name, Type: m.Type, Interval: m.Interval, Parent: &parent, Options: m.Options}})
	if e != nil {
		t.Fatal(e)
	}
	if string(f.edited["parent"]) != "7" || rawString(f.edited, "headers") != "private-headers" || rawString(f.edited, "body") != "private-body" {
		t.Fatal("move lost config")
	}
	if e = validateParentChain(map[string]json.RawMessage{"7": json.RawMessage(`{"id":7,"name":"分组","type":"group","parent":8}`), "8": json.RawMessage(`{"id":8,"name":"子分组","type":"group","parent":7}`)}, 7, func() *int64 { v := int64(8); return &v }()); e == nil {
		t.Fatal("cycle accepted")
	}
}
func TestMonitorOptionsValidationRejectsInvalidTargetsAndHeaders(t *testing.T) {
	for _, tc := range []struct {
		kind    string
		options MonitorOptions
	}{{"port", MonitorOptions{Hostname: "host", Port: 0}}, {"ping", MonitorOptions{Hostname: "https://host"}}, {"http", MonitorOptions{Method: "GET", Timeout: 16, RetryInterval: 60, Headers: "[]", AcceptedStatusCodes: []string{"200-299"}}}} {
		if e := validateOptions(tc.kind, &tc.options); e == nil {
			t.Fatal("invalid options accepted")
		}
	}
}
func TestExtendedMonitorCreateSendsTypeSpecificFields(t *testing.T) {
	for _, kind := range []string{"port", "ping", "dns", "keyword", "push"} {
		t.Run(kind, func(t *testing.T) {
			f := resourceFixture(t)
			c := f.configure(t, true)
			o := monitorOptions(map[string]json.RawMessage{})
			o.Hostname = "monitor.example"
			o.Port = 443
			o.Keyword = "healthy"
			_, e := f.service.Write(context.Background(), 0, WriteInput{Action: "create", ConfigRevision: c.Revision, Monitor: MonitorInput{Name: "测试", Type: kind, URL: "https://monitor.example", Interval: 60, Options: o}})
			if e != nil {
				t.Fatal(e)
			}
			if rawString(f.edited, "type") != kind {
				t.Fatal("wrong type")
			}
			if kind == "push" && len(rawString(f.edited, "pushToken")) < 32 {
				t.Fatal("weak push token")
			}
		})
	}
}
