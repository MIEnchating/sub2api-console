package accountworkbench_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/configstore"
)

func maintenanceProfileFixture(t *testing.T, checker checkerBoundary) (*batchFixture, configstore.WorkbenchMaintenance, string, string) {
	t.Helper()
	f := newProfileFixture(t)
	if checker != nil {
		f.service = accountworkbench.New(f.private, f.taskBoundary, f.business, checker, f.runBoundary)
		f.service.UseOAuthBrowser(f.browsers)
		f.service.UseOAuthAssistTicks(make(chan time.Time))
	}
	f.remote.accounts["101"]["error_message"] = "invalid_grant"
	f.remote.accounts["101"]["concurrency"] = json.Number("17")
	f.remote.accounts["101"]["group_ids"] = []any{json.Number("9")}
	saveProfile(t, f, "101")
	profileExchange(f, "user-101", "workspace-101")
	config, err := f.service.SaveMaintenance(context.Background(), configstore.WorkbenchMaintenance{Enabled: true, ReauthorizeWithProfiles: true, IntervalMinutes: 5, CooldownMinutes: 0, Model: "maintenance-test-model", CheckAfterRepair: checker != nil})
	if err != nil {
		t.Fatal(err)
	}
	owner, token := bindMaintenance(t, f, config.Revision)
	return f, config.WorkbenchMaintenance, owner, token
}

func TestMaintenanceReauthorizationImportsOriginalStableAccountAndPreservesOnlineConfiguration(t *testing.T) {
	checked := make(chan string, 1)
	f, config, owner, _ := maintenanceProfileFixture(t, checkerBoundary(func(_ context.Context, id, _ string, _ map[string]any, model string, _ int) (map[string]any, error) {
		checked <- id + ":" + model
		return map[string]any{"verdict": "SOL_CONSISTENT"}, nil
	}))
	task, err := f.service.CheckMaintenance(context.Background(), config.Revision, true)
	if err != nil {
		t.Fatal(err)
	}
	child := f.awaitTask(t, "account-workbench-oauth", "waiting")
	authorization, err := f.service.MaintenanceAuthorization(context.Background(), owner)
	if err != nil || !authorization.Attached || authorization.CurrentReauthorizationID == "" {
		t.Fatal("owner cannot take over maintenance authorization", err)
	}
	other, err := f.service.MaintenanceAuthorization(context.Background(), "other")
	if err != nil || other.CurrentReauthorizationID != "" || other.CurrentImportTaskID != "" {
		t.Fatal("maintenance leaked another session's task IDs", err)
	}
	if err := f.service.FinishOAuth(owner, child.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, task.ID)
	finished, err := f.tasks.Get(context.Background(), task.ID)
	if err != nil || finished.Status != "succeeded" {
		t.Fatalf("maintenance did not finish authorization and import: %s %s %v", finished.Status, finished.Message, err)
	}
	select {
	case value := <-checked:
		if value != "101:maintenance-test-model" {
			t.Fatal("maintenance ignored check model or original account")
		}
	default:
		t.Fatal("maintenance did not check imported credentials")
	}
	f.remote.mu.Lock()
	defer f.remote.mu.Unlock()
	account := f.remote.accounts["101"]
	if f.remote.created != 0 || account["credentials"].(map[string]any)["refresh_token"] != "rt_profile_private" {
		t.Fatal("maintenance did not update original stable account")
	}
	if account["concurrency"] != json.Number("17") || len(account["group_ids"].([]any)) != 1 || account["group_ids"].([]any)[0] != json.Number("9") || account["schedulable"] != true {
		t.Fatal("maintenance lost online configuration or failed to restore scheduling")
	}
	if f.remote.accounts["102"]["credentials"].(map[string]any)["refresh_token"] == "rt_profile_private" {
		t.Fatal("maintenance updated same-name unrelated account")
	}
	raw, _ := json.Marshal(finished)
	if strings.Contains(string(raw), "rt_profile_private") || strings.Contains(string(raw), "private-saved-password") {
		t.Fatal("maintenance task exposed credentials")
	}
}

func TestMaintenanceReauthorizationRevokedSessionCancelsWaitingBrowserWithoutWriting(t *testing.T) {
	f, config, owner, token := maintenanceProfileFixture(t, nil)
	task, err := f.service.CheckMaintenance(context.Background(), config.Revision, true)
	if err != nil {
		t.Fatal(err)
	}
	f.awaitTask(t, "account-workbench-oauth", "waiting")
	if err := f.private.RevokeSession(context.Background(), token); err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, task.ID)
	view, err := f.service.MaintenanceAuthorization(context.Background(), owner)
	if err != nil || view.Attached {
		t.Fatal("revoked session retained maintenance delegation", err)
	}
	finished, err := f.tasks.Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range finished.Result["items"].([]any) {
		if item.(map[string]any)["status"] == "waiting_input" {
			t.Fatal("completed maintenance left account waiting for revoked session")
		}
	}
	f.remote.mu.Lock()
	defer f.remote.mu.Unlock()
	if f.remote.created != 0 || f.remote.updates != 0 {
		t.Fatal("maintenance wrote after session revocation")
	}
}

func TestMaintenanceReauthorizationChangedConfigCancelsWaitingBrowserAndServiceRestartNeedsBinding(t *testing.T) {
	f, config, owner, _ := maintenanceProfileFixture(t, nil)
	restarted := accountworkbench.New(f.private, f.taskBoundary, f.business, nil, f.runBoundary)
	view, err := restarted.Maintenance(context.Background())
	if err != nil || view.ReauthorizationAttached {
		t.Fatal("service restart reused maintenance session delegation", err)
	}
	task, err := f.service.CheckMaintenance(context.Background(), config.Revision, true)
	if err != nil {
		t.Fatal(err)
	}
	f.awaitTask(t, "account-workbench-oauth", "waiting")
	config.ReauthorizeWithProfiles = false
	if _, err := f.service.SaveMaintenance(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, task.ID)
	authorization, err := f.service.MaintenanceAuthorization(context.Background(), owner)
	if err != nil || authorization.Attached {
		t.Fatal("disabled configuration kept maintenance delegation", err)
	}
	f.remote.mu.Lock()
	defer f.remote.mu.Unlock()
	if f.remote.created != 0 || f.remote.updates != 0 {
		t.Fatal("maintenance wrote after configuration changed")
	}
}

func TestMaintenanceReauthorizationImportResultPersistenceFailureFailsParentTask(t *testing.T) {
	f, config, owner, _ := maintenanceProfileFixture(t, nil)
	f.taskBoundary.failImportFinal = true
	task, err := f.service.CheckMaintenance(context.Background(), config.Revision, true)
	if err != nil {
		t.Fatal(err)
	}
	child := f.awaitTask(t, "account-workbench-oauth", "waiting")
	if err := f.service.FinishOAuth(owner, child.ID); err != nil {
		t.Fatal(err)
	}
	f.awaitDone(t, task.ID)
	finished, err := f.tasks.Get(context.Background(), task.ID)
	if err != nil || finished.Status != "failed" {
		t.Fatalf("parent ignored import result persistence failure: %s %v", finished.Status, err)
	}
}
