package accountworkbench_test

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func TestSecurityBatchRetainsMoreThanTwentySourceResults(t *testing.T) {
	f := newSecurityBatchFixture(t, nil)
	logins := make([]accountworkbench.OAuthLoginInput, 21)
	f.factory.browsers = nil
	for index := range logins {
		id := strconv.Itoa(index + 101)
		logins[index] = accountworkbench.OAuthLoginInput{Email: "owner@example.com", WorkspaceID: "workspace-" + id}
		f.factory.browsers = append(f.factory.browsers, &securityBrowser{identity: browserlogin.SecurityIdentity{Email: "owner@example.com", UserID: "user-" + id}, enrollment: browserlogin.SecurityEnrollment{Secret: securitySecret, SessionID: "private-session-" + id}, closed: make(chan struct{})})
	}
	raw, _ := json.Marshal(logins)
	preview, err := f.service.PreviewOAuthBatch(context.Background(), "owner", accountworkbench.OAuthBatchPreviewInput{Scope: accountworkbench.ScopeLocalExport, Content: string(raw)})
	if err != nil || preview.ID == "" {
		t.Fatalf("source batch preview: %v", err)
	}
	source, err := f.service.StartOAuthBatch(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CancelOAuthBatch("owner", source.ID) })
	refs := make([]accountworkbench.SecuritySourceReference, len(logins))
	for index := range logins {
		child := f.awaitTask(t, "account-workbench-oauth", "waiting")
		id := strconv.Itoa(index + 101)
		profileExchange(f.batchFixture, "user-"+id, "workspace-"+id)
		if err := f.service.FinishOAuth("owner", child.ID); err != nil {
			t.Fatal(err)
		}
		copy := index
		refs[index] = accountworkbench.SecuritySourceReference{OAuthBatchID: source.ID, Index: &copy}
	}
	f.awaitDone(t, source.ID)
	securityPreview, err := f.service.PreviewSecurityBatch(context.Background(), "owner", accountworkbench.SecurityBatchPreviewInput{Scope: source.Scope, Sources: refs, Operation: "totp"})
	if err != nil || securityPreview.ID == "" {
		t.Fatalf("security preview: %v", err)
	}
	view, err := f.service.StartSecurityBatch(context.Background(), "owner", securityPreview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CancelSecurityBatch("owner", view.ID) })
	for range logins {
		f.finishSecurityChild(t, "totp")
	}
	f.awaitDone(t, view.ID)
	stored, err := f.service.ReadSecurityBatch("owner", view.ID)
	if err != nil || stored.Succeeded != 21 {
		t.Fatalf("batch stopped because finished source results occupied browser capacity: %+v %v", stored, err)
	}
	for _, row := range stored.Items {
		if _, err := f.service.Security(context.Background(), "owner", row.SecurityID, false); err != nil {
			t.Fatal("completed security result was lost")
		}
	}
}
