package accountworkbench_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

const batchTOTP = "JBSWY3DPEHPK3PXP"
const batchMicrosoftClient = "11111111-2222-3333-4444-555555555555"

func TestOAuthBatchTwoFieldBase32PasswordIsNeverReclassifiedAsTOTP(t *testing.T) {
	items, failures := accountworkbench.ParseOAuthLogins("owner@example.com----" + batchTOTP)
	if len(failures) != 0 || len(items) != 1 || items[0].Password != batchTOTP || items[0].TOTPSecret != "" {
		t.Fatalf("two-field password was reclassified: items=%d, failures=%+v", len(items), failures)
	}
}

func TestOAuthBatchThreeFieldBase32PasswordKeepsFinalFieldAsTOTP(t *testing.T) {
	items, failures := accountworkbench.ParseOAuthLogins("owner@example.com----AAAAAAAAAAAAAAAA----jbswy3dpehpk3pxp")
	if len(failures) != 0 || len(items) != 1 || items[0].Password != "AAAAAAAAAAAAAAAA" || items[0].TOTPSecret != batchTOTP {
		t.Fatalf("password/TOTP field positions changed: items=%d, failures=%+v", len(items), failures)
	}
}

func TestOAuthBatchExplicitTextFormsPreservePasswordAndMailbox(t *testing.T) {
	for _, fixture := range []struct {
		name, input, password, mailbox, secret string
	}{
		{name: "bare-email", input: "owner@example.com"},
		{name: "password-spaces", input: "owner@example.com----  password with spaces  ", password: "  password with spaces  "},
		{name: "mailbox-only", input: "owner@example.com----https://mail.example/inbox?key=private", mailbox: "https://mail.example/inbox?key=private"},
		{name: "password-mailbox", input: "owner@example.com----password----https://mail.example/inbox", password: "password", mailbox: "https://mail.example/inbox"},
		{name: "mailbox-totp", input: "owner@example.com----https://mail.example/inbox----" + batchTOTP, mailbox: "https://mail.example/inbox", secret: batchTOTP},
		{name: "all-fields", input: "owner@example.com----password----https://mail.example/inbox----" + batchTOTP, password: "password", mailbox: "https://mail.example/inbox", secret: batchTOTP},
		{name: "tab-password-with-pipe", input: "owner@example.com\tpa|ssword", password: "pa|ssword"},
		{name: "quoted-pipe-password", input: "owner@example.com|\"pa|ssword\"", password: "pa|ssword"},
		{name: "quoted-pipe-email", input: "\"owner@example.com\"|\"pa|ssword\"", password: "pa|ssword"},
		{name: "hyphens-in-tab-password", input: "owner@example.com\tpa----ssword", password: "pa----ssword"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			items, failures := accountworkbench.ParseOAuthLogins(fixture.input)
			if len(failures) != 0 || len(items) != 1 {
				t.Fatalf("valid explicit line rejected: %+v", failures)
			}
			item := items[0]
			if item.Email != "owner@example.com" || item.Password != fixture.password || item.TOTPSecret != fixture.secret {
				t.Fatal("parser changed explicit account fields")
			}
			if fixture.mailbox == "" {
				if item.Mailbox != nil {
					t.Fatal("mailbox invented from password")
				}
			} else if item.Mailbox == nil || item.Mailbox.Kind != "http" || item.Mailbox.URL != fixture.mailbox || item.Mailbox.Method != "GET" {
				t.Fatal("explicit mailbox configuration lost")
			}
		})
	}
}

func TestOAuthBatchMicrosoftFourFieldsProduceMatchingPrivateMailbox(t *testing.T) {
	items, failures := accountworkbench.ParseOAuthLogins("owner@example.com----private-password----" + batchMicrosoftClient + "----private-refresh-token-123456789")
	if len(failures) != 0 || len(items) != 1 || items[0].Mailbox == nil {
		t.Fatalf("Microsoft line rejected: %+v", failures)
	}
	item := items[0]
	if item.Password != "private-password" || item.Mailbox.Kind != "microsoft" || item.Mailbox.Email != item.Email || item.Mailbox.ClientID != batchMicrosoftClient || item.Mailbox.RefreshToken != "private-refresh-token-123456789" {
		t.Fatal("Microsoft login fields were lost or rebound")
	}
}

func TestOAuthBatchJSONPreservesExplicitSensitiveFieldBoundaries(t *testing.T) {
	content := `[{"email":"owner@example.com","password":"p----a|s\tsword","totp_secret":"jbsw y3dp ehpk 3pxp","workspace_id":"workspace-1","mailbox":{"kind":"http","url":"https://mail.example/inbox","method":"POST","headers":{"X-Private":"secret-header"},"body":"{\"token\":\"private\"}"}}]`
	items, failures := accountworkbench.ParseOAuthLogins(content)
	if len(failures) != 0 || len(items) != 1 {
		t.Fatalf("valid JSON rejected: %+v", failures)
	}
	if items[0].Password != "p----a|s\tsword" || items[0].TOTPSecret != batchTOTP || items[0].WorkspaceID != "workspace-1" || items[0].Mailbox.Headers["X-Private"] != "secret-header" || items[0].Mailbox.Body != `{"token":"private"}` {
		t.Fatal("JSON sensitive fields were reinterpreted")
	}
}

