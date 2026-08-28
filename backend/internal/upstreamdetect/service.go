package upstreamdetect

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

const maximumResponseBytes = 1 << 20

type Result struct {
	BaseURL      string  `json:"base_url"`
	Host         string  `json:"host"`
	UpstreamType *string `json:"upstream_type"`
	AuthMode     *string `json:"auth_mode"`
	Name         *string `json:"name"`
	TypeDetected bool    `json:"type_detected"`
	NameDetected bool    `json:"name_detected"`
	Evidence     *string `json:"evidence"`
}

type Service struct {
	client *http.Client
}

func New(client *http.Client) *Service {
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	copy := *client
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if copy.Timeout == 0 {
		copy.Timeout = 8 * time.Second
	}
	return &Service{client: &copy}
}

func (s *Service) Detect(ctx context.Context, rawURL string) (Result, error) {
	baseURL, err := configstore.ValidateBaseURL(rawURL)
	if err != nil {
		return Result{}, errors.New("上游地址必须是完整的 http 或 https 地址")
	}
	parsed, _ := url.Parse(baseURL)
	result := Result{BaseURL: baseURL, Host: strings.ToLower(parsed.Host)}
	newAPIPayload := s.readJSON(ctx, baseURL+"/api/status")
	if detected, name := newAPIStatus(newAPIPayload); detected {
		result.UpstreamType, result.AuthMode = textPointer("newapi"), textPointer("newapi_admin_key")
		result.Name, result.TypeDetected, result.NameDetected, result.Evidence = name, true, name != nil, textPointer("/api/status")
		return result, nil
	}
	sub2APIPayload := s.readJSON(ctx, baseURL+"/api/v1/settings/public")
	if detected, name := sub2APISettings(sub2APIPayload); detected {
		result.UpstreamType, result.AuthMode = textPointer("sub2api"), textPointer("sub2api_user_token")
		result.Name, result.TypeDetected, result.NameDetected, result.Evidence = name, true, name != nil, textPointer("/api/v1/settings/public")
		return result, nil
	}
	if sub2APISetup(s.readJSON(ctx, baseURL+"/setup/status")) {
		result.UpstreamType, result.AuthMode = textPointer("sub2api"), textPointer("sub2api_user_token")
		result.TypeDetected, result.Evidence = true, textPointer("/setup/status")
		return result, nil
	}
	if data := responseData(newAPIPayload); data != nil {
		result.Name = publicName(data)
	}
	if result.Name == nil {
		if data := responseData(sub2APIPayload); data != nil {
			result.Name = publicName(data)
		}
	}
	result.NameDetected = result.Name != nil
	return result, nil
}

func (s *Service) readJSON(ctx context.Context, endpoint string) any {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Sub2API-Console/0.1")
	response, err := s.client.Do(request)
	if err != nil {
		return nil
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength > maximumResponseBytes {
		return nil
	}
	reader := io.LimitReader(response.Body, maximumResponseBytes+1)
	data, err := io.ReadAll(reader)
	if err != nil || len(data) > maximumResponseBytes {
		return nil
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return nil
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil
	}
	return payload
}

func responseData(payload any) map[string]any {
	object, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	if data, ok := object["data"].(map[string]any); ok {
		return data
	}
	return object
}

func publicName(data map[string]any) *string {
	for _, key := range []string{"system_name", "site_name", "name"} {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			normalized := []rune(strings.TrimSpace(value))
			if len(normalized) > 100 {
				normalized = normalized[:100]
			}
			text := string(normalized)
			return &text
		}
	}
	return nil
}

func newAPIStatus(payload any) (bool, *string) {
	object, ok := payload.(map[string]any)
	if !ok || object["success"] != true {
		return false, nil
	}
	data, ok := object["data"].(map[string]any)
	if !ok || !hasAny(data, "system_name", "quota_per_unit", "quota_display_type", "enable_data_export", "password_login_enabled") {
		return false, nil
	}
	return true, publicName(data)
}

func sub2APISettings(payload any) (bool, *string) {
	object, ok := payload.(map[string]any)
	if !ok || strings.TrimSpace(asNumberText(object["code"])) != "0" {
		return false, nil
	}
	data, ok := object["data"].(map[string]any)
	if !ok || !hasAny(data, "site_name", "api_base_url", "server_timezone", "registration_enabled", "turnstile_enabled") {
		return false, nil
	}
	return true, publicName(data)
}

func sub2APISetup(payload any) bool {
	object, ok := payload.(map[string]any)
	if !ok || strings.TrimSpace(asNumberText(object["code"])) != "0" {
		return false
	}
	data, ok := object["data"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = data["needs_setup"].(bool)
	return ok
}

func hasAny(value map[string]any, keys ...string) bool {
	for _, key := range keys {
		if _, present := value[key]; present {
			return true
		}
	}
	return false
}

func asNumberText(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(toString(value)), ".0"), "."))
}

func toString(value any) string {
	switch item := value.(type) {
	case json.Number:
		return item.String()
	case string:
		return item
	default:
		return ""
	}
}

func textPointer(value string) *string { return &value }
