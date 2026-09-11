package uptimekuma

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"regexp"
	"strings"
)

type TemplateInput struct {
	BodyEncoding   string                              `json:"body_encoding"`
	Monitoring     *configstore.KumaTemplateMonitoring `json:"monitoring,omitempty"`
	ClearURL       bool                                `json:"clear_url"`
	RequestProfile string                              `json:"request_profile"`
	Model          string                              `json:"model"`
	Revision       int64                               `json:"revision"`
	Name           string                              `json:"name"`
	Method         string                              `json:"method"`
	Headers        string                              `json:"headers"`
	Body           string                              `json:"body"`
	AuthMethod     string                              `json:"auth_method"`
	AuthUsername   string                              `json:"auth_username"`
	AuthPassword   string                              `json:"auth_password"`
	ClearHeaders   bool                                `json:"clear_headers"`
	ClearBody      bool                                `json:"clear_body"`
	ClearAuth      bool                                `json:"clear_auth"`
}
type TemplateSummary struct {
	BodyEncoding      string                              `json:"body_encoding"`
	Monitoring        *configstore.KumaTemplateMonitoring `json:"monitoring,omitempty"`
	URLConfigured     bool                                `json:"url_configured"`
	URLRedacted       bool                                `json:"url_redacted"`
	RequestProfile    string                              `json:"request_profile"`
	Model             string                              `json:"model"`
	ID                string                              `json:"id"`
	Revision          int64                               `json:"revision"`
	Name              string                              `json:"name"`
	Method            string                              `json:"method"`
	AuthMethod        string                              `json:"auth_method"`
	HeadersConfigured bool                                `json:"headers_configured"`
	BodyConfigured    bool                                `json:"body_configured"`
	AuthConfigured    bool                                `json:"auth_configured"`
}

// TemplateDetail is available only through the authenticated, on-demand editor API.
// Dedicated authentication credentials and sensitive monitoring URLs remain private.
type TemplateDetail struct {
	TemplateSummary
	Headers string `json:"headers"`
	Body    string `json:"body"`
}

func (s *Service) Template(ctx context.Context, id string) (TemplateDetail, error) {
	if !templateIDPattern.MatchString(id) {
		return TemplateDetail{}, failure("kuma_invalid_template", "模板 ID 无效", 422)
	}
	item, err := s.store.KumaTemplate(ctx, id)
	if err != nil {
		return TemplateDetail{}, templateError(err)
	}
	return TemplateDetail{TemplateSummary: templateSummary(item), Headers: item.Headers, Body: item.Body}, nil
}

var templateIDPattern = regexp.MustCompile(`^[a-f0-9]{48}$`)

