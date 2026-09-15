package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

const sourceProfileJSON = `{"name":"Local owner","credentials":{"email":"owner@example.com","access_token":"access-local-private","refresh_token":"rt_local_private","chatgpt_account_id":"workspace-1","chatgpt_user_id":"user-1"}}`

func saveLocalSourceProfile(t *testing.T, service *accountworkbench.Service, owner string, source accountworkbench.SourceProfileReference) accountworkbench.SourceProfileView {
	t.Helper()
	identity, err := service.SourceProfileIdentity(context.Background(), owner, accountworkbench.SourceProfileIdentityInput{Scope: accountworkbench.ScopeLocalExport, Source: source})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := service.SaveSourceProfile(context.Background(), owner, accountworkbench.SourceProfileSaveInput{Scope: accountworkbench.ScopeLocalExport, Source: source, SourceRevision: identity.SourceRevision, Confirmed: true, Login: accountworkbench.OAuthLoginInput{Email: identity.Email, Password: "private-source-password", TOTPSecret: securitySecret}})
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func TestSourceProfileArtifactCreationReplacementAndDeletionRemainLocalAndPrivate(t *testing.T) {
	f, _, management := localExportFixture(t)
	artifact, _ := localRegenerationSource(t, f, sourceProfileJSON)
	index := 0
	source := accountworkbench.SourceProfileReference{ArtifactID: artifact.ID, Index: &index}
	profile := saveLocalSourceProfile(t, f.service, "local-owner", source)
	if profile.Scope != accountworkbench.ScopeLocalExport || profile.UserID != "user-1" || !profile.HasPassword || !profile.HasTOTP {
		t.Fatal("saved source metadata did not preserve verified identity")
	}
	if err := f.private.ConfigureTarget(context.Background(), "https://unrelated.example", "not-a-real-key", 3); err != nil {
		t.Fatal(err)
	}
	profiles, err := f.service.SourceProfiles(context.Background(), "local-owner", accountworkbench.ScopeLocalExport)
	if err != nil || len(profiles) != 1 || management.Load() != 0 {
		t.Fatal("local profile depended on configured management target")
	}
	if err := f.service.DeleteLocalExport(context.Background(), "local-owner", artifact.ID); err != nil {
		t.Fatal(err)
	}
	input := accountworkbench.SourceProfileSaveInput{Scope: accountworkbench.ScopeLocalExport, ID: profile.ID, Revision: profile.Revision, Confirmed: true, Login: accountworkbench.OAuthLoginInput{Email: profile.Email, Password: "private-replaced-password"}}
	updated, err := f.service.SaveSourceProfile(context.Background(), "local-owner", input)
	if err != nil || updated.Revision != 2 || updated.HasTOTP {
		t.Fatalf("profile replacement after source expiry/deletion = %+v, %v", updated, err)
	}
	if _, err := f.service.SaveSourceProfile(context.Background(), "local-owner", input); !errors.Is(err, configstore.ErrWorkbenchSourceProfile) {
		t.Fatal("stale replacement was accepted")
	}
	if _, err := f.service.SaveSourceProfile(context.Background(), "other-owner", input); !errors.Is(err, configstore.ErrWorkbenchSourceProfile) {
		t.Fatal("another session replaced private login data")
	}
	raw, _ := json.Marshal([]any{profile, updated, profiles})
	for _, secret := range []string{"private-source-password", securitySecret, "private-replaced-password", "access-local-private", "rt_local_private", "account_id"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("public source profile contains sensitive or fabricated field %q", secret)
		}
	}
	if err := f.service.DeleteSourceProfile(context.Background(), "local-owner", profile.ID, accountworkbench.ScopeLocalExport, updated.Revision, true); err != nil {
		t.Fatal(err)
	}
	profiles, err = f.service.SourceProfiles(context.Background(), "local-owner", accountworkbench.ScopeLocalExport)
	if err != nil || len(profiles) != 0 {
		t.Fatal("deleted local profile remained available")
	}
}

func TestSourceProfileRejectsWrongOwnerScopeIndexIdentityAndSourceRevision(t *testing.T) {
	f, _, _ := localExportFixture(t)
	artifact, _ := localRegenerationSource(t, f, sourceProfileJSON)
	index := 0
	source := accountworkbench.SourceProfileReference{ArtifactID: artifact.ID, Index: &index}
	identity, err := f.service.SourceProfileIdentity(context.Background(), "local-owner", accountworkbench.SourceProfileIdentityInput{Scope: accountworkbench.ScopeLocalExport, Source: source})
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"owner", "scope", "index", "email", "workspace", "revision", "confirmation"} {
		t.Run(scenario, func(t *testing.T) {
			owner := "local-owner"
			input := accountworkbench.SourceProfileSaveInput{Scope: accountworkbench.ScopeLocalExport, Source: source, SourceRevision: identity.SourceRevision, Confirmed: true, Login: accountworkbench.OAuthLoginInput{Email: identity.Email}}
			switch scenario {
			case "owner":
				owner = "another-owner"
			case "scope":
				input.Scope = accountworkbench.ScopeManaged
			case "index":
				otherIndex := 1
				input.Source.Index = &otherIndex
			case "email":
				input.Login.Email = "another@example.com"
			case "workspace":
				input.Login.WorkspaceID = "other-workspace"
			case "revision":
				input.SourceRevision = "stale"
			case "confirmation":
				input.Confirmed = false
			}
			if _, err := f.service.SaveSourceProfile(context.Background(), owner, input); err == nil {
				t.Fatal("invalid profile creation accepted")
			}
		})
	}
	profiles, err := f.service.SourceProfiles(context.Background(), "local-owner", accountworkbench.ScopeLocalExport)
	if err != nil || len(profiles) != 0 {
		t.Fatal("rejected creation persisted credentials")
	}
}

