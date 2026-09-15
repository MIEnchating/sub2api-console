package accountworkbench

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// Mixed indexes are zero-based logical entries. Blank lines do not count;
// arrays and Sub2API account lists expand in order. JSON segments are decoded
// before advancing, so formatted objects are never split on line boundaries.
func parseWorkbenchRun(content string) ([]mixedEntry, []InputError) {
	if len(content) > maxInputBytes || !utf8.ValidString(content) {
		return nil, []InputError{{Message: "单批输入必须为有效 UTF-8 文本且不超过 2 MiB"}}
	}
	remaining := strings.TrimSpace(strings.TrimPrefix(content, "\ufeff"))
	entries := []mixedEntry{}
	failures := []InputError{}
	next := 0
	var parseJSON func(json.RawMessage, int)
	fail := func(message string) { failures = append(failures, InputError{Index: next, Message: message}); next++ }
	appendItem := func(items []InputItem, problems []InputError) {
		if len(problems) > 0 {
			for _, problem := range problems {
				fail(problem.Message)
			}
			return
		}
		for _, item := range items {
			if next >= maxInputItems {
				fail("单批最多处理 500 个账号")
				return
			}
			item.Index = next
			entries = append(entries, mixedEntry{index: next, item: &item})
			next++
		}
	}
	appendLogin := func(raw string) {
		values, problems := ParseOAuthLogins(raw)
		if len(problems) > 0 {
			for _, problem := range problems {
				fail(problem.Message)
			}
			return
		}
		for _, login := range values {
			entries = append(entries, mixedEntry{index: next, login: &login})
			next++
		}
	}
	parseJSON = func(raw json.RawMessage, depth int) {
		if next >= maxInputItems {
			fail("单批最多处理 500 个账号")
			return
		}
		if depth > 12 {
			fail("账号 JSON 嵌套过深")
			return
		}
		text := strings.TrimSpace(string(raw))
		if strings.HasPrefix(text, "[") {
			var values []json.RawMessage
			if json.Unmarshal(raw, &values) != nil || len(values) == 0 {
				fail("账号 JSON 数组不能为空")
				return
			}
			for _, value := range values {
				parseJSON(value, depth+1)
				if next > maxInputItems {
					break
				}
			}
			return
		}
		if strings.HasPrefix(text, "{") {
			var object map[string]json.RawMessage
			if json.Unmarshal(raw, &object) != nil || duplicateOAuthJSONKeys(raw) {
				fail("账号 JSON 含无效或重复字段")
				return
			}
			credential := false
			for _, key := range []string{"credentials", "accounts", "data", "auth", "tokens", "access_token", "refresh_token", "id_token", "rt", "auth_mode", "OPENAI_API_KEY"} {
				if object[key] != nil {
					credential = true
				}
			}
			login := object["password"] != nil || object["totp_secret"] != nil || object["mailbox"] != nil || object["sms"] != nil
			if credential && login {
				fail("同一账号对象不能同时混用授权凭据和邮箱登录字段")
				return
			}
			if !credential && object["email"] != nil {
				appendLogin("[" + text + "]")
				return
			}
		}
		value, err := decodeInputJSON(text)
		if err != nil {
			fail("JSON 格式不正确，请检查括号和字段")
			return
		}
		// Reuse the standard credential parser with the outer mixed offset so
		// valid siblings still occupy their original slots when a wrapper fails.
		parser := inputParser{items: []InputItem{}, failures: []InputError{}, next: next}
		parser.parseValue(value, false, depth)
		for _, item := range parser.items {
			entries = append(entries, mixedEntry{index: item.Index, item: &item})
		}
		failures = append(failures, parser.failures...)
		next = parser.next
	}
	for remaining != "" {
		if next >= maxInputItems {
			fail("单批最多处理 500 个账号")
			break
		}
		if remaining[0] == '"' {
			line, rest, _ := strings.Cut(remaining, "\n")
			if _, err := parseOAuthTextLine(strings.TrimSuffix(line, "\r")); err == nil {
				appendLogin(line)
				remaining = strings.TrimSpace(rest)
				continue
			}
		}
		if strings.ContainsRune("[{\"", rune(remaining[0])) {
			decoder := json.NewDecoder(strings.NewReader(remaining))
			var raw json.RawMessage
			if decoder.Decode(&raw) != nil {
				fail("JSON 格式不正确，请检查括号、引号和多余内容")
				break
			}
			parseJSON(raw, 0)
			remaining = strings.TrimSpace(remaining[decoder.InputOffset():])
			continue
		}
		line, rest, found := strings.Cut(remaining, "\n")
		if !found {
			rest = ""
		}
		line = strings.TrimSuffix(line, "\r")
		if refreshTokenPattern.MatchString(strings.TrimSpace(line)) {
			appendItem(Parse(line))
		} else {
			appendLogin(line)
		}
		remaining = strings.TrimSpace(rest)
	}
	if next == 0 {
		fail("请填写 JSON、rt_ 刷新令牌或邮箱授权账号")
	}
	if len(failures) > 0 {
		return nil, failures
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		key := ""
		if entry.login != nil {
			key = "login:" + strings.ToLower(entry.login.Email) + "\x00" + entry.login.WorkspaceID
		} else {
			key = IdentityKey(*entry.item)
		}
		if key != "" && seen[key] {
			failures = append(failures, InputError{Index: entry.index, Message: "本批存在重复账号，请删除重复项"})
		}
		seen[key] = true
	}
	if len(failures) > 0 {
		return nil, failures
	}
	return entries, failures
}
