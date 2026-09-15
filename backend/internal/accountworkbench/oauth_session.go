package accountworkbench

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

func (s *Service) oauthSession(owner, id string) (*oauthSession, error) {
	s.oauthMu.Lock()
	value := s.oauthSessions[id]
	s.oauthMu.Unlock()
	if value == nil || value.owner != owner || time.Now().After(value.expires) {
		return nil, browserlogin.ErrSession
	}
	return value, nil
}

func (s *Service) ReadOAuth(ctx context.Context, owner, id string) (OAuthView, error) {
	value, err := s.oauthSession(owner, id)
	if err != nil {
		return OAuthView{}, err
	}
	value.op.Lock()
	defer value.op.Unlock()
	value.mu.Lock()
	view, browser := value.view, value.browser
	value.mu.Unlock()
	if view.Status == "waiting" && browser != nil {
		shot, err := browser.Screenshot(ctx)
		if err != nil {
			return OAuthView{}, errors.New("授权画面读取失败，请重试")
		}
		view.Image = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(shot)
	}
	return view, nil
}

func (s *Service) InputOAuth(ctx context.Context, owner, id string, input browserlogin.Input) error {
	if err := input.Validate(); err != nil {
		return err
	}
	value, err := s.oauthSession(owner, id)
	if err != nil {
		return err
	}
	value.op.Lock()
	defer value.op.Unlock()
	value.mu.Lock()
	browser, ready := value.browser, value.view.Status == "waiting"
	value.assistPaused = true
	value.mu.Unlock()
	if browser == nil || !ready {
		return errors.New("授权浏览器尚未就绪或已结束")
	}
	if err := browser.Input(ctx, input); err != nil {
		return errors.New("授权页面操作失败，请刷新画面后重试")
	}
	return nil
}

func (s *Service) FinishOAuth(owner, id string) error {
	value, err := s.oauthSession(owner, id)
	if err != nil {
		return err
	}
	value.mu.Lock()
	defer value.mu.Unlock()
	if value.view.Status != "waiting" {
		return errors.New("授权浏览器尚未就绪或正在验证")
	}
	value.view.Status, value.view.Message = "verifying", "正在验证授权结果"
	select {
	case value.finish <- struct{}{}:
	default:
	}
	return nil
}

func (s *Service) CancelOAuth(owner, id string) error {
	value, err := s.oauthSession(owner, id)
	if err != nil {
		return err
	}
	value.op.Lock()
	defer value.op.Unlock()
	value.mu.Lock()
	value.explicitCancel = true
	value.mu.Unlock()
	if err := s.discardAutomaticCheckpoint(value); err != nil {
		return err
	}
	s.removeOAuth(owner, id)
	return nil
}

// Generic task cancellation must retain the explicit-cancel cleanup semantics.
func (s *Service) CancelOAuthTask(id string) (bool, error) {
	s.oauthMu.Lock()
	value := s.oauthSessions[id]
	s.oauthMu.Unlock()
	if value == nil {
		return false, nil
	}
	return true, s.CancelOAuth(value.owner, id)
}

func (s *Service) removeOAuth(owner, id string) {
	s.oauthMu.Lock()
	value := s.oauthSessions[id]
	if value == nil || value.owner != owner {
		s.oauthMu.Unlock()
		return
	}
	delete(s.oauthSessions, id)
	s.oauthMu.Unlock()
	value.mu.Lock()
	value.view.Status, value.view.Message = "cancelled", "授权已关闭"
	value.credentials = nil
	if value.cancel != nil {
		value.cancel()
	}
	if value.timer != nil {
		value.timer.Stop()
	}
	previews := append([]string(nil), value.previewIDs...)
	value.mu.Unlock()
	for _, previewID := range previews {
		s.DeletePreview(owner, previewID)
	}
}

type OAuthPreviewInput struct {
	Scope            ExportScope `json:"scope,omitempty"`
	ExportOnly       bool        `json:"export_only,omitempty"`
	TemplateID       string      `json:"template_id"`
	CheckAfterImport bool        `json:"check_after_import"`
	Model            string      `json:"model"`
}

func (s *Service) PreviewOAuth(ctx context.Context, owner, id string, input OAuthPreviewInput) (Preview, error) {
	value, err := s.oauthSession(owner, id)
	if err != nil {
		return Preview{}, err
	}
	scope, err := normalizeWorkbenchScope(input.Scope)
	if err != nil {
		return Preview{}, err
	}
	if value.scope == ScopeLocalExport && (scope != ScopeLocalExport || !input.ExportOnly) {
		return Preview{}, errors.New("独立授权仅能生成独立私有文件，请重新选择导出范围")
	}
	if value.scope != ScopeLocalExport && scope == ScopeLocalExport {
		return Preview{}, errors.New("授权范围与预览不一致，请重新启动独立授权")
	}
	value.op.Lock()
	defer value.op.Unlock()
	value.mu.Lock()
	if value.view.Status != "authorized" || value.credentials == nil {
		value.mu.Unlock()
		return Preview{}, errors.New("账号尚未完成授权，请先完成登录")
	}
	raw, err := json.Marshal(value.credentials)
	value.mu.Unlock()
	if err != nil {
		return Preview{}, errors.New("授权凭据无效，请重新授权")
	}
	if scope != ScopeLocalExport {
		ctx = targetguard.Expect(ctx, value.target)
	}
	preview, err := s.Preview(ctx, owner, PreviewInput{Scope: scope, ExportOnly: input.ExportOnly, Content: string(raw), TemplateID: input.TemplateID, CheckAfterImport: input.CheckAfterImport, Model: input.Model})
	if err != nil {
		return Preview{}, err
	}
	value.mu.Lock()
	defer value.mu.Unlock()
	if value.view.Status != "authorized" || time.Now().After(value.expires) {
		s.DeletePreview(owner, preview.ID)
		return Preview{}, browserlogin.ErrSession
	}
	value.previewIDs = append(value.previewIDs, preview.ID)
	return preview, nil
}
