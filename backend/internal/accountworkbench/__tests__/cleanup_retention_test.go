package accountworkbench_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
)

func TestAccountCleanupRetainsHistoryWithoutProfileBindingAndSecurityWithoutTargetBinding(t *testing.T) {
	f := newProfileFixture(t)
	profile := saveProfile(t, f, "101")
	row := cleanupProfileRow(profile)
	row.ProfileID = ""
	cleanupHistoryTask(t, f, "unbound-history", "succeeded", []accountworkbench.OAuthBatchRow{row})
	directory := filepath.Join(t.TempDir(), "security")
	if err := f.service.UseSecurityDirectory(directory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CloseSecurity() })
	id := strings.Repeat("b", 32)
	if err := os.WriteFile(filepath.Join(directory, id+".json"), []byte(`{"version":1,"account_id":"101","user_id":"user-101","email":"owner@example.com","password":"private-result"}`), 0600); err != nil {
		t.Fatal(err)
	}
	preview, err := f.service.PreviewAccountCleanup(context.Background(), "owner", cleanupInput(profile))
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]string{}
	for _, item := range preview.Items {
		actions[item.Kind+":"+item.ID] = item.Action
	}
	if actions["history:unbound-history"] != "retain" || actions["security_result:"+id] != "retain" {
		t.Fatal("cleanup guessed missing target/profile linkage")
	}
	task, err := f.service.CleanupAccounts(context.Background(), "owner", preview.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, task.ID)
	if _, err := f.tasks.Get(context.Background(), "unbound-history"); err != nil {
		t.Fatal("unbound history removed", err)
	}
	if _, err := os.Stat(filepath.Join(directory, id+".json")); err != nil {
		t.Fatal("unbound security data removed", err)
	}
}

func TestAccountCleanupSelectsProfileExportsOnlyWhenAllProfilesBelongToSelection(t *testing.T) {
	f := newProfileFixture(t)
	profile := saveProfile(t, f, "101")
	other := saveProfile(t, f, "102")
	if err := f.service.UseExportDirectory(filepath.Join(t.TempDir(), "exports")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.service.CloseExports() })
	artifacts := []accountworkbench.ExportMetadata{}
	for _, selections := range [][]accountworkbench.ProfileExportSelection{
		{{ID: profile.ID, Revision: profile.Revision}},
		{{ID: profile.ID, Revision: profile.Revision}, {ID: other.ID, Revision: other.Revision}},
	} {
		preview, err := f.service.PreviewProfileExport(context.Background(), "owner", accountworkbench.ProfileExportInput{Items: selections})
		if err != nil {
			t.Fatal(err)
		}
		task, err := f.service.ExportProfiles(context.Background(), "owner", preview.ID, true)
		if err != nil {
			t.Fatal(err)
		}
		f.awaitDone(t, task.ID)
		values, err := f.service.Exports(context.Background(), "owner")
		if err != nil || len(values) == 0 {
			t.Fatal(err)
		}
		artifacts = append(artifacts, values[0])
	}
	preview, err := f.service.PreviewAccountCleanup(context.Background(), "owner", cleanupInput(profile))
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]string{}
	for _, item := range preview.Items {
		if item.Kind == "profile_export" {
			actions[item.ID] = item.Action
		}
	}
	if actions[artifacts[0].ID] != "delete" || actions[artifacts[1].ID] != "retain" {
		t.Fatal("profile export cleanup crossed selected profile scope")
	}
}
