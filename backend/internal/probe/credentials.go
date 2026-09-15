package probe

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/adminclient"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/upstreamsync"
)

type KeyRevealer interface {
	RevealKey(context.Context, configstore.AuthRecord, string, string) (upstreamsync.CreatedKey, error)
}

func (s *Service) UseKeyRevealer(reader KeyRevealer) { s.keyRevealer = reader }

type probeAccountReader interface {
	Account(context.Context, string) (*business.AccountDetail, error)
}

type probeCredentialStore interface {
	UpstreamKeySecret(context.Context, string, string, string) (*configstore.UpstreamKeySecret, error)
	SaveUpstreamKeySecretForAuth(context.Context, configstore.UpstreamKeySecret, configstore.AuthRecord) error
	AuthRecord(context.Context, string) (*configstore.AuthRecord, error)
}

type probeKeySelection struct {
	host     string
	keyID    string
	groupID  string
	baseURL  string
	versions string
}

// The management DTO may omit the API Key. Resolve its exact local binding and
// persist newly obtained keys in the private store after rechecking the binding.
// Subsequent probes, including after a restart, reuse the saved credential.
func (s *Service) resolveProbeKey(ctx context.Context, accountID string, source adminclient.ProbeCredentialSource) (string, error) {
	accounts, ok := s.repository.(probeAccountReader)
	if !ok {
		return "", probeKeyUnavailable("probe_key_binding_unavailable", "管理接口未返回 API Key，且本地账号绑定不可读取，请先同步账号绑定")
	}
	private, ok := s.settings.(probeCredentialStore)
	if !ok {
		return "", probeKeyUnavailable("probe_key_store_unavailable", "管理接口未返回 API Key，且本地凭据库不可读取，请检查控制台凭据配置")
	}
	selection, err := readProbeKeySelection(ctx, accounts, accountID, source.BindingBaseURL)
	if err != nil {
		return "", err
	}
	stored, err := private.UpstreamKeySecret(ctx, selection.host, selection.keyID, selection.groupID)
	if err != nil {
		return "", probeKeyUnavailable("probe_key_store_unavailable", "本地绑定 Key 读取失败，请检查控制台凭据库后重试")
	}
	var secret string
	if stored != nil {
		if configstore.CanonicalHost(stored.Host) != selection.host || stored.KeyID != selection.keyID || stored.GroupID != selection.groupID {
			return "", probeKeyUnavailable("probe_key_binding_mismatch", "本地 Key 与账号绑定不一致，请重新同步账号绑定")
		}
		secret = stored.Secret
	}
	needsSave := !adminclient.UsableProbeKey(secret)
	var sourceAuth configstore.AuthRecord
	if needsSave {
		secret, sourceAuth, err = s.revealProbeKey(ctx, private, selection)
		if err != nil {
			return "", err
		}
	}
	latest, err := readProbeKeySelection(ctx, accounts, accountID, source.BindingBaseURL)
	if err != nil || latest != selection {
		return "", probeKeyUnavailable("probe_key_binding_changed", "读取 Key 期间账号绑定或 Base URL 已变化，请同步后重新探活")
	}
	if needsSave {
		if err := private.SaveUpstreamKeySecretForAuth(ctx, configstore.UpstreamKeySecret{
			Host: selection.host, KeyID: selection.keyID, GroupID: selection.groupID, Secret: secret,
		}, sourceAuth); err != nil {
			if errors.Is(err, configstore.ErrUpstreamKeyAuthChanged) {
				return "", probeKeyUnavailable("probe_key_auth_changed", "读取 Key 期间上游鉴权或 Host 已变化，请核对上游配置后重新探活")
			}
			return "", probeKeyUnavailable("probe_key_save_failed", "绑定 Key 持久保存失败，请检查控制台私有凭据库后重新探活")
		}
	}
	return secret, nil
}

