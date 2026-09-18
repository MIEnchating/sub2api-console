package accountworkbench

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

var inputIdentityFields = []string{"email", "account_id", "chatgpt_account_id", "user_id", "chatgpt_user_id", "workspace_id", "organization_id", "org_id", "plan_type", "subscription_expires_at"}

func pickInputCredentials(source map[string]any) (map[string]any, bool) {
	result := map[string]any{}
	for _, key := range append([]string{"access_token", "refresh_token", "id_token", "client_id", "auth_mode", "token_type", "expires_at", "expires_in"}, inputIdentityFields...) {
		value, exists := source[key]
		if !exists || value == nil {
			continue
		}
		switch v := value.(type) {
		case string:
			if len(v) > 32768 || strings.ContainsAny(v, "\r\n\x00") {
				return nil, false
			}
			if v != "" {
				result[key] = strings.TrimSpace(v)
			}
		case json.Number:
			if key != "expires_at" && key != "expires_in" {
				return nil, false
			}
			result[key] = v
		default:
			return nil, false
		}
	}
	config := TemplateConfig{RateMultiplier: "1", CredentialExtras: map[string]json.RawMessage{}}
	for _, key := range credentialConfigFields {
		if value, exists := source[key]; exists && value != nil {
			raw, err := json.Marshal(value)
			if err != nil {
				return nil, false
			}
			config.CredentialExtras[key] = raw
			result[key] = value
		}
	}
	if config.Validate() != nil {
		return nil, false
	}
	return result, true
}
func copyInputIdentity(destination, source map[string]any) {
	for _, key := range inputIdentityFields {
		if text(destination[key]) == "" {
			if value, ok := source[key].(string); ok && len(value) <= 2048 && !strings.ContainsAny(value, "\r\n\x00") {
				destination[key] = strings.TrimSpace(value)
			}
		}
	}
}

// Decoded JWT claims are hints for preview and deduplication, never verified identity.
func enrichInputIdentity(credentials map[string]any) string {
	result := "explicit"
	for _, key := range []string{"access_token", "id_token"} {
		parts := strings.Split(text(credentials[key]), ".")
		if len(parts) != 3 {
			continue
		}
		payload, err := base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			continue
		}
		var claims map[string]any
		decoder := json.NewDecoder(strings.NewReader(string(payload)))
		decoder.UseNumber()
		if decoder.Decode(&claims) != nil {
			continue
		}
		auth, profile := object(claims["https://api.openai.com/auth"]), object(claims["https://api.openai.com/profile"])
		email := text(claims["email"])
		if email == "" {
			email = text(profile["email"])
		}
		user := text(auth["chatgpt_user_id"])
		if user == "" {
			user = text(auth["user_id"])
		}
		values := map[string]any{"chatgpt_account_id": text(auth["chatgpt_account_id"]), "chatgpt_user_id": user, "plan_type": text(auth["chatgpt_plan_type"]), "email": email}
		for field, value := range values {
			if text(credentials[field]) == "" && text(value) != "" {
				credentials[field] = value
				result = "jwt_claims_unverified"
			}
		}
	}
	return result
}
func inputIdentityKey(item InputItem) string {
	workspace := text(item.Credentials["chatgpt_account_id"])
	if workspace == "" {
		workspace = text(item.Credentials["account_id"])
	}
	user := text(item.Credentials["chatgpt_user_id"])
	if user == "" {
		user = text(item.Credentials["user_id"])
	}
	if workspace != "" && user != "" {
		return "identity:" + digest([]string{strings.ToLower(workspace), strings.ToLower(user)})
	}
	token := text(item.Credentials["refresh_token"])
	if token == "" {
		token = text(item.Credentials["access_token"])
	}
	if token != "" {
		return "token:" + digest(token)
	}
	if item.Kind == "login" {
		return "login:" + strings.ToLower(item.Email)
	}
	return ""
}
func (p *inputParser) deduplicate() {
	positions := map[string]int{}
	items := make([]InputItem, 0, len(p.result.Items))
	for _, item := range p.result.Items {
		key := inputIdentityKey(item)
		if index, exists := positions[key]; exists && key != "" {
			items[index] = item
			p.result.DuplicateCount++
			continue
		}
		if key != "" {
			positions[key] = len(items)
		}
		items = append(items, item)
	}
	p.result.Items = items
}
