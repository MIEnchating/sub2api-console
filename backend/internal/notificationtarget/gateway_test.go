package notificationtarget

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestGatewayListenerCapturesPrivateTargetWithoutReplying(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	mux.HandleFunc("/token", func(writer http.ResponseWriter, request *http.Request) {
		var payload map[string]string
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("token payload: %v", err)
		}
		if payload["appId"] != "app-1" || payload["clientSecret"] != "secret-1" {
			t.Errorf("token payload = %#v", payload)
		}
		writeTestJSON(writer, map[string]any{"access_token": "access-1", "expires_in": 7200})
	})
	mux.HandleFunc("/gateway", func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "QQBot access-1" {
			t.Errorf("gateway authorization = %q", request.Header.Get("Authorization"))
		}
		writeTestJSON(writer, map[string]string{"url": "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"})
	})
	mux.HandleFunc("/ws", func(writer http.ResponseWriter, request *http.Request) {
		connection, err := websocket.Accept(writer, request, nil)
		if err != nil {
			t.Errorf("accept websocket: %v", err)
			return
		}
		defer connection.Close(websocket.StatusNormalClosure, "done")
		ctx := request.Context()
		if err := writeGatewayJSON(ctx, connection, map[string]any{"op": 10, "d": map[string]int{"heartbeat_interval": 45_000}}); err != nil {
			t.Errorf("write hello: %v", err)
			return
		}
		identify, err := readGatewayPacket(ctx, connection)
		if err != nil {
			t.Errorf("read identify: %v", err)
			return
		}
		var identifyData struct {
			Token   string `json:"token"`
			Intents int    `json:"intents"`
		}
		if err := json.Unmarshal(identify.D, &identifyData); err != nil {
			t.Errorf("decode identify: %v", err)
		}
		if identify.Op != 2 || identifyData.Token != "QQBot access-1" || identifyData.Intents != groupAndC2CIntent {
			t.Errorf("identify = %#v data=%#v", identify, identifyData)
		}
		sequence := int64(1)
		if err := writeGatewayJSON(ctx, connection, gatewayPacket{Op: 0, S: &sequence, T: "READY", D: json.RawMessage(`{"session_id":"session-1"}`)}); err != nil {
			t.Errorf("write ready: %v", err)
			return
		}
		sequence++
		if err := writeGatewayJSON(ctx, connection, gatewayPacket{
			Op: 0, S: &sequence, T: "C2C_MESSAGE_CREATE",
			D: json.RawMessage(`{"author":{"user_openid":"user-open-id","username":"测试用户"},"content":"内容不应进入结果"}`),
		}); err != nil {
			t.Errorf("write event: %v", err)
		}
	})

	listener := &GatewayListener{
		client: server.Client(), tokenEndpoint: server.URL + "/token", gatewayEndpoint: server.URL + "/gateway",
		allowInsecureWS: true,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ready := false
	target, err := listener.Listen(ctx, Request{AppID: "app-1", ClientSecret: "secret-1", TargetType: "c2c"}, func() { ready = true })
	if err != nil {
		t.Fatal(err)
	}
	if !ready {
		t.Fatal("ready callback was not called")
	}
	if target.ID != "user-open-id" || target.Type != "c2c" || target.EventType != "C2C_MESSAGE_CREATE" || target.SourceName != "测试用户" {
		t.Fatalf("target = %#v", target)
	}
}

func TestGatewayListenerCopiesClientAndRejectsRedirects(t *testing.T) {
	originalRedirect := func(*http.Request, []*http.Request) error { return nil }
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: originalRedirect}
	listener := NewGatewayListener(client)

	if listener.client == client || listener.client.Timeout != client.Timeout {
		t.Fatalf("listener did not copy the supplied client: listener=%p client=%p", listener.client, client)
	}
	if client.CheckRedirect == nil || client.CheckRedirect(nil, nil) != nil {
		t.Fatal("constructor mutated the caller-owned redirect policy")
	}
	if err := listener.client.CheckRedirect(nil, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("listener follows credential-bearing redirects: %v", err)
	}
}

func TestGatewayListenerRejectsOversizedHTTPResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"access_token":"access-1"}` + strings.Repeat(" ", maximumGatewayResponseSize)))
	}))
	t.Cleanup(server.Close)
	listener := NewGatewayListener(server.Client())
	listener.tokenEndpoint = server.URL

	_, err := listener.accessToken(context.Background(), Request{AppID: "app", ClientSecret: "secret"})
	if err == nil || err.Error() != "QQBot 鉴权响应过大" {
		t.Fatalf("oversized response was accepted: %v", err)
	}
}

func TestTargetFromEventOnlyAcceptsTheSelectedTargetType(t *testing.T) {
	tests := []struct {
		name       string
		targetType string
		eventType  string
		payload    string
		wantID     string
	}{
		{name: "private", targetType: "c2c", eventType: "C2C_MESSAGE_CREATE", payload: `{"author":{"user_openid":"user-1"}}`, wantID: "user-1"},
		{name: "group", targetType: "group", eventType: "GROUP_AT_MESSAGE_CREATE", payload: `{"group_openid":"group-1"}`, wantID: "group-1"},
		{name: "channel", targetType: "channel", eventType: "AT_MESSAGE_CREATE", payload: `{"channel_id":"channel-1"}`, wantID: "channel-1"},
		{name: "wrong event", targetType: "c2c", eventType: "GROUP_AT_MESSAGE_CREATE", payload: `{"group_openid":"group-1"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target, found := targetFromEvent(test.targetType, gatewayPacket{T: test.eventType, D: json.RawMessage(test.payload)})
			if found != (test.wantID != "") || target.ID != test.wantID {
				t.Fatalf("found=%v target=%#v", found, target)
			}
		})
	}
}

func writeTestJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}
