package accountworkbench_test

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/browserlogin"
)

func TestLocalOAuthWithoutManagementCheckpointRestoreKeepsExportOnlyScope(t *testing.T) {
	f := newCheckpointFixture(t)
	if err := f.private.Close(); err != nil {
		t.Fatal(err)
	}
	f.private = unconfiguredWorkbenchStore(t)
	f.state.Store = f.private
	f.configureService()
	f.service.UseTransport(oauthTransportFunc(func(*http.Request) (*http.Response, error) {
		t.Error("local OAuth contacted management endpoint")
		return nil, context.Canceled
	}))
	if err := f.service.UseExportDirectory(filepath.Join(t.TempDir(), "exports")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CloseExports() })
	view, err := f.service.StartOAuthWithInput(context.Background(), "checkpoint-owner", accountworkbench.OAuthStartInput{Scope: accountworkbench.ScopeLocalExport})
	if err != nil || view.Scope != accountworkbench.ScopeLocalExport {
		t.Fatal(err)
	}
	f.active = append(f.active, view.ID)
	f.phase(t, "waiting")
	checkpoint := f.save(t, view.ID)
	if checkpoint.Scope != accountworkbench.ScopeLocalExport {
		t.Fatal("local checkpoint lost export-only scope")
	}
	list, err := f.service.OAuthCheckpointsScoped(context.Background(), "checkpoint-owner", accountworkbench.ScopeLocalExport)
	if err != nil || len(list) != 1 || list[0].ID != checkpoint.ID {
		t.Fatalf("local checkpoint list = %+v, %v", list, err)
	}
	if _, err := f.service.RestoreOAuthCheckpoint(context.Background(), "checkpoint-owner", checkpoint.ID, accountworkbench.OAuthCheckpointAction{Revision: checkpoint.Revision, Confirmed: true}); err == nil {
		t.Fatal("local checkpoint restored into default managed scope")
	}
	restored, err := f.service.RestoreOAuthCheckpoint(context.Background(), "checkpoint-owner", checkpoint.ID, accountworkbench.OAuthCheckpointAction{Scope: accountworkbench.ScopeLocalExport, Revision: checkpoint.Revision, Confirmed: true})
	if err != nil || restored.Scope != accountworkbench.ScopeLocalExport {
		t.Fatal(err)
	}
	f.active = append(f.active, restored.ID)
	f.phase(t, "waiting")
	if err := f.service.InputOAuth(context.Background(), "checkpoint-owner", restored.ID, browserlogin.Input{Kind: "key", Key: "Enter"}); err != nil {
		t.Fatal(err)
	}
	if err := f.service.FinishOAuth("checkpoint-owner", restored.ID); err != nil {
		t.Fatal(err)
	}
	f.phase(t, "authorized")
	if _, err := f.service.PreviewOAuth(context.Background(), "checkpoint-owner", restored.ID, accountworkbench.OAuthPreviewInput{}); err == nil {
		t.Fatal("local OAuth credentials reached managed import preview")
	}
	preview, err := f.service.PreviewOAuth(context.Background(), "checkpoint-owner", restored.ID, accountworkbench.OAuthPreviewInput{Scope: accountworkbench.ScopeLocalExport, ExportOnly: true})
	if err != nil || preview.ID == "" || preview.Scope != accountworkbench.ScopeLocalExport || preview.Target != "" {
		t.Fatalf("local OAuth preview = %+v, %v", preview, err)
	}
	if _, err := f.service.ExportInput(context.Background(), "checkpoint-owner", preview.ID, true); err != nil {
		t.Fatal(err)
	}
	terminal := f.phase(t, "complete")
	if terminal.Status != "succeeded" || f.exchanges.Load() != 1 {
		t.Fatal("local OAuth did not export exactly one official exchange")
	}
	files, err := f.service.LocalExports(context.Background(), "checkpoint-owner")
	if err != nil || len(files) != 1 {
		t.Fatalf("local OAuth file = %+v, %v", files, err)
	}
	public, _ := json.Marshal([]any{view, checkpoint, list, restored, preview, terminal, files})
	if strings.Contains(string(public), "rt_checkpoint_private") || strings.Contains(string(public), "private-checkpoint-code") {
		t.Fatal("local OAuth exposed credential material")
	}
	list, err = f.service.OAuthCheckpointsScoped(context.Background(), "checkpoint-owner", accountworkbench.ScopeLocalExport)
	if err != nil || len(list) != 1 || list[0].Status != "restored" {
		t.Fatal("local checkpoint remained reusable")
	}
	if err := f.service.DeleteOAuthCheckpoint(context.Background(), "checkpoint-owner", checkpoint.ID, accountworkbench.OAuthCheckpointAction{Scope: accountworkbench.ScopeLocalExport, Revision: list[0].Revision, Confirmed: true}); err != nil {
		t.Fatal(err)
	}
}
