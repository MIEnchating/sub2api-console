package accountworkbench

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/workbenchprovider"
	"github.com/pquerna/otp/totp"
	"golang.org/x/net/http/httpguts"
)

var batchHyphenRun = regexp.MustCompile(`-{4,}`)
var batchTOTPPattern = regexp.MustCompile(`^[A-Z2-7]{16,128}$`)
var batchMicrosoftID = regexp.MustCompile(`(?i)^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$`)

// ParseOAuthLogins returns private execution input, never an API response. One
// invalid item rejects the entire batch. Text error indices retain physical
// line positions; JSON error indices refer to the original array entries.
func ParseOAuthLogins(content string) ([]OAuthLoginInput, []InputError) {
	if len(content) > maxInputBytes {
		return nil, []InputError{{Message: "单批授权输入不能超过 2 MiB"}}
	}
	if !utf8.ValidString(content) {
		return nil, []InputError{{Message: "输入不是有效的 UTF-8 文本，请转换编码后重试"}}
	}
	content = strings.TrimPrefix(content, "\ufeff")
	raw := strings.TrimSpace(content)
	if raw == "" {
		return nil, []InputError{{Message: "请填写至少一个授权账号"}}
	}
	var items []OAuthLoginInput
	var positions []int
	var failures []InputError
	if strings.HasPrefix(raw, "[") || strings.HasPrefix(raw, "{") {
		items, positions, failures = parseOAuthJSON(raw)
	} else {
		for lineIndex, line := range strings.Split(content, "\n") {
			line = strings.TrimSuffix(line, "\r")
			if strings.TrimSpace(line) == "" {
				continue
			}
			if len(items)+len(failures) >= maxInputItems {
				failures = append(failures, InputError{Index: lineIndex, Message: "单批最多授权 500 个账号"})
				break
			}
			item, err := parseOAuthTextLine(line)
			if err != nil {
				failures = append(failures, InputError{Index: lineIndex, Message: err.Error()})
				continue
			}
			items = append(items, item)
			positions = append(positions, lineIndex)
		}
	}
	seen := make(map[string]bool, len(items))
	for index := range items {
		item, err := validateOAuthBatchLogin(items[index])
		if err != nil {
			failures = append(failures, InputError{Index: positions[index], Message: err.Error()})
			continue
		}
		identity := strings.ToLower(item.Email) + "\x00" + item.WorkspaceID
		if seen[identity] {
			failures = append(failures, InputError{Index: positions[index], Message: "该邮箱与工作区在本批重复，请删除重复项；不同凭据不会覆盖先前项目"})
		}
		seen[identity] = true
		items[index] = item
	}
	if len(failures) > 0 {
		return nil, failures
	}
	return items, []InputError{}
}

func parseOAuthJSON(raw string) ([]OAuthLoginInput, []int, []InputError) {
	var entries []json.RawMessage
	decoder := json.NewDecoder(strings.NewReader(raw))
	if decoder.Decode(&entries) != nil {
		return nil, nil, []InputError{{Message: "授权 JSON 必须是账号对象数组，请检查括号、引号和字段类型"}}
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return nil, nil, []InputError{{Message: "授权 JSON 数组后不能包含其他内容"}}
	}
	if len(entries) == 0 || len(entries) > maxInputItems {
		return nil, nil, []InputError{{Message: "单批授权 JSON 必须包含 1 至 500 个账号"}}
	}
	items := make([]OAuthLoginInput, 0, len(entries))
	positions := make([]int, 0, len(entries))
	var failures []InputError
	for index, entry := range entries {
		var item OAuthLoginInput
		decoder := json.NewDecoder(bytes.NewReader(entry))
		decoder.DisallowUnknownFields()
		if duplicateOAuthJSONKeys(entry) || !validOAuthJSONShape(entry, reflect.TypeOf(item)) || decoder.Decode(&item) != nil {
			failures = append(failures, InputError{Index: index, Message: "账号 JSON 含未知或重复字段，或字段类型不符；请按授权输入字段填写"})
			continue
		}
		items = append(items, item)
		positions = append(positions, index)
	}
	return items, positions, failures
}

