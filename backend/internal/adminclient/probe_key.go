package adminclient

import (
	"context"
	"errors"
	"strings"
	"unicode"
)

func resolveDirectProbeKey(ctx context.Context, accountID string, source ProbeCredentialSource, account, credentials map[string]any, resolvers []ProbeKeyResolver) (string, error) {
	secret := directProbeString(credentials["api_key"])
	if UsableProbeKey(secret) {
		return secret, nil
	}
	if secret != "" && strings.ContainsFunc(secret, unicode.IsControl) {
		return "", probeUnavailable("probe_credentials_invalid", "账号 API Key 格式无效，请检查账号凭据")
	}
	if status, ok := account["credentials_status"].(map[string]any); ok && status["has_api_key"] == false {
		return "", probeUnavailable("probe_credentials_missing", "管理平台确认账号未配置 API Key，请先配置账号凭据后重试")
	}
	if len(resolvers) == 0 || resolvers[0] == nil {
		return "", probeUnavailable("probe_credentials_unavailable", "管理接口未返回可用 API Key，需通过账号的稳定上游绑定获取密钥")
	}
	secret, err := resolvers[0](ctx, accountID, source)
	if err != nil {
		var unavailable *ProbeUnavailableError
		if errors.As(err, &unavailable) {
			return "", unavailable
		}
		return "", &ProbeUnavailableError{Code: "probe_credentials_unavailable", Message: "获取账号绑定 Key 失败，请检查上游鉴权与 Key 绑定后重试", cause: ctx.Err()}
	}
	secret = strings.TrimSpace(secret)
	if !UsableProbeKey(secret) {
		return "", probeUnavailable("probe_credentials_unavailable", "账号绑定 Key 未返回有效密钥，请检查上游鉴权与 Key 绑定后重试")
	}
	return secret, nil
}

// UsableProbeKey rejects empty, malformed and redacted credential values before
// they can be mistaken for a cached key or sent in a generation request.
func UsableProbeKey(secret string) bool {
	secret = strings.TrimSpace(secret)
	if secret == "" || strings.ContainsFunc(secret, unicode.IsControl) || strings.ContainsAny(secret, "*…") {
		return false
	}
	switch strings.ToLower(secret) {
	case "[redacted]", "[hidden]", "[已隐藏]":
		return false
	}
	return true
}
