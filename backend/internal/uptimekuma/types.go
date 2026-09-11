package uptimekuma

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type Error struct {
	Code, Message string
	Status        int
}

func (e *Error) Error() string                       { return e.Message }
func failure(code, message string, status int) error { return &Error{code, message, status} }

type Config struct {
	BaseURL              string `json:"base_url"`
	Username             string `json:"username"`
	APIKeyConfigured     bool   `json:"api_key_configured"`
	ManagementConfigured bool   `json:"management_configured"`
	Revision             int64  `json:"revision"`
}

func summary(c configstore.UptimeKumaConfig) Config {
	return Config{c.BaseURL, c.Username, c.APIKey != "", c.Token != "", c.Revision}
}

type ConfigInput struct {
	BaseURL           string `json:"base_url"`
	APIKey            string `json:"api_key"`
	Username          string `json:"username"`
	Password          string `json:"password"`
	OTP               string `json:"otp"`
	DisableManagement bool   `json:"disable_management"`
	Revision          int64  `json:"revision"`
}
type Monitor struct {
	TemplateName         string          `json:"template_name,omitempty"`
	TemplateBodyEncoding string          `json:"template_body_encoding,omitempty"`
	Options              *MonitorOptions `json:"options,omitempty"`
	Target               string          `json:"target"`
	ID                   int64           `json:"id"`
	Key                  string          `json:"key"`
	Name                 string          `json:"name"`
	Type                 string          `json:"type"`
	URL                  string          `json:"url"`
	URLRedacted          bool            `json:"url_redacted"`
	Active               bool            `json:"active"`
	Parent               *int64          `json:"parent"`
	Interval             int             `json:"interval"`
	Status               *int            `json:"status"`
	ResponseTime         *float64        `json:"response_time"`
	CertificateDays      *float64        `json:"certificate_days"`
	Uptime               *float64        `json:"uptime"`
	Revision             string          `json:"revision"`
	TemplateID           string          `json:"template_id,omitempty"`
	TemplateRevision     int64           `json:"template_revision,omitempty"`
	TemplateModel        string          `json:"template_model,omitempty"`
}
type Snapshot struct {
	Config   Config    `json:"config"`
	Monitors []Monitor `json:"monitors"`
	Warning  string    `json:"warning"`
}
type MonitorInput struct {
	appliedTemplate          *configstore.KumaMonitorTemplate
	TemplateRetain           bool            `json:"template_retain,omitempty"`
	TemplateModel            string          `json:"template_model,omitempty"`
	TemplateClear            bool            `json:"template_clear,omitempty"`
	TemplateID               string          `json:"template_id,omitempty"`
	TemplateRevision         int64           `json:"template_revision,omitempty"`
	TemplateAuthOverride     bool            `json:"template_auth_override,omitempty"`
	TemplateSettingsOverride bool            `json:"template_settings_override,omitempty"`
	Options                  *MonitorOptions `json:"options,omitempty"`
	Name                     string          `json:"name"`
	Type                     string          `json:"type"`
	URL                      string          `json:"url"`
	Interval                 int             `json:"interval"`
	Parent                   *int64          `json:"parent"`
}
type WriteInput struct {
	ConfigRevision int64        `json:"config_revision"`
	Revision       string       `json:"revision"`
	Action         string       `json:"action"`
	Monitor        MonitorInput `json:"monitor"`
}

// Query strings, URL credentials and fragments may contain upstream secrets.
func publicURL(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", raw != ""
	}
	changed := u.User != nil || u.RawQuery != "" || u.Fragment != ""
	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	if strings.EqualFold(u.Scheme, "http") || strings.EqualFold(u.Scheme, "https") {
		return u.String(), changed
	}
	return "", raw != ""
}
func monitorRevision(raw map[string]json.RawMessage) string {
	fields := map[string]json.RawMessage{}
	if token, ok := raw["pushToken"]; ok {
		fields["pushToken"] = token
	}
	for _, key := range []string{"id", "name", "type", "url", "interval", "parent", "active", "hostname", "port", "keyword", "dns_resolve_type", "dns_resolve_server", "method", "timeout", "retryInterval", "maxretries", "resendInterval", "maxredirects", "ignoreTls", "upsideDown", "accepted_statuscodes", "notificationIDList", "headers", "body", "authMethod", "basic_auth_user", "basic_auth_pass", "bearer_token"} {
		if value, ok := raw[key]; ok {
			fields[key] = value
		}
	}
	b, _ := json.Marshal(fields)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
