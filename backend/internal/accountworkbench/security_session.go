package accountworkbench

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func (s *Service) securitySession(owner, id string) (*securitySession, error) {
	s.securityMu.Lock()
	value := s.securitySessions[id]
	s.securityMu.Unlock()
	if value == nil || value.owner != owner || !time.Now().Before(value.expires) {
		return nil, browserlogin.ErrSession
	}
	return value, nil
}

func (s *Service) Security(ctx context.Context, owner, id string, screenshot bool) (SecurityView, error) {
	value, err := s.securitySession(owner, id)
	if err != nil {
		return SecurityView{}, err
	}
	value.op.Lock()
	defer value.op.Unlock()
	value.mu.Lock()
	view, browser := cloneSecurityView(value.view), value.browser
	value.mu.Unlock()
	if screenshot && (view.Status == "waiting" || view.Status == "awaiting_confirmation") && browser != nil {
		if err := s.validateSecurityOrigin(ctx, value); err != nil {
			return SecurityView{}, err
		}
		shot, err := browser.Screenshot(ctx)
		if err != nil {
			return SecurityView{}, errors.New("安全浏览器画面读取失败，请重试")
		}
		if len(shot) > 0 {
			view.Image = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(shot)
		}
	}
	return view, nil
}

func cloneSecurityView(view SecurityView) SecurityView {
	if view.SourceIndex != nil {
		index := *view.SourceIndex
		view.SourceIndex = &index
	}
	return view
}

func (s *Service) ReadSecurity(ctx context.Context, owner, id string) (SecurityView, error) {
	return s.Security(ctx, owner, id, true)
}

func (s *Service) InputSecurity(ctx context.Context, owner, id string, input browserlogin.Input) error {
	if err := input.Validate(); err != nil {
		return err
	}
	value, err := s.securitySession(owner, id)
	if err != nil {
		return err
	}
	value.op.Lock()
	defer value.op.Unlock()
	value.mu.Lock()
	browser, ready := value.browser, value.view.Status == "waiting"
	value.mu.Unlock()
	if browser == nil || !ready {
		return errors.New("安全浏览器尚未就绪或正在处理，请稍后重试")
	}
	if err := s.validateSecurityOrigin(ctx, value); err != nil {
		return err
	}
	if err := browser.Input(ctx, input); err != nil {
		return errors.New("官方页面操作失败，请刷新画面后重试")
	}
	return nil
}

func (s *Service) InspectSecurityAuth(ctx context.Context, owner, id string) (browserlogin.AuthPage, error) {
	value, err := s.securitySession(owner, id)
	if err != nil {
		return browserlogin.AuthPage{}, err
	}
	value.op.Lock()
	defer value.op.Unlock()
	value.mu.Lock()
	browser, ready := value.browser, value.view.Status == "waiting"
	value.mu.Unlock()
	if browser == nil || !ready {
		return browserlogin.AuthPage{}, browserlogin.ErrSecurityPending
	}
	if err := s.validateSecurityOrigin(ctx, value); err != nil {
		return browserlogin.AuthPage{}, err
	}
	page, err := browser.InspectAuth(ctx)
	if err != nil {
		return browserlogin.AuthPage{}, errors.New("官方验证步骤读取失败，请刷新后重试")
	}
	return page, nil
}

func (s *Service) SubmitSecurityAuth(ctx context.Context, owner, id string, action browserlogin.AuthAction) error {
	if err := action.Validate(); err != nil {
		return err
	}
	if action.Stage == "new_password" {
		return errors.New("新密码由已确认的安全任务提交，请点击继续")
	}
	value, err := s.securitySession(owner, id)
	if err != nil {
		return err
	}
	value.mu.Lock()
	expectedEmail := value.expected.Email
	value.mu.Unlock()
	if expectedEmail != "" && action.Stage == "email" && !strings.EqualFold(action.Value, expectedEmail) {
		return browserlogin.ErrSecurityIdentity
	}
	return s.queueSecurity(ctx, value, &action)
}

func (s *Service) ContinueSecurity(ctx context.Context, owner, id string) error {
	value, err := s.securitySession(owner, id)
	if err != nil {
		return err
	}
	return s.queueSecurity(ctx, value, nil)
}

func (s *Service) queueSecurity(ctx context.Context, value *securitySession, action *browserlogin.AuthAction) error {
	if err := s.validateSecurityOrigin(ctx, value); err != nil {
		return err
	}
	value.mu.Lock()
	defer value.mu.Unlock()
	if value.view.Status != "waiting" || value.browser == nil {
		return errors.New("安全任务正在处理或已结束，请等待当前步骤完成")
	}
	select {
	case value.commands <- securityCommand{auth: action}:
		value.view.Status, value.view.Message = "running", "正在核对官方身份并处理安全步骤"
		return nil
	default:
		return errors.New("安全任务已在处理，请勿重复提交")
	}
}

func (s *Service) CancelSecurity(owner, id string) error {
	if _, err := s.securitySession(owner, id); err != nil {
		return err
	}
	s.removeSecurity(owner, id)
	return nil
}

func (s *Service) removeSecurity(owner, id string) {
	s.securityMu.Lock()
	value := s.securitySessions[id]
	if value == nil || value.owner != owner {
		s.securityMu.Unlock()
		return
	}
	delete(s.securitySessions, id)
	s.securityMu.Unlock()
	value.mu.Lock()
	value.view.Status, value.view.Message = "cancelled", "账号安全会话已关闭"
	if value.cancel != nil {
		value.cancel()
	}
	if value.timer != nil {
		value.timer.Stop()
	}
	value.mu.Unlock()
}
