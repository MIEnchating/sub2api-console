package accountworkbench

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode"
)

const maxInputBytes = 2 * 1024 * 1024
const maxInputItems = 500

var refreshTokenPattern = regexp.MustCompile(`^rt_[A-Za-z0-9._~-]+$`)

var tokenFields = []string{"access_token", "refresh_token", "id_token"}
var identityFields = []string{"email", "account_id", "chatgpt_account_id", "user_id", "chatgpt_user_id", "workspace_id", "organization_id", "org_id", "plan_type", "subscription_expires_at"}
var tokenMetadataFields = []string{"expires_at", "expires_in", "client_id", "auth_mode", "token_type"}

type inputParser struct {
	items    []InputItem
	failures []InputError
	next     int
}

// Parse accepts Codex/Sub2API JSON and standalone rt_ lines. Indices are
// zero-based and follow nonempty input entries. A batch with any error is not
// executable: no credentials are returned until every entry is valid.
func Parse(content string) ([]InputItem, []InputError) {
	parser := inputParser{items: []InputItem{}, failures: []InputError{}}
	if len(content) > maxInputBytes {
		return nil, []InputError{{Message: "单批输入不能超过 2 MiB"}}
	}
	raw := strings.TrimSpace(content)
	if raw == "" {
		return nil, []InputError{{Message: "请填写账号授权 JSON 或 rt_ 刷新令牌"}}
	}
	if strings.HasPrefix(raw, "[") || strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "\"") {
		value, err := decodeInputJSON(raw)
		if err != nil {
			return nil, []InputError{{Message: "JSON 格式不正确，请检查括号、引号及多余内容"}}
		}
		parser.parseValue(value, false, 0)
	} else {
		for _, line := range strings.Split(raw, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if parser.next >= maxInputItems {
				parser.fail("单批最多导入 500 个账号")
				break
			}
			if refreshTokenPattern.MatchString(line) && len(line) <= 65536 {
				parser.add("refresh_token", map[string]any{"refresh_token": line}, nil)
			} else if strings.Contains(line, "@") {
				parser.fail("邮箱或密码凭据需要先完成账号授权，请使用授权后的 JSON 或 rt_ 刷新令牌导入")
			} else {
				parser.fail("无法识别此项；独立刷新令牌必须以 rt_ 开头")
			}
		}
	}
	if len(parser.items) == 0 && len(parser.failures) == 0 {
		parser.fail("输入中没有可用账号")
	}
	if len(parser.failures) != 0 {
		return nil, parser.failures
	}
	return parser.items, parser.failures
}

func (p *inputParser) fail(message string) {
	p.failures = append(p.failures, InputError{Index: p.next, Message: message})
	p.next++
}

func (p *inputParser) parseValue(value any, accountWrapper bool, depth int) {
	if p.next >= maxInputItems {
		p.fail("单批最多导入 500 个账号")
		return
	}
	if depth > 12 {
		p.fail("账号 JSON 嵌套过深，请只保留账号或令牌对象")
		return
	}
	switch node := value.(type) {
	case []any:
		if len(node) == 0 {
			p.fail("账号列表不能为空")
			return
		}
		for _, child := range node {
			if p.next >= maxInputItems {
				p.fail("单批最多导入 500 个账号")
				break
			}
			p.parseValue(child, accountWrapper, depth+1)
		}
	case string:
		token := strings.TrimSpace(node)
		if !refreshTokenPattern.MatchString(token) || len(token) > 65536 {
			p.fail("JSON 字符串必须是以 rt_ 开头的刷新令牌")
			return
		}
		p.add("refresh_token", map[string]any{"refresh_token": token}, nil)
	case map[string]any:
		p.parseObject(node, accountWrapper, depth)
	default:
		p.fail("账号 JSON 必须包含账号对象、令牌对象或刷新令牌字符串")
	}
}

