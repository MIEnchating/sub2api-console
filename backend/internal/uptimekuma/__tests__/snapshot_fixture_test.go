package uptimekuma_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/uptimekuma"
	"github.com/coder/websocket"
)

const snapshotMetrics = `# TYPE monitor_status gauge
monitor_status{monitor_id="19",monitor_name="同名服务",monitor_type="http"} 1
monitor_status{monitor_id="99",monitor_name="同名服务",monitor_type="http"} 1
monitor_status{monitor_name="同名服务",monitor_type="http"} 1
# TYPE monitor_response_time gauge
monitor_response_time{monitor_id="19",monitor_name="同名服务",monitor_type="http"} 42
`

const snapshotMonitors = `{"19":{"id":19,"name":"同名服务","type":"http","active":true,"interval":60,"url":"https://monitor.example/health?token=private-secret","headers":"private-secret"},"23":{"id":23,"name":"同名服务","type":"http","active":true,"interval":60}}`

type snapshotPeer struct {
	metrics       http.HandlerFunc
	login         func(context.Context) bool
	list          func(context.Context, *websocket.Conn) bool
	metricsCalls  atomic.Int32
	socketCalls   atomic.Int32
	listCalls     atomic.Int32
	metricsStart  chan struct{}
	metricsFinish chan error
	metricsClosed chan struct{}
	metricsRead   chan struct{}
	socketStopped chan struct{}
}

type snapshotTransport struct {
	base http.RoundTripper
	peer *snapshotPeer
}

func (t snapshotTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(r)
	if r.URL.Path == "/metrics" {
		if response != nil && response.Body != nil {
			response.Body = &snapshotBody{ReadCloser: response.Body, closed: t.peer.metricsClosed, reading: t.peer.metricsRead}
		}
		t.peer.metricsFinish <- r.Context().Err()
	}
	return response, err
}

type snapshotBody struct {
	io.ReadCloser
	closed  chan struct{}
	reading chan struct{}
	started sync.Once
}

func (b *snapshotBody) Read(p []byte) (int, error) {
	b.started.Do(func() { close(b.reading) })
	return b.ReadCloser.Read(p)
}

func (b *snapshotBody) Close() error {
	err := b.ReadCloser.Close()
	close(b.closed)
	return err
}

func newSnapshotFixture(t *testing.T, peer *snapshotPeer, management bool) (*uptimekuma.Service, *configstore.Store, int64) {
	t.Helper()
	peer.metricsStart = make(chan struct{})
	peer.metricsFinish = make(chan error, 1)
	peer.metricsClosed = make(chan struct{})
	peer.metricsRead = make(chan struct{})
	peer.socketStopped = make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(peer.serve))
	t.Cleanup(server.Close)
	store, err := configstore.Open(filepath.Join(t.TempDir(), "isolated-snapshot.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	config := configstore.UptimeKumaConfig{BaseURL: server.URL, APIKey: "isolated-key"}
	if management {
		config.Token = "isolated-token"
	}
	if err := store.SaveUptimeKuma(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	saved, err := store.UptimeKuma(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveKumaMonitorTemplate(context.Background(), server.URL, configstore.KumaMonitorTemplate{
		MonitorID: 19, TemplateID: "isolated-template", Revision: 3, Name: "请求模板",
	}); err != nil {
		t.Fatal(err)
	}
	client := server.Client()
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client.Transport = snapshotTransport{base: transport, peer: peer}
	return uptimekuma.New(store, client), store, saved.Revision
}

func (p *snapshotPeer) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/metrics" {
		p.metricsCalls.Add(1)
		close(p.metricsStart)
		user, password, ok := r.BasicAuth()
		if !ok || user != "" || password != "isolated-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if p.metrics != nil {
			p.metrics(w, r)
		} else {
			_, _ = w.Write([]byte(snapshotMetrics))
		}
		return
	}
	if r.URL.Path != "/socket.io/" {
		http.NotFound(w, r)
		return
	}
	p.socketCalls.Add(1)
	defer close(p.socketStopped)
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	send := func(s string) bool { return conn.Write(ctx, websocket.MessageText, []byte(s)) == nil }
	if !send(`0{"sid":"snapshot","upgrades":[],"pingInterval":25000,"pingTimeout":20000}`) {
		return
	}
	_, packet, err := conn.Read(ctx)
	if err != nil || string(packet) != "40" || !send(`40{"sid":"snapshot"}`) {
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
		reply := `[{"ok":true}]`
		switch event {
		case "loginByToken":
			var token string
			if len(args) != 2 || json.Unmarshal(args[1], &token) != nil || token != "isolated-token" {
				return
			}
			if p.login != nil && !p.login(ctx) {
				reply = `[{"ok":false}]`
			}
		case "getMonitorList":
			p.listCalls.Add(1)
			if p.list != nil && !p.list(ctx, conn) {
				return
			}
			if !send(`42["monitorList",`+snapshotMonitors+`]`) ||
				!send(`42["heartbeatList","19",[{"status":0}]]`) ||
				!send(`42["heartbeatList","23",[{"status":0}]]`) {
				return
			}
		default:
			return
		}
		if !send("43" + text[2:start] + reply) {
			return
		}
	}
}

func snapshotTestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func waitSnapshotEvent(t *testing.T, ctx context.Context, event <-chan struct{}) {
	t.Helper()
	select {
	case <-event:
	case <-ctx.Done():
		t.Fatal("controlled snapshot request did not reach the expected state")
	}
}
