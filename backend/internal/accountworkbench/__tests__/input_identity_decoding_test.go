package accountworkbench_test

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestInputJWTClaimsRemainUnverifiedHintsAndNeverReplaceExplicitIdentity(t *testing.T) {
	claims := `{"email":"hint@example.test","exp":9007199254740993,"https://api.openai.com/auth":{"chatgpt_account_id":"hint-workspace","chatgpt_user_id":"hint-user","chatgpt_plan_type":"plus"}}`
	token := "header." + base64.RawURLEncoding.EncodeToString([]byte(claims)) + ".signature"
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{false: "JWT hints", true: "explicit identity"}[explicit], func(t *testing.T) {
			input := map[string]any{"access_token": token}
			if explicit {
				input["email"] = "explicit@example.test"
				input["chatgpt_account_id"] = "explicit-workspace"
				input["chatgpt_user_id"] = "explicit-user"
				input["plan_type"] = "pro"
			}
			raw, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			parsed := accountworkbench.ParseInput(string(raw), false)
			if len(parsed.Errors) != 0 || len(parsed.Items) != 1 {
				t.Fatal("valid input rejected", parsed.Errors)
			}
			item := parsed.Items[0]
			prefix, source, plan := "hint", "jwt_claims_unverified", "plus"
			if explicit {
				prefix, source, plan = "explicit", "explicit", "pro"
			}
			if item.Email != prefix+"@example.test" || item.Credentials["chatgpt_account_id"] != prefix+"-workspace" || item.Credentials["chatgpt_user_id"] != prefix+"-user" || item.Plan != plan || item.IdentitySource != source {
				t.Fatal("identity hints replaced explicit identity or became verified")
			}
			public, err := json.Marshal(parsed)
			if err != nil || strings.Contains(string(public), token) || item.Credentials["exp"] != nil {
				t.Fatal("token or unrelated claim exposed", err)
			}
		})
	}
}

func TestInputMalformedJWTClaimsKeepCredentialsWithoutInventingIdentity(t *testing.T) {
	for _, claims := range []string{`{"email":`, `[]`, `null`} {
		t.Run(claims, func(t *testing.T) {
			token := "header." + base64.RawURLEncoding.EncodeToString([]byte(claims)) + ".signature"
			raw, err := json.Marshal(map[string]any{"access_token": token})
			if err != nil {
				t.Fatal(err)
			}
			parsed := accountworkbench.ParseInput(string(raw), true)
			if len(parsed.Errors) != 0 || len(parsed.Items) != 1 {
				t.Fatal("credential was rejected because hints were unreadable", parsed.Errors)
			}
			item := parsed.Items[0]
			if item.Email != "" || item.Credentials["chatgpt_user_id"] != nil || item.IdentitySource != "explicit" || item.Credentials["access_token"] != token {
				t.Fatal("unreadable hints changed identity or credentials")
			}
		})
	}
}
