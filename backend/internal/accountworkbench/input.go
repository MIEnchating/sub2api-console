package accountworkbench

import (
	"encoding/json"
	"io"
	"net/mail"
	"regexp"
	"strings"
)

const maximumBatchItems = 500
const maximumInputBytes = 4 << 20

var refreshTokenPattern = regexp.MustCompile(`^rt_[A-Za-z0-9._~-]+$`)
var loginSeparator = regexp.MustCompile(`-{2,}|\t|\|`)

// Credentials and login text are deliberately excluded from public previews.
type InputItem struct {
	ID             string         `json:"id"`
	Index          int            `json:"index"`
	Kind           string         `json:"kind"`
	Name           string         `json:"name"`
	Email          string         `json:"email"`
	Plan           string         `json:"plan"`
	IdentitySource string         `json:"identity_source"`
	Credentials    map[string]any `json:"-"`
	LoginSource    string         `json:"-"`
}
type InputError struct {
	Index   int    `json:"index"`
	Message string `json:"message"`
}
type ParsedInput struct {
	Items          []InputItem  `json:"items"`
	Errors         []InputError `json:"errors"`
	DuplicateCount int          `json:"duplicate_count"`
}
type inputParser struct {
	result ParsedInput
	index  int
}

func ParseInput(content string, export bool) ParsedInput {
	p := inputParser{result: ParsedInput{Items: []InputItem{}, Errors: []InputError{}}}
	raw := strings.TrimSpace(content)
	if len(content) > maximumInputBytes {
		p.fail("输入超过 4 MB，请分批处理")
		return p.result
	}
	if raw == "" {
		p.fail("请输入账号资料或上传文件")
		return p.result
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err == nil {
		if decoder.Decode(new(any)) != io.EOF {
			p.fail("JSON 包含多余内容，请检查文件格式")
		} else {
			p.jsonValue(value, 0)
		}
	} else if strings.HasPrefix(raw, "{") || strings.HasPrefix(raw, "[") || strings.HasPrefix(raw, `"`) {
		p.fail("JSON 格式无效，请检查后重新导入")
	} else {
		for _, line := range strings.Split(raw, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if p.index > maximumBatchItems {
				break
			}
			p.line(line)
		}
	}
	if p.index > maximumBatchItems {
		p.result = ParsedInput{Items: []InputItem{}, Errors: []InputError{{Message: "单批最多 500 个账号，请分批处理"}}}
		return p.result
	}
	if len(p.result.Items) == 0 && len(p.result.Errors) == 0 {
		p.fail("输入中没有可用账号")
	}
	if !export {
		p.deduplicate()
	}
	return p.result
}
func (p *inputParser) fail(message string) {
	p.result.Errors = append(p.result.Errors, InputError{Index: p.index, Message: message})
	p.index++
}
func (p *inputParser) add(item InputItem) {
	item.ID = newID()
	item.Index = p.index
	p.index++
	if item.Name == "" {
		item.Name = item.Email
	}
	item.Name = strings.TrimPrefix(item.Name, "oauth---")
	for _, key := range []string{"access_token", "refresh_token", "id_token"} {
		if secret := text(item.Credentials[key]); secret != "" {
			if strings.Contains(item.Name, secret) {
				item.Name = ""
			}
			if strings.Contains(item.Email, secret) {
				item.Email = ""
			}
		}
	}
	p.result.Items = append(p.result.Items, item)
}
func (p *inputParser) line(line string) {
	for _, part := range loginSeparator.Split(line, -1) {
		address := loginEmail(strings.TrimSpace(part))
		if address != "" {
			if _, err := ParseLoginDetails(line); err != nil {
				p.fail(err.Error())
				return
			}
			p.add(InputItem{Kind: "login", Email: address, LoginSource: line, Credentials: map[string]any{}, IdentitySource: "login"})
			return
		}
	}
	if refreshTokenPattern.MatchString(line) && len(line) <= 32768 {
		p.add(InputItem{Kind: "refresh_token", Credentials: map[string]any{"refresh_token": line}, IdentitySource: "pending_refresh"})
		return
	}
	p.fail("无法识别该行，请提供邮箱登录资料或以 rt_ 开头的刷新令牌")
}
func validEmail(value string) bool {
	if len(value) > 320 || strings.ContainsAny(value, "\r\n\x00") {
		return false
	}
	address, err := mail.ParseAddress(value)
	return err == nil && address.Address == value && strings.Contains(strings.SplitN(value, "@", 2)[1], ".")
}
func (p *inputParser) jsonValue(value any, depth int) {
	if p.index > maximumBatchItems {
		return
	}
	if depth > 16 {
		p.fail("JSON 嵌套层级过多")
		return
	}
	switch v := value.(type) {
	case []any:
		for _, entry := range v {
			p.jsonValue(entry, depth+1)
			if p.index > maximumBatchItems {
				break
			}
		}
	case string:
		token := strings.TrimSpace(v)
		if refreshTokenPattern.MatchString(token) && len(token) <= 32768 {
			p.add(InputItem{Kind: "refresh_token", Credentials: map[string]any{"refresh_token": token}, IdentitySource: "pending_refresh"})
		} else {
			p.fail("JSON 字符串必须是以 rt_ 开头的刷新令牌")
		}
	case map[string]any:
		if accounts, exists := v["accounts"]; exists {
			entries, ok := accounts.([]any)
			if !ok || len(entries) == 0 {
				p.fail("JSON accounts 必须包含账号")
				return
			}
			p.jsonValue(entries, depth+1)
			return
		}
		if data := object(v["data"]); data != nil && data["accounts"] != nil {
			p.jsonValue(data, depth+1)
			return
		}
		p.jsonAccount(v)
	default:
		p.fail("JSON 必须包含账号对象或刷新令牌")
	}
}
func (p *inputParser) jsonAccount(value map[string]any) {
	kind := "codex_json"
	source := value
	if value["credentials"] != nil || value["platform"] != nil || value["type"] != nil {
		platform, accountType := strings.ToLower(text(value["platform"])), strings.ToLower(text(value["type"]))
		if (platform != "" && platform != "openai") || (accountType != "" && accountType != "oauth") {
			p.fail("仅支持 OpenAI OAuth 账号")
			return
		}
		kind = "sub2api_json"
		source = object(value["credentials"])
	} else if auth := object(value["auth"]); text(auth["access_token"]) != "" {
		source = auth
	} else if tokens := object(value["tokens"]); text(tokens["access_token"]) != "" {
		source = tokens
	}
	credentials, ok := pickInputCredentials(source)
	if !ok {
		p.fail("账号凭据格式无效")
		return
	}
	if text(credentials["refresh_token"]) == "" && text(value["rt"]) != "" {
		credentials["refresh_token"] = text(value["rt"])
	}
	if text(credentials["access_token"]) == "" && text(credentials["refresh_token"]) == "" && text(credentials["id_token"]) == "" {
		p.fail("账号缺少 OAuth 凭据")
		return
	}
	for _, extra := range []map[string]any{object(value["extra"]), value, object(value["auth"]), object(value["tokens"])} {
		copyInputIdentity(credentials, extra)
	}
	if text(credentials["chatgpt_account_id"]) == "" {
		credentials["chatgpt_account_id"] = text(credentials["account_id"])
	}
	identitySource := enrichInputIdentity(credentials)
	if kind != "sub2api_json" && text(credentials["access_token"]) == "" && text(credentials["refresh_token"]) != "" {
		kind = "refresh_token"
		identitySource = "pending_refresh"
	}
	email := text(credentials["email"])
	if email != "" && !validEmail(email) {
		p.fail("账号邮箱格式无效")
		return
	}
	p.add(InputItem{Kind: kind, Name: text(value["name"]), Email: email, Plan: text(credentials["plan_type"]), IdentitySource: identitySource, Credentials: credentials})
}
