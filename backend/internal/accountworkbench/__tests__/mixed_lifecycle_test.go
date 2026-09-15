package accountworkbench_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestMixedRunRequiresStartConfirmationAndRejectsChangedTargetBeforeQueue(t *testing.T) {
	f := newBatchFixture(t, nil)
	preview, err := f.service.PreviewWorkbenchRun(context.Background(), "owner", accountworkbench.WorkbenchRunInput{Content: mixedJSON})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.service.DeleteWorkbenchRunPreview("owner", preview.ID) })
	if _, err := f.service.StartWorkbenchRun(context.Background(), "owner", preview.ID, false); err == nil {
		t.Fatal("mixed run started without scope confirmation")
	}
	if err := f.private.ConfigureTarget(context.Background(), f.server.URL, "changed-admin-key", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.StartWorkbenchRun(context.Background(), "owner", preview.ID, true); err == nil {
		t.Fatal("mixed run used stale management target")
	}
	if f.runBoundary.calls.Load() != 0 {
		t.Fatal("rejected mixed run entered task queue")
	}
}

func TestMixedRunTemplateChangeAfterPreparationRevokesFinalCheckpoint(t *testing.T) {
	f := newBatchFixture(t, nil)
	template, err := f.service.SaveTemplate(context.Background(), "", accountworkbench.TemplateInput{Name: "Mixed template", Config: configstore.WorkbenchTemplateConfig{"concurrency": json.RawMessage(`3`)}})
	if err != nil {
		t.Fatal(err)
	}
	view := startMixed(t, f, mixedJSON, false)
	f.awaitDone(t, view.ID)
	_, err = f.service.SaveTemplate(context.Background(), template.ID, accountworkbench.TemplateInput{Name: template.Name, Revision: template.Revision, Config: configstore.WorkbenchTemplateConfig{"concurrency": json.RawMessage(`4`)}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.PreviewWorkbenchRunResult(context.Background(), "owner", view.ID); err == nil {
		t.Fatal("mixed checkpoint silently selected changed template")
	}
}

func TestMixedRunResultPersistenceFailureDoesNotExposeCredentialsForImport(t *testing.T) {
	f := newBatchFixture(t, nil)
	f.taskBoundary.failMixedFinal = true
	view := startMixed(t, f, mixedJSON, false)
	f.awaitDone(t, view.ID)
	finished, err := f.service.ReadWorkbenchRun(context.Background(), "owner", view.ID)
	if err != nil || finished.Status != "failed" || finished.Available != 0 {
		t.Fatalf("failed persistence remained ready: %+v %v", finished, err)
	}
	if _, err := f.service.PreviewWorkbenchRunResult(context.Background(), "owner", view.ID); err == nil {
		t.Fatal("mixed task exposed unrecorded results")
	}
}

func TestMixedPreviewSupportsQuotedMailboxDelimiterAndSharedSMSMetadataWithoutSecrets(t *testing.T) {
	f := newBatchFixture(t, nil)
	content := "\"owner@example.com\"|\"pipe|inside-password\""
	view, err := f.service.PreviewWorkbenchRun(context.Background(), "owner", accountworkbench.WorkbenchRunInput{Content: content, SMS: &accountworkbench.OAuthSMSInput{Provider: "smsbower", APIKey: "private-sms-key", Service: "dr", Country: "0", MaxPrice: "1.25"}})
	if err != nil || view.ID == "" || len(view.Items) != 1 || view.Items[0].SMSProvider != "smsbower" || !view.Items[0].HasPassword {
		t.Fatalf("quoted mailbox fields not preserved: %+v %v", view, err)
	}
	f.service.DeleteWorkbenchRunPreview("owner", view.ID)
	raw, _ := json.Marshal(view)
	if strings.Contains(string(raw), "pipe|inside-password") || strings.Contains(string(raw), "private-sms-key") {
		t.Fatal("mixed preview exposed private login/SMS metadata")
	}
}

func TestMixedRunOwnsLoginPreparationWhenAnotherOAuthPreviewIsOpened(t *testing.T) {
	f := newBatchFixture(t, nil)
	preview, err := f.service.PreviewWorkbenchRun(context.Background(), "owner", accountworkbench.WorkbenchRunInput{Content: "owner@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.service.PreviewOAuthBatch(context.Background(), "owner", accountworkbench.OAuthBatchPreviewInput{Content: "second@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	defer f.service.DeleteOAuthBatchPreview("owner", other.ID)
	view, err := f.service.StartWorkbenchRun(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CancelWorkbenchRun("owner", view.ID) })
	child := f.awaitTask(t, "account-workbench-oauth", "waiting")
	if err := f.service.FinishOAuth("owner", child.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, view.ID)
	finished, err := f.service.ReadWorkbenchRun(context.Background(), "owner", view.ID)
	if err != nil || finished.Available != 1 {
		t.Fatalf("mixed login preparation was discarded by unrelated preview: %+v %v", finished, err)
	}
}
