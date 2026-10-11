package uptimekuma_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/uptimekuma"
)

func detailFixture(t testing.TB, kind string, count int, extra json.RawMessage) *resourceValidationFixture {
	t.Helper()
	objects := map[string]json.RawMessage{}
	notifications := []json.RawMessage{}
	for id := 1; id <= count; id++ {
		key := strconv.Itoa(id)
		var item any = map[string]any{"id": id, "title": "资源", "slug": "isolated-status", "strategy": "manual", "active": true, "customCSS": strings.Repeat("x", 1024)}
		if kind == "notifications" {
			item = map[string]any{"id": id, "name": "通知", "active": 1, "config": `{"type":"webhook","webhookURL":"https://isolated.test/private-secret","extra":"` + strings.Repeat("x", 1024) + `"}`}
		}
		raw, err := json.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		if kind == "status-pages" {
			key = "slug-" + key
		}
		objects[key] = raw
		notifications = append(notifications, raw)
	}
	event := "maintenanceList"
	var data any = objects
	if kind == "notifications" {
		event = "notificationList"
		if extra != nil {
			notifications = append(notifications, extra)
		}
		data = notifications
	} else {
		if kind == "status-pages" {
			event = "statusPageList"
		}
		if extra != nil {
			objects["99"] = extra
		}
	}
	return newResourceValidationFixture(t, map[string]json.RawMessage{}, map[string]json.RawMessage{}, []any{event, data})
}

func TestResourceDetailMatchesListRevisionAndHidesCredentials(t *testing.T) {
	for _, kind := range []string{"notifications", "maintenance", "status-pages"} {
		t.Run(kind, func(t *testing.T) {
			f := detailFixture(t, kind, 3, nil)
			list, err := f.service.Resources(context.Background(), kind)
			if err != nil {
				t.Fatal(err)
			}
			detail, err := f.service.Resource(context.Background(), kind, 2)
			if err != nil {
				t.Fatal(err)
			}
			if detail.ID != 2 || detail.Revision != list.Items[1].Revision || detail.Name != list.Items[1].Name {
				t.Fatalf("detail/list mismatch: %+v", detail)
			}
			if kind == "maintenance" && (!reflect.DeepEqual(detail.Maintenance.MonitorIDs, []int64{19}) || !reflect.DeepEqual(detail.Maintenance.StatusPageIDs, []int64{51})) {
				t.Fatalf("lost associations: %+v", detail.Maintenance)
			}
			raw, _ := json.Marshal(detail)
			if strings.Contains(string(raw), "private-secret") {
				t.Fatal("credential exposed")
			}
		})
	}
}

func TestResourceDetailValidatesUnrelatedEntriesBeforeReturning(t *testing.T) {
	for _, tc := range []struct {
		kind string
		raw  string
	}{
		{"notifications", `{"id":99,"config":"[]"}`},
		{"notifications", `{"id":99,"config":"null"}`},
		{"notifications", `{"id":99,"config":"invalid"}`},
		{"notifications", `{"id":0,"config":"{}"}`},
		{"maintenance", `{"id":98}`},
		{"maintenance", `{"id":0}`},
		{"status-pages", `{"id":"invalid"}`},
		{"status-pages", `null`},
	} {
		t.Run(tc.kind+"/"+tc.raw, func(t *testing.T) {
			f := detailFixture(t, tc.kind, 1, json.RawMessage(tc.raw))
			_, err := f.service.Resource(context.Background(), tc.kind, 1)
			if err == nil || uptimekuma.PublicError(err).Code != "kuma_protocol_error" {
				t.Fatalf("invalid directory accepted: %v", err)
			}
		})
	}
}

func TestResourceDetailMissingTargetReturnsNotFound(t *testing.T) {
	for _, kind := range []string{"notifications", "maintenance", "status-pages"} {
		for _, count := range []int{0, 1} {
			t.Run(fmt.Sprintf("%s/%d", kind, count), func(t *testing.T) {
				f := detailFixture(t, kind, count, nil)
				_, err := f.service.Resource(context.Background(), kind, 23)
				if err == nil || uptimekuma.PublicError(err).Code != "kuma_resource_missing" {
					t.Fatalf("missing target accepted: %v", err)
				}
			})
		}
	}
}

func BenchmarkResourceDetail(b *testing.B) {
	for _, kind := range []string{"notifications", "maintenance", "status-pages"} {
		for _, count := range []int{1, 1000} {
			b.Run(fmt.Sprintf("%s/%d", kind, count), func(b *testing.B) {
				f := detailFixture(b, kind, count, nil)
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					if _, err := f.service.Resource(context.Background(), kind, 1); err != nil {
						b.Fatal(err)
					}
					f.mu.Lock()
					f.events = f.events[:0]
					f.mu.Unlock()
				}
			})
		}
	}
}
