package accountworkbench_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestExtractTemplateStripsIdentityAndSecretsAndKeepsExactMultiplier(t *testing.T) {
	account := map[string]any{
		"platform": "openai", "type": "oauth", "name": "source", "rate_multiplier": json.Number("0.123456789012345678901"),
		"group_ids": []any{json.Number("7"), "9"}, "concurrency": json.Number("4"),
		"credentials": map[string]any{"access_token": "private-token", "email": "owner@example.com", "chatgpt_account_id": "private-account", "model_mapping": map[string]any{"gpt-5": "gpt-5.6"}},
		"extra":       map[string]any{"password": "private-password", "openai_ws_enabled": true},
	}
	config, err := accountworkbench.ExtractTemplate(account)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "private") || strings.Contains(string(raw), "owner@example.com") {
		t.Fatal("source template included identity or credentials")
	}
	if string(config["rate_multiplier"]) != "0.123456789012345678901" {
		t.Fatal("rate multiplier lost precision")
	}
	if !strings.Contains(string(config["credential_extras"]), "model_mapping") {
		t.Fatal("allowed model mapping was lost")
	}
}

func TestExtractTemplateRejectsInvalidConfigAndOtherAccountTypes(t *testing.T) {
	for name, account := range map[string]map[string]any{
		"wrong platform":         {"platform": "anthropic", "type": "oauth"},
		"wrong type":             {"platform": "openai", "type": "apikey"},
		"fractional concurrency": {"platform": "openai", "type": "oauth", "concurrency": json.Number("1.5")},
		"invalid rate":           {"platform": "openai", "type": "oauth", "rate_multiplier": "1/3"},
		"negative group":         {"platform": "openai", "type": "oauth", "group_ids": []any{json.Number("-1")}},
		"float rate":             {"platform": "openai", "type": "oauth", "rate_multiplier": 0.1},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := accountworkbench.ExtractTemplate(account); err == nil {
				t.Fatal("invalid template accepted")
			}
		})
	}
}

