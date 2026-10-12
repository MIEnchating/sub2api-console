package accountworkbench_test

import (
	"encoding/json"
	"testing"
)

func TestImportedAccountKeepsPrivateGenerationCredentialAfterSessionCleanup(t *testing.T) {
	f := importedRememberedFixture(t, false)
	if err := f.store.RevokeSession(t.Context(), f.token); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Cleanup(t.Context()); err != nil {
		t.Fatal(err)
	}
	records, err := f.store.WorkbenchDocuments(t.Context(), "remembered-account:", 10)
	if err != nil || len(records) != 1 {
		t.Fatalf("saved account missing: %v", err)
	}
	var saved struct {
		GenerationCredentials map[string]any `json:"generation_credentials"`
	}
	if err := json.Unmarshal(records[0].Payload, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.GenerationCredentials["access_token"] != f.account["credentials"].(map[string]any)["access_token"] {
		t.Fatal("verified generation credential was discarded after import")
	}
	if _, ok := saved.GenerationCredentials["refresh_token"]; ok {
		t.Fatal("generation cache must not retain a rotating refresh token")
	}
}

func TestGenerationCredentialsRequireBoundIdentityAndSurviveSessionCleanup(t *testing.T) {
	f := importedRememberedFixture(t, false)
	target, err := f.store.TargetSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := f.account["credentials"].(map[string]any)["access_token"]
	delete(f.account["credentials"].(map[string]any), "access_token")
	if err := f.store.RevokeSession(t.Context(), f.token); err != nil {
		t.Fatal(err)
	}
	if err := f.service.Cleanup(t.Context()); err != nil {
		t.Fatal(err)
	}
	got, _, err := f.service.GenerationCredentials(t.Context(), target, f.account)
	if err != nil || got["access_token"] != want {
		t.Fatalf("bound local credential unavailable: %v", err)
	}
	f.account["credentials"].(map[string]any)["chatgpt_account_id"] = "other-workspace"
	if _, _, err := f.service.GenerationCredentials(t.Context(), target, f.account); err == nil {
		t.Fatal("changed identity reused saved credential")
	}
}

func TestDeletedRememberedAccountCannotSupplyGenerationCredentials(t *testing.T) {
	f := importedRememberedFixture(t, false)
	target, err := f.store.TargetSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	records, err := f.service.RememberedAccounts(t.Context(), f.owner)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.DeleteRememberedAccount(t.Context(), f.owner, records[0].ID, records[0].Revision); err != nil {
		t.Fatal(err)
	}
	got, _, err := f.service.GenerationCredentials(t.Context(), target, f.account)
	if err != nil || got != nil {
		t.Fatalf("deleted credential reused: %v", err)
	}
}

func TestGenerationCredentialsRejectOtherManagementTarget(t *testing.T) {
	f := importedRememberedFixture(t, false)
	target, err := f.store.TargetSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	target.AdminKey = "other-admin"
	if _, _, err := f.service.GenerationCredentials(t.Context(), target, f.account); err == nil {
		t.Fatal("other management target reused credential")
	}
}

func TestGenerationCredentialsValidateSavedExpiryAndLegacyMissingToken(t *testing.T) {
	for _, tc := range []struct {
		name, expiry       string
		missing, wantError bool
	}{
		{name: "RFC3339 future", expiry: "2099-01-01T00:00:00Z"},
		{name: "Unix seconds future", expiry: "4070908800"},
		{name: "Unix milliseconds future", expiry: "4070908800000"},
		{name: "expired", expiry: "2000-01-01T00:00:00Z", wantError: true},
		{name: "invalid", expiry: "invalid", wantError: true},
		{name: "legacy record", missing: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := importedRememberedFixture(t, false)
			records, err := f.store.WorkbenchDocuments(t.Context(), "remembered-account:", 10)
			if err != nil {
				t.Fatal(err)
			}
			var saved map[string]any
			if err := json.Unmarshal(records[0].Payload, &saved); err != nil {
				t.Fatal(err)
			}
			if tc.missing {
				delete(saved, "generation_credentials")
			} else {
				saved["generation_credentials"].(map[string]any)["expires_at"] = tc.expiry
			}
			raw, err := json.Marshal(saved)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.store.SaveWorkbenchDocument(t.Context(), records[0].ID, records[0].Revision, raw); err != nil {
				t.Fatal(err)
			}
			target, err := f.store.TargetSettings(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			got, _, err := f.service.GenerationCredentials(t.Context(), target, f.account)
			if (err != nil) != tc.wantError {
				t.Fatalf("expiry validation: %v", err)
			}
			if !tc.wantError && got["access_token"] == nil {
				t.Fatal("valid local token missing")
			}
		})
	}
}