func templateSummary(item configstore.KumaTemplate) TemplateSummary {
	result := TemplateSummary{BodyEncoding: item.BodyEncoding, ID: item.ID, Revision: item.Revision, Name: item.Name, Method: item.Method, AuthMethod: item.AuthMethod, HeadersConfigured: item.Headers != "", BodyConfigured: item.Body != "", AuthConfigured: item.AuthPassword != "", RequestProfile: item.RequestProfile, Model: item.Model}
	if item.Monitoring != nil {
		settings := *item.Monitoring
		result.URLConfigured = settings.URL != ""
		settings.URL, result.URLRedacted = publicURL(settings.URL)
		if result.URLRedacted {
			settings.URL = ""
		}
		result.Monitoring = &settings
	}
	return result
}
func templateError(err error) error {
	if errors.Is(err, configstore.ErrKumaTemplateConflict) {
		return failure("kuma_template_conflict", err.Error(), 409)
	}
	return err
}
func (s *Service) Templates(ctx context.Context) ([]TemplateSummary, error) {
	items, err := s.store.KumaTemplates(ctx)
	if err != nil {
		return nil, err
	}
	result := []TemplateSummary{}
	for _, item := range items {
		result = append(result, templateSummary(item))
	}
	return result, nil
}
func (s *Service) SaveTemplate(ctx context.Context, id string, in TemplateInput) (TemplateSummary, error) {
	var item configstore.KumaTemplate
	if id != "" {
		if !templateIDPattern.MatchString(id) {
			return TemplateSummary{}, failure("kuma_invalid_template", "模板 ID 无效", 422)
		}
		var err error
		item, err = s.store.KumaTemplate(ctx, id)
		if err != nil {
			return TemplateSummary{}, templateError(err)
		}
		if item.Revision != in.Revision {
			return TemplateSummary{}, templateError(configstore.ErrKumaTemplateConflict)
		}
	} else {
		if in.Revision != 0 {
			return TemplateSummary{}, templateError(configstore.ErrKumaTemplateConflict)
		}
		var err error
		item.ID, err = randomToken()
		if err != nil {
			return TemplateSummary{}, err
		}
	}
	item.Name = strings.TrimSpace(in.Name)
	if in.Monitoring != nil {
		settings := *in.Monitoring
		if settings.URL == "" && !in.ClearURL && item.Monitoring != nil {
			settings.URL = item.Monitoring.URL
		}
		if in.ClearURL {
			settings.URL = ""
		}
		item.Monitoring = &settings
	}
	item.RequestProfile = in.RequestProfile
	item.Model = strings.TrimSpace(in.Model)
	item.Method = in.Method
	if in.BodyEncoding != "" && in.BodyEncoding != "json" && in.BodyEncoding != "form" && in.BodyEncoding != "xml" {
		return TemplateSummary{}, failure("kuma_invalid_body_encoding", "请选择 JSON、表单或 XML 请求体编码", 422)
	}
	item.BodyEncoding = in.BodyEncoding
	if item.Name == "" || len([]rune(item.Name)) > 150 {
		return TemplateSummary{}, failure("kuma_invalid_template_name", "模板名称必须为 1 到 150 个字符", 422)
	}
	if in.Headers != "" || in.ClearHeaders {
		item.Headers = in.Headers
	}
	if in.Body != "" || in.ClearBody {
		item.Body = in.Body
	}
	if item.AuthMethod != in.AuthMethod || in.ClearAuth {
		item.AuthUsername = ""
		item.AuthPassword = ""
	}
	item.AuthMethod = in.AuthMethod
	if in.AuthUsername != "" {
		item.AuthUsername = in.AuthUsername
	}
	if in.AuthPassword != "" {
		item.AuthPassword = in.AuthPassword
	}
	if item.AuthMethod == "none" {
		item.AuthUsername = ""
		item.AuthPassword = ""
	}
	if item.AuthMethod != "none" && item.AuthMethod != "basic" && item.AuthMethod != "bearer" {
		return TemplateSummary{}, failure("kuma_invalid_template_auth", "请选择无鉴权、Basic 或 Bearer", 422)
	}
	if item.AuthMethod != "none" && item.AuthPassword == "" || item.AuthMethod == "basic" && item.AuthUsername == "" {
		return TemplateSummary{}, failure("kuma_invalid_template_auth", "请填写所选鉴权方式需要的用户名和密码或 Token", 422)
	}
	o := monitorOptions(nil)
	o.Method = item.Method
	o.Headers = item.Headers
	o.Body = item.Body
	o.AuthMethod = item.AuthMethod
	o.AuthUsername = item.AuthUsername
	o.AuthPassword = item.AuthPassword
	if in.BodyEncoding == "" {
		if err := prepareTemplateProfile(&item); err != nil {
			return TemplateSummary{}, err
		}
	} else if item.RequestProfile != "" {
		probe := item
		if err := prepareTemplateProfile(&probe); err != nil {
			return TemplateSummary{}, err
		}
	}
	if item.BodyEncoding == "json" && item.Body != "" && !json.Valid([]byte(item.Body)) {
		return TemplateSummary{}, failure("kuma_invalid_template_body", "请求体不是有效 JSON，请检查内容或切换编码", 422)
	}
	o.Method = item.Method
	o.Headers = item.Headers
	o.Body = item.Body
	if item.Monitoring != nil {
		applyTemplateMonitoring(o, item.Monitoring)
		if !supportedMonitorTypes[item.Monitoring.Type] {
			return TemplateSummary{}, failure("kuma_invalid_template_type", "请选择支持的监控类型", 422)
		}
		if err := validateMonitor(MonitorInput{Name: item.Name, Type: item.Monitoring.Type, URL: item.Monitoring.URL, Interval: item.Monitoring.Interval, Options: o}); err != nil {
			return TemplateSummary{}, err
		}
	}
	if err := validateOptions("http", o); err != nil {
		return TemplateSummary{}, err
	}
	if err := s.store.SaveKumaTemplate(ctx, item); err != nil {
		return TemplateSummary{}, templateError(err)
	}
	item.Revision++
	return templateSummary(item), nil
}
func (s *Service) DeleteTemplate(ctx context.Context, id string, revision int64) error {
	if !templateIDPattern.MatchString(id) || revision < 1 {
		return failure("kuma_invalid_template", "模板 ID 或版本无效", 422)
	}
	return templateError(s.store.DeleteKumaTemplate(ctx, id, revision))
}
func (s *Service) resolveTemplate(ctx context.Context, in *MonitorInput) error {
	if in.TemplateID == "" {
		if strings.TrimSpace(in.TemplateModel) != "" {
			return failure("kuma_invalid_template_model", "请先选择功能模板，再设置请求模型", 422)
		}
		return nil
	}
	if !templateIDPattern.MatchString(in.TemplateID) {
		return failure("kuma_invalid_template", "请求模板仅适用于 HTTP 和关键字监控", 422)
	}
	item, err := s.store.KumaTemplate(ctx, in.TemplateID)
	if err != nil {
		return templateError(err)
	}
	if item.Revision != in.TemplateRevision {
		return templateError(configstore.ErrKumaTemplateConflict)
	}
	if item.Monitoring != nil {
		if in.TemplateSettingsOverride && in.Type != item.Monitoring.Type {
			return failure("kuma_invalid_template_type", "模板监控类型与当前监控不一致，请重新选择模板", 422)
		}
		if !in.TemplateSettingsOverride {
			in.Type = item.Monitoring.Type
			in.Interval = item.Monitoring.Interval
		}
		if in.URL == "" {
			in.URL = item.Monitoring.URL
		}
	} else if !isHTTP(in.Type) {
		return failure("kuma_invalid_template_type", "此请求模板仅适用于 HTTP 监控", 422)
	}
	if in.Options == nil {
		in.Options = monitorOptions(nil)
	}
	o := in.Options
	if item.BodyEncoding != "" {
		o.bodyEncoding = item.BodyEncoding
	} else if item.RequestProfile != "" {
		o.bodyEncoding = "json"
	}
	if item.Monitoring != nil && !in.TemplateSettingsOverride {
		applyTemplateMonitoring(o, item.Monitoring)
	}
	if in.TemplateAuthOverride || in.editing {
		if !in.editing && o.AuthMethod != "none" && o.AuthMethod != "basic" && o.AuthMethod != "bearer" {
			return failure("kuma_invalid_auth", "请选择无鉴权、Basic 或 Bearer", 422)
		}
		if !in.editing && (o.AuthMethod != "none" && o.AuthPassword == "" || o.AuthMethod == "basic" && o.AuthUsername == "") {
			return failure("kuma_invalid_auth", "单独设置鉴权时，请填写完整的用户名和密码或 Token", 422)
		}
		if item.Headers != "" {
			var headers map[string]string
			if err := json.Unmarshal([]byte(item.Headers), &headers); err != nil {
				return protocolError()
			}
			for key := range headers {
				if strings.EqualFold(key, "Authorization") || strings.EqualFold(key, "x-api-key") {
					delete(headers, key)
				}
			}
			data, err := json.Marshal(headers)
			if err != nil {
				return err
			}
			item.Headers = string(data)
		}
	} else {
		o.AuthMethod = item.AuthMethod
		o.AuthUsername = item.AuthUsername
		o.AuthPassword = item.AuthPassword
	}
	o.Method = item.Method
	o.Headers = item.Headers
	o.Body, err = templateBodyWithModel(item.Body, o.bodyEncoding, in.Type, in.TemplateModel)
	if err != nil {
		return err
	}
	in.appliedTemplate = &configstore.KumaMonitorTemplate{TemplateID: item.ID, Revision: item.Revision, Name: item.Name, Model: strings.TrimSpace(in.TemplateModel), BodyEncoding: o.bodyEncoding}
	o.replaceAuth = !in.editing
	o.ClearHeaders = true
	o.ClearBody = true
	if !in.editing {
		o.ClearAuth = false
	}
	return nil
}