func TestSourceProfileExportConfirmsVersionsWritesOnlyPrivateFileAndCannotReplay(t *testing.T) {
	f, directory, requests := localExportFixture(t)
	artifact, _ := localRegenerationSource(t, f, sourceProfileJSON)
	index := 0
	profile := saveLocalSourceProfile(t, f.service, "local-owner", accountworkbench.SourceProfileReference{ArtifactID: artifact.ID, Index: &index})
	selection := accountworkbench.SourceProfileSelectionInput{Scope: accountworkbench.ScopeLocalExport, Items: []accountworkbench.ProfileExportSelection{{ID: profile.ID, Revision: profile.Revision}}}
	preview, err := f.service.PreviewSourceProfileExport(context.Background(), "local-owner", selection)
	if err != nil || len(preview.Items) != 1 || preview.Kind != accountworkbench.ExportLoginProfiles {
		t.Fatalf("preview = %+v, %v", preview, err)
	}
	if _, err := f.service.ExportSourceProfiles(context.Background(), "local-owner", preview.ID, false); err == nil {
		t.Fatal("unconfirmed private profile export started")
	}
	if _, err := f.service.ExportSourceProfiles(context.Background(), "other-owner", preview.ID, true); err == nil {
		t.Fatal("another owner consumed profile export preview")
	}
	if _, err := f.service.ExportSourceProfiles(context.Background(), "local-owner", preview.ID, true); err != nil {
		t.Fatal(err)
	}
	task := f.await(t)
	if task.Status != "succeeded" || requests.Load() != 0 {
		t.Fatalf("local private profile export = %+v", task)
	}
	items := task.Result["items"].([]accountworkbench.ResultItem)
	id := items[0].Report["artifact_id"].(string)
	raw, err := os.ReadFile(filepath.Join(directory, id+".json"))
	if err != nil || !strings.Contains(string(raw), "private-source-password") || !strings.Contains(string(raw), securitySecret) || strings.Contains(string(raw), "account_id") {
		t.Fatal("private profile export lost data or fabricated remote account")
	}
	public, _ := json.Marshal([]any{preview, task})
	if strings.Contains(string(public), "private-source-password") || strings.Contains(string(public), securitySecret) {
		t.Fatal("profile export exposed private fields")
	}
	if _, err := f.service.ExportSourceProfiles(context.Background(), "local-owner", preview.ID, true); err == nil {
		t.Fatal("profile export preview replayed")
	}
	files, err := f.service.LocalExports(context.Background(), "local-owner")
	if err != nil || len(files) != 2 {
		t.Fatal("local profile artifact did not appear in local namespace")
	}
}

func TestSourceProfileExportRejectsReplacementAfterPreviewWithoutProducingFile(t *testing.T) {
	f, _, _ := localExportFixture(t)
	artifact, _ := localRegenerationSource(t, f, sourceProfileJSON)
	index := 0
	profile := saveLocalSourceProfile(t, f.service, "local-owner", accountworkbench.SourceProfileReference{ArtifactID: artifact.ID, Index: &index})
	preview, err := f.service.PreviewSourceProfileExport(context.Background(), "local-owner", accountworkbench.SourceProfileSelectionInput{Scope: accountworkbench.ScopeLocalExport, Items: []accountworkbench.ProfileExportSelection{{ID: profile.ID, Revision: profile.Revision}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.SaveSourceProfile(context.Background(), "local-owner", accountworkbench.SourceProfileSaveInput{Scope: accountworkbench.ScopeLocalExport, ID: profile.ID, Revision: profile.Revision, Confirmed: true, Login: accountworkbench.OAuthLoginInput{Email: profile.Email}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ExportSourceProfiles(context.Background(), "local-owner", preview.ID, true); err != nil {
		t.Fatal(err)
	}
	if task := f.await(t); task.Status != "failed" {
		t.Fatal("changed profile export succeeded")
	}
	files, err := f.service.LocalExports(context.Background(), "local-owner")
	if err != nil || len(files) != 1 {
		t.Fatal("changed profile produced an export")
	}
}
