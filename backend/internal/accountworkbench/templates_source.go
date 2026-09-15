package accountworkbench

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
	"github.com/MIEnchating/sub2api-console/backend/internal/redact"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

func validTemplateSourceID(id string) bool {
	number, err := strconv.ParseInt(id, 10, 64)
	return err == nil && number > 0 && number <= 9_007_199_254_740_991 && strconv.FormatInt(number, 10) == id
}

func (s *Service) readTemplateSource(ctx context.Context, id string, target configstore.TargetSettings) (TemplateSource, []string, error) {
	client, err := s.clientFor(target)
	if err != nil {
		return TemplateSource{}, nil, err
	}
	account, err := client.Account(ctx, id)
	if err != nil {
		return TemplateSource{}, nil, publicError(err)
	}
	if stringValue(account["id"]) != id {
		return TemplateSource{}, nil, errors.New("管理接口返回的来源账号 ID 不符，请重新同步账号")
	}
	config, err := ExtractTemplate(account)
	if err != nil {
		return TemplateSource{}, nil, err
	}
	secrets := templateSourceSecrets(account, target.AdminKey)
	plan := strings.ToLower(strings.TrimSpace(stringValue(inputObject(account["credentials"])["plan_type"])))
	if plan == "" {
		plan = strings.ToLower(strings.TrimSpace(stringValue(inputObject(account["extra"])["plan_type"])))
	}
	if len(plan) > 64 || strings.ContainsAny(plan, "\r\n\x00") || templateTextHasSecret(plan, secrets) {
		return TemplateSource{}, nil, errors.New("来源账号的套餐信息无效，请先核对账号")
	}
	source := TemplateSource{AccountID: id, AccountName: safeTemplateSourceName(stringValue(account["name"]), secrets), Config: publicConfig(config), Target: target.BaseURL, Match: configstore.WorkbenchTemplateMatch{PlanType: plan}, SyncedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if plan != "" {
		source.Priority = 10
	}
	if err := validatePublicTemplate(TemplateInput{Config: config}, secrets); err != nil {
		return TemplateSource{}, nil, err
	}
	// The revision excludes runtime counters and OAuth tokens, and binds the
	// transferred settings to the exact management identity and stable source.
	raw, err := json.Marshal(struct {
		Target      string
		AccountID   string
		AccountName string
		Match       configstore.WorkbenchTemplateMatch
		Config      configstore.WorkbenchTemplateConfig
	}{Target: executionTargetFingerprint(target), AccountID: id, AccountName: source.AccountName, Match: source.Match, Config: config})
	if err != nil {
		return TemplateSource{}, nil, errors.New("来源账号配置无法生成预览版本")
	}
	digest := sha256.Sum256(raw)
	source.SourceRevision = hex.EncodeToString(digest[:])
	if _, err := targetguard.Pin(targetguard.Expect(ctx, target), s.private); err != nil {
		return TemplateSource{}, nil, err
	}
	return source, secrets, nil
}

func templateSourceSecrets(account map[string]any, adminKey string) []string {
	secrets := []string{}
	if adminKey != "" {
		secrets = append(secrets, adminKey)
	}
	var visit func(any, bool)
	visit = func(value any, sensitive bool) {
		switch item := value.(type) {
		case string:
			if sensitive && item != "" {
				secrets = append(secrets, item)
			}
		case map[string]any:
			for key, child := range item {
				lower := strings.ToLower(key)
				if lower == "token_type" {
					continue
				}
				secret := sensitive || strings.Contains(lower, "token") || strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "cookie") || strings.Contains(lower, "authorization") || lower == "api_key"
				visit(child, secret)
			}
		case []any:
			for _, child := range item {
				visit(child, sensitive)
			}
		}
	}
	visit(account["credentials"], false)
	return secrets
}

func safeTemplateSourceName(name string, secrets []string) string {
	name = redact.Secrets(name)
	for _, secret := range secrets {
		if secret != "" {
			name = strings.ReplaceAll(name, secret, "[已隐藏]")
		}
	}
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, name))
	if runes := []rune(name); len(runes) > 200 {
		return string(runes[:200])
	}
	return name
}

func templateTextHasSecret(text string, secrets []string) bool {
	if redact.Secrets(text) != text {
		return true
	}
	for _, secret := range secrets {
		if secret != "" && strings.Contains(text, secret) {
			return true
		}
	}
	return false
}

func validatePublicTemplate(input TemplateInput, secrets []string) error {
	if templateTextHasSecret(input.Name, secrets) || templateTextHasSecret(input.Match.PlanType, secrets) || templateTextHasSecret(input.Match.EmailDomain, secrets) {
		return errors.New("模板名称或匹配规则包含授权凭据，请清理后重试")
	}
	var contains func(any) bool
	contains = func(value any) bool {
		switch item := value.(type) {
		case string:
			return templateTextHasSecret(item, secrets)
		case map[string]any:
			for key, child := range item {
				if templateTextHasSecret(key, secrets) || contains(child) {
					return true
				}
			}
		case []any:
			for _, child := range item {
				if contains(child) {
					return true
				}
			}
		}
		return false
	}
	for _, raw := range input.Config {
		value, err := decodeInputJSON(string(raw))
		if err != nil {
			return errors.New("模板配置不是有效 JSON")
		}
		if contains(value) {
			return errors.New("模板可复制配置中包含授权凭据，请清理来源或输入后重试")
		}
	}
	return nil
}
