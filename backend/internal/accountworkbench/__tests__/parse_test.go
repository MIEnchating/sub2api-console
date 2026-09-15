package accountworkbench_test

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestParseCodexAuthTokensPreservesIdentityWithoutSerializingCredentials(t *testing.T) {
	items, failures := accountworkbench.Parse(`{"auth":{"tokens":{"access_token":"access-private","refresh_token":"rt_private","account_id":"workspace-1","user_id":"user-1"}},"email":"owner@example.com"}`)
	if len(failures) != 0 || len(items) != 1 {
		t.Fatalf("parse results = %d items, %v", len(items), failures)
	}
	if items[0].Kind != "codex_json" || items[0].Email != "owner@example.com" || items[0].Credentials["chatgpt_account_id"] != "workspace-1" {
		t.Fatal("Codex identity or input kind was not preserved")
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "access-private") || strings.Contains(string(encoded), "rt_private") || strings.Contains(string(encoded), "credentials") {
		t.Fatal("public item serialization exposed credentials")
	}
}

func TestParseSub2APIWrapperSupportsMultipleAccounts(t *testing.T) {
	items, failures := accountworkbench.Parse(`{"type":"sub2api-data","data":{"accounts":[{"name":"A","platform":"openai","type":"oauth","credentials":{"access_token":"access-a","email":"a@example.com"}},{"name":"B","platform":"openai","type":"oauth","credentials":{"refresh_token":"rt_b"}}]}}`)
	if len(failures) != 0 || len(items) != 2 {
		t.Fatalf("parse = %d items, %v", len(items), failures)
	}
	if items[0].Name != "A" || items[1].Name != "B" || items[1].Index != 1 {
		t.Fatal("account order or names changed")
	}
}

func TestParseRefreshTokenLinesSkipsBlankLines(t *testing.T) {
	items, failures := accountworkbench.Parse("\nrt_first\r\n\nrt_second\n")
	if len(failures) != 0 || len(items) != 2 {
		t.Fatalf("parse = %d items, %v", len(items), failures)
	}
	if items[0].Kind != "refresh_token" || items[1].Credentials["refresh_token"] != "rt_second" {
		t.Fatal("refresh tokens were not recognized")
	}
}

func TestParseInvalidEntryRejectsEntireBatchWithoutEchoingSecret(t *testing.T) {
	items, failures := accountworkbench.Parse("rt_first\nowner@example.com----password-private")
	if len(items) != 0 || len(failures) != 1 {
		t.Fatalf("invalid mixed batch = %d items, %v", len(items), failures)
	}
	if !strings.Contains(failures[0].Message, "授权") || strings.Contains(failures[0].Message, "password-private") {
		t.Fatal("login error must explain authorization without echoing input")
	}
}

func TestParseInvalidInputsReturnNoExecutableItems(t *testing.T) {
	cases := map[string]string{
		"empty": " ", "empty collection": `{"accounts":[]}`, "invalid JSON": `{"tokens":`,
		"trailing JSON": `{"access_token":"access"} {}`, "other platform": `{"platform":"anthropic","type":"oauth","credentials":{"access_token":"access"}}`,
		"wrong account type": `{"platform":"openai","type":"apikey","credentials":{"access_token":"access"}}`,
		"numeric token":      `{"access_token":123}`, "masked token": `{"access_token":"sk-***"}`,
		"ID token only": `{"id_token":"id-private"}`, "invalid refresh token": `{"refresh_token":"contains whitespace"}`,
		"mixed platform":   `[{"tokens":{"access_token":"access"}},{"platform":"anthropic","credentials":{"access_token":"private"}}]`,
		"too many entries": `[` + strings.TrimSuffix(strings.Repeat(`"rt_test",`, 501), ",") + `]`,
		"oversize":         strings.Repeat("x", 2*1024*1024+1),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			items, failures := accountworkbench.Parse(input)
			if len(items) != 0 || len(failures) == 0 {
				t.Fatalf("invalid input accepted: %d items", len(items))
			}
		})
	}
}

func TestParseStructuredRefreshTokenAcceptsOpaqueOAuthValue(t *testing.T) {
	items, failures := accountworkbench.Parse(`{"tokens":{"refresh_token":"opaque-refresh-value"}}`)
	if len(failures) != 0 || len(items) != 1 || items[0].Credentials["refresh_token"] != "opaque-refresh-value" {
		t.Fatal("structured OAuth refresh token was rejected based on an undocumented prefix")
	}
}

func TestParseRejectsCredentialsEmbeddedInVisibleAccountMetadata(t *testing.T) {
	for _, field := range []string{"name", "email", "plan_type"} {
		t.Run(field, func(t *testing.T) {
			input := map[string]any{"access_token": "access-private", field: "prefix-access-private-suffix"}
			raw, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			items, failures := accountworkbench.Parse(string(raw))
			if len(items) != 0 || len(failures) != 1 || strings.Contains(failures[0].Message, "access-private") {
				t.Fatal("visible metadata included a private credential")
			}
		})
	}
}

func TestParseJWTClaimsSupplyMissingMetadataButPreserveExplicitIdentity(t *testing.T) {
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"email":"claim@example.com","https://api.openai.com/auth":{"chatgpt_account_id":"claim-workspace","chatgpt_user_id":"claim-user","chatgpt_plan_type":"plus"}}`))
	input := `{"tokens":{"access_token":"header.` + claims + `.signature","account_id":"explicit-workspace"}}`
	items, failures := accountworkbench.Parse(input)
	if len(failures) != 0 || len(items) != 1 {
		t.Fatalf("parse = %d items, %v", len(items), failures)
	}
	if items[0].Email != "claim@example.com" || items[0].PlanType != "plus" || items[0].Credentials["chatgpt_account_id"] != "explicit-workspace" {
		t.Fatal("JWT metadata overwrote explicit identity or was not read")
	}
}
