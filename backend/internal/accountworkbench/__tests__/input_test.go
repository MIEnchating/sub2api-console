package accountworkbench_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestMixedInputRecognizesLoginAndRTWithoutChoosingFormat(t *testing.T) {
	parsed := accountworkbench.ParseInput("owner@example.test----private-password----JBSWY3DPEHPK3PXP\nrt_private-refresh\nnot-valid", false)
	if len(parsed.Items) != 2 || len(parsed.Errors) != 1 {
		t.Fatalf("items=%d errors=%d", len(parsed.Items), len(parsed.Errors))
	}
	if parsed.Items[0].Kind != "login" || parsed.Items[1].Kind != "refresh_token" || parsed.Errors[0].Index != 2 {
		t.Fatal("input order or types lost")
	}
	raw, _ := json.Marshal(parsed)
	for _, secret := range []string{"private-password", "JBSWY3DPEHPK3PXP", "private-refresh"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("parser response leaks credentials")
		}
	}
}

func TestJSONInputAcceptsSub2APIAndCodexContainers(t *testing.T) {
	for _, content := range []string{
		`{"data":{"accounts":[{"platform":"openai","type":"oauth","credentials":{"access_token":"secret-at","refresh_token":"secret-rt","email":"a@example.test"}}]}}`,
		`{"tokens":{"access_token":"secret-at","refresh_token":"secret-rt","account_id":"workspace"},"email":"a@example.test"}`,
		`{"auth":{"access_token":"secret-at","refresh_token":"secret-rt"},"email":"a@example.test"}`,
	} {
		parsed := accountworkbench.ParseInput(content, false)
		if len(parsed.Errors) > 0 || len(parsed.Items) != 1 || parsed.Items[0].Email != "a@example.test" {
			t.Fatalf("valid JSON input rejected: %+v", parsed.Errors)
		}
	}
}

func TestManagedInputKeepsLastDuplicateAndExportRetainsAll(t *testing.T) {
	content := `[{"credentials":{"access_token":"first","chatgpt_account_id":"workspace","chatgpt_user_id":"user"},"name":"earlier"},{"credentials":{"access_token":"different","chatgpt_account_id":"workspace","chatgpt_user_id":"user"},"name":"latest"},"rt_repeat","rt_repeat"]`
	imported := accountworkbench.ParseInput(content, false)
	if len(imported.Items) != 2 || imported.DuplicateCount != 2 || imported.Items[0].Name != "latest" || imported.Items[0].Index != 1 || imported.Items[1].Index != 3 {
		t.Fatal("import did not retain latest occurrences")
	}
	exported := accountworkbench.ParseInput(content, true)
	if len(exported.Items) != 4 || exported.DuplicateCount != 0 {
		t.Fatal("export unexpectedly removed duplicates")
	}
}

func TestManagedInputDeduplicatesLoginEmailCaseInsensitively(t *testing.T) {
	parsed := accountworkbench.ParseInput("OWNER@example.test----first\nowner@example.test----last", false)
	if len(parsed.Items) != 1 || parsed.Items[0].Index != 1 || parsed.DuplicateCount != 1 {
		t.Fatal("login duplicates not collapsed")
	}
}

func TestInputRejectsMalformedJSONWithoutFallingBackToLogin(t *testing.T) {
	parsed := accountworkbench.ParseInput(`{"email":"owner@example.test",`, false)
	if len(parsed.Items) != 0 || len(parsed.Errors) != 1 {
		t.Fatal("malformed JSON treated as login")
	}
}

func TestInputRejectsOtherPlatformsAndEmptyContainers(t *testing.T) {
	for _, input := range []string{`{"accounts":[]}`, `{"platform":"anthropic","type":"oauth","credentials":{"access_token":"secret"}}`, `{"access_token":{}}`, `[]`, `null`} {
		parsed := accountworkbench.ParseInput(input, false)
		if len(parsed.Items) != 0 || len(parsed.Errors) == 0 {
			t.Fatalf("invalid input accepted: %s", input)
		}
	}
}

func TestInputRejectsOversizeBatchBeforeDedupe(t *testing.T) {
	raw := "[" + strings.Repeat(`"rt_repeat",`, 500) + `"rt_repeat"]`
	parsed := accountworkbench.ParseInput(raw, false)
	if len(parsed.Items) != 0 || len(parsed.Errors) == 0 {
		t.Fatal("oversize batch accepted after dedupe")
	}
}

func TestJSONInputPreservesSupportedCredentialConfiguration(t *testing.T) {
	parsed := accountworkbench.ParseInput(`{"credentials":{"access_token":"private-access","model_mapping":{"gpt-5":"gpt-5.6"},"intercept_warmup_requests":true,"plan_type":"plus"}}`, true)
	if len(parsed.Errors) > 0 || len(parsed.Items) != 1 {
		t.Fatal("valid credential configuration rejected")
	}
	credentials := parsed.Items[0].Credentials
	if credentials["model_mapping"] == nil || credentials["intercept_warmup_requests"] != true {
		t.Fatal("input configuration discarded")
	}
}
