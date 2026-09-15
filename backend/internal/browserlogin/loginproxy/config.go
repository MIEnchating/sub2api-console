// Package loginproxy confines browser and OAuth traffic to reviewed HTTPS
// destinations, optionally through a private upstream proxy.
package loginproxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

var ErrConfig = errors.New("登录代理无效，请使用公开地址的 HTTP、HTTPS 或 SOCKS5 代理；会话占位符只能位于用户名")
var ErrConnect = errors.New("登录代理连接或认证失败，请检查代理配置后新建授权会话")
var ErrDestination = errors.New("登录代理不允许访问该目标")

type Resolver func(context.Context, string) (string, error)

func Validate(raw string) error {
	_, err := parse(raw)
	return err
}

// SessionURL expands a username placeholder once when a caller starts a new
// authorization session. Dialing never rotates credentials or retries.
func SessionURL(raw string) (string, error) {
	u, err := parse(raw)
	if err != nil || u == nil {
		return "", err
	}
	if u.User != nil && strings.Contains(u.User.Username(), "{session}") {
		value := make([]byte, 16)
		if _, err := rand.Read(value); err != nil {
			return "", errors.New("代理会话标识生成失败，请重新授权")
		}
		username := strings.ReplaceAll(u.User.Username(), "{session}", hex.EncodeToString(value))
		if password, ok := u.User.Password(); ok {
			u.User = url.UserPassword(username, password)
		} else {
			u.User = url.User(username)
		}
	}
	return u.String(), nil
}

func parse(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, nil
	}
	if len(raw) > 4096 || strings.TrimSpace(raw) != raw || hasControl(raw) {
		return nil, ErrConfig
	}
	u, err := url.Parse(strings.ReplaceAll(raw, "{session}", "%7Bsession%7D"))
	if err != nil || u.Opaque != "" || u.Hostname() == "" || u.Fragment != "" || u.RawQuery != "" || u.ForceQuery || (u.Path != "" && u.Path != "/") {
		return nil, ErrConfig
	}
	switch u.Scheme {
	case "http", "https", "socks5":
	default:
		return nil, ErrConfig
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if !validHost(host) || strings.ContainsAny(u.Host, "{}%") {
		return nil, ErrConfig
	}
	if ip, err := netip.ParseAddr(host); err == nil && !isPublic(ip) {
		return nil, ErrConfig
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return nil, ErrConfig
		}
	} else if strings.HasSuffix(u.Host, ":") {
		return nil, ErrConfig
	}
	if u.User != nil {
		username, password := u.User.Username(), ""
		password, _ = u.User.Password()
		if username == "" || len(username) > 512 || len(password) > 1024 || hasControl(username) || hasControl(password) || strings.Contains(username, ":") || strings.ContainsAny(password, "{}") {
			return nil, ErrConfig
		}
		if strings.Count(username, "{session}") > 1 || strings.ContainsAny(strings.ReplaceAll(username, "{session}", ""), "{}") {
			return nil, ErrConfig
		}
		if u.Scheme == "socks5" && (len(strings.ReplaceAll(username, "{session}", strings.Repeat("s", 32))) > 255 || len(password) > 255) {
			return nil, ErrConfig
		}
	}
	return u, nil
}

func hasControl(value string) bool {
	return strings.ContainsFunc(value, func(r rune) bool { return r < 32 || r == 127 })
}

func validHost(host string) bool {
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.Zone() == ""
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".internal") || len(host) > 253 || !strings.Contains(host, ".") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if r != '-' && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
				return false
			}
		}
	}
	return true
}

var reservedNetworks = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("64:ff9b::/96"), netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2002::/16"), netip.MustParsePrefix("2001::/32"),
}

func isPublic(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, block := range reservedNetworks {
		if block.Contains(ip) {
			return false
		}
	}
	return true
}

func PublicAddress(ctx context.Context, host string) (string, error) {
	if !validHost(strings.TrimSuffix(strings.ToLower(host), ".")) {
		return "", ErrConfig
	}
	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(addresses) == 0 {
		return "", errors.New("登录代理或官方目标域名解析失败，请检查网络后重试")
	}
	for _, address := range addresses {
		if !isPublic(address) {
			return "", errors.New("登录代理和官方目标必须使用公开地址")
		}
	}
	return addresses[0].Unmap().String(), nil
}
