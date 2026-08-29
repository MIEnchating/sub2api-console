package notificationtarget

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
)

const (
	qqTokenEndpoint    = "https://bots.qq.com/app/getAppAccessToken"
	qqGatewayEndpoint  = "https://api.sgroup.qq.com/gateway"
	groupAndC2CIntent  = 1 << 25
	guildMessageIntent = 1 << 9
)

type GatewayListener struct {
	client          *http.Client
	tokenEndpoint   string
	gatewayEndpoint string
	allowInsecureWS bool
}

type gatewayPacket struct {
	Op int             `json:"op"`
	S  *int64          `json:"s"`
	T  string          `json:"t"`
	D  json.RawMessage `json:"d"`
}

type gatewayEvent struct {
	GroupOpenID string `json:"group_openid"`
	ChannelID   string `json:"channel_id"`
	Author      struct {
		UserOpenID   string `json:"user_openid"`
		MemberOpenID string `json:"member_openid"`
		Username     string `json:"username"`
	} `json:"author"`
}

type gatewayRead struct {
	packet gatewayPacket
	err    error
}

func NewGatewayListener(client *http.Client) *GatewayListener {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &GatewayListener{client: client, tokenEndpoint: qqTokenEndpoint, gatewayEndpoint: qqGatewayEndpoint}
}

func (l *GatewayListener) Listen(ctx context.Context, request Request, ready func()) (Target, error) {
	accessToken, err := l.accessToken(ctx, request)
	if err != nil {
		return Target{}, err
	}
	gatewayURL, err := l.gatewayURL(ctx, accessToken)
	if err != nil {
		return Target{}, err
	}
	if err := l.validateGatewayURL(gatewayURL); err != nil {
		return Target{}, err
	}
	websocketClient := *l.client
	websocketClient.Timeout = 0
	dialContext, cancelDial := context.WithTimeout(ctx, 20*time.Second)
	connection, response, err := websocket.Dial(dialContext, gatewayURL, &websocket.DialOptions{HTTPClient: &websocketClient})
	cancelDial()
	if err != nil {
		if response != nil {
			return Target{}, fmt.Errorf("QQ WebSocket 连接失败（HTTP %d）", response.StatusCode)
		}
		return Target{}, errors.New("QQ WebSocket 连接失败")
	}
	defer connection.Close(websocket.StatusNormalClosure, "target captured")
	hello, err := readGatewayPacket(ctx, connection)
	if err != nil {
		return Target{}, gatewayReadError(err)
	}
	if hello.Op != 10 {
		return Target{}, errors.New("QQ WebSocket 未返回 Hello 消息")
	}
	var helloData struct {
		HeartbeatInterval int `json:"heartbeat_interval"`
	}
	if err := json.Unmarshal(hello.D, &helloData); err != nil || helloData.HeartbeatInterval < 1_000 {
		return Target{}, errors.New("QQ WebSocket 心跳参数无效")
	}
	identify := map[string]any{
		"op": 2,
		"d": map[string]any{
			"token":      "QQBot " + accessToken,
			"intents":    discoveryIntent(request.TargetType),
			"shard":      []int{0, 1},
			"properties": map[string]string{"$os": "linux", "$browser": "sub2api-console", "$device": "sub2api-console"},
		},
	}
	if err := writeGatewayJSON(ctx, connection, identify); err != nil {
		return Target{}, errors.New("QQ WebSocket 鉴权消息发送失败")
	}
	reads := make(chan gatewayRead, 1)
	go readGatewayPackets(ctx, connection, reads)
	heartbeat := time.NewTicker(time.Duration(helloData.HeartbeatInterval) * time.Millisecond)
	defer heartbeat.Stop()
	var sequence *int64
	for {
		select {
		case <-ctx.Done():
			return Target{}, ctx.Err()
		case <-heartbeat.C:
			if err := writeGatewayJSON(ctx, connection, map[string]any{"op": 1, "d": sequence}); err != nil {
				return Target{}, errors.New("QQ WebSocket 心跳发送失败")
			}
		case item := <-reads:
			if item.err != nil {
				return Target{}, gatewayReadError(item.err)
			}
			if item.packet.S != nil {
				value := *item.packet.S
				sequence = &value
			}
			if item.packet.Op == 9 {
				return Target{}, errors.New("QQ WebSocket 鉴权失败，请检查机器人权限")
			}
			if item.packet.Op == 7 {
				return Target{}, errors.New("QQ 要求重新连接，请重新开始获取")
			}
			if item.packet.Op != 0 {
				continue
			}
			if item.packet.T == "READY" {
				ready()
				continue
			}
			target, found := targetFromEvent(request.TargetType, item.packet)
			if found {
				return target, nil
			}
		}
	}
}

