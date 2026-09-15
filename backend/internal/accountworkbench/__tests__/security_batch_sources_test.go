package accountworkbench_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func completedSecuritySourceBatch(t *testing.T, f *securityBatchFixture, sameUser bool) accountworkbench.OAuthBatchView {
	t.Helper()
	preview, err := f.service.PreviewOAuthBatch(context.Background(), "owner", accountworkbench.OAuthBatchPreviewInput{Scope: accountworkbench.ScopeLocalExport, Content: `[{"email":"owner@example.com","workspace_id":"workspace-101"},{"email":"owner@example.com","workspace_id":"workspace-102"}]`})
	if err != nil || len(preview.Errors) != 0 {
		t.Fatalf("source preview: %v %v", err, preview.Errors)
	}
	view, err := f.service.StartOAuthBatch(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CancelOAuthBatch("owner", view.ID) })
	for _, id := range []string{"101", "102"} {
		child := f.awaitTask(t, "account-workbench-oauth", "waiting")
		user := "user-" + id
		if sameUser {
			user = "user-101"
		}
		profileExchange(f.batchFixture, user, "workspace-"+id)
		if err := f.service.FinishOAuth("owner", child.ID); err != nil {
			t.Fatal(err)
		}
	}
	f.awaitDone(t, view.ID)
	return view
}

func sourceBatchReferences(id string) []accountworkbench.SecuritySourceReference {
	first, second := 0, 1
	return []accountworkbench.SecuritySourceReference{{OAuthBatchID: id, Index: &first}, {OAuthBatchID: id, Index: &second}}
}

