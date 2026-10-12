package accountworkbench

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin/loginproxy"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

// PlatformProxy is the public projection; addresses and authentication never leave Go.
type PlatformProxy struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Protocol  string `json:"protocol"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

type platformProxyBinding struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	Fingerprint string `json:"fingerprint"`
}

func platformProxyURL(row map[string]any) (string, error) {
	if text(row["status"]) != "active" {
		return "", errors.New("代理未启用，请在 Sub2API 平台启用后重新同步")
	}
	if expiry := text(row["expires_at"]); expiry != "" {
		at, err := time.Parse(time.RFC3339, expiry)
		if err != nil || !at.After(time.Now()) {
			return "", errors.New("代理已过期或有效期无效，请在平台更新后重新同步")
		}
	}
	if mode := text(row["fallback_mode"]); mode != "" && mode != "none" {
		return "", errors.New("代理配置了到期切换，请在平台关闭切换后使用，以保持登录与调用出口一致")
	}
	host, protocol := text(row["host"]), text(row["protocol"])
	port, err := strconv.Atoi(text(row["port"]))
	if err != nil || port < 1 || port > 65535 || strings.ContainsAny(host, "/%@?#[]") {
		return "", errors.New("平台代理地址无效，请在平台修正后重新同步")
	}
	address := &url.URL{Scheme: protocol, Host: net.JoinHostPort(host, strconv.Itoa(port))}
	username, password := text(row["username"]), text(row["password"])
	if strings.ContainsAny(username+password, "{}") {
		return "", errors.New("平台代理须使用固定会话凭据，请在平台设置固定出口或粘性会话后重新同步")
	}
	if (username == "") != (password == "") {
		return "", errors.New("平台代理认证信息不完整，请检查平台管理接口权限与代理配置")
	}
	if username != "" {
		address.User = url.UserPassword(username, password)
	}
	raw := address.String()
	if loginproxy.Validate(raw) != nil {
		return "", errors.New("平台代理须使用公开地址的 HTTP、HTTPS 或 SOCKS5 配置")
	}
	return raw, nil
}

func (s *Service) PlatformProxies(ctx context.Context) ([]PlatformProxy, error) {
	target, err := targetguard.Settings(ctx, s.private)
	if err != nil {
		return nil, err
	}
	client, err := s.client(target)
	if err != nil {
		return nil, err
	}
	rows, err := client.Proxies(ctx)
	if err != nil {
		return nil, errors.New("平台代理读取失败，请检查管理连接和权限后重试")
	}
	result := make([]PlatformProxy, 0, len(rows))
	for _, row := range rows {
		item := PlatformProxy{ID: text(row["id"]), Name: text(row["name"]), Protocol: text(row["protocol"]), Available: true}
		if _, err := platformProxyURL(row); err != nil {
			item.Available = false
			item.Reason = err.Error()
		}
		result = append(result, item)
	}
	if _, err = targetguard.Pin(targetguard.Expect(ctx, target), s.private); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) resolvePlatformProxy(ctx context.Context, target configstore.TargetSettings, id string) (*platformProxyBinding, PlatformProxy, error) {
	if id == "" {
		return nil, PlatformProxy{}, nil
	}
	n, err := strconv.ParseUint(id, 10, 63)
	if err != nil || n == 0 || strconv.FormatUint(n, 10) != id {
		return nil, PlatformProxy{}, errors.New("平台代理 ID 无效，请重新选择")
	}
	client, err := s.client(target)
	if err != nil {
		return nil, PlatformProxy{}, err
	}
	row, err := client.Proxy(ctx, id)
	if err != nil {
		return nil, PlatformProxy{}, errors.New("平台代理读取失败或已删除，请重新同步并选择代理")
	}
	raw, err := platformProxyURL(row)
	if err != nil {
		return nil, PlatformProxy{}, err
	}
	if _, err = targetguard.Pin(targetguard.Expect(ctx, target), s.private); err != nil {
		return nil, PlatformProxy{}, err
	}
	binding := &platformProxyBinding{ID: id, URL: raw, Fingerprint: digest(map[string]any{"url": raw, "expires_at": row["expires_at"], "fallback_mode": row["fallback_mode"]})}
	return binding, PlatformProxy{ID: id, Name: text(row["name"]), Protocol: text(row["protocol"]), Available: true}, nil
}

func (s *Service) verifyPlatformProxy(ctx context.Context, target configstore.TargetSettings, binding *platformProxyBinding) error {
	if binding == nil {
		return nil
	}
	current, _, err := s.resolvePlatformProxy(ctx, target, binding.ID)
	if err != nil {
		return err
	}
	if current.Fingerprint != binding.Fingerprint || current.URL != binding.URL {
		return errors.New("平台代理配置已变化，请重新预览或重新登录；未切换代理或直连")
	}
	return nil
}

func (s *Service) accountProxy(ctx context.Context, target configstore.TargetSettings, account map[string]any) (*platformProxyBinding, error) {
	binding, _, err := s.resolvePlatformProxy(ctx, target, text(account["proxy_id"]))
	return binding, err
}

func proxyBindingURL(binding *platformProxyBinding) string {
	if binding == nil {
		return ""
	}
	return binding.URL
}