func (s *Service) revealProbeKey(ctx context.Context, private probeCredentialStore, selection probeKeySelection) (string, configstore.AuthRecord, error) {
	if s.keyRevealer == nil {
		return "", configstore.AuthRecord{}, probeKeyUnavailable("probe_key_reader_unavailable", "本地尚未保存有效的绑定 Key，且上游 Key 读取服务不可用，请检查控制台配置")
	}
	record, err := private.AuthRecord(ctx, selection.host)
	if err != nil {
		return "", configstore.AuthRecord{}, probeKeyUnavailable("probe_key_auth_unavailable", "上游授权读取失败，请检查上游鉴权后重试")
	}
	if record == nil || configstore.CanonicalHost(record.Host) != selection.host {
		return "", configstore.AuthRecord{}, probeKeyUnavailable("probe_key_auth_unavailable", "本地尚未保存有效的绑定 Key，且上游缺少匹配的授权，请先完成上游鉴权")
	}
	key, err := s.keyRevealer.RevealKey(ctx, *record, selection.keyID, selection.groupID)
	if err != nil {
		return "", configstore.AuthRecord{}, probeKeyUnavailable("probe_key_read_failed", "无法从上游读取绑定 Key，请检查上游鉴权和 Key 绑定后重试")
	}
	if key.KeyID != selection.keyID || key.GroupID != selection.groupID || !adminclient.UsableProbeKey(key.Secret) {
		return "", configstore.AuthRecord{}, probeKeyUnavailable("probe_key_binding_mismatch", "上游返回的 Key 与账号绑定不一致或密钥不可读，请重新同步账号绑定")
	}
	return key.Secret, *record, nil
}

func readProbeKeySelection(ctx context.Context, accounts probeAccountReader, accountID, liveBaseURL string) (probeKeySelection, error) {
	detail, err := accounts.Account(ctx, accountID)
	if err != nil || detail == nil || detail.ID != accountID {
		return probeKeySelection{}, probeKeyUnavailable("probe_key_binding_unavailable", "无法按账号 ID 核对本地绑定，请同步账号后重试")
	}
	baseURL := ""
	for _, candidate := range []*string{detail.BaseURL, detail.UpstreamBaseURL} {
		if candidate != nil && strings.TrimSpace(*candidate) != "" {
			baseURL = *candidate
			break
		}
	}
	baseURL, valid := normalizedProbeBindingURL(baseURL)
	liveURL, liveValid := normalizedProbeBindingURL(liveBaseURL)
	if !valid || !liveValid || baseURL != liveURL {
		return probeKeySelection{}, probeKeyUnavailable("probe_key_target_changed", "账号当前 Base URL 与本地绑定配置不一致，请同步账号后重新探活")
	}
	selection := probeKeySelection{baseURL: baseURL}
	versions := make([]string, 0, len(detail.Bindings))
	for _, binding := range detail.Bindings {
		if binding.Status != nil && strings.EqualFold(strings.TrimSpace(*binding.Status), "missing") {
			continue
		}
		if binding.LocalAccountID != accountID {
			return probeKeySelection{}, probeKeyUnavailable("probe_key_binding_mismatch", "本地 Key 绑定的账号 ID 不一致，请重新同步账号绑定")
		}
		host := binding.UpstreamHost
		if binding.SourceAuthHost != nil && strings.TrimSpace(*binding.SourceAuthHost) != "" {
			host = *binding.SourceAuthHost
		}
		host = configstore.CanonicalHost(host)
		keyID, groupID := strings.TrimSpace(binding.UpstreamKeyID), ""
		if binding.UpstreamGroupID != nil {
			groupID = strings.TrimSpace(*binding.UpstreamGroupID)
		}
		if host == "" || keyID == "" || groupID == "" {
			continue
		}
		if selection.keyID != "" && (selection.host != host || selection.keyID != keyID || selection.groupID != groupID) {
			return probeKeySelection{}, probeKeyUnavailable("probe_key_binding_ambiguous", "账号存在多个不同 Key 绑定，无法确定探活凭据，请先核对账号绑定")
		}
		selection.host, selection.keyID, selection.groupID = host, keyID, groupID
		versions = append(versions, strconv.FormatInt(binding.ID, 10)+":"+binding.UpdatedAt)
	}
	if selection.keyID == "" {
		return probeKeySelection{}, probeKeyUnavailable("probe_key_binding_missing", "管理接口已隐藏 API Key，且账号没有有效的本地 Key 绑定，请先同步或补充账号绑定")
	}
	sort.Strings(versions)
	selection.versions = strings.Join(versions, "\n")
	return selection, nil
}

func normalizedProbeBindingURL(raw string) (string, bool) {
	normalized, err := configstore.ValidateBaseURL(raw)
	if err != nil {
		return "", false
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", false
	}
	parsed.Host = strings.ToLower(parsed.Host)
	if parsed.Scheme == "https" && parsed.Port() == "443" || parsed.Scheme == "http" && parsed.Port() == "80" {
		parsed.Host = strings.TrimSuffix(parsed.Host, ":"+parsed.Port())
	}
	return parsed.String(), true
}

func probeKeyUnavailable(code, message string) error {
	return &adminclient.ProbeUnavailableError{Code: code, Message: message}
}
