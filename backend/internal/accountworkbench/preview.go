package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin/loginproxy"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

const previewLifetime = 15 * time.Minute

var ErrPreview = errors.New("导入预览已失效或不属于当前会话，请重新预览")

type PreviewInput struct {
	Content      string `json:"content"`
	Action       string `json:"action"`
	TemplateID   string `json:"template_id"`
	Check        bool   `json:"check"`
	Promote      bool   `json:"promote"`
	Model        string `json:"model"`
	ProxyEnabled bool   `json:"proxy_enabled"`
	ProxyURL     string `json:"proxy_url"`
}
type Preview struct {
	ID        string    `json:"id"`
	Revision  int64     `json:"revision"`
	ExpiresAt time.Time `json:"expires_at"`
	Action    string    `json:"action"`
	Check     bool      `json:"check"`
	Promote   bool      `json:"promote"`
	Template  *Template `json:"template"`
	ParsedInput
}
type storedInput struct {
	Item             InputItem      `json:"item"`
	Credentials      map[string]any `json:"credentials"`
	LoginSource      string         `json:"login_source"`
	LoginPassword    string         `json:"login_password,omitempty"`
	MailRefreshToken string         `json:"mail_refresh_token,omitempty"`
	ProxyURL         string         `json:"proxy_url,omitempty"`
}
type privatePreview struct {
	Scope            string                     `json:"scope"`
	Public           Preview                    `json:"public"`
	Owner            string                     `json:"owner"`
	Target           configstore.TargetSettings `json:"target"`
	TemplateRevision int64                      `json:"template_revision"`
	Settings         PreviewInput               `json:"settings"`
	Items            []storedInput              `json:"items"`
	Claimed          bool                       `json:"claimed"`
}

func (s *Service) checkOwner(ctx context.Context, owner string) error {
	active, err := s.private.ActiveSessionOwner(ctx, owner, time.Now().UTC())
	if err != nil || !active {
		return errors.New("控制台登录会话已失效，请重新登录")
	}
	return nil
}
func (s *Service) Preview(ctx context.Context, owner string, input PreviewInput) (Preview, error) {
	if err := s.checkOwner(ctx, owner); err != nil {
		return Preview{}, err
	}
	if input.Action != "import" && input.Action != "export" {
		return Preview{}, errors.New("请选择导入站点或独立 JSON 输出")
	}
	input.Model = strings.TrimSpace(input.Model)
	if input.Model == "" {
		input.Model = "gpt-5.6-sol"
	}
	if len(input.Model) > 200 || strings.ContainsAny(input.Model, "\r\n\x00") {
		return Preview{}, errors.New("检测模型无效")
	}
	if !input.ProxyEnabled {
		input.ProxyURL = ""
	}
	if err := loginproxy.Validate(input.ProxyURL); err != nil {
		return Preview{}, errors.New("登录代理无效，请使用 HTTP、HTTPS 或 SOCKS5 地址")
	}
	if input.ProxyEnabled && strings.TrimSpace(input.ProxyURL) == "" {
		return Preview{}, errors.New("已启用代理，请填写代理地址")
	}
	if input.Action == "import" {
		input.Promote = true
	}
	parsed := ParseInput(input.Content, input.Action == "export")
	result := Preview{Action: input.Action, Check: input.Check, Promote: input.Promote, ParsedInput: parsed}
	if len(parsed.Errors) > 0 {
		return result, nil
	}
	document := privatePreview{Owner: owner, Settings: input, Scope: "managed"}
	document.Settings.Content = ""
	if input.Action == "import" {
		target, err := targetguard.Settings(ctx, s.private)
		if err != nil {
			return Preview{}, err
		}
		document.Target = target
		library, err := s.readTemplates(ctx, target)
		if err != nil {
			return Preview{}, err
		}
		document.TemplateRevision = library.Revision
		selected := input.TemplateID
		if selected == "" {
			selected = library.PreferredID
		}
		if selected == "" && len(library.Items) > 0 {
			selected = library.Items[0].ID
		}
		for i := range library.Items {
			if library.Items[i].ID == selected {
				item := library.Items[i]
				result.Template = &item
				break
			}
		}
		if selected != "" && result.Template == nil {
			return Preview{}, errors.New("模板已不存在，请刷新模板后重新预览")
		}
		if _, err = targetguard.Pin(targetguard.Expect(ctx, target), s.private); err != nil {
			return Preview{}, err
		}
	} else {
		document.Scope = "local-export"
		if input.TemplateID != "" {
			return Preview{}, errors.New("独立 JSON 输出不使用线上模板")
		}
		result.Check = false
		result.Promote = false
		document.Settings.Check = false
		document.Settings.Promote = false
	}
	result.ID = newID()
	result.ExpiresAt = time.Now().UTC().Add(previewLifetime)
	for _, item := range parsed.Items {
		document.Items = append(document.Items, storedInput{Item: item, Credentials: item.Credentials, LoginSource: item.LoginSource})
	}
	document.Public = result
	key := "preview:" + owner
	_, revision, err := s.private.WorkbenchDocument(ctx, key)
	if err != nil {
		return Preview{}, err
	}
	raw, err := json.Marshal(document)
	if err != nil {
		return Preview{}, errors.New("预览资料无法保存")
	}
	result.Revision, err = s.private.SaveWorkbenchDocument(ctx, key, revision, raw)
	return result, err
}
func (s *Service) readPreview(ctx context.Context, owner, id string, revision int64) (privatePreview, error) {
	if err := s.checkOwner(ctx, owner); err != nil {
		return privatePreview{}, err
	}
	raw, current, err := s.private.WorkbenchDocument(ctx, "preview:"+owner)
	if err != nil {
		return privatePreview{}, err
	}
	var document privatePreview
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if current != revision || decoder.Decode(&document) != nil || document.Owner != owner || document.Public.ID != id || document.Claimed || !document.Public.ExpiresAt.After(time.Now().UTC()) {
		return privatePreview{}, ErrPreview
	}
	document.Public.Revision = current
	if (document.Public.Action == "export" && document.Scope != "local-export") || (document.Public.Action == "import" && document.Scope != "managed") {
		return privatePreview{}, ErrPreview
	}
	if document.Public.Action == "import" {
		if _, err = targetguard.Pin(targetguard.Expect(ctx, document.Target), s.private); err != nil {
			return privatePreview{}, err
		}
		library, err := s.readTemplates(ctx, document.Target)
		if err != nil {
			return privatePreview{}, err
		}
		if library.Revision != document.TemplateRevision {
			return privatePreview{}, errors.New("模板已变化，请重新预览")
		}
	}
	return document, nil
}
