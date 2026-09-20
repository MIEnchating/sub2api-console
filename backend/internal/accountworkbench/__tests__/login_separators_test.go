package accountworkbench_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestLoginSeparatorsPreservePasswordAndTOTPThroughInputAndExecutionParsing(t *testing.T) {
	for _, separator := range []string{"--", "---", "----"} {
		for _, email := range []string{"owner+tag@example.test", `owner+tag\@example.test`} {
			t.Run(separator+"/"+email, func(t *testing.T) {
				const password = `private\@password`
				const secret = "JBSWY3DPEHPK3PXP"
				input := strings.Join([]string{email, password, secret}, separator)
				parsed := accountworkbench.ParseInput(input, true)
				if len(parsed.Errors) != 0 || len(parsed.Items) != 1 {
					t.Fatalf("login format rejected: %v", parsed.Errors)
				}
				item := parsed.Items[0]
				if item.Kind != "login" || item.Email != "owner+tag@example.test" {
					t.Fatal("preview did not recognize the normalized login email")
				}
				details, err := accountworkbench.ParseLoginDetails(item.LoginSource)
				if err != nil || details.Email != item.Email || details.Password != password || details.TOTP != secret {
					t.Fatal("execution parsing changed login credentials or disagreed with preview")
				}
				public, err := json.Marshal(parsed)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(public), "private") || strings.Contains(string(public), secret) {
					t.Fatal("public preview exposed login credentials")
				}
			})
		}
	}
}

func TestEscapedLoginEmailDoesNotRelaxOtherEmailValidation(t *testing.T) {
	for _, email := range []string{`owner\\@example.test`, `owner\@example`, `owner\@example.test\junk`} {
		t.Run(email, func(t *testing.T) {
			parsed := accountworkbench.ParseInput(email+"---private-password", true)
			if len(parsed.Items) != 0 || len(parsed.Errors) == 0 {
				t.Fatal("invalid escaped email was accepted")
			}
		})
	}
}
