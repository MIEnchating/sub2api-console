package accountworkbench_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func profileExchange(f *batchFixture, user, workspace string) {
	f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
		claims, _ := json.Marshal(map[string]any{"email": "owner@example.com", "https://api.openai.com/auth": map[string]any{"chatgpt_user_id": user, "chatgpt_account_id": workspace}})
		payload := base64.RawURLEncoding.EncodeToString(claims)
		return oauthResponse(fmt.Sprintf(`{"access_token":"eyJhbGciOiJub25lIn0.%s.profile-private","refresh_token":"rt_profile_private","token_type":"Bearer"}`, payload)), nil
	}))
}

func startReauthorization(t *testing.T, f *batchFixture, id string) accountworkbench.OAuthBatchView {
	t.Helper()
	preview, err := f.service.PreviewReauthorization(context.Background(), "owner", accountworkbench.ReauthorizationPreviewInput{AccountIDs: []string{id}, FreshLogin: true})
	if err != nil || preview.ID == "" || !preview.FreshLogin || len(preview.Items) != 1 || preview.Items[0].AccountID != id {
		t.Fatalf("reAuth preview missing stable binding: %+v %v", preview, err)
	}
	view, err := f.service.StartOAuthBatch(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CancelOAuthBatch("owner", view.ID) })
	return view
}

func finishProfileAuthorization(t *testing.T, f *batchFixture, view accountworkbench.OAuthBatchView) {
	t.Helper()
	child := f.awaitTask(t, "account-workbench-oauth", "waiting")
	if err := f.service.FinishOAuth("owner", child.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, view.ID)
}

func TestReauthorizationRejectsSameEmailWithDifferentOfficialUser(t *testing.T) {
	f := newProfileFixture(t)
	saveProfile(t, f, "101")
	profileExchange(f, "wrong-user", "workspace-101")
	view := startReauthorization(t, f, "101")
	finishProfileAuthorization(t, f, view)
	stored, err := f.service.ReadOAuthBatch("owner", view.ID)
	if err != nil || stored.Status != "failed" || stored.Available != 0 {
		t.Fatalf("same email bypassed stable user check: %+v %v", stored, err)
	}
}

func TestReauthorizationSuccessUpdatesOnlyOriginalAccountAndDeletedProfileRevokesImport(t *testing.T) {
	f := newProfileFixture(t)
	profile := saveProfile(t, f, "101")
	profileExchange(f, "user-101", "workspace-101")
	view := startReauthorization(t, f, "101")
	finishProfileAuthorization(t, f, view)
	preview, err := f.service.PreviewOAuthBatchImport(context.Background(), "owner", view.ID, accountworkbench.OAuthPreviewInput{})
	if err != nil || len(preview.Items) != 1 || preview.Items[0].AccountID != "101" || !preview.Items[0].Duplicate {
		t.Fatalf("reAuth preview changed update target: %+v %v", preview.Items, err)
	}
	if err := f.service.DeleteLoginProfile(context.Background(), "owner", profile.ID, profile.Revision, true); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Import(context.Background(), "owner", preview.ID, true); !errors.Is(err, configstore.ErrWorkbenchLoginProfile) {
		t.Fatalf("deleted login profile did not revoke import: %v", err)
	}
	f.remote.mu.Lock()
	defer f.remote.mu.Unlock()
	if f.remote.updates != 0 || f.remote.created != 0 {
		t.Fatal("revoked reAuth updated upstream")
	}
}

func TestReauthorizationChangedProfileInvalidatesPreviewBeforeTaskLaunch(t *testing.T) {
	f := newProfileFixture(t)
	profile := saveProfile(t, f, "101")
	preview, err := f.service.PreviewReauthorization(context.Background(), "owner", accountworkbench.ReauthorizationPreviewInput{AccountIDs: []string{"101"}, FreshLogin: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.service.SaveLoginProfile(context.Background(), "owner", accountworkbench.LoginProfileSaveInput{ID: profile.ID, AccountID: "101", Revision: profile.Revision, Confirmed: true, Login: accountworkbench.OAuthLoginInput{Email: profile.Email, Password: "new-profile-password"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.StartOAuthBatch(context.Background(), "owner", preview.ID, true); !errors.Is(err, configstore.ErrWorkbenchLoginProfile) {
		t.Fatalf("stale profile preview started: %v", err)
	}
}

func TestReauthorizationFailedBatchCreatesNewAttemptUsingCurrentSavedProfile(t *testing.T) {
	f := newProfileFixture(t)
	saveProfile(t, f, "101")
	profileExchange(f, "wrong-user", "workspace-101")
	view := startReauthorization(t, f, "101")
	finishProfileAuthorization(t, f, view)
	preview, err := f.service.PreviewReauthorization(context.Background(), "owner", accountworkbench.ReauthorizationPreviewInput{FailedBatchID: view.ID, FreshLogin: true})
	if err != nil || len(preview.Items) != 1 || preview.Items[0].AccountID != "101" || preview.ID == view.ID {
		t.Fatalf("failed relogin not separately previewed: %+v %v", preview, err)
	}
	if _, err := f.service.PreviewReauthorization(context.Background(), "other", accountworkbench.ReauthorizationPreviewInput{FailedBatchID: view.ID, FreshLogin: true}); err == nil {
		t.Fatal("foreign session read failed batch profile")
	}
	if _, err := f.service.PreviewReauthorization(context.Background(), "owner", accountworkbench.ReauthorizationPreviewInput{AccountIDs: []string{"101"}}); err == nil {
		t.Fatal("reAuth accepted without fresh login confirmation")
	}
}
