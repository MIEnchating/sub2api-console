package accountworkbench

import (
	"encoding/base32"
	"errors"
	"regexp"
	"strings"

	"github.com/MIEnchating/sub2api-console/backend/internal/workbenchprovider"
)

type LoginDetails struct {
	Email    string                       `json:"-"`
	Password string                       `json:"-"`
	TOTP     string                       `json:"-"`
	Mail     workbenchprovider.MailConfig `json:"-"`
}

var microsoftID = regexp.MustCompile(`(?i)^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$`)

// ParseLoginDetails recognizes the formats accepted by the workbench input,
// without guessing among multiple emails or silently dropping private fields.
func ParseLoginDetails(source string) (LoginDetails, error) {
	result := LoginDetails{}
	fields := splitLoginFields(strings.TrimSpace(source))
	if len(fields) > 16 {
		return result, errors.New("登录资料字段过多，请检查分隔符")
	}
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}
	if len(fields) == 4 && validEmail(fields[0]) && microsoftID.MatchString(fields[2]) && fields[3] != "" {
		return LoginDetails{Email: fields[0], Password: fields[1], Mail: workbenchprovider.MailConfig{Kind: "microsoft", Email: fields[0], ClientID: fields[2], RefreshToken: fields[3]}}, nil
	}
	plain := []string{}
	empty := false
	for _, field := range fields {
		switch {
		case field == "":
			empty = true
		case validEmail(field):
			if result.Email != "" {
				return result, errors.New("同一行包含多个邮箱，请每行仅输入一个账号")
			}
			result.Email = field
		case strings.HasPrefix(field, "https://") || strings.HasPrefix(field, "http://"):
			if result.Mail.Kind != "" || workbenchprovider.ValidateURL(field) != nil {
				return result, errors.New("邮箱收码地址无效，请使用公开 HTTPS 地址")
			}
			result.Mail = workbenchprovider.MailConfig{Kind: "http", URL: field}
		default:
			plain = append(plain, field)
		}
	}
	if result.Email == "" {
		return result, errors.New("登录资料缺少完整邮箱")
	}
	if len(plain) > 0 {
		candidate := strings.ToUpper(strings.ReplaceAll(plain[len(plain)-1], " ", ""))
		secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.TrimRight(candidate, "="))
		if err == nil && len(secret) >= 10 && len(secret) <= 64 && (len(plain) > 1 || result.Mail.Kind != "" || empty) {
			result.TOTP = strings.TrimRight(candidate, "=")
			plain = plain[:len(plain)-1]
		}
	}
	if len(plain) > 1 {
		return result, errors.New("登录资料存在无法确定的字段，请使用 邮箱----密码----2FA密钥 或 邮箱----收码地址")
	}
	if len(plain) == 1 {
		result.Password = plain[0]
	}
	if strings.ContainsAny(result.Password, "\r\n\x00") || len(result.Password) > 4096 {
		return result, errors.New("登录密码格式无效")
	}
	return result, nil
}

func splitLoginFields(source string) []string {
	for _, delimiter := range []string{"----", "---", "\t", "|", "--"} {
		if !strings.Contains(source, delimiter) {
			continue
		}
		fields := strings.Split(source, delimiter)
		for _, field := range fields {
			if validEmail(strings.TrimSpace(field)) {
				return fields
			}
		}
	}
	return []string{source}
}