func (p *inputParser) parseObject(value map[string]any, accountWrapper bool, depth int) {
	if accounts, exists := value["accounts"]; exists {
		if _, ok := accounts.([]any); !ok {
			p.fail("accounts 必须是账号数组")
			return
		}
		p.parseValue(accounts, true, depth+1)
		return
	}
	if data, ok := value["data"].(map[string]any); ok {
		if _, exists := data["accounts"]; exists {
			p.parseValue(data, true, depth+1)
			return
		}
	}
	if platform, exists := value["platform"]; exists && inputText(platform) != "openai" {
		p.fail("仅支持 OpenAI OAuth 账号，不能导入其他平台")
		return
	}
	if kind, exists := value["type"]; exists && inputText(kind) != "oauth" {
		p.fail("仅支持 OAuth 账号，不能导入其他账号类型")
		return
	}
	_, hasCredentials := value["credentials"]
	if hasCredentials || accountWrapper || value["platform"] != nil || value["type"] != nil {
		credentials, ok := value["credentials"].(map[string]any)
		if !ok {
			p.fail("Sub2API 账号缺少有效 credentials 对象")
			return
		}
		p.add("sub2api_json", credentials, value)
		return
	}
	if rt, exists := value["rt"]; exists && value["refresh_token"] == nil {
		copy := cloneInputMap(value)
		copy["refresh_token"] = rt
		p.add("refresh_token", copy, value)
		return
	}
	container := findTokenContainer(value)
	if container == nil {
		p.fail("未识别到 Codex 或 Sub2API 授权结构")
		return
	}
	kind := "codex_json"
	if inputText(container["access_token"]) == "" && container["refresh_token"] != nil {
		kind = "refresh_token"
	}
	p.add(kind, container, value)
}

func (p *inputParser) add(kind string, source, envelope map[string]any) {
	credentials, err := inputCredentials(source)
	if err != nil {
		p.fail(err.Error())
		return
	}
	copyInputIdentity(credentials, envelope)
	if envelope != nil {
		copyInputIdentity(credentials, inputObject(envelope["extra"]))
		copyInputIdentity(credentials, inputObject(envelope["auth"]))
	}
	copyInputIdentity(credentials, credentials)
	enrichInputJWT(credentials)
	item := InputItem{Index: p.next, Kind: kind, Credentials: credentials}
	item.Email = inputText(credentials["email"])
	item.PlanType = inputText(credentials["plan_type"])
	item.Name = inputText(envelope["name"])
	if item.Name == "" {
		item.Name = item.Email
	}
	if len(item.Name) > 1024 || len(item.Email) > 320 || len(item.PlanType) > 128 || strings.ContainsAny(item.Name+item.Email+item.PlanType, "\r\n\x00") {
		p.fail("账号名称、邮箱或套餐信息长度或格式无效")
		return
	}
	for _, key := range tokenFields {
		secret := inputText(credentials[key])
		if secret != "" && (strings.Contains(item.Name, secret) || strings.Contains(item.Email, secret) || strings.Contains(item.PlanType, secret)) {
			p.fail("账号名称、邮箱和套餐信息不能包含授权凭据")
			return
		}
	}
	p.items = append(p.items, item)
	p.next++
}

func inputCredentials(source map[string]any) (map[string]any, error) {
	credentials := map[string]any{}
	for _, key := range tokenFields {
		raw, exists := source[key]
		if !exists || raw == nil {
			continue
		}
		value, ok := raw.(string)
		if !ok {
			return nil, errors.New("OAuth 令牌必须是字符串")
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if len(value) > 65536 || strings.Contains(value, "***") || strings.ContainsFunc(value, unicode.IsSpace) || strings.ContainsRune(value, '\x00') {
			return nil, errors.New("OAuth 令牌不完整或包含无效字符，请使用原始授权内容")
		}
		credentials[key] = value
	}
	if inputText(credentials["access_token"]) == "" && inputText(credentials["refresh_token"]) == "" {
		return nil, errors.New("账号缺少 access_token 或有效的 rt_ 刷新令牌")
	}
	copyInputIdentity(credentials, source)
	for _, key := range tokenMetadataFields {
		value, exists := source[key]
		if !exists || value == nil {
			continue
		}
		switch value.(type) {
		case string, json.Number:
			credentials[key] = value
		default:
			return nil, errors.New("OAuth 令牌附加字段类型无效")
		}
	}
	return credentials, nil
}

func findTokenContainer(value map[string]any) map[string]any {
	auth := inputObject(value["auth"])
	candidates := []map[string]any{inputObject(value["tokens"]), inputObject(auth["tokens"]), auth, value}
	for _, candidate := range candidates {
		for _, key := range tokenFields {
			if _, exists := candidate[key]; exists {
				return candidate
			}
		}
	}
	return nil
}

func copyInputIdentity(target, source map[string]any) {
	for _, key := range identityFields {
		if inputText(target[key]) == "" {
			if text := inputText(source[key]); text != "" {
				target[key] = text
			}
		}
	}
	if inputText(target["chatgpt_account_id"]) == "" && inputText(target["account_id"]) != "" {
		target["chatgpt_account_id"] = target["account_id"]
	}
	if inputText(target["chatgpt_user_id"]) == "" && inputText(target["user_id"]) != "" {
		target["chatgpt_user_id"] = target["user_id"]
	}
}

// JWT payloads supply display and deduplication metadata only. Signature or
// authorization verification must happen at the controlled network boundary.
func enrichInputJWT(credentials map[string]any) {
	for _, key := range []string{"access_token", "id_token"} {
		parts := strings.Split(inputText(credentials[key]), ".")
		if len(parts) != 3 {
			continue
		}
		raw, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			continue
		}
		decoded, err := decodeInputJSON(string(raw))
		if err != nil {
			continue
		}
		claims := inputObject(decoded)
		auth := inputObject(claims["https://api.openai.com/auth"])
		profile := inputObject(claims["https://api.openai.com/profile"])
		values := map[string]any{"chatgpt_account_id": auth["chatgpt_account_id"], "chatgpt_user_id": auth["chatgpt_user_id"], "plan_type": auth["chatgpt_plan_type"], "email": claims["email"]}
		if inputText(values["chatgpt_user_id"]) == "" {
			values["chatgpt_user_id"] = auth["user_id"]
		}
		if inputText(values["email"]) == "" {
			values["email"] = profile["email"]
		}
		copyInputIdentity(credentials, values)
		if key == "access_token" && credentials["expires_at"] == nil {
			if seconds, ok := claims["exp"].(json.Number); ok {
				if epoch, err := seconds.Int64(); err == nil && epoch > 0 && epoch < 253402300800 {
					credentials["expires_at"] = time.Unix(epoch, 0).UTC().Format(time.RFC3339)
				}
			}
		}
	}
}

