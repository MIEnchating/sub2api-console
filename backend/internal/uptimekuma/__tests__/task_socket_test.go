package uptimekuma_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/uptimekuma"
	"github.com/coder/websocket"
)

// A local Socket.IO peer acknowledges login and, for maintenance, the first
// write. The selected write stays unacknowledged until the client disconnects.
func pendingKumaWrite(blockedEvent string, entered, stopped chan struct{}, writes *atomic.Int32) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		send := func(packet string) bool {
			return conn.Write(ctx, websocket.MessageText, []byte(packet)) == nil
		}
		if !send(`0{"sid":"isolated-kuma","upgrades":[],"pingInterval":25000,"pingTimeout":20000}`) {
			return
		}
		_, packet, err := conn.Read(ctx)
		if err != nil || string(packet) != "40" || !send(`40{"sid":"isolated-kuma"}`) {
			return
		}
		for {
			_, packet, err = conn.Read(ctx)
			if err != nil {
				return
			}
			text := string(packet)
			start := strings.IndexByte(text, '[')
			var args []json.RawMessage
			if !strings.HasPrefix(text, "42") || start < 3 || json.Unmarshal(packet[start:], &args) != nil || len(args) == 0 {
				return
			}
			var event string
			if json.Unmarshal(args[0], &event) != nil {
				return
			}
			if event != "loginByToken" {
				writes.Add(1)
			}
			if event == blockedEvent {
				close(entered)
				_, _, _ = conn.Read(ctx)
				close(stopped)
				return
			}
			reply := `[{"ok":true}]`
			if event == "addMaintenance" {
				reply = `[{"ok":true,"maintenanceID":37}]`
			} else if event != "loginByToken" {
				reply = `[{"ok":false}]`
			}
			if !send("43" + text[2:start] + reply) {
				return
			}
		}
	})
}

func configureKumaTaskManagement(t *testing.T, f *kumaTaskFixture) int64 {
	t.Helper()
	if err := f.config.SaveUptimeKuma(f.ctx, configstore.UptimeKumaConfig{
		BaseURL: f.server.URL, APIKey: "private-test-key", Username: "test-admin", Password: "private-password", Token: "private-token",
	}); err != nil {
		t.Fatal(err)
	}
	config, err := f.config.UptimeKuma(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	return config.Revision
}

func cancelKumaWrite(t *testing.T, f *kumaTaskFixture, queued taskstore.Task, entered, stopped <-chan struct{}) taskstore.Task {
	t.Helper()
	awaitKumaEvent(t, f.ctx, entered)
	if accepted, err := f.tasks.store.CancelTask(f.ctx, queued.ID); err != nil || !accepted {
		t.Fatalf("persist cancellation: accepted=%v err=%v", accepted, err)
	}
	if !f.runner.CancelTask(queued.ID) {
		t.Fatal("Kuma write is not registered for cancellation")
	}
	awaitKumaEvent(t, f.ctx, stopped)
	final := f.completed(t)
	if final.Status != "cancelled" || final.Result["phase"] != "cancelled" || !strings.Contains(final.Message, "不会自动撤销") {
		t.Fatalf("cancellation obscures remote outcome: %#v", final)
	}
	encoded, err := json.Marshal(final)
	if err != nil || strings.Contains(string(encoded), "private-") {
		t.Fatal("task contains credentials")
	}
	return final
}

func TestKumaCancelUnacknowledgedWriteClosesSocketWithoutReplaying(t *testing.T) {
	entered, stopped := make(chan struct{}), make(chan struct{})
	var writes atomic.Int32
	f := newKumaTaskFixture(t, pendingKumaWrite("add", entered, stopped, &writes))
	revision := configureKumaTaskManagement(t, f)
	queued, err := f.service.Write(f.ctx, 0, uptimekuma.WriteInput{
		Action: "create", ConfigRevision: revision,
		Monitor: uptimekuma.MonitorInput{Name: "隔离监控", Type: "http", URL: "https://monitor.example.test/health", Interval: 60},
	})
	if err != nil {
		t.Fatal(err)
	}
	final := cancelKumaWrite(t, f, queued, entered, stopped)
	if writes.Load() != 1 || final.Result["remote_write"] == false || final.Result["request_id"] != queued.ID {
		t.Fatalf("unknown write was replayed or labelled unexecuted: %#v, writes=%d", final.Result, writes.Load())
	}
}

func TestKumaCancelPartialResourceWritePreservesAcknowledgedIDAndStopsAssociations(t *testing.T) {
	entered, stopped := make(chan struct{}), make(chan struct{})
	var writes atomic.Int32
	f := newKumaTaskFixture(t, pendingKumaWrite("addMonitorMaintenance", entered, stopped, &writes))
	revision := configureKumaTaskManagement(t, f)
	queued, err := f.service.WriteResource(f.ctx, "maintenance", 0, uptimekuma.ResourceInput{
		Action: "create", ConfigRevision: revision,
		Maintenance: &uptimekuma.MaintenanceConfig{Title: "隔离维护", Strategy: "manual", Timezone: "SAME_AS_SERVER"},
	})
	if err != nil {
		t.Fatal(err)
	}
	final := cancelKumaWrite(t, f, queued, entered, stopped)
	if writes.Load() != 2 || final.Result["saved"] != true || final.Result["resource_id"] != json.Number("37") {
		t.Fatalf("partial result lost or later association submitted: %#v, writes=%d", final.Result, writes.Load())
	}
}
