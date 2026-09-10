package uptimekuma

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
)

type IDReference struct {
	ID    int64  `json:"id"`
	Name  string `json:"name,omitempty"`
	Title string `json:"title,omitempty"`
}
type NotificationConfig struct {
	Name               string `json:"name"`
	Type               string `json:"type"`
	Default            bool   `json:"default"`
	Active             bool   `json:"active"`
	Endpoint           string `json:"endpoint"`
	Token              string `json:"token"`
	Username           string `json:"username"`
	Password           string `json:"password"`
	EndpointConfigured bool   `json:"endpoint_configured"`
	TokenConfigured    bool   `json:"token_configured"`
	UsernameConfigured bool   `json:"username_configured"`
	PasswordConfigured bool   `json:"password_configured"`
	ChatID             string `json:"chat_id"`
	SMTPHost           string `json:"smtp_host"`
	SMTPPort           int    `json:"smtp_port"`
	SMTPSecure         bool   `json:"smtp_secure"`
	From               string `json:"from"`
	To                 string `json:"to"`
	Topic              string `json:"topic"`
}
type MaintenanceConfig struct {
	Title           string  `json:"title"`
	Description     string  `json:"description"`
	Strategy        string  `json:"strategy"`
	Active          bool    `json:"active"`
	Timezone        string  `json:"timezone"`
	Start           string  `json:"start"`
	End             string  `json:"end"`
	Cron            string  `json:"cron"`
	DurationMinutes int     `json:"duration_minutes"`
	IntervalDays    int     `json:"interval_days"`
	StartTime       string  `json:"start_time"`
	EndTime         string  `json:"end_time"`
	Weekdays        []int   `json:"weekdays"`
	DaysOfMonth     []int   `json:"days_of_month"`
	LastDay         bool    `json:"last_day"`
	MonitorIDs      []int64 `json:"monitor_ids"`
	StatusPageIDs   []int64 `json:"status_page_ids"`
	Status          string  `json:"status"`
}
type PublicMonitor struct {
	ID      int64  `json:"id"`
	SendURL bool   `json:"sendUrl"`
	URL     string `json:"url,omitempty"`
}

func (m *PublicMonitor) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID      int64           `json:"id"`
		SendURL json.RawMessage `json:"sendUrl"`
		URL     string          `json:"url"`
	}
	if json.Unmarshal(data, &raw) != nil || raw.ID <= 0 {
		return protocolError()
	}
	m.ID = raw.ID
	m.URL = raw.URL
	if len(raw.SendURL) == 0 || string(raw.SendURL) == "null" {
		return nil
	}
	if json.Unmarshal(raw.SendURL, &m.SendURL) == nil {
		return nil
	}
	var number int
	if json.Unmarshal(raw.SendURL, &number) != nil || (number != 0 && number != 1) {
		return protocolError()
	}
	m.SendURL = number == 1
	return nil
}

type PublicGroup struct {
	ID          int64           `json:"id,omitempty"`
	Name        string          `json:"name"`
	MonitorList []PublicMonitor `json:"monitorList"`
}
type StatusPageConfig struct {
	Title                 string        `json:"title"`
	Slug                  string        `json:"slug"`
	Description           string        `json:"description"`
	Theme                 string        `json:"theme"`
	Footer                string        `json:"footer"`
	ShowTags              bool          `json:"show_tags"`
	ShowPoweredBy         bool          `json:"show_powered_by"`
	ShowCertificateExpiry bool          `json:"show_certificate_expiry"`
	Domains               []string      `json:"domains"`
	Groups                []PublicGroup `json:"groups"`
}
type ResourceItem struct {
	ID                  int64               `json:"id"`
	Name                string              `json:"name"`
	Type                string              `json:"type"`
	Active              bool                `json:"active"`
	Revision            string              `json:"revision"`
	AssociationRevision string              `json:"association_revision"`
	Notification        *NotificationConfig `json:"notification,omitempty"`
	Maintenance         *MaintenanceConfig  `json:"maintenance,omitempty"`
	StatusPage          *StatusPageConfig   `json:"status_page,omitempty"`
}
type ResourceList struct {
	Config      Config         `json:"config"`
	Items       []ResourceItem `json:"items"`
	Monitors    []Monitor      `json:"monitors"`
	StatusPages []IDReference  `json:"status_pages"`
}
type ResourceInput struct {
	Action              string              `json:"action"`
	ConfigRevision      int64               `json:"config_revision"`
	Revision            string              `json:"revision"`
	AssociationRevision string              `json:"association_revision"`
	Notification        *NotificationConfig `json:"notification,omitempty"`
	Maintenance         *MaintenanceConfig  `json:"maintenance,omitempty"`
	StatusPage          *StatusPageConfig   `json:"status_page,omitempty"`
}

func resourceKind(kind string) bool {
	return kind == "notifications" || kind == "maintenance" || kind == "status-pages"
}
func hashValue(value any) string {
	b, _ := json.Marshal(value)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func randomToken() (string, error) {
	b := make([]byte, 24)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
}
func stableResourceRevision(raw map[string]json.RawMessage) string {
	fields := map[string]json.RawMessage{}
	for k, v := range raw {
		switch k {
		case "status", "timeslotList", "timezoneOffset", "duration", "timezone":
			continue
		}
		fields[k] = v
	}
	return hashValue(fields)
}
func notificationObjects(s *socket) (map[int64]map[string]json.RawMessage, error) {
	if s.notifications == nil {
		return nil, protocolError()
	}
	result := map[int64]map[string]json.RawMessage{}
	for _, item := range s.notifications {
		var outer map[string]json.RawMessage
		if json.Unmarshal(item, &outer) != nil {
			return nil, protocolError()
		}
		id := int64(rawInt(outer, "id", 0))
		if id <= 0 {
			return nil, protocolError()
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal([]byte(rawString(outer, "config")), &fields) != nil || fields == nil {
			return nil, protocolError()
		}
		fields["id"] = outer["id"]
		fields["active"], _ = json.Marshal(rawBool(outer, "active"))
		fields["isDefault"], _ = json.Marshal(rawBool(outer, "isDefault"))
		fields["name"] = outer["name"]
		result[id] = fields
	}
	return result, nil
}
func validateNotificationIDs(s *socket, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	all, err := notificationObjects(s)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, ok := all[id]; !ok {
			return failure("kuma_invalid_options", "通知渠道不存在，请刷新后重新选择", 422)
		}
	}
	return nil
}
func resourceRaw(s *socket, kind string) (map[int64]map[string]json.RawMessage, error) {
	if kind == "notifications" {
		return notificationObjects(s)
	}
	source := s.maintenances
	if kind == "status-pages" {
		source = s.statusPages
	}
	if source == nil {
		return nil, protocolError()
	}
	out := map[int64]map[string]json.RawMessage{}
	for key, b := range source {
		var item map[string]json.RawMessage
		if json.Unmarshal(b, &item) != nil {
			return nil, protocolError()
		}
		id := int64(rawInt(item, "id", 0))
		if id <= 0 {
			return nil, protocolError()
		}
		if kind == "maintenance" && key != strconv.FormatInt(id, 10) {
			return nil, protocolError()
		}
		out[id] = item
	}
	return out, nil
}
func sortedResourceIDs(items map[int64]map[string]json.RawMessage) []int64 {
	ids := make([]int64, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
