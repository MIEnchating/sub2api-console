package accountworkbench_test

import (
	"context"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/taskstore"
)

func TestHistoricalReauthorizationUsesSavedStableIdentityAfterNewServiceStarts(t *testing.T) {
	f := newProfileFixture(t)
	profile := saveProfile(t, f, "101")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	row := accountworkbench.OAuthBatchRow{AccountID: "101", ProfileID: profile.ID, UserID: "user-101", WorkspaceID: "workspace-101", Email: profile.Email, Status: "failed"}
	if err := f.tasks.Save(context.Background(), taskstore.Task{ID: "historical-job", Skill: accountworkbench.Skill, Operation: "account-workbench-oauth-batch", Status: "failed", Message: "原授权中断", CreatedAt: now, UpdatedAt: now, Result: map[string]any{"items": []accountworkbench.OAuthBatchRow{row}}}); err != nil {
		t.Fatal(err)
	}
	service := accountworkbench.New(f.private, f.tasks, f.business, nil, f.runner)
	preview, err := service.PreviewReauthorization(context.Background(), "new-session", accountworkbench.ReauthorizationPreviewInput{SourceTaskID: "historical-job", FreshLogin: true})
	if err != nil || len(preview.Items) != 1 || preview.Items[0].AccountID != "101" || !preview.FreshLogin {
		t.Fatalf("historical fresh preview=%+v err=%v", preview, err)
	}
	service.DeleteOAuthBatchPreview("new-session", preview.ID)
	if _, err := service.PreviewReauthorization(context.Background(), "new-session", accountworkbench.ReauthorizationPreviewInput{SourceTaskID: "historical-job", AccountIDs: []string{"102"}, FreshLogin: true}); err == nil {
		t.Fatal("history selected an unrelated account")
	}
}

func TestHistoricalReauthorizationRejectsChangedIdentityAndActiveOrForeignTask(t *testing.T) {
	for _, scenario := range []string{"user", "workspace", "active", "foreign", "missing-profile"} {
		t.Run(scenario, func(t *testing.T) {
			f := newProfileFixture(t)
			profile := saveProfile(t, f, "101")
			now := time.Now().UTC().Format(time.RFC3339Nano)
			row := accountworkbench.OAuthBatchRow{AccountID: "101", ProfileID: profile.ID, UserID: "user-101", WorkspaceID: "workspace-101", Email: profile.Email, Status: "failed"}
			task := taskstore.Task{ID: "source", Skill: accountworkbench.Skill, Operation: "account-workbench-oauth-batch", Status: "failed", Message: "授权结果", CreatedAt: now, UpdatedAt: now}
			switch scenario {
			case "user":
				row.UserID = "other-user"
			case "workspace":
				row.WorkspaceID = "other-workspace"
			case "active":
				task.Status = "running"
			case "foreign":
				task.Skill = "other"
			case "missing-profile":
				row.ProfileID = "missing"
			}
			task.Result = map[string]any{"items": []accountworkbench.OAuthBatchRow{row}}
			if err := f.tasks.Save(context.Background(), task); err != nil {
				t.Fatal(err)
			}
			if _, err := f.service.PreviewReauthorization(context.Background(), "owner", accountworkbench.ReauthorizationPreviewInput{SourceTaskID: "source", FreshLogin: true}); err == nil {
				t.Fatal("invalid historical source started new preview")
			}
		})
	}
}
