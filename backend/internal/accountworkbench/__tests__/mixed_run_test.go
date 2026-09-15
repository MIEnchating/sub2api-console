package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

const mixedJSON = `{"name":"JSON account","credentials":{"access_token":"mixed-json-private","chatgpt_user_id":"mixed-json-user","chatgpt_account_id":"mixed-json-workspace"}}`

func startMixed(t *testing.T, f *batchFixture, content string, exportOnly bool) accountworkbench.WorkbenchRunView {
	t.Helper()
	preview, err := f.service.PreviewWorkbenchRun(context.Background(), "owner", accountworkbench.WorkbenchRunInput{Content: content, ExportOnly: exportOnly, CheckAfterImport: true})
	if err != nil || preview.ID == "" || len(preview.Errors) > 0 {
		t.Fatalf("mixed preview = %+v %v", preview, err)
	}
	view, err := f.service.StartWorkbenchRun(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CancelWorkbenchRun("owner", view.ID) })
	return view
}

func TestMixedRunCombinesJSONRefreshAndOAuthThenExportsOnlyAfterFinalConfirmation(t *testing.T) {
	f := newBatchFixture(t, nil)
	if err := f.service.UseExportDirectory(filepath.Join(t.TempDir(), "exports")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CloseExports() })
	profileExchange(f, "mixed-login-user", "mixed-login-workspace")
	view := startMixed(t, f, mixedJSON+"\nrt_mixed_private\nowner@example.com----login-private", true)
	child := f.awaitTask(t, "account-workbench-oauth", "waiting")
	current, err := f.service.ReadWorkbenchRun(context.Background(), "owner", view.ID)
	if err != nil || current.CurrentOAuthID != child.ID || current.OAuthBatchID == "" || !current.ExportOnly {
		t.Fatalf("mixed browser takeover unavailable: %+v %v", current, err)
	}
	if _, err := f.service.ReadWorkbenchRun(context.Background(), "other", view.ID); !errors.Is(err, browserlogin.ErrSession) {
		t.Fatal("foreign session read mixed batch")
	}
	if err := f.service.FinishOAuth("owner", child.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, view.ID)
	finished, err := f.service.ReadWorkbenchRun(context.Background(), "owner", view.ID)
	if err != nil || finished.Status != "ready" || finished.Available != 3 {
		t.Fatalf("mixed batch not ready: %+v %v", finished, err)
	}
	preview, err := f.service.PreviewWorkbenchRunResult(context.Background(), "owner", view.ID)
	if err != nil || !preview.ExportOnly || preview.CheckAfterImport || len(preview.Items) != 3 {
		t.Fatalf("mixed checkpoint = %+v %v", preview, err)
	}
	for i, row := range preview.Items {
		if row.Index != i {
			t.Fatal("merged checkpoint reordered original indexes")
		}
	}
	artifacts, err := f.service.Exports(context.Background(), "owner")
	if err != nil || len(artifacts) != 0 {
		t.Fatal("mixed run exported before final confirmation")
	}
	if _, err := f.service.Import(context.Background(), "owner", preview.ID, true); err == nil {
		t.Fatal("private export checkpoint accepted account import")
	}
	if _, err := f.service.ExportInput(context.Background(), "owner", preview.ID, false); err == nil {
		t.Fatal("mixed private export did not require final confirmation")
	}
	task, err := f.service.ExportInput(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, task.ID)
	artifacts, err = f.service.Exports(context.Background(), "owner")
	if err != nil || len(artifacts) != 1 || artifacts[0].Count != 3 {
		t.Fatal("mixed private export did not include all sources", err)
	}
	f.remote.mu.Lock()
	defer f.remote.mu.Unlock()
	if f.remote.refreshes != 1 || f.remote.created != 0 || f.remote.updates != 0 {
		t.Fatal("mixed export repeated RT refresh or changed managed accounts")
	}
	history, err := f.tasks.Get(context.Background(), view.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal([]any{finished, preview, history})
	for _, secret := range []string{"mixed-json-private", "rt_mixed_private", "login-private", "rt_profile_private"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("mixed public metadata exposed credentials")
		}
	}
}

