package accountworkbench_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func TestSourceProfileExportReportsLocalScopeForItsPrivateArtifact(t *testing.T) {
	f, _, requests := localExportFixture(t)
	owner := "local-owner"
	digest := sha256.Sum256([]byte(owner))
	profile, err := f.private.SaveWorkbenchSourceProfile(context.Background(), configstore.WorkbenchSourceProfile{
		ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", OwnerHash: hex.EncodeToString(digest[:]), Scope: string(accountworkbench.ScopeLocalExport),
		UserID: "user-1", WorkspaceID: "workspace-1", Email: "local@example.com", HasPassword: true,
		Login: json.RawMessage(`{"email":"local@example.com","workspace_id":"workspace-1","password":"private-source-password"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.service.PreviewSourceProfileExport(context.Background(), owner, accountworkbench.SourceProfileSelectionInput{
		Scope: accountworkbench.ScopeLocalExport, Items: []accountworkbench.ProfileExportSelection{{ID: profile.ID, Revision: profile.Revision}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ExportSourceProfiles(context.Background(), owner, preview.ID, true); err != nil {
		t.Fatal(err)
	}
	task := f.await(t)
	if task.Status != "succeeded" {
		t.Fatalf("source profile export failed: %+v", task)
	}
	rows := task.Result["items"].([]accountworkbench.ResultItem)
	if len(rows) != 1 || rows[0].Report["scope"] != accountworkbench.ScopeLocalExport {
		t.Fatalf("independent source profile artifact has incorrect task scope: %+v", rows)
	}
	artifacts, err := f.service.LocalExports(context.Background(), owner)
	if err != nil || len(artifacts) != 1 || artifacts[0].ID != rows[0].Report["artifact_id"] || artifacts[0].Kind != accountworkbench.ExportLoginProfiles {
		t.Fatalf("task artifact does not belong to local exports: %+v, %v", artifacts, err)
	}
	if requests.Load() != 0 {
		t.Fatal("source profile export contacted the management target")
	}
}
