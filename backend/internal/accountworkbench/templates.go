package accountworkbench

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
	"strings"
	"time"
)

type Template struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	SourceID      string         `json:"source_id"`
	SourceName    string         `json:"source_name"`
	SourceVersion string         `json:"source_version"`
	SyncedAt      string         `json:"synced_at"`
	Revision      int64          `json:"revision"`
	Config        TemplateConfig `json:"config"`
	Summary       Account        `json:"summary"`
}
type TemplateLibrary struct {
	Revision    int64      `json:"revision"`
	PreferredID string     `json:"preferred_id"`
	Items       []Template `json:"items"`
}
type TemplateInput struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	SourceID      string          `json:"source_id"`
	SourceVersion string          `json:"source_version"`
	Revision      int64           `json:"revision"`
	Config        *TemplateConfig `json:"config,omitempty"`
}

func targetKey(target configstore.TargetSettings) string {
	sum := sha256.Sum256([]byte(target.BaseURL + "\x00" + target.AdminKey))
	return hex.EncodeToString(sum[:])
}
func digest(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func newID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(raw[:])
}

func (s *Service) Templates(ctx context.Context) (TemplateLibrary, error) {
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return TemplateLibrary{}, err
	}
	return s.readTemplates(ctx, target)
}
func (s *Service) readTemplates(ctx context.Context, target configstore.TargetSettings) (TemplateLibrary, error) {
	result := TemplateLibrary{Items: []Template{}}
	raw, revision, err := s.private.WorkbenchDocument(ctx, "templates:"+targetKey(target))
	if err != nil {
		return result, err
	}
	if len(raw) > 0 {
		if err = json.Unmarshal(raw, &result); err != nil {
			return result, errors.New("模板记录无法读取")
		}
	}
	result.Revision = revision
	return result, nil
}
func (s *Service) TemplateSource(ctx context.Context, id string) (Template, error) {
	if id == "" || strings.ContainsAny(id, "/\\?#\x00") {
		return Template{}, errors.New("来源账号 ID 无效")
	}
	ctx, err := targetguard.Capture(ctx, s.private)
	if err != nil {
		return Template{}, err
	}
	ctx, err = targetguard.Pin(ctx, s.private)
	if err != nil {
		return Template{}, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return Template{}, err
	}
	client, err := s.client(target)
	if err != nil {
		return Template{}, err
	}
	row, err := client.Account(ctx, id)
	if err != nil {
		return Template{}, errors.New("来源账号读取失败，请刷新账号列表")
	}
	if text(row["id"]) != id {
		return Template{}, errors.New("来源账号 ID 不匹配")
	}
	config, err := ExtractConfig(row)
	if err != nil {
		return Template{}, err
	}
	summary := publicAccount(row)
	result := Template{SourceID: id, SourceName: summary.Name, Config: config, Summary: summary}
	result.SourceVersion = digest(struct {
		Config   TemplateConfig
		Identity any
		Target   string
	}{config, []string{text(object(row["credentials"])["chatgpt_account_id"]), text(object(row["credentials"])["chatgpt_user_id"])}, targetKey(target)})
	if _, err = targetguard.Pin(ctx, s.private); err != nil {
		return Template{}, err
	}
	return result, nil
}
func (s *Service) SaveTemplate(ctx context.Context, input TemplateInput) (TemplateLibrary, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 200 || strings.ContainsAny(input.Name, "\r\n\x00") {
		return TemplateLibrary{}, errors.New("请填写有效的模板名称")
	}
	ctx, err := targetguard.Capture(ctx, s.private)
	if err != nil {
		return TemplateLibrary{}, err
	}
	ctx, release, err := targetguard.Acquire(ctx, s.private)
	if err != nil {
		return TemplateLibrary{}, err
	}
	defer release()
	ctx, err = targetguard.Bind(ctx, s.private)
	if err != nil {
		return TemplateLibrary{}, err
	}
	var source Template
	if input.Config != nil {
		if input.SourceID != "" || input.SourceVersion != "" {
			return TemplateLibrary{}, errors.New("手动配置不能同时指定来源账号")
		}
		if err = input.Config.Validate(); err != nil {
			return TemplateLibrary{}, err
		}
		source = manualTemplate(*input.Config)
	} else {
		if input.SourceVersion == "" {
			return TemplateLibrary{}, errors.New("请先读取来源账号")
		}
		source, err = s.TemplateSource(ctx, input.SourceID)
		if err != nil {
			return TemplateLibrary{}, err
		}
		if source.SourceVersion != input.SourceVersion {
			return TemplateLibrary{}, errors.New("来源配置已变化，请重新读取后保存")
		}
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return TemplateLibrary{}, err
	}
	library, err := s.readTemplates(ctx, target)
	if err != nil {
		return library, err
	}
	if input.Revision != library.Revision {
		return library, configstore.ErrWorkbenchVersion
	}
	source.ID, source.Name, source.SyncedAt = input.ID, input.Name, time.Now().UTC().Format(time.RFC3339)
	if input.ID == "" {
		source.ID = newID()
		source.Revision = 1
		library.Items = append(library.Items, source)
	} else {
		found := false
		for i, item := range library.Items {
			if item.ID == input.ID {
				if item.SourceID != source.SourceID {
					return library, errors.New("不能更改模板来源，请创建新模板")
				}
				source.Revision = item.Revision + 1
				library.Items[i] = source
				found = true
				break
			}
		}
		if !found {
			return library, errors.New("待刷新模板不存在")
		}
	}
	library.PreferredID = source.ID
	return s.saveTemplates(ctx, target, library)
}
func (s *Service) ChangeTemplate(ctx context.Context, id string, revision int64, remove bool) (TemplateLibrary, error) {
	ctx, err := targetguard.Capture(ctx, s.private)
	if err != nil {
		return TemplateLibrary{}, err
	}
	ctx, release, err := targetguard.Acquire(ctx, s.private)
	if err != nil {
		return TemplateLibrary{}, err
	}
	defer release()
	ctx, err = targetguard.Bind(ctx, s.private)
	if err != nil {
		return TemplateLibrary{}, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return TemplateLibrary{}, err
	}
	library, err := s.readTemplates(ctx, target)
	if err != nil {
		return library, err
	}
	if revision != library.Revision {
		return library, configstore.ErrWorkbenchVersion
	}
	found := id == "" && !remove
	for i, item := range library.Items {
		if item.ID == id {
			found = true
			if remove {
				library.Items = append(library.Items[:i], library.Items[i+1:]...)
			}
			break
		}
	}
	if !found {
		return library, errors.New("模板不存在，请刷新")
	}
	if remove {
		if library.PreferredID == id {
			library.PreferredID = ""
		}
	} else {
		library.PreferredID = id
	}
	return s.saveTemplates(ctx, target, library)
}
func (s *Service) saveTemplates(ctx context.Context, target configstore.TargetSettings, library TemplateLibrary) (TemplateLibrary, error) {
	if _, err := targetguard.Pin(targetguard.Expect(ctx, target), s.private); err != nil {
		return library, err
	}
	raw, err := json.Marshal(library)
	if err != nil {
		return library, err
	}
	revision, err := s.private.SaveWorkbenchDocument(ctx, "templates:"+targetKey(target), library.Revision, raw)
	library.Revision = revision
	return library, err
}
