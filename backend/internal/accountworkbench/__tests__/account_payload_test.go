package accountworkbench_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestAccountPayloadUsesTemplateWithoutChangingCredentialIdentity(t *testing.T) {
	var source map[string]any
	decoder := json.NewDecoder(strings.NewReader(sourceAccount))
	decoder.UseNumber()
	if err := decoder.Decode(&source); err != nil {
		t.Fatal(err)
	}
	config, err := accountworkbench.ExtractConfig(source)
	if err != nil {
		t.Fatal(err)
	}
	item := accountworkbench.ParseInput(`{"access_token":"incoming-access","refresh_token":"incoming-refresh","account_id":"incoming-workspace","user_id":"incoming-user"}`, false).Items[0]
	payload, err := accountworkbench.AccountPayload(item, &accountworkbench.Template{Config: config})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"rate_multiplier":0.1234567890123456789`) || !strings.Contains(string(raw), `"concurrency":0`) || !strings.Contains(string(raw), `"group_ids":[7]`) {
		t.Fatal("numeric config lost or converted to string")
	}
	credentials := payload["credentials"].(map[string]any)
	if credentials["chatgpt_account_id"] != "incoming-workspace" || credentials["user_id"] != "incoming-user" || credentials["access_token"] != "incoming-access" || credentials["plan_type"] != "prolite" {
		t.Fatal("template changed identity or lost override")
	}
	if item.Credentials["plan_type"] != nil {
		t.Fatal("payload building mutated original input")
	}
}
func TestExistingAccountMatchRejectsEmailOnlyAndAmbiguousStableIdentity(t *testing.T) {
	item := accountworkbench.InputItem{Email: "same@example.test", Credentials: map[string]any{"chatgpt_account_id": "workspace", "chatgpt_user_id": "user", "access_token": "incoming"}}
	emailOnly := map[string]any{"id": json.Number("1"), "platform": "openai", "type": "oauth", "credentials": map[string]any{"email": item.Email, "access_token": "other"}}
	existing, err := accountworkbench.FindExistingAccount(item, []map[string]any{emailOnly})
	if err != nil || existing != nil {
		t.Fatal("email-only association accepted")
	}
	stable := map[string]any{"id": json.Number("2"), "platform": "openai", "type": "oauth", "credentials": map[string]any{"chatgpt_account_id": "workspace", "chatgpt_user_id": "user", "access_token": "old"}}
	existing, err = accountworkbench.FindExistingAccount(item, []map[string]any{emailOnly, stable})
	if err != nil || existing["id"] != json.Number("2") {
		t.Fatal("stable identity failed to match")
	}
	if _, err = accountworkbench.FindExistingAccount(item, []map[string]any{stable, stable}); err == nil {
		t.Fatal("ambiguous identity accepted")
	}
}
