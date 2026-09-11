package uptimekuma

import (
	"encoding/json"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type MonitorOptions struct {
	replaceAuth         bool
	bodyEncoding        string
	Hostname            string   `json:"hostname"`
	Port                int      `json:"port"`
	Keyword             string   `json:"keyword"`
	DNSRecordType       string   `json:"dns_record_type"`
	DNSResolver         string   `json:"dns_resolver"`
	Method              string   `json:"method"`
	Timeout             int      `json:"timeout"`
	RetryInterval       int      `json:"retry_interval"`
	MaxRetries          int      `json:"max_retries"`
	ResendInterval      int      `json:"resend_interval"`
	MaxRedirects        int      `json:"max_redirects"`
	IgnoreTLS           bool     `json:"ignore_tls"`
	UpsideDown          bool     `json:"upside_down"`
	AcceptedStatusCodes []string `json:"accepted_status_codes"`
	NotificationIDs     []int64  `json:"notification_ids"`
	AuthMethod          string   `json:"auth_method"`
	HeadersConfigured   bool     `json:"headers_configured"`
	BodyConfigured      bool     `json:"body_configured"`
	AuthConfigured      bool     `json:"auth_configured"`
	Headers             string   `json:"headers,omitempty"`
	Body                string   `json:"body,omitempty"`
	AuthUsername        string   `json:"auth_username,omitempty"`
	AuthPassword        string   `json:"auth_password,omitempty"`
	ClearHeaders        bool     `json:"clear_headers,omitempty"`
	ClearBody           bool     `json:"clear_body,omitempty"`
	ClearAuth           bool     `json:"clear_auth,omitempty"`
}

var supportedMonitorTypes = map[string]bool{"http": true, "keyword": true, "port": true, "ping": true, "dns": true, "push": true, "group": true}

func isHTTP(kind string) bool { return kind == "http" || kind == "keyword" }
func rawString(raw map[string]json.RawMessage, key string) string {
	var value string
	_ = json.Unmarshal(raw[key], &value)
	return value
}
func rawInt(raw map[string]json.RawMessage, key string, fallback int) int {
	if string(raw[key]) == "null" {
		return fallback
	}
	var n int
	if json.Unmarshal(raw[key], &n) == nil {
		return n
	}
	var text string
	if json.Unmarshal(raw[key], &text) == nil {
		if v, err := strconv.Atoi(text); err == nil {
			return v
		}
	}
	return fallback
}
func rawBool(raw map[string]json.RawMessage, key string) bool {
	var value bool
	if json.Unmarshal(raw[key], &value) == nil {
		return value
	}
	return rawInt(raw, key, 0) != 0
}
func monitorOptions(raw map[string]json.RawMessage) *MonitorOptions {
	o := &MonitorOptions{Hostname: rawString(raw, "hostname"), Port: rawInt(raw, "port", 0), Keyword: rawString(raw, "keyword"), DNSRecordType: rawString(raw, "dns_resolve_type"), DNSResolver: rawString(raw, "dns_resolve_server"), Method: rawString(raw, "method"), Timeout: rawInt(raw, "timeout", 16), RetryInterval: rawInt(raw, "retryInterval", 60), MaxRetries: rawInt(raw, "maxretries", 0), ResendInterval: rawInt(raw, "resendInterval", 0), MaxRedirects: rawInt(raw, "maxredirects", 10), IgnoreTLS: rawBool(raw, "ignoreTls"), UpsideDown: rawBool(raw, "upsideDown"), AuthMethod: rawString(raw, "authMethod"), HeadersConfigured: rawString(raw, "headers") != "", BodyConfigured: rawString(raw, "body") != "", AuthConfigured: rawString(raw, "basic_auth_pass") != "" || rawString(raw, "bearer_token") != "", AcceptedStatusCodes: []string{"200-299"}, NotificationIDs: []int64{}}
	if o.Method == "" {
		o.Method = "GET"
	}
	if o.DNSRecordType == "" {
		o.DNSRecordType = "A"
	}
	if o.DNSResolver == "" {
		o.DNSResolver = "1.1.1.1"
	}
	if o.AuthMethod == "" {
		o.AuthMethod = "none"
	}
	if v := raw["accepted_statuscodes"]; len(v) > 0 && string(v) != "null" {
		_ = json.Unmarshal(v, &o.AcceptedStatusCodes)
	}
	var ids map[string]bool
	_ = json.Unmarshal(raw["notificationIDList"], &ids)
	for id, active := range ids {
		if n, err := strconv.ParseInt(id, 10, 64); err == nil && active && n > 0 {
			o.NotificationIDs = append(o.NotificationIDs, n)
		}
	}
	sort.Slice(o.NotificationIDs, func(i, j int) bool { return o.NotificationIDs[i] < o.NotificationIDs[j] })
	return o
}
func validHost(s string) bool {
	return s != "" && len(s) <= 253 && !strings.ContainsAny(s, " /\\?#@\r\n\t")
}
func validateOptions(kind string, o *MonitorOptions) error {
	if o == nil {
		return nil
	}
	bad := func(msg string) error { return failure("kuma_invalid_options", msg, 422) }
	if kind == "port" || kind == "ping" || kind == "dns" {
		if !validHost(o.Hostname) {
			return bad("请输入有效的主机名或 IP 地址")
		}
	}
	if kind == "port" && (o.Port < 1 || o.Port > 65535) {
		return bad("TCP 端口必须为 1 到 65535")
	}
	if kind == "keyword" && (strings.TrimSpace(o.Keyword) == "" || len(o.Keyword) > 4096) {
		return bad("请填写要匹配的关键字（最多 4096 字符）")
	}
	if kind == "dns" {
		if !strings.Contains("|A|AAAA|CNAME|MX|NS|TXT|SRV|PTR|SOA|CAA|", "|"+o.DNSRecordType+"|") {
			return bad("DNS 记录类型无效")
		}
		if net.ParseIP(o.DNSResolver) == nil && !validHost(o.DNSResolver) {
			return bad("DNS 解析服务器无效")
		}
	}
	if o.Timeout < 1 || o.Timeout > 3600 || o.RetryInterval < 20 || o.RetryInterval > 86400 || o.MaxRetries < 0 || o.MaxRetries > 100 || o.ResendInterval < 0 || o.ResendInterval > 10000 || o.MaxRedirects < 0 || o.MaxRedirects > 100 {
		return bad("超时、重试或重定向参数超出允许范围")
	}
	if isHTTP(kind) {
		if !strings.Contains("|GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS|", "|"+o.Method+"|") {
			return bad("HTTP 请求方法无效")
		}
		if len(o.AcceptedStatusCodes) == 0 || len(o.AcceptedStatusCodes) > 32 {
			return bad("请填写正常状态码，例如 200-299")
		}
		for _, code := range o.AcceptedStatusCodes {
			parts := strings.Split(code, "-")
			if len(parts) > 2 {
				return bad("正常状态码格式无效")
			}
			last := 0
			for _, part := range parts {
				n, e := strconv.Atoi(part)
				if e != nil || n < 100 || n > 599 || n < last {
					return bad("正常状态码必须在 100 到 599 之间")
				}
				last = n
			}
		}
		if len(o.Headers) > 32768 || len(o.Body) > 65536 || len(o.AuthUsername) > 4096 || len(o.AuthPassword) > 4096 {
			return bad("请求配置内容过长")
		}
		if o.Headers != "" {
			var h map[string]string
			if json.Unmarshal([]byte(o.Headers), &h) != nil || h == nil {
				return bad("请求头必须是名称与值组成的 JSON 对象")
			}
			for k, v := range h {
				if strings.ContainsAny(k+v, "\r\n") {
					return bad("请求头不能包含换行符")
				}
			}
		}
		if len(o.AuthMethod) > 64 {
			return bad("鉴权类型无效")
		}
	}
	seen := map[int64]bool{}
	for _, id := range o.NotificationIDs {
		if id <= 0 || seen[id] {
			return bad("通知渠道 ID 无效或重复")
		}
		seen[id] = true
	}
	return nil
}
func applyOptions(raw map[string]json.RawMessage, kind string, o *MonitorOptions) {
	if o == nil {
		return
	}
	set := func(k string, v any) { raw[k], _ = json.Marshal(v) }
	for k, v := range map[string]any{"timeout": o.Timeout, "retryInterval": o.RetryInterval, "maxretries": o.MaxRetries, "resendInterval": o.ResendInterval, "upsideDown": o.UpsideDown} {
		set(k, v)
	}
	ids := map[string]bool{}
	for _, id := range o.NotificationIDs {
		ids[strconv.FormatInt(id, 10)] = true
	}
	set("notificationIDList", ids)
	if kind == "port" || kind == "ping" || kind == "dns" {
		set("hostname", o.Hostname)
	}
	if kind == "port" {
		set("port", o.Port)
	}
	if kind == "dns" {
		set("dns_resolve_type", o.DNSRecordType)
		set("dns_resolve_server", o.DNSResolver)
	}
	if kind == "keyword" {
		set("keyword", o.Keyword)
	}
	if isHTTP(kind) {
		set("method", o.Method)
		set("maxredirects", o.MaxRedirects)
		set("ignoreTls", o.IgnoreTLS)
		set("accepted_statuscodes", o.AcceptedStatusCodes)
		if o.Headers != "" || o.ClearHeaders {
			set("headers", o.Headers)
		}
		if o.Body != "" || o.ClearBody {
			set("body", o.Body)
		}
		if o.bodyEncoding != "" {
			set("httpBodyEncoding", o.bodyEncoding)
		}
		if unchangedMonitorAuth(raw, o) && !o.replaceAuth {
			return
		}
		if o.replaceAuth || rawString(raw, "authMethod") != o.AuthMethod {
			set("basic_auth_user", "")
			set("basic_auth_pass", "")
			set("bearer_token", "")
		}
		set("authMethod", o.AuthMethod)
		if o.ClearAuth || o.AuthMethod == "none" {
			set("basic_auth_user", "")
			set("basic_auth_pass", "")
			set("bearer_token", "")
		} else if o.AuthMethod == "basic" {
			if o.AuthUsername != "" {
				set("basic_auth_user", o.AuthUsername)
			}
			if o.AuthPassword != "" {
				set("basic_auth_pass", o.AuthPassword)
			}
		} else if o.AuthMethod == "bearer" && o.AuthPassword != "" {
			set("bearer_token", o.AuthPassword)
		}
	}
}
func displayTarget(m Monitor) string {
	if m.URL != "" {
		return m.URL
	}
	if m.Options != nil {
		if m.Type == "port" {
			return net.JoinHostPort(m.Options.Hostname, strconv.Itoa(m.Options.Port))
		}
		return m.Options.Hostname
	}
	return ""
}
func validateHTTPURL(value string) bool {
	u, e := url.Parse(value)
	return e == nil && len(value) <= 4096 && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil && u.Fragment == ""
}
