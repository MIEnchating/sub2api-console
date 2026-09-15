package accountworkbench_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestMixedPreviewAcceptsFormattedJSONRefreshTokenAndDelimitedLoginWithoutRemoteMutation(t *testing.T) {
	f := newBatchFixture(t, nil)
	content := `{
 "credentials":{"access_token":"json-private","chatgpt_account_id":"json-workspace","chatgpt_user_id":"json-user"}
}

rt_mixed_private
owner@example.com----login-private
`
	preview, err := f.service.PreviewWorkbenchRun(context.Background(), "owner", accountworkbench.WorkbenchRunInput{Content: content})
	if err != nil || preview.ID == "" || len(preview.Errors) != 0 || len(preview.Items) != 3 {
		t.Fatalf("mixed preview = %+v %v", preview, err)
	}
	t.Cleanup(func() { f.service.DeleteWorkbenchRunPreview("owner", preview.ID) })
	for i, kind := range []string{"sub2api_json", "refresh_token", "oauth_login"} {
		if preview.Items[i].Index != i || preview.Items[i].Kind != kind {
			t.Fatalf("source index/kind %d = %+v", i, preview.Items[i])
		}
	}
	f.remote.mu.Lock()
	defer f.remote.mu.Unlock()
	if f.remote.refreshes != 0 || f.remote.updates != 0 || f.remote.created != 0 {
		t.Fatal("mixed parse performed external refresh or mutation")
	}
	raw, _ := json.Marshal(preview)
	for _, secret := range []string{"json-private", "rt_mixed_private", "login-private"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("mixed preview leaked credentials")
		}
	}
}

func TestMixedPreviewArrayExpandsHeterogeneousItemsInOrderAndInvalidInputRejectsWholeBatch(t *testing.T) {
	f := newBatchFixture(t, nil)
	content := `[` + importedJSON + `,"rt_second",{"email":"owner@example.com","password":"login-private"}]`
	view, err := f.service.PreviewWorkbenchRun(context.Background(), "owner", accountworkbench.WorkbenchRunInput{Content: content})
	if err != nil || len(view.Items) != 3 || view.Items[2].Index != 2 || view.Items[2].Kind != "oauth_login" {
		t.Fatalf("heterogeneous array = %+v %v", view, err)
	}
	f.service.DeleteWorkbenchRunPreview("owner", view.ID)
	invalid, err := f.service.PreviewWorkbenchRun(context.Background(), "owner", accountworkbench.WorkbenchRunInput{Content: content + "\ninvalid-login"})
	if err != nil || invalid.ID != "" || len(invalid.Items) != 0 || len(invalid.Errors) != 1 || invalid.Errors[0].Index != 3 {
		t.Fatalf("invalid entry did not reject whole batch with original index: %+v %v", invalid, err)
	}
}

func TestMixedPreviewRejectsDuplicateAndAmbiguousJSONLoginFields(t *testing.T) {
	f := newBatchFixture(t, nil)
	for _, content := range []string{
		`{"email":"owner@example.com","password":"private","access_token":"token-private"}`,
		`{"email":"owner@example.com","email":"other@example.com"}`,
		"owner@example.com----private\nowner@example.com----different-private",
		`[` + importedJSON + `,` + importedJSON + `]`,
	} {
		view, err := f.service.PreviewWorkbenchRun(context.Background(), "owner", accountworkbench.WorkbenchRunInput{Content: content})
		if err != nil || view.ID != "" || len(view.Errors) == 0 {
			t.Fatal("ambiguous or duplicate input became executable", err)
		}
	}
}

func TestMixedPreviewPreservesStandardCodexAuthContainerWithEmail(t *testing.T) {
	f := newBatchFixture(t, nil)
	view, err := f.service.PreviewWorkbenchRun(context.Background(), "owner", accountworkbench.WorkbenchRunInput{Content: `{"email":"owner@example.com","auth":{"tokens":{"access_token":"private-auth-token","refresh_token":"rt_private"}}}`})
	if err != nil || view.ID == "" || len(view.Errors) != 0 || view.Items[0].Kind != "codex_json" {
		t.Fatalf("standard Codex auth container was misclassified: %+v %v", view, err)
	}
	f.service.DeleteWorkbenchRunPreview("owner", view.ID)
}

func TestMixedPreviewInvalidWrappedAccountRetainsIndexAmongValidNeighbors(t *testing.T) {
	f := newBatchFixture(t, nil)
	content := `{"accounts":[` + importedJSON + `,{"name":"Missing credentials"},` + mixedJSON + `]}\ninvalid-entry`
	content = strings.Replace(content, `\n`, "\n", 1)
	view, err := f.service.PreviewWorkbenchRun(context.Background(), "owner", accountworkbench.WorkbenchRunInput{Content: content})
	if err != nil || len(view.Errors) != 2 || view.Errors[0].Index != 1 || view.Errors[1].Index != 3 {
		t.Fatalf("wrapped invalid source index = %+v %v", view.Errors, err)
	}
}