func (l *GatewayListener) accessToken(ctx context.Context, request Request) (string, error) {
	body, err := json.Marshal(map[string]string{"appId": request.AppID, "clientSecret": request.ClientSecret})
	if err != nil {
		return "", errors.New("QQBot 鉴权请求编码失败")
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, l.tokenEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", errors.New("QQBot 鉴权请求创建失败")
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	var response struct {
		AccessToken string `json:"access_token"`
	}
	if err := l.doJSON(httpRequest, &response, "QQBot 鉴权"); err != nil {
		return "", err
	}
	if strings.TrimSpace(response.AccessToken) == "" {
		return "", errors.New("QQBot 鉴权响应缺少 access_token")
	}
	return strings.TrimSpace(response.AccessToken), nil
}

func (l *GatewayListener) gatewayURL(ctx context.Context, accessToken string) (string, error) {
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, l.gatewayEndpoint, nil)
	if err != nil {
		return "", errors.New("QQ Gateway 请求创建失败")
	}
	httpRequest.Header.Set("Authorization", "QQBot "+accessToken)
	var response struct {
		URL string `json:"url"`
	}
	if err := l.doJSON(httpRequest, &response, "QQ Gateway"); err != nil {
		return "", err
	}
	return strings.TrimSpace(response.URL), nil
}

func (l *GatewayListener) doJSON(request *http.Request, target any, operation string) error {
	response, err := l.client.Do(request)
	if err != nil {
		return fmt.Errorf("%s请求失败", operation)
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if readErr != nil {
		return fmt.Errorf("%s响应读取失败", operation)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%s失败（HTTP %d）", operation, response.StatusCode)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("%s响应无法解析", operation)
	}
	return nil
}

func (l *GatewayListener) validateGatewayURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return errors.New("QQ Gateway 返回的 WebSocket 地址无效")
	}
	if parsed.Scheme == "wss" {
		return nil
	}
	if l.allowInsecureWS && parsed.Scheme == "ws" {
		return nil
	}
	return errors.New("QQ Gateway 返回的 WebSocket 地址不安全")
}

func discoveryIntent(targetType string) int {
	if targetType == "channel" {
		return guildMessageIntent
	}
	return groupAndC2CIntent
}

func readGatewayPackets(ctx context.Context, connection *websocket.Conn, output chan<- gatewayRead) {
	for {
		packet, err := readGatewayPacket(ctx, connection)
		select {
		case output <- gatewayRead{packet: packet, err: err}:
		case <-ctx.Done():
			return
		}
		if err != nil {
			return
		}
	}
}

func readGatewayPacket(ctx context.Context, connection *websocket.Conn) (gatewayPacket, error) {
	messageType, payload, err := connection.Read(ctx)
	if err != nil {
		return gatewayPacket{}, err
	}
	if messageType != websocket.MessageText {
		return gatewayPacket{}, errors.New("QQ WebSocket 返回了非文本消息")
	}
	var packet gatewayPacket
	if err := json.Unmarshal(payload, &packet); err != nil {
		return gatewayPacket{}, errors.New("QQ WebSocket 消息无法解析")
	}
	return packet, nil
}

func writeGatewayJSON(ctx context.Context, connection *websocket.Conn, payload any) error {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return connection.Write(ctx, websocket.MessageText, encoded)
}

func targetFromEvent(targetType string, packet gatewayPacket) (Target, bool) {
	var event gatewayEvent
	if err := json.Unmarshal(packet.D, &event); err != nil {
		return Target{}, false
	}
	id := ""
	switch targetType {
	case "c2c":
		if packet.T == "C2C_MESSAGE_CREATE" {
			id = strings.TrimSpace(event.Author.UserOpenID)
		}
	case "group":
		if packet.T == "GROUP_AT_MESSAGE_CREATE" || packet.T == "GROUP_MESSAGE_CREATE" {
			id = strings.TrimSpace(event.GroupOpenID)
		}
	case "channel":
		if packet.T == "AT_MESSAGE_CREATE" || packet.T == "MESSAGE_CREATE" {
			id = strings.TrimSpace(event.ChannelID)
		}
	}
	if id == "" {
		return Target{}, false
	}
	return Target{
		ID: id, Type: targetType, EventType: packet.T, SourceName: strings.TrimSpace(event.Author.Username),
		CapturedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, true
}

func gatewayReadError(err error) error {
	if err == nil {
		return nil
	}
	if status := websocket.CloseStatus(err); status != -1 {
		switch status {
		case 4008:
			return errors.New("QQ WebSocket 操作过快，请稍后重试")
		case 4013:
			return errors.New("QQ WebSocket 订阅类型无效")
		case 4014:
			return errors.New("机器人未开通所选消息事件权限，请在 QQ 开放平台检查事件权限")
		case 4914:
			return errors.New("机器人已下架，仅允许连接沙箱环境")
		case 4915:
			return errors.New("机器人已被封禁，无法连接事件服务")
		default:
			return fmt.Errorf("QQ WebSocket 已断开（%d）", status)
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return errors.New("QQ WebSocket 读取失败")
}
