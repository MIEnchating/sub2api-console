package accountworkbench_test

import (
	"encoding/json"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestLoginDetailsRecognizesWorkbenchFormatsWithoutPublicSecrets(t *testing.T) {
	for _, tc := range []struct{ name, input, password, totp, mail string }{
		{"email only", "owner@example.test", "", "", ""},
		{"password", "owner@example.test----test-password", "test-password", "", ""},
		{"password and totp", "owner@example.test----test-password----JBSWY3DPEHPK3PXP", "test-password", "JBSWY3DPEHPK3PXP", ""},
		{"mail and totp", "owner@example.test|https://mail.example.test/inbox|JBSWY3DPEHPK3PXP", "", "JBSWY3DPEHPK3PXP", "http"},
		{"base32 looking password", "owner@example.test----JBSWY3DPEHPK3PXP", "JBSWY3DPEHPK3PXP", "", ""},
		{"microsoft", "owner@example.test----test-password----11111111-2222-3333-4444-555555555555----mail-refresh-secret", "test-password", "", "microsoft"},
		{"email after password", "test-password|owner@example.test", "test-password", "", ""},
		{"empty password before totp", "owner@example.test--------JBSWY3DPEHPK3PXP", "", "JBSWY3DPEHPK3PXP", ""},
		{"password retains other delimiters", "owner@example.test----password--with|separators", "password--with|separators", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := accountworkbench.ParseLoginDetails(tc.input)
			if err != nil || got.Email != "owner@example.test" || got.Password != tc.password || got.TOTP != tc.totp || got.Mail.Kind != tc.mail {
				t.Fatalf("wrong login classification: %v", err)
			}
			raw, _ := json.Marshal(got)
			if string(raw) != "{}" {
				t.Fatal("login details can be serialized publicly")
			}
		})
	}
}

func TestLoginDetailsRejectsAmbiguousEmailsAndUntrustedMailEndpoint(t *testing.T) {
	for _, input := range []string{"owner@example.test|other@example.test", "owner@example.test|https://127.0.0.1/inbox", "owner@example.test|one|two"} {
		if _, err := accountworkbench.ParseLoginDetails(input); err == nil {
			t.Fatal("ambiguous or unsafe input accepted")
		}
	}
}
