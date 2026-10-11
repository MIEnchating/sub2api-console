package uptimekuma_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/uptimekuma"
	"github.com/coder/websocket"
)

// Only the external Kuma endpoint is replaced. Validation, configuration reads,
// authentication and resource writes use the real service and Socket.IO client.
type resourceValidationFixture struct {
	service  *uptimekuma.Service
	revision int64
	mu       sync.Mutex
	events   []string
	monitors []uptimekuma.IDReference
	pages    []uptimekuma.IDReference
	groups   []uptimekuma.PublicGroup
}

func newResourceValidationFixture(t testing.TB, monitors, pages map[string]json.RawMessage, extraEvents ...[]any) *resourceValidationFixture {
	t.Helper()
	f := &resourceValidationFixture{}
	monitorEvent, err := json.Marshal([]any{"monitorList", monitors})
	if err != nil {
		t.Fatal(err)
	}
	pageEvent, err := json.Marshal([]any{"statusPageList", pages})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/status-page/isolated-status" {
			_, _ = w.Write([]byte(`{"publicGroupList":[]}`))
			return
		}
		if r.URL.Path != "/socket.io/" {
			http.NotFound(w, r)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		send := func(p string) bool {
			return conn.Write(ctx, websocket.MessageText, []byte(p)) == nil
		}
		if !send(`0{"sid":"resource-validation","upgrades":[],"pingInterval":25000,"pingTimeout":20000}`) {
			return
		}
		_, packet, err := conn.Read(ctx)
		if err != nil || string(packet) != "40" || !send(`40{"sid":"resource-validation"}`) {
			return
		}
		authenticated := false
		for {
			_, packet, err = conn.Read(ctx)
			if err != nil {
				return
			}
			text := string(packet)
			start := strings.IndexByte(text, '[')
			var args []json.RawMessage
			if !strings.HasPrefix(text, "42") || start < 3 || json.Unmarshal(packet[start:], &args) != nil || len(args) < 2 {
				return
			}
			var event string
			if json.Unmarshal(args[0], &event) != nil {
				return
			}
			reply := map[string]any{"ok": true}
			if event == "loginByToken" {
				var token string
				if json.Unmarshal(args[1], &token) != nil || token != "isolated-token" {
					return
				}
				authenticated = true
				if !send("42"+string(monitorEvent)) || !send("42"+string(pageEvent)) {
					return
				}
				for _, event := range extraEvents {
					body, marshalErr := json.Marshal(event)
					if marshalErr != nil || !send("42"+string(body)) {
						return
					}
				}
			} else {
				if !authenticated {
					return
				}
				f.mu.Lock()
				f.events = append(f.events, event)
				switch event {
				case "getMonitorMaintenance":
					reply["monitors"] = []uptimekuma.IDReference{{ID: 19}}
				case "getMaintenanceStatusPage":
					reply["statusPages"] = []uptimekuma.IDReference{{ID: 51}}
				case "addMaintenance":
					reply["maintenanceID"] = 37
				case "addMonitorMaintenance":
					err = json.Unmarshal(args[2], &f.monitors)
				case "addMaintenanceStatusPage":
					err = json.Unmarshal(args[2], &f.pages)
				case "addStatusPage":
				case "getStatusPage":
					reply["config"] = map[string]any{"id": 51, "slug": "isolated-status"}
				case "saveStatusPage":
					err = json.Unmarshal(args[4], &f.groups)
				default:
					reply["ok"] = false
				}
				f.mu.Unlock()
				if err != nil {
					return
				}
			}
			body, err := json.Marshal([]any{reply})
			if err != nil || !send("43"+text[2:start]+string(body)) {
				return
			}
		}
	}))
	t.Cleanup(server.Close)
	store, err := configstore.Open(filepath.Join(t.TempDir(), "isolated-kuma.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.SaveUptimeKuma(context.Background(), configstore.UptimeKumaConfig{
		BaseURL: server.URL, Token: "isolated-token",
	}); err != nil {
		t.Fatal(err)
	}
	config, err := store.UptimeKuma(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	f.revision = config.Revision
	f.service = uptimekuma.New(store, server.Client())
	return f
}

func (f *resourceValidationFixture) maintenance(monitors, pages []int64) (map[string]any, error) {
	return f.service.WriteResource(context.Background(), "maintenance", 0, uptimekuma.ResourceInput{
		Action: "create", ConfigRevision: f.revision,
		Maintenance: &uptimekuma.MaintenanceConfig{
			Title: "隔离维护", Strategy: "manual", Timezone: "SAME_AS_SERVER",
			MonitorIDs: monitors, StatusPageIDs: pages,
		},
	})
}

func (f *resourceValidationFixture) statusPage(groups []uptimekuma.PublicGroup) (map[string]any, error) {
	return f.service.WriteResource(context.Background(), "status-pages", 0, uptimekuma.ResourceInput{
		Action: "create", ConfigRevision: f.revision,
		StatusPage: &uptimekuma.StatusPageConfig{
			Title: "隔离状态页", Slug: "isolated-status", Theme: "auto", Groups: groups,
		},
	})
}