func TestMixedRunRejectsCrossFormatStableDuplicateAfterOAuthCompletes(t *testing.T) {
	f := newBatchFixture(t, nil)
	profileExchange(f, "mixed-json-user", "mixed-json-workspace")
	view := startMixed(t, f, mixedJSON+"\nowner@example.com", false)
	child := f.awaitTask(t, "account-workbench-oauth", "waiting")
	if err := f.service.FinishOAuth("owner", child.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, view.ID)
	finished, err := f.service.ReadWorkbenchRun(context.Background(), "owner", view.ID)
	if err != nil || finished.Status != "failed" || finished.Available != 0 || len(finished.Errors) != 1 || finished.Errors[0].Index != 1 {
		t.Fatalf("cross-format identity duplicated: %+v %v", finished, err)
	}
	if _, err := f.service.PreviewWorkbenchRunResult(context.Background(), "owner", view.ID); err == nil {
		t.Fatal("duplicate mixed identities received executable checkpoint")
	}
}

func TestMixedRunFailedOAuthRetainsOtherSourcesAtTheirOriginalIndexes(t *testing.T) {
	f := newBatchFixture(t, nil)
	f.service.UseOAuthTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) { return oauthResponse(`{"error":"invalid_grant"}`), nil }))
	view := startMixed(t, f, mixedJSON+"\nowner@example.com\nrt_mixed_private", false)
	child := f.awaitTask(t, "account-workbench-oauth", "waiting")
	if err := f.service.FinishOAuth("owner", child.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, view.ID)
	preview, err := f.service.PreviewWorkbenchRunResult(context.Background(), "owner", view.ID)
	if err != nil || len(preview.Items) != 2 || preview.Items[0].Index != 0 || preview.Items[1].Index != 2 {
		t.Fatalf("partial mixed batch lost source indexes: %+v %v", preview, err)
	}
}

func TestMixedRunCancellationClosesActiveOAuthAndRevokesFinalPreview(t *testing.T) {
	f := newBatchFixture(t, nil)
	view := startMixed(t, f, mixedJSON+"\nowner@example.com", false)
	f.awaitTask(t, "account-workbench-oauth", "waiting")
	if err := f.service.CancelWorkbenchRun("other", view.ID); !errors.Is(err, browserlogin.ErrSession) {
		t.Fatal("foreign owner cancelled mixed batch")
	}
	if err := f.service.CancelWorkbenchRun("owner", view.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, view.ID)
	f.browsers.mu.Lock()
	closed := f.browsers.previous.closed
	f.browsers.mu.Unlock()
	select {
	case <-closed:
	default:
		t.Fatal("mixed cancellation did not close browser")
	}
	if _, err := f.service.PreviewWorkbenchRunResult(context.Background(), "owner", view.ID); !errors.Is(err, browserlogin.ErrSession) {
		t.Fatal("cancelled mixed batch issued final checkpoint")
	}
}

func TestMixedRunParentCancellationStopsOAuthAndDoesNotIssueCheckpoint(t *testing.T) {
	f := newBatchFixture(t, nil)
	view := startMixed(t, f, "owner@example.com", false)
	f.awaitTask(t, "account-workbench-oauth", "waiting")
	if !f.runner.CancelTask(view.TaskID) {
		t.Fatal("mixed parent task not cancellable")
	}
	f.awaitDone(t, view.ID)
	finished, err := f.service.ReadWorkbenchRun(context.Background(), "owner", view.ID)
	if err != nil || finished.Status != "cancelled" || finished.Available != 0 {
		t.Fatalf("parent cancellation = %+v %v", finished, err)
	}
}

func TestMixedRunCompletedBatchClosureRevokesDerivedPreview(t *testing.T) {
	f := newBatchFixture(t, nil)
	view := startMixed(t, f, mixedJSON, false)
	f.awaitDone(t, view.ID)
	preview, err := f.service.PreviewWorkbenchRunResult(context.Background(), "owner", view.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.CancelWorkbenchRun("owner", view.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Import(context.Background(), "owner", preview.ID, true); !errors.Is(err, accountworkbench.ErrPreview) {
		t.Fatal("closed mixed batch retained executable preview")
	}
}
