package uptimekuma

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/coder/websocket"
)

// The fixture exposes only an explicitly created loopback HTTP/WebSocket server.
// It speaks Kuma's Engine.IO 4 handshake and Socket.IO acknowledgements; no
// production endpoint, credential store or database is used by these tests.
type kumaFixture struct {
	resourceEvents                                      map[string]any
	resourceHandler                                     func(string, []json.RawMessage) map[string]any
	publicGroups                                        []PublicGroup
	initialEvents                                       []string
	loginEvents                                         []string
	mu                                                  sync.Mutex
	server                                              *httptest.Server
	store                                               *configstore.Store
	service                                             *Service
	monitors                                            map[string]json.RawMessage
	rejectMetrics, rejectWrites, otpRequired, dropWrite bool
	writeCount                                          int
	edited                                              map[string]json.RawMessage
}

func newFixture(t *testing.T) *kumaFixture {
	t.Helper()
	store, err := configstore.Open(filepath.Join(t.TempDir(), "test-config.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	f := &kumaFixture{store: store, monitors: map[string]json.RawMessage{"19": json.RawMessage(`{"id":19,"name":"智谱","type":"http","url":"https://monitor.example/v1/responses?api_key=private-url-key","active":true,"parent":null,"interval":300,"method":"POST","body":"private-body","headers":"private-headers","accepted_statuscodes":["200-299"],"notificationIDList":{"7":true}}`)}}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	f.service = New(store, f.server.Client())
	t.Cleanup(func() { f.server.Close(); store.Close() })
	return f
}
func (f *kumaFixture) serve(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/status-page/") {
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"publicGroupList": f.publicGroups})
		return
	}
	if r.URL.Path == "/metrics" {
		f.mu.Lock()
		reject := f.rejectMetrics
		f.mu.Unlock()
		u, p, ok := r.BasicAuth()
		if !ok || u != "" || p != "test-key" || reject {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "# HELP monitor_status Monitor Status\n# TYPE monitor_status gauge\nmonitor_status{monitor_id=\"19\",monitor_name=\"智谱\",monitor_type=\"http\"} 1\n")
		return
	}
	if r.URL.Path != "/socket.io/" || r.URL.Query().Get("EIO") != "4" || r.URL.Query().Get("transport") != "websocket" {
		http.NotFound(w, r)
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	send := func(p string) bool { return conn.Write(ctx, websocket.MessageText, []byte(p)) == nil }
	if !send(`0{"sid":"local-test","upgrades":[],"pingInterval":25000,"pingTimeout":20000,"maxPayload":1000000}`) {
		return
	}
	_, b, err := conn.Read(ctx)
	if err != nil || string(b) != "40" {
		return
	}
	if !send(`40{"sid":"local-test"}`) {
		return
	}
	for _, packet := range f.initialEvents {
		if !send(packet) {
			return
		}
	}
	authenticated := false
	for {
		_, b, err = conn.Read(ctx)
		if err != nil {
			return
		}
		p := string(b)
		if p == "3" {
			continue
		}
		start := strings.IndexByte(p, '[')
		if !strings.HasPrefix(p, "42") || start < 3 {
			return
		}
		id := p[2:start]
		var args []json.RawMessage
		if json.Unmarshal([]byte(p[start:]), &args) != nil || len(args) == 0 {
			return
		}
		var event string
		json.Unmarshal(args[0], &event)
		response := map[string]any{"ok": true}
		f.mu.Lock()
		switch event {
		case "login":
			var in struct{ Username, Password, Token string }
			json.Unmarshal(args[1], &in)
			if f.otpRequired && in.Token == "" {
				response = map[string]any{"tokenRequired": true}
			} else if in.Username != "admin" || in.Password != "test-password" || (f.otpRequired && in.Token != "123456") {
				response = map[string]any{"ok": false, "msg": "private-upstream-secret"}
			} else {
				response["token"] = "test-jwt"
				authenticated = true
			}
		case "loginByToken":
			var token string
			json.Unmarshal(args[1], &token)
			authenticated = token == "test-jwt"
			response["ok"] = authenticated
		default:
			if !authenticated {
				f.mu.Unlock()
				return
			}
			switch event {
			case "getMonitorList":
				payload, _ := json.Marshal([]any{"monitorList", f.monitors})
				if !send("42" + string(payload)) {
					f.mu.Unlock()
					return
				}
			case "getMonitor":
				var monitorID int64
				json.Unmarshal(args[1], &monitorID)
				monitor, ok := f.monitors[strconv.FormatInt(monitorID, 10)]
				response["ok"] = ok
				response["monitor"] = monitor
			case "editMonitor", "add", "pauseMonitor", "resumeMonitor", "deleteMonitor":
				f.writeCount++
				if f.dropWrite {
					f.mu.Unlock()
					return
				}
				if f.rejectWrites {
					response = map[string]any{"ok": false, "msg": "private-upstream-secret"}
				} else if event == "add" {
					json.Unmarshal(args[1], &f.edited)
					response["monitorID"] = 20
				} else if event == "editMonitor" {
					json.Unmarshal(args[1], &f.edited)
				}
			default:
				if f.resourceHandler != nil {
					response = f.resourceHandler(event, args)
				} else {
					response = map[string]any{"ok": false}
				}
			}
		}
		if authenticated && (event == "login" || event == "loginByToken") {
			for name, data := range f.resourceEvents {
				payload, _ := json.Marshal([]any{name, data})
				if !send("42" + string(payload)) {
					f.mu.Unlock()
					return
				}
			}
		}
		f.mu.Unlock()
		if authenticated && (event == "login" || event == "loginByToken") {
			for _, packet := range f.loginEvents {
				if !send(packet) {
					return
				}
			}
		}
		// Heartbeats may arrive between request and acknowledgement.
		if !send("2") {
			return
		}
		payload, _ := json.Marshal([]any{response})
		if !send("43" + id + string(payload)) {
			return
		}
	}
}
func (f *kumaFixture) configure(t *testing.T, management bool) Config {
	t.Helper()
	in := ConfigInput{BaseURL: f.server.URL + "/dashboard", APIKey: "test-key"}
	if management {
		in.Username = "admin"
		in.Password = "test-password"
	}
	c, err := f.service.Save(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func TestSaveVerifiesCredentialsAndNeverReturnsSecrets(t *testing.T) {
	f := newFixture(t)
	c := f.configure(t, true)
	if c.BaseURL != f.server.URL || !c.ManagementConfigured || !c.APIKeyConfigured {
		t.Fatalf("config: %#v", c)
	}
	data, _ := json.Marshal(c)
	for _, secret := range []string{"test-key", "test-password", "test-jwt"} {
		if strings.Contains(string(data), secret) {
			t.Fatal("configuration leaked credentials")
		}
	}
	updated, err := f.service.Save(context.Background(), ConfigInput{BaseURL: c.BaseURL, Username: "admin", Revision: c.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 || !updated.ManagementConfigured {
		t.Fatalf("blank secrets were not preserved")
	}
}
func TestSaveRejectsBadKeyWithoutReplacingConfiguration(t *testing.T) {
	f := newFixture(t)
	c := f.configure(t, false)
	_, err := f.service.Save(context.Background(), ConfigInput{BaseURL: c.BaseURL, APIKey: "bad-key", Revision: c.Revision})
	if PublicError(err).Code != "kuma_api_key_rejected" {
		t.Fatalf("error: %v", err)
	}
	stored, _ := f.store.UptimeKuma(context.Background())
	if stored.APIKey != "test-key" || stored.Revision != c.Revision {
		t.Fatal("failed verification replaced configuration")
	}
}
func TestChangingAddressRequiresNewCredentialsBeforeNetworkAccess(t *testing.T) {
	f := newFixture(t)
	c := f.configure(t, true)
	_, err := f.service.Save(context.Background(), ConfigInput{BaseURL: "https://different.invalid", Username: "admin", Revision: c.Revision})
	if PublicError(err).Code != "kuma_key_required" {
		t.Fatalf("error: %v", err)
	}
}
func TestTwoFactorLoginStoresOnlySessionAndNotOTP(t *testing.T) {
	f := newFixture(t)
	f.otpRequired = true
	in := ConfigInput{BaseURL: f.server.URL, APIKey: "test-key", Username: "admin", Password: "test-password"}
	_, err := f.service.Save(context.Background(), in)
	if PublicError(err).Code != "kuma_otp_required" {
		t.Fatalf("error: %v", err)
	}
	in.OTP = "123456"
	if _, err = f.service.Save(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	stored, _ := f.store.UptimeKuma(context.Background())
	if stored.Token != "test-jwt" {
		t.Fatal("missing session")
	}
	if _, err = f.service.Snapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
}
func TestSnapshotUsesStableIDsAndHidesSensitiveMonitorSettings(t *testing.T) {
	f := newFixture(t)
	f.configure(t, true)
	r, err := f.service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Monitors) != 1 || r.Monitors[0].ID != 19 || r.Monitors[0].Status == nil || *r.Monitors[0].Status != 1 {
		t.Fatalf("snapshot: %#v", r)
	}
	data, _ := json.Marshal(r)
	if strings.Contains(string(data), "private-") {
		t.Fatalf("snapshot leaked upstream settings")
	}
}
func TestMetricsFailureRetainsManagementMonitorList(t *testing.T) {
	f := newFixture(t)
	f.configure(t, true)
	f.mu.Lock()
	f.rejectMetrics = true
	f.mu.Unlock()
	r, err := f.service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r.Warning == "" || len(r.Monitors) != 1 {
		t.Fatal("missing management fallback")
	}
}
func TestEditPreservesPrivateSettingsAndBlankURL(t *testing.T) {
	f := newFixture(t)
	c := f.configure(t, true)
	snap, err := f.service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.Write(context.Background(), 19, WriteInput{Action: "edit", ConfigRevision: c.Revision, Revision: snap.Monitors[0].Revision, Monitor: MonitorInput{Name: "改名", Type: "http", Interval: 60}})
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for key, want := range map[string]string{"method": `"POST"`, "body": `"private-body"`, "headers": `"private-headers"`, "notificationIDList": `{"7":true}`, "url": `"https://monitor.example/v1/responses?api_key=private-url-key"`, "name": `"改名"`, "interval": `60`} {
		if string(f.edited[key]) != want {
			t.Fatalf("edit did not preserve or update %s", key)
		}
	}
}
func TestStaleMonitorAndReadOnlyCredentialsCannotWrite(t *testing.T) {
	for _, management := range []bool{false, true} {
		t.Run(strconv.FormatBool(management), func(t *testing.T) {
			f := newFixture(t)
			c := f.configure(t, management)
			_, err := f.service.Write(context.Background(), 19, WriteInput{Action: "delete", ConfigRevision: c.Revision, Revision: "stale"})
			if err == nil {
				t.Fatal("unsafe write accepted")
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.writeCount != 0 {
				t.Fatal("write reached upstream")
			}
		})
	}
}
func TestWriteAcknowledgementFailuresAreNotRetriedOrLeaked(t *testing.T) {
	for _, drop := range []bool{false, true} {
		t.Run(strconv.FormatBool(drop), func(t *testing.T) {
			f := newFixture(t)
			c := f.configure(t, true)
			snap, err := f.service.Snapshot(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			f.mu.Lock()
			f.dropWrite = drop
			f.rejectWrites = !drop
			f.mu.Unlock()
			_, err = f.service.Write(context.Background(), 19, WriteInput{Action: "pause", ConfigRevision: c.Revision, Revision: snap.Monitors[0].Revision})
			if err == nil || strings.Contains(err.Error(), "private-") {
				t.Fatalf("error: %v", err)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.writeCount != 1 {
				t.Fatalf("write retried %d times", f.writeCount)
			}
		})
	}
}
func TestCreateAndResumeAndDeleteUseAcknowledgedStableIDs(t *testing.T) {
	for _, action := range []string{"create", "resume", "delete"} {
		t.Run(action, func(t *testing.T) {
			f := newFixture(t)
			c := f.configure(t, true)
			snap, err := f.service.Snapshot(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			id := int64(19)
			if action == "create" {
				id = 0
			}
			got, err := f.service.Write(context.Background(), id, WriteInput{Action: action, ConfigRevision: c.Revision, Revision: snap.Monitors[0].Revision, Monitor: MonitorInput{Name: "新监控", Type: "http", URL: "https://test.example/health", Interval: 60}})
			if err != nil {
				t.Fatal(err)
			}
			want := int64(19)
			if action == "create" {
				want = 20
			}
			if got != want {
				t.Fatalf("id=%d", got)
			}
		})
	}
}

func TestCredentialRequestsDoNotFollowRedirects(t *testing.T) {
	f := newFixture(t)
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, f.server.URL+"/metrics", http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	_, err := f.service.Save(context.Background(), ConfigInput{BaseURL: redirect.URL, APIKey: "test-key"})
	if err == nil || PublicError(err).Code != "kuma_metrics_unavailable" {
		t.Fatalf("credential request followed redirect: %v", err)
	}
	stored, _ := f.store.UptimeKuma(context.Background())
	if stored.APIKey != "" {
		t.Fatal("unverified redirect endpoint was saved")
	}
}

func TestNonEmptyGroupCannotBeDeleted(t *testing.T) {
	f := newFixture(t)
	f.monitors["7"] = json.RawMessage(`{"id":7,"name":"国模","type":"group","url":"","active":true,"parent":null,"interval":60}`)
	f.monitors["19"] = json.RawMessage(`{"id":19,"name":"智谱","type":"http","url":"https://monitor.example","active":true,"parent":7,"interval":300}`)
	c := f.configure(t, true)
	snap, err := f.service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.Write(context.Background(), 7, WriteInput{Action: "delete", ConfigRevision: c.Revision, Revision: snap.Monitors[0].Revision})
	if PublicError(err).Code != "kuma_group_not_empty" {
		t.Fatalf("error: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.writeCount != 0 {
		t.Fatal("group deletion reached upstream")
	}
}

func TestDisablingManagementRemovesOnlyManagementSecrets(t *testing.T) {
	f := newFixture(t)
	c := f.configure(t, true)
	c, err := f.service.Save(context.Background(), ConfigInput{BaseURL: c.BaseURL, Revision: c.Revision, DisableManagement: true})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := f.store.UptimeKuma(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if c.ManagementConfigured || !c.APIKeyConfigured || stored.Password != "" || stored.Token != "" || stored.Username != "" {
		t.Fatal("management credentials were retained")
	}
}

func TestMalformedMonitorListCannotProduceManagementTargets(t *testing.T) {
	for _, data := range []string{`{"id":20,"name":"wrong ID","type":"http"}`, `{"id":19,"type":"http"}`} {
		t.Run(data, func(t *testing.T) {
			f := newFixture(t)
			f.monitors["19"] = json.RawMessage(data)
			f.configure(t, true)
			_, err := f.service.Snapshot(context.Background())
			if PublicError(err).Code != "kuma_protocol_error" {
				t.Fatalf("error: %v", err)
			}
		})
	}
}

func TestInvalidMonitorParametersAreRejectedBeforeConnecting(t *testing.T) {
	for _, in := range []WriteInput{
		{Action: "unsupported"},
		{Action: "create", Monitor: MonitorInput{Name: "", Type: "http", Interval: 60}},
		{Action: "create", Monitor: MonitorInput{Name: "测试", Type: "http", Interval: 19}},
		{Action: "create", Monitor: MonitorInput{Name: "测试", Type: "dns", Interval: 60}},
		{Action: "create", Monitor: MonitorInput{Name: "测试", Type: "http", Interval: 60, URL: "file:///etc/passwd"}},
		{Action: "create", Monitor: MonitorInput{Name: "测试", Type: "http", Interval: 60}},
	} {
		f := newFixture(t)
		_, err := f.service.Write(context.Background(), 0, in)
		if err == nil || PublicError(err).Status != 422 {
			t.Fatalf("invalid input was not rejected: %v", err)
		}
	}
}

func TestChangingInstanceDoesNotReuseManagementPassword(t *testing.T) {
	f := newFixture(t)
	other := newFixture(t)
	c := f.configure(t, true)
	_, err := f.service.Save(context.Background(), ConfigInput{BaseURL: other.server.URL, APIKey: "test-key", Username: "admin", Revision: c.Revision})
	if PublicError(err).Code != "kuma_credentials_required" {
		t.Fatalf("credentials were reused for another instance: %v", err)
	}
	stored, _ := f.store.UptimeKuma(context.Background())
	if stored.BaseURL != c.BaseURL {
		t.Fatal("failed instance change was saved")
	}
}

func TestEditingMonitorInGroupReplacesOnlyExplicitURLAndBasicFields(t *testing.T) {
	f := newFixture(t)
	f.monitors["7"] = json.RawMessage(`{"id":7,"name":"分组","type":"group","active":true,"interval":60,"parent":null}`)
	f.monitors["19"] = json.RawMessage(`{"id":19,"name":"智谱","type":"http","active":true,"interval":60,"parent":7,"url":"https://old.example","headers":"private-headers"}`)
	c := f.configure(t, true)
	snapshot, err := f.service.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	parent := int64(7)
	_, err = f.service.Write(context.Background(), 19, WriteInput{Action: "edit", Revision: snapshot.Monitors[1].Revision, ConfigRevision: c.Revision, Monitor: MonitorInput{Name: "智谱", Type: "http", Interval: 60, Parent: &parent, URL: "https://new.example/health"}})
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if string(f.edited["url"]) != `"https://new.example/health"` || string(f.edited["headers"]) != `"private-headers"` || string(f.edited["parent"]) != "7" {
		t.Fatal("group monitor edit overwrote unrelated settings")
	}
}
