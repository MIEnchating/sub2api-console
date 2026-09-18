package accountworkbench

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
)

// AccountPayload contains private credentials; only execution and private
// artifacts may serialize it. The public run uses InputItem summaries instead.
func AccountPayload(item InputItem, template *Template) (map[string]any, error) {
	credentials, ok := pickInputCredentials(item.Credentials)
	if !ok || (text(credentials["access_token"]) == "" && text(credentials["refresh_token"]) == "") {
		return nil, errors.New("账号缺少可用 OAuth 凭据")
	}
	config := TemplateConfig{Concurrency: 1, Priority: 50, RateMultiplier: "1", AutoPauseOnExpired: true, GroupIDs: []string{}, CredentialExtras: map[string]json.RawMessage{}, Extra: map[string]json.RawMessage{}}
	if template != nil {
		config = template.Config
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}
	body := map[string]any{}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err = decoder.Decode(&body); err != nil {
		return nil, err
	}
	delete(body, "credential_extras")
	maps.Copy(credentials, rawConfigMap(config.CredentialExtras))
	body["credentials"] = credentials
	body["platform"] = "openai"
	body["type"] = "oauth"
	body["rate_multiplier"] = json.Number(config.RateMultiplier)
	if config.LoadFactor != nil {
		body["load_factor"] = json.Number(*config.LoadFactor)
	}
	if config.ProxyID != nil {
		body["proxy_id"] = json.Number(*config.ProxyID)
	}
	groups := make([]json.Number, 0, len(config.GroupIDs))
	for _, id := range config.GroupIDs {
		groups = append(groups, json.Number(id))
	}
	body["group_ids"] = groups
	name := strings.TrimSpace(item.Name)
	if name == "" {
		name = item.Email
	}
	if name == "" {
		name = fmt.Sprintf("OpenAI OAuth %d", item.Index+1)
	}
	body["name"] = name
	return body, nil
}
func rawConfigMap(input map[string]json.RawMessage) map[string]any {
	result := make(map[string]any, len(input))
	for key, raw := range input {
		var value any
		decoder := json.NewDecoder(strings.NewReader(string(raw)))
		decoder.UseNumber()
		if decoder.Decode(&value) == nil {
			result[key] = value
		}
	}
	return result
}

// FindExistingAccount never associates by display name or email. Ambiguity
// stops a write instead of choosing an arbitrary stable ID.
func FindExistingAccount(item InputItem, accounts []map[string]any) (map[string]any, error) {
	workspace, user := credentialIdentity(item.Credentials)
	var matched map[string]any
	for _, account := range accounts {
		if text(account["platform"]) != "openai" || text(account["type"]) != "oauth" {
			continue
		}
		current := object(account["credentials"])
		currentWorkspace, currentUser := credentialIdentity(current)
		same := false
		if workspace != "" && user != "" {
			same = workspace == currentWorkspace && user == currentUser
		} else {
			for _, key := range []string{"refresh_token", "access_token"} {
				token := text(item.Credentials[key])
				if token != "" && token == text(current[key]) {
					same = true
				}
			}
		}
		if !same {
			continue
		}
		if text(account["id"]) == "" || matched != nil {
			return nil, errors.New("线上存在不明确的账号身份，请先整理重复账号")
		}
		matched = account
	}
	return matched, nil
}
func credentialIdentity(credentials map[string]any) (string, string) {
	workspace := text(credentials["chatgpt_account_id"])
	if workspace == "" {
		workspace = text(credentials["account_id"])
	}
	user := text(credentials["chatgpt_user_id"])
	if user == "" {
		user = text(credentials["user_id"])
	}
	return workspace, user
}
