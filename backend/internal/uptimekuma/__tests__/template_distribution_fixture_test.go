package uptimekuma_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/uptimekuma"
	"github.com/coder/websocket"
)

type distributionPeer struct {
	blockedID  int64
	entered    chan struct{}
	stopped    chan struct{}
	mu         sync.Mutex
	monitors   map[string]json.RawMessage
	writes     []int64
	failID     int64
	afterWrite func(int64)
}

func (p *distributionPeer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	send := func(packet string) bool { return conn.Write(ctx, websocket.MessageText, []byte(packet)) == nil }
	if !send(`0{"sid":"distribution-test","upgrades":[],"pingInterval":25000,"pingTimeout":20000}`) {
		return
	}
	_, packet, err := conn.Read(ctx)
	if err != nil || string(packet) != "40" || !send(`40{"sid":"distribution-test"}`) {
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
		if start < 3 || json.Unmarshal(packet[start:], &args) != nil {
			return
		}
		var event string
		if json.Unmarshal(args[0], &event) != nil {
			return
		}
		p.mu.Lock()
		reply := map[string]any{"ok": true}
		switch event {
		case "loginByToken":
		case "getMonitorList":
			list, _ := json.Marshal([]any{"monitorList", p.monitors})
			send("42" + string(list))
		case "getMonitor":
			var id int64
			_ = json.Unmarshal(args[1], &id)
			reply["monitor"] = p.monitors[strconv.FormatInt(id, 10)]
		case "editMonitor":
			var monitor struct {
				ID int64 `json:"id"`
			}
			_ = json.Unmarshal(args[1], &monitor)
			p.writes = append(p.writes, monitor.ID)
			if monitor.ID == p.blockedID {
				p.mu.Unlock()
				close(p.entered)
				_, _, _ = conn.Read(ctx)
				close(p.stopped)
				return
			}
			if monitor.ID == p.failID {
				reply["ok"] = false
			} else {
				p.monitors[strconv.FormatInt(monitor.ID, 10)] = append(json.RawMessage(nil), args[1]...)
				if p.afterWrite != nil {
					p.afterWrite(monitor.ID)
				}
			}
		default:
			reply["ok"] = false
		}
		body, _ := json.Marshal([]any{reply})
		p.mu.Unlock()
		if !send("43" + text[2:start] + string(body)) {
			return
		}
	}
}

func distributionFixture(t *testing.T) (*kumaTaskFixture, *distributionPeer, uptimekuma.TemplateSummary) {
	t.Helper()
	peer := &distributionPeer{monitors: map[string]json.RawMessage{}}
	for _, id := range []int64{1, 2, 3} {
		data, _ := json.Marshal(map[string]any{
			"id": id, "name": "监控 " + strconv.FormatInt(id, 10), "type": "http", "interval": 60, "timeout": 16, "active": false, "parent": 99,
			"url": "https://isolated.example.test/health?key=private-url", "method": "GET", "authMethod": "bearer", "bearer_token": "private-token",
			"headers": `{"Authorization":"private-header","X-Old":"old"}`, "body": `{"model":"old"}`, "httpBodyEncoding": "json",
			"notificationIDList": map[string]bool{"5": true, "7": false}, "customOption": "keep",
		})
		peer.monitors[strconv.FormatInt(id, 10)] = data
	}
	f := newKumaTaskFixture(t, peer)
	configureKumaTaskManagement(t, f)
	input := uptimekuma.TemplateInput{Name: "探活模板", Method: "POST", AuthMethod: "bearer", AuthPassword: "private-template-token", Headers: `{"X-New":"new","Authorization":"private-template-header"}`, Body: `{"model":"template-model","max_tokens":32}`, BodyEncoding: "json", Monitoring: &configstore.KumaTemplateMonitoring{Type: "http", URL: "https://template.example.test", Interval: 300, Timeout: 90, RetryInterval: 120, MaxRetries: 3, MaxRedirects: 4, AcceptedStatusCodes: []string{"200-299"}}}
	template, err := f.service.Service.SaveTemplate(f.ctx, "", input)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{1, 2} {
		model := ""
		if id == 1 {
			model = "monitor-model"
		}
		if err := f.config.SaveKumaMonitorTemplate(f.ctx, f.server.URL, configstore.KumaMonitorTemplate{MonitorID: id, TemplateID: template.ID, Revision: template.Revision, Name: template.Name, Model: model, BodyEncoding: "json"}); err != nil {
			t.Fatal(err)
		}
	}
	input.Revision = template.Revision
	template, err = f.service.Service.SaveTemplate(f.ctx, template.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	return f, peer, template
}

func distributionPreview(t *testing.T, f *kumaTaskFixture, template uptimekuma.TemplateSummary) uptimekuma.TemplateDistributionPreview {
	t.Helper()
	preview, err := f.service.Service.PreviewTemplateDistribution(f.ctx, template.ID, template.Revision)
	if err != nil {
		t.Fatal(err)
	}
	return preview
}

func runDistribution(t *testing.T, f *kumaTaskFixture, preview uptimekuma.TemplateDistributionPreview) {
	t.Helper()
	if _, err := f.service.DistributeTemplate(f.ctx, preview.TemplateID, uptimekuma.TemplateDistributionInput{Revision: preview.Revision, Scope: preview.Scope}); err != nil {
		t.Fatal(err)
	}
}