func decodeInputJSON(raw string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return nil, errors.New("JSON 包含多余内容")
	}
	return value, nil
}

func inputText(value any) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}
func inputObject(value any) map[string]any { object, _ := value.(map[string]any); return object }

func cloneInputMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = cloneInputValue(item)
	}
	return result
}

func cloneInputValue(value any) any {
	switch node := value.(type) {
	case map[string]any:
		return cloneInputMap(node)
	case []any:
		result := make([]any, len(node))
		for i, child := range node {
			result[i] = cloneInputValue(child)
		}
		return result
	default:
		return value
	}
}

type oauthIdentity struct{ workspace, user, fingerprint string }

func inputIdentity(credentials map[string]any) oauthIdentity {
	value := cloneInputMap(credentials)
	copyInputIdentity(value, value)
	enrichInputJWT(value)
	identity := oauthIdentity{workspace: inputText(value["chatgpt_account_id"]), user: inputText(value["chatgpt_user_id"])}
	token := inputText(value["refresh_token"])
	if token == "" {
		token = inputText(value["access_token"])
	}
	if token != "" && !strings.Contains(token, "***") {
		hash := sha256.Sum256([]byte(token))
		identity.fingerprint = hex.EncodeToString(hash[:])
	}
	return identity
}

func IdentityKey(item InputItem) string {
	return inputIdentity(item.Credentials).key()
}

func (identity oauthIdentity) key() string {
	if identity.workspace != "" && identity.user != "" {
		raw, _ := json.Marshal([]string{identity.workspace, identity.user})
		return "identity:" + string(raw)
	}
	if identity.fingerprint != "" {
		return "fingerprint:" + identity.fingerprint
	}
	return ""
}

func StableIdentityMatch(item InputItem, account map[string]any) bool {
	if inputText(account["platform"]) != "openai" || inputText(account["type"]) != "oauth" {
		return false
	}
	return identitiesMatch(inputIdentity(item.Credentials), accountIdentity(account))
}

func accountIdentity(account map[string]any) oauthIdentity {
	currentCredentials := cloneInputMap(inputObject(account["credentials"]))
	copyInputIdentity(currentCredentials, inputObject(account["extra"]))
	copyInputIdentity(currentCredentials, account)
	return inputIdentity(currentCredentials)
}

func identitiesMatch(incoming, current oauthIdentity) bool {
	if incoming.workspace != "" && current.workspace != "" && incoming.workspace != current.workspace {
		return false
	}
	if incoming.user != "" && current.user != "" && incoming.user != current.user {
		return false
	}
	if incoming.workspace != "" && incoming.user != "" {
		return incoming.workspace == current.workspace && incoming.user == current.user
	}
	return incoming.fingerprint != "" && incoming.fingerprint == current.fingerprint
}