func TestSecurityBatchLocalOAuthResultsExecuteEveryStableIndexAndRemainExportable(t *testing.T) {
	f := newSecurityBatchFixture(t, nil)
	source := completedSecuritySourceBatch(t, f, false)
	if err := f.private.ConfigureTarget(context.Background(), "https://changed.example", "changed-test-key", 3); err != nil {
		t.Fatal(err)
	}
	preview, err := f.service.PreviewSecurityBatch(context.Background(), "owner", accountworkbench.SecurityBatchPreviewInput{Scope: accountworkbench.ScopeLocalExport, Sources: sourceBatchReferences(source.ID), Operation: "totp"})
	if err != nil || preview.ID == "" || len(preview.Errors) != 0 || len(preview.Items) != 2 {
		t.Fatalf("source security preview=%+v err=%v", preview, err)
	}
	*preview.Items[0].Source.Index = 400
	view, err := f.service.StartSecurityBatch(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CancelSecurityBatch("owner", view.ID) })
	first := f.finishSecurityChild(t, "totp")
	second := f.finishSecurityChild(t, "totp")
	f.awaitDone(t, view.ID)
	stored, err := f.service.ReadSecurityBatch("owner", view.ID)
	if err != nil || stored.Status != "succeeded" || stored.Succeeded != 2 || stored.Scope != accountworkbench.ScopeLocalExport {
		t.Fatalf("source batch=%+v err=%v", stored, err)
	}
	for index, row := range stored.Items {
		if row.AccountID != "" || row.Source == nil || row.Source.OAuthBatchID != source.ID || row.Source.Index == nil || *row.Source.Index != index || row.SecurityID != []string{first, second}[index] {
			t.Fatalf("source row binding=%+v", row)
		}
		child, err := f.service.Security(context.Background(), "owner", row.SecurityID, false)
		if err != nil || child.SourceOAuthBatchID != source.ID || child.SourceIndex == nil || *child.SourceIndex != index {
			t.Fatalf("completed source child lost binding: %+v %v", child, err)
		}
		*child.SourceIndex = 400
		unchanged, err := f.service.Security(context.Background(), "owner", row.SecurityID, false)
		if err != nil || unchanged.SourceIndex == nil || *unchanged.SourceIndex != index {
			t.Fatal("returned source index mutated the session binding")
		}
		raw, err := os.ReadFile(filepath.Join(f.directory, row.ArtifactID+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var artifact struct {
			BatchID   string `json:"source_oauth_batch_id"`
			Index     *int   `json:"source_index"`
			AccountID string `json:"account_id"`
		}
		if json.Unmarshal(raw, &artifact) != nil || artifact.BatchID != source.ID || artifact.Index == nil || *artifact.Index != index || artifact.AccountID != "" {
			t.Fatal("source private artifact not bound to original result")
		}
	}
	export, err := f.service.PreviewOAuthBatchImport(context.Background(), "owner", source.ID, accountworkbench.OAuthPreviewInput{Scope: accountworkbench.ScopeLocalExport, ExportOnly: true})
	if err != nil || len(export.Items) != 2 {
		t.Fatalf("original results no longer exportable: %v", err)
	}
}

func TestSecurityBatchSourceDuplicateOfficialUserFailsWholePreview(t *testing.T) {
	f := newSecurityBatchFixture(t, nil)
	source := completedSecuritySourceBatch(t, f, true)
	preview, err := f.service.PreviewSecurityBatch(context.Background(), "owner", accountworkbench.SecurityBatchPreviewInput{Scope: accountworkbench.ScopeLocalExport, Sources: sourceBatchReferences(source.ID), Operation: "totp"})
	if err != nil || preview.ID != "" || len(preview.Items) != 0 || len(preview.Errors) != 1 {
		t.Fatalf("duplicate user accepted: %+v %v", preview, err)
	}
}

func TestSecurityBatchSourceCancelledAfterPreviewCannotStart(t *testing.T) {
	f := newSecurityBatchFixture(t, nil)
	source := completedSecuritySourceBatch(t, f, false)
	preview, err := f.service.PreviewSecurityBatch(context.Background(), "owner", accountworkbench.SecurityBatchPreviewInput{Scope: accountworkbench.ScopeLocalExport, Sources: sourceBatchReferences(source.ID), Operation: "totp"})
	if err != nil || preview.ID == "" {
		t.Fatalf("preview: %v", err)
	}
	if err := f.service.CancelOAuthBatch("owner", source.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.StartSecurityBatch(context.Background(), "owner", preview.ID, true); err == nil {
		t.Fatal("revoked OAuth result started safety batch")
	}
	f.factory.mu.Lock()
	defer f.factory.mu.Unlock()
	if f.factory.opened != 0 {
		t.Fatal("revoked source reached official browser")
	}
}

func TestSecurityBatchSourceRejectsAmbiguousReferencesAndManagedIDs(t *testing.T) {
	f, source := sourceSecurityFixture(t, accountworkbench.ScopeLocalExport, sourceSecurityClaims)
	for _, input := range []accountworkbench.SecurityBatchPreviewInput{
		{Scope: accountworkbench.ScopeLocalExport, AccountIDs: []string{"101"}, Sources: []accountworkbench.SecuritySourceReference{{OAuthID: source.ID}}, Operation: "totp"},
		{Scope: accountworkbench.ScopeLocalExport, Sources: []accountworkbench.SecuritySourceReference{{OAuthID: source.ID, OAuthBatchID: source.ID}}, Operation: "totp"},
		{Scope: accountworkbench.ScopeLocalExport, Sources: []accountworkbench.SecuritySourceReference{{OAuthID: source.ID}, {OAuthID: source.ID}}, Operation: "totp"},
	} {
		if _, err := f.service.PreviewSecurityBatch(context.Background(), "security-owner", input); err == nil {
			t.Fatal("ambiguous source preview accepted")
		}
	}
}

func TestSecurityBatchSingleOAuthSourceRetainsOriginalAfterTask(t *testing.T) {
	f, source := sourceSecurityFixture(t, accountworkbench.ScopeLocalExport, sourceSecurityClaims)
	preview, err := f.service.PreviewSecurityBatch(context.Background(), "security-owner", accountworkbench.SecurityBatchPreviewInput{Scope: accountworkbench.ScopeLocalExport, Sources: []accountworkbench.SecuritySourceReference{{OAuthID: source.ID}}, Operation: "totp"})
	if err != nil || preview.ID == "" {
		t.Fatalf("preview: %v", err)
	}
	view, err := f.service.StartSecurityBatch(context.Background(), "security-owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CancelSecurityBatch("security-owner", view.ID) })
	child := f.awaitSecurity(t, "waiting")
	if err := f.service.ContinueSecurity(context.Background(), "security-owner", child.ID); err != nil {
		t.Fatal(err)
	}
	for {
		task := f.awaitSecurity(t, "succeeded")
		if task.ID == view.ID {
			break
		}
	}
	if _, err := f.service.SecuritySource(context.Background(), "security-owner", source.ID); err != nil {
		t.Fatalf("batch consumed original OAuth result: %v", err)
	}
}

func TestSecurityBatchCheckpointChildRequiresIdentityConfirmationAndRemainsAvailable(t *testing.T) {
	f, _, source := checkpointSecurityFixture(t, accountworkbench.ScopeLocalExport)
	preview, err := f.service.PreviewSecurityBatch(context.Background(), "checkpoint-owner", accountworkbench.SecurityBatchPreviewInput{Scope: source.Scope, Sources: []accountworkbench.SecuritySourceReference{{CheckpointID: source.ID, Revision: source.Revision, CheckpointRevision: source.CheckpointRevision}}, Operation: "totp"})
	if err != nil || preview.ID == "" || preview.Items[0].UserID != "" {
		t.Fatalf("checkpoint batch preview=%+v err=%v", preview, err)
	}
	view, err := f.service.StartSecurityBatch(context.Background(), "checkpoint-owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CancelSecurityBatch("checkpoint-owner", view.ID) })
	child := f.phase(t, "awaiting_confirmation")
	confirmCheckpointIdentity(t, f, child.ID)
	if err := f.service.ContinueSecurity(context.Background(), "checkpoint-owner", child.ID); err != nil {
		t.Fatal(err)
	}
	for {
		task := f.phase(t, "succeeded")
		if task.ID == view.ID {
			break
		}
	}
	stored, err := f.service.ReadSecurityBatch("checkpoint-owner", view.ID)
	if err != nil || stored.Items[0].SecurityID != child.ID || stored.Items[0].UserID != "user-1" {
		t.Fatalf("checkpoint result row=%+v %v", stored, err)
	}
	if _, err := f.service.Security(context.Background(), "checkpoint-owner", child.ID, false); err != nil {
		t.Fatalf("completed checkpoint child not retained: %v", err)
	}
}

func TestSecurityBatchCheckpointConfirmedUserCannotBeWrittenAgainByLaterOAuthSource(t *testing.T) {
	f, browser, checkpoint := checkpointSecurityFixture(t, accountworkbench.ScopeLocalExport)
	source, err := f.service.StartOAuthWithInput(context.Background(), "checkpoint-owner", accountworkbench.OAuthStartInput{Scope: checkpoint.Scope})
	if err != nil {
		t.Fatal(err)
	}
	f.active = append(f.active, source.ID)
	f.phase(t, "waiting")
	if err := f.service.InputOAuth(context.Background(), "checkpoint-owner", source.ID, browserlogin.Input{Kind: "key", Key: "Tab"}); err != nil {
		t.Fatal(err)
	}
	if err := f.service.FinishOAuth("checkpoint-owner", source.ID); err != nil {
		t.Fatal(err)
	}
	f.phase(t, "authorized")
	select {
	case <-f.completed:
	case <-time.After(5 * time.Second):
		t.Fatal("source browser did not close")
	}
	preview, err := f.service.PreviewSecurityBatch(context.Background(), "checkpoint-owner", accountworkbench.SecurityBatchPreviewInput{Scope: checkpoint.Scope, Operation: "totp", Sources: []accountworkbench.SecuritySourceReference{{CheckpointID: checkpoint.ID, Revision: checkpoint.Revision, CheckpointRevision: checkpoint.CheckpointRevision}, {OAuthID: source.ID}}})
	if err != nil || preview.ID == "" {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	view, err := f.service.StartSecurityBatch(context.Background(), "checkpoint-owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CancelSecurityBatch("checkpoint-owner", view.ID) })
	child := f.phase(t, "awaiting_confirmation")
	confirmCheckpointIdentity(t, f, child.ID)
	if err := f.service.ContinueSecurity(context.Background(), "checkpoint-owner", child.ID); err != nil {
		t.Fatal(err)
	}
	f.phase(t, "failed")
	stored, err := f.service.ReadSecurityBatch("checkpoint-owner", view.ID)
	if err != nil || stored.Succeeded != 1 || stored.Items[1].SecurityID != "" {
		t.Fatalf("later duplicate user reached safety browser: %+v %v", stored, err)
	}
	browser.mu.Lock()
	defer browser.mu.Unlock()
	if browser.enrolls != 1 || browser.activations != 1 {
		t.Fatal("duplicate user was changed more than once")
	}
}