func parseOAuthTextLine(line string) (OAuthLoginInput, error) {
	var candidates [][]string
	if strings.Contains(line, "----") {
		fields := strings.Split(line, "----")
		if batchEmailAnchor(fields[0]) {
			for _, run := range batchHyphenRun.FindAllString(line, -1) {
				if len(run)%4 != 0 {
					return OAuthLoginInput{}, errors.New("连续短横线的数量存在歧义，请使用 JSON 或四个短横线明确分隔字段")
				}
			}
			candidates = append(candidates, fields)
		}
	}
	for _, delimiter := range []rune{'\t', '|'} {
		if !strings.ContainsRune(line, delimiter) {
			continue
		}
		reader := csv.NewReader(strings.NewReader(line))
		reader.Comma, reader.FieldsPerRecord = delimiter, -1
		fields, err := reader.Read()
		if err != nil || len(fields) < 2 || !batchEmailAnchor(fields[0]) {
			continue
		}
		if _, err := reader.Read(); err != io.EOF {
			continue
		}
		candidates = append(candidates, fields)
	}
	if len(candidates) == 0 {
		email := strings.TrimSpace(line)
		if batchAuthValid("email", email) {
			return OAuthLoginInput{Email: email}, nil
		}
		return OAuthLoginInput{}, errors.New("无法确定邮箱和凭据字段，请使用 JSON 或以四个短横线明确分隔，邮箱放在首字段")
	}
	for _, candidate := range candidates[1:] {
		if !reflect.DeepEqual(candidates[0], candidate) {
			return OAuthLoginInput{}, errors.New("输入存在多种字段分隔方式，请使用 JSON 或四个短横线明确分隔")
		}
	}
	return oauthLoginFromFields(candidates[0])
}

func oauthLoginFromFields(fields []string) (OAuthLoginInput, error) {
	if len(fields) < 2 || len(fields) > 4 {
		return OAuthLoginInput{}, errors.New("授权文本需要 2 至 4 个字段，复杂密码或请求配置请改用 JSON")
	}
	item := OAuthLoginInput{Email: strings.TrimSpace(fields[0])}
	if len(fields) == 4 && batchMicrosoftID.MatchString(strings.TrimSpace(fields[2])) {
		credential, err := workbenchprovider.ParseMicrosoftCredentialLine(strings.Join(fields, "----"))
		if err != nil {
			return item, errors.New("Microsoft 四字段凭据无法明确解析，请检查客户端 ID 和刷新令牌，包含分隔符的凭据请改用 JSON")
		}
		item.Email, item.Password = credential.Email, credential.Password
		item.Mailbox = &OAuthMailboxInput{Kind: "microsoft", Email: credential.Email, ClientID: credential.ClientID, RefreshToken: credential.RefreshToken}
		return item, nil
	}
	if looksLikeOAuthMailbox(fields[1]) {
		if len(fields) == 4 {
			return item, errors.New("邮箱接口放在第二字段时最多追加一个 2FA 字段，其他配置请改用 JSON")
		}
		item.Mailbox = &OAuthMailboxInput{Kind: "http", URL: strings.TrimSpace(fields[1]), Method: http.MethodGet}
		if len(fields) == 3 {
			item.TOTPSecret = fields[2]
		}
		return item, nil
	}
	item.Password = fields[1]
	if len(fields) == 2 {
		return item, nil
	}
	if looksLikeOAuthMailbox(fields[2]) {
		item.Mailbox = &OAuthMailboxInput{Kind: "http", URL: strings.TrimSpace(fields[2]), Method: http.MethodGet}
	} else if len(fields) == 3 {
		item.TOTPSecret = fields[2]
	} else if strings.TrimSpace(fields[2]) != "" {
		return item, errors.New("四字段格式应为邮箱、密码、HTTPS 邮箱接口和 2FA 密钥；Microsoft 凭据需有效客户端 ID，请使用 JSON 消除歧义")
	}
	if len(fields) == 4 {
		item.TOTPSecret = fields[3]
	}
	return item, nil
}