func TestExtractTemplateNormalizesOnlyNullNotes(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value any
		want  string
	}{
		{name: "null", value: nil, want: `""`},
		{name: "empty", value: "", want: `""`},
		{name: "existing text", value: "Source notes", want: `"Source notes"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config, err := accountworkbench.ExtractTemplate(map[string]any{
				"platform": "openai", "type": "oauth", "notes": tt.value,
				"proxy_id": nil, "expires_at": nil, "load_factor": nil,
			})
			if err != nil {
				t.Fatal(err)
			}
			if string(config["notes"]) != tt.want {
				t.Fatalf("notes = %s, want %s", config["notes"], tt.want)
			}
			for _, key := range []string{"proxy_id", "expires_at", "load_factor"} {
				if string(config[key]) != "null" {
					t.Fatalf("nullable field %s changed: %s", key, config[key])
				}
			}
		})
	}
}

func TestExtractTemplateRejectsMalformedNotes(t *testing.T) {
	for name, value := range map[string]any{
		"number": json.Number("42"), "object": map[string]any{},
		"overlong": strings.Repeat("x", 16385), "null byte": "note\x00",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := accountworkbench.ExtractTemplate(map[string]any{
				"platform": "openai", "type": "oauth", "notes": value,
			})
			if err == nil || !strings.Contains(err.Error(), "notes") {
				t.Fatalf("expected notes validation error, got %v", err)
			}
		})
	}
}

func TestMatchTemplateUsesPlanAndEmailDomainThenStablePriority(t *testing.T) {
	item := accountworkbench.InputItem{PlanType: "plus", Email: "owner@example.com"}
	templates := []configstore.WorkbenchTemplate{
		{ID: "fallback", Priority: 99},
		{ID: "z", Priority: 10, Match: configstore.WorkbenchTemplateMatch{PlanType: "plus", EmailDomain: "example.com"}},
		{ID: "a", Priority: 10, Match: configstore.WorkbenchTemplateMatch{PlanType: "plus", EmailDomain: "EXAMPLE.COM"}},
		{ID: "other", Priority: 100, Match: configstore.WorkbenchTemplateMatch{PlanType: "team"}},
	}
	matched, err := accountworkbench.MatchTemplate(item, templates, "")
	if err != nil || matched == nil || matched.ID != "a" {
		t.Fatalf("match = %v, %v", matched, err)
	}
}

func TestMatchTemplateExplicitChoiceMustExistAndMatchIdentity(t *testing.T) {
	item := accountworkbench.InputItem{PlanType: "plus"}
	templates := []configstore.WorkbenchTemplate{{ID: "team", Match: configstore.WorkbenchTemplateMatch{PlanType: "team"}}}
	if _, err := accountworkbench.MatchTemplate(item, templates, "missing"); err == nil {
		t.Fatal("missing explicit template accepted")
	}
	if matched, err := accountworkbench.MatchTemplate(item, templates, "team"); err != nil || matched == nil || matched.ID != "team" {
		t.Fatalf("explicit operator selection rejected: %v", err)
	}
}

func TestApplyTemplateKeepsInputCredentialsAndWhitelistedConfig(t *testing.T) {
	item := accountworkbench.InputItem{Index: 0, Email: "owner@example.com", Credentials: map[string]any{"access_token": "private", "chatgpt_account_id": "workspace"}}
	template := configstore.WorkbenchTemplate{Config: configstore.WorkbenchTemplateConfig{"concurrency": json.RawMessage(`4`), "rate_multiplier": json.RawMessage(`"0.123456789012345678901"`), "group_ids": json.RawMessage(`["7",9]`), "credential_extras": json.RawMessage(`{"model_mapping":{"gpt-5":"gpt-5.6"}}`)}}
	payload, err := accountworkbench.ApplyTemplate(item, &template)
	if err != nil {
		t.Fatal(err)
	}
	credentials := payload["credentials"].(map[string]any)
	if credentials["access_token"] != "private" || credentials["chatgpt_account_id"] != "workspace" {
		t.Fatal("input identity changed")
	}
	if payload["rate_multiplier"] != json.Number("0.123456789012345678901") {
		t.Fatalf("rate lost precision: %v", payload["rate_multiplier"])
	}
	credentials["access_token"] = "changed"
	if item.Credentials["access_token"] != "private" {
		t.Fatal("applying template mutated input")
	}
}

func TestApplyTemplateRejectsInjectedCredentialFields(t *testing.T) {
	item := accountworkbench.InputItem{Credentials: map[string]any{"access_token": "private"}}
	for _, config := range []configstore.WorkbenchTemplateConfig{
		{"credentials": json.RawMessage(`{"access_token":"other"}`)},
		{"credential_extras": json.RawMessage(`{"access_token":"other"}`)},
		{"extra": json.RawMessage(`{"email":"other@example.com"}`)},
	} {
		if _, err := accountworkbench.ApplyTemplate(item, &configstore.WorkbenchTemplate{Config: config}); err == nil {
			t.Fatal("unsafe template was accepted")
		}
	}
}

func TestStableIdentityMatchRequiresWorkspaceAndUserOrExactToken(t *testing.T) {
	item := accountworkbench.InputItem{Email: "same@example.com", Credentials: map[string]any{"chatgpt_account_id": "workspace", "chatgpt_user_id": "user", "access_token": "token-one"}}
	matching := map[string]any{"platform": "openai", "type": "oauth", "credentials": map[string]any{"account_id": "workspace", "user_id": "user", "access_token": "token-two"}}
	if !accountworkbench.StableIdentityMatch(item, matching) {
		t.Fatal("same stable identity did not match")
	}
	matching["credentials"] = map[string]any{"chatgpt_account_id": "other-workspace", "chatgpt_user_id": "user", "email": "same@example.com"}
	if accountworkbench.StableIdentityMatch(item, matching) {
		t.Fatal("email or partial identity matched another workspace")
	}
	unknown := accountworkbench.InputItem{Credentials: map[string]any{"refresh_token": "rt_exact"}}
	matching["credentials"] = map[string]any{"refresh_token": "rt_exact"}
	if !accountworkbench.StableIdentityMatch(unknown, matching) {
		t.Fatal("exact credential fingerprint did not match")
	}
	matching["platform"] = "anthropic"
	if accountworkbench.StableIdentityMatch(unknown, matching) {
		t.Fatal("different platform matched")
	}
}
