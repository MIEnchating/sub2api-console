package uptimekuma_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/uptimekuma"
)

func TestMaintenanceAssociationsPreserveSelectedStableIDsAndOrder(t *testing.T) {
	f := newResourceValidationFixture(t,
		map[string]json.RawMessage{"remote-a": json.RawMessage(`{"id":19,"name":"同名"}`), "remote-b": json.RawMessage(`{"id":23,"name":"同名"}`)},
		map[string]json.RawMessage{"status-slug": json.RawMessage(`{"id":9,"title":"状态"}`)},
	)
	result, err := f.maintenance([]int64{23, 19}, []int64{9})
	if err != nil || result["resource_id"] != int64(37) {
		t.Fatalf("maintenance failed: %v, %v", result, err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !reflect.DeepEqual(f.monitors, []uptimekuma.IDReference{{ID: 23}, {ID: 19}}) ||
		!reflect.DeepEqual(f.pages, []uptimekuma.IDReference{{ID: 9}}) {
		t.Fatalf("association identities/order changed: monitors=%v pages=%v", f.monitors, f.pages)
	}
	if !reflect.DeepEqual(f.events, []string{"addMaintenance", "addMonitorMaintenance", "addMaintenanceStatusPage"}) {
		t.Fatalf("unexpected writes: %v", f.events)
	}
}

func TestMaintenanceInvalidAssociationRejectsBeforeAnyRemoteWrite(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ids     []int64
		objects map[string]json.RawMessage
		code    string
		status  int
	}{
		{"zero", []int64{0}, map[string]json.RawMessage{}, "kuma_invalid_resource_ids", 422},
		{"negative", []int64{-19}, map[string]json.RawMessage{}, "kuma_invalid_resource_ids", 422},
		{"duplicate", []int64{19, 19}, map[string]json.RawMessage{"19": json.RawMessage(`{"id":19}`)}, "kuma_invalid_resource_ids", 422},
		{"deleted", []int64{19}, map[string]json.RawMessage{}, "kuma_invalid_resource_ids", 409},
		{"key-is-not-identity", []int64{19}, map[string]json.RawMessage{"19": json.RawMessage(`{"id":23,"name":"19"}`)}, "kuma_invalid_resource_ids", 409},
		{"string-id", []int64{19}, map[string]json.RawMessage{"19": json.RawMessage(`{"id":"19"}`)}, "kuma_protocol_error", 502},
		{"invalid-name", []int64{19}, map[string]json.RawMessage{"19": json.RawMessage(`{"id":19,"name":42}`)}, "kuma_protocol_error", 502},
		{"null-object", []int64{19}, map[string]json.RawMessage{"19": json.RawMessage(`null`)}, "kuma_invalid_resource_ids", 409},
		{"missing-before-invalid", []int64{999, 0}, map[string]json.RawMessage{"19": json.RawMessage(`{"id":19}`)}, "kuma_invalid_resource_ids", 409},
		{"missing-before-duplicate", []int64{19, 19}, map[string]json.RawMessage{}, "kuma_invalid_resource_ids", 409},
		{"invalid-after-existing", []int64{19, 0}, map[string]json.RawMessage{"19": json.RawMessage(`{"id":19}`)}, "kuma_invalid_resource_ids", 422},
		{"malformed-before-invalid", []int64{19, 0}, map[string]json.RawMessage{"19": json.RawMessage(`{"id":"19"}`)}, "kuma_protocol_error", 502},
	} {
		for _, target := range []string{"monitors", "pages"} {
			t.Run(tc.name+"/"+target, func(t *testing.T) {
				monitors, pages := map[string]json.RawMessage{}, map[string]json.RawMessage{}
				var monitorIDs, pageIDs []int64
				if target == "monitors" {
					monitors, monitorIDs = tc.objects, tc.ids
				} else {
					pages, pageIDs = tc.objects, tc.ids
				}
				f := newResourceValidationFixture(t, monitors, pages)
				_, err := f.maintenance(monitorIDs, pageIDs)
				public := uptimekuma.PublicError(err)
				if err == nil || public.Code != tc.code || public.Status != tc.status {
					t.Fatalf("unexpected validation error: %v (%+v)", err, public)
				}
				f.mu.Lock()
				defer f.mu.Unlock()
				if len(f.events) != 0 {
					t.Fatalf("invalid association wrote remotely: %v", f.events)
				}
			})
		}
	}
}

func TestEmptyMaintenanceAssociationsDoNotDecodeUnusedDirectoryEntries(t *testing.T) {
	f := newResourceValidationFixture(t,
		map[string]json.RawMessage{"bad": json.RawMessage(`{"id":"incompatible"}`)},
		map[string]json.RawMessage{"bad": json.RawMessage(`{"id":"incompatible"}`)},
	)
	if _, err := f.maintenance(nil, nil); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.monitors) != 0 || len(f.pages) != 0 || len(f.events) != 3 {
		t.Fatalf("empty associations changed: %v %v %v", f.monitors, f.pages, f.events)
	}
}

func TestStatusPageAllowsSameMonitorInMultipleDisplayGroups(t *testing.T) {
	f := newResourceValidationFixture(t,
		map[string]json.RawMessage{"remote": json.RawMessage(`{"id":19,"name":"服务"}`)}, map[string]json.RawMessage{},
	)
	groups := []uptimekuma.PublicGroup{
		{Name: "主要服务", MonitorList: []uptimekuma.PublicMonitor{{ID: 19}}},
		{Name: "全部服务", MonitorList: []uptimekuma.PublicMonitor{{ID: 19}}},
	}
	result, err := f.statusPage(groups)
	if err != nil || result["resource_id"] != int64(51) {
		t.Fatalf("status page failed: %v, %v", result, err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !reflect.DeepEqual(f.groups, groups) {
		t.Fatalf("duplicate display membership was removed: %+v", f.groups)
	}
}

func TestStatusPageMissingMonitorRejectsBeforeCreatingPage(t *testing.T) {
	f := newResourceValidationFixture(t, map[string]json.RawMessage{}, map[string]json.RawMessage{})
	_, err := f.statusPage([]uptimekuma.PublicGroup{{Name: "服务", MonitorList: []uptimekuma.PublicMonitor{{ID: 19}}}})
	if err == nil || uptimekuma.PublicError(err).Code != "kuma_invalid_resource_ids" {
		t.Fatalf("missing monitor accepted: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.events) != 0 {
		t.Fatalf("invalid status page was created: %v", f.events)
	}
}
