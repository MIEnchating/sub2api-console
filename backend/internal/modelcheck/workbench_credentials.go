package modelcheck

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin/loginproxy"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

type OAuthCredentialSource interface {
	GenerationCredentials(context.Context, configstore.TargetSettings, map[string]any) (map[string]any, string, error)
}

func (s *Service) UseOAuthCredentialSource(source OAuthCredentialSource) {
	s.profilesMu.Lock()
	defer s.profilesMu.Unlock()
	s.oauthCredentialSource = source
}

func (s *Service) workbenchOAuthCredential(ctx context.Context, target configstore.TargetSettings, remote map[string]any, account selectedAccount) (*oauthCredential, error) {
	s.profilesMu.RLock()
	source := s.oauthCredentialSource
	s.profilesMu.RUnlock()
	if source == nil {
		return nil, nil
	}
	credentials, proxyURL, err := source.GenerationCredentials(ctx, target, remote)
	if err != nil {
		return nil, err
	}
	if credentials == nil {
		return nil, nil
	}
	credential, err := parseOAuthCredential(credentials)
	if err != nil {
		return nil, err
	}
	if err := loginproxy.Validate(proxyURL); err != nil {
		return nil, errors.New("工作台检测代理无效，请检查登录 / 检测代理设置")
	}
	for _, secret := range credential.secrets {
		if strings.Contains(account.Name, secret) {
			return nil, errors.New("OAuth 账号名称包含凭据，请修改账号名称后重试")
		}
	}
	credential.proxyURL = proxyURL
	return &credential, nil
}

func (s *Service) animationOAuthTransport(ctx context.Context, timeout int, credential *oauthCredential) (http.RoundTripper, func(), error) {
	s.profilesMu.RLock()
	transport := s.oauthTransport
	s.profilesMu.RUnlock()
	if transport != nil {
		return transport, func() {}, nil
	}
	if strings.TrimSpace(credential.proxyURL) == "" {
		direct := newOAuthDirectTransport(timeout)
		return direct, direct.CloseIdleConnections, nil
	}
	raw, err := loginproxy.SessionURL(credential.proxyURL)
	if err != nil {
		return nil, nil, errors.New("工作台检测代理配置无效")
	}
	resolve, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	address, err := loginproxy.PublicAddress(resolve, "chatgpt.com")
	if err != nil {
		return nil, nil, errors.New("检测目标解析失败，请检查网络后重试")
	}
	dialer, err := loginproxy.NewDialer(resolve, loginproxy.Config{UpstreamURL: raw, Destinations: map[string]string{"chatgpt.com:443": net.JoinHostPort(address, "443")}})
	if err != nil {
		return nil, nil, errors.New("工作台检测代理连接配置失败，请检查代理后重试")
	}
	proxied := dialer.Transport()
	return proxied, proxied.CloseIdleConnections, nil
}