func TestOAuthBatchStrictJSONRejectsUnknownDuplicateAndMistypedFields(t *testing.T) {
	for _, input := range []string{
		`{"email":"owner@example.com"}`,
		`[{"email":"owner@example.com","private-unknown-field":"secret"}]`,
		`[{"email":"owner@example.com","mailbox":{"kind":"http","url":"https://mail.example/","secret-unknown":"value"}}]`,
		`[{"email":"owner@example.com","password":"first","password":"second"}]`,
		`[{"email":"owner@example.com","EMAIL":"other@example.com"}]`,
		`[{"email":"owner@example.com","password":123}]`,
		`[{"email":"owner@example.com","password":null}]`,
		`[{"email":"owner@example.com"}] {}`,
		`[null]`,
		`[]`,
	} {
		items, failures := accountworkbench.ParseOAuthLogins(input)
		if len(items) != 0 || len(failures) == 0 {
			t.Fatalf("invalid JSON shape accepted: %s", input)
		}
		for _, failure := range failures {
			if strings.Contains(failure.Message, "secret") || strings.Contains(failure.Message, "private-unknown") || strings.Contains(failure.Message, "other@example.com") {
				t.Fatal("JSON error echoed private source text")
			}
		}
	}
}

func TestOAuthBatchInvalidLineReportsOriginalLineAndReturnsNoPartialCredentials(t *testing.T) {
	items, failures := accountworkbench.ParseOAuthLogins("first@example.com----private-first\n\nsecond@example.com----private-second----not-a-totp")
	if len(items) != 0 || len(failures) != 1 || failures[0].Index != 2 {
		t.Fatalf("failed batch leaked executable partial results or wrong line: items=%d, failures=%+v", len(items), failures)
	}
	if strings.Contains(failures[0].Message, "private-second") || strings.Contains(failures[0].Message, "not-a-totp") {
		t.Fatal("line error echoed private credential")
	}
}

func TestOAuthBatchDuplicateEmailAndWorkspaceRejectsReplacement(t *testing.T) {
	items, failures := accountworkbench.ParseOAuthLogins(`[{"email":"Owner@example.com","workspace_id":"workspace-1","password":"first-private"},{"email":"owner@example.com","workspace_id":"workspace-1","password":"second-private"}]`)
	if len(items) != 0 || len(failures) != 1 || failures[0].Index != 1 {
		t.Fatalf("duplicate login replaced an earlier item: items=%d, failures=%+v", len(items), failures)
	}
	if strings.Contains(failures[0].Message, "private") {
		t.Fatal("duplicate error revealed credentials")
	}
}

func TestOAuthBatchSameEmailDifferentWorkspaceRemainsSeparate(t *testing.T) {
	items, failures := accountworkbench.ParseOAuthLogins(`[{"email":"owner@example.com","workspace_id":"workspace-1"},{"email":"owner@example.com","workspace_id":"workspace-2"}]`)
	if len(failures) != 0 || len(items) != 2 {
		t.Fatalf("distinct stable workspaces were collapsed: %+v", failures)
	}
}

func TestOAuthBatchRejectsUnsafeMailboxAndMismatchedMicrosoftIdentity(t *testing.T) {
	for _, input := range []string{
		"owner@example.com----http://mail.example/inbox",
		"owner@example.com----https://127.0.0.1/inbox",
		`[{"email":"owner@example.com","mailbox":{"kind":"microsoft","email":"other@example.com","client_id":"` + batchMicrosoftClient + `","refresh_token":"private-refresh-token-123456789"}}]`,
		`[{"email":"owner@example.com","mailbox":{"kind":"http","url":"https://mail.example/","headers":{"X-Test":"private\r\nInjected: header"}}}]`,
	} {
		items, failures := accountworkbench.ParseOAuthLogins(input)
		if len(items) != 0 || len(failures) == 0 {
			t.Fatal("invalid private mailbox accepted")
		}
		if strings.Contains(failures[0].Message, "private") {
			t.Fatal("mailbox validation echoed credentials")
		}
	}
}

func TestOAuthBatchEnforcesInputSizeAndFiveHundredItemLimit(t *testing.T) {
	lines := make([]string, 501)
	for index := range lines {
		lines[index] = fmt.Sprintf("owner-%d@example.com", index)
	}
	if items, failures := accountworkbench.ParseOAuthLogins(strings.Join(lines[:500], "\n")); len(items) != 500 || len(failures) != 0 {
		t.Fatal("valid 500-item batch rejected")
	}
	if items, failures := accountworkbench.ParseOAuthLogins(strings.Join(lines, "\n")); len(items) != 0 || len(failures) == 0 {
		t.Fatal("501-item batch accepted")
	}
	if items, failures := accountworkbench.ParseOAuthLogins(strings.Repeat("x", (2<<20)+1)); len(items) != 0 || len(failures) == 0 {
		t.Fatal("oversized credential input accepted")
	}
}
