package accountworkbench

import (
	"context"
	"encoding/hex"
	"errors"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/workbenchprovider"
)

type OAuthSMSAttachmentInput struct {
	Scope    ExportScope   `json:"scope,omitempty"`
	Revision string        `json:"revision"`
	SMS      OAuthSMSInput `json:"sms"`
}

type OAuthSMSAttachmentView struct {
	Scope      ExportScope `json:"scope"`
	Stage      string      `json:"stage"`
	Revision   string      `json:"revision"`
	Configured bool        `json:"configured"`
	CanAttach  bool        `json:"can_attach"`
}

func (s *Service) ReadOAuthSMSAttachment(ctx context.Context, owner, id string, scope ExportScope) (OAuthSMSAttachmentView, error) {
	value, err := s.oauthSession(owner, id)
	if err != nil {
		return OAuthSMSAttachmentView{}, err
	}
	value.op.Lock()
	defer value.op.Unlock()
	return s.readOAuthSMSAttachment(ctx, value, scope)
}

// The session operation lock serializes provider attachment with browser input
// and task-owned assistance. No provider operation occurs in this handler.
func (s *Service) AttachOAuthSMS(ctx context.Context, owner, id string, input OAuthSMSAttachmentInput) error {
	value, err := s.oauthSession(owner, id)
	if err != nil {
		return err
	}
	value.op.Lock()
	defer value.op.Unlock()
	current, err := s.readOAuthSMSAttachment(ctx, value, input.Scope)
	if err != nil {
		return err
	}
	if !current.CanAttach || current.Revision != input.Revision {
		return errors.New("当前手机号页面不可附加接码配置，或已经配置及购买号码，请刷新后核对原订单")
	}
	candidate := &oauthAssist{http: workbenchprovider.NewHTTP(s.providerTransport), attempted: make(map[string]bool), smsOnly: true, smsAttachRevision: input.Revision}
	accepted := false
	defer func() {
		if !accepted {
			candidate.close()
		}
	}()
	if err := candidate.prepareSMS(&input.SMS, s.smsPool); err != nil {
		return err
	}
	if err := s.journalOAuthSMSScoped(owner, value.smsOriginTaskID, value.scope, value.target, candidate); err != nil {
		return err
	}
	if err := s.validateOAuthScope(ctx, value); err != nil {
		return err
	}
	value.mu.Lock()
	defer value.mu.Unlock()
	if value.view.Status != "waiting" || value.assist == nil || ctx.Err() != nil {
		return errors.New("授权会话已变化，未附加接码配置")
	}
	candidate.login.Email, candidate.login.WorkspaceID = value.expectedEmail, value.expectedWorkspace
	value.assist.close()
	*value.assist = *candidate
	value.assistPaused = false
	accepted = true
	return nil
}

func (s *Service) readOAuthSMSAttachment(ctx context.Context, value *oauthSession, scope ExportScope) (OAuthSMSAttachmentView, error) {
	if scope == "" {
		scope = value.scope
	}
	scope, err := normalizeWorkbenchScope(scope)
	if err != nil || scope != value.scope {
		return OAuthSMSAttachmentView{}, errors.New("接码配置范围与当前授权不一致")
	}
	if err := s.validateOAuthScope(ctx, value); err != nil {
		return OAuthSMSAttachmentView{}, err
	}
	view := OAuthSMSAttachmentView{Scope: scope}
	value.mu.Lock()
	browser, assist, status := value.browser, value.assist, value.view.Status
	taskID := value.view.ID
	value.mu.Unlock()
	if assist != nil {
		view.Configured = assist.sms != nil || assist.smsOperation != "" || assist.attempted["stage:phone"] || assist.attempted["stage:sms_code"]
	}
	receipts, err := s.smsReceipts(ctx, value.owner, value.scope)
	if err != nil {
		return view, err
	}
	for _, receipt := range receipts {
		if receipt.TaskID == value.smsOriginTaskID || receipt.TaskID == taskID {
			view.Configured = true
		}
	}
	if view.Configured || status != "waiting" || browser == nil || assist == nil || value.profile != nil {
		return view, nil
	}
	automation, ok := browser.(browserlogin.OAuthAutomation)
	if !ok {
		return view, nil
	}
	page, err := automation.InspectAuth(ctx)
	if err != nil {
		return view, errors.New("手机号页面状态读取失败，请刷新授权画面后重试")
	}
	if page.Stage != "phone" {
		view.Stage = "manual"
		return view, nil
	}
	if _, err := hex.DecodeString(page.Revision); err != nil || len(page.Revision) != 64 {
		return view, browserlogin.ErrAuthPageChanged
	}
	view.Stage, view.Revision, view.CanAttach = "phone", page.Revision, true
	return view, nil
}
