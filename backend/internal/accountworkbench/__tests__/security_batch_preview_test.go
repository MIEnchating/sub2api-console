package accountworkbench_test

import (
	"context"
	"errors"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
	"github.com/MIEnchating/sub2api-console/backend/internal/targetguard"
)

func TestSecurityBatchPreviewRejectsInvalidRangeAndWeakOrCrossOperationPassword(t *testing.T) {
	f := newSecurityBatchFixture(t, nil)
	for _, input := range []accountworkbench.SecurityBatchPreviewInput{
		{Operation: "totp"},
		{AccountIDs: []string{"01"}, Operation: "totp"},
		{AccountIDs: []string{"101", "101"}, Operation: "totp"},
		{AccountIDs: make([]string, 501), Operation: "totp"},
		{AccountIDs: []string{"101"}, Operation: "password", Password: "weak"},
		{AccountIDs: []string{"101"}, Operation: "totp", Password: securityPassword},
	} {
		if _, err := f.service.PreviewSecurityBatch(context.Background(), "owner", input); err == nil {
			t.Fatal("invalid security batch input accepted")
		}
	}
}

func TestSecurityBatchPreviewRejectsDuplicateOfficialUserAcrossStableAccounts(t *testing.T) {
	f := newSecurityBatchFixture(t, nil)
	f.remote.accounts["102"]["credentials"].(map[string]any)["chatgpt_user_id"] = "user-101"
	preview, err := f.service.PreviewSecurityBatch(context.Background(), "owner", accountworkbench.SecurityBatchPreviewInput{AccountIDs: []string{"101", "102"}, Operation: "totp"})
	if err != nil || preview.ID != "" || len(preview.Items) != 0 || len(preview.Errors) != 1 || preview.Errors[0].Index != 1 {
		t.Fatalf("duplicate official user not rejected: %+v %v", preview, err)
	}
}

func TestSecurityBatchPreviewRejectsMissingAndUnsupportedAccountsWithoutPartialScope(t *testing.T) {
	f := newSecurityBatchFixture(t, nil)
	f.remote.accounts["102"]["platform"] = "anthropic"
	preview, err := f.service.PreviewSecurityBatch(context.Background(), "owner", accountworkbench.SecurityBatchPreviewInput{AccountIDs: []string{"101", "102", "103"}, Operation: "totp"})
	if err != nil || preview.ID != "" || len(preview.Items) != 0 || len(preview.Errors) != 2 {
		t.Fatalf("invalid scope retained partial executable preview: %+v %v", preview, err)
	}
}

func TestSecurityBatchStartRejectsChangedTargetBeforeBrowserLaunch(t *testing.T) {
	f := newSecurityBatchFixture(t, nil)
	preview := f.previewSecurityBatch(t, "totp", "101")
	if err := f.private.ConfigureTarget(context.Background(), f.server.URL, "changed-test-key", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.StartSecurityBatch(context.Background(), "owner", preview.ID, true); !errors.Is(err, targetguard.ErrChanged) {
		t.Fatalf("changed target accepted: %v", err)
	}
}

func TestSecurityBatchStartRejectsChangedStableIdentityAndKeepsNameIrrelevant(t *testing.T) {
	f := newSecurityBatchFixture(t, nil)
	preview := f.previewSecurityBatch(t, "totp", "101")
	f.remote.mu.Lock()
	f.remote.accounts["101"]["credentials"].(map[string]any)["chatgpt_user_id"] = "changed-user"
	f.remote.mu.Unlock()
	if _, err := f.service.StartSecurityBatch(context.Background(), "owner", preview.ID, true); err == nil {
		t.Fatal("rebound stable account accepted")
	}
}

func TestSecurityBatchPreviewAndStartCannotBeUsedByForeignOwnerOrWithoutConfirmation(t *testing.T) {
	f := newSecurityBatchFixture(t, nil)
	preview := f.previewSecurityBatch(t, "totp", "101")
	if _, err := f.service.StartSecurityBatch(context.Background(), "foreign", preview.ID, true); err == nil {
		t.Fatal("foreign preview accepted")
	}
	if _, err := f.service.StartSecurityBatch(context.Background(), "owner", preview.ID, false); err == nil {
		t.Fatal("unconfirmed batch accepted")
	}
	view, err := f.service.StartSecurityBatch(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CancelSecurityBatch("owner", view.ID) })
	if _, err := f.service.ReadSecurityBatch("foreign", view.ID); !errors.Is(err, browserlogin.ErrSession) {
		t.Fatal("foreign batch read accepted")
	}
	if err := f.service.CancelSecurityBatch("foreign", view.ID); !errors.Is(err, browserlogin.ErrSession) {
		t.Fatal("foreign batch cancellation accepted")
	}
}
