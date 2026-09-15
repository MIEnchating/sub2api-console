package accountworkbench_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/MIEnchating/sub2api-console/backend/internal/accountworkbench"
	"github.com/MIEnchating/sub2api-console/backend/internal/business"
)

func maintenanceFixture(t *testing.T, checker checkerBoundary) (*importFixture, accountworkbench.MaintenanceView) {
	t.Helper()
	f := newImportFixture(t, checker)
	f.existing()
	f.remote.accounts["101"]["status"] = "error"
	f.remote.accounts["101"]["error_message"] = "401 access token expired"
	config, err := f.service.Maintenance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return f, config
}

func TestMaintenanceFailedCheckKeepsPreviouslyActiveAccountPaused(t *testing.T) {
	f, config := maintenanceFixture(t, func(context.Context, string, string, map[string]any, string, int) (map[string]any, error) {
		return map[string]any{"verdict": "INCONCLUSIVE"}, nil
	})
	if _, err := f.service.CheckMaintenance(context.Background(), config.Revision, true); err != nil {
		t.Fatal(err)
	}
	f.await(t)
	if f.remote.refreshes != 1 || f.remote.accounts["101"]["schedulable"] != false {
		t.Fatal("failed behavioral check left refreshed credentials in active scheduling")
	}
}

func TestMaintenanceEquivalentIdentityAliasesAllowRecovery(t *testing.T) {
	f, config := maintenanceFixture(t, func(context.Context, string, string, map[string]any, string, int) (map[string]any, error) {
		return map[string]any{"verdict": "SOL_CONSISTENT"}, nil
	})
	f.remote.mutated = func(path string, account, _ map[string]any) {
		if path == "/openai/accounts/101/refresh" {
			credentials := account["credentials"].(map[string]any)
			delete(credentials, "chatgpt_account_id")
			delete(credentials, "chatgpt_user_id")
			credentials["account_id"], credentials["user_id"] = "workspace-1", "user-1"
		}
	}
	queued, err := f.service.CheckMaintenance(context.Background(), config.Revision, true)
	if err != nil {
		t.Fatal(err)
	}
	task := f.await(t)
	if task.Status != "succeeded" || f.remote.accounts["101"]["schedulable"] != true || f.remote.accounts["101"]["status"] != "active" {
		t.Fatal("equivalent stable identity did not recover")
	}
	view, err := f.service.Maintenance(context.Background())
	if err != nil || view.LastTaskID != queued.ID || view.LastRunAt == "" {
		t.Fatal("maintenance history does not identify the completed task")
	}
}

func TestMaintenanceMissingIdentityAfterRefreshStopsRecovery(t *testing.T) {
	f, config := maintenanceFixture(t, func(context.Context, string, string, map[string]any, string, int) (map[string]any, error) {
		return map[string]any{"verdict": "SOL_CONSISTENT"}, nil
	})
	f.remote.mutated = func(path string, account, _ map[string]any) {
		if path == "/openai/accounts/101/refresh" {
			account["credentials"] = map[string]any{"access_token": "access-refreshed-private", "refresh_token": "rt_refreshed_private"}
		}
	}
	if _, err := f.service.CheckMaintenance(context.Background(), config.Revision, true); err != nil {
		t.Fatal(err)
	}
	task := f.await(t)
	if task.Status == "succeeded" || f.remote.accounts["101"]["schedulable"] != false {
		t.Fatal("maintenance recovered account without confirming its original identity")
	}
}

func TestMaintenanceConfigChangeDuringCheckPreventsRecovery(t *testing.T) {
	var f *importFixture
	var config accountworkbench.MaintenanceView
	f, config = maintenanceFixture(t, func(ctx context.Context, _, _ string, _ map[string]any, _ string, _ int) (map[string]any, error) {
		changed := config.WorkbenchMaintenance
		changed.Model = "changed-model"
		_, err := f.service.SaveMaintenance(ctx, changed)
		return map[string]any{"verdict": "SOL_CONSISTENT"}, err
	})
	if _, err := f.service.CheckMaintenance(context.Background(), config.Revision, true); err != nil {
		t.Fatal(err)
	}
	task := f.await(t)
	if task.Status == "succeeded" || f.remote.accounts["101"]["schedulable"] != false {
		t.Fatal("changed maintenance configuration allowed obsolete recovery")
	}
}

