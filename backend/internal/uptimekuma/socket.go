package uptimekuma

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/coder/websocket"
)

// Kuma exposes management through Socket.IO, not a REST API. This adapter uses
// Engine.IO v4 websocket transport and only the default namespace's JSON events.
// Every connection is bounded by the caller's deadline and is never reconnected
// automatically: replaying an unacknowledged write could duplicate a monitor.
type socket struct {
	notifications []json.RawMessage
	maintenances  map[string]json.RawMessage
	statusPages   map[string]json.RawMessage
	conn          *websocket.Conn
	next          int
	monitors      map[string]json.RawMessage
	statuses      map[int64]int
}
type reply struct {
	ID            int64           `json:"id"`
	MaintenanceID int64           `json:"maintenanceID"`
	Maintenance   json.RawMessage `json:"maintenance"`
	Config        json.RawMessage `json:"config"`
	Monitors      []IDReference   `json:"monitors"`
	StatusPages   []IDReference   `json:"statusPages"`
	OK            bool            `json:"ok"`
	TokenRequired bool            `json:"tokenRequired"`
	Token         string          `json:"token"`
	Monitor       json.RawMessage `json:"monitor"`
	MonitorID     int64           `json:"monitorID"`
}

func dial(ctx context.Context, base string, client *http.Client) (*socket, error) {
	u, _ := url.Parse(base + "/socket.io/")
	q := u.Query()
	q.Set("EIO", "4")
	q.Set("transport", "websocket")
	u.RawQuery = q.Encode()
	conn, _, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{HTTPClient: client})
	if err != nil {
		return nil, failure("kuma_connection_failed", "无法连接 Uptime Kuma，请检查地址及反向代理的 WebSocket 支持", 502)
	}
	s := &socket{conn: conn, statuses: map[int64]int{}}
	conn.SetReadLimit(8 << 20)
	packet, err := s.read(ctx)
	if err != nil || !strings.HasPrefix(packet, "0{") {
		conn.CloseNow()
		return nil, protocolError()
	}
	if err = s.send(ctx, "40"); err != nil {
		conn.CloseNow()
		return nil, err
	}
	for {
		packet, err = s.read(ctx)
		if err != nil {
			conn.CloseNow()
			return nil, err
		}
		if strings.HasPrefix(packet, "40") {
			break
		}
		if strings.HasPrefix(packet, "44") {
			conn.CloseNow()
			return nil, protocolError()
		}
	}
	return s, nil
}
func protocolError() error {
	return failure("kuma_protocol_error", "Uptime Kuma 返回了不兼容的数据，请检查版本和服务地址", 502)
}
func (s *socket) send(ctx context.Context, p string) error {
	if err := s.conn.Write(ctx, websocket.MessageText, []byte(p)); err != nil {
		return failure("kuma_connection_lost", "连接中断，操作结果尚未确认，请刷新监控项后再决定是否重试", 502)
	}
	return nil
}
func (s *socket) read(ctx context.Context) (string, error) {
	for {
		kind, b, err := s.conn.Read(ctx)
		if err != nil {
			return "", failure("kuma_connection_lost", "连接中断或超时，操作结果尚未确认，请刷新监控项后再决定是否重试", 502)
		}
		if kind != websocket.MessageText {
			return "", protocolError()
		}
		p := string(b)
		if p == "2" {
			if err := s.send(ctx, "3"); err != nil {
				return "", err
			}
			continue
		}
		if p == "1" || p == "41" {
			return "", protocolError()
		}
		return p, nil
	}
}
func (s *socket) event(p string) error {
	var event []json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimPrefix(p, "42")), &event); err != nil || len(event) < 1 {
		return protocolError()
	}
	var name string
	if json.Unmarshal(event[0], &name) != nil || name == "" {
		return protocolError()
	}
	switch name {
	case "notificationList":
		if len(event) < 2 || json.Unmarshal(event[1], &s.notifications) != nil || s.notifications == nil {
			return protocolError()
		}
	case "maintenanceList":
		if len(event) < 2 || json.Unmarshal(event[1], &s.maintenances) != nil || s.maintenances == nil {
			return protocolError()
		}
	case "statusPageList":
		if len(event) < 2 || json.Unmarshal(event[1], &s.statusPages) != nil || s.statusPages == nil {
			return protocolError()
		}

	case "monitorList":
		if len(event) < 2 {
			return protocolError()
		}
		if json.Unmarshal(event[1], &s.monitors) != nil || s.monitors == nil {
			return protocolError()
		}
	case "heartbeatList":
		if len(event) < 3 {
			return protocolError()
		}
		id, err := heartbeatMonitorID(event[1])
		if err != nil {
			return err
		}
		var beats []struct {
			Status int `json:"status"`
		}
		if json.Unmarshal(event[2], &beats) != nil {
			return protocolError()
		}
		if len(beats) > 0 {
			s.statuses[id] = beats[len(beats)-1].Status
		}
	}
	return nil
}

// Kuma's afterLogin iterates monitorList object keys, so heartbeat IDs arrive
// as decimal strings; events from other callers can use numeric IDs.
func heartbeatMonitorID(raw json.RawMessage) (int64, error) {
	var id int64
	if json.Unmarshal(raw, &id) == nil && id > 0 {
		return id, nil
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return 0, protocolError()
	}
	id, err := strconv.ParseInt(text, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != text {
		return 0, protocolError()
	}
	return id, nil
}
func (s *socket) call(ctx context.Context, name string, args ...any) (reply, error) {
	s.next++
	id := strconv.Itoa(s.next)
	payload := append([]any{name}, args...)
	b, err := json.Marshal(payload)
	if err != nil {
		return reply{}, protocolError()
	}
	if err = s.send(ctx, "42"+id+string(b)); err != nil {
		return reply{}, err
	}
	for {
		p, err := s.read(ctx)
		if err != nil {
			return reply{}, err
		}
		if strings.HasPrefix(p, "42") {
			if err = s.event(p); err != nil {
				return reply{}, err
			}
			continue
		}
		if !strings.HasPrefix(p, "43"+id+"[") {
			continue
		}
		var replies []reply
		if json.Unmarshal([]byte(strings.TrimPrefix(p, "43"+id)), &replies) != nil || len(replies) != 1 {
			return reply{}, protocolError()
		}
		r := replies[0]
		if r.TokenRequired {
			return reply{}, failure("kuma_otp_required", "请在接入配置中填写当前两步验证码，重新验证并保存", 422)
		}
		if !r.OK {
			if name == "login" || name == "loginByToken" {
				return reply{}, failure("kuma_auth_failed", "Uptime Kuma 登录失败，请检查账号、密码及两步验证码后重新保存配置", 422)
			}
			return reply{}, failure("kuma_operation_rejected", "Uptime Kuma 拒绝了操作，请刷新监控项并检查参数和账号权限", 422)
		}
		return r, nil
	}
}
