package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func saveProfile(t *testing.T, f *batchFixture, id string) accountworkbench.LoginProfileView {
	t.Helper()
	view, err := f.service.SaveLoginProfile(context.Background(), "owner", accountworkbench.LoginProfileSaveInput{AccountID: id, Confirmed: true, Login: accountworkbench.OAuthLoginInput{Email: "owner@example.com", Password: "private-saved-password", TOTPSecret: securitySecret}})
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func newProfileFixture(t *testing.T) *batchFixture {
	f := newBatchFixture(t, nil)
	f.remote.accounts["101"] = exportAccount("101", "Shared name")
	f.remote.accounts["102"] = exportAccount("102", "Shared name")
	return f
}

func TestLoginProfileSaveRequiresExplicitConfirmationAndMatchingStableIdentity(t *testing.T) {
	f := newProfileFixture(t)
	for _, input := range []accountworkbench.LoginProfileSaveInput{
		{AccountID: "101", Login: accountworkbench.OAuthLoginInput{Email: "owner@example.com"}},
		{AccountID: "101", Confirmed: true, Login: accountworkbench.OAuthLoginInput{Email: "other@example.com"}},
		{AccountID: "101", Confirmed: true, Login: accountworkbench.OAuthLoginInput{Email: "owner@example.com", WorkspaceID: "other-workspace"}},
		{AccountID: "01", Confirmed: true, Login: accountworkbench.OAuthLoginInput{Email: "owner@example.com"}},
	} {
		if _, err := f.service.SaveLoginProfile(context.Background(), "owner", input); err == nil {
			t.Fatal("invalid or unconfirmed profile persisted")
		}
	}
	values, err := f.service.LoginProfiles(context.Background())
	if err != nil || len(values) != 0 {
		t.Fatal("rejected input left saved credentials")
	}
}

func TestLoginProfilesExposeOnlyMetadataAndCASProtectsUpdateAndDelete(t *testing.T) {
	f := newProfileFixture(t)
	profile := saveProfile(t, f, "101")
	profiles, err := f.service.LoginProfiles(context.Background())
	if err != nil || len(profiles) != 1 || !profiles[0].HasPassword || !profiles[0].HasTOTP || profiles[0].UserID != "user-101" || profiles[0].WorkspaceID != "workspace-101" {
		t.Fatalf("profile metadata missing: %+v %v", profiles, err)
	}
	raw, _ := json.Marshal(profiles)
	for _, secret := range []string{"private-saved-password", securitySecret, "export-access-private", "test-admin-key"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("profile metadata exposes credentials")
		}
	}
	input := accountworkbench.LoginProfileSaveInput{ID: profile.ID, Revision: profile.Revision, AccountID: profile.AccountID, Confirmed: true, Login: accountworkbench.OAuthLoginInput{Email: profile.Email, Password: "private-updated-password"}}
	updated, err := f.service.SaveLoginProfile(context.Background(), "owner", input)
	if err != nil || updated.Revision != 2 {
		t.Fatal(err)
	}
	if _, err := f.service.SaveLoginProfile(context.Background(), "owner", input); !errors.Is(err, configstore.ErrWorkbenchLoginProfile) {
		t.Fatal("stale profile update accepted")
	}
	if err := f.service.DeleteLoginProfile(context.Background(), "owner", profile.ID, 1, true); !errors.Is(err, configstore.ErrWorkbenchLoginProfile) {
		t.Fatal("stale deletion accepted")
	}
	if err := f.service.DeleteLoginProfile(context.Background(), "owner", profile.ID, 2, false); err == nil {
		t.Fatal("unconfirmed deletion accepted")
	}
	if err := f.service.DeleteLoginProfile(context.Background(), "owner", profile.ID, 2, true); err != nil {
		t.Fatal(err)
	}
}

func TestLoginProfileMaintenanceCandidatesExcludeChangedIdentityAndOtherTargets(t *testing.T) {
	f := newProfileFixture(t)
	saveProfile(t, f, "101")
	values, err := f.service.ReauthorizationCandidates(context.Background(), []string{"101", "102"})
	if err != nil || len(values) != 1 || !values[0].RequiresConfirmation {
		t.Fatalf("maintenance candidate missing: %+v %v", values, err)
	}
	f.remote.mu.Lock()
	f.remote.accounts["101"]["credentials"].(map[string]any)["chatgpt_user_id"] = "other-user"
	f.remote.mu.Unlock()
	values, err = f.service.ReauthorizationCandidates(context.Background(), []string{"101"})
	if err != nil || len(values) != 0 {
		t.Fatal("changed official identity remained auto-maintenance candidate")
	}
	if err := f.private.ConfigureTarget(context.Background(), f.server.URL, "new-admin-key", 3); err != nil {
		t.Fatal(err)
	}
	profiles, err := f.service.LoginProfiles(context.Background())
	if err != nil || len(profiles) != 0 {
		t.Fatal("profiles available under changed target credential")
	}
}