func TestMaintenanceConfigChangeAfterPausePreventsCredentialRefresh(t *testing.T) {
	f, config := maintenanceFixture(t, nil)
	var changeErr error
	f.remote.mutated = func(path string, _ map[string]any, body map[string]any) {
		if path == "/accounts/101/schedulable" && body["schedulable"] == false {
			changed := config.WorkbenchMaintenance
			changed.Model = "changed-model"
			_, changeErr = f.service.SaveMaintenance(context.Background(), changed)
		}
	}
	if _, err := f.service.CheckMaintenance(context.Background(), config.Revision, true); err != nil {
		t.Fatal(err)
	}
	f.await(t)
	if changeErr != nil {
		t.Fatal(changeErr)
	}
	if f.remote.refreshes != 0 {
		t.Fatal("maintenance refreshed credentials after its configuration changed")
	}
}

func TestMaintenanceManualProtectionAddedDuringCheckPreventsRecovery(t *testing.T) {
	var f *importFixture
	var protectionErr error
	f, config := maintenanceFixture(t, func(ctx context.Context, _, _ string, _ map[string]any, _ string, _ int) (map[string]any, error) {
		protectionErr = f.business.CommitAccountControlReadback(ctx, "101", "pause", "test-admin", false, business.AccountOperation{OperationID: "manual-pause", OperationType: "pause", State: "succeeded", Phase: "readback", Actor: "test-admin", ObjectID: "101"})
		return map[string]any{"verdict": "SOL_CONSISTENT"}, protectionErr
	})
	_, err := f.business.SyncManagementSnapshot(context.Background(), []map[string]any{{"id": json.Number("101"), "name": "Original", "schedulable": true}}, []map[string]any{}, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.CheckMaintenance(context.Background(), config.Revision, true); err != nil {
		t.Fatal(err)
	}
	f.await(t)
	if protectionErr != nil {
		t.Fatal(protectionErr)
	}
	protection, err := f.business.AccountMutationProtection(context.Background(), "101")
	if err != nil || !protection.Paused {
		t.Fatal("manual pause fixture was not persisted")
	}
	if f.remote.accounts["101"]["schedulable"] != false {
		t.Fatal("automatic maintenance overrode manual pause")
	}
}

func TestMaintenanceRecoveryResetsTemporaryUnschedulabilityBeforeEnabling(t *testing.T) {
	f, config := maintenanceFixture(t, func(context.Context, string, string, map[string]any, string, int) (map[string]any, error) {
		return map[string]any{"verdict": "SOL_CONSISTENT"}, nil
	})
	f.remote.accounts["101"]["temp_unschedulable_reason"] = "401 access token expired"
	f.remote.accounts["101"]["temp_unschedulable_until"] = "2099-01-01T00:00:00Z"
	if _, err := f.service.CheckMaintenance(context.Background(), config.Revision, true); err != nil {
		t.Fatal(err)
	}
	task := f.await(t)
	account := f.remote.accounts["101"]
	if task.Status != "succeeded" || account["schedulable"] != true || account["temp_unschedulable_reason"] != nil || account["temp_unschedulable_until"] != nil {
		t.Fatal("maintenance declared recovery before temporary scheduling rejection was reset")
	}
}

func TestMaintenanceRecoveryReadbackRejectsUnclearedRuntimeState(t *testing.T) {
	f, config := maintenanceFixture(t, func(context.Context, string, string, map[string]any, string, int) (map[string]any, error) {
		return map[string]any{"verdict": "SOL_CONSISTENT"}, nil
	})
	f.remote.mutated = func(path string, account, _ map[string]any) {
		if path == "/accounts/101/recover-state" {
			account["overload_until"] = "2099-01-01T00:00:00Z"
		}
	}
	if _, err := f.service.CheckMaintenance(context.Background(), config.Revision, true); err != nil {
		t.Fatal(err)
	}
	task := f.await(t)
	if task.Status == "succeeded" || f.remote.accounts["101"]["schedulable"] != false {
		t.Fatal("maintenance enabled an account whose recovery state was not confirmed")
	}
}

func TestMaintenanceRecoveryDetectsConcurrentGroupChangeBeforeEnabling(t *testing.T) {
	f, config := maintenanceFixture(t, func(context.Context, string, string, map[string]any, string, int) (map[string]any, error) {
		return map[string]any{"verdict": "SOL_CONSISTENT"}, nil
	})
	f.remote.mutated = func(path string, account, _ map[string]any) {
		if path == "/accounts/101/clear-error" {
			account["group_ids"] = []any{json.Number("9")}
		}
	}
	if _, err := f.service.CheckMaintenance(context.Background(), config.Revision, true); err != nil {
		t.Fatal(err)
	}
	f.await(t)
	if f.remote.accounts["101"]["schedulable"] != false {
		t.Fatal("automatic recovery ignored a concurrent account group change")
	}
}
