package accountworkbench_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestAccountCleanupRejectsAccountExportWithUnsupportedEnvelope(t *testing.T) {
	for _, field := range []string{"type", "version"} {
		t.Run(field, func(t *testing.T) {
			f := newProfileFixture(t)
			profile := saveProfile(t, f, "101")
			directory := filepath.Join(t.TempDir(), "exports")
			if err := f.service.UseExportDirectory(directory); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = f.service.CloseExports() })
			artifact := createCleanupExport(t, f, []string{"101"})
			path := filepath.Join(directory, artifact.ID+".json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var envelope map[string]json.RawMessage
			if err := json.Unmarshal(raw, &envelope); err != nil {
				t.Fatal(err)
			}
			envelope[field] = json.RawMessage(`2`)
			if field == "type" {
				envelope[field] = json.RawMessage(`"unsupported-account-data"`)
			}
			raw, err = json.Marshal(envelope)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := f.service.PreviewAccountCleanup(context.Background(), "owner", cleanupInput(profile)); !errors.Is(err, accountworkbench.ErrExportUnavailable) {
				t.Fatal("cleanup accepted an unsupported account export envelope", err)
			}
		})
	}
}

func TestAccountCleanupRetainsProfileExportWhenOneWorkspaceIdentityIsMissing(t *testing.T) {
	f := newProfileFixture(t)
	profile := saveProfile(t, f, "101")
	other := saveProfile(t, f, "102")
	directory := filepath.Join(t.TempDir(), "exports")
	if err := f.service.UseExportDirectory(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CloseExports() })
	selections := []accountworkbench.ProfileExportSelection{{ID: profile.ID, Revision: profile.Revision}, {ID: other.ID, Revision: other.Revision}}
	exportPreview, err := f.service.PreviewProfileExport(context.Background(), "owner", accountworkbench.ProfileExportInput{Items: selections})
	if err != nil {
		t.Fatal(err)
	}
	task, err := f.service.ExportProfiles(context.Background(), "owner", exportPreview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, task.ID)
	artifacts, err := f.service.Exports(context.Background(), "owner")
	if err != nil || len(artifacts) != 1 {
		t.Fatal("profile export fixture unavailable", err)
	}
	path := filepath.Join(directory, artifacts[0].ID+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	var profiles []map[string]json.RawMessage
	if err := json.Unmarshal(envelope["profiles"], &profiles); err != nil {
		t.Fatal(err)
	}
	delete(profiles[0], "workspace_id")
	envelope["profiles"], err = json.Marshal(profiles)
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	preview, err := f.service.PreviewAccountCleanup(context.Background(), "owner", accountworkbench.CleanupPreviewInput{Items: selections})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range preview.Items {
		if item.Kind == "profile_export" && item.ID == artifacts[0].ID {
			if item.Action != "retain" {
				t.Fatal("cleanup selected a profile export without complete workspace identities")
			}
			return
		}
	}
	t.Fatal("related incomplete profile export was absent from the retained scope")
}
