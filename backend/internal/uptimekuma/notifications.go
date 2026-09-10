package uptimekuma

import (
	"encoding/json"
	"strings"
)

var notificationProviders = map[string]bool{"webhook": true, "telegram": true, "smtp": true, "ntfy": true, "discord": true}

func notificationKeys(kind string) (endpoint, token, username, password string) {
	switch kind {
	case "webhook":
		return "webhookURL", "", "", ""
	case "telegram":
		return "telegramServerUrl", "telegramBotToken", "", ""
	case "smtp":
		return "", "", "smtpUsername", "smtpPassword"
	case "ntfy":
		return "ntfyserverurl", "ntfyaccesstoken", "ntfyusername", "ntfypassword"
	case "discord":
		return "discordWebhookUrl", "", "", ""
	}
	return
}
func decodeNotification(raw map[string]json.RawMessage) *NotificationConfig {
	n := &NotificationConfig{Name: rawString(raw, "name"), Type: rawString(raw, "type"), Default: rawBool(raw, "isDefault"), Active: rawBool(raw, "active"), ChatID: rawString(raw, "telegramChatID"), SMTPHost: rawString(raw, "smtpHost"), SMTPPort: rawInt(raw, "smtpPort", 587), SMTPSecure: rawBool(raw, "smtpSecure"), From: rawString(raw, "smtpFrom"), To: rawString(raw, "smtpTo"), Topic: rawString(raw, "ntfytopic")}
	e, t, u, p := notificationKeys(n.Type)
	n.EndpointConfigured = rawString(raw, e) != ""
	n.TokenConfigured = rawString(raw, t) != ""
	n.UsernameConfigured = rawString(raw, u) != ""
	n.PasswordConfigured = rawString(raw, p) != ""
	return n
}
func notificationPayload(n *NotificationConfig, current map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	bad := func(msg string) (map[string]json.RawMessage, error) {
		return nil, failure("kuma_invalid_notification", msg, 422)
	}
	if n == nil || strings.TrimSpace(n.Name) == "" || len(n.Name) > 150 {
		return bad("请填写通知渠道名称（最多 150 字符）")
	}
	if !notificationProviders[n.Type] {
		return bad("请选择支持的通知渠道类型")
	}
	if current != nil && rawString(current, "type") != n.Type {
		return bad("编辑时不能变更通知渠道类型，请新增渠道")
	}
	raw := map[string]json.RawMessage{}
	for k, v := range current {
		raw[k] = v
	}
	set := func(k string, v any) { raw[k], _ = json.Marshal(v) }
	set("name", strings.TrimSpace(n.Name))
	set("type", n.Type)
	set("isDefault", n.Default)
	set("applyExisting", false)
	e, t, u, p := notificationKeys(n.Type)
	for key, value := range map[string]string{e: n.Endpoint, t: n.Token, u: n.Username, p: n.Password} {
		if key != "" && value != "" {
			if len(value) > 8192 {
				return bad("通知凭据过长")
			}
			set(key, value)
		}
	}
	if n.Endpoint != "" && !validateHTTPURL(n.Endpoint) {
		return bad("通知服务地址必须是完整的 HTTP(S) URL")
	}
	switch n.Type {
	case "webhook":
		if rawString(raw, e) == "" {
			return bad("请填写 Webhook 地址")
		}
		if current == nil {
			set("webhookContentType", "json")
		}
	case "discord":
		if rawString(raw, e) == "" {
			return bad("请填写 Discord Webhook 地址")
		}
	case "telegram":
		if rawString(raw, t) == "" || strings.TrimSpace(n.ChatID) == "" {
			return bad("请填写 Telegram Bot Token 和聊天 ID")
		}
		set("telegramChatID", n.ChatID)
	case "smtp":
		if !validHost(n.SMTPHost) || n.SMTPPort < 1 || n.SMTPPort > 65535 || !strings.Contains(n.From, "@") || !strings.Contains(n.To, "@") {
			return bad("请检查 SMTP 主机、端口和收发邮件地址")
		}
		set("smtpHost", n.SMTPHost)
		set("smtpPort", n.SMTPPort)
		set("smtpSecure", n.SMTPSecure)
		set("smtpFrom", n.From)
		set("smtpTo", n.To)
	case "ntfy":
		if rawString(raw, e) == "" || strings.TrimSpace(n.Topic) == "" {
			return bad("请填写 ntfy 服务地址和主题")
		}
		set("ntfytopic", n.Topic)
		if rawString(raw, t) != "" {
			set("ntfyAuthenticationMethod", "accessToken")
		} else if rawString(raw, u) != "" {
			set("ntfyAuthenticationMethod", "usernamePassword")
		} else {
			set("ntfyAuthenticationMethod", "none")
		}
		if current == nil {
			set("ntfyPriority", 3)
		}
	}
	return raw, nil
}