func validateOAuthBatchLogin(item OAuthLoginInput) (OAuthLoginInput, error) {
	if err := browserlogin.ValidateProxyURL(item.ProxyURL); err != nil {
		return item, err
	}
	item.Email, item.WorkspaceID = strings.TrimSpace(item.Email), strings.TrimSpace(item.WorkspaceID)
	if !batchAuthValid("email", item.Email) {
		return item, errors.New("邮箱格式无效，请填写完整登录邮箱")
	}
	if item.Password != "" && !batchAuthValid("password", item.Password) {
		return item, errors.New("密码包含不支持的控制字符或超过长度限制，请检查密码")
	}
	if item.WorkspaceID != "" && !batchAuthValid("workspace", item.WorkspaceID) {
		return item, errors.New("工作区 ID 格式无效，请填写官方稳定 ID")
	}
	if item.TOTPSecret != "" {
		secret := strings.Map(func(value rune) rune {
			if unicode.IsSpace(value) {
				return -1
			}
			return unicode.ToUpper(value)
		}, item.TOTPSecret)
		secret = strings.TrimRight(secret, "=")
		if !batchTOTPPattern.MatchString(secret) {
			return item, errors.New("2FA 密钥格式无效，请填写 16 至 128 位 Base32 密钥；若该字段是密码，请改用 JSON 明确字段")
		}
		if _, err := totp.GenerateCode(secret, time.Unix(0, 0)); err != nil {
			return item, errors.New("2FA 密钥无法解码，请检查 Base32 内容和长度")
		}
		item.TOTPSecret = secret
	}
	if item.Mailbox == nil && item.SMS == nil {
		return item, nil
	}
	client := workbenchprovider.NewHTTP(nil)
	defer client.Close()
	if item.Mailbox != nil {
		mailbox := *item.Mailbox
		mailbox.Kind, mailbox.URL, mailbox.Email = strings.TrimSpace(mailbox.Kind), strings.TrimSpace(mailbox.URL), strings.TrimSpace(mailbox.Email)
		mailbox.Method = strings.ToUpper(strings.TrimSpace(mailbox.Method))
		if mailbox.Kind == "http" && mailbox.Method == "" {
			mailbox.Method = http.MethodGet
		}
		if mailbox.Kind == "microsoft" && !strings.EqualFold(mailbox.Email, item.Email) {
			return item, errors.New("Microsoft 收码邮箱必须与该项登录邮箱一致")
		}
		headerNames := map[string]bool{}
		for name, value := range mailbox.Headers {
			canonical := http.CanonicalHeaderKey(name)
			if headerNames[canonical] || !httpguts.ValidHeaderFieldName(name) || !httpguts.ValidHeaderFieldValue(value) {
				return item, errors.New("邮箱请求头格式无效或名称重复，请检查请求头配置")
			}
			headerNames[canonical] = true
		}
		box, err := workbenchprovider.NewMailbox(workbenchprovider.MailConfig{Kind: mailbox.Kind, URL: mailbox.URL, Method: mailbox.Method, Headers: mailbox.Headers, Body: mailbox.Body, Email: mailbox.Email, ClientID: mailbox.ClientID, RefreshToken: mailbox.RefreshToken}, client, nil)
		if err != nil {
			return item, errors.New("邮箱收码配置无效，请检查公开 HTTPS 地址、请求配置或 Microsoft 凭据")
		}
		box.Close()
		item.Mailbox = &mailbox
	}
	if item.SMS != nil {
		provider, err := workbenchprovider.NewSMS(item.SMS.config(), client)
		if err != nil {
			return item, errors.New("短信收码配置无效，请检查供应商设置或自定义号码列表")
		}
		provider.Close()
	}
	return item, nil
}

func batchEmailAnchor(raw string) bool {
	value := strings.TrimSpace(raw)
	_, domain, _ := strings.Cut(value, "@")
	return strings.Contains(domain, ".") && batchAuthValid("email", value)
}

func batchAuthValid(stage, value string) bool {
	return (browserlogin.AuthAction{Stage: stage, Revision: strings.Repeat("0", 64), Value: value}).Validate() == nil
}

func looksLikeOAuthMailbox(raw string) bool {
	value := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(value, "http:") || strings.HasPrefix(value, "https:")
}

// Keep the accepted JSON field names aligned with the actual input structs.
// Scalar nulls and case-folded aliases are not valid typed credentials.
func validOAuthJSONShape(raw json.RawMessage, shape reflect.Type) bool {
	null := bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
	if shape.Kind() == reflect.Pointer {
		return null || validOAuthJSONShape(raw, shape.Elem())
	}
	switch shape.Kind() {
	case reflect.Struct:
		if null {
			return false
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			return false
		}
		known := make(map[string]reflect.Type, shape.NumField())
		for index := range shape.NumField() {
			field := shape.Field(index)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name != "" && name != "-" {
				known[name] = field.Type
			}
		}
		for name, value := range fields {
			field, exists := known[name]
			if !exists || !validOAuthJSONShape(value, field) {
				return false
			}
		}
	case reflect.Map:
		if null {
			return true
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			return false
		}
		for _, value := range fields {
			if !validOAuthJSONShape(value, shape.Elem()) {
				return false
			}
		}
	default:
		return !null
	}
	return true
}

func duplicateOAuthJSONKeys(raw json.RawMessage) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return scanOAuthJSONKeys(decoder, 0) != nil
}

func scanOAuthJSONKeys(decoder *json.Decoder, depth int) error {
	if depth > 32 {
		return errors.New("JSON nesting limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	seen := map[string]bool{}
	for decoder.More() {
		if delimiter == '{' {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, valid := key.(string)
			if !valid || seen[name] {
				return errors.New("JSON duplicate key")
			}
			seen[name] = true
		}
		if err := scanOAuthJSONKeys(decoder, depth+1); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}
