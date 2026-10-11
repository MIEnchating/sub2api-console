package accountworkbench

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

type rememberedSource struct {
	ID             string `json:"id"`
	Revision       int64  `json:"revision"`
	AccountID      string `json:"account_id"`
	AccountVersion string `json:"account_version"`
	Workspace      string `json:"workspace"`
	User           string `json:"user"`
}

// PreviewRememberedLogin creates a new session-bound import preview using the
// existing account's current configuration and its exact verified identity.
func (s *Service) PreviewRememberedLogin(ctx context.Context, owner, id string, revision int64) (Preview, error) {
	if err := s.checkOwner(ctx, owner); err != nil {
		return Preview{}, err
	}
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return Preview{}, err
	}
	saved, err := s.readRememberedAccount(ctx, target, id)
	if err != nil {
		return Preview{}, err
	}
	if revision < 1 || saved.Public.Revision != revision {
		return Preview{}, configstore.ErrWorkbenchVersion
	}
	if saved.Input.LoginSource == "" {
		return Preview{}, errors.New("该账号未保存邮箱登录资料，请从导入页补充登录资料")
	}
	client, err := s.client(target)
	if err != nil {
		return Preview{}, err
	}
	account, err := client.Account(ctx, saved.Public.AccountID)
	if err != nil {
		return Preview{}, errors.New("线上账号读取失败，请检查站点连接")
	}
	if text(account["id"]) != saved.Public.AccountID || !sameAccountIdentity(account, saved.Input.Credentials) {
		return Preview{}, errors.New("线上账号身份已变化，未使用旧登录资料")
	}
	config, err := ExtractConfig(account)
	if err != nil {
		return Preview{}, err
	}
	templateRevision, err := s.templateRevision(ctx, target)
	if err != nil {
		return Preview{}, err
	}
	input := saved.Input
	input.Item.ID = newID()
	input.Item.Kind = "login"
	input.Item.Index = 0
	input.Item.Name = text(account["name"])
	input.Item.Credentials = nil
	input.Credentials = map[string]any{}
	workspace, user := credentialIdentity(saved.Input.Credentials)
	source := &rememberedSource{ID: id, Revision: revision, AccountID: saved.Public.AccountID, AccountVersion: accountVersion(account), Workspace: workspace, User: user}
	result := Preview{ID: newID(), ExpiresAt: time.Now().UTC().Add(previewLifetime), Action: "import", Promote: true, Check: s.checker != nil,
		Template:    &Template{Name: "账号当前配置", SourceID: saved.Public.AccountID, Config: config, Summary: publicAccount(account)},
		ParsedInput: ParsedInput{Items: []InputItem{input.Item}, Errors: []InputError{}}}
	document := privatePreview{Scope: "managed", Owner: owner, Target: target, TemplateRevision: templateRevision, Public: result, Items: []storedInput{input}, RememberedSource: source,
		Settings: PreviewInput{Action: "import", Promote: true, Check: result.Check, Model: "gpt-5.6-sol"}}
	if _, err = targetguard.Pin(targetguard.Expect(ctx, target), s.private); err != nil {
		return Preview{}, err
	}
	raw, err := json.Marshal(document)
	if err != nil {
		return Preview{}, err
	}
	key := "preview:" + owner
	current, err := s.private.WorkbenchDocumentRevision(ctx, key)
	if err != nil {
		return Preview{}, err
	}
	result.Revision, err = s.private.SaveWorkbenchDocument(ctx, key, current, raw)
	return result, err
}

func (s *Service) rememberedSourceAllowed(ctx context.Context, value *privateRun) error {
	source := value.RememberedSource
	if source == nil {
		return nil
	}
	revision, err := s.private.WorkbenchDocumentRevision(ctx, rememberedAccountKey(value.Target, source.ID))
	if err != nil {
		return err
	}
	if revision != source.Revision {
		return errors.New("已记住账号资料已变化或删除，请重新预览")
	}
	return nil
}
