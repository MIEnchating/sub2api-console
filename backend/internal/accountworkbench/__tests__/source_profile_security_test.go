package accountworkbench_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestSourceProfileConfirmedSafetyResultUpdatesOnlyMatchingPrivateField(t *testing.T) {
	for _, operation := range []string{"password", "totp"} {
		t.Run(operation, func(t *testing.T) {
			f, source := sourceSecurityFixture(t, accountworkbench.ScopeLocalExport, sourceSecurityClaims)
			profile := saveLocalSourceProfile(t, f.service, "security-owner", accountworkbench.SourceProfileReference{SourceOAuthID: source.ID})
			f.browser.passwordResult = true
			input := accountworkbench.SecurityStartInput{Scope: accountworkbench.ScopeLocalExport, SourceOAuthID: source.ID, Operation: operation, Confirmed: true}
			if operation == "password" {
				input.Password = securityPassword
			}
			var err error
			f.view, err = f.service.StartSecurity(context.Background(), "security-owner", input)
			if err != nil {
				t.Fatal(err)
			}
			f.awaitSecurity(t, "waiting")
			f.continueSecurity(t, "succeeded")
			apply := accountworkbench.SourceProfileSecurityInput{Scope: accountworkbench.ScopeLocalExport, Revision: profile.Revision, SecurityID: f.view.ID, Confirmed: true}
			updated, err := f.service.ApplySecurityToSourceProfile(context.Background(), "security-owner", profile.ID, apply)
			if err != nil || updated.Revision != profile.Revision+1 {
				t.Fatalf("source safety update = %+v, %v", updated, err)
			}
			hash := sha256.Sum256([]byte("security-owner"))
			private, err := f.private.WorkbenchSourceProfile(context.Background(), hex.EncodeToString(hash[:]), "local-export", profile.ID)
			if err != nil {
				t.Fatal(err)
			}
			var login accountworkbench.OAuthLoginInput
			if json.Unmarshal(private.Login, &login) != nil || login.TOTPSecret != securitySecret {
				t.Fatal("security result changed unrelated profile fields")
			}
			if operation == "password" && login.Password != securityPassword || operation == "totp" && login.Password != "private-source-password" {
				t.Fatal("private safety field update was incorrect")
			}
			public, _ := json.Marshal(updated)
			if strings.Contains(string(public), securityPassword) || strings.Contains(string(public), securitySecret) {
				t.Fatal("security profile update exposed secret")
			}
			if _, err := f.service.ApplySecurityToSourceProfile(context.Background(), "security-owner", profile.ID, apply); err == nil {
				t.Fatal("stale security profile update accepted")
			}
		})
	}
}
