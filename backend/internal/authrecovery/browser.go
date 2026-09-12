package authrecovery

import (
	"context"
	"errors"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

func (s *Service) UseBrowserLogin(manager *browserlogin.Manager) { s.browser = manager }
func (s *Service) StartBrowserLogin(ctx context.Context, owner, host, actor string) (browserlogin.View, error) {
	if s.browser == nil {
		return browserlogin.View{}, errors.New("浏览器验证服务未启用")
	}
	host = configstore.CanonicalHost(host)
	record, err := s.recoveryRecord(ctx, host)
	if err != nil {
		return browserlogin.View{}, err
	}
	if !strings.EqualFold(record.UpstreamType, "sub2api") {
		return browserlogin.View{}, errors.New("浏览器验证当前仅支持 Sub2API 上游")
	}
	current, err := s.private.AuthRecord(ctx, record.Host)
	if err != nil {
		return browserlogin.View{}, err
	}
	expected := cloneAuthRecord(*record)
	if current != nil {
		expected = cloneAuthRecord(*current)
	}
	return s.browser.Start(ctx, owner, *record, func(commitCtx context.Context, candidate configstore.AuthRecord) error {
		// Treat browser output as untrusted. The selected stable target and original
		// configuration remain authoritative, including when the worker is replaced.
		if candidate.Host != expected.Host || candidate.BaseURL != expected.BaseURL || candidate.UpstreamType != expected.UpstreamType || candidate.AuthMode != "sub2api_user_token" {
			return errors.New("浏览器登录结果与所选上游不一致，请重新开始验证")
		}
		if err := commitCtx.Err(); err != nil {
			return err
		}
		verifier, ok := s.authenticator.(CaptchaVerifier)
		if !ok {
			return errors.New("鉴权复核服务未就绪")
		}
		if err := verifier.Verify(commitCtx, candidate); err != nil {
			return errors.New("上游凭据复核未通过：" + safeReason(err.Error()))
		}
		if err := s.commitRecoveredAuth(commitCtx, candidate, expected, current != nil, false); err != nil {
			return err
		}
		outcome := successfulOutcome(business.AuthRecoveryOutcome{Host: record.Host, Attempted: true}, "recovered_by_browser", "人工浏览器登录并完成后端复核", "browser", candidate.AuthMode)
		if _, err := s.repository.PersistAuthRecoveryOutcomes(commitCtx, []business.AuthRecoveryOutcome{outcome}, actor); err != nil {
			return errors.New("凭据已保存，但鉴权状态更新失败，请重新同步上游")
		}
		_, _ = s.balances.SyncHost(commitCtx, record.Host, upstreamsync.Scope{Catalog: true, Balance: true}, actor)
		return nil
	})
}
func (s *Service) ReadBrowserLogin(ctx context.Context, owner, id string) (browserlogin.View, error) {
	if s.browser == nil {
		return browserlogin.View{}, browserlogin.ErrSession
	}
	return s.browser.Read(ctx, owner, id)
}
func (s *Service) InputBrowserLogin(ctx context.Context, owner, id string, input browserlogin.Input) error {
	if s.browser == nil {
		return browserlogin.ErrSession
	}
	return s.browser.Input(ctx, owner, id, input)
}
func (s *Service) FinishBrowserLogin(owner, id string) error {
	if s.browser == nil {
		return browserlogin.ErrSession
	}
	return s.browser.Finish(owner, id)
}
func (s *Service) CancelBrowserLogin(owner, id string) error {
	if s.browser == nil {
		return browserlogin.ErrSession
	}
	return s.browser.Cancel(owner, id)
}
